package codegraph

import (
	"strings"
	"testing"
)

// TestScannerExcludesMinBundleSuffixes guards the TS parity contract: the
// built-in EXCLUDED_BUNDLE_SUFFIXES list (.min.js, .bundle.js, .min.mjs,
// .bundle.ts, ...) must keep minified/bundled code out of GetFilesToProcess.
// This was silently dropped when the scan pipeline called the package-level
// isSupported() (extension-only) instead of Scanner.IsSupported().
func TestScannerExcludesMinBundleSuffixes(t *testing.T) {
	tmp := t.TempDir()
	for _, f := range []string{
		"app.min.js",
		"app.bundle.js",
		"lib.min.mjs",
		"lib.bundle.mjs",
		"types.min.ts",
		"types.bundle.ts",
		"comp.min.jsx",
	} {
		writeFile(t, tmp, f, "export const x = 1;")
	}
	writeFile(t, tmp, "keep.js", "export const keep = 1;")
	writeFile(t, tmp, "keep.go", "package m\nfunc Keep() {}\n")

	s := NewScanner(nil, tmp)
	files, err := s.GetFilesToProcess()
	if err != nil {
		t.Fatalf("GetFilesToProcess: %v", err)
	}

	var got []string
	for _, f := range files {
		got = append(got, f.Path)
	}
	for _, f := range got {
		lower := strings.ToLower(f)
		for _, sfx := range []string{".min.js", ".bundle.js", ".min.mjs", ".bundle.mjs", ".min.ts", ".bundle.ts"} {
			if strings.HasSuffix(lower, sfx) {
				t.Errorf("min/bundle file %q leaked into GetFilesToProcess", f)
			}
		}
	}
	// TS contract boundary: EXCLUDED_BUNDLE_SUFFIXES requires a terminal suffix,
	// so comp.min.jsx (suffix .jsx) stays supported — pin that boundary so a
	// future change to suffix matching is intentional.
	if !containsString(got, "comp.min.jsx") {
		t.Errorf("comp.min.jsx should remain supported (TS excludes only terminal .min.*/.bundle.* suffixes), got %v", got)
	}
	if len(got) != 3 {
		t.Errorf("expected keep.js, keep.go and comp.min.jsx, got %v", got)
	}
	if !containsString(got, "keep.js") || !containsString(got, "keep.go") {
		t.Errorf("expected keep.js and keep.go in results, got %v", got)
	}
}

// TestScannerGetDiskFilesExcludesMinBundle guards the TS getDiskFiles() parity:
// TS getDiskFiles() filters f => !isIgnored(f) && isSupported(f), so min/bundle
// files must never appear in the disk listing either.
func TestScannerGetDiskFilesExcludesMinBundle(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "app.min.js", "var a=1;")
	writeFile(t, tmp, "app.bundle.js", "var b=2;")
	writeFile(t, tmp, "keep.go", "package m\nfunc Keep() {}\n")

	s := NewScanner(nil, tmp)
	files, err := s.GetDiskFiles()
	if err != nil {
		t.Fatalf("GetDiskFiles: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f), ".min.js") || strings.HasSuffix(strings.ToLower(f), ".bundle.js") {
			t.Errorf("min/bundle file %q leaked into GetDiskFiles", f)
		}
	}
	if len(files) != 1 || files[0] != "keep.go" {
		t.Errorf("expected only keep.go, got %v", files)
	}
}

// TestScannerCodegraphIgnoreWildcardFromWorkspace proves that a *.min.js /
// *.bundle.js wildcard pattern in the workspace's own .codegraphignore (a
// non-zen-mcp workspace) is honored, including for deeply nested files.
func TestScannerCodegraphIgnoreWildcardFromWorkspace(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, ".codegraphignore", "*.min.js\n*.bundle.js\n*.mjs\n")
	writeFile(t, tmp, "web/public/assets/js/mermaid/chunks/mermaid.core/chunk-3JNJP5BE.mjs", "export const x = 1;")
	writeFile(t, tmp, "web/public/assets/js/lib.min.js", "var a=1;")
	writeFile(t, tmp, "web/public/assets/js/app.bundle.js", "var b=2;")
	writeFile(t, tmp, "src/keep.go", "package m\nfunc Keep() {}\n")

	s := NewScanner(nil, tmp)
	files, err := s.GetFilesToProcess()
	if err != nil {
		t.Fatalf("GetFilesToProcess: %v", err)
	}
	if len(files) != 1 || files[0].Path != "src/keep.go" {
		t.Errorf("expected only src/keep.go to be processed, got %+v", files)
	}
}

// TestScannerCodegraphIgnoreDirPatternFromWorkspace proves a directory pattern
// (e.g. assets/) in the workspace .codegraphignore prunes whole subtrees.
func TestScannerCodegraphIgnoreDirPatternFromWorkspace(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, ".codegraphignore", "assets/\n")
	writeFile(t, tmp, "web/public/assets/js/mermaid/chunks/mermaid.core/chunk-3JNJP5BE.mjs", "export const x = 1;")
	writeFile(t, tmp, "src/keep.go", "package m\nfunc Keep() {}\n")

	s := NewScanner(nil, tmp)
	files, err := s.GetFilesToProcess()
	if err != nil {
		t.Fatalf("GetFilesToProcess: %v", err)
	}
	if len(files) != 1 || files[0].Path != "src/keep.go" {
		t.Errorf("expected only src/keep.go to be processed, got %+v", files)
	}
}

// TestIndexSkipsMinBundle end-to-end: indexing a workspace that contains
// min/bundle files must not create any index nodes for them.
func TestIndexSkipsMinBundle(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "app.min.js", "var x = 1;")
	writeFile(t, tmp, "app.bundle.js", "var y = 2;")
	writeFile(t, tmp, "calc.go", "package m\nfunc Add() {}\n")

	cg, err := NewCodeGraph(tmp)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	res, err := cg.Index()
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if res.Indexed != 1 {
		t.Errorf("expected 1 file indexed, got %d", res.Indexed)
	}

	for _, fr := range cg.storage.GetAllFiles() {
		if strings.Contains(fr.Path, ".min.") || strings.Contains(fr.Path, ".bundle.") {
			t.Errorf("min/bundle file %q was written to the index", fr.Path)
		}
	}
	if cg.storage.GetFileByPath("calc.go") == nil {
		t.Errorf("expected calc.go to be indexed")
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
