package tools

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// Google SERP extraction settings for the search tool. The "AI Overview" block
// is rendered asynchronously inside the aimc subtree, so the flow is tab-addressed
// like fetch: navigate (own background tab) -> wait for the container -> extract
// text, every step carrying the TabRegistry id of that tab.
const (
	// googleSearchEndpoint is the plain Google web search endpoint.
	googleSearchEndpoint = "https://www.google.com/search"
	// aiOverviewSelector targets the AI Overview answer column of the SERP.
	// aiOverviewSelector = `div[data-subtree="aimc"][decode-data-ved="1"] div[data-container-id="main-col"]`
	aiOverviewSelector = `div[data-container-id="main-col"]`
	// searchDefaultNum reproduces the ?num= page-size hint of the SERP request.
	searchDefaultNum = 50
	searchMinNum     = 50
	searchMaxNum     = 50
	// searchNavigateTimeoutMs bounds the SERP load inside the bridge.
	searchNavigateTimeoutMs = 30000
	// searchOverviewWaitMs bounds the first wait for the async AI Overview container.
	// Bounded well under the tool timeout so a query without an AI Overview fails
	// fast instead of stalling the whole handler budget.
	searchOverviewWaitMs = 10000
	// searchOverviewRetryWaitMs is the second, shorter wait, for a SERP whose AI
	// Overview renders late because its tab is in the background.
	searchOverviewRetryWaitMs = 8000
	// searchOverviewIdleMs is the mutation-quiet period required before extracting,
	// so a still-streaming overview is not read half-written.
	searchOverviewIdleMs = 800
	// searchMaxContentChars caps the answer text echoed back to the model.
	searchMaxContentChars = 20000
)

func defSearch(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "search",
		Title:       "Web Search (Google AI Overview)",
		Description: "Search Google through the Firefox bridge (userChrome.js MCP Bridge) and return the AI Overview answer as token-efficient markdown (provenance line, then the answer text).",
		Schema: jsonSchema(map[string]any{
			"query":    strProp("Search query"),
			"num":      numProp("[query] Google page size hint (num=), 1-100, default 10"),
			"keep_tab": boolProp("Leave the result tab open; by default it is closed once the answer is extracted"),
		}, []string{"query"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSearchAction(ctx, workspace, deps, req), nil
		},
	}
}

// HandleSearchAction navigates to the Google SERP for the query and returns the
// AI Overview text of the result page.
func HandleSearchAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()

	rawQuery := searchStringArg(args["query"])
	query := strings.TrimSpace(bridge.DecodeHTMLEntities(bridge.FixMojibake(rawQuery)))
	if query == "" {
		return toolresponse.WrapErrorWithContext(ctx, "search", fmt.Errorf("query is required"), start)
	}

	num, err := searchNumParam(args["num"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "search", err, start)
	}

	searchURL := googleSearchURL(query, num)

	// Tab-addressed flow, the same shape as fetch: this call's own background tab,
	// its TabRegistry id on every step, so the extraction cannot land on another
	// caller's SERP and the sequence needs no reservation of the whole browser
	// (see internal/bridge/address.go).
	container := ""
	if c, ok := containerForURL(searchURL); ok {
		container = c
	}
	nav, err := bridge.CallBridge(ctx, "navigate", navigateParamsTab(searchURL, searchNavigateTimeoutMs, container))
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "search", err, start)
	}
	if detail, failed := bridgeError(nav); failed {
		return toolresponse.WrapErrorWithContext(ctx, "search", fmt.Errorf("navigate to the SERP for %q failed: %s", query, detail), start)
	}
	tabID, err := requireTabID(nav, searchURL)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "search", err, start)
	}

	ctx, releaseTab, err := bridge.WithTabTurn(ctx, tabID, "search "+query)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "search", err, start)
	}
	defer releaseTab()
	if !keepTabArg(args["keep_tab"]) {
		defer closeTabFlow(ctx, tabID)
	}

	if _, err := bridge.CallBridge(ctx, "wait_for_network_idle", withTab(map[string]any{"idle_ms": 1000}, tabID)); err != nil {
		logfilter.Debugf("[search] network idle wait skipped: %v", err)
	}

	overview, err := searchOverview(ctx, tabID)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "search", searchMissError(query, searchURL, err), start)
	}

	content, truncated := clampSearchContent(overview)
	if strings.TrimSpace(content) == "" {
		return toolresponse.WrapErrorWithContext(ctx, "search", fmt.Errorf("AI Overview for %q is empty (%s)", query, searchURL), start)
	}

	// The SERP URL is fully derivable from the query (and is quoted in every
	// error), so the provenance line carries the query rather than repeating a
	// long encoded URL in every successful response.
	doc := renderBridgeMarkdown(bridgeProvenance(bridgeTerm("query", query), truncatedTerm(truncated)), content)
	logSessionEvent(workspace, "search", fmt.Sprintf("search: %s", query), content)
	logfilter.Info(content)
	return toolresponse.WrapSuccess(ctx, "search", doc, start)
}

// searchOverview waits for the AI Overview container on the addressed tab and
// extracts its text. The block streams in after the SERP paints, and a background
// tab is not painted at all, so a first pass is followed by one retry with a fresh
// wait before the query is called answerless. A failing wait is never fatal —
// extraction is what decides.
func searchOverview(ctx context.Context, tabID string) (string, error) {
	var lastErr error = fmt.Errorf("no AI Overview container found")
	for _, waitMs := range []int{searchOverviewWaitMs, searchOverviewRetryWaitMs} {
		if _, err := bridge.CallBridge(ctx, "wait_for_selector", withTab(map[string]any{
			"selector": aiOverviewSelector,
			"timeout":  waitMs,
			"deep":     true,
			"idle_ms":  searchOverviewIdleMs,
		}, tabID)); err != nil {
			logfilter.Debugf("[search] AI Overview wait skipped: %v", err)
		}

		res, err := bridge.CallBridge(ctx, "get_text", withTab(map[string]any{"selector": aiOverviewSelector}, tabID))
		if err != nil {
			lastErr = err
			continue
		}
		if detail, failed := bridgeError(res); failed {
			lastErr = fmt.Errorf("%s", detail)
			continue
		}
		if err := verifyTab(res, tabID, "AI Overview"); err != nil {
			return "", err
		}
		if content := aiOverviewContent(res); strings.TrimSpace(content) != "" {
			return content, nil
		}
		lastErr = fmt.Errorf("AI Overview container is empty")
	}
	return "", lastErr
}

// searchStringArg coerces an argument to a string; non-string scalars (a bare
// number query from a CLI wrapper) are stringified, absent ones are "".
func searchStringArg(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	default:
		return fmt.Sprint(s)
	}
}

// googleSearchURL builds the SERP request URL, e.g.
// https://www.google.com/search?num=10&q=how+to+use+hyperframes
func googleSearchURL(query string, num int) string {
	values := url.Values{}
	values.Set("q", query)
	values.Set("udm", strconv.Itoa(num))
	return googleSearchEndpoint + "?" + values.Encode()
}

// searchNumParam normalizes the optional num argument.
func searchNumParam(v any) (int, error) {
	if v == nil {
		return searchDefaultNum, nil
	}
	num, ok := bridgeIntArg(v)
	if !ok {
		return 0, fmt.Errorf("num must be a number between %d and %d, got %v", searchMinNum, searchMaxNum, v)
	}
	if num < searchMinNum || num > searchMaxNum {
		return 0, fmt.Errorf("num must be between %d and %d, got %d", searchMinNum, searchMaxNum, num)
	}
	return num, nil
}

// aiOverviewContent pulls the extracted text out of a bridge get_text response,
// which may carry a single element, a list of elements, or a bare string.
func aiOverviewContent(res map[string]any) string {
	switch d := res["data"].(type) {
	case map[string]any:
		if c, ok := d["content"].(string); ok {
			return c
		}
	case string:
		return d
	case []any:
		parts := make([]string, 0, len(d))
		for _, item := range d {
			switch e := item.(type) {
			case string:
				if strings.TrimSpace(e) != "" {
					parts = append(parts, e)
				}
			case map[string]any:
				if c, ok := e["content"].(string); ok && strings.TrimSpace(c) != "" {
					parts = append(parts, c)
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	}
	if c, ok := res["content"].(string); ok {
		return c
	}
	return ""
}

// searchMissError reframes a failed extraction. The bridge answers an unmatched
// selector with HTTP 500 + "Not found", which for this tool really means "this
// SERP has no AI Overview", so that case gets an explicit message instead of a
// raw transport error.
func searchMissError(query, searchURL string, err error) error {
	detail := err.Error()
	var fail *bridge.ErrBridgeFailure
	if errors.As(err, &fail) && strings.TrimSpace(fail.Body) != "" {
		detail = fail.Body
	}
	if !strings.Contains(detail, "Not found") {
		return err
	}
	return fmt.Errorf("no AI Overview for %q (%s): %s", query, searchURL, strings.TrimSpace(detail))
}

// bridgeError reports the failure detail of a bridge response that came back
// with success:false (HTTP 200 with an error body is how the bridge signals a
// "not found" style miss).
func bridgeError(res map[string]any) (string, bool) {
	if success, ok := res["success"].(bool); !ok || success {
		return "", false
	}
	detail := strings.TrimSpace(fmt.Sprint(res["error"]))
	if detail == "" || detail == "<nil>" {
		detail = "bridge reported failure"
	}
	return detail, true
}

// clampSearchContent bounds the extracted overview so a huge AI answer can never
// flood the model context.
func clampSearchContent(content string) (string, bool) {
	return clampBridgeContent(content, searchMaxContentChars)
}
