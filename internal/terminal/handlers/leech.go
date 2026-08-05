package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("leech", func(args []string, sessionID string) error {
		fmt.Println("Leech mode...")
		return nil
	})
}
