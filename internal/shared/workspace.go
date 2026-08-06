package shared

import "os"

// ResolveWorkspace mirrors workspace-resolver.ts priority:
// 1. explicit caller-provided workspace
// 2. registration-time workspace bound to the tool instance
// 3. shared `workspace-root` set by the MCP `workspace` tool
// 4. `MCP_WORKSPACE_ROOT` env
// 5. `process.cwd()`
func ResolveWorkspace(inputWorkspace, registrationWorkspace string, st *Store) string {
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
