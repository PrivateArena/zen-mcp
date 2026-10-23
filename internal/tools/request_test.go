package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func requestReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "request",
			Arguments: args,
		},
	}
}

// requestRawSource is the untouched HTML source of the target, which is exactly
// what the request tool must hand back: unlike fetch there is no Readability pass,
// so tags, scripts and whitespace survive.
const requestRawSource = `<!doctype html>
<html lang="en">
  <head><title>Example Domain</title></head>
  <body><h1>Example Domain</h1><p>This domain is reserved for documentation.</p></body>
</html>`

var requestOKResponses = map[string]string{
	"request": fmt.Sprintf(`{"success":true,"status":200,"statusText":"OK","headers":{"content-type":"text/html; charset=utf-8","server":"cloudflare"},"data":%q}`, requestRawSource),
}

// requestRun drives the tool and returns the parsed response document.
func requestRun(t *testing.T, args map[string]any) (bridgeDoc, *bridgeStub) {
	t.Helper()
	stub := bridgeStubAt(t, requestOKResponses)
	res := HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(args))
	return docOf(t, res), stub
}

func TestRequestCallsBridgeOnceWithBoundedTimeout(t *testing.T) {
	doc, stub := requestRun(t, map[string]any{"url": "https://example.com/"})

	// The whole point of the tool: one bridge "request", never a navigate/read
	// pair. A tab is not opened, so nothing renders and nothing is extracted.
	if got, want := strings.Join(stub.actions(), ","), "request"; got != want {
		t.Fatalf("bridge call sequence = %q, want %q", got, want)
	}

	call := stub.byAction("request")
	if call["url"] != "https://example.com/" {
		t.Errorf("request url = %v", call["url"])
	}
	if call["method"] != requestDefaultMethod {
		t.Errorf("request method = %v, want the %s default", call["method"], requestDefaultMethod)
	}
	// Unset optionals must not be sent, or the bridge would receive "null" bodies.
	if call["body"] != nil {
		t.Errorf("body must be omitted when not requested, got %v", call["body"])
	}
	if call["headers"] != nil {
		t.Errorf("headers must be omitted when not requested, got %v", call["headers"])
	}
	// The bridge round-trip carries its own bounded timeout instead of inheriting
	// the browser tool's 40-minute budget.
	if ms, ok := call["timeout"].(float64); !ok || ms <= 0 || ms > requestTimeoutMs {
		t.Errorf("timeout = %v, want a bounded positive ms", call["timeout"])
	}

	// Raw source, verbatim, no markdown conversion.
	if !strings.Contains(doc.body, "<!doctype html>") || !strings.Contains(doc.body, "<h1>Example Domain</h1>") {
		t.Errorf("body must be the raw HTML source, got %q", probeSnippet(doc.body))
	}
	if doc.terms["url"] != "https://example.com/" {
		t.Errorf("url = %v", doc.terms["url"])
	}
	if doc.terms["method"] != requestDefaultMethod {
		t.Errorf("method = %v", doc.terms["method"])
	}
	if doc.terms["status"] != "200" {
		t.Errorf("status = %v, want 200", doc.terms["status"])
	}
	if doc.terms["content-type"] != "text/html; charset=utf-8" {
		t.Errorf("content-type = %v", doc.terms["content-type"])
	}
	for _, absent := range []string{"truncated", "re-encoded", "body"} {
		if _, present := doc.terms[absent]; present {
			t.Errorf("%s term must be absent for a plain in-budget body, got %v", absent, doc.terms[absent])
		}
	}
}

func TestRequestRequiresURL(t *testing.T) {
	for _, args := range []map[string]any{
		{},
		{"url": ""},
		{"url": "   \t "},
	} {
		stub := bridgeStubAt(t, requestOKResponses)
		res := HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(args))
		if text := resultText(t, res); !strings.Contains(text, "url is required") {
			t.Errorf("args %v: expected missing-url error, got %q", args, text)
		}
		if len(stub.actions()) != 0 {
			t.Errorf("args %v: bridge must not be called, got %v", args, stub.actions())
		}
	}
}

func TestRequestNormalizesTargetURL(t *testing.T) {
	cases := []struct {
		name    string
		url     any
		wantURL string // "" => must be rejected before the bridge is touched
	}{
		{name: "scheme kept", url: "https://example.com/a?b=1", wantURL: "https://example.com/a?b=1"},
		{name: "query encoding untouched", url: "https://example.com/s?q=a%26b", wantURL: "https://example.com/s?q=a%26b"},
		{name: "bare host upgraded", url: "api.example.com/v1/items", wantURL: "https://api.example.com/v1/items"},
		{name: "localhost is http", url: "localhost:8080/health", wantURL: "http://localhost:8080/health"},
		{name: "lan ip is http", url: "192.168.1.20/api", wantURL: "http://192.168.1.20/api"},
		{name: "mDNS name is http", url: "printer.local/status", wantURL: "http://printer.local/status"},
		{name: "entities decoded", url: "example.com/s?a=1&amp;b=2", wantURL: "https://example.com/s?a=1&b=2"},
		{name: "file scheme rejected", url: "file:///etc/passwd", wantURL: ""},
		{name: "non-http scheme rejected", url: "ftp://example.com/file", wantURL: ""},
		{name: "hostless url rejected", url: "https://", wantURL: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := bridgeStubAt(t, requestOKResponses)
			HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(map[string]any{"url": tc.url}))

			call := stub.byAction("request")
			if tc.wantURL == "" {
				if call != nil {
					t.Fatalf("invalid url %v must abort before the bridge; got %v", tc.url, call)
				}
				return
			}
			if call == nil {
				t.Fatalf("request never called for %v", tc.url)
			}
			if call["url"] != tc.wantURL {
				t.Errorf("request url = %v, want %v", call["url"], tc.wantURL)
			}
		})
	}
}

func TestRequestMethodIsUppercasedAndForwarded(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{in: nil, want: requestDefaultMethod},
		{in: "", want: requestDefaultMethod},
		{in: "  post  ", want: "POST"},
		{in: "patch", want: "PATCH"},
		{in: "DELETE", want: "DELETE"},
		// Custom methods are legal tokens and reach the bridge untouched.
		{in: "report", want: "REPORT"},
	}
	for _, tc := range cases {
		stub := bridgeStubAt(t, requestOKResponses)
		args := map[string]any{"url": "https://example.com/"}
		if tc.in != nil {
			args["method"] = tc.in
		}
		HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(args))

		call := stub.byAction("request")
		if call == nil {
			t.Fatalf("method %v: request never called", tc.in)
		}
		if call["method"] != tc.want {
			t.Errorf("method %v -> %v, want %v", tc.in, call["method"], tc.want)
		}
	}
}

func TestRequestBodyReachesTheBridgeVerbatim(t *testing.T) {
	// The body is the payload the caller typed, so it must not be re-indented,
	// re-keyed, or JSON-round-tripped on the way to the bridge.
	body := `{"a":  1, "b":"x y","items":[1,2,3]}`
	stub := bridgeStubAt(t, map[string]string{
		"request": `{"success":true,"status":201,"statusText":"Created","headers":{"content-type":"application/json"},"data":"{\"ok\":true}"}`,
	})

	res := HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(map[string]any{
		"url":    "https://example.com/items",
		"method": "post",
		"body":   body,
	}))

	call := stub.byAction("request")
	if call["body"] != body {
		t.Errorf("body = %v, want the verbatim string %v", call["body"], body)
	}
	if call["method"] != "POST" {
		t.Errorf("method = %v, want POST", call["method"])
	}
	doc := docOf(t, res)
	wantVerbatim(t, doc.body, `{"ok":true}`)
	if doc.terms["status"] != "201" {
		t.Errorf("status = %v, want 201", doc.terms["status"])
	}
}

func TestRequestBodyBlankIsOmitted(t *testing.T) {
	// A blank CLI argument (--body "") is an absent body, not a zero-length one.
	for _, blank := range []any{"", "   ", "\n"} {
		stub := bridgeStubAt(t, requestOKResponses)
		HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(map[string]any{
			"url":  "https://example.com/",
			"body": blank,
		}))
		if call := stub.byAction("request"); call != nil && call["body"] != nil {
			t.Errorf("body %q must be omitted, got %v", blank, call["body"])
		}
	}
}

func TestRequestHeadersAcceptsEveryCallerShape(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want map[string]string
	}{
		{
			name: "json object",
			in:   map[string]any{"Authorization": "Bearer t0k", "Accept": "application/json"},
			want: map[string]string{"Authorization": "Bearer t0k", "Accept": "application/json"},
		},
		{
			name: "json string (CLI wrapper)",
			in:   `{"Authorization":"Bearer t0k","Accept":"application/json"}`,
			want: map[string]string{"Authorization": "Bearer t0k", "Accept": "application/json"},
		},
		{
			name: "header lines",
			in:   "Authorization: Bearer t0k\nAccept: application/json",
			want: map[string]string{"Authorization": "Bearer t0k", "Accept": "application/json"},
		},
		{
			name: "crlf lines with blank lines",
			in:   "Authorization: Bearer t0k\r\n\r\nAccept: application/json\r\n",
			want: map[string]string{"Authorization": "Bearer t0k", "Accept": "application/json"},
		},
		{
			name: "single line",
			in:   "Accept: application/json",
			want: map[string]string{"Accept": "application/json"},
		},
		{
			name: "array of lines",
			in:   []any{"Authorization: Bearer t0k", "Accept: application/json"},
			want: map[string]string{"Authorization": "Bearer t0k", "Accept": "application/json"},
		},
		{
			name: "numeric value from an MCP client",
			in:   map[string]any{"X-Count": float64(3)},
			want: map[string]string{"X-Count": "3"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := bridgeStubAt(t, requestOKResponses)
			HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(map[string]any{
				"url":     "https://example.com/",
				"headers": tc.in,
			}))

			call := stub.byAction("request")
			if call == nil {
				t.Fatalf("request never called: valid headers %v must succeed", tc.in)
			}
			switch headers := call["headers"].(type) {
			case map[string]any:
				if len(headers) != len(tc.want) {
					t.Fatalf("headers = %v, want %v", headers, tc.want)
				}
				for name, want := range tc.want {
					if headers[name] != want {
						t.Errorf("%s = %v, want %v", name, headers[name], want)
					}
				}
			default:
				t.Fatalf("headers reached the bridge as %T (%v), want an object", call["headers"], call["headers"])
			}
		})
	}
}

func TestRequestHeaderValuesAreNotRewritten(t *testing.T) {
	// Values that legitimately carry commas, colons and semicolons must survive:
	// splitting on "," would silently corrupt an Accept header.
	stub := bridgeStubAt(t, requestOKResponses)
	HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(map[string]any{
		"url":     "https://example.com/",
		"headers": "Accept: text/html,application/xhtml+xml;q=0.9\nContent-Type: text/plain; charset=utf-8\nX-Trace: a:b:c",
	}))

	call := stub.byAction("request")
	headers, ok := call["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers = %v, want an object", call["headers"])
	}
	if got, want := headers["Accept"], "text/html,application/xhtml+xml;q=0.9"; got != want {
		t.Errorf("Accept = %v, want %v", got, want)
	}
	if got, want := headers["Content-Type"], "text/plain; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %v, want %v", got, want)
	}
	if got, want := headers["X-Trace"], "a:b:c"; got != want {
		t.Errorf("X-Trace = %v, want %v (split on the first colon only)", got, want)
	}
}

func TestRequestRejectsHeaderInjection(t *testing.T) {
	cases := []struct {
		name  string
		in    any
		wants string
	}{
		{
			name:  "CRLF smuggled into a value",
			in:    map[string]any{"X-Evil": "1\r\nX-Admin: true"},
			wants: "single line",
		},
		{
			name:  "lone LF in a value",
			in:    map[string]any{"X-Evil": "1\nX-Admin: true"},
			wants: "single line",
		},
		{
			name:  "space in a header name",
			in:    map[string]any{"X Evil": "1"},
			wants: "invalid header name",
		},
		{
			name:  "colon in a header name",
			in:    map[string]any{"X:Evil": "1"},
			wants: "invalid header name",
		},
		{
			name:  "nested value",
			in:    map[string]any{"X-Evil": map[string]any{"a": "b"}},
			wants: "single string value",
		},
		{
			name:  "colonless line",
			in:    "just-some-text",
			wants: `must be "Name: value"`,
		},
		{
			name:  "brace-prefixed garbage",
			in:    "{not json",
			wants: "headers must be a JSON object",
		},
		{
			name:  "unsupported shape",
			in:    float64(42),
			wants: "headers must be a JSON object",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := bridgeStubAt(t, requestOKResponses)
			res := HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(map[string]any{
				"url":     "https://example.com/",
				"headers": tc.in,
			}))
			text := resultText(t, res)
			if !strings.Contains(text, tc.wants) {
				t.Errorf("expected %q in error, got %q", tc.wants, text)
			}
			if len(stub.actions()) != 0 {
				t.Errorf("malicious/invalid headers %v must abort before the bridge; got %v", tc.in, stub.actions())
			}
		})
	}
}

func TestRequestRejectsInvalidMethod(t *testing.T) {
	for _, bad := range []any{"GE T", "GET / HTTP/1.1", "G\r\nET", "GET\nX: 1", "🚀"} {
		stub := bridgeStubAt(t, requestOKResponses)
		res := HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(map[string]any{
			"url":    "https://example.com/",
			"method": bad,
		}))
		text := resultText(t, res)
		if !strings.Contains(text, "HTTP method token") {
			t.Errorf("method %v: expected token error, got %q", bad, text)
		}
		if len(stub.actions()) != 0 {
			t.Errorf("method %v must abort before the bridge; got %v", bad, stub.actions())
		}
	}
}

func TestRequestNonSuccessStatusStillReturnsSource(t *testing.T) {
	// Like curl: the body of a 404 is exactly the source that was requested, so it
	// is a successful tool call with the status surfaced in the provenance line.
	stub := bridgeStubAt(t, map[string]string{
		"request": `{"success":true,"status":404,"statusText":"Not Found","headers":{"content-type":"text/html; charset=utf-8"},"data":"<html><body>404</body></html>"}`,
	})

	doc := requestResult(t, stub, map[string]any{"url": "https://example.com/nope"})
	wantVerbatim(t, doc.body, "<html><body>404</body></html>")
	if doc.terms["status"] != "404" {
		t.Errorf("status = %v, want 404", doc.terms["status"])
	}
}

func TestRequestReportsBridgeLevelFailure(t *testing.T) {
	// Real bridge behaviour for an unreachable target: HTTP 200 with
	// success:false, status 0 and statusText "Network Error".
	stub := bridgeStubAt(t, map[string]string{
		"request": `{"success":false,"status":0,"statusText":"Network Error"}`,
	})

	res := HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(map[string]any{
		"url":    "https://127.0.0.1:1/",
		"method": "get",
	}))
	text := resultText(t, res)
	if !strings.Contains(text, "request GET https://127.0.0.1:1/ failed") {
		t.Errorf("expected a labelled request failure, got %q", text)
	}
	if !strings.Contains(text, "status 0 Network Error") {
		t.Errorf("bridge status line must be reported, got %q", text)
	}
	if len(stub.actions()) != 1 {
		t.Errorf("unexpected call sequence %v", stub.actions())
	}
}

func TestRequestReportsTransportFailureWithContext(t *testing.T) {
	stub := bridgeStubAt(t, map[string]string{"request": ""}, "request")
	res := HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(map[string]any{
		"url": "https://example.com/",
	}))
	text := resultText(t, res)
	if !strings.Contains(text, "request GET https://example.com/ failed") {
		t.Errorf("expected a labelled request failure, got %q", text)
	}
	if !strings.Contains(text, "bridge down") {
		t.Errorf("bridge detail must be reported, got %q", text)
	}
	if len(stub.actions()) != 1 {
		t.Errorf("unexpected call sequence %v", stub.actions())
	}
}

func TestRequestReportsFinalURLAfterRedirect(t *testing.T) {
	stub := bridgeStubAt(t, map[string]string{
		"request": `{"success":true,"url":"https://example.com/final","status":200,"statusText":"OK","headers":{"content-type":"text/plain"},"data":"moved"}`,
	})
	doc := requestResult(t, stub, map[string]any{"url": "https://example.com/starting"})
	if doc.terms["url"] != "https://example.com/final" {
		t.Errorf("url = %v, want the post-redirect URL", doc.terms["url"])
	}
}

func TestRequestParsedJSONBodyIsReEncodedAndFlagged(t *testing.T) {
	// The bridge auto-parses a JSON body into an object, which loses the original
	// whitespace. The tool must hand back the JSON and flag the re-encoding rather
	// than pretend the bytes are the ones the server sent.
	stub := bridgeStubAt(t, map[string]string{
		"request": `{"success":true,"status":200,"statusText":"OK","headers":{"content-type":"application/json"},"data":{"items":[1,2],"ok":true}}`,
	})

	doc := requestResult(t, stub, map[string]any{"url": "https://example.com/api/items"})
	var round map[string]any
	if err := json.Unmarshal([]byte(doc.body), &round); err != nil {
		t.Fatalf("body must be JSON, got %q: %v", probeSnippet(doc.body), err)
	}
	if round["ok"] != true || len(round["items"].([]any)) != 2 {
		t.Errorf("re-encoded body lost content: %v", round)
	}
	if strings.Contains(doc.body, "\n") {
		t.Errorf("re-encoded JSON must stay compact, got %q", probeSnippet(doc.body))
	}
	if _, present := doc.terms["re-encoded"]; !present {
		t.Error("a re-encoded body must be flagged in the provenance line")
	}
}

func TestRequestEmptyBodyIsNotAnError(t *testing.T) {
	// 204/304 and DELETE-style API answers are legitimately bodyless; the caller
	// still needs the status, so this is a success with an explicit term.
	for _, bodyless := range []string{
		`{"success":true,"status":204,"statusText":"No Content","headers":{},"data":""}`,
		`{"success":true,"status":204,"statusText":"No Content","headers":{}}`,
		`{"success":true,"status":204,"statusText":"No Content","headers":{},"data":null}`,
	} {
		stub := bridgeStubAt(t, map[string]string{"request": bodyless})
		doc := requestResult(t, stub, map[string]any{"url": "https://example.com/item/1"})
		if doc.body != "" {
			t.Errorf("%s: body = %q, want empty", bodyless, probeSnippet(doc.body))
		}
		if doc.terms["body"] != "empty" {
			t.Errorf("%s: body term = %v, want \"empty\"", bodyless, doc.terms["body"])
		}
		if doc.terms["status"] != "204" {
			t.Errorf("%s: status = %v, want 204", bodyless, doc.terms["status"])
		}
	}
}

func TestRequestBinaryBodyPassesThrough(t *testing.T) {
	// The bridge already replaces a binary body with a size placeholder; forwarding
	// it is more useful than an error, and it keeps the tool honest about content.
	stub := bridgeStubAt(t, map[string]string{
		"request": `{"success":true,"status":200,"statusText":"OK","headers":{"content-type":"image/x-icon"},"data":"[Binary Data: 5430 bytes]"}`,
	})
	doc := requestResult(t, stub, map[string]any{"url": "https://example.com/favicon.ico"})
	wantVerbatim(t, doc.body, "[Binary Data: 5430 bytes]")
	if doc.terms["content-type"] != "image/x-icon" {
		t.Errorf("content-type = %v", doc.terms["content-type"])
	}
}

func TestRequestTruncatesLongBody(t *testing.T) {
	huge := strings.Repeat("x", requestDefaultMaxChars+5000)
	stub := bridgeStubAt(t, map[string]string{
		"request": fmt.Sprintf(`{"success":true,"status":200,"statusText":"OK","headers":{"content-type":"text/plain"},"data":%q}`, huge),
	})

	doc := requestResult(t, stub, map[string]any{"url": "https://example.com/big.txt"})
	if len(doc.body) <= requestDefaultMaxChars {
		t.Fatalf("body of %d chars must be clamped to %d, got %d", len(huge), requestDefaultMaxChars, len(doc.body))
	}
	if !strings.Contains(doc.body, "TRUNCATED") {
		t.Errorf("truncation marker must stay visible in the body, got %q", probeSnippet(doc.body))
	}
	if doc.terms["truncated"] != "true" {
		t.Errorf("truncated flag = %v, want present", doc.terms["truncated"])
	}
}

func TestRequestParamValidation(t *testing.T) {
	cases := []struct {
		name  string
		args  map[string]any
		wants string
	}{
		{name: "max_chars below floor", args: map[string]any{"max_chars": float64(10)}, wants: "max_chars must be between"},
		{name: "max_chars above ceiling", args: map[string]any{"max_chars": float64(requestMaxChars + 1)}, wants: "max_chars must be between"},
		{name: "non numeric max_chars", args: map[string]any{"max_chars": "lots"}, wants: "max_chars must be a number"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := bridgeStubAt(t, requestOKResponses)
			args := map[string]any{"url": "https://example.com/"}
			for k, v := range tc.args {
				args[k] = v
			}
			res := HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(args))
			if call := stub.byAction("request"); call != nil {
				t.Fatalf("invalid args %v must abort before the bridge; got %v", tc.args, call)
			}
			if text := resultText(t, res); !strings.Contains(text, tc.wants) {
				t.Errorf("expected %q in error, got %q", tc.wants, text)
			}
		})
	}
}

func TestRequestMaxCharsAcceptsCLIString(t *testing.T) {
	stub := bridgeStubAt(t, requestOKResponses)
	doc := requestResult(t, stub, map[string]any{"url": "https://example.com/", "max_chars": "40000"})
	wantVerbatim(t, doc.body, requestRawSource)
}

func TestRequestProvenanceStaysOneLine(t *testing.T) {
	// A content-type carrying pipes or newlines must not split the provenance line,
	// which is the parsing contract every bridge-backed content tool shares.
	stub := bridgeStubAt(t, map[string]string{
		"request": `{"success":true,"status":200,"statusText":"OK","headers":{"content-type":"text/html | charset=utf-\r\n8"},"data":"ok"}`,
	})
	doc := requestResult(t, stub, map[string]any{"url": "https://example.com/"})
	if got, want := doc.terms["content-type"], "text/html / charset=utf- 8"; got != want {
		t.Errorf("content-type = %q, want the flattened %q", got, want)
	}
	if doc.body != "ok" {
		t.Errorf("body = %q", probeSnippet(doc.body))
	}
}

func TestRequestLongContentTypeIsBounded(t *testing.T) {
	long := "application/" + strings.Repeat("v", 400)
	stub := bridgeStubAt(t, map[string]string{
		"request": fmt.Sprintf(`{"success":true,"status":200,"statusText":"OK","headers":{"content-type":%q},"data":"ok"}`, long),
	})
	doc := requestResult(t, stub, map[string]any{"url": "https://example.com/"})
	term := doc.terms["content-type"]
	if len(term) > requestMaxTermChars+len("... ")+40 {
		t.Errorf("content-type term must be bounded, got %d chars", len(term))
	}
	if !strings.HasSuffix(term, "...") {
		t.Errorf("a clamped content-type must be marked, got %q", probeSnippet(term))
	}
}

func TestRequestOmitsUncapturedStatus(t *testing.T) {
	// status 0 without a failure flag means the bridge never saw the channel; that
	// is "unknown" and must not be served as a status code.
	stub := bridgeStubAt(t, map[string]string{
		"request": `{"success":true,"status":0,"statusText":"","headers":{},"data":"cached source"}`,
	})
	doc := requestResult(t, stub, map[string]any{"url": "https://example.com/"})
	if _, present := doc.terms["status"]; present {
		t.Errorf("status must be omitted when uncaptured, got %v", doc.terms["status"])
	}
	wantVerbatim(t, doc.body, "cached source")
}

func TestRequestReportsUncapturableBody(t *testing.T) {
	// A body over the bridge read cap is cut mid-JSON, so CallBridge hands back the
	// unparsable text instead of a report. That must read as "too large", never as
	// an empty successful body.
	stub := bridgeStubAt(t, map[string]string{
		"request": `{"success":true,"status":200,"data":"` + strings.Repeat("y", 4096),
	})

	res := HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(map[string]any{
		"url": "https://example.com/huge.json",
	}))
	text := resultText(t, res)
	if !strings.Contains(text, "could not be captured") {
		t.Errorf("expected an uncapturable-body error, got %q", probeSnippet(text))
	}
	if strings.Contains(text, "body: empty") {
		t.Errorf("an oversized body must not be reported as empty, got %q", probeSnippet(text))
	}
	if len(stub.actions()) != 1 {
		t.Errorf("unexpected call sequence %v", stub.actions())
	}
}

func TestRequestToolRegisteredInAllDefs(t *testing.T) {
	stub := bridgeStubAt(t, requestOKResponses)

	var found *ToolDef
	defs := AllDefs("/ws", Deps{})
	for i := range defs {
		if defs[i].Name == "request" {
			found = &defs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("request tool not registered in AllDefs")
	}
	if found.Handler == nil {
		t.Fatal("request tool has no handler")
	}
	if got, ok := found.Schema["required"].([]string); !ok || len(got) != 1 || got[0] != "url" {
		t.Errorf("request schema required = %v, want [url]", found.Schema["required"])
	}
	props, _ := found.Schema["properties"].(map[string]any)
	for _, name := range []string{"url", "method", "headers", "body", "max_chars"} {
		if _, ok := props[name]; !ok {
			t.Errorf("request schema missing %s property", name)
		}
	}
	// headers and body are deliberately untyped: the CLI wrapper can only send
	// strings, an MCP client sends objects, and both must validate.
	for _, name := range []string{"headers", "body"} {
		prop, _ := props[name].(map[string]any)
		if _, typed := prop["type"]; typed {
			t.Errorf("%s must stay untyped to accept both object and string shapes", name)
		}
	}

	res, err := found.Handler(context.Background(), requestReq(map[string]any{"url": "https://example.com/"}))
	if err != nil {
		t.Fatalf("handler returned go error: %v", err)
	}
	if doc := docOf(t, res); doc.body != requestRawSource {
		t.Errorf("registered handler body = %q", probeSnippet(doc.body))
	}
	if len(stub.actions()) != 1 {
		t.Errorf("registered handler sequence = %v", stub.actions())
	}
}

// requestResult runs the tool against an already-installed stub and pins that
// the request actually reached the bridge.
func requestResult(t *testing.T, stub *bridgeStub, args map[string]any) bridgeDoc {
	t.Helper()
	doc := docOf(t, HandleRequestAction(context.Background(), t.TempDir(), Deps{}, requestReq(args)))
	if stub.byAction("request") == nil {
		t.Fatalf("request never reached the bridge: %v", stub.actions())
	}
	return doc
}
