package codegraph

import (
	"testing"
)

// TestNodeResolverMatchesDatabase pins the memoized resolver's contract: it
// must return exactly the rows FindNodesByName returns, memoize repeat
// lookups (no re-query), and report empty for unknown names.
func TestNodeResolverMatchesDatabase(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "calc.go", "package p\n\nfunc Add(a int, b int) int {\n\treturn a + b\n}\n\nfunc Sub(a int, b int) int {\n\treturn a - b\n}\n")

	cg, err := NewCodeGraph(tmp)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	r := newNodesResolver(cg.storage)

	fromDB, err := cg.storage.FindNodesByName("Add")
	if err != nil {
		t.Fatalf("FindNodesByName(Add): %v", err)
	}
	if len(fromDB) != 1 {
		t.Fatalf("expected 1 Add node from DB, got %d", len(fromDB))
	}

	fromResolver := r.find("Add")
	if len(fromResolver) != len(fromDB) {
		t.Fatalf("resolver returned %d nodes, DB returned %d", len(fromResolver), len(fromDB))
	}
	for i := range fromDB {
		if fromResolver[i].ID != fromDB[i].ID || fromResolver[i].Name != fromDB[i].Name {
			t.Fatalf("resolver node %d mismatch: %+v vs %+v", i, fromResolver[i], fromDB[i])
		}
	}

	// Memoization: a repeat lookup must return the same cached slice, not a
	// freshly queried one. Comparing element addresses is a cheap proxy for
	// "no re-query happened".
	if again := r.find("Add"); &again[0] != &fromResolver[0] {
		t.Fatalf("expected repeat lookup to reuse the cached slice")
	}

	// Unknown names resolve to empty, cached as such.
	if got := r.find("DoesNotExist"); len(got) != 0 {
		t.Fatalf("expected empty result for unknown name, got %d nodes", len(got))
	}
	if got := r.find("DoesNotExist"); got != nil {
		t.Fatalf("expected cached empty result to stay nil")
	}
}

// TestResolveNodesForScopeMemoized pins scope semantics through the memoized
// resolver: same file first, then same directory, then a global INFERRED
// fallback — identical to the pre-memoization behavior of resolveNodesForScope.
func TestResolveNodesForScopeMemoized(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "d1/a.go", "package a1\n\nfunc Call1() { Handle() }\n")
	writeFile(t, tmp, "d1/h.go", "package a1\n\nfunc Handle() {}\n")
	writeFile(t, tmp, "d2/b.go", "package b1\n\nfunc Handle() {}\n")

	cg, err := NewCodeGraph(tmp)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	r := newNodesResolver(cg.storage)

	// Same directory match for a target referenced from d1/a.go.
	targets, conf := resolveNodesForScope(r, "Handle", "d1/a.go", false)
	if conf != "EXTRACTED" {
		t.Fatalf("expected EXTRACTED for same-dir match, got %q", conf)
	}
	if len(targets) != 1 || targets[0].Path != "d1/h.go" {
		t.Fatalf("expected Handle to resolve to d1/h.go, got %+v", targets)
	}

	// Same directory match for the d2 copy.
	targets2, conf2 := resolveNodesForScope(r, "Handle", "d2/b.go", false)
	if conf2 != "EXTRACTED" {
		t.Fatalf("expected EXTRACTED for same-dir match, got %q", conf2)
	}
	if len(targets2) != 1 || targets2[0].Path != "d2/b.go" {
		t.Fatalf("expected Handle to resolve to d2/b.go, got %+v", targets2)
	}

	// requireSameFile with no in-file match must resolve to nothing.
	_, conf3 := resolveNodesForScope(r, "Handle", "d1/a.go", true)
	if conf3 != "EXTRACTED" {
		t.Fatalf("expected no in-file match under requireSameFile, got conf %q", conf3)
	}

	// Unresolvable name → empty result.
	missing, conf4 := resolveNodesForScope(r, "NoSuchSymbol", "d1/a.go", false)
	if len(missing) != 0 || conf4 != "EXTRACTED" {
		t.Fatalf("expected empty resolution for unknown symbol, got %+v / %q", missing, conf4)
	}
}