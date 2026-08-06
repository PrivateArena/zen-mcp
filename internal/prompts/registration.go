package prompts

import (
	"context"
	"log"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterPrompts registers all prompts with the MCP server.
func RegisterPrompts(srv *mcpserver.MCPServer, workspace string) {
	defs, err := LoadPromptDefinitions()
	if err != nil {
		return
	}

	for _, p := range defs {
		opts := []mcp.PromptOption{
			mcp.WithPromptDescription(p.Description),
		}
		for _, arg := range p.Arguments {
			argOpts := []mcp.ArgumentOption{
				mcp.ArgumentDescription(arg.Description),
			}
			if arg.Required {
				argOpts = append(argOpts, mcp.RequiredArgument())
			}
			opts = append(opts, mcp.WithArgument(arg.Name, argOpts...))
		}

		promptDef := mcp.NewPrompt(p.Name, opts...)

		handler := func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			safeArgs := make(map[string]string)
			for k, v := range req.Params.Arguments {
				safeArgs[k] = v
			}
			text, err := ResolvePrompt(p, safeArgs, workspace)
			if err != nil {
				log.Printf("[DEBUG] prompts/get name=%s args=%v status=ERROR err=%v", p.Name, safeArgs, err)
				return nil, err
			}
			log.Printf("[DEBUG] prompts/get name=%s args=%v status=OK len=%d", p.Name, safeArgs, len(text))
			return &mcp.GetPromptResult{
				Description: p.Description,
				Messages: []mcp.PromptMessage{
					{
						Role: "user",
						Content: mcp.TextContent{
							Type: "text",
							Text: text,
						},
					},
				},
			}, nil
		}

		srv.AddPrompt(promptDef, handler)
	}
}

// RegisterResources registers the tools/catalog resource.
func RegisterResources(srv *mcpserver.MCPServer) {
	srv.AddResource(
		mcp.NewResource("tools/catalog", "tools://catalog", mcp.WithResourceDescription("Full action catalog for all MCP tools"), mcp.WithMIMEType("text/plain")),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "tools://catalog",
					MIMEType: "text/plain",
					Text:     BuildToolCatalog(),
				},
			}, nil
		},
	)
}

// BuildToolCatalog builds the tool catalog text.
func BuildToolCatalog() string {
	return "Tool catalog - see tools/list for full details"
}
