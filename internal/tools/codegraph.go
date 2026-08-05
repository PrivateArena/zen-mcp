package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/jang/zen-mcp/internal/codegraph"
	"github.com/jang/zen-mcp/internal/toolresponse"
)

var graphRegistry = map[string]*codegraph.CodeGraph{}

func getOrCreateGraph(workspace string) (*codegraph.CodeGraph, error) {
	if g, ok := graphRegistry[workspace]; ok {
		return g, nil
	}
	g, err := codegraph.NewCodeGraph(workspace)
	if err != nil {
		return nil, err
	}
	graphRegistry[workspace] = g
	return g, nil
}

func defCodegraph(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "codegraph",
		Title:       "Code Graph",
		Description: "Code graph engine. Actions: index, search, status, map, skeletons, mermaid, usage, neighbors, files, explain, related, deadcode, shortestPath, findCycles, markdown, impact.",
		Schema: jsonSchema(map[string]any{
			"action": strEnumProp("Codegraph action.", []string{
				"index", "search", "status", "map", "skeletons", "mermaid",
				"usage", "neighbors", "files", "explain", "related",
				"deadcode", "shortestPath", "findCycles", "markdown", "impact",
			}),
			"query": strProp("Search query or symbol name"),
			"limit": numProp("Result limit"),
			"isolate": numProp("Graph isolate level (0 = root)"),
			"semantic": boolProp("Use semantic search"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleCodegraphAction(ctx, workspace, deps, req), nil
		},
	}
}

func handleCodegraphAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)
	if action == "" {
		return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("action is required"), start)
	}

	workspaceRoot := resolveWorkspaceFromDeps("", workspace)
	if workspaceRoot == "" {
		cwd, _ := os.Getwd()
		workspaceRoot = cwd
	}

	g, err := getOrCreateGraph(workspaceRoot)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("failed to open codegraph: %s", err.Error()), start)
	}

	switch action {
	case "index":
		result, err := g.Index()
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", map[string]any{
			"indexed": result.Indexed,
			"total":   result.Total,
			"deleted": result.Deleted,
		}, start)

	case "search":
		query, _ := args["query"].(string)
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("query is required for search"), start)
		}
		results, err := g.Search(query)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", results, start)

	case "status":
		status, err := g.Status()
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", status, start)

	case "map":
		m, err := g.Map()
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", m, start)

	case "skeletons":
		s, err := g.Skeletons()
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", s, start)

	case "mermaid":
		m, err := g.Mermaid()
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", m, start)

	case "usage":
		query, _ := args["query"].(string)
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("query is required for usage"), start)
		}
		results, err := g.Usage(query)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", results, start)

	case "neighbors":
		query, _ := args["query"].(string)
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("query is required for neighbors"), start)
		}
		neighbors, err := g.Neighbors(query)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", neighbors, start)

	case "files":
		filter, _ := args["query"].(string)
		limit := 200
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		files, err := g.Files(filter)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		if len(files) > limit {
			files = files[:limit]
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", files, start)

	case "explain":
		query, _ := args["query"].(string)
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("query is required for explain"), start)
		}
		explanation, err := g.Explain(query)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", explanation, start)

	case "related":
		query, _ := args["query"].(string)
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("query is required for related"), start)
		}
		related, err := g.Related(query)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", related, start)

	case "deadcode":
		result, err := g.Deadcode()
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", result, start)

	case "shortestPath":
		query, _ := args["query"].(string)
		parts := strings.SplitN(query, ",", 2)
		if len(parts) < 2 {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("query must be 'from,to' for shortestPath"), start)
		}
		path, err := g.ShortestPath(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", path, start)

	case "findCycles":
		cycles, err := g.FindCycles()
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", cycles, start)

	case "markdown":
		md, err := g.Markdown()
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", md, start)

	case "impact":
		query, _ := args["query"].(string)
		if query == "" {
			query = "HEAD"
		}
		impact, err := g.Impact(query)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", impact, start)

	default:
		return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("unknown action: %s", action), start)
	}
}
