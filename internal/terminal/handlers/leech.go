package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("leech", func(args []string) error {
		fmt.Println("Leech mode...")
		return nil
	})
}
