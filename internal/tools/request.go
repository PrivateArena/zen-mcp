package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// Raw HTTP request settings for the request tool. Unlike fetch, which renders
// the page before reading it, this tool drives the bridge's "request" action:
// one fetch() from inside the browser, so it carries the real browser TLS/JA3
// fingerprint and session (hence "stealth") but returns the response body as
// delivered — no DOM, no JavaScript, no Readability pass.
const (
	// requestTimeoutMs bounds the request inside the bridge. Kept well under the
	// tool timeout so an unreachable host fails fast instead of eating the whole
	// handler budget.
	requestTimeoutMs = 30000
	// requestDefaultMethod matches curl's default when --method is omitted.
	requestDefaultMethod = "GET"
	// requestDefaultMaxChars caps the returned body by default; requestMaxChars
	// is the ceiling an explicit max_chars may ask for. The bridge itself caps a
	// response read at 1 MiB (see bridge.CallBridge), so a bigger ceiling buys
	// nothing but context damage.
	requestDefaultMaxChars = 30000
	requestMinChars        = 1000
	requestMaxChars        = 100000
	// requestMaxTermChars bounds one provenance term, so a long content-type or
	// statusText can never bloat the leading line.
	requestMaxTermChars = 120
)

func defRequest(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "request",
		Title:       "HTTP Request (Firefox Bridge)",
		Description: "Send an HTTP request through the Firefox bridge (userChrome.js MCP Bridge) and return the raw response source (no rendering, no JavaScript) as token-efficient markdown (provenance line, then the body verbatim). Like curl, but from inside the browser, so it is not blocked by bot fingerprinting.",
		Schema: jsonSchema(map[string]any{
			"url":    strProp("URL to request; a bare host is upgraded to https:// (http:// for localhost and LAN IPs)"),
			"method": strProp(fmt.Sprintf("HTTP method, default %s", requestDefaultMethod)),
			// Untyped on purpose: MCP clients send a headers object, while the CLI
			// wrapper can only ever send a string (JSON object or "Name: value" lines).
			"headers":   map[string]any{"description": "Request headers: {\"Name\":\"value\"} object, the same as a JSON string, or newline-separated \"Name: value\" lines"},
			"body":      map[string]any{"description": "Request body, sent verbatim as a string (JSON/text payloads); omit for GET/HEAD"},
			"max_chars": numProp(fmt.Sprintf("Cap on returned body, %d-%d, default %d", requestMinChars, requestMaxChars, requestDefaultMaxChars)),
		}, []string{"url"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleRequestAction(ctx, workspace, deps, req), nil
		},
	}
}

// HandleRequestAction performs one bridge request and returns the response body
// verbatim under a provenance line. Non-2xx statuses are a successful call: the
// body of a 404 is still the source the caller asked for, exactly like curl.
func HandleRequestAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()

	targetURL, err := bridgeTargetURL("request", args["url"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "request", err, start)
	}
	method, err := requestMethodParam(args["method"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "request", err, start)
	}
	headers, err := requestHeadersParam(args["headers"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "request", err, start)
	}
	maxChars, err := requestMaxCharsParam(args["max_chars"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "request", err, start)
	}

	params := map[string]any{"url": targetURL, "method": method, "timeout": requestTimeoutMs}
	if len(headers) > 0 {
		params["headers"] = headers
	}
	if body := requestBodyParam(args["body"]); body != nil {
		params["body"] = body
	}

	res, err := bridge.CallBridge(ctx, "request", params)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "request", requestCallError(method, targetURL, err), start)
	}
	// The bridge answers unreachable hosts with HTTP 200 + success:false
	// (status 0, statusText "Network Error"), which must not be read as a body.
	if detail, failed := requestFailureDetail(res); failed {
		return toolresponse.WrapErrorWithContext(ctx, "request", fmt.Errorf("request %s %s failed: %s", method, targetURL, detail), start)
	}
	// A report with no data and no success field is a body CallBridge could not
	// decode, which is what happens when the response is bigger than its read cap.
	// Reporting "empty body" here would be a lie about a body that does exist.
	if detail, lost := requestUncapturedBody(res); lost {
		return toolresponse.WrapErrorWithContext(ctx, "request", fmt.Errorf("request %s %s failed: %s", method, targetURL, detail), start)
	}

	content, reEncoded := requestBodyText(res)
	content, truncated := clampBridgeContent(content, maxChars)

	doc := renderBridgeMarkdown(requestProvenance(method, targetURL, res, reEncoded, content, truncated), content)
	logSessionEvent(workspace, "request", fmt.Sprintf("request %s: %s", method, targetURL), content)
	logfilter.Debugf("[request] %s %s -> %d bytes", method, targetURL, len(content))
	return toolresponse.WrapSuccess(ctx, "request", doc, start)
}

// requestMethodParam normalizes the optional method argument. The result must be
// an RFC 9110 token: it is the only place where a caller-supplied string reaches
// the bridge verbatim as a request line, and a token cannot carry CRLF.
func requestMethodParam(v any) (string, error) {
	method := strings.ToUpper(strings.TrimSpace(searchStringArg(v)))
	if method == "" {
		return requestDefaultMethod, nil
	}
	if !isHTTPToken(method) {
		return "", fmt.Errorf("method must be an HTTP method token (letters, digits and !#$%%&'*+.^_`|-, got %q)", v)
	}
	return method, nil
}

// requestHeadersParam normalizes the optional headers argument into the string
// map the bridge expects. Every shape a caller can actually produce is accepted:
// a JSON object from an MCP client, a JSON string or "Name: value" lines from the
// CLI wrapper.
func requestHeadersParam(v any) (map[string]string, error) {
	switch h := v.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return requestHeaders(h)
	case []any:
		lines := make([]string, 0, len(h))
		for _, item := range h {
			switch item.(type) {
			case map[string]any, []any:
				return nil, fmt.Errorf("headers must be a JSON object or \"Name: value\" lines, got the nested value %v", item)
			}
			lines = append(lines, scalarHeaderValue(item))
		}
		return requestHeadersLines(lines)
	case string:
		text := strings.TrimSpace(h)
		if text == "" {
			return nil, nil
		}
		if strings.HasPrefix(text, "{") {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(text), &parsed); err != nil {
				return nil, fmt.Errorf("headers must be a JSON object or \"Name: value\" lines, got unparseable %q", h)
			}
			return requestHeaders(parsed)
		}
		return requestHeadersLines(strings.Split(text, "\n"))
	default:
		return nil, fmt.Errorf("headers must be a JSON object or \"Name: value\" lines, got %v", v)
	}
}

// requestHeaders validates the entries of a headers object.
func requestHeaders(in map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for name, value := range in {
		switch value.(type) {
		case map[string]any, []any:
			return nil, fmt.Errorf("header %q must be a single string value, got %v", name, value)
		case nil:
			continue
		}
		header, err := requestHeaderPair(name, scalarHeaderValue(value))
		if err != nil {
			return nil, err
		}
		out[header.name] = header.value
	}
	return out, nil
}

// requestHeadersLines parses "Name: value" lines, the shape a shell can pass in
// one argument. Lines that are blank are skipped; a colonless line is always a
// mistake by the caller, never a header, so it is rejected.
func requestHeadersLines(lines []string) (map[string]string, error) {
	out := map[string]string{}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		name, value, found := strings.Cut(line, ":")
		if !found {
			return nil, fmt.Errorf("header %q must be \"Name: value\"", line)
		}
		header, err := requestHeaderPair(name, value)
		if err != nil {
			return nil, err
		}
		out[header.name] = header.value
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

type headerPair struct {
	name  string
	value string
}

// requestHeaderPair validates one header. Names are restricted to HTTP tokens and
// values to printable field content, so no caller-supplied string can inject a
// second header or break the bridge's request.
func requestHeaderPair(name, value string) (headerPair, error) {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if !isHTTPToken(name) {
		return headerPair{}, fmt.Errorf("invalid header name %q: use a token without spaces or colons", name)
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return headerPair{}, fmt.Errorf("invalid header %q: value must be a single line", name)
	}
	return headerPair{name: name, value: value}, nil
}

// scalarHeaderValue renders a header value that is not already a string. JSON
// numbers arrive as float64 and the CLI sends strings; anything else is refused by
// the callers above.
func scalarHeaderValue(v any) string {
	switch tv := v.(type) {
	case string:
		return tv
	case nil:
		return ""
	default:
		return fmt.Sprint(tv)
	}
}

// isHTTPToken reports whether s is a non-empty RFC 9110 token, the character class
// shared by header field names and method names.
func isHTTPToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isHTTPTokenByte(s[i]) {
			return false
		}
	}
	return true
}

func isHTTPTokenByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		return true
	}
	return false
}

// requestBodyParam normalizes the optional body argument. A string reaches the
// bridge verbatim — that is the whole point of a curl-shaped tool — so JSON
// bodies keep their exact byte layout instead of being re-serialized.
func requestBodyParam(v any) any {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		return s
	}
	return v
}

// requestMaxCharsParam normalizes the optional max_chars argument.
func requestMaxCharsParam(v any) (int, error) {
	if v == nil {
		return requestDefaultMaxChars, nil
	}
	maxChars, ok := bridgeIntArg(v)
	if !ok {
		return 0, fmt.Errorf("max_chars must be a number between %d and %d, got %v", requestMinChars, requestMaxChars, v)
	}
	if maxChars < requestMinChars || maxChars > requestMaxChars {
		return 0, fmt.Errorf("max_chars must be between %d and %d, got %d", requestMinChars, requestMaxChars, maxChars)
	}
	return maxChars, nil
}

// requestBodyText pulls the response body out of a bridge request report. The
// bridge hands text back as a string but auto-parses JSON bodies into an object,
// so the second shape is re-encoded and reported as such — the original byte
// layout of a parsed body is not recoverable.
func requestBodyText(res map[string]any) (string, bool) {
	switch d := res["data"].(type) {
	case nil:
		return "", false
	case string:
		return d, false
	default:
		encoded, err := json.Marshal(d)
		if err != nil {
			return fmt.Sprint(d), true
		}
		return string(encoded), true
	}
}

// requestFailureDetail reports the failure detail of a bridge request that came
// back with success:false. That is how the bridge signals an unreachable host or a
// blocked request while still answering HTTP 200, so the status line is the only
// detail available.
func requestFailureDetail(res map[string]any) (string, bool) {
	if success, ok := res["success"].(bool); !ok || success {
		return "", false
	}
	if detail := strings.TrimSpace(searchStringArg(res["error"])); detail != "" {
		return detail, true
	}
	status, hasStatus := bridgeIntArg(res["status"])
	reason := strings.TrimSpace(searchStringArg(res["statusText"]))
	switch {
	case reason != "" && hasStatus:
		// status 0 means the request never completed, so it is reported alongside the
		// reason rather than treated as "absent": that pair is the useful detail.
		return fmt.Sprintf("status %d %s", status, requestReason(reason)), true
	case reason != "":
		return requestReason(reason), true
	case hasStatus:
		return fmt.Sprintf("status %d", status), true
	}
	return "bridge reported failure", true
}

// requestUncapturedBody detects the one response shape that carries no body at
// all: CallBridge hands back { raw: ... } when the bridge answer is not a JSON
// object, which for this tool means the response was cut off at its read cap.
// Any real bridge report has data or success, so a plain bodyless answer (204) is
// never mistaken for a lost payload.
func requestUncapturedBody(res map[string]any) (string, bool) {
	if _, ok := res["success"]; ok {
		return "", false
	}
	if _, ok := res["data"]; ok {
		return "", false
	}
	raw, ok := res["raw"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return "", false
	}
	return fmt.Sprintf("response could not be captured: it is larger than the bridge read limit or arrived unparseable (%d bytes seen)", len(raw)), true
}

// requestCallError reframes a failed bridge round-trip. The bridge answers its own
// errors with HTTP >= 400, whose body carries the real reason.
func requestCallError(method, targetURL string, err error) error {
	detail := err.Error()
	var fail *bridge.ErrBridgeFailure
	if errors.As(err, &fail) && strings.TrimSpace(fail.Body) != "" {
		detail = strings.TrimSpace(fail.Body)
	}
	return fmt.Errorf("request %s %s failed: %s", method, targetURL, detail)
}

// requestProvenance formats the metadata the caller needs to judge the payload:
// what was requested, what the server answered, and whether the body was
// re-encoded or clamped.
func requestProvenance(method, targetURL string, res map[string]any, reEncoded bool, content string, truncated bool) string {
	bodyTerm := ""
	if strings.TrimSpace(content) == "" {
		// A 204/304 or an empty API answer is legitimate; say so instead of
		// serving a document that looks like a lost payload.
		bodyTerm = bridgeTerm("body", "empty")
	}
	return bridgeProvenance(
		bridgeTerm("url", bridgeResponseString(res, "url", targetURL)),
		bridgeTerm("method", method),
		requestStatusTerm(res),
		bridgeTerm("content-type", requestContentType(res)),
		bridgeTerm("re-encoded", requestReEncodedTerm(reEncoded)),
		bodyTerm,
		truncatedTerm(truncated),
	)
}

// requestStatusTerm reports the response status when the bridge captured it; 0 or
// absent means "unknown", which is not worth a term in the payload.
func requestStatusTerm(res map[string]any) string {
	status, ok := bridgeIntArg(res["status"])
	if !ok || status <= 0 {
		return ""
	}
	return fmt.Sprintf("status: %d", status)
}

func requestReEncodedTerm(reEncoded bool) string {
	if !reEncoded {
		return ""
	}
	return "json (bridge parsed the body, original whitespace lost)"
}

// requestContentType extracts the response content type, the one response header
// that changes how a body must be read. Provenance is a single line, so the value
// is flattened and bounded.
func requestContentType(res map[string]any) string {
	headers, ok := res["headers"].(map[string]any)
	if !ok {
		return ""
	}
	for name, value := range headers {
		if !strings.EqualFold(strings.TrimSpace(name), "content-type") {
			continue
		}
		return requestReason(scalarHeaderValue(value))
	}
	return ""
}

// requestReason flattens a bridge-supplied reason so it can never break the
// single-line provenance contract or smuggle a control character into the term.
// Clamping is rune-safe so a UTF-8 value never ends in a broken rune.
func requestReason(s string) string {
	flat := strings.NewReplacer("\r", " ", "\n", " ", "\t", " ", "|", "/").Replace(s)
	// Field content is single-line whitespace-normalized text, so collapsing runs of
	// spaces is lossless and keeps the term parseable.
	flat = strings.Join(strings.Fields(flat), " ")
	if len(flat) <= requestMaxTermChars {
		return flat
	}
	cut := requestMaxTermChars
	for cut > 0 && !utf8.RuneStart(flat[cut]) {
		cut--
	}
	return flat[:cut] + "..."
}

// bridgeResponseString reads a top-level string of a bridge report, falling back
// when the bridge did not capture the field.
func bridgeResponseString(res map[string]any, key, fallback string) string {
	if v, ok := res[key].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
