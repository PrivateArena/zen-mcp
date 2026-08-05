package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("prompt", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: prompt <name> [args...]")
		}
		fmt.Printf("Previewing prompt: %s\n", args[0])
		return nil
	})
	terminal.Register("generate-commands", func(args []string, sessionID string) error {
		fmt.Println("Generating commands from prompts...")
		return nil
	})
	terminal.Register("export-commands", func(args []string, sessionID string) error {
		fmt.Println("Exporting commands...")
		return nil
	})
}
