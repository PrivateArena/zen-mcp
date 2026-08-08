package codegraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestShortestPathNowWorks guards C2: FindShortestPath used to query a
// nonexistent nodes.path column and always returned Found=false.
func TestShortestPathNowWorks(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "a.go", "package p\n\nfunc A() { B() }\n")
	writeFile(t, tmp, "b.go", "package p\n\nfunc B() { C() }\n")
	writeFile(t, tmp, "c.go", "package p\n\nfunc C() {}\n")

	cg, err := NewCodeGraph(tmp)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	direct, err := cg.FindShortestPath("A", "B", 6)
	if err != nil {
		t.Fatalf("FindShortestPath direct: %v", err)
	}
	if !direct.Found {
		t.Fatalf("expected direct path A->B to be found, got %+v", direct)
	}
	if len(direct.Path) != 1 {
		t.Fatalf("expected 1 step for A->B, got %+v", direct.Path)
	}

	multi, err := cg.FindShortestPath("A", "C", 6)
	if err != nil {
		t.Fatalf("FindShortestPath multi: %v", err)
	}
	if !multi.Found {
		t.Fatalf("expected multi-hop path A->C to be found, got %+v", multi)
	}
	if len(multi.Path) != 2 {
		t.Fatalf("expected 2 steps for A->C, got %+v", multi.Path)
	}
	if multi.Path[0].SourceName != "A" || multi.Path[0].TargetName != "B" {
		t.Fatalf("step 1 wrong: %+v", multi.Path[0])
	}
	if multi.Path[1].SourceName != "B" || multi.Path[1].TargetName != "C" {
		t.Fatalf("step 2 wrong: %+v", multi.Path[1])
	}

	missing, err := cg.FindShortestPath("A", "DoesNotExist", 6)
	if err != nil {
		t.Fatalf("FindShortestPath missing: %v", err)
	}
	if missing.Found {
		t.Fatalf("expected no path for missing symbol, got %+v", missing)
	}
}

// TestReindexPreservesIncomingEdges guards C3: deleting a changed file's nodes
// used to cascade-delete incoming edges from unchanged referrers, and those
// edges were never rebuilt, hollowing out the call graph.
func TestReindexPreservesIncomingEdges(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "a.go", "package p\n\nfunc Process() {}\n")
	writeFile(t, tmp, "b.go", "package p\n\nfunc Caller() {\n\tProcess()\n}\n")

	cg, err := NewCodeGraph(tmp)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	processBefore, _ := cg.storage.FindNodesByName("Process")
	if len(processBefore) == 0 {
		t.Fatalf("expected Process node after index")
	}
	callersBefore, _, _ := cg.storage.GetNeighbors(processBefore[0].ID, 20)
	if !hasNamedNode(callersBefore, "Caller") {
		t.Fatalf("expected Caller to call Process before reindex, callers=%+v", callersBefore)
	}

	// Mutate a.go in place (Process keeps its name and start line) and re-index.
	writeFile(t, tmp, "a.go", "package p\n\nfunc Process() {\n\t_ = 1\n}\n\nfunc Extra() {}\n")

	result, err := cg.Index()
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if result.Indexed != 1 {
		t.Fatalf("expected exactly 1 file re-indexed, got %d (total=%d)", result.Indexed, result.Total)
	}

	processAfter, _ := cg.storage.FindNodesByName("Process")
	if len(processAfter) == 0 {
		t.Fatalf("expected Process node after reindex")
	}
	if processAfter[0].ID != processBefore[0].ID {
		t.Fatalf("Process node id changed across reindex: %d -> %d", processBefore[0].ID, processAfter[0].ID)
	}

	callersAfter, _, _ := cg.storage.GetNeighbors(processAfter[0].ID, 20)
	if !hasNamedNode(callersAfter, "Caller") {
		t.Fatalf("incoming edge Caller->Process lost after incremental reindex, callers=%+v", callersAfter)
	}

	extra, _ := cg.storage.FindNodesByName("Extra")
	if len(extra) == 0 {
		t.Fatalf("expected Extra node after reindex")
	}
}

func hasNamedNode(nodes []NodeRecord, name string) bool {
	for _, n := range nodes {
		if n.Name == name {
			return true
		}
	}
	return false
}

// TestDeletedFileCleanup guards C4: files removed from disk used to leave
// ghost records in the index forever, and IndexResult.Deleted was never set.
func TestDeletedFileCleanup(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "a.go", "package p\n\nfunc A() {}\n")
	writeFile(t, tmp, "b.go", "package p\n\nfunc B() {}\n")

	cg, err := NewCodeGraph(tmp)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}
	if cg.storage.GetFileByPath("b.go") == nil {
		t.Fatalf("expected b.go indexed")
	}

	if err := os.Remove(filepath.Join(tmp, "b.go")); err != nil {
		t.Fatalf("remove b.go: %v", err)
	}

	result, err := cg.Index()
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("expected Deleted=1, got %d", result.Deleted)
	}
	if cg.storage.GetFileByPath("b.go") != nil {
		t.Fatalf("expected b.go record removed from index")
	}
	if cg.storage.GetFileByPath("a.go") == nil {
		t.Fatalf("expected a.go still indexed")
	}
}

// TestEdgeResolutionScoped guards C1: relations must resolve to symbols in the
// source file's scope first, with a global name match only as an INFERRED
// fallback, instead of fanning out to every same-named node in the repo.
func TestEdgeResolutionScoped(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "d1/a.go", "package a1\n\nfunc Call1() { Handle() }\n")
	writeFile(t, tmp, "d1/h.go", "package a1\n\nfunc Handle() {}\n")
	writeFile(t, tmp, "d2/b.go", "package b1\n\nfunc Handle() {}\n\nfunc Call2() { Other() }\n")
	writeFile(t, tmp, "d3/other.go", "package c1\n\nfunc Other() {}\n")

	cg, err := NewCodeGraph(tmp)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	// Call1's Handle must resolve to d1/h.go (same directory), not d2/b.go.
	call1, _ := cg.storage.FindNodesByName("Call1")
	if len(call1) == 0 {
		t.Fatalf("expected Call1 node")
	}
	_, callees, err := cg.storage.GetNeighbors(call1[0].ID, 20)
	if err != nil {
		t.Fatalf("GetNeighbors: %v", err)
	}
	if len(callees) != 1 || callees[0].Name != "Handle" || callees[0].Path != "d1/h.go" {
		t.Fatalf("expected single scoped Handle edge to d1/h.go, got %+v", callees)
	}

	// Call2's Other resolves only via global fallback -> INFERRED confidence.
	var confidence string
	err = cg.storage.db.QueryRow(`
		SELECT e.confidence
		FROM edges e JOIN nodes n ON e.source_id = n.id
		WHERE n.name = 'Call2' LIMIT 1
	`).Scan(&confidence)
	if err != nil {
		t.Fatalf("query confidence: %v", err)
	}
	if confidence != "INFERRED" {
		t.Fatalf("expected INFERRED confidence for global fallback edge, got %q", confidence)
	}
}

// TestIncrementalScannerFastPath guards M1/M2/M3: a second scan must not
// re-read/re-hash unchanged files, and changed files must carry their content
// through so Index does not read the file a second time.
func TestIncrementalScannerFastPath(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "calc.go", "package p\n\nfunc Add() {}\n")

	cg, err := NewCodeGraph(tmp)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	first, err := cg.scanner.GetFilesToProcess()
	if err != nil {
		t.Fatalf("GetFilesToProcess: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("expected 1 file to process on first scan, got %d", len(first))
	}
	if len(first[0].content) == 0 {
		t.Fatalf("expected scanner to thread file content through FileRecord")
	}

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	second, err := cg.scanner.GetFilesToProcess()
	if err != nil {
		t.Fatalf("GetFilesToProcess (second): %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("expected 0 files to process on second scan (unchanged), got %d", len(second))
	}
}

// TestTruncateRuneSafe guards N6: byte-based truncation used to split a
// multi-byte UTF-8 rune, storing invalid UTF-8 in node content.
func TestTruncateRuneSafe(t *testing.T) {
	s := "héllo wörld"
	cut := truncate(s, 2)
	if len(cut) > 2 {
		t.Fatalf("truncate(%q, 2) len=%d > 2", s, len(cut))
	}
	if !utf8.ValidString(cut) {
		t.Fatalf("truncate produced invalid UTF-8: %q", cut)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("truncate with maxLen >= len should return input, got %q", got)
	}
}

// TestSanitizeFtsQuery guards N4: single-token operators and column syntax must
// not leak into FTS5 as raw syntax, while plain words and prefix wildcards
// still pass through unquoted.
func TestSanitizeFtsQuery(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"foo", "foo"},
		{"foo*", "foo*"},
		{"foo_bar", "foo_bar"},
		{"foo bar", `"foo bar"`},
		{"content:bar", `"content:bar"`},
		{"-foo", `"-foo"`},
		{`he"llo`, `"he""llo"`},
		{"", `""`},
		{"   ", `""`},
	}
	for _, c := range cases {
		if got := sanitizeFtsQuery(c.in); got != c.want {
			t.Errorf("sanitizeFtsQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if !strings.Contains(sanitizeFtsQuery("foo bar"), `"`) {
		t.Errorf("expected whitespace query to be quoted")
	}
}
