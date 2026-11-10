package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func fetchReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "fetch",
			Arguments: args,
		},
	}
}

const fetchPageMarkdown = "# Example Domains\n\nThis domain is reserved for documentation."

// fetchFlow is the call sequence of the tab-addressed fetch flow: navigate opens
// its own background tab, the wait and the read carry that tab's registry id, and
// the tab is closed again on the way out.
const fetchFlow = "navigate,wait_for_network_idle,read,close_tab"

var fetchOKResponses = map[string]string{
	"navigate":              `{"success":true,"url":"https://example.com/","containerId":5,"title":"Example Domain","status":200,"statusText":"OK"}`,
	"wait_for_network_idle": `{"success":true,"data":{"idle":true,"deep":false}}`,
	"read":                  fmt.Sprintf(`{"success":true,"data":{"content":%q}}`, fetchPageMarkdown),
}

func TestFetchNavigatesWaitsThenReads(t *testing.T) {
	stub := bridgeStubAt(t, fetchOKResponses)

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/",
	}))

	if got, want := strings.Join(stub.actions(), ","), fetchFlow; got != want {
		t.Fatalf("bridge call sequence = %q, want %q", got, want)
	}

	tabID := stub.lastTabID()
	if !strings.HasPrefix(tabID, "t_") {
		t.Fatalf("navigate reported no tab id (%q)", tabID)
	}

	nav := stub.byAction("navigate")
	if nav["url"] != "https://example.com/" {
		t.Errorf("navigate url = %v", nav["url"])
	}
	// The page must load in this call's own background tab: a foreground new_tab
	// would move the selection out from under every other caller and the user.
	if nav["back_tab"] != true {
		t.Errorf("navigate must open a background tab, got %v", nav)
	}
	if nav["new_tab"] != nil {
		t.Errorf("navigate must not request a foreground new_tab, got %v", nav["new_tab"])
	}
	// Both bridge round-trips must carry a bounded load timeout instead of
	// inheriting the browser tool's 40-minute tool config.
	if ms, ok := nav["timeout"].(float64); !ok || ms <= 0 || ms > 30000 {
		t.Errorf("navigate timeout = %v, want a bounded positive ms", nav["timeout"])
	}
	idle := stub.byAction("wait_for_network_idle")
	if ms, ok := idle["timeout"].(float64); !ok || ms <= 0 || ms > 20000 {
		t.Errorf("wait timeout = %v, want a bounded positive ms", ms)
	}
	if idle["index"] != tabID {
		t.Errorf("wait is not addressed to the flow's tab: %v != %v", idle["index"], tabID)
	}
	if read := stub.byAction("read"); read["index"] != tabID {
		t.Errorf("read is not addressed to the flow's tab: %v != %v", read["index"], tabID)
	}
	if close := stub.byAction("close_tab"); close["index"] != tabID {
		t.Errorf("close_tab is not addressed to the flow's tab: %v != %v", close["index"], tabID)
	}
	if ms, ok := idle["idle_ms"].(float64); !ok || ms != fetchDefaultIdleMs {
		t.Errorf("idle_ms = %v, want the %d ms default", idle["idle_ms"], fetchDefaultIdleMs)
	}

	if read := stub.byAction("read"); read["selector"] != nil {
		t.Errorf("read must extract the whole page when no selector is given, got %v", read["selector"])
	}

	doc := docOf(t, res)
	wantVerbatim(t, doc.body, fetchPageMarkdown)
	// URL/title/status come back from the bridge, so redirects are visible.
	if doc.terms["url"] != "https://example.com/" {
		t.Errorf("url = %v", doc.terms["url"])
	}
	if doc.terms["title"] != "Example Domain" {
		t.Errorf("title = %v", doc.terms["title"])
	}
	if doc.terms["status"] != "200" {
		t.Errorf("status = %v, want 200", doc.terms["status"])
	}
	if _, present := doc.terms["selector"]; present {
		t.Errorf("selector must be omitted for a whole-page read, got %v", doc.terms["selector"])
	}
	if _, present := doc.terms["truncated"]; present {
		t.Errorf("truncated flag must be absent for a page that fits, got %v", doc.terms["truncated"])
	}
}

func TestFetchRequiresURL(t *testing.T) {
	for _, args := range []map[string]any{
		{},
		{"url": ""},
		{"url": "   \t "},
	} {
		stub := bridgeStubAt(t, fetchOKResponses)
		res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(args))
		if text := resultText(t, res); !strings.Contains(text, "url is required") {
			t.Errorf("args %v: expected missing-url error, got %q", args, text)
		}
		if len(stub.actions()) != 0 {
			t.Errorf("args %v: bridge must not be called, got %v", args, stub.actions())
		}
	}
}

func TestFetchNormalizesTargetURL(t *testing.T) {
	cases := []struct {
		name    string
		url     any
		wantURL string // "" => must be rejected before navigate
	}{
		{name: "scheme kept", url: "https://example.com/a?b=1", wantURL: "https://example.com/a?b=1"},
		{name: "query encoding untouched", url: "https://example.com/s?q=a%26b", wantURL: "https://example.com/s?q=a%26b"},
		{name: "bare host upgraded", url: "example.com/docs", wantURL: "https://example.com/docs"},
		{name: "www host upgraded", url: "www.iana.org/help", wantURL: "https://www.iana.org/help"},
		{name: "localhost is http", url: "localhost:3000/settings", wantURL: "http://localhost:3000/settings"},
		{name: "lan ip is http", url: "192.168.1.20/status", wantURL: "http://192.168.1.20/status"},
		{name: "mDNS name is http", url: "printer.local/", wantURL: "http://printer.local/"},
		{name: "entities decoded", url: "example.com/s?a=1&amp;b=2", wantURL: "https://example.com/s?a=1&b=2"},
		{name: "non-http scheme rejected", url: "ftp://example.com/file", wantURL: ""},
		{name: "hostless url rejected", url: "https://", wantURL: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := bridgeStubAt(t, fetchOKResponses)
			res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": tc.url}))

			nav := stub.byAction("navigate")
			if tc.wantURL == "" {
				if nav != nil {
					t.Fatalf("invalid url %v must abort before navigate; got %v", tc.url, nav)
				}
				return
			}
			if nav == nil {
				t.Fatalf("navigate never called: %s", resultText(t, res))
			}
			if nav["url"] != tc.wantURL {
				t.Errorf("navigate url = %v, want %v", nav["url"], tc.wantURL)
			}
		})
	}
}

func TestFetchUsesContainerMapForKnownSites(t *testing.T) {
	stub := bridgeStubAt(t, fetchOKResponses)

	HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://github.com/mark3labs/mcp-go",
	}))

	nav := stub.byAction("navigate")
	if nav == nil {
		t.Fatal("navigate never called")
	}
	// Parity with browser.go: known hosts land in their CONTAINER_MAP container.
	if nav["container"] != "Work" {
		t.Errorf("navigate container = %v, want Work", nav["container"])
	}
}

func TestFetchSelectorScopesExtraction(t *testing.T) {
	stub := bridgeStubAt(t, map[string]string{
		"navigate":              `{"success":true,"url":"https://example.com/","title":"Example Domain","status":200}`,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  `{"success":true,"data":{"content":"# Article","el":{"tag":"ARTICLE"}}}`,
	})

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url":       "https://example.com/blog/post",
		"selector":  " article ",
		"idle_ms":   float64(2500),
		"max_chars": "5000",
	}))

	read := stub.byAction("read")
	// The selector is trimmed: models routinely pad it with whitespace.
	if read["selector"] != "article" {
		t.Errorf("read selector = %v, want %q", read["selector"], "article")
	}
	idle := stub.byAction("wait_for_network_idle")
	if idle["idle_ms"] != float64(2500) {
		t.Errorf("idle_ms = %v, want 2500", idle["idle_ms"])
	}

	doc := docOf(t, res)
	wantVerbatim(t, doc.body, "# Article")
	if doc.terms["selector"] != "article" {
		t.Errorf("provenance selector = %v", doc.terms["selector"])
	}
	if _, present := doc.terms["status"]; !present {
		t.Error("status must be reported when the bridge captured it")
	}
}

func TestFetchParamValidation(t *testing.T) {
	cases := []struct {
		name  string
		args  map[string]any
		wants []string
	}{
		{name: "idle_ms zero allowed", args: map[string]any{"idle_ms": float64(0)}},
		{name: "idle_ms above ceiling", args: map[string]any{"idle_ms": float64(fetchMaxIdleMs + 1)}, wants: []string{"idle_ms must be between"}},
		{name: "negative idle_ms", args: map[string]any{"idle_ms": float64(-1)}, wants: []string{"idle_ms must be between"}},
		{name: "non numeric idle_ms", args: map[string]any{"idle_ms": "soon"}, wants: []string{"idle_ms must be a number"}},
		{name: "max_chars below floor", args: map[string]any{"max_chars": float64(10)}, wants: []string{"max_chars must be between"}},
		{name: "max_chars above ceiling", args: map[string]any{"max_chars": float64(fetchMaxChars + 1)}, wants: []string{"max_chars must be between"}},
		{name: "cli string max_chars", args: map[string]any{"max_chars": "40000"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := bridgeStubAt(t, fetchOKResponses)
			args := map[string]any{"url": "https://example.com/"}
			for k, v := range tc.args {
				args[k] = v
			}
			res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(args))

			if len(tc.wants) == 0 {
				if doc := docOf(t, res); doc.body != fetchPageMarkdown {
					t.Errorf("valid args %v must succeed, got %q", tc.args, probeSnippet(doc.body))
				}
				return
			}
			if nav := stub.byAction("navigate"); nav != nil {
				t.Fatalf("invalid args %v must abort before navigate; got %v", tc.args, nav)
			}
			text := resultText(t, res)
			for _, want := range tc.wants {
				if !strings.Contains(text, want) {
					t.Errorf("expected %q in error, got %q", want, text)
				}
			}
		})
	}
}

func TestFetchEmptyPageIsAnError(t *testing.T) {
	for _, body := range []string{
		`{"success":true,"data":{"content":"   \n "}}`,
		`{"success":true,"data":null}`,
	} {
		stub := bridgeStubAt(t, map[string]string{
			"navigate":              `{"success":true,"url":"https://example.com/","title":"Blank","status":200}`,
			"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
			"read":                  body,
		})

		res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/blank"}))
		if text := resultText(t, res); !strings.Contains(text, "has no readable content") {
			t.Errorf("body %s: expected empty-content error, got %q", body, text)
		}
		if len(stub.actions()) != 4 {
			t.Errorf("unexpected call sequence %v", stub.actions())
		}
	}
}

func TestFetchOmitsUncapturedNavigateStatus(t *testing.T) {
	// The bridge reports status 0 when it never saw the channel (cached/about:
	// style loads); that is "unknown" and must not be served as a status code.
	bridgeStubAt(t, map[string]string{
		"navigate":              `{"success":true,"url":"https://example.com/","title":"Example","status":0,"statusText":"unknown"}`,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  fmt.Sprintf(`{"success":true,"data":{"content":%q}}`, fetchPageMarkdown),
	})

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/"}))
	doc := docOf(t, res)
	if _, present := doc.terms["status"]; present {
		t.Errorf("status must be omitted when uncaptured, got %v", doc.terms["status"])
	}
	if doc.terms["url"] != "https://example.com/" || doc.terms["title"] != "Example" {
		t.Errorf("navigate report must still be surfaced: %v", doc.terms)
	}
}

func TestFetchReportsUnmatchedSelector(t *testing.T) {
	// Real bridge behaviour for a selector that matches nothing: HTTP 500 with
	// "Not found: <selector>". That must read as a miss, not a transport failure.
	stub := bridgeStubStatusAt(t, map[string]string{
		"navigate":              `{"success":true,"url":"https://example.com/","title":"Example","status":200}`,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  `{"success":false,"data":null,"error":"Not found: #missing"}`,
	}, map[string]int{"read": http.StatusInternalServerError})

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url":      "https://example.com/",
		"selector": "#missing",
	}))

	text := resultText(t, res)
	if !strings.Contains(text, "#missing") || !strings.Contains(text, "not found") {
		t.Errorf("expected selector-miss error, got %q", text)
	}
	if !strings.Contains(text, "Not found") {
		t.Errorf("bridge detail must be reported, got %q", text)
	}
	if stub.byAction("read") == nil {
		t.Error("read should still have been attempted")
	}
}

func TestFetchReportsBridgeFailure(t *testing.T) {
	// A read that fails for another reason (here a 500 with a JS error) keeps the
	// raw bridge detail so the caller can tell a broken page from a bad selector.
	bridgeStubStatusAt(t, map[string]string{
		"navigate":              `{"success":true,"url":"https://example.com/","status":200}`,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  `{"success":false,"error":"TypeError: win is undefined"}`,
	}, map[string]int{"read": http.StatusInternalServerError})

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/"}))
	if text := resultText(t, res); !strings.Contains(text, "read https://example.com/ failed") || !strings.Contains(text, "TypeError") {
		t.Errorf("expected read failure error, got %q", text)
	}
}

func TestFetchWaitFailureIsNonFatal(t *testing.T) {
	// A bridge-level failure in the idle wait must not lose the page: extraction
	// still runs against whatever already rendered.
	stub := bridgeStubAt(t, map[string]string{
		"navigate": fetchNavResponse,
		"read":     fmt.Sprintf(`{"success":true,"data":{"content":%q}}`, fetchPageMarkdown),
	}, "wait_for_network_idle")

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/"}))

	if doc := docOf(t, res); doc.body != fetchPageMarkdown {
		t.Errorf("body = %v, want %v", doc.body, fetchPageMarkdown)
	}
	if got, want := strings.Join(stub.actions(), ","), fetchFlow; got != want {
		t.Errorf("sequence must continue past the failed wait, got %q want %q", got, want)
	}
}

const fetchNavResponse = `{"success":true,"url":"https://example.com/","title":"Example Domain","status":200,"statusText":"OK"}`

func TestFetchNavigateFailureAbortsBeforeRead(t *testing.T) {
	cases := []struct {
		name      string
		responses map[string]string
		fail      []string
		want      string
	}{
		{
			name:      "transport failure",
			responses: map[string]string{"read": `{"success":true,"data":{"content":"stale page"}}`},
			fail:      []string{"navigate"},
			want:      "bridge down",
		},
		{
			// navigate answers HTTP 200 with success:false for its own timeouts.
			name:      "bridge timeout",
			responses: map[string]string{"navigate": `{"success":false,"error":"navigate_timeout"}`, "read": `{"success":true,"data":{"content":"stale page"}}`},
			want:      "navigate_timeout",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := bridgeStubAt(t, tc.responses, tc.fail...)

			res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://slow.example.com/"}))

			text := resultText(t, res)
			if strings.Contains(text, "stale page") {
				t.Fatalf("extraction must not run after a failed navigation: %q", text)
			}
			if !strings.Contains(text, tc.want) {
				t.Errorf("expected %q in error, got %q", tc.want, text)
			}
			if stub.byAction("read") != nil {
				t.Errorf("read must not run after a failed navigation; calls=%v", stub.actions())
			}
		})
	}
}

func TestFetchTruncatesOversizedPage(t *testing.T) {
	huge := strings.Repeat("a", fetchDefaultMaxChars+5000)
	bridgeStubAt(t, map[string]string{
		"navigate":              fetchNavResponse,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  fmt.Sprintf(`{"success":true,"data":{"content":%q}}`, huge),
	})

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/long"}))
	doc := docOf(t, res)
	if doc.terms["truncated"] != "true" {
		t.Errorf("truncated flag = %v, want it reported in provenance", doc.terms["truncated"])
	}
	if !strings.Contains(doc.body, fmt.Sprintf("[TRUNCATED %d chars]", 5000)) {
		t.Errorf("missing truncation marker in %d-char body", len(doc.body))
	}
	if len(doc.body) >= len(huge) {
		t.Errorf("body not clamped: %d >= %d", len(doc.body), len(huge))
	}
}

func TestFetchRespondsWithRawMarkdownNotJSON(t *testing.T) {
	// The payload is markdown: a JSON envelope would escape every newline and
	// quote of the page, which is pure token overhead for the caller.
	page := "# Quotes \"and\" newlines\n\n- item \\ backslash\n- caf\u00e9 \u4e16"
	stub := bridgeStubAt(t, map[string]string{
		"navigate":              `{"success":true,"url":"https://example.com/","title":"Quotes","status":200}`,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  fmt.Sprintf(`{"success":true,"data":{"content":%q}}`, page),
	})

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/"}))

	text := resultText(t, res)
	if !strings.Contains(text, "\n\n- item \\ backslash\n") {
		t.Errorf("markdown must reach the caller unescaped, got %q", probeSnippet(text))
	}
	if strings.Contains(text, `\n`) || strings.Contains(text, "\"content\"") || strings.Contains(text, "firefox_bridge_read") {
		t.Errorf("response still looks JSON-shaped: %q", probeSnippet(text))
	}
	doc := docOf(t, res)
	wantVerbatim(t, doc.body, page)
	if len(stub.actions()) != 4 {
		t.Errorf("unexpected call sequence %v", stub.actions())
	}
}

func TestFetchMarkdownIsSmallerThanTheJSONItReplaces(t *testing.T) {
	// Realistic page markdown is full of newlines and quotes; a JSON envelope has
	// to escape every one of them, so the same payload costs more tokens.
	page := strings.Repeat("## Section \"quoted\"\n\n- bullet \\ backslash\n- caf\u00e9\n\n", 80)
	bridgeStubAt(t, map[string]string{
		"navigate":              `{"success":true,"url":"https://example.com/","title":"Long Page","status":200}`,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  fmt.Sprintf(`{"success":true,"data":{"content":%q}}`, page),
	})

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/"}))
	text := resultText(t, res)

	raw, err := json.Marshal(map[string]any{
		"url":       "https://example.com/",
		"title":     "Long Page",
		"source":    "firefox_bridge_read",
		"content":   strings.TrimSpace(page),
		"truncated": false,
		"status":    200,
	})
	if err != nil {
		t.Fatalf("marshal JSON baseline: %v", err)
	}
	if len(text) >= len(raw) {
		t.Errorf("markdown response = %d bytes, want smaller than the %d-byte JSON envelope", len(text), len(raw))
	}
}

func TestFetchProvenanceTerms(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "full navigate report",
			line: fetchProvenance("https://example.com/", " Example Domain ", 200, "", false),
			want: "url: https://example.com/ | title: Example Domain | status: 200",
		},
		{
			name: "uncaptured status and no title",
			line: fetchProvenance("https://example.com/", "", 0, "article", true),
			want: "url: https://example.com/ | selector: article | truncated",
		},
		{
			name: "url always present",
			line: fetchProvenance("https://example.com/", "", 0, "", false),
			want: "url: https://example.com/",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.line != tc.want {
				t.Errorf("provenance = %q, want %q", tc.line, tc.want)
			}
		})
	}
}

func TestClampBridgeContentIsRuneSafe(t *testing.T) {
	// 21000 bytes of a 3-byte rune: a byte-offset cut lands mid-rune, so the
	// clamped string must not carry a dangling partial sequence.
	multibyte := strings.Repeat("世", fetchDefaultMaxChars/3+1000)
	if len(multibyte) <= fetchDefaultMaxChars {
		t.Fatalf("fixture too small to exercise truncation: %d bytes", len(multibyte))
	}

	clamped, truncated := clampBridgeContent(multibyte, fetchDefaultMaxChars)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if !utf8.ValidString(clamped) {
		t.Errorf("clamped content is not valid UTF-8")
	}
	short, truncated := clampBridgeContent("plain page", 4096)
	if truncated {
		t.Error("short content must not be flagged truncated")
	}
	if short != "plain page" {
		t.Errorf("short content must pass through, got %q", short)
	}
}

func TestFetchToolRegisteredInAllDefs(t *testing.T) {
	stub := bridgeStubAt(t, fetchOKResponses)

	var found *ToolDef
	defs := AllDefs("/ws", Deps{})
	for i := range defs {
		if defs[i].Name == "fetch" {
			found = &defs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("fetch tool not registered in AllDefs")
	}
	if found.Handler == nil {
		t.Fatal("fetch tool has no handler")
	}
	if got, ok := found.Schema["required"].([]string); !ok || len(got) != 1 || got[0] != "url" {
		t.Errorf("fetch schema required = %v, want [url]", found.Schema["required"])
	}
	props, _ := found.Schema["properties"].(map[string]any)
	for _, name := range []string{"url", "selector", "idle_ms", "max_chars", "keep_tab"} {
		if _, ok := props[name]; !ok {
			t.Errorf("fetch schema missing %s property", name)
		}
	}

	res, err := found.Handler(context.Background(), fetchReq(map[string]any{"url": "https://example.com/"}))
	if err != nil {
		t.Fatalf("handler returned go error: %v", err)
	}
	if doc := docOf(t, res); doc.body != fetchPageMarkdown {
		t.Errorf("registered handler body = %q", probeSnippet(doc.body))
	}
	if len(stub.actions()) != 4 {
		t.Errorf("registered handler sequence = %v", stub.actions())
	}
}

// Section addressing: a long page must be resumable and sliceable by the offsets the
// tool itself reported, which is what separates a fetch that answers a question from a
// fetch that dumps a page and cuts it off.

const fetchLongPage = `# Fetch API

Intro prose that sets the page up.

## Concepts and usage

Some prose about the concepts of the page, long enough to be worth keeping.

## Browser compatibility

| Feature | Chrome | Firefox |
| --- | --- | --- |
| fetch | 42 | 39 |

## See also

- Using Fetch
`

func fetchPageResponses(t *testing.T, data string) map[string]string {
	t.Helper()
	return map[string]string{
		"navigate":              fetchNavResponse,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  fmt.Sprintf(`{"success":true,"data":%s}`, data),
	}
}

func TestFetchOutlineReturnsTheSectionMap(t *testing.T) {
	fixture := fmt.Sprintf(`{"content":%q,"extracted":"landmark","shadow_roots":2}`, fetchLongPage)
	bridgeStubAt(t, fetchPageResponses(t, fixture))

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/", "outline": true,
	}))
	doc := docOf(t, res)
	for _, want := range []string{"Concepts and usage", "Browser compatibility", "See also"} {
		if !strings.Contains(doc.body, want) {
			t.Errorf("outline is missing section %q: %q", want, probeSnippet(doc.body))
		}
	}
	if strings.Contains(doc.body, "Intro prose") {
		t.Errorf("outline mode must not return the body: %q", probeSnippet(doc.body))
	}
	if doc.terms["extracted"] != "landmark" || doc.terms["shadow"] != "2" {
		t.Errorf("extraction report not surfaced in provenance: %q", probeSnippet(doc.body))
	}
}

func TestFetchSectionSelectsOneSection(t *testing.T) {
	bridgeStubAt(t, fetchPageResponses(t, fmt.Sprintf(`{"content":%q,"extracted":"landmark"}`, fetchLongPage)))

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/", "section": "compatibility",
	}))
	doc := docOf(t, res)
	if !strings.HasPrefix(strings.TrimSpace(doc.body), "## Browser compatibility") {
		t.Errorf("section payload must start at the matched heading, got %q", probeSnippet(doc.body))
	}
	if strings.Contains(doc.body, "See also") {
		t.Errorf("section must stop at the next heading of the same level: %q", probeSnippet(doc.body))
	}
	if strings.Contains(doc.body, "Concepts and usage") {
		t.Errorf("section must not include earlier content: %q", probeSnippet(doc.body))
	}
	if doc.terms["section"] != "compatibility" {
		t.Errorf("provenance must report the selected section, got %q", doc.terms["section"])
	}
}

func TestFetchUnknownSectionNamesTheSections(t *testing.T) {
	bridgeStubAt(t, fetchPageResponses(t, fmt.Sprintf(`{"content":%q}`, fetchLongPage)))

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/", "section": "browser support matrix",
	}))
	text := resultText(t, res)
	if !strings.Contains(text, "no section") {
		t.Fatalf("want an unknown-section error, got %q", probeSnippet(text))
	}
	// The names of what IS there turn one wasted call into none.
	for _, want := range []string{"Concepts and usage", "See also"} {
		if !strings.Contains(text, want) {
			t.Errorf("unknown-section error must list the available sections, missing %q", want)
		}
	}
}

func TestFetchClampsAtSectionBoundaryAndReportsNextStart(t *testing.T) {
	// Two sections, the second one starting inside the budget: the cut belongs there.
	page := "# Head\n\n" + strings.Repeat("a", 1500) + "\n\n## Tail\n\n" + strings.Repeat("b", 4000)
	bridgeStubAt(t, fetchPageResponses(t, fmt.Sprintf(`{"content":%q,"extracted":"landmark"}`, page)))

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/", "max_chars": 2000,
	}))
	doc := docOf(t, res)
	if doc.terms["truncated"] != "true" {
		t.Fatal("expected the truncated flag")
	}
	if strings.Contains(doc.body, "bbbb") {
		t.Errorf("cut must land on the section edge, not inside the tail: %q", probeSnippet(doc.body))
	}
	next, ok := doc.terms["next_start"]
	if !ok {
		t.Fatal("a clamped payload must report where to continue")
	}
	offset, err := strconv.Atoi(next)
	if err != nil {
		t.Fatalf("next_start is not an offset: %q", next)
	}
	if !strings.HasPrefix(page[offset:], "## Tail") {
		t.Errorf("next_start %d does not point at the omitted heading", offset)
	}
	if doc.terms["size"] != fmt.Sprint(len(page)) {
		t.Errorf("size term = %q, want the full page length %d", doc.terms["size"], len(page))
	}
}

func TestFetchStartIndexResumesWhereNextStartPointed(t *testing.T) {
	page := "# Head\n\n" + strings.Repeat("a", 1500) + "\n\n## Tail\n\n" + strings.Repeat("b", 4000)
	bridgeStubAt(t, fetchPageResponses(t, fmt.Sprintf(`{"content":%q}`, page)))

	first := docOf(t, HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/", "max_chars": 2000,
	})))
	resume, err := strconv.Atoi(first.terms["next_start"])
	if err != nil {
		t.Fatalf("no usable next_start in %v", first.terms)
	}

	second := docOf(t, HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/", "max_chars": 2000, "start_index": resume,
	})))
	if !strings.HasPrefix(strings.TrimSpace(second.body), "## Tail") {
		t.Errorf("resumed payload = %q, want it to start at the omitted section", probeSnippet(second.body))
	}
}

func TestFetchStartIndexPastTheEndIsAnError(t *testing.T) {
	bridgeStubAt(t, fetchPageResponses(t, fmt.Sprintf(`{"content":%q}`, fetchLongPage)))

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/", "start_index": 900000,
	}))
	if text := resultText(t, res); !strings.Contains(text, "past the end") {
		t.Errorf("want an out-of-range error, got %q", probeSnippet(text))
	}
}

func TestFetchReportsWholeBodyFallback(t *testing.T) {
	// The MDN failure mode: article detection gave up and handed back the whole body.
	// That has to be visible without reading the payload.
	bridgeStubAt(t, fetchPageResponses(t, `{"content":"# Example Domains","extracted":"body","unhydrated":4,"extract_error":"Readability is not a constructor"}`))

	doc := docOf(t, HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/",
	})))
	if doc.terms["extracted"] != "body" {
		t.Errorf("extracted = %q, want body", doc.terms["extracted"])
	}
	if doc.terms["unhydrated"] != "4" {
		t.Errorf("unhydrated = %q, want 4", doc.terms["unhydrated"])
	}
	if !strings.Contains(provenanceLine(doc), "Readability is not a constructor") {
		t.Errorf("extract_error not surfaced: %q", probeSnippet(doc.body))
	}
}

func provenanceLine(doc bridgeDoc) string {
	terms := make([]string, 0, len(doc.terms))
	for k, v := range doc.terms {
		terms = append(terms, k+":"+v)
	}
	return strings.Join(terms, " | ")
}

func TestBridgeHeadingsSkipsFencedCode(t *testing.T) {
	md := "# Title\n\n```md\n## not a heading\n```\n\n## Real\n\ntext\n"
	heads := bridgeHeadings(md)
	if len(heads) != 2 {
		t.Fatalf("headings = %+v, want Title and Real", heads)
	}
	if heads[1].title != "Real" {
		t.Errorf("second heading = %q", heads[1].title)
	}
	if !strings.HasPrefix(md[heads[1].start:], "## Real") {
		t.Errorf("offset %d does not point at the heading", heads[1].start)
	}
}

func TestFetchSchemaDeclaresTheAddressingParams(t *testing.T) {
	found := lookupDef(t, "fetch")
	props, _ := found.Schema["properties"].(map[string]any)
	for _, name := range []string{"outline", "section", "start_index"} {
		if _, ok := props[name]; !ok {
			t.Errorf("fetch schema missing %s property", name)
		}
	}
}

// lookupDef finds a registered tool definition by name, for schema assertions.
func lookupDef(t *testing.T, name string) ToolDef {
	t.Helper()
	defs := AllDefs("/ws", Deps{})
	for i := range defs {
		if defs[i].Name == name {
			return defs[i]
		}
	}
	t.Fatalf("tool %q is not registered in AllDefs", name)
	return ToolDef{}
}

func TestFetchReadsTextBooleansFromTheCLIWrapper(t *testing.T) {
	// The generated z* wrappers post every value as a string, so -o true must behave
	// exactly like outline: true. Before bridgeBool it silently returned the whole page.
	page := "# Head\n\nprose\n\n## Tail\n\nmore\n"
	bridgeStubAt(t, fetchPageResponses(t, fmt.Sprintf(`{"content":%q}`, page)))

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/", "outline": "true",
	}))
	doc := docOf(t, res)
	if !strings.Contains(doc.body, "section map") {
		t.Errorf("outline=%q must select outline mode, body = %q", "true", probeSnippet(doc.body))
	}

}
