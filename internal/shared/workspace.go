package shared

import "os"

// WorkspaceProvider exposes the session module surface that workspace
// resolution needs. session.Manager implements it; the interface keeps this
// package free of an import cycle (session imports shared for the Store).
type WorkspaceProvider interface {
	GetLastActiveSessionId() string
	GetSessionWorkspaceRoot(id string) string
}

// ResolveWorkspace mirrors workspace-resolver.ts priority:
// 1. explicit caller-provided workspace
// 2. registration-time workspace bound to the tool instance
// 3. active session workspace
// 4. shared `workspace-root` set by the MCP `workspace` tool
// 5. `MCP_WORKSPACE_ROOT` env
// 6. `process.cwd()`
func ResolveWorkspace(inputWorkspace, registrationWorkspace string, st *Store, sp WorkspaceProvider) string {
	if inputWorkspace != "" {
		return inputWorkspace
	}
	if registrationWorkspace != "" {
		return registrationWorkspace
	}

	if sp != nil {
		if sid := sp.GetLastActiveSessionId(); sid != "" {
			if sessionWs := sp.GetSessionWorkspaceRoot(sid); sessionWs != "" {
				return sessionWs
			}
		}
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
