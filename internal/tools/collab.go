package tools

import "sync"

// CollaborationState is the lifecycle of a pending collaborative capture.
// A collaboration is settled exactly once: either the HTTP /api/collaborate
// POST resolves it, or the capture tool's 60s timeout expires it. Any later
// Resolve call is a no-op returning false.
type CollaborationState int

const (
	CollabPending CollaborationState = iota
	CollabResolved
	CollabExpired
)

type collabEntry struct {
	resolve func(string)
	state   CollaborationState
}

// CollaborationRegistry is a mutex-guarded registry of pending collaborative
// capture sessions shared by the capture tool (writer) and the /api/collaborate
// route (reader/deleter). It replaces the racy raw map (F5) and guarantees a
// single-owner resolve so the timeout-vs-HTTP race cannot double-resolve or
// drop a late path (F11).
type CollaborationRegistry struct {
	mu    sync.Mutex
	items map[string]*collabEntry
}

func NewCollaborationRegistry() *CollaborationRegistry {
	return &CollaborationRegistry{items: make(map[string]*collabEntry)}
}

// Register records a pending collaboration for id. A later Resolve invokes
// resolve exactly once. Nil-receiver and nil resolve are tolerated.
func (r *CollaborationRegistry) Register(id string, resolve func(string)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = make(map[string]*collabEntry)
	}
	r.items[id] = &collabEntry{resolve: resolve, state: CollabPending}
}

// Resolve claims a pending collaboration and invokes its callback with path.
// It returns true only for the single caller that wins the claim; unknown,
// already-resolved, and expired ids return false.
func (r *CollaborationRegistry) Resolve(id, path string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	e, ok := r.items[id]
	if !ok || e.state != CollabPending {
		r.mu.Unlock()
		return false
	}
	e.state = CollabResolved
	delete(r.items, id)
	resolve := e.resolve
	r.mu.Unlock()

	if resolve != nil {
		resolve(path)
	}
	return true
}

// Expire settles a pending collaboration as expired (e.g. tool timeout),
// making any late Resolve a no-op. Returns true if it claimed the entry.
func (r *CollaborationRegistry) Expire(id string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.items[id]
	if !ok || e.state != CollabPending {
		return false
	}
	e.state = CollabExpired
	delete(r.items, id)
	return true
}

// Contains reports whether id is currently pending.
func (r *CollaborationRegistry) Contains(id string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.items[id]
	return ok && e.state == CollabPending
}
