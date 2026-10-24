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

// The bridge addresses tabs through its own registry: navigate reports the id of the
// tab it opened, an addressed call answers for that tab alone, and an id that no
// longer resolves is a hard tab_not_found. These tests pin what broke in the field —
// two zfetch calls issued at the same time, each reading whichever tab loaded last,
// so a response carried one URL's provenance line over another URL's content — and
// what the fix must not do: serialize calls that are provably independent.

// fakeBrowser is a stateful Firefox bridge with a tab registry. navigate opens a tab
// and reports its id, addressed calls answer for that tab only, unaddressed calls
// answer for the selected tab. steps widens each action so overlapping callers
// genuinely interleave instead of accidentally missing each other.
type fakeBrowser struct {
	mu     sync.Mutex
	pages  map[string]string // registry id -> page marker
	active string            // page marker of the selected tab
	nextID int
	// overlap counts simultaneous calls overall, pageMax per tab. The second number
	// is the property the queues must guarantee; the first is what tab addressing
	// buys back.
	overlap  int
	inFlight int
	pageMax  map[string]int
	perPage  map[string]int
	steps    time.Duration
	log      []string
}

func (b *fakeBrowser) serve(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	action, _ := body["action"].(string)
	target, _ := body["url"].(string)
	tabID, _ := body["index"].(string)

	page := b.enter(action, target, tabID)
	if b.steps > 0 {
		time.Sleep(b.steps)
	}
	status, payload := b.answer(action, target, tabID, page, body)

	if status != http.StatusOK {
		http.Error(w, fmt.Sprint(payload), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	raw, _ := json.Marshal(payload)
	_, _ = w.Write(raw)
}

// enter accounts for a call in flight against the tab it targets.
func (b *fakeBrowser) enter(action, target, tabID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	page := b.active
	switch {
	case action == "navigate" && tabID != "":
		page = b.pages[tabID]
	case action == "navigate":
		page = fakePageID(target)
	case tabID != "":
		page = b.pages[tabID]
	}

	b.inFlight++
	if b.inFlight > b.overlap {
		b.overlap = b.inFlight
	}
	b.perPage[page]++
	if b.perPage[page] > b.pageMax[page] {
		b.pageMax[page] = b.perPage[page]
	}
	b.log = append(b.log, action+":"+page)
	return page
}

// leave accounts for a call that finished.
func (b *fakeBrowser) leave(page string) {
	b.inFlight--
	b.perPage[page]--
}

// answer produces the response and applies the state change it stands for.
func (b *fakeBrowser) answer(action, target, tabID, page string, body map[string]any) (int, any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.leave(page)

	switch action {
	case "navigate":
		id := tabID
		if id == "" {
			b.nextID++
			id = fmt.Sprintf("t_fake_%d", b.nextID)
		}
		b.pages[id] = page
		// Only a foreground navigation moves the selection; a background tab leaves
		// every other caller's view of the browser untouched.
		if background, _ := body["back_tab"].(bool); !background {
			b.active = page
		}
		return http.StatusOK, map[string]any{
			"success": true, "id": id, "url": target, "title": "Page " + page, "status": 200, "containerId": 0,
		}

	case "close_tab":
		if _, ok := b.pages[tabID]; !ok {
			return http.StatusNotFound, map[string]any{"success": false, "error": "tab_not_found", "index": tabID}
		}
		delete(b.pages, tabID)
		return http.StatusOK, map[string]any{"success": true, "id": tabID}

	case "read", "get_text", "get_content":
		resolved, echoID := b.active, ""
		if tabID != "" {
			p, known := b.pages[tabID]
			if !known {
				return http.StatusNotFound, map[string]any{"success": false, "error": "tab_not_found", "index": tabID}
			}
			resolved, echoID = p, tabID
		}
		res := map[string]any{
			"success": true,
			"data":    map[string]any{"content": fakePageMarkdown(resolved)},
		}
		if echoID != "" {
			// The provenance echo of an addressed frame action (runFrameAction).
			res["id"] = echoID
			res["url"] = "http://127.0.0.1:8899/" + resolved
		}
		return http.StatusOK, res

	default:
		return http.StatusOK, map[string]any{"success": true}
	}
}

// maxOverlap is the deepest simultaneous-in-flight count across all tabs.
func (b *fakeBrowser) maxOverlap() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.overlap
}

// maxPageOverlap is the deepest simultaneous count on any single tab.
func (b *fakeBrowser) maxPageOverlap() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	max := 0
	for _, v := range b.pageMax {
		if v > max {
			max = v
		}
	}
	return max
}

func (b *fakeBrowser) sequence() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.log...)
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

// fakeBrowserAt installs the stateful bridge for the duration of a test.
func fakeBrowserAt(t *testing.T, steps time.Duration) *fakeBrowser {
	t.Helper()
	fb := &fakeBrowser{
		steps:   steps,
		pages:   map[string]string{},
		perPage: map[string]int{},
		pageMax: map[string]int{},
	}
	ts := httptest.NewServer(http.HandlerFunc(fb.serve))
	t.Cleanup(ts.Close)
	pointBridgeAt(t, ts.URL)
	return fb
}

// fetchBody is the payload a fetch response carries, minus the provenance line.
func fetchBody(res string) (string, bool) {
	_, body, ok := strings.Cut(res, "\n\n")
	return body, ok
}

// TestParallelFetchesReadTheirOwnPages is the regression itself, plus its counter
// claim: each call must read its own tab, and the calls must genuinely overlap —
// tab-addressed flows are independent, so queueing them one by one would throw the
// parallelism away for nothing.
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
			body, ok := fetchBody(got[i].body)
			if !ok {
				t.Fatalf("round %d: fetch(%s) returned no provenance/body split: %q", round, target, probeSnippet(got[i].body))
			}
			if !strings.Contains(body, "marker-"+id+"-") {
				t.Fatalf("round %d: fetch(%s) returned another page's content: %q", round, target, probeSnippet(body))
			}
			if other := targets[1-i]; strings.Contains(body, "marker-"+fakePageID(other)+"-") {
				t.Fatalf("round %d: fetch(%s) leaked %s: %q", round, target, other, probeSnippet(body))
			}
		}
	}

	if overlap := fb.maxOverlap(); overlap < 2 {
		t.Errorf("tab-addressed fetches never overlapped (max %d), so the flow still queues on the whole browser; sequence %v", overlap, fb.sequence())
	}
	if max := fb.maxPageOverlap(); max > 1 {
		t.Errorf("one tab saw %d simultaneous calls, want 1 (sequence %v)", max, fb.sequence())
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
	if overlap := fb.maxOverlap(); overlap < 2 {
		t.Errorf("tab-addressed searches never overlapped (max %d)", overlap)
	}
	if max := fb.maxPageOverlap(); max > 1 {
		t.Errorf("one tab saw %d simultaneous calls, want 1", max)
	}
}

// TestFetchAndBrowserReadDoNotShareATab proves the tab-scoping holds across tools,
// not just between two calls of the same one: browser read navigates too, so it must
// not move the page under a fetch that is mid-sequence.
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
	if max := fb.maxPageOverlap(); max > 1 {
		t.Errorf("one tab saw %d simultaneous calls, want 1 (sequence %v)", max, fb.sequence())
	}
}

// TestCallersSharingOneTabSerialize: two flows on the *same* registry tab are not
// independent, so the per-tab queue must keep them off each other's DOM even while
// different tabs run in parallel.
func TestCallersSharingOneTabSerialize(t *testing.T) {
	fb := fakeBrowserAt(t, 8*time.Millisecond)

	nav, err := bridge.CallBridge(context.Background(), "navigate", map[string]any{
		"url": "http://127.0.0.1:8899/shared", "back_tab": true,
	})
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	tabID := bridge.TabID(nav)
	if tabID == "" {
		t.Fatalf("navigate reported no tab id: %v", nav)
	}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := bridge.CallBridge(context.Background(), "read", map[string]any{"index": tabID}); err != nil {
				t.Errorf("read: %v", err)
			}
		}()
	}
	wg.Wait()

	if max := fb.maxPageOverlap(); max > 1 {
		t.Errorf("callers sharing tab %s ran %d at a time, want 1 (sequence %v)", tabID, max, fb.sequence())
	}
}

// TestTabFlowIsNotBlockedByAWedgedBrowser: the queue ceiling is what turns a wedged
// bridge into a readable "bridge busy" error, but a tab-addressed flow does not share
// the wedged resource, so it must not sit in that queue.
func TestTabFlowIsNotBlockedByAWedgedBrowser(t *testing.T) {
	fakeBrowserAt(t, 0)

	holder, err := bridge.DefaultGate().Acquire(context.Background(), "stuck-browser.chat")
	if err != nil {
		t.Fatalf("acquire holder: %v", err)
	}
	defer holder()

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "http://127.0.0.1:8899/one", "idle_ms": 0,
	}))
	if text := resultText(t, res); !strings.Contains(text, "marker-one-") {
		t.Fatalf("addressed fetch was blocked by an unrelated holder: %q", probeSnippet(text))
	}
}

// TestFetchReportsABusyBridgeInsteadOfHanging: with `bridge.tabParallel: false` every
// call is back on the single global queue, and a wedged bridge must surface as an
// actionable error naming the holder, never as a silently wrong page.
func TestFetchReportsABusyBridgeInsteadOfHanging(t *testing.T) {
	fakeBrowserAt(t, 0)
	serial := false
	restore := withBridgeQueue(t, mcpcfg.BridgeConfig{
		MaxConcurrent: 1, QueueTimeoutMs: 60, TabParallel: &serial,
	})
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
