package codegraph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkIndex(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		tmpDir := b.TempDir()

		src := []byte("package foo\n\nfunc Add(a int, b int) int {\n\treturn a + b\n}\n\nfunc mul(x, y int) int {\n\treturn x * y\n}\n")
		if err := os.WriteFile(filepath.Join(tmpDir, "calc.go"), src, 0644); err != nil {
			b.Fatalf("write fixture: %v", err)
		}

		cg, err := NewCodeGraph(tmpDir)
		if err != nil {
			b.Fatalf("NewCodeGraph: %v", err)
		}
		_, _ = cg.Index()
		cg.Close()
	}
}

func BenchmarkIndexScanner(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		tmpDir := b.TempDir()

		src := []byte("package foo\n\nfunc Add(a int, b int) int {\n\treturn a + b\n}\n")
		if err := os.WriteFile(filepath.Join(tmpDir, "calc.go"), src, 0644); err != nil {
			b.Fatalf("write fixture: %v", err)
		}

		cg, err := NewCodeGraph(tmpDir)
		if err != nil {
			b.Fatalf("NewCodeGraph: %v", err)
		}
		_, _ = cg.scanner.GetFilesToProcess()
		cg.Close()
	}
}

func BenchmarkIndexParse(b *testing.B) {
	tmpDir := b.TempDir()

	src := []byte("package foo\n\nfunc Add(a int, b int) int {\n\treturn a + b\n}\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "calc.go"), src, 0644); err != nil {
		b.Fatalf("write fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		b.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	files, _ := cg.scanner.GetFilesToProcess()
	content, _ := os.ReadFile(filepath.Join(tmpDir, files[0].Path))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = cg.parser.Parse(filepath.Ext(files[0].Path), content)
	}
}

func BenchmarkIndexDBWrite(b *testing.B) {
	tmpDir := b.TempDir()

	src := []byte("package foo\n\nfunc Add(a int, b int) int {\n\treturn a + b\n}\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "calc.go"), src, 0644); err != nil {
		b.Fatalf("write fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		b.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	files, _ := cg.scanner.GetFilesToProcess()
	content, _ := os.ReadFile(filepath.Join(tmpDir, files[0].Path))
	nodes, _, _ := cg.parser.Parse(filepath.Ext(files[0].Path), content)
	fr := files[0]
	fileID, _ := cg.storage.UpsertFile(fr)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = cg.storage.DeleteNodesForFile(fileID)
		_ = cg.storage.DeleteEdgesForFile(fileID)
		for _, n := range nodes {
			nr := NodeRecord{
				FileID:        fileID,
				Type:          n.Type,
				Name:          n.Name,
				Language:      fr.Language,
				QualifiedName: *n.QualifiedName,
				Signature:     n.Signature,
				Docstring:     n.Docstring,
				StartLine:     n.StartLine,
				EndLine:       n.EndLine,
				Content:       n.Content,
			}
			_, _ = cg.storage.InsertNode(nr)
		}
	}
}

func BenchmarkIndexMultiFile(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		tmpDir := b.TempDir()

		for j := 0; j < 50; j++ {
			src := []byte("package foo\n\nfunc Add" + string(rune('0'+j)) + "(a int, b int) int {\n\treturn a + b\n}\n")
			os.WriteFile(filepath.Join(tmpDir, "calc"+string(rune('0'+j))+".go"), src, 0644)
		}

		cg, err := NewCodeGraph(tmpDir)
		if err != nil {
			b.Fatalf("NewCodeGraph: %v", err)
		}
		_, _ = cg.Index()
		cg.Close()
	}
}

// writeRelationDenseRepo writes a synthetic Go project whose relation profile
// mirrors a real repo: many files, functions calling a shared set of internal
// symbols (which resolve and emit edges) plus a large set of unique external
// symbols (e.g. stdlib names) that never exist in the index. The latter are
// the worst case for per-name resolution: every one is a distinct database
// miss, and only the in-memory node index can avoid querying for all of them.
func writeRelationDenseRepo(tb testing.TB) string {
	tb.Helper()
	tmpDir := tb.TempDir()

	var shared strings.Builder
	shared.WriteString("package p\n\n")
	for k := 0; k < 10; k++ {
		fmt.Fprintf(&shared, "func Shared%d() {}\n\n", k)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "shared.go"), []byte(shared.String()), 0644); err != nil {
		tb.Fatalf("write shared fixture: %v", err)
	}

	for i := 0; i < 60; i++ {
		var sb strings.Builder
		fmt.Fprintf(&sb, "package p\n\n")
		for j := 0; j < 40; j++ {
			fmt.Fprintf(&sb, "func Fn%d_%d() {\n", i, j)
			for k := 0; k < 6; k++ {
				// Unique external reference that cannot resolve in the index.
				fmt.Fprintf(&sb, "\tExt%d_%d_%d()\n", i, j, k)
			}
			// One shared reference that resolves and emits an edge.
			fmt.Fprintf(&sb, "\tShared%d()\n", j%10)
			sb.WriteString("}\n\n")
		}
		if err := os.WriteFile(filepath.Join(tmpDir, fmt.Sprintf("f%02d.go", i)), []byte(sb.String()), 0644); err != nil {
			tb.Fatalf("write fixture: %v", err)
		}
	}
	return tmpDir
}

// TestProfileRelationDense reports the end-to-end index time for a synthetic
// relation-heavy repo so Phase 3 resolution costs can be measured without
// touching a real workspace database.
func TestProfileRelationDense(t *testing.T) {
	tmpDir := writeRelationDenseRepo(t)

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	result, err := cg.Index()
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	t.Logf("Index total: %d files indexed, %d total; see [CodeGraph] phase timings above", result.Indexed, result.Total)
}

func BenchmarkIndexRelationDense(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cg, err := NewCodeGraph(writeRelationDenseRepo(b))
		if err != nil {
			b.Fatalf("NewCodeGraph: %v", err)
		}
		_, _ = cg.Index()
		cg.Close()
	}
}
