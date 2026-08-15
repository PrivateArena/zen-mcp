package server

import (
	"context"
	"encoding/json"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"zen-mcp/internal/pooling"
	"zen-mcp/internal/prompts"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/toolresponse"
	"zen-mcp/internal/tools"
	"zen-mcp/internal/toolstate"
)

// RegisterAllTools registers the M4 tool subset onto srv for the given
// registration workspace, tracks them in reg, applies the effective
// enabled/disabled state and publishes the tools://catalog resource.
//
// When deps.HideTools is set (the agent-facing MCP server in mcp2cli mode)
// nothing is registered onto srv and every tool is tracked in reg as disabled:
// the server advertises no tools capability, tools/list is empty, tools/call
// fails with an unknown-tool error and no tools://catalog resource is
// published, so the agent never sees a single tool schema (zero token waste).
// The CLI server that zen-* wrappers target leaves HideTools false and keeps
// the full tool set.
func RegisterAllTools(ctx context.Context, srv *mcpserver.MCPServer, reg *toolregistry.ToolRegistry, deps tools.Deps, workspace string) error {
	hideTools := deps.HideTools
	defs := tools.AllDefs(workspace, deps)
	for i := range defs {
		def := defs[i]

		rawSchema, err := json.Marshal(def.Schema)
		if err != nil {
			return err
		}

		handler := wrapIfPooled(def.Name, def.Handler)

		if !hideTools {
			tool := mcp.Tool{
				Name:           def.Name,
				Description:    def.Description,
				RawInputSchema: rawSchema,
				Annotations: mcp.ToolAnnotation{
					Title:           def.Title,
					ReadOnlyHint:    boolPtr(false),
					DestructiveHint: boolPtr(true),
					IdempotentHint:  boolPtr(false),
					OpenWorldHint:   boolPtr(true),
				},
				Execution: &mcp.ToolExecution{TaskSupport: mcp.TaskSupportForbidden},
			}
			srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				ctx = toolresponse.WithToolContext(ctx, toolresponse.ToolContext{
					ToolName: def.Name,
					Params:   req.GetArguments(),
				})
				return handler(ctx, req)
			})
		}

		reg.Track(toolregistry.ToolRegistration{
			Name:           def.Name,
			DefaultEnabled: true,
			Description:    def.Description,
			Schema:         def.Schema,
			Handler:        handler,
		})
		if hideTools {
			// Toolstate-style total hide: registry state says disabled so the
			// catalog stays empty and FilterEnabled agrees with the server.
			reg.SetToolEnabled(def.Name, false)
		}
		toolresponse.SetToolSchema(def.Name, def.Schema)
	}

	if hideTools {
		// No toolstate (config must not re-enable tools), no catalog, no tool
		// catalog resource. Prompts are still published so mcp2cli mode can
		// serve CLI-transformed prompt text.
		prompts.RegisterPrompts(srv, workspace)
		return nil
	}

	// Apply global config enabled_tools + workspace .zenmcp/config.json.
	toolstate.ApplyToolStates(workspace, reg)

	registerToolCatalogResource(srv, reg)
	prompts.RegisterPrompts(srv, workspace)
	prompts.RegisterResources(srv)
	return nil
}

// boolPtr is a helper function
func boolPtr(b bool) *bool { return &b }

// wrapIfPooled applies the tool-call pooling wrapper. Every tool except the
// pool tool itself is wrapped at registration, but Wrap re-reads config per
// call and is a pass-through unless pooling is enabled AND name is in the
// configured Tools list. This makes the config.json toggle fully live: enabling
// pooling never requires a restart (the per-workspace server cache would
// otherwise pin the pre-enable registration decision). The pool tool is never
// wrapped — a wrapped pool poll would spawn a job and mint a second pool_id.
func wrapIfPooled(name string, handler toolregistry.Handler) toolregistry.Handler {
	if name == "pool" {
		return handler
	}
	return pooling.Wrap(name, pooling.Global(), handler)
}

// FilterEnabled returns an mcp-go ToolFilterFunc that hides tools whose
// registry entry is currently disabled.
func FilterEnabled(reg *toolregistry.ToolRegistry) func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
	return func(ctx context.Context, list []mcp.Tool) []mcp.Tool {
		out := make([]mcp.Tool, 0, len(list))
		for _, t := range list {
			if reg.IsToolEnabled(t.Name) {
				out = append(out, t)
			}
		}
		return out
	}
}
