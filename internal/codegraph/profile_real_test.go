package codegraph

import (
	"os"
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

// TestProfileGodotProject indexes a real Godot 4 project (175 GDScript files, two of
// them over 1 MB) to keep the gdscript plugin honest about parse cost and output size.
// Skipped when the corpus is not on this machine.
func TestProfileGodotProject(t *testing.T) {
	rootDir := "/media/jang/free/MidgardSurvivors/zen-rea/work/60_patches/effective_0.2.7"
	if _, err := os.Stat(rootDir); err != nil {
		t.Skip("Godot corpus not present")
	}

	start := time.Now()
	cg, err := NewCodeGraph(rootDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

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

	nodes, err := cg.storage.GetAllNodes()
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	byType := map[string]int{}
	for _, n := range nodes {
		byType[n.Type]++
	}
	t.Logf("Symbols by kind: %v", byType)

	for _, want := range []string{"class", "function", "signal", "constant", "autoload"} {
		if byType[want] == 0 {
			t.Errorf("no %s symbols indexed from the Godot project", want)
		}
	}
}
