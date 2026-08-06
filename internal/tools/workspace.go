package tools

import (
	"context"
	"os"
	"path/filepath"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/jang/zen-mcp/internal/logfilter"
	"github.com/jang/zen-mcp/internal/mcpcfg"
	"github.com/jang/zen-mcp/internal/projectmemory"
	"github.com/jang/zen-mcp/internal/toolresponse"
	"github.com/jang/zen-mcp/internal/toolstate"
)

func defWorkspace(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "workspace",
		Title:       "Project Workspace",
		Description: "Set the workspace root directory for the current session.",
		Schema: jsonSchema(map[string]any{
			"path": strProp("Absolute or relative path to set as workspace root"),
		}, []string{"path"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			path := req.GetArguments()["path"].(string)
			return HandleWorkspaceAction(ctx, path, workspace, deps), nil
		},
	}
}

func HandleWorkspaceAction(ctx context.Context, path, workspace string, deps Deps) *mcp.CallToolResult {
	start := time.Now()
	cwd, _ := os.Getwd()
	workspaceRoot := workspace
	if workspaceRoot == "" {
		workspaceRoot = cwd
	}

	prevPath := workspaceRoot
	if v, ok := deps.Store.Get("workspace-root"); ok && v != "" {
		prevPath = v
	}

	resolver := NewPathResolver(LoadAliasMap(), "")
	newRoot, ok := resolver.Resolve(path)
	if !ok {
		return toolresponse.WrapErrorWithContext(ctx, "workspace", &workspaceErr{msg: "path is required"}, start)
	}

	deps.Store.Set("workspace-root", newRoot)
	actualNewRoot := newRoot
	projectmemory.RegisterProjectInMap(actualNewRoot, nil)

	// Write/update the project-root map.json and symlink .zenmcp/map.json to it.
	mapFile := mcpcfg.MapFilePath()
	if _, err := os.Stat(mapFile); os.IsNotExist(err) {
		if err := os.WriteFile(mapFile, []byte("{}"), 0o644); err != nil {
			logfilter.Debugf("[Workspace] Failed to write map.json: %v", err)
		}
	}
	zenmcpDir := filepath.Join(actualNewRoot, ".zenmcp")
	if err := os.MkdirAll(zenmcpDir, 0o755); err != nil {
		logfilter.Debugf("[Workspace] Failed to create .zenmcp dir: %v", err)
	}
	linkPath := filepath.Join(zenmcpDir, "map.json")
	_ = os.Remove(linkPath)
	if err := os.Symlink(mapFile, linkPath); err != nil {
		logfilter.Info("[Workspace] Failed to create .zenmcp/map.json symlink: " + err.Error())
	}

	toolsChanged := []string{}
	resolvedNewRoot := actualNewRoot
	if resolvedNewRoot != prevPath && deps.Reg != nil {
		result := toolstate.ApplyToolStates(resolvedNewRoot, deps.Reg)
		if result.Changed != nil {
			toolsChanged = result.Changed
		}
	}

	return toolresponse.WrapSuccess(ctx, "workspace", map[string]any{
		"path":          resolvedNewRoot,
		"prev_path":     prevPath,
		"tools_changed": toolsChanged,
		"message":       "Workspace -> " + resolvedNewRoot,
	}, start)
}

type workspaceErr struct{ msg string }

func (e *workspaceErr) Error() string { return e.msg }
