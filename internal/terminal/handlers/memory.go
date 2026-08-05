package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("bl", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("memory", map[string]any{"action": "load"}))
		return nil
	})
	terminal.Register("bs", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: bs <query>")
		}
		fmt.Printf("Searching brain: %s\n", args[0])
		return nil
	})
	terminal.Register("be", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: be <query>")
		}
		fmt.Printf("Extracting brain entry: %s\n", args[0])
		return nil
	})
	terminal.Register("loadi", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("memory", map[string]any{"action": "load"}))
		return nil
	})
	terminal.Register("loads", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("memory", map[string]any{"action": "load"}))
		return nil
	})
	terminal.Register("savei", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: savei <title> [notes...]")
		}
		fmt.Printf("Saving to isolated whiteboard: %s\n", args[0])
		return nil
	})
	terminal.Register("saves", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: saves <title> [notes...]")
		}
		fmt.Printf("Saving to shared whiteboard: %s\n", args[0])
		return nil
	})
	terminal.Register("scope", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("memory", map[string]any{"action": "scope"}))
		return nil
	})
	terminal.Register("scopes", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("memory", map[string]any{"action": "scope"}))
		return nil
	})
}
