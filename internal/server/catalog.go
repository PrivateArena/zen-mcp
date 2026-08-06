package server

import (
	"context"
	"fmt"
	"sort"
	"strings"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/jang/zen-mcp/internal/toolregistry"
	"github.com/jang/zen-mcp/internal/tools"
)

// registerToolCatalogResource ports index.ts tools/catalog: a static resource
// (tools://catalog) whose content is the action catalog of enabled tools.
func registerToolCatalogResource(srv *mcpserver.MCPServer, reg *toolregistry.ToolRegistry, deps tools.Deps) {
	catalogURI := "tools://catalog"
	srv.AddResource(
		mcp.NewResource(catalogURI, "tools/catalog",
			mcp.WithResourceDescription("Full action catalog for all MCP tools"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:  catalogURI,
					Text: buildToolCatalog(reg),
				},
			}, nil
		},
	)
}

// buildToolCatalog ports buildToolCatalog from terminal/registry.ts.
func buildToolCatalog(reg *toolregistry.ToolRegistry) string {
	lines := []string{"# MCP Tools Catalog", "", "Full action and parameter reference for all registered tools.", ""}

	regs := reg.ListTools()
	sort.Slice(regs, func(i, j int) bool { return regs[i].Name < regs[j].Name })

	for _, entry := range regs {
		if !entry.Enabled {
			continue
		}
		lines = append(lines, "## "+entry.Name)
		lines = append(lines, "")
		lines = append(lines, entry.Description)
		lines = append(lines, "")

		schema := entry.Schema
		if schema != nil {
			if actionSchema, ok := schema["action"].(map[string]any); ok {
				if enum, ok := actionSchema["enum"].([]any); ok && len(enum) > 0 {
					lines = append(lines, "Actions:")
					for _, v := range enum {
						if s, ok := v.(string); ok {
							lines = append(lines, "  - "+s)
						}
					}
					lines = append(lines, "")
				}
			}
			keys := make([]string, 0, len(schema))
			for k := range schema {
				if k != "action" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				if prop, ok := schema[key].(map[string]any); ok {
					if desc, ok := prop["description"].(string); ok && desc != "" {
						lines = append(lines, fmt.Sprintf("- %s: %s", key, desc))
					}
				}
			}
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}
