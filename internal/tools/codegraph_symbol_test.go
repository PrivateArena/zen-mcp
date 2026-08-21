package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestSymbolNoDuplicationAcrossScopes guards the root-cause fix for the
// symbol-action duplication bug: a path query like "ssl/https.lua:fn" must not
// match the same file through multiple directory prefixes (greedy suffix
// matching) nor emit the block once per layered scope. The output must be the
// raw block exactly once even when the file is indexed by the root graph and a
// sub-graph and also appears under a deeper directory.
func TestSymbolNoDuplicationAcrossScopes(t *testing.T) {
	ws := t.TempDir()
	lua := `local function default_https_port(u)
   return url.build(url.parse(u, {port = PORT}))
end
`
	// Root scope sees ssl/https.lua (path ssl/https.lua) AND a deeper copy at
	// sub/ssl/https.lua (path sub/ssl/https.lua) which a greedy suffix match
	// would also resolve from the query "ssl/https.lua".
	writeFixture(t, ws, "ssl/https.lua", lua)
	writeFixture(t, ws, filepath.Join("sub", "ssl", "https.lua"), lua)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("root index failed: %s", toolText(res))
	}
	ClearSessionGraphByWorkspace(ws)

	// A sub-graph at ws/sub also indexes ssl/https.lua (path ssl/https.lua
	// relative to the sub-graph root), so the symbol resolves in two scopes.
	res = HandleCodegraphAction(ctx, filepath.Join(ws, "sub"), deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("sub index failed: %s", toolText(res))
	}
	ClearSessionGraphByWorkspace(filepath.Join(ws, "sub"))

	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "symbol", "query": "ssl/https.lua:default_https_port"}))
	text := toolText(res)
	t.Logf("symbol output (%d bytes):\n%q", len(text), text)

	const expected = "local function default_https_port(u)\n   return url.build(url.parse(u, {port = PORT}))\nend"
	if text != expected {
		t.Fatalf("symbol output should be the raw block exactly once, got:\n%q\nwant:\n%q", text, expected)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestSymbolBareNameMatchesByBasename verifies that a bare filename query (no
// directory) still resolves a symbol in any directory, while a path query with
// a directory component stays precise (only the exact relative path matches).
func TestSymbolBareNameMatchesByBasename(t *testing.T) {
	ws := t.TempDir()
	lua := `local function default_https_port(u)
   return url.build(url.parse(u, {port = PORT}))
end
`
	writeFixture(t, ws, "ssl/https.lua", lua)
	writeFixture(t, ws, filepath.Join("sub", "ssl", "https.lua"), lua)

	ctx := context.Background()
	deps := Deps{}
	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	// Bare name: both files define default_https_port -> two raw blocks.
	resBare := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "symbol", "query": "default_https_port"}))
	textBare := toolText(resBare)
	t.Logf("bare symbol output:\n%q", textBare)
	if strings.Count(textBare, "local function default_https_port(u)") != 2 {
		t.Fatalf("bare name should match the symbol in both directories, got:\n%q", textBare)
	}

	// Path form: only ssl/https.lua (exact), not sub/ssl/https.lua.
	resPath := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "symbol", "query": "ssl/https.lua:default_https_port"}))
	textPath := toolText(resPath)
	t.Logf("path symbol output:\n%q", textPath)
	if strings.Count(textPath, "local function default_https_port(u)") != 1 {
		t.Fatalf("path form should match exactly one file, got:\n%q", textPath)
	}
	ClearSessionGraphByWorkspace(ws)
}
