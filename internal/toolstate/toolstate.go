package toolstate

import (
	"strings"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolregistry"
)

type ToolStateLayers struct {
	Builtin   bool
	Global    *bool
	Workspace *bool
	Effective bool
	Source    string
}

func BuiltinDefaultEnabled(toolName string) bool {
	return true
}

// ReadWorkspaceConfigRaw is the project tier of the config, parsed verbatim. It
// lives in mcpcfg (ProjectConfigFilePath) so the tool-state layer and every other
// per-workspace override read the same file.
func ReadWorkspaceConfigRaw(workspaceRoot string) map[string]any {
	return mcpcfg.ReadProjectConfigRaw(workspaceRoot)
}

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

// DiffToolStates reports which tools a workspace tier would switch, without
// touching reg. The workspace's own MCP server instance applies them when it is
// built; this is only what `zworkspace` reports back to the caller, so moving
// between projects cannot leave the previous project's registry mis-flagged.
func DiffToolStates(workspaceRoot string, reg *toolregistry.ToolRegistry) []string {
	if reg == nil {
		return nil
	}
	var workspaceCfg map[string]bool
	if workspaceRoot != "" {
		workspaceCfg = ReadWorkspaceToolConfig(workspaceRoot)
	}
	var changed []string
	for _, name := range reg.ListToolNames() {
		if ResolveToolState(name, workspaceRoot, workspaceCfg, reg).Effective != reg.IsToolEnabled(name) {
			changed = append(changed, name)
		}
	}
	return changed
}

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
