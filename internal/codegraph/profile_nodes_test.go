package codegraph

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileNodes(t *testing.T) {
	rootDir := "/media/jang/home/Deve/zen-mcp"
	relPath := "internal/codegraph/parser.go"

	content, err := os.ReadFile(filepath.Join(rootDir, relPath))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	cg, err := NewCodeGraph(rootDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	start := time.Now()
	nodes, relations, err := cg.parser.Parse(filepath.Ext(relPath), content)
	t.Logf("Parse: %v, nodes=%d, relations=%d, err=%v", time.Since(start), len(nodes), len(relations), err)
	if len(nodes) > 0 {
		t.Logf("First node: %s %s (lines %d-%d)", nodes[0].Type, nodes[0].Name, nodes[0].StartLine, nodes[0].EndLine)
	}
	if len(nodes) > 10 {
		t.Logf("Last node: %s %s (lines %d-%d)", nodes[len(nodes)-1].Type, nodes[len(nodes)-1].Name, nodes[len(nodes)-1].StartLine, nodes[len(nodes)-1].EndLine)
	}
}
