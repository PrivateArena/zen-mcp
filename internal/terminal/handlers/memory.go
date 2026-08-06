package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/jang/zen-mcp/internal/projectmemory"
	"github.com/jang/zen-mcp/internal/terminal"
)

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

	terminal.Register("be", func(args []string) error {
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
		lines := strings.Split(string(raw), "\n")
		terms := strings.ToLower(q)
		terms = strings.ReplaceAll(terms, ",", " ")
		termList := strings.Fields(terms)
		if len(termList) == 0 {
			terminal.Logf("ERROR: No search terms")
			return nil
		}

		type scoredLine struct {
			line  string
			score int
			data  map[string]any
		}
		var scored []scoredLine
		for _, line := range lines {
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
				scored = append(scored, scoredLine{line: line, score: score, data: parsed})
			}
		}

		if len(scored) == 0 {
			terminal.Logf("RESULT: No matching brain entries found.")
			return nil
		}

		best := scored[0].data
		b, _ := json.MarshalIndent(best, "", "  ")
		terminal.Logf("RESULT:\n%s", string(b))
		return nil
	})

	terminal.Register("brain-extract", func(args []string) error {
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
		lines := strings.Split(string(raw), "\n")
		terms := strings.ToLower(q)
		terms = strings.ReplaceAll(terms, ",", " ")
		termList := strings.Fields(terms)
		if len(termList) == 0 {
			terminal.Logf("ERROR: No search terms")
			return nil
		}

		type scoredLine struct {
			line  string
			score int
			data  map[string]any
		}
		var scored []scoredLine
		for _, line := range lines {
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
				scored = append(scored, scoredLine{line: line, score: score, data: parsed})
			}
		}

		if len(scored) == 0 {
			terminal.Logf("RESULT: No matching brain entries found.")
			return nil
		}

		best := scored[0].data
		b, _ := json.MarshalIndent(best, "", "  ")
		terminal.Logf("RESULT:\n%s", string(b))
		return nil
	})

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
