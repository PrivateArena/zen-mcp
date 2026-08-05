package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("root", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: root <path>")
		}
		fmt.Printf("Setting workspace root: %s\n", args[0])
		return nil
	})
	terminal.Register("cd", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: cd <path>")
		}
		fmt.Printf("Changing directory: %s\n", args[0])
		return nil
	})
}
