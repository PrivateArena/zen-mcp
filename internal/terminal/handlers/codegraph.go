package handlers

import (
	"os"
	"path/filepath"
	"time"

	"zen-mcp/internal/terminal"
	"zen-mcp/internal/tools"
)

func init() {
	terminal.Register("index", func(args []string) error {
		isForce := false
		for _, a := range args {
			if a == "--force" || a == "---force" {
				isForce = true
				break
			}
		}
		if isForce {
			terminal.Logf("Force Reindexing...")
			workspace := terminal.Ws()
			tools.ClearSessionGraphByWorkspace(workspace)
			dbFile := filepath.Join(workspace, ".zenmcp", "codegraph.db")
			if _, err := os.Stat(dbFile); err == nil {
				terminal.Logf("Force Reindexing: Removing old database at %s...", dbFile)
				os.Remove(dbFile)
			}
		}
		terminal.Logf("CodeGraph Indexing...")
		start := time.Now()
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "index"})
		elapsed := time.Since(start)
		terminal.Logf("Index completed in %s", elapsed)
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("search", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		if parsed.Query == "" {
			terminal.Logf("ERROR: Missing query")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":   "search",
			"query":    parsed.Query,
			"limit":    parsed.Limit,
			"isolate":  parsed.Isolate,
			"semantic": false,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("map", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "map",
			"limit":   parsed.Limit,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("skeletons", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		if parsed.Query == "" {
			terminal.Logf("ERROR: Missing file paths")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "skeletons",
			"query":   parsed.Query,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("mermaid", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "mermaid",
			"query":   parsed.Query,
			"limit":   parsed.Limit,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("neighbors", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		if parsed.Query == "" {
			terminal.Logf("ERROR: Missing query")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "neighbors",
			"query":   parsed.Query,
			"limit":   parsed.Limit,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("usage", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		if parsed.Query == "" {
			terminal.Logf("ERROR: Missing query")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "usage",
			"query":   parsed.Query,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("files", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "files",
			"query":   parsed.Query,
			"limit":   parsed.Limit,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("explain", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		if parsed.Query == "" {
			terminal.Logf("ERROR: Missing query")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "explain",
			"query":   parsed.Query,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("related", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		if parsed.Query == "" {
			terminal.Logf("ERROR: Missing file path")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "related",
			"query":   parsed.Query,
			"limit":   parsed.Limit,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("deadcode", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "deadcode",
			"query":   parsed.Query,
			"limit":   parsed.Limit,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("markdown", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "markdown",
			"query":   parsed.Query,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("shortestpath", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		if parsed.Query == "" {
			terminal.Logf("ERROR: Missing source,target. Usage: shortestpath <source,target> [limit] [isolate=N] [--json]")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":      "shortestPath",
			"query":       parsed.Query,
			"limit":       parsed.Limit,
			"isolate":     parsed.Isolate,
			"format_json": parsed.FormatJson,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("findcycles", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "findCycles",
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("impact", func(args []string) error {
		parsed := terminal.ParseCodegraphArgs(args)
		res := terminal.ExecuteTool("codegraph", map[string]any{
			"action":  "impact",
			"query":   parsed.Query,
			"isolate": parsed.Isolate,
		})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})
}
