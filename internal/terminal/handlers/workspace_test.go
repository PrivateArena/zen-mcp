package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/shared"
	"zen-mcp/internal/terminal"
	"zen-mcp/internal/tools"
)

func setupCdTest(t *testing.T) (string, string, func() string, *shared.Store) {
	t.Helper()
	dir := t.TempDir()
	projA := filepath.Join(dir, "zen-mcp")
	projB := filepath.Join(dir, "web-reader-mcp-master")
	for _, d := range []string{projA, projB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = dir
	t.Cleanup(func() { mcpcfg.ProjectRoot = old })

	body := "{\n  \"" + projA + "\": {},\n  \"" + projB + "\": {}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "map.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	store := shared.NewStore()
	terminal.SetDeps(tools.Deps{Store: store})

	var buf strings.Builder
	oldOut := terminal.LogOut
	terminal.LogOut = &buf
	t.Cleanup(func() { terminal.LogOut = oldOut })

	return projA, projB, func() string { return buf.String() }, store
}

func TestCdResolvesFuzzyToFullPath(t *testing.T) {
	projA, projB, _, store := setupCdTest(t)

	if err := cd([]string{"zen-mcp"}); err != nil {
		t.Fatalf("cd(zen-mcp) error = %v", err)
	}
	if v, _ := store.Get("workspace-root"); v != projA {
		t.Errorf("workspace-root = %q, want %q", v, projA)
	}

	// multi-word input: "server mcp" -> tie between projA and projB,
	// deterministic winner is the lexicographically-first path.
	if err := cd([]string{"server", "mcp"}); err != nil {
		t.Fatalf("cd(server mcp) error = %v", err)
	}
	if v, _ := store.Get("workspace-root"); v != projB {
		t.Errorf("workspace-root = %q, want %q", v, projB)
	}
}

func TestCdNonExistentKeepsCurrent(t *testing.T) {
	_, _, log, store := setupCdTest(t)

	store.Set("workspace-root", "/keep-me")
	if err := cd([]string{"no-such-workspace"}); err != nil {
		t.Fatalf("cd(nonexistent) error = %v", err)
	}
	if v, _ := store.Get("workspace-root"); v != "/keep-me" {
		t.Errorf("workspace-root = %q, want /keep-me", v)
	}
	if !strings.Contains(log(), "does not exist") {
		t.Errorf("missing ERROR log: %q", log())
	}
}

func TestCdForceRequiresExistingPath(t *testing.T) {
	_, _, _, store := setupCdTest(t)

	err := cd([]string{"--force", "no-such-workspace"})
	if err == nil {
		t.Fatal("cd --force <nonexistent> should error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want contains 'does not exist'", err)
	}
	if v, _ := store.Get("workspace-root"); v != "" {
		t.Errorf("workspace-root = %q, want unset", v)
	}
}

func TestCdForceSetsWorkspace(t *testing.T) {
	projA, _, log, store := setupCdTest(t)

	if err := cd([]string{"--force", "zen-mcp"}); err != nil {
		t.Fatalf("cd --force zen-mcp error = %v", err)
	}
	if v, _ := store.Get("workspace-root"); v != projA {
		t.Errorf("workspace-root = %q, want %q", v, projA)
	}
	if !strings.Contains(log(), "Reload config.json") {
		t.Errorf("missing reload hint: %q", log())
	}
}

func TestCdNoArgsErrors(t *testing.T) {
	setupCdTest(t)
	if err := cd([]string{}); err == nil {
		t.Fatal("cd with no args should error")
	}
}
