package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"zen-mcp/internal/projectmemory"
	"zen-mcp/internal/terminal"
)

var (
	brainUnsafeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	brainCollapseRe   = regexp.MustCompile(`_+`)
)

// brainExtract ports the TS be/brain-extract handler: scores timeline entries,
// picks the best, converts it to markdown, and writes it under .zenmcp/brain/.
func brainExtract(args []string) error {
	q := strings.TrimSpace(strings.Join(args, " "))
	if q == "" {
		terminal.Logf("ERROR: Missing query. Usage: be <query>")
		return nil
	}
	terminal.Logf("BRAIN EXTRACT: \"%s\"", q)
	wRoot := terminal.Ws()
	timelinePath := filepath.Join(wRoot, ".zenmcp", "brain_timeline.jsonl")
	if _, err := os.Stat(timelinePath); os.IsNotExist(err) {
		terminal.Logf("ERROR: No brain_timeline.jsonl found in workspace")
		return nil
	}
	raw, err := os.ReadFile(timelinePath)
	if err != nil {
		terminal.Logf("ERROR: Failed to read brain_timeline.jsonl: %v", err)
		return nil
	}

	termList := strings.Fields(strings.ToLower(q))
	if len(termList) == 0 {
		terminal.Logf("ERROR: No search terms")
		return nil
	}

	type scoredLine struct {
		line  string
		score int
	}
	var scored []scoredLine
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			continue
		}
		haystack := strings.ToLower(line)
		score := 0
		for _, term := range termList {
			if strings.Contains(haystack, term) {
				score++
			}
		}
		if score > 0 {
			scored = append(scored, scoredLine{line: line, score: score})
		}
	}

	if len(scored) == 0 {
		terminal.Logf("RESULT: No matching brain entries found.")
		return nil
	}

	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

	md := projectmemory.JSONToMarkdown(scored[0].line)

	brainDir := filepath.Join(wRoot, ".zenmcp", "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		terminal.Logf("ERROR: Failed to create brain dir: %v", err)
		return nil
	}
	outPath := filepath.Join(brainDir, brainSafeName(q)+".md")
	if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
		terminal.Logf("ERROR: Failed to write brain extract: %v", err)
		return nil
	}
	terminal.Logf("OK: Brain extract saved to %s", outPath)
	return nil
}

// brainSafeName ports the TS filename derivation for brain extracts.
func brainSafeName(q string) string {
	s := brainUnsafeNameRe.ReplaceAllString(q, "_")
	s = brainCollapseRe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	s = strings.ToUpper(s)
	if s == "" {
		return "BRAIN_EXTRACT"
	}
	return s
}

func init() {
	terminal.Register("bl", func(args []string) error {
		res := terminal.ExecuteTool("memory", map[string]any{"action": "load"})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("load", func(args []string) error {
		res := terminal.ExecuteTool("memory", map[string]any{"action": "load"})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("history", func(args []string) error {
		res := terminal.ExecuteTool("memory", map[string]any{"action": "load"})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("bs", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			terminal.Logf("ERROR: Missing query")
			return nil
		}
		terminal.Logf("BRAIN SEARCH: \"%s\"", q)
		wRoot := terminal.Ws()
		dataDir := filepath.Join(wRoot, ".zenmcp")
		dbPath := filepath.Join(dataDir, "context.db")
		results := projectmemory.SearchIndexedMemory(dbPath, q, 10)
		if len(results) == 0 {
			terminal.Logf("RESULT: No matching brain entries found.")
			return nil
		}
		for i, r := range results {
			terminal.Logf("  [%d] %s: %s", i+1, r.Title, r.Content)
		}
		return nil
	})

	terminal.Register("history-search", func(args []string) error {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			terminal.Logf("ERROR: Missing query")
			return nil
		}
		terminal.Logf("BRAIN SEARCH: \"%s\"", q)
		wRoot := terminal.Ws()
		dataDir := filepath.Join(wRoot, ".zenmcp")
		dbPath := filepath.Join(dataDir, "context.db")
		results := projectmemory.SearchIndexedMemory(dbPath, q, 10)
		if len(results) == 0 {
			terminal.Logf("RESULT: No matching brain entries found.")
			return nil
		}
		for i, r := range results {
			terminal.Logf("  [%d] %s: %s", i+1, r.Title, r.Content)
		}
		return nil
	})

	terminal.Register("be", brainExtract)
	terminal.Register("brain-extract", brainExtract)

	terminal.Register("loadi", func(args []string) error {
		wRoot := terminal.Ws()
		res := terminal.ExecuteTool("memory_isolate", map[string]any{"action": "load", "workspace": wRoot})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("loads", func(args []string) error {
		wRoot := terminal.Ws()
		res := terminal.ExecuteTool("memory_shared", map[string]any{"action": "load", "workspace": wRoot})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("savei", func(args []string) error {
		if len(args) == 0 {
			terminal.Logf("ERROR: Missing title. Usage: savei <title> [notes...]")
			return nil
		}
		title := args[0]
		notes := strings.Join(args[1:], " ")
		wRoot := terminal.Ws()
		res := terminal.ExecuteTool("memory_isolate", map[string]any{"action": "save", "workspace": wRoot, "session_title": title, "session_notes": notes})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("saves", func(args []string) error {
		if len(args) == 0 {
			terminal.Logf("ERROR: Missing title. Usage: saves <title> [notes...]")
			return nil
		}
		title := args[0]
		notes := strings.Join(args[1:], " ")
		wRoot := terminal.Ws()
		res := terminal.ExecuteTool("memory_shared", map[string]any{"action": "save", "workspace": wRoot, "session_title": title, "session_notes": notes})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("scope", func(args []string) error {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		if name == "" {
			terminal.Logf("ERROR: Missing scope name")
			return nil
		}
		wRoot := terminal.Ws()
		res := terminal.ExecuteTool("memory", map[string]any{"action": "scope", "workspace": wRoot, "scope": name})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})

	terminal.Register("scopes", func(args []string) error {
		wRoot := terminal.Ws()
		res := terminal.ExecuteTool("memory", map[string]any{"action": "scope", "workspace": wRoot})
		terminal.Logf("RESULT:\n%s", res)
		return nil
	})
}
