package codegraph

import "testing"

const rubySample = `require 'net/http'
require_relative 'config'

module MyMod
  class Foo < Bar
    def initialize
      @x = 1
    end

    def self.build
      Foo.new
    end

    def run(a)
      helper(a)
      config_load
    end
  end
end
`

func TestRubyPluginNodes(t *testing.T) {
	p := newRubyPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	nodes, _, err := p.Parse([]byte(rubySample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := map[string]int{}
	for _, n := range nodes {
		got[n.Type+":"+n.Name]++
	}
	// Module and class are also expected if queries capture them.
	if got["module:MyMod"] != 1 {
		t.Errorf("expected module:MyMod=1, got %d", got["module:MyMod"])
	}
	if got["class:Foo"] != 1 {
		t.Errorf("expected class:Foo=1, got %d", got["class:Foo"])
	}
	if got["method:initialize"] != 1 {
		t.Errorf("expected method:initialize=1, got %d", got["method:initialize"])
	}
	if got["method:build"] != 1 {
		t.Errorf("expected method:build=1 (singleton), got %d", got["method:build"])
	}
	if got["method:run"] != 1 {
		t.Errorf("expected method:run=1, got %d", got["method:run"])
	}
}

func TestRubyPluginRelations(t *testing.T) {
	p := newRubyPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, relations, err := p.Parse([]byte(rubySample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(relations) == 0 {
		t.Fatal("expected relations, got none")
	}
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
	// self.build calls Foo.new -> method field is "new"
	if !gotCalls["self.build->new"] {
		t.Errorf("expected self.build calls new, got %+v", gotCalls)
	}
	// run calls helper
	if !gotCalls["run->helper"] {
		t.Errorf("expected run calls helper, got %+v", gotCalls)
	}
	// run calls config_load (bare identifier in expression_statement)
	if !gotCalls["run->config_load"] {
		t.Errorf("expected run calls config_load, got %+v", gotCalls)
	}
	if !gotImports["net/http"] || !gotImports["config"] {
		t.Errorf("expected imports net/http and config, got %+v", gotImports)
	}
}