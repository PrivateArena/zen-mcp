package handlers

import (
	"fmt"
	"strings"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("uv", func(args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("usage: uv <path> <message>")
		}
		appPath := args[0]
		message := strings.Join(args[1:], " ")
		terminal.Logf("UI-VISION: %s — \"%s\"...", appPath, message)
		res := terminal.ExecuteTool("ui-vision", map[string]any{
			"path":    appPath,
			"message": message,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})
}
