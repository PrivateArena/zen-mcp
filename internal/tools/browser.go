package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jang/zen-mcp/internal/bridge"
	"github.com/jang/zen-mcp/internal/mcpcfg"
	"github.com/jang/zen-mcp/internal/toolresponse"
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

	switch action {
	case "restart":
		res, err := bridge.CallBridge(ctx, "reload", bridgeParams)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "browser", res, start)
	case "read":
		if urlStr, ok := bridgeParams["url"].(string); ok && urlStr != "" {
			if _, err := bridge.CallBridge(ctx, "navigate", map[string]any{"url": urlStr, "new_tab": true}); err != nil {
				return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
			}
		}
		res, err := bridge.CallBridge(ctx, "read", bridgeParams)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "browser", extractData(res), start)
	}

	res, err := bridge.CallBridge(ctx, action, bridgeParams)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "browser", err, start)
	}
	return toolresponse.WrapSuccess(ctx, "browser", extractData(res), start)
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
