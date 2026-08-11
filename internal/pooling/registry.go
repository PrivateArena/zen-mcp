package pooling

//
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
)

// Pool job states, mirrored in the pool tool's running/cancelled payloads and
// the pool's JSON output.
const (
	StateRunning   = "running"
	StateDone      = "done"
	StateCancelled = "cancelled"
	StateUnknown   = "unknown"
)

// ErrRegistryFull is returned by Register when the job cap is reached and no
// expired slot can be reclaimed.
var ErrRegistryFull = errors.New("pool registry full: too many concurrent long-running jobs")

// Job is a single in-flight (or completed) pooled tool call. Result is set and
// Done is closed exactly once, by Complete; any reader that observes the
// closed Done channel is guaranteed (via channel happens-before) to see Result.
type Job struct {
	ID         string
	Cancelled  bool
	Done       chan struct{}
	Result     *mcp.CallToolResult
	CreatedAt  time.Time
	FinishedAt time.Time
}

// JobInfo is the externally visible summary of one job, as returned by List.
type JobInfo struct {
	ID    string `json:"id"`
	State string `json:"state"`
	AgeMs int64  `json:"ageMs"`
}

// PollOutcome is the result of a LongPoll.
type PollOutcome struct {
	State  string
	Result *mcp.CallToolResult
}

// Registry is an in-memory, process-local job store. It is safe for
// concurrent use. State is intentionally not persisted: a server restart
// surfaces as StateUnknown so the LLM re-issues the original tool call.
type Registry struct {
	mu    sync.Mutex
	jobs  map[string]*Job
	ttl   time.Duration // eviction age from CreatedAt for running/cancelled jobs
	grace time.Duration // eviction age from FinishedAt for done jobs
	max   int
}

// NewRegistry builds a registry. Non-positive values fall back to sane
// defaults (60m TTL/grace, 256 jobs).
func NewRegistry(ttl, grace time.Duration, max int) *Registry {
	if ttl <= 0 {
		ttl = time.Hour
	}
	if grace <= 0 {
		grace = time.Hour
	}
	if max <= 0 {
		max = 256
	}
	return &Registry{jobs: make(map[string]*Job), ttl: ttl, grace: grace, max: max}
}

// Register stores job and returns the pool_id under which it is stored. If
// job.ID is empty a fresh id is generated and assigned to job.ID. The returned
// id is the ONLY id that exists for this job: it is the map key, the value of
// job.ID, and the value handed to the client for polling.
func (r *Registry) Register(job *Job) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.jobs) >= r.max {
		r.evictLocked(time.Now())
	}
	if len(r.jobs) >= r.max {
		return "", ErrRegistryFull
	}
	if job.ID == "" {
		job.ID = newPoolID()
	} else if _, exists := r.jobs[job.ID]; exists {
		return "", errors.New("duplicate pool_id " + job.ID)
	}
	job.Done = make(chan struct{})
	job.CreatedAt = time.Now()
	r.jobs[job.ID] = job
	return job.ID, nil
}

// Complete stores the finished result and closes job.Done. The Result write
// happens-before the close, so any LongPoll observing the closed channel sees
// the result. Returns false when the job has already been evicted.
func (r *Registry) Complete(id string, res *mcp.CallToolResult) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return false
	}
	select {
	case <-job.Done:
		return false
	default:
	}
	job.Result = res
	job.FinishedAt = time.Now()
	close(job.Done)
	return true
}

// Cancel softly marks the job as cancelled. The underlying tool call is NOT
// killed; the pool tool reports cancelled for this id.
func (r *Registry) Cancel(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return false
	}
	job.Cancelled = true
	return true
}

// Get returns the held *Job for id. The pointer stays valid even after the
// reaper evicts the id from the map; callers must re-Get to confirm liveness.
func (r *Registry) Get(id string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	return job, ok
}

// LongPoll blocks until the job finishes, the wait window elapses, the context
// is done, or the job is cancelled. It NEVER creates a job: an unknown id
// yields StateUnknown so callers surface an explicit error instead of silently
// starting a second job (which would mint a second pool_id).
func (r *Registry) LongPoll(ctx context.Context, id string, wait time.Duration) PollOutcome {
	job, ok := r.Get(id)
	if !ok {
		return PollOutcome{State: StateUnknown}
	}
	if r.cancelled(job) {
		return PollOutcome{State: StateCancelled}
	}
	if wait <= 0 {
		wait = time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-job.Done:
		return PollOutcome{State: StateDone, Result: job.Result}
	case <-timer.C:
	case <-ctx.Done():
	}
	if r.cancelled(job) {
		return PollOutcome{State: StateCancelled}
	}
	return PollOutcome{State: StateRunning}
}

// State reports the current state of id without blocking: running, done,
// cancelled, or unknown. It never creates a job for a missing id.
func (r *Registry) State(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return StateUnknown
	}
	return r.jobStateLocked(job)
}

// List returns a snapshot of all jobs, newest first.
func (r *Registry) List() []JobInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	out := make([]JobInfo, 0, len(r.jobs))
	for id, job := range r.jobs {
		out = append(out, JobInfo{ID: id, State: r.jobStateLocked(job), AgeMs: now.Sub(job.CreatedAt).Milliseconds()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgeMs > out[j].AgeMs })
	return out
}

// EvictExpired removes jobs past their TTL (running/cancelled, from CreatedAt)
// or past their post-completion grace (done, from FinishedAt). Returns the
// number evicted. Running jobs whose background goroutine later completes are
// dropped silently by Complete (map miss).
func (r *Registry) EvictExpired(now time.Time) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.evictLocked(now)
}

func (r *Registry) evictLocked(now time.Time) int {
	var evicted int
	for id, job := range r.jobs {
		done := false
		select {
		case <-job.Done:
			done = true
		default:
		}
		if done {
			if !job.FinishedAt.IsZero() && now.Sub(job.FinishedAt) > r.grace {
				delete(r.jobs, id)
				evicted++
			}
		} else if now.Sub(job.CreatedAt) > r.ttl {
			delete(r.jobs, id)
			evicted++
		}
	}
	return evicted
}

// cancelled reports the soft-cancel flag under lock.
func (r *Registry) cancelled(job *Job) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return job.Cancelled
}

func (r *Registry) jobStateLocked(job *Job) string {
	if job.Cancelled {
		return StateCancelled
	}
	select {
	case <-job.Done:
		return StateDone
	default:
		return StateRunning
	}
}

func newPoolID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "pool-" + hex.EncodeToString(b)
}

// ---- process-wide singleton (shared by both HTTP listeners) ----

var (
	globalOnce sync.Once
	globalReg  *Registry
)

// Global returns the process-wide registry shared by the wrapped tool calls
// and the pool tool. TTL/grace/cap are snapshotted from config.json at first
// use; the enabled/elapsedMs knobs are re-read per call for live toggling.
func Global() *Registry {
	globalOnce.Do(func() {
		pc := mcpcfg.Get().Pooling
		globalReg = NewRegistry(
			time.Duration(pc.TTLMinutes)*time.Minute,
			time.Duration(pc.TTLMinutes)*time.Minute,
			pc.MaxJobs,
		)
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				if n := globalReg.EvictExpired(time.Now()); n > 0 {
					logfilter.Infof("[pooling] evicted %d expired jobs", n)
				}
			}
		}()
	})
	return globalReg
}
