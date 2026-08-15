package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"zen-mcp/internal/codegraph"
	"zen-mcp/internal/shared"
)

// buildCodegraphDB writes a small indexed graph into dir/.zenmcp/codegraph.db:
// two files (main.go, pkg/util.go), one symbol edge (run → helper), and one
// import record (main.go imports "pkg"). Returns the workspace dir.
func buildCodegraphDB(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".zenmcp"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbPath := filepath.Join(dir, ".zenmcp", "codegraph.db")

	w, err := codegraph.NewStorage(dbPath)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	defer w.Close()

	mainID, err := w.UpsertFile(codegraph.FileRecord{Path: "main.go", Hash: "h1", MTime: 1, Language: "go", IsTest: false})
	if err != nil {
		t.Fatalf("UpsertFile main: %v", err)
	}
	utilID, err := w.UpsertFile(codegraph.FileRecord{Path: "pkg/util.go", Hash: "h2", MTime: 1, Language: "go", IsTest: false})
	if err != nil {
		t.Fatalf("UpsertFile util: %v", err)
	}
	runID, err := w.InsertNode(codegraph.NodeRecord{FileID: mainID, Type: "function", Name: "run", Language: "go", Path: "main.go", StartLine: 1, EndLine: 5, Content: ""})
	if err != nil {
		t.Fatalf("InsertNode run: %v", err)
	}
	helperID, err := w.InsertNode(codegraph.NodeRecord{FileID: utilID, Type: "function", Name: "helper", Language: "go", Path: "pkg/util.go", StartLine: 1, EndLine: 3, Content: ""})
	if err != nil {
		t.Fatalf("InsertNode helper: %v", err)
	}
	if err := w.InsertEdge(runID, helperID, "calls", ""); err != nil {
		t.Fatalf("InsertEdge: %v", err)
	}
	if err := w.RecordImport(mainID, "pkg", false); err != nil {
		t.Fatalf("RecordImport: %v", err)
	}
	return dir
}

func TestServeHTMLReturns200(t *testing.T) {
	mux := http.NewServeMux()
	store := shared.NewStore()
	SetupLiveGraphRoutes(mux, store)

	req := httptest.NewRequest("GET", "/codegraph", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Fatalf("unexpected Content-Type: %s", ct)
	}
	if body := w.Body.String(); len(body) == 0 || !contains(body, "Codegraph Live") {
		t.Fatalf("expected inline SPA with D3, got %d bytes", len(body))
	}
}

func TestServeGraphDataNoWorkspace(t *testing.T) {
	mux := http.NewServeMux()
	store := shared.NewStore()
	SetupLiveGraphRoutes(mux, store)

	req := httptest.NewRequest("GET", "/api/codegraph/data", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var p GraphPayload
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Error == "" {
		t.Fatal("expected non-empty Error field, got empty")
	}
}

func TestServeGraphDataBadPath(t *testing.T) {
	mux := http.NewServeMux()
	store := shared.NewStore()
	store.Set("workspace-root", "/nonexistent/workspace/path")
	SetupLiveGraphRoutes(mux, store)

	req := httptest.NewRequest("GET", "/api/codegraph/data", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var p GraphPayload
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Error == "" {
		t.Fatal("expected non-empty Error field for bad DB path")
	}
}

func TestServeGraphDataWorkspaceNotIndexed(t *testing.T) {
	dir := t.TempDir() // exists, but never indexed → no codegraph.db

	mux := http.NewServeMux()
	store := shared.NewStore()
	store.Set("workspace-root", dir)
	SetupLiveGraphRoutes(mux, store)

	req := httptest.NewRequest("GET", "/api/codegraph/data", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var p GraphPayload
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Error == "" {
		t.Fatal("expected non-empty Error field for un-indexed workspace")
	}
}

func TestServeGraphDataRealWorkspace(t *testing.T) {
	dir := buildCodegraphDB(t, t.TempDir())

	mux := http.NewServeMux()
	store := shared.NewStore()
	store.Set("workspace-root", dir)
	SetupLiveGraphRoutes(mux, store)

	req := httptest.NewRequest("GET", "/api/codegraph/data", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var p GraphPayload
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Error != "" {
		t.Fatalf("unexpected Error: %s", p.Error)
	}
	if p.Workspace != dir {
		t.Fatalf("expected workspace %q, got %q", dir, p.Workspace)
	}
	if len(p.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(p.Files))
	}
}

func TestGraphPayloadStructure(t *testing.T) {
	dir := buildCodegraphDB(t, t.TempDir())

	payload := buildGraphPayload(dir)
	if payload.Error != "" {
		t.Fatalf("unexpected error: %s", payload.Error)
	}
	if len(payload.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(payload.Files))
	}
	if payload.Stats.FileCount != 2 || payload.Stats.NodeCount != 2 {
		t.Fatalf("unexpected stats: %+v", payload.Stats)
	}

	// Every edge must carry a valid level.
	for _, e := range payload.Edges {
		if e.Level != "file" && e.Level != "symbol" {
			t.Fatalf("edge has invalid Level: %q", e.Level)
		}
	}

	// The one symbol edge (run → helper) must appear at symbol level.
	symbolCalls := false
	// The symbol edge must also collapse to a file-level "calls" edge.
	fileCalls := false
	// The import record (main.go imports "pkg") must resolve to a file-level
	// "imports" edge targeting the only file inside pkg/.
	fileImports := false
	fileID := map[string]int64{}
	for _, f := range payload.Files {
		fileID[f.Path] = f.ID
	}
	for _, e := range payload.Edges {
		if e.Level == "symbol" && e.Relation == "calls" && e.SourceID != e.TargetID {
			symbolCalls = true
		}
		if e.Level == "file" && e.Relation == "calls" && e.SourceID == fileID["main.go"] && e.TargetID == fileID["pkg/util.go"] {
			fileCalls = true
		}
		if e.Level == "file" && e.Relation == "imports" && e.SourceID == fileID["main.go"] && e.TargetID == fileID["pkg/util.go"] {
			fileImports = true
		}
	}
	if !symbolCalls {
		t.Fatal("expected symbol-level calls edge")
	}
	if !fileCalls {
		t.Fatal("expected file-level calls edge derived from symbol collapse")
	}
	if !fileImports {
		t.Fatal("expected file-level imports edge resolved from imports table")
	}

	// Nodes must carry the symbol metadata the sidebar renders.
	for _, n := range payload.Nodes {
		if n.Name == "" || n.Type == "" {
			t.Fatalf("node missing metadata: %+v", n)
		}
	}
}

func TestGraphPayloadEmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".zenmcp"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbPath := filepath.Join(dir, ".zenmcp", "codegraph.db")
	w, err := codegraph.NewStorage(dbPath)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	w.Close()

	payload := buildGraphPayload(dir)
	if payload.Error != "" {
		t.Fatalf("unexpected error: %s", payload.Error)
	}
	if payload.Files == nil || payload.Nodes == nil || payload.Edges == nil {
		t.Fatal("empty workspace must yield empty slices, not nil (JSON must be [] not null)")
	}
	if bs, err := json.Marshal(payload.Files); err != nil || string(bs) != "[]" {
		t.Fatalf("expected files to serialize as [], got %s (err=%v)", bs, err)
	}
}

func TestBuildGraphPayloadMissingDB(t *testing.T) {
	payload := buildGraphPayload("/nonexistent/workspace/path")
	if payload.Error == "" {
		t.Fatal("expected non-empty Error for missing DB")
	}
}

func TestBuildGraphPayloadIsReadOnly(t *testing.T) {
	dir := buildCodegraphDB(t, t.TempDir())
	dbPath := filepath.Join(dir, ".zenmcp", "codegraph.db")
	before, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	payload := buildGraphPayload(dir)
	if payload.Error != "" {
		t.Fatalf("unexpected error: %s", payload.Error)
	}

	entries, err := os.ReadDir(filepath.Join(dir, ".zenmcp"))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "codegraph.db" {
		t.Fatalf("buildGraphPayload must not create or remove files, saw: %v", entries)
	}

	after, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if after.Size() != before.Size() || after.ModTime() != before.ModTime() {
		t.Fatalf("buildGraphPayload modified the database (size %d→%d)", before.Size(), after.Size())
	}
}

func TestResolveImportTarget(t *testing.T) {
	byPath := map[string]int64{
		"main.go":         1,
		"pkg/util.go":     2,
		"pkg/sub/deep.go": 3,
		"internal/srv.go": 4,
	}
	filesByDir := map[string][]int64{
		"pkg":      {2, 3},
		"internal": {4},
		".":        {1},
	}

	cases := []struct {
		spec string
		want int64
	}{
		{spec: "pkg/util.go", want: 2},    // exact path match
		{spec: `"pkg/util.go"`, want: 2},  // surrounding quotes stripped
		{spec: "pkg", want: 2},            // directory match, first file by ID
		{spec: "./internal", want: 4},     // relative prefix stripped
		{spec: "os", want: 0},             // standard library: no indexed file
		{spec: "github.com/x/y", want: 0}, // third-party: no indexed file
		{spec: "", want: 0},               // empty spec
		{spec: "   ", want: 0},            // whitespace-only
	}
	for _, c := range cases {
		if got := resolveImportTarget(c.spec, byPath, filesByDir); got != c.want {
			t.Errorf("resolveImportTarget(%q) = %d, want %d", c.spec, got, c.want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
