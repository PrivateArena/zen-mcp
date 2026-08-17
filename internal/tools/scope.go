package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolresponse"
)

func defScope(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "scope",
		Title:       "Project Scope",
		Description: "View or update project scopes from map.json.",
		Schema: jsonSchema(map[string]any{
			"id": strProp("Scope ID to view/update"),
		}, []string{"id"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleScope(ctx, workspace, deps, req), nil
		},
	}
}

func HandleScope(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	id, _ := args["id"].(string)

	actualWorkspace := resolveWorkspaceFromDeps("", workspace)
	if actualWorkspace == "" {
		return toolresponse.WrapErrorWithContext(ctx, "scope",
			errors.New("Workspace path is required but could not be determined."), start)
	}

	mapFile := mcpcfg.MapFilePath()
	if _, err := os.Stat(mapFile); err != nil {
		return toolresponse.WrapSuccess(ctx, "scope", map[string]any{
			"project": actualWorkspace,
			"scopes":  map[string]any{},
		}, start)
	}
	raw, err := os.ReadFile(mapFile)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "scope", err, start)
	}
	var mapData map[string]any
	if json.Unmarshal(raw, &mapData) != nil {
		return toolresponse.WrapErrorWithContext(ctx, "scope", errors.New("Invalid map.json"), start)
	}
	entry, _ := mapData[actualWorkspace].(map[string]any)
	if entry == nil {
		return toolresponse.WrapSuccess(ctx, "scope", map[string]any{
			"project": actualWorkspace,
			"scopes":  map[string]any{},
		}, start)
	}

	scopes, _ := entry["scopes"].(map[string]any)
	if scopes == nil {
		scopes = map[string]any{}
	}

	if id != "" {
		if paths, ok := scopes[id]; ok {
			return toolresponse.WrapSuccess(ctx, "scope", map[string]any{
				"id":           id,
				"paths":        paths,
				"dependencies": dependenciesOrEmpty(entry),
			}, start)
		}
		return toolresponse.WrapErrorWithContext(ctx, "scope",
			errors.New("Scope not found: "+id), start)
	}

	return toolresponse.WrapSuccess(ctx, "scope", map[string]any{
		"project":      actualWorkspace,
		"scopes":       scopes,
		"dependencies": dependenciesOrEmpty(entry),
	}, start)
}

func dependenciesOrEmpty(entry map[string]any) any {
	if deps, ok := entry["dependencies"]; ok && deps != nil {
		return deps
	}
	return []any{}
}
