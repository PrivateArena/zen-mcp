package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("refactor-copy", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: refactor-copy <target> [mode] [--dry-run] [isolate=N] --sources <json>")
		}
		targetPath := args[0]
		mode := "append"
		sourcesJSON := "[]"
		dryRun := false
		isolate := 0

		sourcesIdx := -1
		for i, a := range args {
			switch a {
			case "append", "overwrite":
				mode = a
			case "--dry-run":
				dryRun = true
			case "--json":
				// handled by codegraph action
			default:
				if strings.HasPrefix(a, "isolate=") {
					fmt.Sscanf(a, "isolate=%d", &isolate)
				} else if a == "--sources" {
					sourcesIdx = i
				}
			}
		}
		if sourcesIdx != -1 && sourcesIdx+1 < len(args) {
			sourcesJSON = args[sourcesIdx+1]
		}

		terminal.Logf("REFACTOR COPY: %s (mode: %s, dry-run: %v)", targetPath, mode, dryRun)

		var sources []map[string]any
		if err := json.Unmarshal([]byte(sourcesJSON), &sources); err != nil {
			terminal.Logf("ERROR: Failed to parse sources JSON: %v", err)
			return nil
		}

		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":      "refactor_copy",
			"query":       targetPath,
			"isolate":     isolate,
			"dry_run":     dryRun,
			"target_path": targetPath,
			"mode":        mode,
			"sources":     sources,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("refactor-delete", func(args []string, sessionID string) error {
		targetsJSON := "[]"
		dryRun := false
		isolate := 0

		targetsIdx := -1
		for i, a := range args {
			switch a {
			case "--dry-run":
				dryRun = true
			case "--json":
				// handled by codegraph action
			default:
				if strings.HasPrefix(a, "isolate=") {
					fmt.Sscanf(a, "isolate=%d", &isolate)
				} else if a == "--targets" {
					targetsIdx = i
				}
			}
		}
		if targetsIdx != -1 && targetsIdx+1 < len(args) {
			targetsJSON = args[targetsIdx+1]
		}

		terminal.Logf("REFACTOR DELETE (dry-run: %v)", dryRun)

		var targets []map[string]any
		if err := json.Unmarshal([]byte(targetsJSON), &targets); err != nil {
			terminal.Logf("ERROR: Failed to parse targets JSON: %v", err)
			return nil
		}

		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":   "refactor_delete",
			"isolate":  isolate,
			"dry_run":  dryRun,
			"targets":  targets,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("refactor-rollback", func(args []string, sessionID string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: refactor-rollback <file> [isolate=N]")
		}
		query := args[0]
		isolate := 0
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "isolate=") {
				fmt.Sscanf(a, "isolate=%d", &isolate)
			}
		}

		terminal.Logf("REFACTOR ROLLBACK: \"%s\"", query)
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "refactor_rollback",
			"query":   query,
			"isolate": isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})
}
