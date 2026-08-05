package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("sl", func(args []string, sessionID string) error {
		fmt.Println("Available skills:")
		return nil
	})
	terminal.Register("sg", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: sg <id>")
		}
		fmt.Printf("Skill details: %s\n", args[0])
		return nil
	})
	terminal.Register("si", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: si <path>")
		}
		fmt.Printf("Installing skill from: %s\n", args[0])
		return nil
	})
}
