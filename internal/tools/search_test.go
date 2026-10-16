package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// searchSERP builds the SERP URL the search tool is expected to navigate to:
// the query plus the fixed udm=50 view-mode hint, url.Values-encoded in key order.
func searchSERP(encodedQuery string) string {
	return fmt.Sprintf("https://www.google.com/search?q=%s&udm=%d", encodedQuery, searchDefaultNum)
}

func searchReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "search",
			Arguments: args,
		},
	}
}

func searchPayload(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	text := resultText(t, res)
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("result is not a JSON payload: %q", text)
	}
	return out
}

const searchOverviewAnswer = "HyperFrames is an open-source video rendering framework by HeyGen."

var searchOKResponse = fmt.Sprintf(`{"success":true,"data":{"content":%q,"el":{"tag":"DIV"}}}`, searchOverviewAnswer)

var searchOKResponses = map[string]string{
	"navigate":          `{"success":true,"index":7046}`,
	"wait_for_selector": `{"success":true,"data":true}`,
	"get_text":          searchOKResponse,
}

func TestSearchNavigatesGoogleSERPAndExtractsAIOverview(t *testing.T) {
	stub := bridgeStubAt(t, searchOKResponses)

	res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(map[string]any{
		"query": "how to use hyperframes",
	}))

	if got, want := strings.Join(stub.actions(), ","), "navigate,wait_for_network_idle,wait_for_selector,get_text"; got != want {
		t.Fatalf("bridge call sequence = %q, want %q", got, want)
	}

	nav := stub.byAction("navigate")
	// The SERP URL must carry the query plus the udm= view-mode hint, with spaces
	// encoded as '+' (Google's own search-link format).
	if nav["url"] != searchSERP("how+to+use+hyperframes") {
		t.Errorf("navigate url = %v", nav["url"])
	}
	if nav["new_tab"] != true {
		t.Errorf("navigate must open a foreground new_tab, got %v", nav["new_tab"])
	}
	// Parity with browser.go: google.com traffic uses the CONTAINER_MAP container.
	if nav["container"] != "Personal" {
		t.Errorf("navigate container = %v, want Personal", nav["container"])
	}

	wait := stub.byAction("wait_for_selector")
	if wait["selector"] != aiOverviewSelector {
		t.Errorf("wait selector = %v", wait["selector"])
	}
	if wait["deep"] != true {
		t.Errorf("wait must use deep mode so a streaming overview settles, got %v", wait["deep"])
	}
	// Both bridge calls must carry a bounded page-load timeout; inheriting the
	// browser tool's 40-minute tool config would stall the handler on a SERP that
	// never renders an AI Overview.
	if ms, ok := nav["timeout"].(float64); !ok || ms <= 0 || ms > 30000 {
		t.Errorf("navigate timeout = %v, want a bounded positive ms", nav["timeout"])
	}
	if ms, ok := wait["timeout"].(float64); !ok || ms <= 0 || ms > 30000 {
		t.Errorf("wait timeout = %v, want a bounded positive ms", wait["timeout"])
	}

	get := stub.byAction("get_text")
	if get["selector"] != aiOverviewSelector {
		t.Fatalf("get_text selector = %v, want %v", get["selector"], aiOverviewSelector)
	}

	out := searchPayload(t, res)
	if out["content"] != searchOverviewAnswer {
		t.Errorf("content = %v, want %v", out["content"], searchOverviewAnswer)
	}
	if out["query"] != "how to use hyperframes" {
		t.Errorf("query = %v", out["query"])
	}
	if out["source"] != "google_ai_overview" {
		t.Errorf("source = %v", out["source"])
	}
	if out["truncated"] != false {
		t.Errorf("truncated = %v, want false", out["truncated"])
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	for _, args := range []map[string]any{
		{},
		{"query": ""},
		{"query": "   \t "},
	} {
		stub := bridgeStubAt(t, searchOKResponses)
		res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(args))
		if text := resultText(t, res); !strings.Contains(text, "query is required") {
			t.Errorf("args %v: expected missing-query error, got %q", args, text)
		}
		if len(stub.actions()) != 0 {
			t.Errorf("args %v: bridge must not be called, got %v", args, stub.actions())
		}
	}
}

func TestSearchNumParam(t *testing.T) {
	cases := []struct {
		name    string
		num     any
		wantNum string // expected udm= value in the SERP URL, "" => must error
	}{
		{name: "default", num: nil, wantNum: "50"},
		{name: "json number", num: float64(50), wantNum: "50"},
		{name: "cli string", num: "50", wantNum: "50"},
		{name: "below the fixed page size rejected", num: float64(10), wantNum: ""},
		{name: "zero rejected", num: float64(0), wantNum: ""},
		{name: "negative rejected", num: float64(-1), wantNum: ""},
		{name: "above the fixed page size rejected", num: float64(51), wantNum: ""},
		{name: "non numeric string rejected", num: "ten", wantNum: ""},
		{name: "wrong type rejected", num: []any{1}, wantNum: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := bridgeStubAt(t, searchOKResponses)
			args := map[string]any{"query": "hyperframes"}
			if tc.num != nil {
				args["num"] = tc.num
			}
			res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(args))

			nav := stub.byAction("navigate")
			if tc.wantNum == "" {
				if nav != nil {
					t.Fatalf("invalid num %v must abort before navigate; got %v", tc.num, nav)
				}
				if text := resultText(t, res); !strings.Contains(text, "num must be") {
					t.Fatalf("expected num validation error, got %q", text)
				}
				return
			}
			if nav == nil {
				t.Fatalf("navigate never called: %s", resultText(t, res))
			}
			if want := "udm=" + tc.wantNum; !strings.Contains(fmt.Sprint(nav["url"]), want) {
				t.Errorf("navigate url = %v, want to contain %q", nav["url"], want)
			}
			if out := searchPayload(t, res); fmt.Sprint(out["num"]) != tc.wantNum {
				t.Errorf("payload num = %v, want %q", out["num"], tc.wantNum)
			}
		})
	}
}

func TestSearchReportsMissingAIOverview(t *testing.T) {
	// The bridge answers HTTP 200 with success:false when the selector matches
	// nothing (query with no AI Overview) — that must surface as an error, not an
	// empty success payload.
	stub := bridgeStubAt(t, map[string]string{
		"navigate":          `{"success":true,"index":7046}`,
		"wait_for_selector": `{"success":true,"data":false}`,
		"get_text":          `{"success":false,"data":null,"error":"Not found: div[data-subtree=\"aimc\"]"}`,
	})

	res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(map[string]any{
		"query": "obscure thing with no overview",
	}))

	text := resultText(t, res)
	if !strings.Contains(text, "no AI Overview") {
		t.Errorf("expected AI Overview miss error, got %q", text)
	}
	if !strings.Contains(text, "Not found") {
		t.Errorf("bridge detail must be reported, got %q", text)
	}
	if stub.byAction("get_text") == nil {
		t.Error("get_text should still have been attempted")
	}
}

func TestSearchReportsMissingAIOverviewOverHTTPError(t *testing.T) {
	// Real bridge behaviour for a SERP with no AI Overview: get_text answers HTTP
	// 500 with "Not found: <selector>". That must read as a miss, not a raw
	// transport failure.
	stub := bridgeStubStatusAt(t, map[string]string{
		"navigate":          `{"success":true,"index":7046}`,
		"wait_for_selector": `{"success":true,"data":false}`,
		"get_text":          `{"success":false,"data":null,"error":"Not found: div[data-subtree=\"aimc\"]"}`,
	}, map[string]int{"get_text": http.StatusInternalServerError})

	res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(map[string]any{
		"query": "zzqx nonexistentsearchterm",
	}))

	text := resultText(t, res)
	if !strings.Contains(text, "no AI Overview") {
		t.Errorf("expected AI Overview miss error, got %q", text)
	}
	if !strings.Contains(text, "zzqx nonexistentsearchterm") || !strings.Contains(text, "Not found") {
		t.Errorf("query and bridge detail must be reported, got %q", text)
	}
	if stub.byAction("get_text") == nil {
		t.Error("get_text should still have been attempted")
	}
}

func TestSearchEmptyOverviewIsAnError(t *testing.T) {
	stub := bridgeStubAt(t, map[string]string{
		"navigate":          `{"success":true,"index":7046}`,
		"wait_for_selector": `{"success":true,"data":true}`,
		"get_text":          `{"success":true,"data":{"content":"   ","el":{"tag":"DIV"}}}`,
	})

	res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(map[string]any{"query": "x"}))
	if text := resultText(t, res); !strings.Contains(text, "is empty") {
		t.Errorf("expected empty-overview error, got %q", text)
	}
	if len(stub.actions()) != 4 {
		t.Errorf("unexpected call sequence %v", stub.actions())
	}
}

func TestSearchWaitFailureIsNonFatal(t *testing.T) {
	// A bridge-level failure in the wait must not lose the search: extraction
	// still runs against whatever the SERP already rendered.
	stub := bridgeStubAt(t, map[string]string{
		"navigate": `{"success":true,"index":7046}`,
		"get_text": searchOKResponse,
	}, "wait_for_selector")

	res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(map[string]any{"query": "hyperframes"}))

	if out := searchPayload(t, res); out["content"] != searchOverviewAnswer {
		t.Errorf("content = %v, want %v", out["content"], searchOverviewAnswer)
	}
	if got, want := strings.Join(stub.actions(), ","), "navigate,wait_for_network_idle,wait_for_selector,get_text"; got != want {
		t.Errorf("sequence must continue past the failed wait, got %q want %q", got, want)
	}
}

func TestSearchNavigateFailureAbortsBeforeExtraction(t *testing.T) {
	stub := bridgeStubAt(t, map[string]string{"get_text": searchOKResponse}, "navigate")

	res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(map[string]any{"query": "hyperframes"}))

	if text := resultText(t, res); strings.Contains(text, "HyperFrames") {
		t.Fatalf("extraction must not run after a failed navigation: %q", text)
	}
	if stub.byAction("get_text") != nil {
		t.Errorf("get_text must not run after a failed navigation; calls=%v", stub.actions())
	}
}

func TestSearchTruncatesOversizedOverview(t *testing.T) {
	huge := strings.Repeat("a", searchMaxContentChars+5000)
	bridgeStubAt(t, map[string]string{
		"navigate":          `{"success":true,"index":7046}`,
		"wait_for_selector": `{"success":true,"data":true}`,
		"get_text":          fmt.Sprintf(`{"success":true,"data":{"content":%q}}`, huge),
	})

	res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(map[string]any{"query": "hyperframes"}))
	out := searchPayload(t, res)
	content, _ := out["content"].(string)
	if out["truncated"] != true {
		t.Errorf("truncated = %v, want true", out["truncated"])
	}
	if !strings.Contains(content, fmt.Sprintf("[TRUNCATED %d chars]", 5000)) {
		t.Errorf("missing truncation marker in %d-char content", len(content))
	}
	if len(content) >= len(huge) {
		t.Errorf("content not clamped: %d >= %d", len(content), len(huge))
	}
}

func TestClampSearchContentIsRuneSafe(t *testing.T) {
	// 21000 bytes of a 3-byte rune: the 20000-byte cut lands mid-rune, so the
	// clamped string must not carry a dangling partial sequence.
	multibyte := strings.Repeat("世", searchMaxContentChars/3+1000)
	if len(multibyte) <= searchMaxContentChars {
		t.Fatalf("fixture too small to exercise truncation: %d bytes", len(multibyte))
	}

	clamped, truncated := clampSearchContent(multibyte)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if !utf8.ValidString(clamped) {
		t.Errorf("clamped content is not valid UTF-8")
	}
	short, truncated := clampSearchContent("plain answer")
	if truncated {
		t.Error("short content must not be flagged truncated")
	}
	if short != "plain answer" {
		t.Errorf("short content must pass through, got %q", short)
	}
}

func TestSearchDecodesHTMLEntitiesInQuery(t *testing.T) {
	stub := bridgeStubAt(t, searchOKResponses)

	HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(map[string]any{
		"query": "coffee &amp; tea",
	}))

	nav := stub.byAction("navigate")
	if nav == nil {
		t.Fatal("navigate never called")
	}
	urlText := fmt.Sprint(nav["url"])
	// Decoded '&' must be percent-encoded once, never double-encoded to pct2526.
	const want = "q=coffee+" + "%26" + "+tea"
	if strings.Contains(urlText, "%2526") || !strings.Contains(urlText, want) {
		t.Errorf("navigate url = %q, want to contain %q", urlText, want)
	}
}

func TestSearchParsesListShapedBridgePayload(t *testing.T) {
	// get_text may answer with an array of matched elements; the tool must still
	// find the answer text instead of returning an empty payload.
	stub := bridgeStubAt(t, map[string]string{
		"navigate":          `{"success":true,"index":7046}`,
		"wait_for_selector": `{"success":true,"data":true}`,
		"get_text":          `{"success":true,"data":["",{"content":"part one"},"part two"]}`,
	})

	res := HandleSearchAction(context.Background(), t.TempDir(), Deps{}, searchReq(map[string]any{"query": "hyperframes"}))
	out := searchPayload(t, res)
	if out["content"] != "part one\n\npart two" {
		t.Errorf("content = %v", out["content"])
	}
	if stub.byAction("get_text") == nil {
		t.Error("get_text never called")
	}
}

func TestSearchToolRegisteredInAllDefs(t *testing.T) {
	stub := bridgeStubAt(t, searchOKResponses)

	var found *ToolDef
	defs := AllDefs("/ws", Deps{})
	for i := range defs {
		if defs[i].Name == "search" {
			found = &defs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("search tool not registered in AllDefs")
	}
	if found.Handler == nil {
		t.Fatal("search tool has no handler")
	}
	if got, ok := found.Schema["required"].([]string); !ok || len(got) != 1 || got[0] != "query" {
		t.Errorf("search schema required = %v, want [query]", found.Schema["required"])
	}
	props, _ := found.Schema["properties"].(map[string]any)
	if _, ok := props["query"]; !ok {
		t.Error("search schema missing query property")
	}

	res, err := found.Handler(context.Background(), searchReq(map[string]any{"query": "hyperframes"}))
	if err != nil {
		t.Fatalf("handler returned go error: %v", err)
	}
	if out := searchPayload(t, res); out["content"] != searchOverviewAnswer {
		t.Errorf("registered handler content = %v", out["content"])
	}
	if len(stub.actions()) != 4 {
		t.Errorf("registered handler sequence = %v", stub.actions())
	}
}

func TestContainerForURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
		ok   bool
	}{
		{url: searchSERP("x"), want: "Personal", ok: true},
		{url: "https://github.com/owner/repo", want: "Work", ok: true},
		{url: "https://slack.com/app", want: "Work", ok: true},
		{url: "https://www.youtube.com/watch?v=x", want: "Personal", ok: true},
		{url: "https://localhost:3000/", want: "Development", ok: true},
		{url: "https://example.com/thing", want: "", ok: false},
		{url: "", want: "", ok: false},
	}
	for _, tc := range cases {
		got, ok := containerForURL(tc.url)
		if ok != tc.ok || got != tc.want {
			t.Errorf("containerForURL(%q) = (%q,%v), want (%q,%v)", tc.url, got, ok, tc.want, tc.ok)
		}
	}
}

func TestGoogleSearchURL(t *testing.T) {
	cases := []struct{ query, want string }{
		{"how to use hyperframes", searchSERP("how+to+use+hyperframes")},
		{"a&b", searchSERP("a%26b")},
		{"emoji 😀", searchSERP("emoji+%F0%9F%98%80")},
	}
	for _, tc := range cases {
		if got := googleSearchURL(tc.query, searchDefaultNum); got != tc.want {
			t.Errorf("googleSearchURL(%q) = %q, want %q", tc.query, got, tc.want)
		}
	}
}
