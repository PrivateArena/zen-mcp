package tools

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

var CONTAINER_MAP = map[string]string{
	"github.com":  "Work",
	"slack.com":   "Work",
	"google.com":  "Personal",
	"youtube.com": "Personal",
	"localhost":   "Development",
}

func defBrowser(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "browser",
		Title:       "browser Controller (Firefox Bridge)",
		Description: "Control Firefox via userChrome.js MCP Bridge. Categories: Navigation & Tabs, Interaction, Extraction & State, Execution & Waiting, Vision & AI, Storage & Misc, Recording.",
		Schema: jsonSchema(map[string]any{
			"action": strEnumProp("Browser action. The list names the common ones; any bridge action is forwarded verbatim (wait_for_selector, get_text, is_visible, click, type, scroll, upload, close_tab, …).", []string{
				"list_containers", "active_tab", "restart", "refresh", "web_logs", "chrome_logs",
				"read", "navigate", "web_eval", "chrome_eval", "get_content", "chat",
				"screenshot", "request",
				"recorder_start", "recorder_status", "recorder_stop", "recorder_list",
				"recorder_body", "recorder_search", "recorder_clear",
			}),
			"url":             strProp("[nav/new_tab/cookie/search] URL"),
			"index":           strProp("[-i] TabRegistry id from navigate/active_tab (\"t_…\") to address that tab in any window, or a positional tab index; omit for the active tab"),
			"keep_tab":        boolProp("[read/search with url] Leave the opened tab open; by default it is closed once the content is extracted"),
			"container":       strProp("[navigate/new_tab/chat/cookie] Firefox container name or id that selects the identity; an explicit value always beats the per-domain default. Applied only when the call creates a tab"),
			"new_tab":         boolProp("[navigate/new_tab] Open in a new foreground (selected) tab"),
			"back_tab":        boolProp("[navigate/new_tab] Open in a new background tab; needs no focus and is addressable by the returned id"),
			"timeout":         intProp("[any action] This call's wait budget in ms; passed to the bridge untouched. Omit for the bridge's per-action default"),
			"text":            strProp("[type/trusted_paste] Text to enter"),
			"path":            strProp("[upload] Absolute path of the local file to attach; [screenshot] output PNG path (default: /tmp/firefox_bridge_ss_<ts>.png)"),
			"full_page":       boolProp("[screenshot] Capture the whole document (scrolls to the top for the shot, then restores the page position)"),
			"preload":         boolProp("[screenshot full_page] Walk the page first so lazy-loaded images decode (default true); set false to shoot immediately"),
			"scale":           numProp("[screenshot] Output pixels per CSS pixel. 1 = CSS size (default); the window's devicePixelRatio (~1.25-2) gives native display resolution"),
			"y":               numProp("[scroll] Pixel scroll offset"),
			"block":           strEnumProp("[scroll] Vertical alignment of a scrolled element.", []string{"start", "center", "end"}),
			"selector":        strProp("[locator] CSS selector, auto-scrolled into view"),
			"scope":           strEnumProp("[get_interactive_map] Rows to include: 'viewport' (default, cheap) or 'all' (whole document).", []string{"viewport", "all"}),
			"filter":          strProp("[get_interactive_map] Keep only rows whose action, selector or label contains this text"),
			"shortest":        boolProp("[get_interactive_map] Also report the shortest unique selector variant per row"),
			"limit":           intProp("[get_interactive_map] Max rows (default 80); [list_tabs] max tabs"),
			"by_text":         strProp("[locator] Partial case-insensitive text match"),
			"by_role":         strProp("[locator] ARIA role; narrow it with 'name'"),
			"name":            strProp("[locator] Accessible name, used with by_role"),
			"by_label":        strProp("[locator] Associated <label> text"),
			"by_name":         strProp("[locator] HTML name attribute"),
			"by_alt":          strProp("[locator] Image alt attribute"),
			"upload_files":    arrayStringProp("[chat] File path(s); string or array"),
			"code":            strProp("[web_eval/chrome_eval] JS; must \"return\""),
			"method":          strProp("[request] HTTP method"),
			"headers":         map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "[request] Headers dict"},
			"body":            map[string]any{"description": "[request] Body (string/object)"},
			"provider":        arrayStringProp("[chat] AI provider(s); string or array (default: claude)"),
			"message":         arrayStringProp("[chat](required) Message(s); string or array."),
			"take_screenshot": boolProp("[chat] Take screenshot of current tab and send to AI"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleBrowserAction(ctx, workspace, deps, req), nil
		},
	}
}

func HandleBrowserAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)
	if action == "" {
		return toolresponse.WrapErrorWithContext(ctx, "browser", fmt.Errorf("action is required"), start)
	}

	params := sanitizeBrowserParams(args)

	if action == "chat" {
		if _, ok := params["message"]; !ok {
			return toolresponse.WrapErrorWithContext(ctx, "browser", fmt.Errorf("Parameter 'message' is required for action 'chat'."), start)
		}
		// Same default as the chat tool, so `browser --action chat` and `zchat` agree
		// on who answers an unqualified question.
		if chatBlank(params["provider"]) {
			params["provider"] = chatDefaultProvider
		}
		if _, ok := params["upload_files"].([]any); ok {
			if _, ok2 := params["take_screenshot"].(bool); ok2 {
				return toolresponse.WrapErrorWithContext(ctx, "browser", fmt.Errorf("Both 'upload_files' and 'take_screenshot' cannot be used together in action 'chat'."), start)
			}
		}
	}

	if urlStr, ok := params["url"].(string); ok && urlStr != "" {
		if container, ok := containerForURL(urlStr); ok {
			if _, has := params["container"]; !has {
				params["container"] = container
			}
		}
	}

	bridgeParams := make(map[string]any, len(params))
	for k, v := range params {
		if k == "action" {
			continue
		}
		bridgeParams[k] = v
	}
	// The bridge reads `timeout` as the *action's own* wait budget (page load, frame
	// round-trip, network idle). Only the agent actions that genuinely run for minutes
	// may inherit the browser tool timeout — injecting it into every payload made a
	// single wait action wedge the bridge for the whole tool timeout, long after its
	// caller gave up. A caller-supplied timeout always wins; otherwise the bridge's own
	// per-action defaults apply.
	if _, ok := bridgeParams["timeout"]; !ok && isChatLike(action) {
		bridgeParams["timeout"] = mcpcfg.GetToolConfig("browser").Timeout
	}

	if uploadFiles, ok := bridgeParams["upload_files"]; ok {
		if resolved := resolveUploadFiles(uploadFiles, workspace); resolved != nil {
			bridgeParams["upload_files"] = resolved
		}
	}

	if action == "brainstorm" || action == "brainstorm_status" {
		if msg, ok := params["message"]; ok && msg != "" {
			if _, hasPrompt := bridgeParams["prompt"]; !hasPrompt {
				bridgeParams["prompt"] = msg
				delete(params, "message")
			}
		}
	}

	if action == "chat" {
		if uploadFiles, ok := bridgeParams["upload_files"]; ok && uploadFiles != nil {
			if _, hasPath := bridgeParams["path"]; !hasPath {
				bridgeParams["path"] = uploadFiles
				delete(bridgeParams, "upload_files")
			}
		}
		if screenshot, ok := params["take_screenshot"]; ok && screenshot != nil {
			if _, hasScreenshot := bridgeParams["screenshot"]; !hasScreenshot {
				bridgeParams["screenshot"] = screenshot
				delete(bridgeParams, "take_screenshot")
			}
		}
	}

	var res map[string]any
	var err error

	switch action {
	case "read":
		// read is tab-addressed: an `index` that is a TabRegistry id reads that tab
		// whatever is selected, and a bare url is opened in this call's own
		// background tab so the flow never touches another caller's page.
		targetURL, _ := bridgeParams["url"].(string)
		ctx, tabID, releaseTab, tabErr := browserFlowTab(ctx, "browser read "+browserTurnSubject(params), strings.TrimSpace(targetURL), params)
		if tabErr != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", tabErr, start)
		}
		defer releaseTab()

		res, err = bridge.CallBridge(ctx, "read", addressStep(tabStepParams(bridgeParams), tabID))
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		if err := verifyTab(res, tabID, "read"); err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "browser", extractData(res), start)

	case "search":
		searchQuery := bridgeParams["url"]
		if searchQuery == nil {
			searchQuery = params["query"]
		}
		searchQueryStr, _ := searchQuery.(string)
		if searchQueryStr == "" {
			return toolresponse.WrapErrorWithContext(ctx, "browser", fmt.Errorf("url (search query) or query is required for search"), start)
		}
		searchCount := 10
		if limit, ok := params["limit"].(float64); ok {
			searchCount = int(limit)
		}
		ctx, tabID, releaseTab, tabErr := browserFlowTab(ctx, "browser search "+searchQueryStr, searchQueryStr, params)
		if tabErr != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", tabErr, start)
		}
		defer releaseTab()

		if _, err := bridge.CallBridge(ctx, "wait_for_network_idle", addressStep(map[string]any{"idle_ms": 1000}, tabID)); err != nil {
			// ignore wait error
		}
		readParams := addressStep(tabStepParams(bridgeParams), tabID)
		readParams["limit"] = searchCount
		res, err = bridge.CallBridge(ctx, "read", readParams)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		if err := verifyTab(res, tabID, "search read"); err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "browser", extractData(res), start)

	case "get_interactive_map":
		// Forward the params: this is a frame action, so `index` addresses the tab.
		// Dropping them made the call always map the active tab and report someone
		// else's page under a concurrent caller's handle.
		res, err = bridge.CallBridge(ctx, "get_interactive_map", bridgeParams)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		// The frame renders the map itself: one line per control, `<n> <action> ["label"]
		// <selector>`, where the selector is ready to paste into click/type/select_option.
		// It goes out as plain text - wrapping a line-oriented table in JSON only adds
		// escaping (\t, \", \u2026) on top of the information the caller needs.
		if block, ok := res["data"].(map[string]any); ok && block != nil {
			if text, isText := block["map"].(string); isText && text != "" {
				if id, hasID := res["id"].(string); hasID && id != "" {
					text += "\n# tab " + id
					if urlStr, hasURL := res["url"].(string); hasURL && urlStr != "" {
						text += " \u00b7 " + urlStr
					}
				}
				return toolresponse.WrapSuccess(ctx, "browser", text, start)
			}
		}
		// Stale frame script (pre-selector-engine bridge): hand back whatever it sent.
		return toolresponse.WrapSuccess(ctx, "browser", extractData(res), start)

	case "restart":
		res, err = bridge.CallBridge(ctx, "reload", nil)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "browser", res, start)

	case "screenshot":
		// The bridge answers with geometry metadata next to the path. Forwarding it as a
		// plain string would hide which region and scale were actually captured, which is
		// exactly what a caller needs to know when a shot looks wrong.
		res, err = bridge.CallBridge(ctx, "screenshot", bridgeParams)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		out := map[string]any{}
		for _, k := range []string{"data", "path", "mode", "width", "height", "scale", "zoom", "rect", "scroll", "preScrollPasses", "viewport", "content", "id", "url"} {
			if v, ok := res[k]; ok && v != nil {
				out[k] = v
			}
		}
		if len(out) == 0 {
			out = res
		}
		return toolresponse.WrapSuccess(ctx, "browser", out, start)

	case "list_tabs":
		listParams := map[string]any{
			"query": params["query"],
			"limit": params["limit"],
		}
		res, err = bridge.CallBridge(ctx, "list_tabs", listParams)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		out := map[string]any{}
		if d, ok := res["data"]; ok && d != nil {
			out = res
		} else {
			out["tabs"] = res["data"]
			for k, v := range res {
				if k != "data" {
					out[k] = v
				}
			}
		}
		msg := ""
		if tabs, ok := out["tabs"].([]any); ok && len(tabs) == 9 && params["limit"] == nil {
			msg = "Showing window around active tab. Use 'query', 'limit', or 'offset' for more."
		}
		if msg != "" {
			out["message"] = msg
		}
		return toolresponse.WrapSuccess(ctx, "browser", out, start)
	}

	if action == "brainstorm" || action == "brainstorm_status" {
		if msg, ok := params["message"]; ok && msg != "" && bridgeParams["prompt"] == nil {
			bridgeParams["prompt"] = msg
			delete(bridgeParams, "message")
		}
	}

	if action == "chat" {
		if uploadFiles, ok := bridgeParams["upload_files"]; ok && uploadFiles != nil && bridgeParams["path"] == nil {
			bridgeParams["path"] = uploadFiles
			delete(bridgeParams, "upload_files")
		}
		if screenshot, ok := params["take_screenshot"]; ok && screenshot != nil && bridgeParams["screenshot"] == nil {
			bridgeParams["screenshot"] = screenshot
			delete(bridgeParams, "take_screenshot")
		}
	}

	// Large-response file handling for chat/brainstorm/brainstorm_status
	output := wrapBridgeOutput(ctx, action, bridgeParams, deps, workspace, start)
	return toolresponse.WrapSuccess(ctx, "browser", output, start)
}

func wrapBridgeOutput(ctx context.Context, action string, bridgeParams map[string]any, deps Deps, workspace string, start time.Time) any {
	res, err := bridge.CallBridge(ctx, action, bridgeParams)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
	}

	if isChatLike(action) {
		if success, ok := res["success"].(bool); ok && !success {
			errDetail := ""
			if e, ok := res["error"].(string); ok {
				errDetail = e
			} else {
				errDetail = fmt.Sprintf("%v", res)
			}
			return toolresponse.WrapErrorWithContext(ctx, "browser", fmt.Errorf("Bridge %s returned failure: %s", action, errDetail), start)
		}
	}

	data := res

	if action == "get_content" {
		if sel, ok := bridgeParams["selector"].(string); ok && sel != "" {
			if arr, ok := data["data"].([]any); ok {
				converted := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						converted = append(converted, s)
					}
				}
				data["markdown"] = converted
			} else if s, ok := data["data"].(string); ok {
				data["markdown"] = s
			}
		}
	}

	if action == "get_content" || action == "get_text" {
		maxLen := 30000
		if action == "get_text" {
			maxLen = 15000
		}
		if d, ok := data["data"].(string); ok && len(d) > maxLen {
			data["data"] = d[:maxLen] + fmt.Sprintf("... [TRUNCATED %d chars]", len(d)-maxLen)
		} else if arr, ok := data["data"].([]any); ok {
			for i, item := range arr {
				if s, ok := item.(string); ok && len(s) > maxLen {
					arr[i] = s[:maxLen] + fmt.Sprintf("... [TRUNCATED %d chars]", len(s)-maxLen)
				}
			}
			data["data"] = arr
		}
	}

	var output any
	if action == "web_eval" || action == "chrome_eval" || action == "request" {
		output = data
	} else if isChatLike(action) {
		if m, ok := data["response"]; ok && m != nil {
			output = m
		} else if data["data"] != nil {
			if dc, ok := data["data"].(map[string]any); ok {
				if c, ok := dc["content"].(string); ok && c != "" {
					output = c
				} else {
					output = dc
				}
			} else if c, ok := data["data"].(string); ok && c != "" {
				output = c
			} else {
				output = data["data"]
			}
		} else if data["content"] != nil {
			output = data["content"]
		} else {
			output = data
		}
	} else {
		if dc, ok := data["data"].(map[string]any); ok {
			if c, ok := dc["content"].(string); ok && c != "" {
				output = c
			} else {
				output = dc
			}
		} else if c, ok := data["content"].(string); ok && c != "" {
			output = c
		} else if d, ok := data["data"]; ok && d != nil {
			output = d
		} else {
			output = data
		}
	}

	if isChatLike(action) {
		// A long answer is spilled to the chat output dir instead of the context; the
		// chat tool shares that bound, see chatReplyDocument.
		if bounded, replaced := chatReplyDocument(action, chatPlainText(output), workspace, deps); replaced {
			output = bounded
		}
	}

	// Recorder replies are structured metadata, not prose, so they are not clamped as if they
	// were. Two bounds do apply: the one field that carries bytes is capped like any other
	// fetched document, and an index that outgrew the spill threshold goes to a file with a
	// preview. Without this, `recorder_body` would ride the unclamped passthrough and the
	// transport would silently corrupt it above 1 MiB.
	if strings.HasPrefix(action, "recorder_") {
		if m, ok := data["body"].(string); ok && m != "" {
			clamped, _ := clampBridgeContent(m, recorderBodyInlineChars)
			data["body"] = clamped
		}
		if bounded, replaced := recorderReplyDocument(action, chatPlainText(output), workspace, deps); replaced {
			output = bounded
		}
	}

	logSessionEvent(workspace, "browser", fmt.Sprintf("browser: %s", action), fmt.Sprintf("Params: %v\n\nOutput: %v", bridgeParams, output))
	logfilter.Info(output)
	return output
}

func isChatLike(action string) bool {
	switch action {
	case "chat", "brainstorm", "brainstorm_status":
		return true
	}
	return false
}

// recorderBodyInlineChars bounds one response body inside a recorder_body reply. It matches the
// bridge's own RECORDER_BODY_MAX_CHARS, so neither side is the binding constraint and a larger
// body is paged with offset instead of being silently cut.
const recorderBodyInlineChars = 100000

// resolveUploadFiles resolves relative file paths against workspaceRoot,
// preserving the caller's shape: a plain string stays a string, an array stays
// an array. Absolute paths pass through untouched. Returns nil only when the
// input is nil, so callers can detect "nothing to resolve".
func resolveUploadFiles(v any, workspaceRoot string) any {
	switch files := v.(type) {
	case []string:
		out := make([]string, 0, len(files))
		for _, f := range files {
			out = append(out, resolveUploadPath(f, workspaceRoot))
		}
		return out
	case []any:
		out := make([]any, 0, len(files))
		for _, f := range files {
			if s, ok := f.(string); ok {
				out = append(out, resolveUploadPath(s, workspaceRoot))
			}
		}
		return out
	case string:
		return resolveUploadPath(files, workspaceRoot)
	}
	return v
}

func resolveUploadPath(f, workspaceRoot string) string {
	if filepath.IsAbs(f) {
		return f
	}
	return filepath.Join(workspaceRoot, f)
}

func sanitizeBrowserParams(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		switch k {
		case "message", "prompt":
			out[k] = bridge.DecodeHTMLEntities(bridge.FixMojibake(fmt.Sprint(v)))
		case "by_name":
			out["byName"] = v
			out["name"] = v
		case "by_label":
			out["byLabel"] = v
		case "by_alt":
			out["byAltText"] = v
		case "by_text":
			out["byText"] = v
		case "by_role":
			out["byRole"] = v
		default:
			out[k] = v
		}
	}
	// `by_name` doubles as `name` (the bridge reads either). That alias must never
	// shadow an explicit `name`, which is how a byRole+name locator is spelled —
	// map iteration order would otherwise decide which one wins.
	if v, ok := args["name"]; ok {
		out["name"] = v
	}
	return out
}

// containerForURL maps a URL's hostname onto CONTAINER_MAP so bridge traffic for
// a known site lands in the same Firefox container the browser tool would use.
func containerForURL(urlStr string) (string, bool) {
	u, err := parseURL(urlStr)
	if err != nil {
		return "", false
	}
	domain := u.Hostname()
	for pattern, container := range CONTAINER_MAP {
		if strings.Contains(domain, pattern) {
			return container, true
		}
	}
	return "", false
}

// bridgeIntArg coerces a tool argument to an int. JSON numbers arrive as
// float64 while CLI wrappers may send strings, so every numeric shape the
// bridge-facing tools can receive is accepted; ok is false for absent or
// non-numeric values, letting each tool report its own range error.
func bridgeIntArg(v any) (int, bool) {
	switch n := v.(type) {
	case nil:
		return 0, false
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func parseURL(s string) (*url.URL, error) {
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	return url.Parse(s)
}

// browserTurnSubject names what a reserved browser sequence is working on, so a
// queued bridge caller can see in the busy error which action owns the browser.
// browserFlowTab resolves the tab a read/search flow works on and reserves it for
// the whole sequence:
//
//   - an index that is a TabRegistry id addresses that tab, and a url given with it
//     navigates that same tab in place;
//   - a url with no id opens this call's own background tab, which is closed again
//     on release unless keep_tab is set;
//   - neither means the active tab, which is not reservable and stays on the global
//     queue.
//
// The returned context carries the tab reservation, so the follow-up calls of the
// flow reuse it instead of queueing again.
func browserFlowTab(ctx context.Context, label, targetURL string, params map[string]any) (context.Context, string, func(), error) {
	noop := func() {}
	container, _ := params["container"].(string)

	if tabID := bridge.RegistryID(params["index"]); tabID != "" {
		if targetURL != "" {
			navParams := withTab(map[string]any{"url": targetURL}, tabID)
			if container != "" {
				navParams["container"] = container
			}
			nav, err := bridge.CallBridge(ctx, "navigate", navParams)
			if err != nil {
				return ctx, tabID, noop, err
			}
			if detail, failed := bridgeError(nav); failed {
				return ctx, tabID, noop, fmt.Errorf("navigate to %s failed: %s", targetURL, detail)
			}
		}
		turnCtx, release, err := bridge.WithTabTurn(ctx, tabID, label)
		if err != nil {
			return ctx, tabID, noop, err
		}
		return turnCtx, tabID, release, nil
	}

	if targetURL == "" {
		return ctx, "", noop, nil
	}

	// timeout 0 leaves the page-load budget at the bridge's own default.
	nav, err := bridge.CallBridge(ctx, "navigate", navigateParamsTab(targetURL, 0, container))
	if err != nil {
		return ctx, "", noop, err
	}
	if detail, failed := bridgeError(nav); failed {
		return ctx, "", noop, fmt.Errorf("navigate to %s failed: %s", targetURL, detail)
	}
	tabID, err := requireTabID(nav, targetURL)
	if err != nil {
		return ctx, "", noop, err
	}
	turnCtx, releaseTab, err := bridge.WithTabTurn(ctx, tabID, label)
	if err != nil {
		closeTabFlow(ctx, tabID)
		return ctx, tabID, noop, err
	}
	if keepTabArg(params["keep_tab"]) {
		return turnCtx, tabID, releaseTab, nil
	}
	return turnCtx, tabID, func() {
		releaseTab()
		closeTabFlow(turnCtx, tabID)
	}, nil
}

func browserTurnSubject(params map[string]any) string {
	for _, key := range []string{"url", "query", "selector"} {
		if s, ok := params[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func extractData(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if data, ok := m["data"].(map[string]any); ok {
		if content, ok := data["content"].(string); ok && content != "" {
			return content
		}
		return data
	}
	if content, ok := m["content"].(string); ok && content != "" {
		return content
	}
	return m
}
