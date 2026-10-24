package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"zen-mcp/internal/mcpcfg"
)

// tabTraffic is a bridge stub that records how deeply calls overlap, overall and
// per addressed tab, which is exactly what the routing in CallBridge is supposed to
// control.
type tabTraffic struct {
	mu       sync.Mutex
	hold     time.Duration
	inFlight int
	overlap  int
	perPage  map[string]int
	pageMax  map[string]int
}

func newTabTraffic(hold time.Duration) *tabTraffic {
	return &tabTraffic{hold: hold, perPage: map[string]int{}, pageMax: map[string]int{}}
}

func (tr *tabTraffic) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		tab, _ := body["index"].(string)
		if tab == "" {
			tab = "active"
		}

		tr.mu.Lock()
		tr.inFlight++
		if tr.inFlight > tr.overlap {
			tr.overlap = tr.inFlight
		}
		tr.perPage[tab]++
		if tr.perPage[tab] > tr.pageMax[tab] {
			tr.pageMax[tab] = tr.perPage[tab]
		}
		tr.mu.Unlock()

		time.Sleep(tr.hold)

		tr.mu.Lock()
		tr.inFlight--
		tr.perPage[tab]--
		tr.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "id": tab})
	}
}

func (tr *tabTraffic) maxOverlap() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.overlap
}

func (tr *tabTraffic) maxOverlapFor(tab string) int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.pageMax[tab]
}

// withBridgeSettings swaps the `bridge` config block for one test.
func withBridgeSettings(t *testing.T, bc mcpcfg.BridgeConfig) {
	t.Helper()
	orig := mcpcfg.Get()
	cfg := *orig
	cfg.Bridge = bc
	mcpcfg.Config.Store(&cfg)
	t.Cleanup(func() { mcpcfg.Config.Store(orig) })
}

func parallelOn() *bool  { b := true; return &b }
func parallelOff() *bool { b := false; return &b }

func TestAddressedCallsOnDifferentTabsRunTogether(t *testing.T) {
	tr := newTabTraffic(30 * time.Millisecond)
	_, restore := testServer(t, tr.handler())
	defer restore()

	var wg sync.WaitGroup
	for _, id := range []string{"t_aaa_1", "t_bbb_2", "t_ccc_3"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := CallBridge(context.Background(), "read", map[string]any{"index": id}); err != nil {
				t.Errorf("read %s: %v", id, err)
			}
		}(id)
	}
	wg.Wait()

	if got := tr.maxOverlap(); got < 3 {
		t.Errorf("addressed calls overlapped %d, want all 3 at once — tab addressing must not queue on the whole browser", got)
	}
	for _, id := range []string{"t_aaa_1", "t_bbb_2", "t_ccc_3"} {
		if got := tr.maxOverlapFor(id); got != 1 {
			t.Errorf("tab %s ran %d at a time, want 1", id, got)
		}
	}
}

func TestUnaddressedCallsStillSerialize(t *testing.T) {
	tr := newTabTraffic(20 * time.Millisecond)
	_, restore := testServer(t, tr.handler())
	defer restore()

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := CallBridge(context.Background(), "read", nil); err != nil {
				t.Errorf("read: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := tr.maxOverlap(); got != 1 {
		t.Errorf("active-tab calls overlapped %d, want 1 (the Gate is the only thing keeping them off each other)", got)
	}
}

func TestAddressedCallsOnOneTabSerialize(t *testing.T) {
	tr := newTabTraffic(20 * time.Millisecond)
	_, restore := testServer(t, tr.handler())
	defer restore()

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := CallBridge(context.Background(), "read", map[string]any{"index": "t_same_1"}); err != nil {
				t.Errorf("read: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := tr.maxOverlapFor("t_same_1"); got != 1 {
		t.Errorf("one tab ran %d at a time, want 1", got)
	}
}

func TestWithTabTurnCoversTheWholeSequence(t *testing.T) {
	tr := newTabTraffic(20 * time.Millisecond)
	_, restore := testServer(t, tr.handler())
	defer restore()

	ctx, release, err := WithTabTurn(context.Background(), "t_seq_1", "fetch seq")
	if err != nil {
		t.Fatalf("WithTabTurn: %v", err)
	}
	defer release()
	if TabTurn(ctx) != "t_seq_1" {
		t.Fatalf("ctx must carry the reservation, got %q", TabTurn(ctx))
	}

	// The nested steps of the reserved flow must not queue behind their own turn.
	for _, action := range []string{"wait_for_network_idle", "read"} {
		if _, err := CallBridge(ctx, action, map[string]any{"index": "t_seq_1"}); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}

	// A second caller on the same tab waits until the flow releases it.
	blocked := make(chan struct{})
	go func() {
		if _, err := CallBridge(context.Background(), "read", map[string]any{"index": "t_seq_1"}); err != nil {
			t.Errorf("queued read: %v", err)
		}
		close(blocked)
	}()
	select {
	case <-blocked:
		t.Fatal("a second caller read the tab while the flow held the reservation")
	case <-time.After(60 * time.Millisecond):
	}
	release()
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("the queued caller was never admitted after the release")
	}
}

func TestBackgroundNavigationNeedsNoTurn(t *testing.T) {
	tr := newTabTraffic(10 * time.Millisecond)
	_, restore := testServer(t, tr.handler())
	defer restore()

	holder, err := DefaultGate().Acquire(context.Background(), "stuck-browser.chat")
	if err != nil {
		t.Fatalf("acquire holder: %v", err)
	}
	defer holder()

	done := make(chan error, 1)
	go func() {
		_, err := CallBridge(context.Background(), "navigate", map[string]any{
			"url": "https://example.com", "back_tab": true,
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("background navigate: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("a background navigation queued behind an unrelated holder")
	}

	// A foreground new_tab moves the selection, so it is not independent.
	fg := make(chan struct{})
	go func() {
		CallBridge(context.Background(), "navigate", map[string]any{"url": "https://example.com", "new_tab": true})
		close(fg)
	}()
	select {
	case <-fg:
		t.Fatal("a foreground new_tab skipped the global queue")
	case <-time.After(120 * time.Millisecond):
	}
}

func TestFocusBoundActionKeepsTheGlobalQueue(t *testing.T) {
	tr := newTabTraffic(10 * time.Millisecond)
	_, restore := testServer(t, tr.handler())
	defer restore()

	if !FocusBound("switch_tab") || !FocusBound("human_click") || !FocusBound("screenshot") {
		t.Fatal("selection/cursor actions must be classified focus-bound")
	}
	if FocusBound("read") || FocusBound("wait_for_selector") {
		t.Fatal("frame reads are not focus-bound")
	}

	holder, err := DefaultGate().Acquire(context.Background(), "holder")
	if err != nil {
		t.Fatalf("acquire holder: %v", err)
	}
	defer holder()

	done := make(chan struct{})
	go func() {
		CallBridge(context.Background(), "screenshot", map[string]any{"index": "t_aaa_1"})
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("an addressed screenshot skipped the global queue")
	case <-time.After(120 * time.Millisecond):
	}
}

func TestTabParallelOffRestoresGlobalSerial(t *testing.T) {
	tr := newTabTraffic(20 * time.Millisecond)
	_, restore := testServer(t, tr.handler())
	defer restore()
	withBridgeSettings(t, mcpcfg.BridgeConfig{MaxConcurrent: 1, QueueTimeoutMs: 5000, TabParallel: parallelOff()})

	var wg sync.WaitGroup
	for _, id := range []string{"t_aaa_1", "t_bbb_2"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := CallBridge(context.Background(), "read", map[string]any{"index": id}); err != nil {
				t.Errorf("read %s: %v", id, err)
			}
		}(id)
	}
	wg.Wait()

	if got := tr.maxOverlap(); got != 1 {
		t.Errorf("with bridge.tabParallel off, calls overlapped %d, want strict 1", got)
	}
}

func TestTabQueuesAreReleasedWhenIdle(t *testing.T) {
	tr := newTabTraffic(0)
	_, restore := testServer(t, tr.handler())
	defer restore()

	if _, err := CallBridge(context.Background(), "read", map[string]any{"index": "t_leak_1"}); err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, ok := TabQueueStatus("t_leak_1"); ok {
		t.Error("the per-tab queue of a finished flow was never dropped")
	}
}

func TestRegistryIDShape(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{in: "t_mvm0rsyp_5v3", want: "t_mvm0rsyp_5v3"}, // a registry id
		{in: "  t_a_1 ", want: "t_a_1"},                // padded
		{in: "3", want: ""},                            // positional index
		{in: float64(3), want: ""},                     // positional index as a number
		{in: "t_", want: ""},                           // prefix only, not an id
		{in: "", want: ""},
		{in: nil, want: ""},
	}
	for _, tc := range cases {
		if got := RegistryID(tc.in); got != tc.want {
			t.Errorf("RegistryID(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTabParallelDefaultIsOn(t *testing.T) {
	withBridgeSettings(t, mcpcfg.BridgeConfig{MaxConcurrent: 1, QueueTimeoutMs: 1000, TabParallel: nil})
	if !tabParallelEnabled() {
		t.Error("tab addressing must be on by default")
	}
	withBridgeSettings(t, mcpcfg.BridgeConfig{MaxConcurrent: 1, QueueTimeoutMs: 1000, TabParallel: parallelOn()})
	if !tabParallelEnabled() {
		t.Error("bridge.tabParallel: true must enable tab addressing")
	}
}
