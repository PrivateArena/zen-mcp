package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("accept", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: accept [id]")
		}
		fmt.Printf("Accepting confirmation: %s\n", args[0])
		return nil
	})
	terminal.Register("reject", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: reject [id] [suggestion...]")
		}
		fmt.Printf("Rejecting confirmation: %s\n", args[0])
		return nil
	})
	terminal.Register("pending", func(args []string, sessionID string) error {
		fmt.Println("Pending confirmations:")
		return nil
	})
}
