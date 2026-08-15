package tools

import (
	"zen-mcp/internal/gatekeeper"
	"zen-mcp/internal/shared"
	"zen-mcp/internal/toolregistry"
)

// Deps bundles the collaborators the M4 tool subset needs. Everything is
// injected from cmd/zen; the tools package never constructs singletons.
type Deps struct {
	Store                 *shared.Store
	Reg                   *toolregistry.ToolRegistry
	Gatekeeper            *gatekeeper.Gatekeeper
	PendingCollaborations *CollaborationRegistry
	// HideTools marks the agent-facing MCP server in mcp2cli mode: server/tools.go
	// then registers no tools onto the MCP server and tracks every tool as
	// disabled, so the agent never sees tool schemas. The CLI server (zen-*
	// wrappers) and the terminal keep this false.
	HideTools bool
}

// ToolDef describes one MCP tool. The Schema is the exact JSON Schema served
// in tools/list (replicating the TS zod-to-json-schema output); server/tools.go
// is responsible for building the mcp.Tool and wiring the handler.
type ToolDef struct {
	Name        string
	Title       string
	Description string
	Schema      map[string]any
	Handler     toolregistry.Handler
}

// AllDefs returns the M4 + M5 + M6 tool subset in TS registration order.
func AllDefs(workspace string, deps Deps) []ToolDef {
	return []ToolDef{
		defWorkspace(workspace, deps),
		defMemory(workspace, deps),
		defContext(workspace, deps),
		defShell(workspace, deps),
		defThink(workspace, deps),
		defRun(deps),
		defBrowser(workspace, deps),
		defMemoryIsolate(workspace, deps),
		defMemoryShared(workspace, deps),
		defColab(workspace, deps),
		defCapture(workspace, deps),
		defUiVision(workspace, deps),
		defPool(deps),
		defSkills(workspace, deps),
		defCodegraph(workspace, deps),
	}
}

// jsonSchema is a helper function
func jsonSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
		"$schema":              "http://json-schema.org/draft-07/schema#",
	}
}

// strProp is a helper function
func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// strEnumProp is a helper function
func strEnumProp(desc string, enum []string) map[string]any {
	return map[string]any{"type": "string", "enum": enum, "description": desc}
}

// numProp is a helper function
func numProp(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

// boolProp is a helper function
func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
