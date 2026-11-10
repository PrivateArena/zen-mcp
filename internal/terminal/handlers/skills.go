package handlers

import (
	"fmt"

	"zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("sl", func(args []string) error {
		fmt.Println(terminal.ExecuteTool("skill", map[string]any{"action": "list"}))
		return nil
	})
	terminal.Register("sg", func(args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: sg <id> [section ...]   (sections narrow the skill down, see so <id>)")
		}
		call := map[string]any{"action": "get", "id": args[0]}
		if len(args) > 1 {
			call["filter_sections"] = args[1:]
		}
		fmt.Println(terminal.ExecuteTool("skill", call))
		return nil
	})
	terminal.Register("so", func(args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: so <id>")
		}
		fmt.Println(terminal.ExecuteTool("skill", map[string]any{"action": "outline", "id": args[0]}))
		return nil
	})
}
