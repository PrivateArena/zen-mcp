package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func TestForceReindexProducesSkeletons(t *testing.T) {
	ws := t.TempDir()

	src := `package foo

func Add(a int, b int) int {
	return a + b
}
`
	if err := os.WriteFile(filepath.Join(ws, "calc.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ctx := context.Background()
	deps := Deps{}

	// Step 1: initial index
	reqIndex := makeFakeRequest(map[string]any{"action": "index"})
	resIndex := HandleCodegraphAction(ctx, ws, deps, reqIndex)
	if resIndex == nil {
		t.Fatalf("index result is nil")
	}
	textIndex := toolText(resIndex)
	t.Logf("index result: %s", textIndex)

	// Step 2: skeleton should work after index
	reqSkel := makeFakeRequest(map[string]any{"action": "skeletons", "query": "calc.go"})
	resSkel := HandleCodegraphAction(ctx, ws, deps, reqSkel)
	textSkel := toolText(resSkel)
	t.Logf("skeleton after initial index: %s", textSkel)
	if strings.Contains(textSkel, "has no indexed symbols") {
		t.Fatalf("expected skeleton after initial index, got: %s", textSkel)
	}

	// Step 3: simulate --force: clear session cache + delete DB
	ClearSessionGraphByWorkspace(ws)
	dbPath := filepath.Join(ws, ".zenmcp", "codegraph.db")
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove db: %v", err)
	}

	// Step 4: re-index
	resIndex2 := HandleCodegraphAction(ctx, ws, deps, reqIndex)
	textIndex2 := toolText(resIndex2)
	t.Logf("index after --force: %s", textIndex2)

	// Step 5: skeleton should STILL work after force reindex
	resSkel2 := HandleCodegraphAction(ctx, ws, deps, reqSkel)
	textSkel2 := toolText(resSkel2)
	t.Logf("skeleton after --force: %s", textSkel2)
	if strings.Contains(textSkel2, "has no indexed symbols") {
		t.Fatalf("expected skeleton after force reindex, got: %s", textSkel2)
	}
}

func makeFakeRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "",
			Arguments: args,
		},
	}
}

func toolText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
