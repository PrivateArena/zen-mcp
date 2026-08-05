package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("shell", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: shell <cmd>")
		}
		fmt.Printf("Executing shell command: %s\n", args[0])
		return nil
	})
}
