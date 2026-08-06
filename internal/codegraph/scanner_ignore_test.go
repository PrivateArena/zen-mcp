package codegraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestCodegraphIgnoreMatchesGitignoreSemantics(t *testing.T) {
	tmp := t.TempDir()

	writeFile(t, tmp, "src/pkg/a.go", "package pkg\nfunc A() {}\n")
	writeFile(t, tmp, "src/pkg/a_test.go", "package pkg\nfunc TestA(t) {}\n")
	writeFile(t, tmp, "run.log", "log\n")
	writeFile(t, tmp, "vendor/x.go", "package x\n")
	writeFile(t, tmp, ".codegraphignore", "*_test.go\nvendored/\n")
	writeFile(t, tmp, ".gitignore", "*.log\n")

	s := NewScanner(nil, tmp)

	cases := []struct {
		path string
		dir  bool
		want bool
	}{
		{"src/pkg/a_test.go", false, true},
		{"src/pkg/a.go", false, false},
		{"run.log", false, true},
		{"vendored", true, true},
		{"vendored/x.go", false, true},
		{"src/pkg/b.go", false, false},
	}
	for _, c := range cases {
		if got := s.IsIgnored(c.path, c.dir); got != c.want {
			t.Errorf("IsIgnored(%q, dir=%v) = %v, want %v", c.path, c.dir, got, c.want)
		}
	}
}

func TestScannerGetDiskFilesHonorsCodegraphIgnore(t *testing.T) {
	tmp := t.TempDir()

	writeFile(t, tmp, "keep.go", "package m\nfunc Keep() {}\n")
	writeFile(t, tmp, "drop_test.go", "package m\nfunc TestDrop(t) {}\n")
	writeFile(t, tmp, "generated/gen.go", "package gen\n")
	writeFile(t, tmp, ".codegraphignore", "*_test.go\ngenerated/\n")

	s := NewScanner(nil, tmp)
	files, err := s.GetDiskFiles()
	if err != nil {
		t.Fatalf("GetDiskFiles: %v", err)
	}

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			t.Errorf("ignored file still present in disk files: %s", f)
		}
		if strings.HasPrefix(f, "generated/") {
			t.Errorf("ignored dir file still present in disk files: %s", f)
		}
	}

	// The ignored entries must be absent and the kept one present.
	var got []string
	for _, f := range files {
		got = append(got, f)
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "keep.go") {
		t.Errorf("expected keep.go in disk files, got %v", got)
	}
	if strings.Contains(joined, "drop_test.go") {
		t.Errorf("expected drop_test.go to be ignored, got %v", got)
	}
	if strings.Contains(joined, "generated/gen.go") {
		t.Errorf("expected generated/gen.go to be ignored, got %v", got)
	}
}
