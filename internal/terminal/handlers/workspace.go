package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("cd", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: cd <path>")
		}
		fmt.Println(terminal.ExecuteTool("workspace", map[string]any{"path": args[0]}))
		return nil
	})
}
