package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"zen-mcp/internal/mcpcfg"
)

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// TestPathResolverCharacterization pins the observable resolution contract of
// PathResolver.Resolve after the move from internal/tools/workspaceresolver.go.
// Every case must resolve exactly as the pre-move implementation did.
func TestPathResolverCharacterization(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "zen-mcp")
	b := filepath.Join(root, "web-reader-mcp-master")
	c := filepath.Join(root, "zen-webserver")
	sub := filepath.Join(root, "sub")
	mkdirs(t, a, b, c, sub)

	alias := map[string]string{
		a: a, "zen-mcp": a,
		b: b, "web-reader-mcp-master": b,
		c: c, "zen-webserver": c,
	}
	p := NewPathResolver(alias, root)

	cases := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{"exact-full-path", a, a, true},
		{"exact-base-name", "zen-mcp", a, true},
		{"prefix-match", "zen-w", c, true},
		{"substring-match", "reader", b, true},
		{"token-match-dashed", "reader-mcp", b, true},
		{"absolute-path-not-in-alias", sub, sub, true},
		{"relative-path-from-cwd", "sub", sub, true},
		{"no-match", "nonexistent-thing", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p.Resolve(tc.input)
			if ok != tc.ok || got != tc.want {
				t.Errorf("Resolve(%q) = %q,%v want %q,%v", tc.input, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestPathResolverMultiWord is the new capability: space-separated queries must
// tokenize so `server mcp`-style input resolves to a registered full path.
func TestPathResolverMultiWord(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "zen-mcp")
	b := filepath.Join(root, "web-reader-mcp-master")
	c := filepath.Join(root, "zen-webserver")
	mkdirs(t, a, b, c)

	alias := map[string]string{
		a: a, "zen-mcp": a,
		b: b, "web-reader-mcp-master": b,
		c: c, "zen-webserver": c,
	}
	p := NewPathResolver(alias, root)

	if got, ok := p.Resolve("zen mcp"); !ok || got != a {
		t.Errorf("Resolve(%q) = %q,%v want %q,true", "zen mcp", got, ok, a)
	}
	if got, ok := p.Resolve("reader mcp"); !ok || got != b {
		t.Errorf("Resolve(%q) = %q,%v want %q,true", "reader mcp", got, ok, b)
	}
}

// TestPathResolverTieBreak is the new determinism guarantee: equal-scoring
// candidates resolve to the lexicographically-first existing path instead of a
// map-iteration-random winner.
func TestPathResolverTieBreak(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "zen-mcp")
	b := filepath.Join(root, "web-reader-mcp-master")
	mkdirs(t, a, b)

	alias := map[string]string{
		a: a, "zen-mcp": a,
		b: b, "web-reader-mcp-master": b,
	}
	p := NewPathResolver(alias, root)

	// "server mcp" matches only the "mcp" token on both a and b -> tie,
	// broken deterministically by lexicographic path order (b < a).
	for i := 0; i < 10; i++ {
		got, ok := p.Resolve("server mcp")
		if !ok || got != b {
			t.Errorf("Resolve(%q) iteration %d = %q,%v want %q,true", "server mcp", i, got, ok, b)
		}
	}
}

func TestLoadAliasMap(t *testing.T) {
	dir := t.TempDir()
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = dir
	defer func() { mcpcfg.ProjectRoot = old }()

	mapPath := filepath.Join(dir, "map.json")
	body := `{
  "/a/foo-bar": {"lastVisited": "x"},
  "/b/baz": {"lastVisited": "y"},
  "/c/foo-bar": {"lastVisited": "z"}
}`
	if err := os.WriteFile(mapPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got := LoadAliasMap()
	want := map[string]string{
		"/a/foo-bar": "/a/foo-bar",
		"/b/baz":     "/b/baz",
		"/c/foo-bar": "/c/foo-bar",
		"foo-bar":    "/c/foo-bar", // duplicate base -> last registered path wins
		"baz":        "/b/baz",
	}
	if len(got) != len(want) {
		t.Errorf("LoadAliasMap() has %d entries, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("LoadAliasMap()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestLoadAliasMapMissingFile(t *testing.T) {
	dir := t.TempDir()
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = dir
	defer func() { mcpcfg.ProjectRoot = old }()

	got := LoadAliasMap()
	if got == nil || len(got) != 0 {
		t.Errorf("LoadAliasMap() with no map.json = %v, want empty non-nil map", got)
	}
}
