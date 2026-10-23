package tools

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// Page fetch settings for the fetch tool. The flow mirrors the browser tool's
// read action: navigate (new tab, foreground) -> wait_for_network_idle -> read,
// which is what makes the extraction land on the freshly loaded page instead of
// whatever tab was already active.
const (
	// fetchNavigateTimeoutMs bounds the page load inside the bridge. Kept well
	// under the tool timeout so a hanging site fails fast instead of eating the
	// whole handler budget.
	fetchNavigateTimeoutMs = 30000
	// fetchDefaultIdleMs is the network-quiet period required before reading.
	fetchDefaultIdleMs = 1000
	fetchMinIdleMs     = 0
	fetchMaxIdleMs     = 15000
	// fetchIdleTimeoutMs bounds the wait_for_network_idle call itself. The wait is
	// best-effort, so it must never outlive the page-load budget by much.
	fetchIdleTimeoutMs = 20000
	// fetchDefaultMaxChars caps the returned markdown by default; fetchMaxChars is
	// the ceiling an explicit max_chars may ask for.
	fetchDefaultMaxChars = 30000
	fetchMinChars        = 1000
	fetchMaxChars        = 100000
)

func defFetch(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "fetch",
		Title:       "Fetch Page (Firefox Bridge)",
		Description: "Fetch a URL through the Firefox bridge (userChrome.js MCP Bridge) and return the page content as token-efficient markdown (provenance line, then the page markdown verbatim).",
		Schema: jsonSchema(map[string]any{
			"url":       strProp("URL to read; a bare host is upgraded to https:// (http:// for localhost and LAN IPs)"),
			"selector":  strProp("[read] CSS selector to scope the extraction to one element; omit for whole-page article markdown"),
			"idle_ms":   numProp(fmt.Sprintf("[wait_for_network_idle] quiet period before reading, %d-%d, default %d", fetchMinIdleMs, fetchMaxIdleMs, fetchDefaultIdleMs)),
			"max_chars": numProp(fmt.Sprintf("Cap on returned markdown, %d-%d, default %d", fetchMinChars, fetchMaxChars, fetchDefaultMaxChars)),
		}, []string{"url"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleFetchAction(ctx, workspace, deps, req), nil
		},
	}
}

// HandleFetchAction loads the URL in a new foreground tab, waits for the
// network to go quiet, then returns the Readability markdown of the page (or of
// the element matched by selector).
func HandleFetchAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()

	targetURL, err := fetchTargetURL(args["url"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", err, start)
	}
	selector := strings.TrimSpace(searchStringArg(args["selector"]))
	idleMs, err := fetchIdleMsParam(args["idle_ms"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", err, start)
	}
	maxChars, err := fetchMaxCharsParam(args["max_chars"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", err, start)
	}

	navigateParams := map[string]any{"url": targetURL, "new_tab": true, "timeout": fetchNavigateTimeoutMs}
	if container, ok := containerForURL(targetURL); ok {
		navigateParams["container"] = container
	}
	nav, err := bridge.CallBridge(ctx, "navigate", navigateParams)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", err, start)
	}
	// navigate answers HTTP 200 with success:false for its own failures (e.g.
	// navigate_timeout), which must not be mistaken for a loaded page.
	if detail, failed := bridgeError(nav); failed {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", fmt.Errorf("navigate to %s failed: %s", targetURL, detail), start)
	}

	// Best-effort: late XHR-driven content may still be in flight. A quiet network
	// is a bonus, not a requirement, so a failing wait must not lose the page.
	if _, err := bridge.CallBridge(ctx, "wait_for_network_idle", map[string]any{
		"idle_ms": idleMs,
		"timeout": fetchIdleTimeoutMs,
	}); err != nil {
		logfilter.Debugf("[fetch] network idle wait skipped: %v", err)
	}

	readParams := map[string]any{}
	if selector != "" {
		readParams["selector"] = selector
	}
	res, err := bridge.CallBridge(ctx, "read", readParams)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", fetchReadError(targetURL, selector, err), start)
	}
	if detail, failed := bridgeError(res); failed {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", fmt.Errorf("read %s failed: %s", fetchSubject(targetURL, selector), detail), start)
	}

	content, truncated := clampBridgeContent(fetchPageContent(res), maxChars)
	if strings.TrimSpace(content) == "" {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", fmt.Errorf("%s has no readable content", fetchSubject(targetURL, selector)), start)
	}

	finalURL := fetchNavString(nav, "url", targetURL)
	title := fetchNavString(nav, "title", "")
	// The bridge only reports a status when it managed to capture the channel;
	// 0/absent means "unknown", which is not worth a term in the provenance line.
	status := fetchNavInt(nav, "status")

	doc := renderBridgeMarkdown(fetchProvenance(finalURL, title, status, selector, truncated), content)
	logSessionEvent(workspace, "fetch", fmt.Sprintf("fetch: %s", finalURL), content)
	logfilter.Info(content)
	return toolresponse.WrapSuccess(ctx, "fetch", doc, start)
}

// fetchProvenance formats the metadata the caller needs to judge the payload:
// the final URL (redirects are invisible otherwise), the page title, the HTTP
// status when captured, the selector the extraction was scoped to, and whether
// the page was clamped.
func fetchProvenance(finalURL, title string, status int, selector string, truncated bool) string {
	statusTerm := ""
	if status > 0 {
		statusTerm = fmt.Sprintf("status: %d", status)
	}
	return bridgeProvenance(
		bridgeTerm("url", finalURL),
		bridgeTerm("title", title),
		statusTerm,
		bridgeTerm("selector", selector),
		truncatedTerm(truncated),
	)
}

// bridgeTerm assembles one "label: value" provenance term and returns "" when
// there is no value to report, so absent metadata costs nothing.
func bridgeTerm(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return label + ": " + value
}

// bridgeProvenance joins the metadata terms of a markdown response into the one
// leading line that renderBridgeMarkdown places above the payload. Empty terms
// are dropped, so optional metadata costs nothing when absent.
func bridgeProvenance(terms ...string) string {
	kept := make([]string, 0, len(terms))
	for _, term := range terms {
		if strings.TrimSpace(term) != "" {
			kept = append(kept, term)
		}
	}
	return strings.Join(kept, " | ")
}

// truncatedTerm flags a clamped payload at the top of the document, where a
// caller skimming a huge page still sees it.
func truncatedTerm(truncated bool) string {
	if !truncated {
		return ""
	}
	return "truncated"
}

// fetchTargetURL normalizes the url argument of the fetch tool; see
// bridgeTargetURL for the normalization rules shared by the bridge HTTP tools.
func fetchTargetURL(v any) (string, error) {
	return bridgeTargetURL("fetch", v)
}

// bridgeTargetURL normalizes the url argument shared by the bridge-backed HTTP
// tools (fetch, request): entities/mojibake are decoded, a missing scheme is
// added, and anything that is not a loadable http(s) host is rejected before the
// bridge is touched. tool names the caller in the error text.
func bridgeTargetURL(tool string, v any) (string, error) {
	raw := strings.TrimSpace(bridge.DecodeHTMLEntities(bridge.FixMojibake(searchStringArg(v))))
	if raw == "" {
		return "", fmt.Errorf("url is required")
	}
	// A scheme-less target is upgraded here rather than inside url.Parse, so the
	// scheme check below always sees the real scheme of the caller's input. An
	// input that already carries one is passed through verbatim, so exotic but
	// valid query strings are never re-encoded on the way to the bridge.
	target := raw
	if !strings.Contains(target, "://") {
		target = fetchDefaultSchemeURL(target)
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("invalid url %q: %v", raw, err)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("invalid url %q: no host", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported url scheme %q in %q, %s only speaks http(s)", u.Scheme, raw, tool)
	}
	return target, nil
}

// fetchDefaultSchemeURL adds a scheme to a bare host. Loopback, LAN and mDNS
// hosts are served over plain http far more often than https, so guessing https
// there would only produce a certificate error.
func fetchDefaultSchemeURL(raw string) string {
	authority := raw
	if i := strings.Index(authority, "/"); i >= 0 {
		authority = authority[:i]
	}
	hostname := authority
	if h, _, err := net.SplitHostPort(authority); err == nil {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") ||
		strings.HasSuffix(hostname, ".local") || net.ParseIP(hostname) != nil {
		return "http://" + raw
	}
	return "https://" + raw
}

// fetchIdleMsParam normalizes the optional idle_ms argument.
func fetchIdleMsParam(v any) (int, error) {
	if v == nil {
		return fetchDefaultIdleMs, nil
	}
	idle, ok := bridgeIntArg(v)
	if !ok {
		return 0, fmt.Errorf("idle_ms must be a number between %d and %d, got %v", fetchMinIdleMs, fetchMaxIdleMs, v)
	}
	if idle < fetchMinIdleMs || idle > fetchMaxIdleMs {
		return 0, fmt.Errorf("idle_ms must be between %d and %d, got %d", fetchMinIdleMs, fetchMaxIdleMs, idle)
	}
	return idle, nil
}

// fetchMaxCharsParam normalizes the optional max_chars argument.
func fetchMaxCharsParam(v any) (int, error) {
	if v == nil {
		return fetchDefaultMaxChars, nil
	}
	maxChars, ok := bridgeIntArg(v)
	if !ok {
		return 0, fmt.Errorf("max_chars must be a number between %d and %d, got %v", fetchMinChars, fetchMaxChars, v)
	}
	if maxChars < fetchMinChars || maxChars > fetchMaxChars {
		return 0, fmt.Errorf("max_chars must be between %d and %d, got %d", fetchMinChars, fetchMaxChars, maxChars)
	}
	return maxChars, nil
}

// fetchPageContent pulls the markdown out of a bridge read response, which
// carries it as { data: { content } }.
func fetchPageContent(res map[string]any) string {
	if content, ok := extractData(res).(string); ok {
		return content
	}
	return ""
}

// fetchReadError reframes a failed read. The bridge answers an unmatched
// selector with HTTP 500 + "Not found", which for this tool means "that
// selector is not on the page", so it gets an explicit message instead of a raw
// transport error.
func fetchReadError(targetURL, selector string, err error) error {
	detail := err.Error()
	var fail *bridge.ErrBridgeFailure
	if errors.As(err, &fail) && strings.TrimSpace(fail.Body) != "" {
		detail = strings.TrimSpace(fail.Body)
	}
	if !strings.Contains(detail, "Not found") {
		return fmt.Errorf("read %s failed: %s", fetchSubject(targetURL, selector), detail)
	}
	return fmt.Errorf("%s not found on %s: %s", selector, targetURL, detail)
}

// fetchSubject names what was being read, for error messages that survive a
// round-trip through the model context.
func fetchSubject(targetURL, selector string) string {
	if selector != "" {
		return fmt.Sprintf("%q on %s", selector, targetURL)
	}
	return targetURL
}

// fetchNavString / fetchNavInt read the navigation report (final URL after
// redirects, page title, HTTP status) that the bridge returns alongside success.
func fetchNavString(nav map[string]any, key, fallback string) string {
	if v, ok := nav[key].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func fetchNavInt(nav map[string]any, key string) int {
	if v, ok := bridgeIntArg(nav[key]); ok {
		return v
	}
	return 0
}

// renderBridgeMarkdown assembles the response of the bridge-backed content
// tools: one provenance line, a blank line, then the extracted markdown
// verbatim. Handing back markdown instead of a JSON envelope avoids escaping
// every newline and quote of a multi-kilobyte page, which is pure token
// overhead for the caller. Truncation stays visible because clampBridgeContent
// appends its marker to the content itself.
func renderBridgeMarkdown(provenance, content string) string {
	return provenance + "\n\n" + strings.TrimSpace(content) + "\n"
}

// clampBridgeContent bounds extracted page text so a huge page can never flood
// the model context. Truncation is rune-safe.
func clampBridgeContent(content string, maxChars int) (string, bool) {
	if len(content) <= maxChars {
		return content, false
	}
	cut := strings.ToValidUTF8(content[:maxChars], "")
	return cut + fmt.Sprintf("\n... [TRUNCATED %d chars]", len(content)-maxChars), true
}
