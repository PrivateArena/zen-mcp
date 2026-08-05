package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("commit-review", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: commit-review <A> [B]")
		}
		fmt.Printf("Reviewing commit(s): %s\n", args[0])
		return nil
	})
	terminal.Register("git-tmp", func(args []string, sessionID string) error {
		fmt.Println("Mirroring current repo to junk GitHub org for review...")
		return nil
	})
	terminal.Register("git-twipe", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: git-twipe <id>")
		}
		fmt.Printf("Deleting temp repo: %s\n", args[0])
		return nil
	})
}
