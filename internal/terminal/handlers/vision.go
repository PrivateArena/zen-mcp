package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("uv", func(args []string, sessionID string) error {
		if len(args) < 2 {
			return fmt.Errorf("usage: uv <path> <message>")
		}
		fmt.Printf("Launching & describing GUI via Gemini: %s\n", args[0])
		return nil
	})
}
