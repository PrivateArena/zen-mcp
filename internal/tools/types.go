package tools

import (
	"github.com/jang/zen-mcp/internal/gatekeeper"
	"github.com/jang/zen-mcp/internal/shared"
	"github.com/jang/zen-mcp/internal/toolregistry"
)

// Deps bundles the collaborators the M4 tool subset needs. Everything is
// injected from cmd/zen; the tools package never constructs singletons.
type Deps struct {
	Store                *shared.Store
	Reg                  *toolregistry.ToolRegistry
	Gatekeeper           *gatekeeper.Gatekeeper
	PendingCollaborations map[string]func(string)
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
		defSkills(workspace, deps),
		defCodegraph(workspace, deps),
	}
}

func jsonSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
		"$schema":              "http://json-schema.org/draft-07/schema#",
	}
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func strEnumProp(desc string, enum []string) map[string]any {
	return map[string]any{"type": "string", "enum": enum, "description": desc}
}

func numProp(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
