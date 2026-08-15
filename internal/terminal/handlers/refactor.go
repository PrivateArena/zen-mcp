package handlers

import (
	"encoding/json"

	"zen-mcp/internal/terminal"
)

// initializes the package
func init() {
	terminal.Register("refactor-copy", func(args []string) error {
		if len(args) == 0 {
			terminal.Logf("ERROR: Missing target_path. Usage: refactor-copy <target_path> [mode] [--dry-run] [isolate=N] --sources <json>")
			return nil
		}
		parsed := terminal.ParseCodegraphArgs(args)
		targetPath := args[0]
		mode := "append"
		sourcesJSON := "[]"

		sourcesIdx := -1
		for i, a := range args {
			switch a {
			case "append", "overwrite":
				mode = a
			case "--sources":
				sourcesIdx = i
			}
		}
		if sourcesIdx != -1 && sourcesIdx+1 < len(args) {
			sourcesJSON = args[sourcesIdx+1]
		}

		terminal.Logf("REFACTOR COPY: %s (mode: %s, dry-run: %v)", targetPath, mode, parsed.DryRun)

		var sources []map[string]any
		if err := json.Unmarshal([]byte(sourcesJSON), &sources); err != nil {
			terminal.Logf("ERROR: Failed to parse sources JSON: %v", err)
			return nil
		}

		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":      "refactor_copy",
			"query":       targetPath,
			"isolate":     parsed.Isolate,
			"dry_run":     parsed.DryRun,
			"target_path": targetPath,
			"mode":        mode,
			"sources":     sources,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("refactor-delete", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		targetsJSON := "[]"

		targetsIdx := -1
		for i, a := range args {
			switch a {
			case "--targets":
				targetsIdx = i
			}
		}
		if targetsIdx != -1 && targetsIdx+1 < len(args) {
			targetsJSON = args[targetsIdx+1]
		}

		terminal.Logf("REFACTOR DELETE (dry-run: %v)", parsed.DryRun)

		var targets []map[string]any
		if err := json.Unmarshal([]byte(targetsJSON), &targets); err != nil {
			terminal.Logf("ERROR: Failed to parse targets JSON: %v", err)
			return nil
		}

		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "refactor_delete",
			"isolate": parsed.Isolate,
			"dry_run": parsed.DryRun,
			"targets": targets,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("refactor-rollback", func(args []string) error {
		if len(args) == 0 {
			terminal.Logf("ERROR: Missing file path. Usage: refactor-rollback <file_path> [isolate=N]")
			return nil
		}
		parsed := terminal.ParseCodegraphArgs(args)
		query := args[0]
		terminal.Logf("REFACTOR ROLLBACK: \"%s\"", query)
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "refactor_rollback",
			"query":   query,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})
}
