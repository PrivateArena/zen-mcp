package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zen-mcp/internal/shared"
	"zen-mcp/internal/terminal"
	"zen-mcp/internal/tools"
)

func TestBrainExtractSavesMarkdown(t *testing.T) {
	dir := t.TempDir()
	zen := filepath.Join(dir, ".zenmcp")
	if err := os.MkdirAll(zen, 0o755); err != nil {
		t.Fatal(err)
	}
	tl := filepath.Join(zen, "brain_timeline.jsonl")
	lines := []string{
		`{"schema_version":3,"timestamp":"2024-01-01T00:00:00.000Z","title":"One","objective":"gamma alpha","notes":"first"}`,
		`{"schema_version":3,"timestamp":"2024-01-02T00:00:00.000Z","title":"Two","objective":"alpha beta","notes":"second"}`,
	}
	if err := os.WriteFile(tl, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	store := shared.NewStore()
	store.Set("workspace-root", dir)
	terminal.SetDeps(tools.Deps{Store: store})

	var buf strings.Builder
	old := terminal.LogOut
	terminal.LogOut = &buf
	defer func() { terminal.LogOut = old }()

	if err := brainExtract([]string{"alpha", "beta"}); err != nil {
		t.Fatalf("brainExtract error = %v", err)
	}

	if !strings.Contains(buf.String(), "OK: Brain extract saved to") {
		t.Errorf("missing OK log: %q", buf.String())
	}

	outPath := filepath.Join(zen, "brain", "ALPHA_BETA.md")
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("brain extract not written: %v", err)
	}
	if !strings.Contains(string(got), "**title**: Two") {
		t.Errorf("expected highest-scoring entry selected, got:\n%s", got)
	}
	if !strings.Contains(string(got), "**objective**: alpha beta") {
		t.Errorf("markdown content wrong:\n%s", got)
	}
}

func TestBrainExtractNoMatch(t *testing.T) {
	dir := t.TempDir()
	zen := filepath.Join(dir, ".zenmcp")
	if err := os.MkdirAll(zen, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zen, "brain_timeline.jsonl"), []byte(`{"schema_version":3,"timestamp":"2024-01-01T00:00:00.000Z","title":"One","objective":"gamma","notes":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store := shared.NewStore()
	store.Set("workspace-root", dir)
	terminal.SetDeps(tools.Deps{Store: store})

	var buf strings.Builder
	old := terminal.LogOut
	terminal.LogOut = &buf
	defer func() { terminal.LogOut = old }()

	if err := brainExtract([]string{"zzz"}); err != nil {
		t.Fatalf("brainExtract error = %v", err)
	}
	if !strings.Contains(buf.String(), "RESULT: No matching brain entries found.") {
		t.Errorf("missing no-match log: %q", buf.String())
	}
	if _, err := os.Stat(filepath.Join(zen, "brain")); !os.IsNotExist(err) {
		t.Errorf("brain dir should not be created on no match")
	}
}

func TestBrainExtractMissingTimeline(t *testing.T) {
	dir := t.TempDir()
	store := shared.NewStore()
	store.Set("workspace-root", dir)
	terminal.SetDeps(tools.Deps{Store: store})

	var buf strings.Builder
	old := terminal.LogOut
	terminal.LogOut = &buf
	defer func() { terminal.LogOut = old }()

	if err := brainExtract([]string{"foo"}); err != nil {
		t.Fatalf("brainExtract error = %v", err)
	}
	if !strings.Contains(buf.String(), "ERROR: No brain_timeline.jsonl found in workspace") {
		t.Errorf("missing error log: %q", buf.String())
	}
}
