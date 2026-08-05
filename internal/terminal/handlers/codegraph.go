package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("index", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("codegraph", map[string]any{"action": "status"}))
		return nil
	})

	terminal.Register("search", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: search <query>")
		}
		fmt.Println(terminal.ExecuteTool("codegraph", map[string]any{"action": "search", "query": args[0]}))
		return nil
	})

	terminal.Register("map", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("codegraph", map[string]any{"action": "map"}))
		return nil
	})

	terminal.Register("cs", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("codegraph", map[string]any{"action": "status"}))
		return nil
	})
}
