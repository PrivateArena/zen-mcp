package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"zen-mcp/internal/mcpcfg"
)

// The gate is the contract that keeps parallel zfetch/zsearch calls from reading
// each other's tabs, so every assertion below is about observable ordering — who
// is inside the bridge, and in which order — rather than internals.

// tracker counts how many callers are simultaneously inside the bridge, which is
// the externally visible symptom of the original race.
type tracker struct {
	mu       sync.Mutex
	in       int
	peakSeen int
	finished []string
}

func (tr *tracker) enter() {
	tr.mu.Lock()
	tr.in++
	if tr.in > tr.peakSeen {
		tr.peakSeen = tr.in
	}
	tr.mu.Unlock()
}

func (tr *tracker) exit(label string) {
	tr.mu.Lock()
	tr.in--
	tr.finished = append(tr.finished, label)
	tr.mu.Unlock()
}

func (tr *tracker) peak() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.peakSeen
}

func (tr *tracker) done() []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return append([]string(nil), tr.finished...)
}

// acquire turns the gate into a test helper that fails loudly.
func acquire(t *testing.T, g *Gate, label string) func() {
	t.Helper()
	release, err := g.Acquire(context.Background(), label)
	if err != nil {
		t.Fatalf("Acquire(%s): %v", label, err)
	}
	return release
}

// waitForIdle waits for the gate to drain, so a leaked turn fails the test that
// leaked it instead of every later test in the package.
func waitForIdle(t *testing.T, g *Gate) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := g.Status()
		if st.Active == 0 && st.Waiting == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("gate left dirty: %+v", g.Status())
}

// waitQueueing blocks until the gate reports `want` queued callers, which makes
// queue order a deterministic assertion instead of a sleep race.
func waitQueueing(t *testing.T, g *Gate, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if g.Status().Waiting == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("queue depth = %d, want %d (%+v)", g.Status().Waiting, want, g.Status())
}

func TestGateAdmitsOneCallerAtATime(t *testing.T) {
	g := NewGate(1, 0)
	var tr tracker
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			release := acquire(t, g, fmt.Sprintf("call-%d", i))
			tr.enter()
			time.Sleep(2 * time.Millisecond) // stand in for the bridge round-trip
			tr.exit("in")
			release()
		}(i)
	}
	wg.Wait()

	if peak := tr.peak(); peak != 1 {
		t.Fatalf("peak concurrent bridge callers = %d, want 1", peak)
	}
	if n := len(tr.done()); n != 8 {
		t.Fatalf("%d callers completed, want 8", n)
	}
	if st := g.Status(); st.Active != 0 || st.Waiting != 0 {
		t.Fatalf("gate left dirty: %+v", st)
	}
	waitForIdle(t, g)
}

func TestGateAdmitsInArrivalOrder(t *testing.T) {
	g := NewGate(1, 0)
	blocker := acquire(t, g, "blocker")

	var tr tracker
	var wg sync.WaitGroup
	for i, name := range []string{"first", "second", "third"} {
		wg.Add(1)
		go func(label string) {
			defer wg.Done()
			release := acquire(t, g, label)
			defer release()
			tr.enter()
			tr.exit(label)
		}(name)
		waitQueueing(t, g, i+1)
	}
	blocker()
	wg.Wait()

	got := strings.Join(tr.done(), ",")
	if got != "first,second,third" {
		t.Fatalf("admission order = %q, want first,second,third — a lost race is exactly what this gate fixes", got)
	}
}

func TestGateLimitAllowsBoundedParallelism(t *testing.T) {
	g := NewGate(3, 0)
	var tr tracker
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			release := acquire(t, g, fmt.Sprintf("call-%d", i))
			tr.enter()
			time.Sleep(2 * time.Millisecond)
			tr.exit("in")
			release()
		}(i)
	}
	wg.Wait()

	if peak := tr.peak(); peak > 3 || peak < 2 {
		t.Fatalf("peak concurrency = %d with a limit of 3, want between 2 and 3", peak)
	}
	if n := len(tr.done()); n != 12 {
		t.Fatalf("%d callers completed, want 12", n)
	}
}

func TestGateCanceledWaiterLeavesTheQueue(t *testing.T) {
	g := NewGate(1, 0)
	blocker := acquire(t, g, "blocker")

	ctx, cancel := context.WithCancel(context.Background())
	gaveUp := make(chan error, 1)
	go func() {
		_, err := g.Acquire(ctx, "canceled")
		gaveUp <- err
	}()
	waitQueueing(t, g, 1)

	next := make(chan struct{})
	go func() {
		release := acquire(t, g, "next")
		close(next)
		release()
	}()

	cancel()
	select {
	case err := <-gaveUp:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter got %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled waiter never returned")
	}

	// The turn goes to the next caller in line as soon as the blocker releases it:
	// the canceled waiter must have left the queue, not stalled it.
	blocker()
	select {
	case <-next:
	case <-time.After(2 * time.Second):
		t.Fatalf("queue wedged behind a canceled waiter: %+v", g.Status())
	}
	if st := g.Status(); st.Waiting != 0 {
		t.Fatalf("canceled waiter still queued: %+v", st)
	}
}

func TestGateQueueCeilingReportsTheHolder(t *testing.T) {
	g := NewGate(1, 40*time.Millisecond)
	blocker := acquire(t, g, "long-navigation")
	defer blocker()

	_, err := g.Acquire(context.Background(), "queued-fetch")
	var busy *ErrBridgeBusy
	if !errors.As(err, &busy) {
		t.Fatalf("Acquire over the queue ceiling returned %v, want *ErrBridgeBusy", err)
	}
	if !strings.Contains(err.Error(), "long-navigation") {
		t.Errorf("busy error must name the holder, got: %v", err)
	}
	if busy.Status.Limit != 1 {
		t.Errorf("busy error limit = %d, want 1", busy.Status.Limit)
	}
}

func TestGateReleaseIsIdempotent(t *testing.T) {
	g := NewGate(1, 0)
	release := acquire(t, g, "holder")
	release()
	release()

	got := make(chan string, 1)
	go func() {
		second := acquire(t, g, "after-double-release")
		defer second()
		got <- "admitted"
	}()
	select {
	case v := <-got:
		if v != "admitted" {
			t.Fatalf("second acquire after double release: %s", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("double release wedged the queue")
	}
}

// TestGateFollowsConfigConcurrency proves the `bridge.maxConcurrent` knob is
// live: the queue re-reads it per Acquire, so tuning needs no restart.
func TestGateFollowsConfigConcurrency(t *testing.T) {
	defer withBridgeConfig(t, mcpcfg.BridgeConfig{MaxConcurrent: 2, QueueTimeoutMs: 5000})()

	g := DefaultGate()
	limit, ceiling := g.Settings()
	if limit != 2 {
		t.Fatalf("gate limit = %d, want the configured 2", limit)
	}
	if ceiling != 5*time.Second {
		t.Fatalf("queue ceiling = %v, want the configured 5s", ceiling)
	}

	first := acquire(t, g, "first")
	second := acquire(t, g, "second")
	if st := g.Status(); st.Active != 2 {
		t.Fatalf("active = %d, want 2 under the configured limit", st.Active)
	}
	thirdErr := make(chan error, 1)
	go func() {
		release, err := g.Acquire(context.Background(), "third")
		if err != nil {
			thirdErr <- err
			return
		}
		thirdErr <- nil
		release()
	}()
	waitQueueing(t, g, 1)
	first()
	if err := <-thirdErr; err != nil {
		t.Fatalf("third caller: %v", err)
	}
	second()
	waitForIdle(t, g)
}

func TestCallBridgeSerializesConcurrentCalls(t *testing.T) {
	defer withBridgeConfig(t, mcpcfg.BridgeConfig{MaxConcurrent: 1, QueueTimeoutMs: 5000})()

	var tr tracker
	_, done := testServer(t, countingHandler(&tr))
	defer done()

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := CallBridge(context.Background(), "navigate", map[string]any{"url": fmt.Sprintf("page-%d", i)}); err != nil {
				t.Errorf("CallBridge: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if peak := tr.peak(); peak != 1 {
		t.Fatalf("peak concurrent bridge requests = %d, want 1", peak)
	}
	waitForIdle(t, DefaultGate())
}

// TestCallBridgeTurnIsReentrantAcrossASequence is the fetch/search contract: the
// reservation covers navigate -> wait -> read, so no other caller can slip
// between the steps and move the active tab.
func TestCallBridgeTurnIsReentrantAcrossASequence(t *testing.T) {
	defer withBridgeConfig(t, mcpcfg.BridgeConfig{MaxConcurrent: 1, QueueTimeoutMs: 5000})()

	var tr tracker
	_, done := testServer(t, countingHandler(&tr))
	defer done()

	ctx, release, err := WithTurn(context.Background(), "sequence")
	if err != nil {
		t.Fatalf("WithTurn: %v", err)
	}

	if !Held(ctx) {
		t.Fatal("Held(ctx) false inside a reserved turn")
	}

	// An outside caller must not enter while the sequence is mid-flight.
	interrupted := make(chan struct{})
	go func() {
		if _, err := CallBridge(context.Background(), "navigate", map[string]any{"url": "other"}); err != nil {
			t.Errorf("interrupting CallBridge: %v", err)
			return
		}
		close(interrupted)
	}()

	for _, action := range []string{"navigate", "wait_for_network_idle", "read"} {
		if _, err := CallBridge(ctx, action, nil); err != nil {
			t.Fatalf("CallBridge(%s) inside the turn: %v", action, err)
		}
	}
	select {
	case <-interrupted:
		t.Fatal("another caller entered the bridge mid-sequence")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-interrupted:
	case <-time.After(2 * time.Second):
		t.Fatal("queued caller never admitted after the turn was released")
	}
	waitForIdle(t, DefaultGate())
}

func TestWithTurnNestsWithoutDeadlock(t *testing.T) {
	outer, releaseOuter, err := WithTurn(context.Background(), "outer")
	if err != nil {
		t.Fatalf("outer WithTurn: %v", err)
	}
	defer releaseOuter()

	_, releaseInner, err := WithTurn(outer, "inner")
	if err != nil {
		t.Fatalf("nested WithTurn: %v", err)
	}
	if releaseInner == nil {
		t.Fatal("nested WithTurn returned a nil release")
	}
	defer releaseInner()

	// Only the outer reservation may release the turn.
	if st := DefaultGate().Status(); st.Active != 1 {
		t.Fatalf("nested reservations took %d turns, want 1", st.Active)
	}
	releaseInner()
	if st := DefaultGate().Status(); st.Active != 1 {
		t.Fatalf("inner release freed the shared turn: %+v", st)
	}
}

// TestQueueAgentActionsDefaultOff pins the deliberate exemption: chat drives an
// AI page for minutes, so queueing every fetch behind it would stall the whole
// tool family. `bridge.queueAgentActions` opts back into strict ordering.
func TestQueueAgentActionsDefaultOff(t *testing.T) {
	_, done := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	defer done()

	g := DefaultGate()
	holder := acquire(t, g, "test-holder")

	proceeds := make(chan error, 1)
	go func() {
		_, err := CallBridge(context.Background(), "chat", map[string]any{"message": "hi"})
		proceeds <- err
	}()
	select {
	case err := <-proceeds:
		if err != nil && !strings.Contains(err.Error(), "connection refused") {
			t.Fatalf("chat failed for an unexpected reason: %v", err)
		}
	case <-time.After(2 * time.Second):
		holder()
		t.Fatal("chat was queued behind the turn holder")
	}
	holder()
	waitForIdle(t, DefaultGate())
}

func TestQueueAgentActionsCanBeMadeStrict(t *testing.T) {
	_, done := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	defer done()

	strict := mcpcfg.BridgeConfig{MaxConcurrent: 1, QueueTimeoutMs: 40, QueueAgentActions: boolPtr(true)}
	defer withBridgeConfig(t, strict)()

	holder := acquire(t, DefaultGate(), "test-holder")

	_, err := CallBridge(context.Background(), "chat", map[string]any{"message": "hi"})
	if err == nil {
		holder()
		t.Fatal("chat bypassed the queue with queueAgentActions enabled")
	}
	holder()
	if !strings.Contains(err.Error(), "bridge busy") {
		t.Fatalf("chat with queueAgentActions got %v, want a queue-ceiling error", err)
	}
	waitForIdle(t, DefaultGate())
}

func boolPtr(v bool) *bool { return &v }

// countingHandler answers like the bridge while recording overlap between calls.
func countingHandler(tr *tracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		action, _ := body["action"].(string)
		tr.enter()
		time.Sleep(5 * time.Millisecond)
		tr.exit(action)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

// withBridgeConfig swaps the `bridge` config block the way editing the config
// file would, and hands back the restore.
func withBridgeConfig(t *testing.T, bc mcpcfg.BridgeConfig) func() {
	t.Helper()
	orig := mcpcfg.Get()
	cfg := *orig
	cfg.Bridge = bc
	mcpcfg.Config.Store(&cfg)
	return func() { mcpcfg.Config.Store(orig) }
}
