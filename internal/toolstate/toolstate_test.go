package toolstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jang/zen-mcp/internal/mcpcfg"
	"github.com/jang/zen-mcp/internal/toolregistry"
)

func setupConfig(t *testing.T, enabledTools map[string]bool) {
	t.Helper()
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir()
	t.Cleanup(func() {
		mcpcfg.ProjectRoot = old
	})
	if err := mcpcfg.Load(); err != nil {
		t.Fatal(err)
	}
	mcpcfg.Get().EnabledTools = enabledTools
}

func regWithTools(defaults map[string]bool) *toolregistry.ToolRegistry {
	reg := toolregistry.Create()
	for name, def := range defaults {
		reg.Track(toolregistry.ToolRegistration{Name: name, DefaultEnabled: def})
	}
	return reg
}

func writeWorkspaceConfig(t *testing.T, ws string, enabledTools map[string]any) {
	t.Helper()
	dir := filepath.Join(ws, ".zenmcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{}
	if enabledTools != nil {
		cfg["enabled_tools"] = enabledTools
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinDefaultEnabled(t *testing.T) {
	if !BuiltinDefaultEnabled("anything") {
		t.Error("builtinDefaultEnabled should always return true")
	}
}

func TestResolveToolStatePrecedence(t *testing.T) {
	setupConfig(t, map[string]bool{"capture": false}) // global capture=false
	reg := regWithTools(map[string]bool{"workspace": true, "capture": true, "think": true})

	ws := t.TempDir()
	writeWorkspaceConfig(t, ws, map[string]any{"think": false}) // workspace think=false

	// builtin: no global, no workspace -> builtin
	layers := ResolveToolState("workspace", "", nil, reg)
	if layers.Effective != true || layers.Source != "builtin" {
		t.Errorf("workspace layers = %+v", layers)
	}
	// global beats builtin
	layers = ResolveToolState("capture", "", nil, reg)
	if layers.Effective != false || layers.Source != "global" {
		t.Errorf("capture layers = %+v", layers)
	}
	// workspace beats global
	layers = ResolveToolState("think", ws, nil, reg)
	if layers.Effective != false || layers.Source != "workspace" {
		t.Errorf("think layers = %+v", layers)
	}
}

func TestApplyToolStates(t *testing.T) {
	setupConfig(t, map[string]bool{"capture": false})
	reg := regWithTools(map[string]bool{"workspace": true, "capture": true})

	res := ApplyToolStates("", reg)
	if !reg.IsToolEnabled("workspace") {
		t.Error("workspace should stay enabled")
	}
	if reg.IsToolEnabled("capture") {
		t.Error("capture should be disabled by global config")
	}
	if len(res.Changed) != 1 || res.Changed[0] != "capture" {
		t.Errorf("Changed = %v", res.Changed)
	}
}

func TestApplyToolStatesWorkspaceOverride(t *testing.T) {
	setupConfig(t, map[string]bool{"capture": false})
	reg := regWithTools(map[string]bool{"capture": true})
	ws := t.TempDir()
	writeWorkspaceConfig(t, ws, map[string]any{"capture": true})

	ApplyToolStates(ws, reg)
	if !reg.IsToolEnabled("capture") {
		t.Error("workspace config should re-enable capture")
	}
}

func TestReadWorkspaceToolConfigInvalidJSON(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".zenmcp")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{ bad json"), 0o644)
	if got := ReadWorkspaceToolConfig(ws); got != nil {
		t.Errorf("invalid json should return nil, got %v", got)
	}
}
