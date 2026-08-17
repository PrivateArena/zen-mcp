package codegraph

import "testing"

const cppSample = `#include <thread>
namespace sfz {
namespace detail {
class Inner {
public:
    int go() { return 1; }
};
}
class BasicSndfileReader : public AudioReader {
public:
    explicit BasicSndfileReader(int h) : h_(h) {}
    virtual ~BasicSndfileReader() {}
    int format() const override { return 1; }
    int64_t frames() const override { return 2; }
};
struct Point { int x; };
enum Color { RED };
int BasicSndfileReader::getSampleRate() const { return 44100; }
AudioReaderPtr createAudioReader(int path) { return nullptr; }
`

func TestCppPluginNodes(t *testing.T) {
	p := newCppPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	nodes, _, err := p.Parse([]byte(cppSample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := map[string]int{}
	for _, n := range nodes {
		got[n.Type+":"+n.Name]++
	}
	want := map[string]int{
		"class:Inner":                 1,
		"class:BasicSndfileReader":    1,
		"method:go":                   1,
		"method:format":               1,
		"method:frames":               1,
		"method:getSampleRate":        1, // out-of-line
		"method:BasicSndfileReader":   1, // ctor
		"method:~BasicSndfileReader":  1, // dtor
		"struct:Point":                1,
		"enum:Color":                  1,
		"function:createAudioReader":  1,
		"namespace:sfz":               1,
		"namespace:detail":            1,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("expected %s=%d, got %d", k, v, got[k])
		}
	}
}

func TestCppPluginParsesPureC(t *testing.T) {
	// The cpp grammar is a superset of C; pure C headers must still yield symbols.
	src := []byte(`
int add(int a, int b) { return a + b; }
struct Point { int x; };
enum Color { RED };
`)
	p := newCppPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	nodes, _, err := p.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := map[string]int{}
	for _, n := range nodes {
		got[n.Type+":"+n.Name]++
	}
	if got["function:add"] != 1 {
		t.Errorf("expected function:add=1, got %d", got["function:add"])
	}
	if got["struct:Point"] != 1 {
		t.Errorf("expected struct:Point=1, got %d", got["struct:Point"])
	}
}

func TestCppPluginRelations(t *testing.T) {
	src := []byte(`#include <memory>
namespace sfz {
class Foo {
public:
    void bar() { baz(); }
};
void baz() {}
void caller() {
    Foo f;
    f.bar();
    baz();
    auto p = new Foo();
}
}
`)
	p := newCppPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, relations, err := p.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(relations) == 0 {
		t.Fatal("expected relations, got none")
	}
	found := false
	for _, r := range relations {
		if r.SourceName == "caller" && r.TargetName == "baz" && r.Relation == "calls" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected caller calls baz, got %+v", relations)
	}
	hasImport := false
	for _, r := range relations {
		if r.Relation == "imports" {
			hasImport = true
		}
	}
	if !hasImport {
		t.Errorf("expected an imports relation, got %+v", relations)
	}
}