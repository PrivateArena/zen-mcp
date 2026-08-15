package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolresponse"
	"zen-mcp/internal/whiteboard"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// defMemoryShared is a helper function
func defMemoryShared(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "memory_shared",
		Title:       "Memory Shared",
		Description: "Persistent project state via shared whiteboard (port 3035). All projects share one DB with named whiteboards and inter-project link_cards. Actions: load (full board map or single-card drill-down), save, scope.",
		Schema: jsonSchema(map[string]any{
			"action":        strEnumProp("Action", []string{"load", "save", "scope"}),
			"workspace":     strProp("Project path (default: current session workspace)"),
			"session_title": strProp("[save] One-line label, only if changed"),
			"objective":     strProp("[save] 1-2 sentence goal, only if changed"),
			"session_notes": strProp("[save] This session's notes as markdown"),
			"scope":         strProp("[scope] Scope ID to view/update"),
			"card_slug":     strProp("[load] Card slug for single-card drill-down (omit for full board map)"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleMemorySharedAction(ctx, workspace, deps, req), nil
		},
	}
}

// HandleMemorySharedAction is a helper function
func HandleMemorySharedAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)
	inputWorkspace, _ := args["workspace"].(string)

	actualWorkspace := resolveWorkspaceFromDeps(inputWorkspace, workspace)
	if actualWorkspace == "" {
		return toolresponse.WrapErrorWithContext(ctx, "memory_shared",
			fmt.Errorf("Workspace path is required but could not be determined."), start)
	}

	slugInfo := whiteboard.ResolveProjectSlug(actualWorkspace)
	client := whiteboard.NewClient("http://127.0.0.1:3035", slugInfo.Slug, slugInfo.Title, slugInfo.Slug)

	switch action {
	case "load":
		return HandleSharedLoad(ctx, client, actualWorkspace, args, start)
	case "save":
		return HandleSharedSave(ctx, client, actualWorkspace, args, start)
	case "scope":
		return HandleSharedScope(ctx, client, actualWorkspace, args, start)
	default:
		return toolresponse.WrapErrorWithContext(ctx, "memory_shared", fmt.Errorf("Unknown action: %s", action), start)
	}
}

// HandleSharedLoad is a helper function
func HandleSharedLoad(ctx context.Context, client *whiteboard.Client, ws string, args map[string]any, start time.Time) *mcp.CallToolResult {
	state, err := client.LoadBoardState(ctx)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "memory_shared", err, start)
	}
	if cardSlug, ok := args["card_slug"].(string); ok && cardSlug != "" {
		target := findCard(state.Cards, cardSlug)
		if target == nil {
			return toolresponse.WrapErrorWithContext(ctx, "memory_shared", fmt.Errorf("Card not found: %s", cardSlug), start)
		}
		neighbors := findNeighbors(state, cardSlug)
		return toolresponse.WrapSuccess(ctx, "memory_shared", map[string]any{"card": target, "neighbors": neighbors}, start)
	}

	related := loadRelatedProjects()
	slugInfo := whiteboard.ResolveProjectSlug(ws)
	depSlugs := related[slugInfo.Slug]
	if depSlugs == nil {
		depSlugs = []string{}
	}

	type depResult struct {
		Workspace    string
		SessionTitle string
		Objective    string
		Warn         string
	}

	depResults := make([]depResult, 0, len(depSlugs))
	for _, depSlug := range depSlugs {
		depClient := whiteboard.NewClient("http://127.0.0.1:3035", depSlug, depSlug, depSlug)
		depState, depErr := depClient.LoadBoardState(ctx)
		if depErr != nil {
			depResults = append(depResults, depResult{Workspace: depSlug, Warn: "unreachable"})
			continue
		}
		cards := depState.Cards
		latestTitle := ""
		latestDesc := ""
		if len(cards) > 0 {
			latestTitle = cards[len(cards)-1].Title
			latestDesc = cards[len(cards)-1].Description
		}
		depResults = append(depResults, depResult{
			Workspace:    depSlug,
			SessionTitle: latestTitle,
			Objective:    latestDesc,
		})
	}

	dependencyContext := make([]any, 0, len(depResults))
	for _, r := range depResults {
		entry := map[string]any{
			"workspace":     r.Workspace,
			"session_title": r.SessionTitle,
			"objective":     r.Objective,
		}
		if r.Warn != "" {
			entry["warn"] = r.Warn
		}
		dependencyContext = append(dependencyContext, entry)
	}

	return toolresponse.WrapSuccess(ctx, "memory_shared", map[string]any{
		"board": map[string]any{
			"slug": state.Slug, "title": state.Title,
			"sections": state.Sections, "viewport": state.Viewport,
		},
		"cards":              state.Cards,
		"connections":        state.Connections,
		"dependency_context": dependencyContext,
	}, start)
}

// HandleSharedSave is a helper function
func HandleSharedSave(ctx context.Context, client *whiteboard.Client, ws string, args map[string]any, start time.Time) *mcp.CallToolResult {
	sessionTitle, _ := args["session_title"].(string)
	sessionNotes, _ := args["session_notes"].(string)
	slugInfo := whiteboard.ResolveProjectSlug(ws)

	ts := time.Now().UTC().Format("2006-01-02-150405")
	slugified := ""
	if sessionTitle != "" {
		slugified = slugify(sessionTitle)
	}
	cardSlug := slugInfo.Slug + "-" + slugified + "-" + ts
	if slugified == "" {
		cardSlug = slugInfo.Slug + "-unnamed-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}

	if err := client.EnsureBoard(ctx); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "memory_shared", err, start)
	}
	if err := client.UpsertCard(ctx, cardSlug, sessionTitle, sessionNotes, slugInfo.Title); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "memory_shared", err, start)
	}

	related := loadRelatedProjects()
	targets := related[slugInfo.Slug]
	if targets == nil {
		targets = []string{}
	}
	var warnMsg string

	linkResults := make([]error, 0, len(targets))
	for _, target := range targets {
		linkErr := client.LinkCards(ctx, cardSlug, target+"-summary")
		linkResults = append(linkResults, linkErr)
	}

	failedLinks := make([]string, 0)
	linkedProjects := make([]string, 0)
	for i, target := range targets {
		if linkResults[i] != nil {
			failedLinks = append(failedLinks, target)
		} else {
			linkedProjects = append(linkedProjects, target)
		}
	}
	if len(failedLinks) > 0 {
		warnMsg = "link_cards failed for: " + strings.Join(failedLinks, ", ")
	}

	result := map[string]any{
		"saved":      true,
		"whiteboard": slugInfo.Slug,
		"slug":       cardSlug,
	}
	if len(linkedProjects) > 0 {
		result["linkedProjects"] = linkedProjects
	}
	if warnMsg != "" {
		result["warn"] = warnMsg
	}
	return toolresponse.WrapSuccess(ctx, "memory_shared", result, start)
}

// HandleSharedScope is a helper function
func HandleSharedScope(ctx context.Context, client *whiteboard.Client, ws string, args map[string]any, start time.Time) *mcp.CallToolResult {
	state, err := client.LoadBoardState(ctx)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "memory_shared", err, start)
	}
	cards := state.Cards
	groups := uniqueGroups(cards)

	if scope, ok := args["scope"].(string); ok && scope != "" {
		groupCards := filterByGroup(cards, scope)
		return toolresponse.WrapSuccess(ctx, "memory_shared", map[string]any{
			"scope":     scope,
			"cardCount": len(groupCards),
			"cards":     groupCards,
			"groups":    groups,
		}, start)
	}

	related := loadRelatedProjects()
	slugInfo := whiteboard.ResolveProjectSlug(ws)
	return toolresponse.WrapSuccess(ctx, "memory_shared", map[string]any{
		"project":    ws,
		"whiteboard": slugInfo.Slug,
		"groups":     groups,
		"totalCards": len(cards),
		"relatedProjects": func() []string {
			if r, ok := related[slugInfo.Slug]; ok {
				return r
			}
			return []string{}
		}(),
	}, start)
}

// loadRelatedProjects is a helper function
func loadRelatedProjects() map[string][]string {
	configPath := filepath.Join(mcpcfg.ProjectRoot, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return map[string][]string{}
	}
	var cfg map[string]any
	if json.Unmarshal(data, &cfg) != nil {
		return map[string][]string{}
	}
	related, ok := cfg["related_projects"].(map[string]any)
	if !ok {
		return map[string][]string{}
	}
	out := map[string][]string{}
	for k, v := range related {
		if arr, ok := v.([]any); ok {
			strs := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					strs = append(strs, s)
				}
			}
			out[k] = strs
		}
	}
	return out
}
