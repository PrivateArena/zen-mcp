package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zen-mcp/internal/codegraph"
	"zen-mcp/internal/mcpcfg"
)

// withMapRegistration points the global project root at a temp dir holding a
// map.json that registers root (and only root), so watcher eligibility checks
// read a controlled registry instead of the real one.
func withMapRegistration(t *testing.T, root string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "map.json"), []byte(`{`+jsonQ(root)+`:{"lastVisited":"2026-01-01T00:00:00.000Z"}}`), 0o644); err != nil {
		t.Fatalf("write map.json: %v", err)
	}
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = dir
	t.Cleanup(func() { mcpcfg.ProjectRoot = old })
}

func jsonQ(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// makeCodegraphDB creates root/.zenmcp/codegraph.db by running one full index.
func makeCodegraphDB(t *testing.T, root string) {
	t.Helper()
	cg, err := codegraph.NewCodeGraph(root)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()
	if _, err := cg.Index(); err != nil {
		t.Fatalf("initial index: %v", err)
	}
}

func TestWatcherEligibilityRequiresDBAndMapRegistration(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", goFixture)

	// No codegraph.db, not registered -> ineligible.
	if isCodegraphCompatible(root) {
		t.Error("ineligible: no codegraph.db, not registered")
	}

	// Registered but no DB -> ineligible.
	withMapRegistration(t, root)
	if isCodegraphCompatible(root) {
		t.Error("ineligible: no codegraph.db even when registered")
	}

	// DB exists but folder not registered -> ineligible.
	makeCodegraphDB(t, root)
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir() // empty registry, nothing registered
	if isCodegraphCompatible(root) {
		t.Error("ineligible: registered in map.json required")
	}
	mcpcfg.ProjectRoot = old

	// DB exists AND registered -> eligible.
	withMapRegistration(t, root)
	if !isCodegraphCompatible(root) {
		t.Error("eligible: codegraph.db exists and folder registered")
	}
}

func TestWatcherRegistrationTrailingSlashMatches(t *testing.T) {
	root := t.TempDir()
	withMapRegistration(t, root)

	// map.json key without trailing slash, query with one, and vice versa.
	if !isRegisteredInMap(root) {
		t.Fatalf("registered root should match")
	}
	if !isRegisteredInMap(root + "/") {
		t.Fatalf("root with trailing slash should match")
	}
	if isRegisteredInMap(t.TempDir()) {
		t.Fatal("unregistered folder must not match")
	}
}

func TestWatcherSkipsIneligibleAndStartsForEligible(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", goFixture)

	cfg := watcherConfig{enabled: true, debounce: 50 * time.Millisecond}
	if w := startCodegraphWatcher(root, root, cfg); w != nil {
		t.Fatal("watcher must not start without codegraph.db + map.json registration")
	}

	withMapRegistration(t, root)
	makeCodegraphDB(t, root)
	w := startCodegraphWatcher(root, root, cfg)
	if w == nil {
		t.Fatal("watcher must start for an eligible root")
	}
	defer w.Stop()

	// Idempotent: second start returns the same watcher, no second fsnotify loop.
	if again := startCodegraphWatcher(root, root, cfg); again != w {
		t.Fatal("startCodegraphWatcher must be idempotent per root")
	}
}

func TestWatcherDisabledReturnsNil(t *testing.T) {
	root := t.TempDir()
	withMapRegistration(t, root)
	makeCodegraphDB(t, root)
	if w := startCodegraphWatcher(root, root, watcherConfig{enabled: false}); w != nil {
		t.Fatal("disabled watcher must not start")
	}
}

func TestWatcherDebouncedIncrementalIndex(t *testing.T) {
	root := t.TempDir()
	withMapRegistration(t, root)
	writeFixture(t, root, "main.go", `package foo
func Alpha() {}
`)
	makeCodegraphDB(t, root)

	w := startCodegraphWatcher(root, root, watcherConfig{
		enabled:  true,
		debounce: 150 * time.Millisecond,
	})
	if w == nil {
		t.Fatal("watcher should start for eligible root")
	}
	defer w.Stop()

	// Add a symbol to an existing indexed file: the watcher must re-index it
	// incrementally after the debounce window.
	writeFixture(t, root, "main.go", `package foo
func Alpha() {}
func Beta() {}
`)

	deadline := time.Now().Add(15 * time.Second)
	for {
		found := symbolIndexed(t, root, "Beta")
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("watcher did not index the new symbol within deadline")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestWatcherIndexesDeletedFiles(t *testing.T) {
	root := t.TempDir()
	withMapRegistration(t, root)
	writeFixture(t, root, "keep.go", `package foo
func Keep() {}
`)
	writeFixture(t, root, "gone.go", `package foo
func Gone() {}
`)
	makeCodegraphDB(t, root)

	w := startCodegraphWatcher(root, root, watcherConfig{
		enabled:  true,
		debounce: 150 * time.Millisecond,
	})
	if w == nil {
		t.Fatal("watcher should start for eligible root")
	}
	defer w.Stop()

	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatalf("remove gone.go: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		if !symbolIndexed(t, root, "Gone") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("watcher did not drop the deleted file's symbols within deadline")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func symbolIndexed(t *testing.T, root, name string) bool {
	t.Helper()
	cg, err := codegraph.NewCodeGraph(root)
	if err != nil {
		t.Fatalf("open graph: %v", err)
	}
	defer cg.Close()
	res, _ := cg.Search(name, 5)
	for _, r := range res {
		if r.Name == name {
			return true
		}
	}
	return false
}

func TestWatcherAutoLintOnlyGitChangedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	oldLookPath := lookPath
	lookPath = func(name string) string {
		if name == "gofmt" {
			return "gofmt"
		}
		return ""
	}
	defer func() { lookPath = oldLookPath }()

	root := t.TempDir()
	withMapRegistration(t, root)
	writeFixture(t, root, "clean.go", "package foo\n")
	writeFixture(t, root, "dirty.go", "package foo\n")
	makeCodegraphDB(t, root)

	// Commit a baseline so git tracks both files.
	git(t, root, "init", "-q")
	git(t, root, "add", ".")
	git(t, root, "-c", "user.email=test@test", "-c", "user.name=test", "commit", "-qm", "baseline")

	// dirty.go becomes unformatted and git-modified; clean.go is left alone.
	writeFixture(t, root, "dirty.go", "package foo\n\n")
	gitChanged, ok := gitChangedFiles(root)
	if !ok {
		t.Fatal("root should be a git repo")
	}
	if !gitChanged["dirty.go"] {
		t.Fatal("dirty.go should be reported by git")
	}
	if gitChanged["clean.go"] {
		t.Fatal("clean.go should not be reported by git")
	}

	w := startCodegraphWatcher(root, root, watcherConfig{enabled: true, debounce: 10 * time.Millisecond, autoLint: true})
	if w == nil {
		t.Fatal("watcher should start")
	}
	defer w.Stop()

	// Lint only the git-modified file; the untouched file must not change.
	w.lintChanged([]string{"clean.go", "dirty.go"})
	if got := readFile(t, filepath.Join(root, "dirty.go")); got != "package foo\n" {
		t.Errorf("git-modified file should have been formatted, got %q", got)
	}
	if got := readFile(t, filepath.Join(root, "clean.go")); got != "package foo\n" {
		t.Errorf("unmodified file must not be touched by auto-lint, got %q", got)
	}
}

func TestLinterSelection(t *testing.T) {
	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()

	lookPath = func(name string) string {
		switch name {
		case "goimports", "gofmt", "rustfmt", "black", "prettier", "clang-format":
			return "/usr/bin/" + name
		}
		return ""
	}

	tests := []struct {
		file string
		want string
	}{
		{"a.go", "goimports"},
		{"b.rs", "rustfmt"},
		{"c.py", "black"},
		{"d.ts", "prettier"},
		{"e.jsx", "prettier"},
		{"f.c", "clang-format"},
		{"g.cpp", "clang-format"},
		{"h.rb", ""},  // standardrb intentionally not faked
		{"i.lua", ""}, // stylua intentionally not faked
		{"README.md", ""},
		{"noext", ""},
	}
	for _, tt := range tests {
		tool, _ := linterFor(tt.file)
		if tool != tt.want {
			t.Errorf("linterFor(%s) = %q, want %q", tt.file, tool, tt.want)
		}
	}

	// goimports missing -> gofmt fallback for Go.
	lookPath = func(name string) string {
		if name == "gofmt" {
			return "/usr/bin/gofmt"
		}
		return ""
	}
	if tool, _ := linterFor("a.go"); tool != "gofmt" {
		t.Errorf("expected gofmt fallback, got %q", tool)
	}

	// No formatter installed -> graceful skip.
	lookPath = func(name string) string { return "" }
	if tool, _ := linterFor("a.go"); tool != "" {
		t.Errorf("expected empty linter when nothing installed, got %q", tool)
	}
}

func TestBestGraphRootRouting(t *testing.T) {
	root := "/repo"
	roots := []string{root, root + "/www", root + "/www/sub"}
	tests := []struct {
		full string
		want string
	}{
		{root + "/main.go", root},
		{root + "/www/handler.go", root + "/www"},
		{root + "/www/sub/deep.go", root + "/www/sub"},
		{root + "/outside/x.go", root},
		{"/repo-other/x.go", ""},
		{"/repo2/x.go", ""},
	}
	for _, tt := range tests {
		if got := bestGraphRootFor(tt.full, roots); got != tt.want {
			t.Errorf("bestGraphRootFor(%s) = %q, want %q", tt.full, got, tt.want)
		}
	}
}

func TestWatcherIgnoresNonSourceAndIgnoredPaths(t *testing.T) {
	root := t.TempDir()
	w := &codegraphWatcher{root: root, ignore: watcherIgnoreSet()}

	if isSupportedWatcherFile("main.go") != true {
		t.Error(".go should be watchable")
	}
	if isSupportedWatcherFile("notes.txt") != false {
		t.Error("non-source files must be ignored")
	}
	if isSupportedWatcherFile(".min.js") != false {
		t.Error("minified files must be ignored")
	}
	if !isSkippedPath("node_modules/pkg/a.go", w.ignore) {
		t.Error("node_modules paths must be skipped")
	}
	if isSkippedPath("src/util.go", w.ignore) {
		t.Error("normal source paths must not be skipped")
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
