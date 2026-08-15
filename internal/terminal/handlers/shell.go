package handlers

import (
	"fmt"
	"strings"

	"zen-mcp/internal/terminal"
)

// initializes the package
func init() {
	terminal.Register("shell", func(args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: shell <cmd>")
		}
		fmt.Println(terminal.ExecuteTool("shell", map[string]any{"action": "run", "cmd": strings.Join(args, " ")}))
		return nil
	})
}
