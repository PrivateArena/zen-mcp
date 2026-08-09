package workspace

import (
	"os"
	"path/filepath"

	"zen-mcp/internal/shared"
)

// ResolveWorkspace mirrors workspace-resolver.ts priority:
// 1. explicit caller-provided workspace
// 2. registration-time workspace bound to the tool instance
// 3. shared `workspace-root` set by the MCP `workspace` tool
// 4. `MCP_WORKSPACE_ROOT` env
// 5. `process.cwd()`
func ResolveWorkspace(inputWorkspace, registrationWorkspace string, st *shared.Store) string {
	if inputWorkspace != "" {
		return inputWorkspace
	}
	if registrationWorkspace != "" {
		return registrationWorkspace
	}

	if st != nil {
		if sharedWs, ok := st.Get("workspace-root"); ok && sharedWs != "" {
			return sharedWs
		}
	}

	if env := os.Getenv("MCP_WORKSPACE_ROOT"); env != "" {
		return env
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

// ResolveWorkspacePath mirrors the TS resolvePath helper: try the map.json
// alias/heuristic resolver first, then fall back to an absolute path or a
// cwd-joined relative path. Callers must re-check existence of the fallback.
func ResolveWorkspacePath(input, cwd string) string {
	if input == "" {
		return ""
	}
	resolver := NewPathResolver(LoadAliasMap(), cwd)
	if p, ok := resolver.Resolve(input); ok {
		return p
	}
	if filepath.IsAbs(input) {
		return input
	}
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	return filepath.Join(cwd, input)
}
