package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// bridgeStub is a fake Firefox bridge that answers each action with a canned
// body while recording every POST, so tests can assert the exact call sequence
// (navigate -> wait_for_network_idle -> read) a bridge-backed tool produces.
type bridgeStub struct {
	mu   sync.Mutex
	reqs []map[string]any
	// responses maps a bridge action to the JSON body returned for it; a missing
	// entry falls back to a generic success body.
	responses map[string]string
	// statuses forces a non-200 status for an action, e.g. the bridge's HTTP 500
	// "Not found" answer for an unmatched selector.
	statuses map[string]int
	// fail lists actions answered with HTTP 502, i.e. a transport-level bridge
	// failure (as opposed to a success:false body).
	fail map[string]bool
}

func (s *bridgeStub) serve(w http.ResponseWriter, req *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	action, _ := body["action"].(string)

	s.mu.Lock()
	s.reqs = append(s.reqs, body)
	raw, ok := s.responses[action]
	failed := s.fail[action]
	status := s.statuses[action]
	s.mu.Unlock()

	if failed {
		http.Error(w, "bridge down", http.StatusBadGateway)
		return
	}
	if status != 0 {
		http.Error(w, raw, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		raw = `{"success":true,"data":{"content":"generic"}}`
	}
	_, _ = w.Write([]byte(raw))
}

func (s *bridgeStub) actions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.reqs))
	for _, r := range s.reqs {
		out = append(out, fmt.Sprint(r["action"]))
	}
	return out
}

func (s *bridgeStub) byAction(action string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.reqs {
		if r["action"] == action {
			return r
		}
	}
	return nil
}

// bridgeStubAt installs a fake bridge for the duration of the test, so no test
// can accidentally drive the real Firefox bridge.
func bridgeStubAt(t *testing.T, responses map[string]string, fail ...string) *bridgeStub {
	return installBridgeStub(t, responses, nil, fail...)
}

// bridgeStubStatusAt additionally answers the listed actions with a non-200
// status carrying the canned body.
func bridgeStubStatusAt(t *testing.T, responses map[string]string, statuses map[string]int) *bridgeStub {
	return installBridgeStub(t, responses, statuses)
}

func installBridgeStub(t *testing.T, responses map[string]string, statuses map[string]int, fail ...string) *bridgeStub {
	t.Helper()
	failures := map[string]bool{}
	for _, a := range fail {
		failures[a] = true
	}
	stub := &bridgeStub{responses: responses, statuses: statuses, fail: failures}
	ts := httptest.NewServer(http.HandlerFunc(stub.serve))
	t.Cleanup(ts.Close)
	pointBridgeAt(t, ts.URL)
	return stub
}

// bridgeDoc is a parsed markdown response of a bridge-backed content tool
// (fetch, search): the single leading provenance line split into its terms,
// plus the payload body that follows it.
type bridgeDoc struct {
	terms map[string]string
	body  string
}

// docOf parses a tool response and pins the token-efficiency contract: raw
// markdown (never a JSON envelope, which would escape every newline of a
// multi-kilobyte page), one provenance line, then the payload verbatim.
func docOf(t *testing.T, res *mcp.CallToolResult) bridgeDoc {
	t.Helper()
	text := resultText(t, res)
	if trimmed := strings.TrimSpace(text); strings.HasPrefix(trimmed, "{") {
		var probe map[string]any
		if json.Unmarshal([]byte(trimmed), &probe) == nil {
			t.Fatalf("response is a JSON envelope, want raw markdown: %q", probeSnippet(trimmed))
		}
	}
	line, body, ok := strings.Cut(text, "\n\n")
	if !ok {
		t.Fatalf("response has no provenance/body split: %q", probeSnippet(text))
	}
	if strings.Contains(line, "\n") {
		t.Errorf("provenance must stay on a single line, got %q", probeSnippet(line))
	}
	doc := bridgeDoc{terms: map[string]string{}}
	for _, term := range strings.Split(line, " | ") {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		label, value, hasValue := strings.Cut(term, ": ")
		if !hasValue {
			// Bare flag term, e.g. "truncated".
			doc.terms[term] = "true"
			continue
		}
		doc.terms[strings.TrimSpace(label)] = strings.TrimSpace(value)
	}
	doc.body = strings.TrimSuffix(body, "\n")
	return doc
}

// wantVerbatim asserts that the extracted text reached the response untouched,
// i.e. without JSON string escaping or re-wrapping.
func wantVerbatim(t *testing.T, body, want string) {
	t.Helper()
	if body != want {
		t.Errorf("body = %q, want the extracted text verbatim %q", probeSnippet(body), probeSnippet(want))
	}
	for _, escape := range []string{`\n`, `\"`, `\u00`} {
		if strings.Contains(body, escape) {
			t.Errorf("body carries JSON string escapes (%s): %q", escape, probeSnippet(body))
		}
	}
}

// probeSnippet shortens a long payload for failure messages.
func probeSnippet(s string) string {
	const max = 240
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// resultText flattens a call result into its text content for message asserts.
func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil {
		t.Fatal("nil result")
	}
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return text.String()
}
