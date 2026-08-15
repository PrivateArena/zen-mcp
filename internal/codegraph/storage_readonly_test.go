package codegraph

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProbeDB(t *testing.T, dir string) (dbPath string) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, ".zenmcp"), 0o755)
	dbPath = filepath.Join(dir, ".zenmcp", "codegraph.db")

	w, err := NewStorage(dbPath)
	if err != nil {
		t.Fatalf("writer NewStorage: %v", err)
	}
	defer w.Close()

	fid, err := w.UpsertFile(FileRecord{Path: "a.go", Hash: "h", MTime: 1, Language: "go", IsTest: false})
	if err != nil {
		t.Fatalf("UpsertFile: %v", err)
	}
	if _, err := w.InsertNode(NodeRecord{FileID: fid, Type: "function", Name: "foo", Language: "go", Path: "a.go", StartLine: 1, EndLine: 3, Content: ""}); err != nil {
		t.Fatalf("InsertNode: %v", err)
	}
	if err := w.InsertEdge(fid, fid, "calls", ""); err != nil {
		t.Fatalf("InsertEdge: %v", err)
	}
	if err := w.RecordImport(fid, "pkg/util", false); err != nil {
		t.Fatalf("RecordImport: %v", err)
	}
	return dbPath
}

func TestNewReadOnlyStorageReadsWrites(t *testing.T) {
	dbPath := writeProbeDB(t, t.TempDir())

	r, err := NewReadOnlyStorage(dbPath)
	if err != nil {
		t.Fatalf("NewReadOnlyStorage: %v", err)
	}
	defer r.Close()

	files := r.GetAllFiles()
	if len(files) != 1 || files[0].Path != "a.go" {
		t.Fatalf("unexpected files: %+v", files)
	}
	nodes, err := r.GetAllNodes()
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Name != "foo" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
	edges := r.GetAllEdgeRecords("", 0)
	if len(edges) != 1 || edges[0].Relation != "calls" {
		t.Fatalf("unexpected edges: %+v", edges)
	}
	imports := r.GetAllImports()
	if len(imports) != 1 || imports[0].ImportPath != "pkg/util" || imports[0].IsSideEffect {
		t.Fatalf("unexpected imports: %+v", imports)
	}
}

func TestNewReadOnlyStorageBlocksWrites(t *testing.T) {
	dbPath := writeProbeDB(t, t.TempDir())

	r, err := NewReadOnlyStorage(dbPath)
	if err != nil {
		t.Fatalf("NewReadOnlyStorage: %v", err)
	}
	defer r.Close()

	if _, err := r.UpsertFile(FileRecord{Path: "b.go", Hash: "h", MTime: 1, Language: "go"}); err == nil {
		t.Fatal("expected UpsertFile to fail on read-only connection")
	}
	if err := r.InsertEdge(1, 2, "calls", ""); err == nil {
		t.Fatal("expected InsertEdge to fail on read-only connection")
	}
}

func TestNewReadOnlyStorageMissingDB(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, ".zenmcp", "codegraph.db")

	r, err := NewReadOnlyStorage(missing)
	if r != nil {
		r.Close()
	}
	if err == nil {
		t.Fatal("expected NewReadOnlyStorage to error on a missing database file")
	}
}
