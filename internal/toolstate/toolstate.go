package toolstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolregistry"
)

const (
	workspaceDir     = ".zenmcp"
	workspaceCfgName = "config.json"
)

type ToolStateLayers struct {
	Builtin   bool
	Global    *bool
	Workspace *bool
	Effective bool
	Source    string
}

// BuiltinDefaultEnabled is a helper function
func BuiltinDefaultEnabled(toolName string) bool {
	return true
}

// ReadWorkspaceConfigRaw is a helper function
func ReadWorkspaceConfigRaw(workspaceRoot string) map[string]any {
	path := filepath.Join(workspaceRoot, workspaceDir, workspaceCfgName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		logfilter.Debugf("[ToolState] Failed to parse %s: %v", path, err)
		return nil
	}
	return parsed
}

// ReadWorkspaceToolConfig is a helper function
func ReadWorkspaceToolConfig(workspaceRoot string) map[string]bool {
	raw := ReadWorkspaceConfigRaw(workspaceRoot)
	if raw == nil {
		return nil
	}
	enabled, ok := raw["enabled_tools"].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]bool{}
	for k, v := range enabled {
		if b, ok := v.(bool); ok {
			out[k] = b
		}
	}
	return out
}

// ResolveToolState is a helper function
func ResolveToolState(name string, workspaceRoot string, workspaceCfg map[string]bool, reg *toolregistry.ToolRegistry) ToolStateLayers {
	builtin := true
	if entry, ok := reg.GetTool(name); ok {
		builtin = entry.DefaultEnabled
	} else {
		builtin = BuiltinDefaultEnabled(name)
	}

	var global *bool
	if c := mcpcfg.Get(); c != nil {
		if v, ok := c.EnabledTools[name]; ok {
			global = &v
		}
	}

	var workspace *bool
	if workspaceRoot != "" {
		cfg := workspaceCfg
		if cfg == nil {
			cfg = ReadWorkspaceToolConfig(workspaceRoot)
		}
		if v, ok := cfg[name]; ok {
			workspace = &v
		}
	}

	var effective bool
	var source string
	switch {
	case workspace != nil:
		effective = *workspace
		source = "workspace"
	case global != nil:
		effective = *global
		source = "global"
	default:
		effective = builtin
		source = "builtin"
	}

	return ToolStateLayers{Builtin: builtin, Global: global, Workspace: workspace, Effective: effective, Source: source}
}

type ApplyResult struct {
	Changed []string
	Skipped []string
}

// ApplyToolStates is a helper function
func ApplyToolStates(workspaceRoot string, reg *toolregistry.ToolRegistry) ApplyResult {
	var workspaceCfg map[string]bool
	if workspaceRoot != "" {
		workspaceCfg = ReadWorkspaceToolConfig(workspaceRoot)
	}
	var changed, skipped []string

	for _, name := range reg.ListToolNames() {
		layers := ResolveToolState(name, workspaceRoot, workspaceCfg, reg)
		if reg.IsToolEnabled(name) == layers.Effective {
			skipped = append(skipped, name)
			continue
		}
		if reg.SetToolEnabled(name, layers.Effective) {
			changed = append(changed, name)
		} else {
			skipped = append(skipped, name)
		}
	}

	if len(changed) > 0 {
		logfilter.Debugf("[ToolState] Applied tool states — changed: [%s], skipped: %d", strings.Join(changed, ", "), len(skipped))
	}

	applyPromptState(workspaceRoot)

	return ApplyResult{Changed: changed, Skipped: skipped}
}

// applyPromptState mirrors prompt-state.ts applyPromptState. Persona resolution
// lives in internal/prompts (F7) and is wired there; here it is a no-op hook.
func applyPromptState(workspaceRoot string) {}
