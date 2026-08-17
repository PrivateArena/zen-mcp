package codegraph

import "testing"

func TestCPluginParseFunctionsStructsEnumsTypedefs(t *testing.T) {
	src := []byte(`
#include <stdio.h>
static char* helper(int x) { return 0; }
int *dupstr(const char* s) { return 0; }
int main(void) { return 0; }
struct Point { int x; };
enum Color { RED, GREEN };
typedef struct Point Point_t;
`)
	p := newCPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	nodes, relations, err := p.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := map[string]int{}
	for _, n := range nodes {
		got[n.Type+":"+n.Name]++
	}
	want := map[string]int{
		"function:helper": 1,
		"function:dupstr": 1,
		"function:main":   1,
		"struct:Point":    1,
		"enum:Color":      1,
		"type:Point_t":    1,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("expected %s=%d, got %d", k, v, got[k])
		}
	}
	if len(relations) == 0 {
		t.Error("expected include relations, got none")
	}
	foundImport := false
	for _, r := range relations {
		if r.Relation == "imports" && r.TargetName == "stdio.h" {
			foundImport = true
		}
	}
	if !foundImport {
		t.Errorf("expected imports stdio.h relation, got %+v", relations)
	}
}

func TestCPluginSkipsForwardStructDecl(t *testing.T) {
	src := []byte(`
struct Point;
struct Point { int x; };
`)
	p := newCPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	nodes, _, err := p.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	count := 0
	for _, n := range nodes {
		if n.Type == "struct" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 struct, got %d", count)
	}
}

func TestCPluginCallRelation(t *testing.T) {
	src := []byte(`
int add(int a, int b) { return a + b; }
int main(void) { return add(1, 2); }
`)
	p := newCPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	nodes, relations, err := p.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(nodes))
	}
	found := false
	for _, r := range relations {
		if r.SourceName == "main" && r.TargetName == "add" && r.Relation == "calls" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected main calls add relation, got %+v", relations)
	}
}