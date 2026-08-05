package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("br", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: br <url>")
		}
		fmt.Println(terminal.ExecuteTool("browser", map[string]any{"action": "navigate", "url": args[0]}))
		return nil
	})
	terminal.Register("request", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: request <url> [method] [body] [headersJson]")
		}
		fmt.Println(terminal.ExecuteTool("browser", map[string]any{"action": "request", "url": args[0]}))
		return nil
	})
	terminal.Register("browser-request", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: browser-request <url>")
		}
		fmt.Println(terminal.ExecuteTool("browser", map[string]any{"action": "request", "url": args[0]}))
		return nil
	})
}
