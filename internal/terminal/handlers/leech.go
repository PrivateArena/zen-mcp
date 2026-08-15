package handlers

import (
	"zen-mcp/internal/terminal"
)

// initializes the package
func init() {
	terminal.Register("leech", func(args []string) error {
		terminal.Logf("Leech mode...")
		return nil
	})
}
