package projectmemory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jang/zen-mcp/internal/mcpcfg"
)

func TestNormalizeKey(t *testing.T) {
	if got := NormalizeKey("Hello World!"); got != "hello world" {
		t.Errorf("NormalizeKey() = %q, want %q", got, "hello world")
	}
}

func TestMigrateToV3(t *testing.T) {
	raw := map[string]any{"schema_version": 2, "timestamp": "2024-01-01T00:00:00.000Z", "session_title": "T", "objective": "O", "session_notes": "N"}
	ev := MigrateToV3(raw)
	if ev.SessionTitle != "T" || ev.Objective != "O" {
		t.Errorf("MigrateToV3() = %+v, want T/O", ev)
	}
	if ev.SchemaVersion != 3 {
		t.Errorf("MigrateToV3() schema_version = %d, want 3", ev.SchemaVersion)
	}
}

func TestAppendEventAndReconstruct(t *testing.T) {
	dir := t.TempDir()
	memName := "brain"

	e1 := BrainEvent{SchemaVersion: 3, Timestamp: "2024-01-01T00:00:00.000Z", SessionTitle: "T1", Objective: "O1", SessionNotes: "N1"}
	if err := AppendEvent(dir, memName, e1); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	e2 := BrainEvent{SchemaVersion: 3, Timestamp: "2024-01-02T00:00:00.000Z", SessionTitle: "T2", Objective: "O2", SessionNotes: "N2"}
	if err := AppendEvent(dir, memName, e2); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	state := ReconstructState(dir, memName)
	if state.SessionTitle != "T2" {
		t.Errorf("ReconstructState() SessionTitle = %q, want %q", state.SessionTitle, "T2")
	}
	if state.Objective != "O2" {
		t.Errorf("ReconstructState() Objective = %q, want %q", state.Objective, "O2")
	}
	if state.SchemaVersion != 3 {
		t.Errorf("ReconstructState() SchemaVersion = %d, want 3", state.SchemaVersion)
	}
}

func TestReconstructStateEmpty(t *testing.T) {
	dir := t.TempDir()
	state := ReconstructState(dir, "brain")
	if state.SessionTitle != "" || state.Objective != "" {
		t.Errorf("ReconstructState(empty) = %+v, want empty", state)
	}
}

func TestJSONToMarkdown(t *testing.T) {
	raw := `{"schema_version":3,"timestamp":"2024-01-01T00:00:00.000Z","session_title":"Port memory","objective":"be handler","session_notes":"## Progress\n- Done\n- Pending"}`
	got := JSONToMarkdown(raw)
	want := "**schema_version**: 3\n**timestamp**: 2024-01-01T00:00:00.000Z\n**session_title**: Port memory\n**objective**: be handler\n**session_notes**: ## Progress\n- Done\n- Pending"
	if got != want {
		t.Errorf("JSONToMarkdown()\n got: %q\nwant: %q", got, want)
	}
}

func TestJSONToMarkdownNested(t *testing.T) {
	raw := `{"tasks":[{"name":"a","steps":["s1",1]},true],"env":{"k":"v","n":2,"flag":false},"nullv":null}`
	got := JSONToMarkdown(raw)
	want := "## tasks\n  -\n    **name**: a\n    ## steps\n      - s1\n      - 1\n  - true\n## env\n  **k**: v\n  **n**: 2\n  **flag**: false\n**nullv**: null"
	if got != want {
		t.Errorf("JSONToMarkdown(nested)\n got: %q\nwant: %q", got, want)
	}
}

func TestJSONToMarkdownStringInput(t *testing.T) {
	if got := JSONToMarkdown(`"plain"`); got != "plain" {
		t.Errorf("JSONToMarkdown(string) = %q, want %q", got, "plain")
	}
}

func TestRegisterProjectInMap(t *testing.T) {
	dir := t.TempDir()
	mapFile := filepath.Join(dir, "map.json")

	projectPath := filepath.Join(dir, "ws")
	zenDir := filepath.Join(projectPath, ".zenmcp")
	_ = os.MkdirAll(zenDir, 0o755)

	first := filepath.Join(dir, "first")
	_ = os.MkdirAll(filepath.Join(first, ".zenmcp"), 0o755)

	origRoot := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = dir
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	RegisterProjectInMap(first, nil)
	RegisterProjectInMap(projectPath, nil)

	raw, err := os.ReadFile(mapFile)
	if err != nil {
		t.Fatalf("RegisterProjectInMap() failed to write map: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("RegisterProjectInMap() invalid JSON: %v", err)
	}
	if _, ok := data[projectPath]; !ok {
		t.Errorf("RegisterProjectInMap() missing project entry in %s", string(raw))
	}
	wsPos := strings.Index(string(raw), "\""+projectPath+"\"")
	firstPos := strings.Index(string(raw), "\""+first+"\"")
	if wsPos < 0 || firstPos < 0 || wsPos > firstPos {
		t.Errorf("RegisterProjectInMap() recent key not moved to top, got:\n%s", string(raw))
	}
	wsTS := data[projectPath].(map[string]any)["lastVisited"].(string)
	firstTS := data[first].(map[string]any)["lastVisited"].(string)
	want := "{\n  \"" + projectPath + "\": {\n    \"lastVisited\": \"" + wsTS +
		"\"\n  },\n  \"" + first + "\": {\n    \"lastVisited\": \"" + firstTS + "\"\n  }\n}"
	if string(raw) != want {
		t.Errorf("RegisterProjectInMap() output not canonically indented, got:\n%s\nwant:\n%s", string(raw), want)
	}
}
