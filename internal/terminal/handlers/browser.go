package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("br", func(args []string, sessionID string) error {
		if len(args) == 0 {
			terminal.Logf("ERROR: Missing browser action. Usage: br <action> [args...]")
			terminal.Logf("Available: containers, active, restart, refresh, logs, chrome, nav <url>, read [url], content [selector], text, chat <message>, screenshot, tabs [query], eval <code>, request <url> [method], search <query>, map")
			return nil
		}

		action := args[0]
		var toolArgs map[string]any

		switch action {
		case "containers":
			toolArgs = map[string]any{"action": "list_containers"}
		case "active":
			toolArgs = map[string]any{"action": "active_tab"}
		case "restart":
			toolArgs = map[string]any{"action": "restart"}
		case "refresh":
			toolArgs = map[string]any{"action": "refresh"}
		case "logs":
			toolArgs = map[string]any{"action": "web_logs"}
		case "chrome":
			toolArgs = map[string]any{"action": "chrome_logs"}
		case "nav", "navigate":
			if len(args) < 2 {
				terminal.Logf("ERROR: Missing URL. Usage: br nav <url>")
				return nil
			}
			toolArgs = map[string]any{"action": "navigate", "url": args[1]}
		case "read":
			if len(args) < 2 {
				terminal.Logf("ERROR: Missing URL. Usage: br read <url>")
				return nil
			}
			toolArgs = map[string]any{"action": "read", "url": args[1]}
		case "content", "get_content":
			if len(args) < 2 {
				terminal.Logf("ERROR: Missing selector. Usage: br content <selector>")
				return nil
			}
			toolArgs = map[string]any{"action": "get_content", "selector": args[1]}
		case "text", "get_text":
			toolArgs = map[string]any{"action": "get_text"}
		case "chat":
			if len(args) < 2 {
				terminal.Logf("ERROR: Missing message. Usage: br chat <message>")
				return nil
			}
			toolArgs = map[string]any{"action": "chat", "message": strings.Join(args[1:], " ")}
		case "screenshot":
			toolArgs = map[string]any{"action": "screenshot"}
		case "tabs", "list_tabs":
			query := ""
			if len(args) > 1 {
				query = args[1]
			}
			toolArgs = map[string]any{"action": "list_tabs", "query": query}
		case "eval", "web_eval":
			if len(args) < 2 {
				terminal.Logf("ERROR: Missing code. Usage: br eval <code>")
				return nil
			}
			toolArgs = map[string]any{"action": "web_eval", "code": strings.Join(args[1:], " ")}
		case "request":
			if len(args) < 2 {
				terminal.Logf("ERROR: Missing URL. Usage: br request <url> [method]")
				return nil
			}
			method := "GET"
			if len(args) > 2 {
				method = args[2]
			}
			toolArgs = map[string]any{"action": "request", "url": args[1], "method": method}
		case "search":
			if len(args) < 2 {
				terminal.Logf("ERROR: Missing query. Usage: br search <query>")
				return nil
			}
			toolArgs = map[string]any{"action": "search", "url": strings.Join(args[1:], " ")}
		case "map":
			toolArgs = map[string]any{"action": "get_interactive_map"}
		default:
			terminal.Logf("ERROR: Unknown browser action '%s'", action)
			terminal.Logf("Available: containers, active, restart, refresh, logs, chrome, nav, read, content, text, chat, screenshot, tabs, eval, request, search, map")
			return nil
		}

		terminal.Logf("[browser] %s...", action)
		res := terminal.ExecuteTool("browser", toolArgs)
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("request", func(args []string, sessionID string) error {
		return handleBrowserRequest(args, sessionID)
	})
	terminal.Register("browser-request", func(args []string, sessionID string) error {
		return handleBrowserRequest(args, sessionID)
	})

	terminal.Register("commit-review", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: commit-review <commitA> [commitB]")
		}
		commitA := args[0]
		commitB := ""
		if len(args) > 1 {
			commitB = args[1]
		}
		terminal.Logf("COMMIT-REVIEW: %s %s", commitA, commitB)
		terminal.Logf("(commit-review requires git integration; use codegraph impact for change analysis)")
		return nil
	})
}

func handleBrowserRequest(args []string, sessionID string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: request <url> [method] [body] [headersJson]")
	}
	url := args[0]
	method := "GET"
	if len(args) > 1 {
		method = args[1]
	}
	bodyStr := ""
	if len(args) > 2 {
		bodyStr = args[2]
	}
	headersStr := ""
	if len(args) > 3 {
		headersStr = args[3]
	}

	toolArgs := map[string]any{
		"action": "request",
		"url":    url,
		"method": method,
	}
	if bodyStr != "" {
		var body any
		if err := json.Unmarshal([]byte(bodyStr), &body); err != nil {
			body = bodyStr
		}
		toolArgs["body"] = body
	}
	if headersStr != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersStr), &headers); err == nil {
			toolArgs["headers"] = headers
		}
	}

	terminal.Logf("REQUEST: %s %s...", method, url)
	res := terminal.ExecuteTool("browser", toolArgs)
	terminal.Logf("RESULT:\n%s", res)
	return nil
}
