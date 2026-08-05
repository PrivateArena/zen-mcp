package handlers

import (
	"fmt"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("status", func(args []string, sessionID string) error {
		fmt.Println("STATUS:\n - Server: READY\n - Session:", sessionID)
		return nil
	})

	terminal.Register("log-level", func(args []string, sessionID string) error {
		if len(args) == 0 {
			fmt.Println("Current Log Level: info")
			return nil
		}
		fmt.Printf("Log Level set to: %s\n", args[0])
		return nil
	})

	terminal.Register("loglevel", func(args []string, sessionID string) error {
		if len(args) == 0 {
			fmt.Println("Current Log Level: info")
			return nil
		}
		fmt.Printf("Log Level set to: %s\n", args[0])
		return nil
	})

	terminal.Register("abort", func(args []string, sessionID string) error {
		fmt.Println("Aborted 0 command(s).")
		return nil
	})

	terminal.Register("ls", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("workspace", map[string]any{}))
		return nil
	})

	terminal.Register("sessions", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("workspace", map[string]any{}))
		return nil
	})

	terminal.Register("telemetry", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("memory", map[string]any{"action": "scope"}))
		return nil
	})

	terminal.Register("mcp-catalog", func(args []string, sessionID string) error {
		fmt.Println(terminal.ExecuteTool("codegraph", map[string]any{"action": "map"}))
		return nil
	})

	terminal.Register("mcp-cost", func(args []string, sessionID string) error {
		fmt.Println("MCP Tool Registration Token Cost Estimation:")
		return nil
	})

	terminal.Register("export-cli", func(args []string, sessionID string) error {
		fmt.Println("Exporting CLI wrappers...")
		return nil
	})
}
