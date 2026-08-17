package codegraph

import "testing"

const luaSample = `local M = {}

local function helper(x)
    return x * 2
end

function M.export()
    local y = require("util")
    return y.go()
end

function obj:method()
    helper(1)
end

local anon = function() end
`

func TestLuaPluginNodes(t *testing.T) {
	p := newLuaPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	nodes, _, err := p.Parse([]byte(luaSample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := map[string]int{}
	for _, n := range nodes {
		got[n.Type+":"+n.Name]++
	}
	want := map[string]int{
		"function:helper":   1,
		"function:M.export": 1,
		"function:obj:method": 1,
		"class:M":           1,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("expected %s=%d, got %d", k, v, got[k])
		}
	}
}

func TestLuaPluginRelations(t *testing.T) {
	p := newLuaPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, relations, err := p.Parse([]byte(luaSample))
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
	// M.export calls y.go (dot_index callee)
	if !gotCalls["M.export->y.go"] {
		t.Errorf("expected M.export calls y.go, got %+v", gotCalls)
	}
	// obj:method calls helper
	if !gotCalls["obj:method->helper"] {
		t.Errorf("expected obj:method calls helper, got %+v", gotCalls)
	}
	if !gotImports["util"] {
		t.Errorf("expected imports util (require), got %+v", gotImports)
	}
}