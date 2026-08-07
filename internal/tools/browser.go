package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/shared"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

var currentContainer string

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
			"action": strEnumProp("Browser action.", []string{
				"list_containers", "active_tab", "restart", "refresh", "web_logs", "chrome_logs",
				"read", "navigate", "web_eval", "chrome_eval", "get_content", "chat",
				"screenshot", "request",
			}),
			"url":             strProp("[nav/new_tab/cookie/search] URL"),
			"upload_files":    arrayStringProp("[chat] File path(s); string or array"),
			"code":            strProp("[web_eval/chrome_eval] JS; must \"return\""),
			"method":          strProp("[request] HTTP method"),
			"headers":         map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "[request] Headers dict"},
			"body":            map[string]any{"description": "[request] Body (string/object)"},
			"provider":        arrayStringProp("[chat] AI provider(s); string or array"),
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
		if _, ok := params["upload_files"].([]any); ok {
			if _, ok2 := params["take_screenshot"].(bool); ok2 {
				return toolresponse.WrapErrorWithContext(ctx, "browser", fmt.Errorf("Both 'upload_files' and 'take_screenshot' cannot be used together in action 'chat'."), start)
			}
		}
	}

	if urlStr, ok := params["url"].(string); ok && urlStr != "" {
		u, err := parseURL(urlStr)
		if err == nil {
			domain := u.Hostname()
			for pattern, container := range CONTAINER_MAP {
				if strings.Contains(domain, pattern) {
					if _, ok := params["container"]; !ok {
						params["container"] = container
					}
					break
				}
			}
		}
	}
	if c, ok := params["container"].(string); ok && c != "" {
		currentContainer = c
	}

	bridgeParams := make(map[string]any, len(params))
	for k, v := range params {
		if k == "action" {
			continue
		}
		bridgeParams[k] = v
	}
	bridgeParams["timeout"] = mcpcfg.GetToolConfig("browser").Timeout

	if uploadFiles, ok := bridgeParams["upload_files"]; ok {
		workspaceRoot := workspace
		var files []string
		switch v := uploadFiles.(type) {
		case []string:
			files = v
		case []any:
			files = make([]string, 0, len(v))
			for _, f := range v {
				if s, ok := f.(string); ok {
					files = append(files, s)
				}
			}
		case string:
			files = []string{v}
		}

		if len(files) > 0 {
			resolved := make([]string, 0, len(files))
			for _, f := range files {
				if filepath.IsAbs(f) {
					resolved = append(resolved, f)
				} else {
					resolved = append(resolved, filepath.Join(workspaceRoot, f))
				}
			}
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
		if uploadFiles, ok := params["upload_files"]; ok && uploadFiles != nil {
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
	case "start_recording":
		recSessionId := ""
		if sid, ok := params["session_id"].(string); ok && sid != "" {
			recSessionId = sid
		} else {
			recSessionId = fmt.Sprintf("rec_%d", time.Now().UnixMilli())
		}
		body := map[string]any{
			"enabled":    true,
			"domains":    params["domains"],
			"types":      params["types"],
			"session_id": recSessionId,
		}
		res, err = postJSON(ctx, fmt.Sprintf("http://%s:%d/api/recorder/toggle", mcpcfg.Get().Host, mcpcfg.Get().ProxyPort), body)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		if code, ok := res["statusCode"].(float64); ok && code >= 400 {
			return toolresponse.WrapErrorWithContext(ctx, "browser", fmt.Errorf("Recorder failed: %v", res), start)
		}
		return toolresponse.WrapSuccess(ctx, "browser", map[string]any{"message": "Recording started", "session_id": recSessionId}, start)

	case "stop_recording":
		body := map[string]any{"enabled": false}
		res, err = postJSON(ctx, fmt.Sprintf("http://%s:%d/api/recorder/toggle", mcpcfg.Get().Host, mcpcfg.Get().ProxyPort), body)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		if code, ok := res["statusCode"].(float64); ok && code >= 400 {
			return toolresponse.WrapErrorWithContext(ctx, "browser", fmt.Errorf("Recorder failed: %v", res), start)
		}
		return toolresponse.WrapSuccess(ctx, "browser", map[string]any{"message": "Recording stopped"}, start)

	case "read":
		if urlStr, ok := bridgeParams["url"].(string); ok && urlStr != "" {
			if _, err := bridge.CallBridge(ctx, "navigate", map[string]any{"url": urlStr, "new_tab": true}); err != nil {
				return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
			}
		}
		res, err = bridge.CallBridge(ctx, "read", bridgeParams)
		if err != nil {
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
		if _, err := bridge.CallBridge(ctx, "navigate", map[string]any{"url": searchQueryStr, "new_tab": true}); err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		if _, err := bridge.CallBridge(ctx, "wait_for_network_idle", map[string]any{"idle_ms": 1000}); err != nil {
			// ignore wait error
		}
		readParams := make(map[string]any, len(bridgeParams))
		for k, v := range bridgeParams {
			readParams[k] = v
		}
		readParams["limit"] = searchCount
		res, err = bridge.CallBridge(ctx, "read", readParams)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "browser", extractData(res), start)

	case "get_interactive_map":
		res, err = bridge.CallBridge(ctx, "get_interactive_map", nil)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		dataList := res
		var items []map[string]any
		if d, ok := dataList["data"].([]map[string]any); ok {
			items = d
		} else if arr, ok := dataList["data"].([]any); ok {
			for _, it := range arr {
				if m, ok := it.(map[string]any); ok {
					items = append(items, m)
				}
			}
		}
		lines := make([]string, 0, len(items))
		for _, el := range items {
			id := ""
			if v, ok := el["id"]; ok {
				id = fmt.Sprintf("%v", v)
			}
			tag := ""
			if v, ok := el["tag"].(string); ok {
				tag = v
			}
			typ := ""
			if v, ok := el["type"].(string); ok && v != "" {
				typ = fmt.Sprintf(` type="%s"`, v)
			}
			text := ""
			if v, ok := el["text"].(string); ok {
				text = v
			}
			role := ""
			if v, ok := el["role"].(string); ok && v != "" {
				role = fmt.Sprintf(" (role: %s)", v)
			}
			aria := ""
			if v, ok := el["ariaLabel"].(string); ok && v != "" {
				aria = fmt.Sprintf(" (aria: %s)", v)
			}
			lines = append(lines, fmt.Sprintf(`- [ID %s] <%s%s> "%s"%s%s`, id, tag, typ, text, role, aria))
		}
		return toolresponse.WrapSuccess(ctx, "browser", map[string]any{
			"message":         "Interactive map extracted.",
			"interactive_map": res,
			"markdown_map":    strings.Join(lines, "\n"),
		}, start)

	case "restart":
		res, err = bridge.CallBridge(ctx, "reload", nil)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "browser", res, start)

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
		if uploadFiles, ok := params["upload_files"]; ok && uploadFiles != nil && bridgeParams["path"] == nil {
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
		outputStr := ""
		if s, ok := output.(string); ok {
			outputStr = s
		} else {
			b, _ := json.Marshal(output)
			outputStr = string(b)
		}
		threshold := 200
		if mcpcfg.Get().ChatFileThresholdKb > 0 {
			threshold = mcpcfg.Get().ChatFileThresholdKb
		}
		maxSize := threshold * 1024
		if len(outputStr) > maxSize {
			chatDir := ""
			if mcpcfg.Get().ChatOutputPath != nil && *mcpcfg.Get().ChatOutputPath != "" {
				chatDir = *mcpcfg.Get().ChatOutputPath
			} else {
				ws := shared.ResolveWorkspace(workspace, workspace, deps.Store)
				chatDir = filepath.Join(ws, ".zenmcp", "chat")
			}
			if err := os.MkdirAll(chatDir, 0755); err == nil {
				fp := filepath.Join(chatDir, fmt.Sprintf("%s-%d.md", action, time.Now().UnixMilli()))
				if writeErr := os.WriteFile(fp, []byte(outputStr), 0644); writeErr == nil {
					output = fmt.Sprintf("Response saved to `%s` (%.1f KB)", fp, float64(len(outputStr))/1024.0)
				} else {
					maxInline := 50000
					if len(outputStr) > maxInline {
						output = outputStr[:maxInline] + fmt.Sprintf("\n... [TRUNCATED: file write failed, showing first 50KB of %.1f KB total]", float64(len(outputStr))/1024.0)
					} else {
						output = outputStr
					}
				}
			}
		}
	}

	logSessionEvent(workspace, "browser", fmt.Sprintf("browser: %s", action), fmt.Sprintf("Params: %v\n\nOutput: %v", bridgeParams, output))

	return output
}

func isChatLike(action string) bool {
	switch action {
	case "chat", "brainstorm", "brainstorm_status":
		return true
	}
	return false
}

func postJSON(ctx context.Context, url string, body map[string]any) (map[string]any, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", "recorder.local")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var result any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return map[string]any{"statusCode": resp.StatusCode}, nil
	}
	if m, ok := result.(map[string]any); ok {
		m["statusCode"] = resp.StatusCode
		return m, nil
	}
	return map[string]any{"statusCode": resp.StatusCode, "body": result}, nil
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
	return out
}

func parseURL(s string) (*url.URL, error) {
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	return url.Parse(s)
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
