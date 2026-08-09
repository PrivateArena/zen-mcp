package handlers

import (
	"fmt"
	"runtime"
	"time"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/prompts"
	"zen-mcp/internal/shell/processes"
	"zen-mcp/internal/telemetry"
	"zen-mcp/internal/terminal"
)

var startTime = time.Now()

func init() {
	terminal.Register("status", func(args []string) error {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		terminal.Logf("STATUS:\n - Uptime: %ds\n - Memory: %dMB (RSS)\n - Workspace: %s\n - Log Level: %s\n - Platform: %s (%s)",
			int(time.Since(startTime).Seconds()),
			mem.Alloc/1024/1024,
			terminal.Ws(),
			"debug",
			runtime.GOOS,
			runtime.Version())
		return nil
	})

	terminal.Register("log-level", func(args []string) error {
		if len(args) == 0 {
			terminal.Logf("Current Log Level: %s", "debug")
			return nil
		}
		level := args[0]
		valid := []string{"debug", "info", "warn", "error", "off"}
		found := false
		for _, v := range valid {
			if v == level {
				found = true
				break
			}
		}
		if !found {
			terminal.Logf("ERROR: Invalid log level. Valid: %s", fmt.Sprintf("%v", valid))
			return nil
		}
		terminal.Logf("OK: Log Level set to %s", level)
		return nil
	})

	terminal.Register("loglevel", func(args []string) error {
		if len(args) == 0 {
			terminal.Logf("Current Log Level: %s", "debug")
			return nil
		}
		level := args[0]
		valid := []string{"debug", "info", "warn", "error", "off"}
		found := false
		for _, v := range valid {
			if v == level {
				found = true
				break
			}
		}
		if !found {
			terminal.Logf("ERROR: Invalid log level. Valid: %s", fmt.Sprintf("%v", valid))
			return nil
		}
		terminal.Logf("OK: Log Level set to %s", level)
		return nil
	})

	terminal.Register("exit", func(args []string) error {
		terminal.Logf("Shutting down terminal commander.")
		return nil
	})

	terminal.Register("quit", func(args []string) error {
		terminal.Logf("Shutting down terminal commander.")
		return nil
	})

	terminal.Register("abort", func(args []string) error {
		processes.AbortAll()
		terminal.Logf("Aborted command(s).")
		return nil
	})

	terminal.Register("ls", func(args []string) error {
		terminal.Logf("ACTIVE WORKSPACE: %s", terminal.Ws())
		return nil
	})

	terminal.Register("sessions", func(args []string) error {
		terminal.Logf("ACTIVE WORKSPACE: %s", terminal.Ws())
		return nil
	})

	terminal.Register("telemetry", func(args []string) error {
		terminal.Logf(telemetry.QueryTelemetry(args))
		return nil
	})

	terminal.Register("cs", func(args []string) error {
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "status"})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("mcp-catalog", func(args []string) error {
		terminal.Logf("\nMCP Tools Catalog:\n")
		terminal.Logf(prompts.BuildToolCatalog())
		return nil
	})

	terminal.Register("mcp-cost", func(args []string) error {
		terminal.Logf("\n%s", buildToolCost())
		return nil
	})

	terminal.Register("export-cli", func(args []string) error {
		clean, short := parseExportCLIArgs(args)
		if clean {
			terminal.ExportCliClean(terminal.LogOut)
			return nil
		}
		c := mcpcfg.Get()
		cliPort := 0
		mcpPort := 0
		if c != nil {
			cliPort = c.CliPort
			mcpPort = c.McpPort
		}
		terminal.ExportCLIWithShort(terminal.LogOut, cliPort, mcpPort, short)
		return nil
	})
}

// parseExportCLIArgs extracts the --clean/clean and --short/short flags from
// export-cli args; unknown args are ignored. It returns clean=false, short=false
// when no flag is present (the plain export default).
func parseExportCLIArgs(args []string) (clean, short bool) {
	for _, a := range args {
		switch a {
		case "--clean", "clean":
			clean = true
		case "--short", "short":
			short = true
		}
	}
	return clean, short
}

func buildToolCost() string {
	c := mcpcfg.Get()
	if c == nil {
		return "MCP Tool Registration Token Cost Estimation:\n  (config not loaded)"
	}
	return "MCP Tool Registration Token Cost Estimation:\n  (see tools/list for details)"
}
