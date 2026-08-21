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

// writeFixture writes a small source file under dir and returns its rel path.
func writeFixture(t *testing.T, dir, rel, src string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(p, []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

const goFixture = `package foo

func Add(a int, b int) int {
	return a + b
}
`

// TestFilesListsSubgraphScopes reproduces the reported bug: with a workspace
// root that contains a sub-directory holding its own .zenmcp/codegraph.db
// (a sub-graph), the files action must list that sub-graph's indexed files,
// not only the root scope.
func TestFilesListsSubgraphScopes(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "main.go", goFixture)
	writeFixture(t, ws, filepath.Join("www", "handler.go"), goFixture)

	ctx := context.Background()
	deps := Deps{}

	// Index the www sub-graph independently (it gets its own .zenmcp/codegraph.db).
	res := HandleCodegraphAction(ctx, filepath.Join(ws, "www"), deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("www index failed: %s", toolText(res))
	}
	ClearSessionGraphByWorkspace(filepath.Join(ws, "www"))

	// Now use the root workspace: files must surface both scopes.
	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "files"}))
	text := toolText(res)
	t.Logf("files output:\n%s", text)

	if !strings.Contains(text, "[scope: ROOT]") {
		t.Fatalf("expected a ROOT scope, got:\n%s", text)
	}
	if !strings.Contains(text, "[scope: www]") {
		t.Fatalf("expected a www sub-graph scope, got:\n%s", text)
	}
	if !strings.Contains(text, "handler.go") {
		t.Fatalf("expected www sub-graph files to be listed, got:\n%s", text)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestSubgraphDiscoveredAfterInitialSession guards the stale-session hazard:
// a session cached before a sub-graph was indexed must pick the sub-graph up
// on a later call instead of forever serving the root-only snapshot.
func TestSubgraphDiscoveredAfterInitialSession(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "main.go", goFixture)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("root index failed: %s", toolText(res))
	}
	ClearSessionGraphByWorkspace(ws)

	// First files call: only ROOT exists at this point. The session stays
	// cached (no ClearSessionGraphByWorkspace) so the stale hazard is real.
	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "files"}))
	first := toolText(res)
	if strings.Contains(first, "[scope: www]") {
		t.Fatalf("unexpected www scope before it exists:\n%s", first)
	}

	// Create + index a www sub-graph, then re-list files WITHOUT clearing the
	// cached root session. The session must re-discover the new sub-graph.
	writeFixture(t, ws, filepath.Join("www", "handler.go"), goFixture)
	res = HandleCodegraphAction(ctx, filepath.Join(ws, "www"), deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("www index failed: %s", toolText(res))
	}
	ClearSessionGraphByWorkspace(filepath.Join(ws, "www"))

	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "files"}))
	second := toolText(res)
	t.Logf("files after adding www:\n%s", second)
	if !strings.Contains(second, "[scope: www]") {
		t.Fatalf("sub-graph created after the root session was cached is not listed:\n%s", second)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestDocsFullAndDocsLessActions guards the docstring-maintenance actions:
// docsfull must list only documented symbols (with their docstrings), while
// docsless must list only symbols missing a docstring. Neither action requires
// a query — they behave like a global skeletons scan.
func TestDocsFullAndDocsLessActions(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "calc.go", `package foo

// Adds a and b.
func Add(a int, b int) int {
	return a + b
}

func mul(x, y int) int {
	return x * y
}
`)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	resFull := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "docsfull"}))
	full := toolText(resFull)
	t.Logf("docsfull output:\n%s", full)
	if !strings.Contains(full, "function Add func") {
		t.Fatalf("docsfull should list documented symbol Add:\n%s", full)
	}
	if !strings.Contains(full, "Adds a and b") {
		t.Fatalf("docsfull should include the docstring text:\n%s", full)
	}
	if strings.Contains(full, "function mul func") {
		t.Fatalf("docsfull should not list undocumented symbol mul:\n%s", full)
	}

	resLess := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "docsless"}))
	less := toolText(resLess)
	t.Logf("docsless output:\n%s", less)
	if !strings.Contains(less, "function mul func") {
		t.Fatalf("docsless should list undocumented symbol mul:\n%s", less)
	}
	if strings.Contains(less, "function Add func") {
		t.Fatalf("docsless should not list documented symbol Add:\n%s", less)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestDocslessAllDocumented guards the fully-documented happy path: when every
// indexed symbol carries a docstring, docsless must report a clear message
// instead of returning an empty or erroring result.
func TestDocslessAllDocumented(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "calc.go", `package foo

// Adds a and b.
func Add(a int, b int) int {
	return a + b
}
`)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	resLess := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "docsless"}))
	less := toolText(resLess)
	t.Logf("docsless (all documented) output:\n%s", less)
	if !strings.Contains(less, "No symbols missing docstrings") {
		t.Fatalf("docsless should report no missing docstrings when all symbols are documented:\n%s", less)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestSymbolReturnsRawCodeBlock guards the new `symbol` action: it must return
// ONLY the raw source lines of the symbol (no headers, line numbers, file
// annotations, or scope banners) so the output can be piped straight into sed
// for code replacement. Both the path-qualified form (calc.go:Add) and the bare
// symbol form (Add) must resolve to the same raw block.
func TestSymbolReturnsRawCodeBlock(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "calc.go", goFixture)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	expected := "func Add(a int, b int) int {\n\treturn a + b\n}"

	// path-qualified form: calc.go:Add
	resPath := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "symbol", "query": "calc.go:Add"}))
	textPath := toolText(resPath)
	t.Logf("symbol calc.go:Add output:\n%q", textPath)
	if textPath != expected {
		t.Fatalf("symbol calc.go:Add should return the raw code block only, got:\n%q\nwant:\n%q", textPath, expected)
	}
	// raw contract: no file/package/line noise that would break a sed insert
	for _, noise := range []string{"package foo", "File:", "lines", "Scope", "scope"} {
		if strings.Contains(textPath, noise) {
			t.Fatalf("symbol output must be raw; found %q in:\n%q", noise, textPath)
		}
	}

	// bare symbol form: Add (no path)
	resBare := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "symbol", "query": "Add"}))
	textBare := toolText(resBare)
	t.Logf("symbol Add output:\n%q", textBare)
	if textBare != expected {
		t.Fatalf("symbol Add (no path) should return the raw code block, got:\n%q\nwant:\n%q", textBare, expected)
	}

	// unknown symbol -> error text, no panic, no partial block
	resMiss := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "symbol", "query": "Nope"}))
	textMiss := toolText(resMiss)
	t.Logf("symbol Nope output:\n%q", textMiss)
	if !strings.Contains(textMiss, "not found") {
		t.Fatalf("unknown symbol should report not found, got:\n%q", textMiss)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestSymbolQualifiedNameKeepsColon guards path:symbol parsing against
// qualified symbol names that contain colons (e.g. C++ Class::method): only
// the FIRST colon separates path from symbol, so the whole Class::method must
// survive as the symbol name.
func TestSymbolQualifiedNameKeepsColon(t *testing.T) {
	path, sym := parseSymbolQuery("src/foo.cpp:Class::method")
	if path != "src/foo.cpp" {
		t.Fatalf("expected path 'src/foo.cpp', got %q", path)
	}
	if sym != "Class::method" {
		t.Fatalf("expected symbol 'Class::method', got %q", sym)
	}

	// a bare qualified name with no path must stay intact
	p2, s2 := parseSymbolQuery("Class::method")
	if p2 != "" || s2 != "Class::method" {
		t.Fatalf("bare qualified name should be the symbol; got path=%q sym=%q", p2, s2)
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

// TestSearchReturnsMarkdownTable verifies that the search action returns a
// Markdown table rather than a JSON array.
func TestSearchReturnsMarkdownTable(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "calc.go", `package foo

// Adds a and b.
func Add(a int, b int) int {
	return a + b
}
`)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "search", "query": "Add"}))
	text := toolText(res)
	t.Logf("search output:\n%s", text)

	if !strings.Contains(text, "| Name |") {
		t.Fatalf("search should return a Markdown table, got:\n%s", text)
	}
	if strings.Contains(text, `"name":"Add"`) {
		t.Fatalf("search should not return JSON, got:\n%s", text)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestUsageReturnsMarkdownTable verifies that the usage action returns a
// Markdown table rather than a JSON array.
func TestUsageReturnsMarkdownTable(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "calc.go", `package foo

// Adds a and b.
func Add(a int, b int) int {
	return a + b
}
`)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "usage", "query": "Add"}))
	text := toolText(res)
	t.Logf("usage output:\n%s", text)

	if !strings.Contains(text, "| Name |") {
		t.Fatalf("usage should return a Markdown table, got:\n%s", text)
	}
	if strings.Contains(text, `"name":"Add"`) {
		t.Fatalf("usage should not return JSON, got:\n%s", text)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestNeighborsReturnsMarkdownTable verifies that the neighbors action returns
// a Markdown table rather than a JSON object.
func TestNeighborsReturnsMarkdownTable(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "calc.go", `package foo

func Add(a int, b int) int {
	return a + b
}

func Calculate() int {
	return Add(1, 2)
}
`)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "neighbors", "query": "Calculate"}))
	text := toolText(res)
	t.Logf("neighbors output:\n%s", text)

	if !strings.Contains(text, "| Name |") {
		t.Fatalf("neighbors should return a Markdown table, got:\n%s", text)
	}
	if strings.Contains(text, `"callers"`) && strings.Contains(text, `"callees"`) {
		t.Fatalf("neighbors should not return JSON object, got:\n%s", text)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestStatusReturnsMarkdown verifies that the status action returns Markdown
// rather than a JSON object.
func TestStatusReturnsMarkdown(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "calc.go", `package foo

func Add(a int, b int) int {
	return a + b
}
`)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "status"}))
	text := toolText(res)
	t.Logf("status output:\n%s", text)

	if !strings.Contains(text, "**Working Dir**") {
		t.Fatalf("status should return Markdown with bold keys, got:\n%s", text)
	}
	if strings.Contains(text, `"workingDir"`) {
		t.Fatalf("status should not return JSON, got:\n%s", text)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestMapReturnsMarkdown verifies that the map action returns Markdown
// rather than a JSON object.
func TestMapReturnsMarkdown(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "calc.go", `package foo

func Add(a int, b int) int {
	return a + b
}
`)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "map"}))
	text := toolText(res)
	t.Logf("map output:\n%s", text)

	if !strings.Contains(text, "# Repository Map") {
		t.Fatalf("map should return Markdown with a title, got:\n%s", text)
	}
	if !strings.Contains(text, "| Language |") {
		t.Fatalf("map should return Markdown tables, got:\n%s", text)
	}
	ClearSessionGraphByWorkspace(ws)
}

// TestShortestPathReturnsMarkdownNotJSON verifies that shortestPath returns
// Markdown text and never emits JSON even when a path exists.
func TestShortestPathReturnsMarkdownNotJSON(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "calc.go", `package foo

func Add(a int, b int) int {
	return a + b
}

func Calculate() int {
	return Add(1, 2)
}
`)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "shortestPath", "query": "Calculate,Add"}))
	text := toolText(res)
	t.Logf("shortestPath output:\n%s", text)

	if strings.Contains(text, "{") && strings.Contains(text, `"Found"`) {
		t.Fatalf("shortestPath should not return JSON, got:\n%s", text)
	}
	ClearSessionGraphByWorkspace(ws)
}

func TestExplainReturnsMarkdownWithCallersCalleesAndTwoHop(t *testing.T) {
	ws := t.TempDir()
	writeFixture(t, ws, "main.go", `package foo

func main() {
	Process()
}
`)
	writeFixture(t, ws, "processor.go", `package foo

func Process() {
	Validate()
	Save()
}
`)
	writeFixture(t, ws, "validator.go", `package foo

func Validate() {
	Check()
}
`)
	writeFixture(t, ws, "checker.go", `package foo

func Check() {}
`)
	writeFixture(t, ws, "saver.go", `package foo

func Save() {}
`)

	ctx := context.Background()
	deps := Deps{}

	res := HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "index"}))
	if strings.Contains(toolText(res), "failed") {
		t.Fatalf("index failed: %s", toolText(res))
	}

	res = HandleCodegraphAction(ctx, ws, deps, makeFakeRequest(map[string]any{"action": "explain", "query": "Process"}))
	text := toolText(res)
	t.Logf("explain output:\n%s", text)

	if !strings.Contains(text, "Callers (1-hop") {
		t.Fatalf("expected Callers section in explain output, got:\n%s", text)
	}
	if !strings.Contains(text, "Callees (1-hop") {
		t.Fatalf("expected Callees section in explain output, got:\n%s", text)
	}
	if !strings.Contains(text, "2-hop neighbors") {
		t.Fatalf("expected 2-hop neighbors section in explain output, got:\n%s", text)
	}
	if !strings.Contains(text, "← main") {
		t.Fatalf("expected caller arrow for main, got:\n%s", text)
	}
	if !strings.Contains(text, "→ Validate") {
		t.Fatalf("expected callee arrow for Validate, got:\n%s", text)
	}
	if !strings.Contains(text, "→ Save") {
		t.Fatalf("expected callee arrow for Save, got:\n%s", text)
	}
	if !strings.Contains(text, "[via main]") {
		t.Fatalf("expected 2-hop 'via main' suffix, got:\n%s", text)
	}
	if !strings.Contains(text, "[via Validate]") {
		t.Fatalf("expected 2-hop 'via Validate' suffix, got:\n%s", text)
	}
	if strings.Contains(text, "[via calls]") {
		t.Fatalf("explain must not contain stale [via calls] placeholder, got:\n%s", text)
	}
	ClearSessionGraphByWorkspace(ws)
}
