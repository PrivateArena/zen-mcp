package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("refactor-copy", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: refactor-copy <target> [mode] [--dry-run] [isolate=N] --sources <json>")
		}
		fmt.Printf("Refactor copy: %s\n", args[0])
		return nil
	})
	terminal.Register("refactor-delete", func(args []string, sessionID string) error {
		fmt.Println("Refactor delete...")
		return nil
	})
	terminal.Register("refactor-rollback", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: refactor-rollback <file> [isolate=N]")
		}
		fmt.Printf("Refactor rollback: %s\n", args[0])
		return nil
	})
}
