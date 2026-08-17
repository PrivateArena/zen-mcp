package codegraph

import "testing"

const javaSample = `import java.util.List;
import java.util.Map;

class Account {
    private int balance;

    Account(int b) { balance = b; }

    public int getBalance() { return balance; }

    public void transfer(Account target, int amt) {
        target.deposit(amt);
        withdraw(amt);
        List items = new ArrayList();
    }

    private void withdraw(int amt) { balance -= amt; }
}

interface Repository {
    Account find(int id);
}

enum Color {
    RED, GREEN;
    void apply() {}
}
`

func TestJavaPluginNodes(t *testing.T) {
	p := newJavaPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	nodes, _, err := p.Parse([]byte(javaSample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := map[string]int{}
	for _, n := range nodes {
		got[n.Type+":"+n.Name]++
	}
	want := map[string]int{
		"class:Account":        1,
		"interface:Repository": 1,
		"enum:Color":           1,
		"method:Account":       1, // constructor
		"method:getBalance":    1,
		"method:transfer":      1,
		"method:withdraw":      1,
		"method:find":          1, // interface method
		"method:apply":         1, // enum body method
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("expected %s=%d, got %d", k, v, got[k])
		}
	}
}

func TestJavaPluginRelations(t *testing.T) {
	p := newJavaPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, relations, err := p.Parse([]byte(javaSample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(relations) == 0 {
		t.Fatal("expected relations, got none")
	}
	// transfer calls deposit / withdraw / ArrayList (object creation)
	gotCalls := map[string]bool{}
	gotImports := map[string]bool{}
	for _, r := range relations {
		if r.Relation == "calls" {
			gotCalls[r.SourceName+"->"+r.TargetName] = true
		}
		if r.Relation == "imports" {
			gotImports[r.TargetName] = true
		}
	}
	if !gotCalls["transfer->deposit"] {
		t.Errorf("expected transfer calls deposit, got %+v", gotCalls)
	}
	if !gotCalls["transfer->withdraw"] {
		t.Errorf("expected transfer calls withdraw, got %+v", gotCalls)
	}
	if !gotCalls["transfer->ArrayList"] {
		t.Errorf("expected transfer calls ArrayList (object creation), got %+v", gotCalls)
	}
	if !gotImports["java.util.List"] || !gotImports["java.util.Map"] {
		t.Errorf("expected imports java.util.List/Map, got %+v", gotImports)
	}
}
