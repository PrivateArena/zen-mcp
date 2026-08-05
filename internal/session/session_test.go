package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jang/zen-mcp/internal/mcpcfg"
	"github.com/jang/zen-mcp/internal/shared"
)

func setupSessionDir(t *testing.T) (cwd string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	oldRoot := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = dir
	cleanup = func() {
		mcpcfg.ProjectRoot = oldRoot
	}
	return dir, cleanup
}

func writeMap(t *testing.T, cwd string, keys []string) {
	t.Helper()
	var content string
	content = "{\n"
	for i, k := range keys {
		comma := ","
		if i == len(keys)-1 {
			comma = ""
		}
		content += "  " + "\"" + k + "\": {}" + comma + "\n"
	}
	content += "}\n"
	if err := os.WriteFile(filepath.Join(cwd, "map.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSaveLoadSessionState(t *testing.T) {
	dir, cleanup := setupSessionDir(t)
	defer cleanup()

	real := filepath.Join(dir, "real-project")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}

	m := newWithCwd(shared.NewStore(), dir)
	m.SetSessionWorkspaceRoot("s1", real)
	m.UpdateSessionActivity("s1")
	m.SetLastActiveSessionId("s1")

	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".zenmcp", "sessions.json")); err != nil {
		t.Fatalf("session file not written: %v", err)
	}

	// fresh manager loads the file
	m2 := newWithCwd(shared.NewStore(), dir)
	if err := m2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := m2.GetSessionWorkspaceRoot("s1"); got != real {
		t.Errorf("restored workspace = %q, want %q", got, real)
	}
	if m2.GetLastActiveSessionId() != "" {
		t.Error("lastActiveSessionId should not be persisted")
	}
}

func TestGetActiveWorkspaceRootChain(t *testing.T) {
	dir, cleanup := setupSessionDir(t)
	defer cleanup()

	real := filepath.Join(dir, "proj")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	st := shared.NewStore()
	m := newWithCwd(st, dir)

	// 1. last active session
	m.SetSessionWorkspaceRoot("active", real)
	m.SetLastActiveSessionId("active")
	if got := m.GetActiveWorkspaceRoot(); got != real {
		t.Errorf("last-active chain: got %q want %q", got, real)
	}

	// 2. persistent session (no last-active)
	m2 := newWithCwd(st, dir)
	m2.SetSessionWorkspaceRoot("zen-persistent-session", real)
	if got := m2.GetActiveWorkspaceRoot(); got != real {
		t.Errorf("persistent chain: got %q want %q", got, real)
	}

	// 3. shared workspace-root
	st.Set("workspace-root", filepath.Join(dir, "shared"))
	m3 := newWithCwd(st, dir)
	if got := m3.GetActiveWorkspaceRoot(); got != filepath.Join(dir, "shared") {
		t.Errorf("shared chain: got %q", got)
	}

	// 4. env
	st.Set("workspace-root", "")
	t.Setenv("MCP_WORKSPACE_ROOT", filepath.Join(dir, "env"))
	m4 := newWithCwd(st, dir)
	if got := m4.GetActiveWorkspaceRoot(); got != filepath.Join(dir, "env") {
		t.Errorf("env chain: got %q", got)
	}
}

func TestSetSessionWorkspaceRootOnlyExists(t *testing.T) {
	dir, cleanup := setupSessionDir(t)
	defer cleanup()

	real := filepath.Join(dir, "exists")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	m := newWithCwd(shared.NewStore(), dir)
	m.SetSessionWorkspaceRoot("s1", filepath.Join(dir, "missing"))
	if got := m.GetSessionWorkspaceRoot("s1"); got != "" {
		t.Errorf("nonexistent path should not be stored, got %q", got)
	}
	m.SetSessionWorkspaceRoot("s1", real)
	if got := m.GetSessionWorkspaceRoot("s1"); got != real {
		t.Errorf("existing path should be stored, got %q", got)
	}
}

func TestPathResolverAliases(t *testing.T) {
	dir, cleanup := setupSessionDir(t)
	defer cleanup()

	proj := filepath.Join(dir, "my-project")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMap(t, dir, []string{proj})

	m := newWithCwd(shared.NewStore(), dir)

	// exact alias by base name
	if got, ok := m.ResolveWorkspacePath("my-project"); !ok || got != proj {
		t.Errorf("alias by base: %q,%v want %q", got, ok, proj)
	}
	// exact full path
	if got, ok := m.ResolveWorkspacePath(proj); !ok || got != proj {
		t.Errorf("alias by full path: %q,%v", got, ok)
	}
	// prefix match by basename
	if got, ok := m.ResolveWorkspacePath("my-proj"); !ok || got != proj {
		t.Errorf("prefix match: %q,%v", got, ok)
	}
	// token match
	if got, ok := m.ResolveWorkspacePath("project"); !ok || got != proj {
		t.Errorf("token match: %q,%v", got, ok)
	}
}

func TestSaveSortsByActivity(t *testing.T) {
	dir, cleanup := setupSessionDir(t)
	defer cleanup()

	m := newWithCwd(shared.NewStore(), dir)
	now := time.Now()
	m.sessionTimestamps["old"] = now.Add(-time.Hour)
	m.sessionTimestamps["new"] = now
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	m2 := newWithCwd(shared.NewStore(), dir)
	_ = m2.Load()
	known := m2.KnownSessions()
	if len(known) != 2 {
		t.Fatalf("expected 2 known sessions, got %v", known)
	}
}

func TestPathResolverTokenize(t *testing.T) {
	p := NewPathResolver(map[string]string{})
	got := p.tokenize("Foo-Bar_baz/qux")
	want := []string{"foo", "bar", "baz", "qux"}
	if len(got) != len(want) {
		t.Fatalf("tokenize = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tokenize[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
