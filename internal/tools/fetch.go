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

// Page fetch settings for the fetch tool. The flow is tab-addressed: navigate
// (own background tab) -> wait_for_network_idle -> read, every step carrying the
// TabRegistry id of that tab, which is what makes the extraction land on the
// freshly loaded page instead of whatever tab was already active.
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
	// fetchSettleQuietMs is the DOM-quiet period the extraction waits for inside the page
	// after the network goes quiet. A component fetches its data, then renders it, and the
	// render is the part the extractor needs - 400 ms of silence costs one round trip on a
	// settled page and saves a whole missing table on an unhydrated one.
	fetchSettleQuietMs = 400
	// fetchDefaultMaxChars caps the returned markdown by default; fetchMaxChars is
	// the ceiling an explicit max_chars may ask for.
	fetchDefaultMaxChars = 30000
	fetchMinChars        = 1000
	fetchMaxChars        = 100000
	// fetchOutlineMaxChars bounds the section map returned by outline=true, which is a
	// table of contents and must never cost more than the page it indexes.
	fetchOutlineMaxChars = 4000
	// fetchSectionCutFloor and fetchSectionCutRatio bound where a section-boundary cut may
	// land: stopping at a heading 1 KB into a 30 KB allowance would throw away the payload
	// to save a marker, so only edges in the tail of the window are considered.
	fetchSectionCutFloor = 512
	fetchSectionCutRatio = 2
)

func defFetch(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "fetch",
		Title:       "Fetch Page (Firefox Bridge)",
		Description: "Fetch a URL through the Firefox bridge (userChrome.js MCP Bridge) and return the page's main content as token-efficient markdown: one provenance line reporting how the payload was extracted, then the content. Web components are read through their shadow roots, so component-rendered tables survive. Address a long page with outline, section and start_index rather than re-reading it.",
		Schema: jsonSchema(map[string]any{
			"url":         strProp("URL to read; a bare host is upgraded to https:// (http:// for localhost and LAN IPs)"),
			"selector":    strProp("[read] CSS selector to scope the extraction to one element; omit for whole-page article markdown"),
			"idle_ms":     numProp(fmt.Sprintf("[wait_for_network_idle] quiet period before reading, %d-%d, default %d", fetchMinIdleMs, fetchMaxIdleMs, fetchDefaultIdleMs)),
			"max_chars":   numProp(fmt.Sprintf("Cap on returned markdown, %d-%d, default %d", fetchMinChars, fetchMaxChars, fetchDefaultMaxChars)),
			"outline":     boolProp("Return only the section map (heading, level, byte offset) instead of the body, to pick what to fetch next"),
			"section":     strProp("Return only this section, matched against heading text (case-insensitive, substring ok); max_chars and start_index then apply to the section"),
			"start_index": numProp("Byte offset to resume from, taken from the outline or the next_start provenance term of a previous fetch"),
			"keep_tab":    boolProp("Leave the page tab open; by default it is closed once the content is extracted"),
		}, []string{"url"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleFetchAction(ctx, workspace, deps, req), nil
		},
	}
}

// HandleFetchAction loads the URL in a background tab of its own, waits for the network
// to go quiet and then for the DOM to stop changing, and returns the page's main content
// as markdown: the landmark that holds the answer, Readability's pick if the page has no
// landmark, the whole body as a last resort - which the provenance line says out loud.
// With a selector, the extraction is scoped to that element instead. The tab is closed on
// the way out, and the payload is then addressed: one section, or the byte window a
// previous call said it stopped at.
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
	section := strings.TrimSpace(searchStringArg(args["section"]))
	startIndex, err := fetchStartIndexParam(args["start_index"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", err, start)
	}
	// bridgeBool, not a type assert: the generated CLI wrapper posts every value as a
	// string, so --outline true has to read as true.
	outlineOnly, _ := bridgeBool(args["outline"])

	// Tab-addressed flow: navigate opens a background tab of its own and reports its
	// TabRegistry id, and every later step carries that id. The sequence therefore
	// never depends on which tab is selected, and concurrent flows cannot read each
	// other's pages, so it no longer reserves the whole bridge (address.go).
	container := ""
	if c, ok := containerForURL(targetURL); ok {
		container = c
	}
	nav, err := bridge.CallBridge(ctx, "navigate", navigateParamsTab(targetURL, fetchNavigateTimeoutMs, container))
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", err, start)
	}
	// navigate answers HTTP 200 with success:false for its own failures (e.g.
	// navigate_timeout), which must not be mistaken for a loaded page.
	if detail, failed := bridgeError(nav); failed {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", fmt.Errorf("navigate to %s failed: %s", targetURL, detail), start)
	}
	tabID, err := requireTabID(nav, targetURL)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", err, start)
	}

	// One reservation on this one tab: a second caller holding the same id waits
	// instead of driving the page underneath, while other tabs run in parallel.
	ctx, releaseTab, err := bridge.WithTabTurn(ctx, tabID, "fetch "+targetURL)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", err, start)
	}
	defer releaseTab()
	if !keepTabArg(args["keep_tab"]) {
		defer closeTabFlow(ctx, tabID)
	}

	// Best-effort: late XHR-driven content may still be in flight. A quiet network
	// is a bonus, not a requirement, so a failing wait must not lose the page.
	if _, err := bridge.CallBridge(ctx, "wait_for_network_idle", withTab(map[string]any{
		"idle_ms": idleMs,
		"timeout": fetchIdleTimeoutMs,
	}, tabID)); err != nil {
		logfilter.Debugf("[fetch] network idle wait skipped: %v", err)
	}

	readParams := withTab(map[string]any{}, tabID)
	if selector != "" {
		readParams["selector"] = selector
	}
	readParams["settle_ms"] = fetchSettleQuietMs
	res, err := bridge.CallBridge(ctx, "read", readParams)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", fetchReadError(targetURL, selector, err), start)
	}
	if detail, failed := bridgeError(res); failed {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", fmt.Errorf("read %s failed: %s", fetchSubject(targetURL, selector), detail), start)
	}
	if err := verifyTab(res, tabID, fetchSubject(targetURL, selector)); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", err, start)
	}

	full := fetchPageContent(res)
	if strings.TrimSpace(full) == "" {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", fmt.Errorf("%s has no readable content", fetchSubject(targetURL, selector)), start)
	}

	// Everything from here is a function of the extracted markdown alone, so the section
	// map, the slice and the window are all addressable with offsets the tool itself
	// reported: a caller pages through a long page without reading it twice.
	page, matched := bridgeSectionSlice(full, section)
	if section != "" && !matched {
		return toolresponse.WrapErrorWithContext(ctx, "fetch", fmt.Errorf("no section %q on %s; sections: %s",
			section, fetchSubject(targetURL, selector), strings.Join(bridgeHeadingTitles(full), " | ")), start)
	}
	if startIndex > 0 {
		if startIndex >= len(page) {
			return toolresponse.WrapErrorWithContext(ctx, "fetch", fmt.Errorf("start_index %d is past the end of %s (%d chars)", startIndex, fetchSubject(targetURL, selector), len(page)), start)
		}
		page = page[startIndex:]
	}

	var content string
	var truncated bool
	var nextStart int
	if outlineOnly {
		var clipped bool
		content, clipped = clampBridgeContent(bridgeOutline(page, startIndex), fetchOutlineMaxChars)
		truncated = clipped
	} else {
		var omitted []bridgeHeading
		content, omitted, truncated = clampBridgeAtSection(page, maxChars)
		if truncated {
			if len(omitted) > 0 {
				nextStart = startIndex + omitted[0].start
			}
			content += bridgeTruncationMarker(len(page)-len(content), omitted, startIndex, nextStart)
		}
	}

	finalURL := fetchNavString(nav, "url", targetURL)
	title := fetchNavString(nav, "title", "")
	// The bridge only reports a status when it managed to capture the channel;
	// 0/absent means "unknown", which is not worth a term in the provenance line.
	status := fetchNavInt(nav, "status")

	doc := renderBridgeMarkdown(fetchProvenance(finalURL, title, status, selector, truncated,
		fetchResponseTerms(res, section, len(full), nextStart)...), content)
	logSessionEvent(workspace, "fetch", fmt.Sprintf("fetch: %s", finalURL), content)
	logfilter.Info(content)
	return toolresponse.WrapSuccess(ctx, "fetch", doc, start)
}

// fetchProvenance formats the metadata the caller needs to judge the payload:
// the final URL (redirects are invisible otherwise), the page title, the HTTP
// status when captured, the selector the extraction was scoped to, and whether
// the page was clamped.
func fetchProvenance(finalURL, title string, status int, selector string, truncated bool, extra ...string) string {
	statusTerm := ""
	if status > 0 {
		statusTerm = fmt.Sprintf("status: %d", status)
	}
	terms := []string{
		bridgeTerm("url", finalURL),
		bridgeTerm("title", title),
		statusTerm,
		bridgeTerm("selector", selector),
		truncatedTerm(truncated),
	}
	return bridgeProvenance(append(terms, extra...)...)
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

// fetchStartIndexParam normalizes the optional start_index argument. A negative offset
// is a caller mistake worth reporting, since it would otherwise silently restart the page.
func fetchStartIndexParam(v any) (int, error) {
	if v == nil {
		return 0, nil
	}
	index, ok := bridgeIntArg(v)
	if !ok {
		return 0, fmt.Errorf("start_index must be a number of bytes, got %v", v)
	}
	if index < 0 {
		return 0, fmt.Errorf("start_index must not be negative, got %d", index)
	}
	return index, nil
}

// fetchPageContent pulls the markdown out of a bridge read response, which
// carries it as { data: { content } }.
func fetchPageContent(res map[string]any) string {
	if content, ok := extractData(res).(string); ok {
		return content
	}
	return ""
}

// fetchReadReport returns the extraction metadata the bridge reports beside the
// content (which selector-free path produced it, how many shadow roots it had to open,
// what it failed to hydrate), or nil for an older bridge.
func fetchReadReport(res map[string]any) map[string]any {
	if data, ok := res["data"].(map[string]any); ok {
		return data
	}
	return nil
}

// bridgeReportTerms turns that report into provenance terms, dropping the ones that
// cost bytes and say nothing (a whole-body dump is reported, an empty shadow count is
// not). extracted: body is the term that tells the caller the page defeated the article
// detection, which is exactly the case that used to fail silently.
func bridgeReportTerms(report map[string]any) []string {
	if report == nil {
		return nil
	}
	terms := []string{}
	if method, ok := report["extracted"].(string); ok && method != "" {
		terms = append(terms, bridgeTerm("extracted", method))
	}
	for _, key := range []struct{ field, label string }{
		{"shadow_roots", "shadow"},
		{"unhydrated", "unhydrated"},
	} {
		if n, ok := bridgeIntArg(report[key.field]); ok && n > 0 {
			terms = append(terms, bridgeTerm(key.label, fmt.Sprint(n)))
		}
	}
	if settled, ok := report["settled"].(bool); ok && !settled {
		// The page was still mutating when it was read: content may be missing, and a
		// larger idle_ms is the remedy. Reported rather than waited away in silence.
		terms = append(terms, bridgeTerm("settled", "false"))
	}
	if fail, ok := report["extract_error"].(string); ok && strings.TrimSpace(fail) != "" {
		terms = append(terms, bridgeTerm("extract_error", fail))
	}
	return terms
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

// fetchResponseTerms are the provenance terms this call adds to the bridge's own report:
// which slice of the page is being returned, how big the page really is, and the offset
// that continues it. Together they turn a clamped payload into a resumable one.
func fetchResponseTerms(res map[string]any, section string, totalChars, nextStart int) []string {
	terms := bridgeReportTerms(fetchReadReport(res))
	if section != "" {
		terms = append(terms, bridgeTerm("section", section))
	}
	if nextStart > 0 {
		terms = append(terms,
			bridgeTerm("size", fmt.Sprint(totalChars)),
			bridgeTerm("next_start", fmt.Sprint(nextStart)))
	}
	return terms
}

// bridgeHeading is one ATX heading of the extracted markdown and the byte offset it
// starts at. The offset is what makes a long page addressable: it is echoed by the
// outline and by next_start, and start_index consumes it.
type bridgeHeading struct {
	level int
	title string
	start int
}

// bridgeHeadings indexes the section structure of extracted markdown. Lines inside a
// fenced code block are not headings, and a page of docs is full of them.
func bridgeHeadings(md string) []bridgeHeading {
	var heads []bridgeHeading
	var start int
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			start += len(line) + 1
			continue
		}
		if !inFence {
			if level, title, ok := atxHeading(trimmed); ok {
				heads = append(heads, bridgeHeading{level: level, title: title, start: start})
			}
		}
		start += len(line) + 1
	}
	return heads
}

// atxHeading parses one "## Title" line, requiring the space so that "#hashtag" and a
// "---" rule are not mistaken for structure.
func atxHeading(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level == len(line) || line[level] != ' ' {
		return 0, "", false
	}
	title := strings.TrimSpace(line[level:])
	// A heading whose whole content is emphasis markers carries no addressable name.
	title = strings.Trim(title, "_* ")
	if title == "" {
		return 0, "", false
	}
	return level, title, true
}

// bridgeHeadingTitles names the sections of a page, for the error message that replaces
// a section name the page does not have.
func bridgeHeadingTitles(md string) []string {
	heads := bridgeHeadings(md)
	titles := make([]string, 0, len(heads))
	for _, h := range heads {
		titles = append(titles, h.title)
	}
	return titles
}

// bridgeSectionSlice narrows the page to one section: the heading that matches, up to
// the next heading of the same or a higher level. Matching is case- and punctuation-
// tolerant because a caller copies a heading name out of an outline, not out of the DOM.
func bridgeSectionSlice(md, want string) (string, bool) {
	if want == "" {
		return md, true
	}
	normalized := normalizeHeadingTitle(want)
	heads := bridgeHeadings(md)
	for i, h := range heads {
		title := normalizeHeadingTitle(h.title)
		exact := title == normalized || strings.Contains(title, normalized) || strings.Contains(normalized, title)
		if !exact {
			continue
		}
		end := len(md)
		for _, later := range heads[i+1:] {
			if later.level <= h.level {
				end = later.start
				break
			}
		}
		return md[h.start:end], true
	}
	return md, false
}

func normalizeHeadingTitle(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	var kept strings.Builder
	for _, r := range lower {
		if r == '-' || r == '_' || r == ':' || r == '/' || r == '(' || r == ')' {
			r = ' '
		}
		if r == ' ' || r == '\t' || r == '\n' {
			if kept.Len() > 0 && !strings.HasSuffix(kept.String(), " ") {
				kept.WriteRune(' ')
			}
			continue
		}
		kept.WriteRune(r)
	}
	return strings.Trim(kept.String(), " ")
}

// bridgeOutline renders the section map of a page, with the offsets a caller passes back
// as start_index. Numbering is by offset into the page as the caller last saw it, so the
// offsets of a whole-page outline stay valid across calls for the same URL.
func bridgeOutline(md string, baseOffset int) string {
	heads := bridgeHeadings(md)
	if len(heads) == 0 {
		return "[no sections: the page has no headings]"
	}
	var out strings.Builder
	fmt.Fprintf(&out, "[section map: %d sections, %d chars; fetch --section <name> or --start_index <offset>]\n", len(heads), len(md))
	for _, h := range heads {
		fmt.Fprintf(&out, "%s%s (%d)\n", strings.Repeat("  ", h.level-1), h.title, baseOffset+h.start)
	}
	return out.String()
}

// clampBridgeAtSection clamps the payload at a section boundary when one falls inside
// the budget, and returns the headings that fell outside it. Cutting a page mid-paragraph
// while a clean section edge sat 200 bytes back is the failure mode of a byte cap.
func clampBridgeAtSection(content string, maxChars int) (string, []bridgeHeading, bool) {
	if len(content) <= maxChars {
		return content, nil, false
	}
	headings := bridgeHeadings(content)
	floor := maxChars * fetchSectionCutRatio / 3
	if floor < fetchSectionCutFloor {
		floor = fetchSectionCutFloor
	}
	cut := maxChars
	for _, h := range headings {
		if h.start < cut && h.start >= floor {
			cut = h.start
		}
	}
	kept := strings.ToValidUTF8(content[:cut], "")
	var omitted []bridgeHeading
	for _, h := range headings {
		if h.start >= cut {
			omitted = append(omitted, h)
		}
	}
	return kept, omitted, true
}

// bridgeTruncationMarker states what was lost, and how to get it back, instead of
// leaving a bare "TRUNCATED" behind. The section list doubles as the map of what is
// missing, and its offsets are absolute in the page as the caller received it, so they
// are directly reusable as start_index. A page with no headings at all gets the plain
// marker, because a mid-paragraph cut has no resume point to advertise.
func bridgeTruncationMarker(omittedChars int, omitted []bridgeHeading, baseOffset, nextStart int) string {
	if len(omitted) == 0 {
		return fmt.Sprintf("\n... [TRUNCATED %d chars]", omittedChars)
	}
	var out strings.Builder
	fmt.Fprintf(&out, "\n... [TRUNCATED %d chars]\n[%d sections omitted; continue with --start_index %d]", omittedChars, len(omitted), nextStart)
	for _, h := range omitted {
		if out.Len() > fetchOutlineMaxChars {
			fmt.Fprintf(&out, "\n  ... (%d more sections; fetch --outline for the whole map)", len(omitted))
			break
		}
		fmt.Fprintf(&out, "\n%s%s (%d)", strings.Repeat("  ", h.level-1), h.title, baseOffset+h.start)
	}
	return out.String()
}
