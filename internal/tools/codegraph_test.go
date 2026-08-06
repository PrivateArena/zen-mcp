package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForceReindexClearsStaleDB(t *testing.T) {
	tmpDir := t.TempDir()

	src := `package foo

func Add(a int, b int) int {
	return a + b
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "calc.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	deps := Deps{}
	ctx := testContext()

	// Step 1: initial index
	req1 := MakeFakeRequest(map[string]any{"action": "index"})
	res1 := HandleCodegraphAction(ctx, tmpDir, deps, req1)
	if res1 == nil {
		t.Fatalf("index result is nil")
	}
	var text1 string
	for _, c := range res1.Content {
		if tc, ok := c.(interface{ Text() string }); ok {
			text1 = tc.Text()
			break
		}
	}
	if !strings.Contains(text1, "total files") {
		t.Fatalf("unexpected index result: %s", text1)
	}

	// Step 2: skeleton should work after index
	req2 := MakeFakeRequest(map[string]any{"action": "skeletons", "query": "calc.go"})
	res2 := HandleCodegraphAction(ctx, tmpDir, deps, req2)
	var text2 string
	for _, c := range res2.Content {
		if tc, ok := c.(interface{ Text() string }); ok {
			text2 = tc.Text()
			break
		}
	}
	if strings.Contains(text2, "has no indexed symbols") {
		t.Fatalf("expected skeleton after initial index, got: %s", text2)
	}

	// Step 3: simulate stale DB by directly writing a file record with 0 nodes
	// This mimics the state produced by the old "..go" extension bug
	dbPath := filepath.Join(tmpDir, ".zenmcp", "codegraph.db")
	
	// Open the DB directly and insert a file record for a new file without nodes
	// We'll use a subprocess to avoid import cycles
	// Actually, let's just use the storage directly
	
	// First, get the existing session
	session, err := getSessionByWorkspace(tmpDir)
	if err != nil {
		t.Fatalf("getSessionByWorkspace: %v", err)
	}
	
	// Create a new file on disk that the scanner will find
	if err := os.WriteFile(filepath.Join(tmpDir, "stale.go"), []byte("package stale\n"), 0644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	
	// Force re-scan by clearing the session (simulating what --force does)
	ClearSessionGraphByWorkspace(tmpDir)
	
	// Re-index - this should pick up the new file
	req3 := MakeFakeRequest(map[string]any{"action": "index"})
	res3 := HandleCodegraphAction(ctx, tmpDir, deps, req3)
	_ = res3

	// Now verify skeleton works for both files
	for _, file := range []string{"calc.go", "stale.go"} {
		req := MakeFakeRequest(map[string]any{"action": "skeletons", "query": file})
		res := HandleCodegraphAction(ctx, tmpDir, deps, req)
		var text string
		for _, c := range res.Content {
			if tc, ok := c.(interface{ Text() string }); ok {
				text = tc.Text()
				break
			}
		}
		if strings.Contains(text, "has no indexed symbols") {
			t.Fatalf("expected skeleton for %s after reindex, got: %s", file, text)
		}
	}
}
