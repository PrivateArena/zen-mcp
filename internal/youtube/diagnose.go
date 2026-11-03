package youtube

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Diag is the bounded diagnostic record one action produces (§4.6): the maxDiagBytes newest bytes of any
// reply, the tab id, the screenshot paths, and the headroom the next wait has. It is what makes a stuck
// wizard debuggable from the tool reply instead of from a browser devtools session.
type Diag struct {
	mu        sync.Mutex
	container string
	maxBytes  int
	dir       string
	entries   []DiagEntry
	shots     []string
	start     time.Time
}

// DiagEntry is one bridge call, compressed to what a human needs to spot a wrong turn.
type DiagEntry struct {
	At      time.Time `json:"at"`
	Action  string    `json:"action"`
	Subject string    `json:"subject"`
	Result  string    `json:"result"`
}

// NewDiag builds the record for one action. dir is where screenshots land (may be empty to skip
// evidence); maxBytes bounds every stored string, so the ring is bounded by count as well as size.
func NewDiag(container string, maxBytes int, dir string) (*Diag, error) {
	if maxBytes <= 0 {
		maxBytes = 8192
	}
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create diagnostics dir %s: %w", dir, err)
		}
	}
	return &Diag{container: container, maxBytes: maxBytes, dir: dir, start: time.Now()}, nil
}

func (d *Diag) Container() string {
	if d == nil {
		return ""
	}
	return d.container
}

// MaxDiagBytes is the per-string bound, handed to the page programs so the cap is applied where the data
// is collected rather than after it has crossed the wire.
func (d *Diag) MaxDiagBytes() int {
	if d == nil || d.maxBytes <= 0 {
		return 8192
	}
	if d.maxBytes < 256 {
		return 256
	}
	if d.maxBytes > 65536 {
		return 65536
	}
	return d.maxBytes
}

func (d *Diag) Note(action, subject, result string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.appendLocked(action, subject, result)
}

// appendLocked is the body of Note, for callers that already hold the mutex.
func (d *Diag) appendLocked(action, subject, result string) {
	d.entries = append(d.entries, DiagEntry{
		At:      time.Now(),
		Action:  action,
		Subject: d.clip(subject),
		Result:  d.clip(result),
	})
	// The ring is bounded in entries as well as bytes-per-entry: a long wait loop would otherwise grow
	// it without limit even though each line is short.
	if over := len(d.entries) - 200; over > 0 {
		d.entries = append([]DiagEntry(nil), d.entries[over:]...)
	}
}

// Shot records evidence the flow captured. An unwritable path is reported through the normal error
// channel by the caller; here it is only noted.
func (d *Diag) Shot(path string, err error) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	result := "saved"
	if err != nil {
		result = err.Error()
	} else {
		d.shots = append(d.shots, path)
	}
	// Recorded inline: Note takes the same mutex, and calling it from here would deadlock the first
	// screenshot of every run.
	d.appendLocked("screenshot", filepath.Base(path), result)
}

// Dir is where a flow should write its evidence, or "" when none was configured.
func (d *Diag) Dir() string {
	if d == nil {
		return ""
	}
	return d.dir
}

// Tail returns the last n entries, oldest first — what goes into a tool reply.
func (d *Diag) Tail(n int) []DiagEntry {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if n <= 0 || n > len(d.entries) {
		n = len(d.entries)
	}
	out := make([]DiagEntry, n)
	copy(out, d.entries[len(d.entries)-n:])
	return out
}

// Screenshots lists the evidence paths actually written.
func (d *Diag) Screenshots() []string {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.shots...)
}

// Reqs converts an elapsed duration plus a per-action safety margin into the `timeout` ms a wait
// action should carry — the only place the wizard's long budget is spent. Callers pass the *remaining*
// headroom; a negative budget is clamped to the floor so a wait can never be issued with timeout 0,
// which the bridge reads as "forever".
func (d *Diag) Reqs(elapsed time.Duration, budget time.Duration) int {
	const floorMS = 2000
	left := budget - elapsed
	if left < 0 {
		left = 0
	}
	ms := int(left / time.Millisecond)
	if ms < floorMS {
		ms = floorMS
	}
	return ms
}

// Elapsed is the wall clock since the action started, for the budget maths.
func (d *Diag) Elapsed() time.Duration {
	if d == nil {
		return 0
	}
	return time.Since(d.start)
}

func (d *Diag) clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= d.MaxDiagBytes() {
		return s
	}
	return s[:d.MaxDiagBytes()-3] + "..."
}

// String renders the tail for a log line or a tool reply.
func (d *Diag) String() string {
	entries := d.Tail(12)
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s %s %s -> %s\n", e.At.Format("15:04:05"), e.Action, e.Subject, e.Result)
	}
	return b.String()
}
