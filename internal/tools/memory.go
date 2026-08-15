package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/projectmemory"
	"zen-mcp/internal/toolresponse"
)

// defMemory is a helper function
func defMemory(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "memory",
		Title:       "Project Memory",
		Description: "Persistent project state (.zenmcp/). Actions: load, save, scope.",
		Schema: jsonSchema(map[string]any{
			"action":        strEnumProp("Action", []string{"load", "save", "scope"}),
			"workspace":     strProp("Project path (default: current session workspace)"),
			"session_title": strProp("[save] One-line label, only if changed"),
			"objective":     strProp("[save] 1-2 sentence goal, only if changed"),
			"session_notes": strProp("[save] This session's notes as markdown. See the project-compact prompt for the required section headers."),
			"scope":         strProp("[scope] Scope ID to view/update"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleMemoryAction(ctx, workspace, deps, req), nil
		},
	}
}

// HandleMemoryAction is a helper function
func HandleMemoryAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)
	inputWorkspace, _ := args["workspace"].(string)
	sessionTitle, _ := args["session_title"].(string)
	objective, _ := args["objective"].(string)
	sessionNotes, _ := args["session_notes"].(string)
	scope, _ := args["scope"].(string)

	actualWorkspace := resolveWorkspaceFromDeps(inputWorkspace, workspace)
	if actualWorkspace == "" {
		return toolresponse.WrapErrorWithContext(ctx, "memory",
			errors.New("Workspace path is required but could not be determined. Please provide it explicitly or set a workspace root first."), start)
	}

	dataDir := filepath.Join(actualWorkspace, ".zenmcp")
	const memoryName = "brain"
	dbPath := filepath.Join(dataDir, "context.db")

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "memory", err, start)
		}
	}

	var result any
	switch action {
	case "load":
		state, err := loadProjectMemoryState(actualWorkspace, memoryName, deps)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "memory", err, start)
		}
		result = state
	case "save":
		result = actionSave(dataDir, memoryName, dbPath, actualWorkspace, sessionTitle, objective, sessionNotes)
	case "scope":
		result = actionScope(actualWorkspace, scope)
	}
	return toolresponse.WrapSuccess(ctx, "memory", result, start)
}

// loadProjectMemoryState ports loadProjectMemoryState.
func loadProjectMemoryState(workspace, memoryName string, deps Deps) (map[string]any, error) {
	dataDir := filepath.Join(workspace, ".zenmcp")
	dbPath := filepath.Join(dataDir, "context.db")

	if err := deps.Gatekeeper.ValidatePathSafety(workspace, "memory load"); err != nil {
		return nil, err
	}

	state := projectmemory.ReconstructState(dataDir, memoryName)
	stateMap := map[string]any{}
	raw, _ := json.Marshal(state)
	_ = json.Unmarshal(raw, &stateMap)

	lastVisited := state.Timestamp
	if lastVisited == "" {
		lastVisited = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02T15:04:05.000Z")
	}
	stateMap["git_signals"] = projectmemory.GetGitSignals(workspace, lastVisited)

	stateMap["dependency_context"] = loadDependencyContext(workspace)

	projectmemory.RegisterProjectInMap(workspace, nil)

	promptContextFile := filepath.Join(dataDir, "prompt_context.json")
	if raw, err := os.ReadFile(promptContextFile); err == nil {
		var pc any
		if json.Unmarshal(raw, &pc) == nil {
			stateMap["prompt_context"] = pc
		}
	}

	stateMap["recent_commands"] = loadRecentCommands(dbPath)

	return stateMap, nil
}

// loadRecentCommands is a helper function
func loadRecentCommands(dbPath string) []map[string]any {
	commands := projectmemory.RecentCommands(dbPath)
	out := make([]map[string]any, 0, len(commands))
	for _, c := range commands {
		out = append(out, map[string]any{
			"command":   c.Command,
			"timestamp": c.Timestamp,
		})
	}
	return out
}

// loadDependencyContext is a helper function
func loadDependencyContext(workspace string) []map[string]any {
	deps := []map[string]any{}
	mapFile := mcpcfg.MapFilePath()
	if _, err := os.Stat(mapFile); err != nil {
		return deps
	}
	raw, err := os.ReadFile(mapFile)
	if err != nil {
		return deps
	}
	var mapData map[string]any
	if json.Unmarshal(raw, &mapData) != nil {
		return deps
	}
	entry, _ := mapData[workspace].(map[string]any)
	depList, _ := entry["dependencies"].([]any)
	for _, d := range depList {
		depPath, _ := d.(string)
		if depPath == "" {
			continue
		}
		depState := projectmemory.ReconstructState(filepath.Join(depPath, ".zenmcp"), "brain")
		deps = append(deps, map[string]any{
			"workspace":     depPath,
			"session_title": depState.SessionTitle,
			"objective":     depState.Objective,
		})
	}
	return deps
}

// actionSave ports actionSave.
func actionSave(dataDir, memoryName, dbPath, workspace, sessionTitle, objective, sessionNotes string) map[string]any {
	prevState := projectmemory.ReconstructState(dataDir, memoryName)
	prevTitle, prevObjective := "", ""
	if prevState.SessionTitle != "" {
		prevTitle = prevState.SessionTitle
	}
	if prevState.Objective != "" {
		prevObjective = prevState.Objective
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	merged := projectmemory.BrainEvent{
		SchemaVersion: 3,
		Timestamp:     now,
		SessionTitle:  firstNonEmpty(sessionTitle, prevTitle),
		Objective:     firstNonEmpty(objective, prevObjective),
		SessionNotes:  sessionNotes,
	}
	_ = projectmemory.AppendEvent(dataDir, memoryName, merged)

	if merged.SessionNotes != "" {
		projectmemory.IndexActiveMemory(dbPath, []projectmemory.MemoryIndexItem{
			{Type: "task", Title: "Session Notes — " + now, Content: merged.SessionNotes},
		})
	}

	return map[string]any{
		"success":   true,
		"message":   "Project memory saved and indexed successfully.",
		"workspace": workspace,
	}
}

// actionScope ports actionScope.
func actionScope(workspace, scope string) map[string]any {
	mapFile := mcpcfg.MapFilePath()
	if _, err := os.Stat(mapFile); err != nil {
		return map[string]any{"error": true, "message": "No projects registered in map.json yet."}
	}
	raw, err := os.ReadFile(mapFile)
	if err != nil {
		return map[string]any{"error": true, "message": "No projects registered in map.json yet."}
	}
	var mapData map[string]any
	if json.Unmarshal(raw, &mapData) != nil {
		return map[string]any{"error": true, "message": "No projects registered in map.json yet."}
	}
	entry, _ := mapData[workspace].(map[string]any)
	if entry == nil {
		return map[string]any{
			"error":   true,
			"message": "Project path " + workspace + " is not registered in map.json. Please run registerProjectInMap or visit the project first.",
		}
	}

	if scopes, ok := entry["scopes"].(map[string]any); !ok || scopes == nil {
		entry["scopes"] = map[string]any{}
	}

	if scope != "" {
		scopes, _ := entry["scopes"].(map[string]any)
		paths, _ := scopes[scope].([]any)
		if paths == nil {
			return map[string]any{
				"error":   true,
				"message": `Scope "` + scope + `" not found in project ` + workspace + `.`,
			}
		}
		return map[string]any{
			"scope":        scope,
			"paths":        paths,
			"dependencies": dependenciesOrEmpty(entry),
		}
	}

	return map[string]any{
		"project":      workspace,
		"scopes":       entry["scopes"],
		"dependencies": dependenciesOrEmpty(entry),
	}
}

// dependenciesOrEmpty is a helper function
func dependenciesOrEmpty(entry map[string]any) any {
	if deps, ok := entry["dependencies"]; ok && deps != nil {
		return deps
	}
	return []any{}
}
// firstNonEmpty is a helper function
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolveWorkspaceFromDeps ports resolveWorkspace(inputWorkspace, workspace).
func resolveWorkspaceFromDeps(inputWorkspace, workspace string) string {
	if inputWorkspace != "" {
		return inputWorkspace
	}
	cwd, _ := os.Getwd()
	if workspace != "" {
		return workspace
	}
	return cwd
}
