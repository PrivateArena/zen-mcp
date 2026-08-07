package projectmemory

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"zen-mcp/internal/logfilter"
)

// NormalizeKey ports timeline.ts normalizeKey.
func NormalizeKey(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`[.!?]+$`).ReplaceAllString(s, "")
	return s
}

// BrainEvent mirrors the schema_version 3 shape.
type BrainEvent struct {
	SchemaVersion int    `json:"schema_version"`
	Timestamp     string `json:"timestamp"`
	SessionTitle  string `json:"session_title,omitempty"`
	Objective     string `json:"objective,omitempty"`
	SessionNotes  string `json:"session_notes,omitempty"`
}

// ReconstructedState mirrors timeline.ts ReconstructedState.
type ReconstructedState struct {
	Workspace     string `json:"workspace"`
	Timestamp     string `json:"timestamp"`
	SchemaVersion int    `json:"schema_version"`
	SessionTitle  string `json:"session_title"`
	Objective     string `json:"objective"`
	SessionNotes  string `json:"session_notes"`
}

// v1FieldsToMarkdown renders legacy v1 fields into v3 markdown headers.
func v1FieldsToMarkdown(raw map[string]any) string {
	var parts []string

	if s, ok := raw["status"].(string); ok && s != "" {
		parts = append(parts, "**Status:** "+s)
	}
	if v, ok := raw["completed_tasks"].([]any); ok && len(v) > 0 {
		var lines []string
		for _, item := range v {
			m, _ := item.(map[string]any)
			task, _ := m["task"].(string)
			outcome, _ := m["outcome"].(string)
			lines = append(lines, "- "+task+" — "+outcome)
		}
		parts = append(parts, "## Progress: Done\n\n"+strings.Join(lines, "\n"))
	}
	if v, ok := raw["pending_tasks"].([]any); ok && len(v) > 0 {
		var lines []string
		for _, item := range v {
			m, _ := item.(map[string]any)
			task, _ := m["task"].(string)
			blocker, _ := m["blocker"].(string)
			nextAction, _ := m["next_action"].(string)
			line := "- " + task
			if blocker != "" {
				line += " (blocked: " + blocker + ")"
			}
			line += " — next: " + nextAction
			lines = append(lines, line)
		}
		parts = append(parts, "## Progress: Pending\n\n"+strings.Join(lines, "\n"))
	}
	if v, ok := raw["key_decisions"].([]any); ok && len(v) > 0 {
		var lines []string
		for _, item := range v {
			m, _ := item.(map[string]any)
			decision, _ := m["decision"].(string)
			rationale, _ := m["rationale"].(string)
			lines = append(lines, "- "+decision+" — "+rationale)
		}
		parts = append(parts, "## Key Decisions\n\n"+strings.Join(lines, "\n"))
	}
	if v, ok := raw["last_test_commands"].([]any); ok && len(v) > 0 {
		var lines []string
		for _, item := range v {
			lines = append(lines, "- `"+asString(item)+"`")
		}
		parts = append(parts, "## Test Commands\n\n"+strings.Join(lines, "\n"))
	}
	if env, ok := raw["environment"].(map[string]any); ok && len(env) > 0 {
		var lines []string
		for k, v := range env {
			lines = append(lines, "- "+k+": "+asString(v))
		}
		parts = append(parts, "## Facts (migrated)\n\n"+strings.Join(lines, "\n"))
	}
	if s, ok := raw["context"].(string); ok && s != "" {
		parts = append(parts, "## Critical Context\n\n"+s)
	}

	return strings.Join(parts, "\n\n")
}

// v2FieldsToMarkdown renders legacy v2 structured fields into v3 markdown.
func v2FieldsToMarkdown(raw map[string]any) string {
	var parts []string

	if s, ok := raw["what_is_done"].(string); ok && s != "" {
		parts = append(parts, "## Progress: Done\n\n"+s)
	}
	if v, ok := raw["episodic"].([]any); ok && len(v) > 0 {
		var lines []string
		for _, item := range v {
			m, _ := item.(map[string]any)
			text, _ := m["text"].(string)
			outcome, _ := m["outcome"].(string)
			line := "- " + text
			if outcome != "" {
				line += " — " + outcome
			}
			lines = append(lines, line)
		}
		parts = append(parts, "## Events\n\n"+strings.Join(lines, "\n"))
	}
	if v, ok := raw["procedural"].([]any); ok && len(v) > 0 {
		var blocks []string
		for _, item := range v {
			m, _ := item.(map[string]any)
			name, _ := m["name"].(string)
			steps, _ := m["steps"].([]any)
			var numbered []string
			for i, s := range steps {
				numbered = append(numbered, itoa(i+1)+". "+asString(s))
			}
			blocks = append(blocks, "### "+name+"\n\n"+strings.Join(numbered, "\n"))
		}
		parts = append(parts, "## Procedures\n\n"+strings.Join(blocks, "\n\n"))
	}
	if semantic, ok := raw["semantic"].(map[string]any); ok {
		if facts, ok := semantic["facts"].([]any); ok && len(facts) > 0 {
			var lines []string
			for _, item := range facts {
				m, _ := item.(map[string]any)
				key, _ := m["key"].(string)
				val, _ := m["value"].(string)
				lines = append(lines, "- "+key+": "+val)
			}
			parts = append(parts, "## Facts (migrated)\n\n"+strings.Join(lines, "\n"))
		}
		if prefs, ok := semantic["preferences"].([]any); ok && len(prefs) > 0 {
			var lines []string
			for _, item := range prefs {
				m, _ := item.(map[string]any)
				key, _ := m["key"].(string)
				val, _ := m["value"].(string)
				lines = append(lines, "- "+key+": "+val)
			}
			parts = append(parts, "## Preferences (migrated)\n\n"+strings.Join(lines, "\n"))
		}
	}

	return strings.Join(parts, "\n\n")
}

// MigrateToV3 normalizes any raw event into the current BrainEvent shape.
func MigrateToV3(raw map[string]any) BrainEvent {
	ev := BrainEvent{SchemaVersion: 3}
	switch schemaVersionOf(raw) {
	case 3:
		ev.Timestamp, _ = raw["timestamp"].(string)
		ev.SessionTitle, _ = raw["session_title"].(string)
		ev.Objective, _ = raw["objective"].(string)
		ev.SessionNotes, _ = raw["session_notes"].(string)
	case 2:
		ev.Timestamp, _ = raw["timestamp"].(string)
		ev.SessionTitle, _ = raw["session_title"].(string)
		ev.Objective, _ = raw["objective"].(string)
		notes := v2FieldsToMarkdown(raw)
		if notes != "" {
			ev.SessionNotes = notes
		}
	default:
		ev.Timestamp, _ = raw["timestamp"].(string)
		ev.SessionTitle, _ = raw["session_title"].(string)
		ev.Objective, _ = raw["objective"].(string)
		notes := v1FieldsToMarkdown(raw)
		if notes != "" {
			ev.SessionNotes = notes
		}
	}
	return ev
}

// schemaVersionOf normalizes a JSON-decoded schema_version (float64, or int
// when constructed in-process) to an int for switch matching.
func schemaVersionOf(raw map[string]any) int {
	switch v := raw["schema_version"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return 0
}

// LatestEvent reads the most recent valid event from the timeline log by
// scanning backwards from the end of the file, so large histories stay cheap
// to load on every prompt resolution. Unparseable trailing lines are skipped.
func LatestEvent(dataDir, memoryName string) (BrainEvent, bool) {
	timelinePath := filepath.Join(dataDir, memoryName+"_timeline.jsonl")
	f, err := os.Open(timelinePath)
	if err != nil {
		return BrainEvent{}, false
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.Size() == 0 {
		return BrainEvent{}, false
	}

	const chunkSize = 64 * 1024
	size := stat.Size()
	chunk := size
	if chunk > chunkSize {
		chunk = chunkSize
	}
	buf := make([]byte, chunk)
	if _, err := f.ReadAt(buf, size-chunk); err != nil {
		return BrainEvent{}, false
	}

	start := size - chunk
	s := string(buf)
	if start > 0 {
		if idx := strings.IndexByte(s, '\n'); idx >= 0 {
			s = s[idx+1:]
		}
	}
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		return MigrateToV3(raw), true
	}
	return BrainEvent{}, false
}

// EventToMarkdown renders a single BrainEvent as markdown for prompt
// injection. Returns "" when the event carries no substantive content (a bare
// timestamp alone is treated as empty).
func EventToMarkdown(ev BrainEvent) string {
	if ev.SessionTitle == "" && ev.Objective == "" && ev.SessionNotes == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Session — " + ev.Timestamp)
	if ev.SessionTitle != "" {
		b.WriteString("\n\n**Session Title:** " + ev.SessionTitle)
	}
	if ev.Objective != "" {
		b.WriteString("\n\n**Objective:** " + ev.Objective)
	}
	if ev.SessionNotes != "" {
		b.WriteString("\n\n" + ev.SessionNotes)
	}
	return b.String()
}

// AppendEvent ports appendEvent: appends one JSONL line to the timeline log.
func AppendEvent(dataDir, memoryName string, event BrainEvent) error {
	timelinePath := filepath.Join(dataDir, memoryName+"_timeline.jsonl")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	ev := event
	ev.SchemaVersion = 3
	ev.Timestamp = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(timelinePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

// ReconstructState ports reconstructState: chronologically merges the
// timeline log. session_title/objective are last-write-wins; session_notes
// blocks accumulate.
func ReconstructState(dataDir, memoryName string) ReconstructedState {
	timelinePath := filepath.Join(dataDir, memoryName+"_timeline.jsonl")
	defaultState := ReconstructedState{
		Timestamp:     time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		SchemaVersion: 3,
	}

	f, err := os.Open(timelinePath)
	if err != nil {
		return defaultState
	}
	defer f.Close()

	state := defaultState
	var notesBlocks []string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			logfilter.Debugf("[ProjectMemory] Failed to parse timeline line: %v", err)
			continue
		}
		event := MigrateToV3(raw)
		if event.SessionTitle != "" {
			state.SessionTitle = event.SessionTitle
		}
		if event.Objective != "" {
			state.Objective = event.Objective
		}
		if event.SessionNotes != "" {
			notesBlocks = append(notesBlocks, "## Session — "+event.Timestamp+"\n\n"+event.SessionNotes)
		}
		state.Timestamp = event.Timestamp
	}

	state.SessionNotes = strings.Join(notesBlocks, "\n\n---\n\n")
	return state
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
