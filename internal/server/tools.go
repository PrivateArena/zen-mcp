package server

import (
	"context"
	"encoding/json"

	mcpserver "github.com/mark3labs/mcp-go/server"
	mcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/jang/zen-mcp/internal/toolregistry"
	"github.com/jang/zen-mcp/internal/toolresponse"
	"github.com/jang/zen-mcp/internal/tools"
	"github.com/jang/zen-mcp/internal/toolstate"
)

// RegisterAllTools registers the M4 tool subset onto srv for the given
// registration workspace, tracks them in reg, applies the effective
// enabled/disabled state and publishes the tools://catalog resource.
func RegisterAllTools(ctx context.Context, srv *mcpserver.MCPServer, reg *toolregistry.ToolRegistry, deps tools.Deps, workspace string) error {
	defs := tools.AllDefs(workspace, deps)
	for i := range defs {
		def := defs[i]

		rawSchema, err := json.Marshal(def.Schema)
		if err != nil {
			return err
		}

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

		handler := def.Handler
		srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handler(ctx, req)
		})

		reg.Track(toolregistry.ToolRegistration{
			Name:           def.Name,
			DefaultEnabled: true,
			Description:    def.Description,
			Schema:         def.Schema,
			Handler:        handler,
		})
		toolresponse.SetToolSchema(def.Name, def.Schema)
	}

	// Apply global config enabled_tools + workspace .zenmcp/config.json.
	toolstate.ApplyToolStates(workspace, reg)

	registerToolCatalogResource(srv, reg, deps)
	return nil
}

func boolPtr(b bool) *bool { return &b }

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
