package bridge

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
)

// The Firefox bridge drives one browser window, and every one of its actions
// addresses the *active* tab by name rather than by tab handle. That makes it a
// single-resource device: two tool calls issued at the same time navigate over
// each other, and the caller that navigated first ends up reading the tab the
// other one opened (a zfetch of page A answered with page B's content under A's
// provenance line).
//
// Gate is the client-side reservation queue that fixes this. Calls are admitted
// one at a time, strictly in arrival order, and a multi-step flow
// (navigate -> wait -> read) reserves the turn once for the whole sequence with
// WithTurn instead of once per call.
//
// Why FIFO instead of a mutex/channel semaphore: the ordering is also the
// fairness contract the caller needs. With an unordered channel, a burst of
// parallel zfetch calls picks a random winner, and a call that keeps losing the
// race keeps reading the wrong page.

const (
	// defaultBridgeConcurrency is the number of bridge operations allowed in
	// flight when the `bridge` config block does not say otherwise. One, because
	// the bridge itself cannot tell two concurrent callers apart.
	defaultBridgeConcurrency = 1
	// defaultBridgeQueueMs bounds how long a caller may sit in the queue before
	// it gets ErrBridgeBusy. Without a ceiling a wedged bridge POST (CallBridge
	// deliberately runs without a client deadline) would stall every other bridge
	// tool forever; with it the queue waiter reports who holds the turn instead.
	defaultBridgeQueueMs = 120_000
	// bridgeSlowQueue logs a queue wait long enough to be worth explaining in the
	// server log, as opposed to the usual sub-millisecond pass-through.
	bridgeSlowQueue = 2 * time.Second
	// bridgeLabelFallback keeps holder diagnostics non-empty.
	bridgeLabelFallback = "bridge"
)

// Gate is a FIFO counting semaphore over bridge access. The zero value is not
// usable; build one with NewGate or take DefaultGate.
type Gate struct {
	mu      sync.Mutex
	limit   int
	maxWait time.Duration
	// policy, when set, re-reads the admission settings on every Acquire so a
	// config edit takes effect without a restart. Only the process-wide gate has it.
	policy func() (int, time.Duration)

	// eligible is the ticket of the next caller that may be admitted; tickets are
	// handed out from 0 in arrival order, so admission always walks them upward.
	eligible uint64
	// next is the next ticket to hand out.
	next    uint64
	holders []*holder
	pending map[uint64]*waiter
}

type holder struct {
	ticket uint64
	label  string
	since  time.Time
}

type waiter struct {
	ticket   uint64
	label    string
	ready    chan struct{} // closed exactly once, when the turn is handed over
	admitted bool
}

// Status is a snapshot of the queue, used in busy errors and diagnostics. The
// durations are plain milliseconds because this is handed to the model, where a
// nanosecond integer is unreadable.
type Status struct {
	Limit     int      `json:"limit"`
	Active    int      `json:"active"`
	Waiting   int      `json:"waiting"`
	Holder    string   `json:"holder,omitempty"`
	HeldForMs int64    `json:"heldForMs,omitempty"`
	Queue     []string `json:"queue,omitempty"`
}

// ErrBridgeBusy reports that the caller gave up waiting for the bridge turn
// after the configured queue ceiling. The message names the current holder so
// the caller learns what is occupying the browser instead of timing out blind.
type ErrBridgeBusy struct {
	Label   string        `json:"label"`
	Waited  time.Duration `json:"waited"`
	Ceiling time.Duration `json:"ceiling"`
	Status  Status        `json:"status"`
}

func (e *ErrBridgeBusy) Error() string {
	s := e.Status
	if s.Active == 0 {
		return fmt.Sprintf("bridge queue stalled: %s waited %s with the queue ceiling of %s and no active holder",
			e.Label, e.Waited.Round(time.Millisecond), e.Ceiling)
	}
	return fmt.Sprintf("bridge busy: %s waited %s (ceiling %s) behind %d queued call(s); turn held by %s for %dms (limit %d)",
		e.Label, e.Waited.Round(time.Millisecond), e.Ceiling, s.Waiting, s.Holder, s.HeldForMs, s.Limit)
}

// NewGate returns a gate with a fixed admission limit and queue ceiling. A
// limit below 1 means 1; a maxWait of 0 means "wait until the caller's context
// is done".
func NewGate(limit int, maxWait time.Duration) *Gate {
	if limit < 1 {
		limit = defaultBridgeConcurrency
	}
	return &Gate{limit: limit, maxWait: maxWait, pending: map[uint64]*waiter{}}
}

var defaultGate = &Gate{
	limit:   defaultBridgeConcurrency,
	maxWait: defaultBridgeQueueMs * time.Millisecond,
	policy:  bridgePolicy,
	pending: map[uint64]*waiter{},
}

// DefaultGate is the process-wide gate every CallBridge goes through unless the
// caller already holds the turn. Its limits follow the `bridge` config block
// live, so tuning it needs no restart.
func DefaultGate() *Gate { return defaultGate }

// bridgePolicy maps the `bridge` config block onto gate settings, falling back
// to the defaults for absent or nonsensical values.
func bridgePolicy() (int, time.Duration) {
	limit, ceiling := defaultBridgeConcurrency, defaultBridgeQueueMs
	if c := mcpcfg.Get(); c != nil {
		if c.Bridge.MaxConcurrent > 0 {
			limit = c.Bridge.MaxConcurrent
		}
		if c.Bridge.QueueTimeoutMs > 0 {
			ceiling = c.Bridge.QueueTimeoutMs
		}
	}
	return limit, time.Duration(ceiling) * time.Millisecond
}

// Settings returns the admission limit and queue ceiling currently in force.
func (g *Gate) Settings() (int, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.settingsLocked()
}

func (g *Gate) settingsLocked() (int, time.Duration) {
	if g.policy == nil {
		return g.limit, g.maxWait
	}
	limit, maxWait := g.policy()
	if limit < 1 {
		limit = defaultBridgeConcurrency
	}
	g.limit = limit
	g.maxWait = maxWait
	return limit, maxWait
}

// Acquire takes the next free turn, waiting in arrival order for one. The
// returned release must be called when the caller is done with the bridge; it is
// safe to call more than once. Acquire returns ctx.Err() when the caller's
// context ends first, or *ErrBridgeBusy when the queue ceiling runs out.
func (g *Gate) Acquire(ctx context.Context, label string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = bridgeLabelFallback
	}
	_, ceiling := g.Settings()

	g.mu.Lock()
	ticket := g.next
	g.next++
	if len(g.pending) == 0 && ticket == g.eligible && len(g.holders) < g.limit {
		g.admitLocked(ticket, label)
		g.mu.Unlock()
		return g.releaseFunc(ticket), nil
	}
	w := &waiter{ticket: ticket, label: label, ready: make(chan struct{})}
	g.pending[ticket] = w
	g.mu.Unlock()

	waited := time.Now()
	if ceiling > 0 {
		timer := time.NewTimer(ceiling)
		defer timer.Stop()
		select {
		case <-w.ready:
			g.logQueue(label, waited)
			return g.releaseFunc(ticket), nil
		case <-ctx.Done():
			if !g.abandon(w) {
				<-w.ready
				return g.releaseFunc(ticket), nil
			}
			return nil, ctx.Err()
		case <-timer.C:
			if !g.abandon(w) {
				<-w.ready
				return g.releaseFunc(ticket), nil
			}
			return nil, &ErrBridgeBusy{Label: label, Waited: time.Since(waited), Ceiling: ceiling, Status: g.Status()}
		}
	}

	select {
	case <-w.ready:
		g.logQueue(label, waited)
		return g.releaseFunc(ticket), nil
	case <-ctx.Done():
		if !g.abandon(w) {
			<-w.ready
			return g.releaseFunc(ticket), nil
		}
		return nil, ctx.Err()
	}
}

// Status snapshots the queue.
func (g *Gate) Status() Status {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := Status{Limit: g.limit, Active: len(g.holders), Waiting: len(g.pending)}
	var oldest *holder
	for _, h := range g.holders {
		if oldest == nil || h.since.Before(oldest.since) {
			oldest = h
		}
	}
	if oldest != nil {
		st.Holder = oldest.label
		st.HeldForMs = int64(time.Since(oldest.since) / time.Millisecond)
	}
	if st.Waiting > 0 {
		for _, w := range g.queuedLocked() {
			st.Queue = append(st.Queue, w.label)
		}
	}
	return st
}

// queuedLocked returns the pending waiters in ticket order.
func (g *Gate) queuedLocked() []*waiter {
	out := make([]*waiter, 0, len(g.pending))
	for _, w := range g.pending {
		out = append(out, w)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].ticket < out[j-1].ticket; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// admitLocked hands the turn to one caller. Callers must keep ticket order.
func (g *Gate) admitLocked(ticket uint64, label string) {
	g.eligible = ticket + 1
	g.holders = append(g.holders, &holder{ticket: ticket, label: label, since: time.Now()})
}

// admitPendingLocked fills free turns in ticket order.
func (g *Gate) admitPendingLocked() {
	for len(g.holders) < g.limit {
		w, ok := g.pending[g.eligible]
		if !ok {
			return
		}
		delete(g.pending, w.ticket)
		g.admitLocked(w.ticket, w.label)
		w.admitted = true
		close(w.ready)
	}
}

// abandon removes a waiter that gave up. It reports false when the turn had
// already been handed over, in which case the caller owns the turn.
func (g *Gate) abandon(w *waiter) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if w.admitted {
		return false
	}
	if _, ok := g.pending[w.ticket]; !ok {
		return false
	}
	delete(g.pending, w.ticket)
	if w.ticket == g.eligible {
		g.eligible++
	}
	g.admitPendingLocked()
	return true
}

// releaseFunc builds an idempotent release for one turn.
func (g *Gate) releaseFunc(ticket uint64) func() {
	var once sync.Once
	return func() {
		once.Do(func() { g.release(ticket) })
	}
}

func (g *Gate) release(ticket uint64) {
	g.mu.Lock()
	for i, h := range g.holders {
		if h.ticket == ticket {
			g.holders = append(g.holders[:i], g.holders[i+1:]...)
			break
		}
	}
	g.admitPendingLocked()
	g.mu.Unlock()
}

func (g *Gate) logQueue(label string, since time.Time) {
	if waited := time.Since(since); waited >= bridgeSlowQueue {
		logfilter.Debugf("[bridge] %s waited %s for the bridge turn", label, waited.Round(time.Millisecond))
	}
}

// QueueStatus reports the state of the process-wide bridge queue.
func QueueStatus() Status { return DefaultGate().Status() }

// --- turn scoping -------------------------------------------------------

type turnKey struct{}

// turn is the marker that ctx carries while a reservation is live.
type turn struct{}

// WithTurn reserves the bridge for a whole sequence of calls and returns the
// context to run them on: every CallBridge made with that context reuses the
// reservation instead of queueing again, so navigate -> wait -> read can never be
// interleaved with another caller. The returned release must be deferred; it is
// safe to call twice, which lets a helper hand the reservation back to its
// caller.
//
// The reservation belongs to the calling goroutine: do not carry the returned
// context into another goroutine and call the bridge from there, because the
// re-entry check would treat it as already covered.
//
// A context that already carries a turn is returned unchanged with a no-op
// release, so nesting tool calls never deadlocks on the queue.
func WithTurn(ctx context.Context, label string) (context.Context, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if held := turnFrom(ctx); held != nil {
		return ctx, func() {}, nil
	}
	release, err := DefaultGate().Acquire(ctx, label)
	if err != nil {
		return ctx, nil, err
	}
	t := &turn{}
	return context.WithValue(ctx, turnKey{}, t), release, nil
}

// turnFrom returns the turn carried by ctx, or nil.
func turnFrom(ctx context.Context) *turn {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(turnKey{}).(*turn)
	return t
}

// Held reports whether ctx already carries a bridge reservation.
func Held(ctx context.Context) bool { return turnFrom(ctx) != nil }

// agentActionExempt reports whether an action skips the queue. The chat-like
// actions drive an AI page for minutes (browser.chat runs up to the 900s tool
// budget) and are already serialized inside the bridge, so putting them at the
// head of the queue would starve every fetch of a browser that is busy talking.
// `bridge.queueAgentActions: true` opts into fully strict serialization.
func agentActionExempt(action string) bool {
	if !isChatLike(action) {
		return false
	}
	c := mcpcfg.Get()
	return c == nil || c.Bridge.QueueAgentActions == nil || !*c.Bridge.QueueAgentActions
}

// isChatLike mirrors the browser tool's classification of the agent actions.
func isChatLike(action string) bool {
	switch action {
	case "chat", "brainstorm", "brainstorm_status":
		return true
	}
	return false
}
