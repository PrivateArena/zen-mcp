package codegraph

import (
	"os"
	"path/filepath"
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
