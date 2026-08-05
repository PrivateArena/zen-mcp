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
		fmt.Printf("Opening browser: %s\n", args[0])
		return nil
	})
	terminal.Register("request", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: request <url> [method] [body] [headersJson]")
		}
		fmt.Printf("HTTP request: %s\n", args[0])
		return nil
	})
	terminal.Register("browser-request", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: browser-request <url>")
		}
		fmt.Printf("Browser request: %s\n", args[0])
		return nil
	})
}
