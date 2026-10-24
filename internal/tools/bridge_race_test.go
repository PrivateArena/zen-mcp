package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/mcpcfg"
)

// The bridge addresses the active tab, not a tab handle, so these tests pin the
// interaction that broke in the field: two zfetch calls fired at the same time
// each navigated, then both read whichever tab loaded last, so a response carried
// one URL's provenance line over another URL's content. The gate in
// internal/bridge/gate.go must make that impossible without changing what a
// single call returns.

// fakeBrowser is a stateful Firefox bridge: navigate swaps the active tab, and
// read/get_text answer with whatever that tab holds. steps widens each action so
// overlapping callers genuinely interleave instead of accidentally missing each
// other.
type fakeBrowser struct {
	mu     sync.Mutex
	active string
	// overlap is the deepest simultaneous-in-flight count observed.
	overlap  int
	inFlight int
	steps    time.Duration
	// log records "action:target" per bridge call in arrival order.
	log []string
}

func (b *fakeBrowser) serve(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	action, _ := body["action"].(string)
	target, _ := body["url"].(string)

	b.mu.Lock()
	b.inFlight++
	if b.inFlight > b.overlap {
		b.overlap = b.inFlight
	}
	b.log = append(b.log, action+":"+fakePageID(target))
	b.mu.Unlock()

	if b.steps > 0 {
		time.Sleep(b.steps)
	}

	b.mu.Lock()
	if action == "navigate" && target != "" {
		b.active = target
	}
	page := fakePageID(b.active)
	b.inFlight--
	b.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch action {
	case "navigate":
		_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"url":%q,"title":"Page %s","status":200}`, target, page)))
	case "read", "get_text", "get_content":
		_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"content":%q}}`, fakePageMarkdown(page))))
	default:
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

// fakePageID reduces a target to its page marker: the last path segment for a
// plain page, the query text for a SERP.
func fakePageID(rawURL string) string {
	if rawURL == "" {
		return "blank"
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if q := u.Query().Get("q"); q != "" {
		return q
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	return segs[len(segs)-1]
}

func fakePageMarkdown(id string) string {
	return fmt.Sprintf("# PAGE %s\n\nmarker-%s-%s", id, id, strings.Repeat("x", 120))
}

func (b *fakeBrowser) maxOverlap() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.overlap
}

func (b *fakeBrowser) sequence() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.log...)
}

// fakeBrowserAt installs the stateful bridge for the duration of a test.
func fakeBrowserAt(t *testing.T, steps time.Duration) *fakeBrowser {
	t.Helper()
	fb := &fakeBrowser{steps: steps}
	ts := httptest.NewServer(http.HandlerFunc(fb.serve))
	t.Cleanup(ts.Close)
	pointBridgeAt(t, ts.URL)
	return fb
}

// fetchBody is the payload a fetch response carries, minus the provenance line.
func fetchBody(t *testing.T, res string) string {
	t.Helper()
	_, body, ok := strings.Cut(res, "\n\n")
	if !ok {
		t.Fatalf("response has no provenance/body split: %q", probeSnippet(res))
	}
	return body
}

// TestParallelFetchesReadTheirOwnPages is the regression itself: the two calls
// are issued at the same time, so without the queue each one reads the tab the
// other one opened.
func TestParallelFetchesReadTheirOwnPages(t *testing.T) {
	fb := fakeBrowserAt(t, 6*time.Millisecond)

	type outcome struct {
		url, body string
	}
	run := func(target string) outcome {
		res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
			"url": target, "idle_ms": 0,
		}))
		return outcome{url: target, body: resultText(t, res)}
	}

	for round := 0; round < 3; round++ {
		var wg sync.WaitGroup
		got := make([]outcome, 2)
		targets := []string{"http://127.0.0.1:8899/one", "http://127.0.0.1:8899/two"}
		for i, target := range targets {
			wg.Add(1)
			go func(i int, target string) {
				defer wg.Done()
				got[i] = run(target)
			}(i, target)
		}
		wg.Wait()

		for i, target := range targets {
			id := fakePageID(target)
			body := fetchBody(t, got[i].body)
			if !strings.Contains(body, "marker-"+id+"-") {
				t.Fatalf("round %d: fetch(%s) returned another page's content: %q", round, target, probeSnippet(body))
			}
			if other := targets[1-i]; strings.Contains(body, "marker-"+fakePageID(other)+"-") {
				t.Fatalf("round %d: fetch(%s) leaked %s: %q", round, target, other, probeSnippet(body))
			}
		}
	}

	if overlap := fb.maxOverlap(); overlap != 1 {
		t.Errorf("bridge saw %d simultaneous actions, want 1 (sequence %v)", overlap, fb.sequence())
	}
}

// TestParallelSearchesReadTheirOwnOverviews covers the second multi-step flow:
// navigate -> wait -> wait_for_selector -> get_text.
func TestParallelSearchesReadTheirOwnOverviews(t *testing.T) {
	fb := fakeBrowserAt(t, 6*time.Millisecond)

	queries := []string{"hyperframes rendering", "zen browser containers"}
	got := make([]string, len(queries))
	var wg sync.WaitGroup
	for i, q := range queries {
		wg.Add(1)
		go func(i int, q string) {
			defer wg.Done()
			res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(map[string]any{"query": q}))
			got[i] = resultText(t, res)
		}(i, q)
	}
	wg.Wait()

	for i, q := range queries {
		if !strings.Contains(got[i], "marker-"+q+"-") {
			t.Fatalf("search(%q) returned: %q", q, probeSnippet(got[i]))
		}
	}
	if overlap := fb.maxOverlap(); overlap != 1 {
		t.Errorf("bridge saw %d simultaneous actions, want 1", overlap)
	}
}

// TestFetchAndBrowserReadDoNotShareATab proves the reservation covers a mixed
// workload, not just two calls of the same tool: browser read navigates too, so
// it must not move the tab under a fetch that is mid-sequence.
func TestFetchAndBrowserReadDoNotShareATab(t *testing.T) {
	fb := fakeBrowserAt(t, 6*time.Millisecond)

	fetchOut, browserOut := "", ""
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		fetchOut = resultText(t, HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
			"url": "http://127.0.0.1:8899/fetched", "idle_ms": 0,
		})))
	}()
	go func() {
		defer wg.Done()
		browserOut = resultText(t, HandleBrowserAction(context.Background(), t.TempDir(), Deps{}, browserReq(map[string]any{
			"action": "read", "url": "http://127.0.0.1:8899/read",
		})))
	}()
	wg.Wait()

	if !strings.Contains(fetchOut, "marker-fetched-") {
		t.Errorf("fetch returned: %q", probeSnippet(fetchOut))
	}
	if !strings.Contains(browserOut, "marker-read-") {
		t.Errorf("browser read returned: %q", probeSnippet(browserOut))
	}
	if overlap := fb.maxOverlap(); overlap != 1 {
		t.Errorf("bridge saw %d simultaneous actions, want 1 (sequence %v)", overlap, fb.sequence())
	}
}

// TestFetchReportsABusyBridgeInsteadOfHanging: a wedged bridge must surface as an
// actionable error naming the holder, never as a silently wrong page.
func TestFetchReportsABusyBridgeInsteadOfHanging(t *testing.T) {
	fakeBrowserAt(t, 0)
	restore := withBridgeQueue(t, mcpcfg.BridgeConfig{MaxConcurrent: 1, QueueTimeoutMs: 60})
	defer restore()

	holder, err := bridge.DefaultGate().Acquire(context.Background(), "stuck-browser.chat")
	if err != nil {
		t.Fatalf("acquire holder: %v", err)
	}
	defer holder()

	start := time.Now()
	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "http://127.0.0.1:8899/one"}))
	text := resultText(t, res)
	if !strings.Contains(text, "bridge busy") {
		t.Fatalf("response = %q, want the queue-ceiling error", probeSnippet(text))
	}
	if !strings.Contains(text, "stuck-browser.chat") {
		t.Errorf("the error must name who holds the bridge, got %q", probeSnippet(text))
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("queued fetch took %v, want it to fail at the queue ceiling", elapsed)
	}
}

// withBridgeQueue swaps the `bridge` config block for one test.
func withBridgeQueue(t *testing.T, bc mcpcfg.BridgeConfig) func() {
	t.Helper()
	orig := mcpcfg.Get()
	cfg := *orig
	cfg.Bridge = bc
	mcpcfg.Config.Store(&cfg)
	return func() { mcpcfg.Config.Store(orig) }
}
