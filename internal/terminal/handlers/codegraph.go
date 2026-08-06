package handlers

import (
	"strings"

	"github.com/jang/zen-mcp/internal/terminal"
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
			res := terminal.ExecuteTool("codegraph", map[string]any{"action": "index"})
			terminal.Logf("RESULT:\n%s", res)
			return nil
		}
		terminal.Logf("CodeGraph Indexing...")
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "index"})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("search", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			terminal.Logf("ERROR: Missing query")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "search", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("map", func(args []string) error {
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "map"})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("skeletons", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			terminal.Logf("ERROR: Missing file paths")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "skeletons", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("mermaid", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "mermaid", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("neighbors", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			terminal.Logf("ERROR: Missing query")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "neighbors", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("usage", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			terminal.Logf("ERROR: Missing query")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "usage", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("files", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "files", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("explain", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			terminal.Logf("ERROR: Missing query")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "explain", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("related", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			terminal.Logf("ERROR: Missing file path")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "related", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("deadcode", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "deadcode", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("markdown", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "markdown", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("shortestpath", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			terminal.Logf("ERROR: Missing source,target. Usage: shortestpath <source,target> [limit] [isolate=N] [--json]")
			return nil
		}
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "shortestPath", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("findcycles", func(args []string) error {
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "findCycles"})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("impact", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		res := terminal.ExecuteTool("codegraph", map[string]any{"action": "impact", "query": q})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})
}
