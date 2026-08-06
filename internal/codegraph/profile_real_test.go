package codegraph

import (
	"testing"
	"time"
)

func TestProfileRealProject(t *testing.T) {
	rootDir := "/media/jang/home/Deve/zen-mcp"

	start := time.Now()
	cg, err := NewCodeGraph(rootDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()
	t.Logf("NewCodeGraph: %v", time.Since(start))

	files, err := cg.scanner.GetFilesToProcess()
	if err != nil {
		t.Fatalf("GetFilesToProcess: %v", err)
	}
	t.Logf("Files to process: %d", len(files))

	start = time.Now()
	result, err := cg.Index()
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	t.Logf("Index: %v, result=%+v", time.Since(start), result)
}
