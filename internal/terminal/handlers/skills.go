package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("sg", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: sg <id>")
		}
		fmt.Println(terminal.ExecuteTool("skill", map[string]any{"action": "get", "id": args[0]}))
		return nil
	})
}
