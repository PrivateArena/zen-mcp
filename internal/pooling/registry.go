package pooling

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
)

var ErrRegistryFull = errors.New("pool registry full")

type Job struct {
	ID         string
	Cancelled  bool
	Done       chan struct{}
	Result     *mcp.CallToolResult
	CreatedAt  time.Time
	FinishedAt time.Time
}

type JobInfo struct {
	ID    string
	State string
	AgeMs int64
}

type PollOutcome struct {
	State  string
	Result *mcp.CallToolResult
}

type Registry struct {
	mu    sync.Mutex
	jobs  map[string]*Job
	ttl   time.Duration
	grace time.Duration
	max   int
}

func NewRegistry(ttl, grace time.Duration, max int) *Registry {
	return &Registry{
		jobs:  make(map[string]*Job, max),
		ttl:   ttl,
		grace: grace,
		max:   max,
	}
}

func (r *Registry) Register(job *Job) (string, error) {
	if job.Done == nil {
		job.Done = make(chan struct{})
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.max > 0 && r.countLocked() >= r.max {
		if err := r.evictOldestExpiredLocked(time.Now()); err != nil {
			return "", err
		}
		if r.countLocked() >= r.max {
			return "", ErrRegistryFull
		}
	}

	id, err := newJobID()
	if err != nil {
		return "", err
	}
	job.ID = id
	r.jobs[id] = job
	return id, nil
}

func (r *Registry) Complete(id string, res *mcp.CallToolResult) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return false
	}
	job.Result = res
	job.FinishedAt = time.Now()
	close(job.Done)
	return true
}

func (r *Registry) Cancel(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok || job.Cancelled {
		return false
	}
	job.Cancelled = true
	return true
}

func (r *Registry) Get(id string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	return job, ok
}

func (r *Registry) LongPoll(ctx context.Context, id string, wait time.Duration) PollOutcome {
	r.mu.Lock()
	job, ok := r.jobs[id]
	if !ok {
		r.mu.Unlock()
		return PollOutcome{State: "unknown", Result: nil}
	}
	r.mu.Unlock()

	if job.Cancelled {
		return PollOutcome{State: "cancelled", Result: nil}
	}

	if isDone(job) {
		return PollOutcome{State: "done", Result: job.Result}
	}

	if wait <= 0 {
		return PollOutcome{State: "running", Result: nil}
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-job.Done:
		return PollOutcome{State: "done", Result: job.Result}
	case <-timer.C:
		return PollOutcome{State: "running", Result: nil}
	case <-ctx.Done():
		return PollOutcome{State: "running", Result: nil}
	}
}

func (r *Registry) List() []JobInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]JobInfo, 0, len(r.jobs))
	now := time.Now()
	for _, job := range r.jobs {
		state := "running"
		if job.Cancelled {
			state = "cancelled"
		}
		select {
		case <-job.Done:
			state = "done"
		default:
		}
		age := now.Sub(job.CreatedAt).Milliseconds()
		if state == "done" && !job.FinishedAt.IsZero() {
			age = now.Sub(job.FinishedAt).Milliseconds()
		}
		out = append(out, JobInfo{ID: job.ID, State: state, AgeMs: age})
	}
	return out
}

func (r *Registry) EvictExpired(now time.Time) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.evictExpiredLocked(now)
}

func (r *Registry) evictExpiredLocked(now time.Time) int {
	var evicted int
	for id, job := range r.jobs {
		switch {
		case job.Cancelled:
			if now.Sub(job.CreatedAt) > r.ttl {
				delete(r.jobs, id)
				evicted++
			}
		case isDone(job):
			if !job.FinishedAt.IsZero() && now.Sub(job.FinishedAt) > r.grace {
				delete(r.jobs, id)
				evicted++
			}
		default:
			if now.Sub(job.CreatedAt) > r.ttl {
				delete(r.jobs, id)
				evicted++
			}
		}
	}
	return evicted
}

func (r *Registry) evictOldestExpiredLocked(now time.Time) error {
	var oldestID string
	var oldestTime time.Time
	for id, job := range r.jobs {
		if job.Cancelled || isDone(job) {
			ref := job.CreatedAt
			if isDone(job) && !job.FinishedAt.IsZero() {
				ref = job.FinishedAt
			}
			if oldestID == "" || ref.Before(oldestTime) {
				oldestID = id
				oldestTime = ref
			}
		} else if now.Sub(job.CreatedAt) > r.ttl {
			if oldestID == "" || job.CreatedAt.Before(oldestTime) {
				oldestID = id
				oldestTime = job.CreatedAt
			}
		}
	}
	if oldestID == "" {
		return ErrRegistryFull
	}
	delete(r.jobs, oldestID)
	return nil
}

func (r *Registry) countLocked() int {
	return len(r.jobs)
}

func isDone(job *Job) bool {
	select {
	case <-job.Done:
		return true
	default:
		return false
	}
}

func newJobID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "pool-" + hex.EncodeToString(b), nil
}

var globalOnce sync.Once
var globalRegistry *Registry
var globalReaperStop func()

func Global() *Registry {
	globalOnce.Do(func() {
		cfg := mcpcfg.Get().Pooling
		ttl := time.Duration(cfg.TTLMinutes) * time.Minute
		if ttl <= 0 {
			ttl = 60 * time.Minute
		}
		grace := ttl
		max := cfg.MaxJobs
		if max <= 0 {
			max = 256
		}
		globalRegistry = NewRegistry(ttl, grace, max)
		ticker := time.NewTicker(60 * time.Second)
		var done chan struct{}
		done = make(chan struct{})
		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					globalRegistry.EvictExpired(time.Now())
				case <-done:
					return
				}
			}
		}()
		globalReaperStop = func() { close(done) }
	})
	return globalRegistry
}

// SetGlobalRegistryForTest replaces the global registry function for tests.
// It does not stop any running reaper goroutine.
func SetGlobalRegistryForTest(fn func() *Registry) {
	globalRegistryFn = fn
}

// ResetGlobalForTest stops the reaper and resets the once so the next Global()
// call reinitializes from the current config.
func ResetGlobalForTest() {
	if globalReaperStop != nil {
		globalReaperStop()
	}
	globalOnce = sync.Once{}
	globalRegistry = nil
	globalRegistryFn = Global
}

var globalRegistryFn func() *Registry = Global

// GlobalRegistry returns the current global registry function. For tests,
// use SetGlobalRegistryForTest to inject a custom registry.
func GlobalRegistry() *Registry {
	return globalRegistryFn()
}
