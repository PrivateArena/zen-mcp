package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/shared"
)

func cwd(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestResolveWorkspacePriority is the exact contract previously exercised by
// internal/shared/state_test.go; it moves with ResolveWorkspace unchanged.
func TestResolveWorkspacePriority(t *testing.T) {
	cwd := cwd(t)
	cases := []struct {
		name         string
		input        string
		registration string
		store        func() *shared.Store
		env          string
		want         string
	}{
		{
			name:  "1-explicit-input-wins",
			input: "/explicit",
			want:  "/explicit",
		},
		{
			name:         "2-registration-next",
			input:        "",
			registration: "/reg-ws",
			want:         "/reg-ws",
		},
		{
			name:  "3-shared-workspace-root",
			input: "",
			store: func() *shared.Store {
				st := shared.NewStore()
				st.Set("workspace-root", "/shared-ws")
				return st
			},
			want: "/shared-ws",
		},
		{
			name:  "4-env-workspace-root",
			input: "",
			store: func() *shared.Store { return shared.NewStore() },
			env:   "/env-ws",
			want:  "/env-ws",
		},
		{
			name:  "5-cwd-fallback",
			input: "",
			store: func() *shared.Store { return shared.NewStore() },
			want:  cwd,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MCP_WORKSPACE_ROOT", tc.env)
			var st *shared.Store
			if tc.store != nil {
				st = tc.store()
			}
			got := ResolveWorkspace(tc.input, tc.registration, st)
			if got != tc.want {
				t.Errorf("ResolveWorkspace = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveWorkspacePath(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "zen-mcp")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = dir
	defer func() { mcpcfg.ProjectRoot = old }()
	mapPath := filepath.Join(dir, "map.json")
	if err := os.WriteFile(mapPath, []byte("{\n  \""+proj+"\": {}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		input string
		cwd   string
		want  string
	}{
		{"resolves-fuzzy-base-via-alias", "zen-mcp", dir, proj},
		{"resolves-absolute-directly", proj, dir, proj},
		{"empty-input", "", dir, ""},
		{"fallback-joins-cwd", "sub", dir, filepath.Join(dir, "sub")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveWorkspacePath(tc.input, tc.cwd); got != tc.want {
				t.Errorf("ResolveWorkspacePath(%q,%q) = %q, want %q", tc.input, tc.cwd, got, tc.want)
			}
		})
	}
}
