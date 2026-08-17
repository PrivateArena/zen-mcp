package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"zen-mcp/internal/toolresponse"
	"zen-mcp/internal/whiteboard"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func defMemoryIsolate(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "memory_isolate",
		Title:       "Memory Isolate",
		Description: "Persistent project state via isolated whiteboard (port 3034). Each project owns a segregated SQLite DB. Actions: load (full board map or single-card drill-down), save.",
		Schema: jsonSchema(map[string]any{
			"action":    strEnumProp("Action", []string{"load", "save"}),
			"title":     strProp("[save] One-line label, only if changed"),
			"objective": strProp("[save] 1-2 sentence goal, only if changed"),
			"notes":     strProp("[save] This session's notes as markdown"),
			"card_slug": strProp("[load] Card slug for single-card drill-down (omit for full board map)"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleMemoryIsolateAction(ctx, workspace, deps, req), nil
		},
	}
}

func HandleMemoryIsolateAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)
	inputWorkspace, _ := args["workspace"].(string)

	actualWorkspace := resolveWorkspaceFromDeps(inputWorkspace, workspace)
	if actualWorkspace == "" {
		return toolresponse.WrapErrorWithContext(ctx, "memory_isolate",
			fmt.Errorf("Workspace path is required but could not be determined."), start)
	}

	slugInfo := whiteboard.ResolveProjectSlug(actualWorkspace)
	client := whiteboard.NewClient("http://127.0.0.1:3034", slugInfo.Slug, slugInfo.Title, slugInfo.Slug)

	switch action {
	case "load":
		return HandleIsolateLoad(ctx, client, actualWorkspace, args, start)
	case "save":
		return HandleIsolateSave(ctx, client, actualWorkspace, args, start)
	default:
		return toolresponse.WrapErrorWithContext(ctx, "memory_isolate", fmt.Errorf("Unknown action: %s", action), start)
	}
}

func HandleIsolateLoad(ctx context.Context, client *whiteboard.Client, ws string, args map[string]any, start time.Time) *mcp.CallToolResult {
	state, err := client.LoadBoardState(ctx)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "memory_isolate", err, start)
	}
	if cardSlug, ok := args["card_slug"].(string); ok && cardSlug != "" {
		target := findCard(state.Cards, cardSlug)
		if target == nil {
			return toolresponse.WrapErrorWithContext(ctx, "memory_isolate", fmt.Errorf("Card not found: %s", cardSlug), start)
		}
		neighbors := findNeighbors(state, cardSlug)
		return toolresponse.WrapSuccess(ctx, "memory_isolate", map[string]any{"card": target, "neighbors": neighbors}, start)
	}
	return toolresponse.WrapSuccess(ctx, "memory_isolate", map[string]any{
		"board": map[string]any{
			"slug": state.Slug, "title": state.Title,
			"sections": state.Sections, "viewport": state.Viewport,
		},
		"cards":       state.Cards,
		"connections": state.Connections,
	}, start)
}

func HandleIsolateSave(ctx context.Context, client *whiteboard.Client, ws string, args map[string]any, start time.Time) *mcp.CallToolResult {
	sessionTitle, _ := args["title"].(string)
	sessionNotes, _ := args["notes"].(string)
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
		return toolresponse.WrapErrorWithContext(ctx, "memory_isolate", err, start)
	}
	if err := client.UpsertCard(ctx, cardSlug, sessionTitle, sessionNotes, slugInfo.Title); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "memory_isolate", err, start)
	}
	return toolresponse.WrapSuccess(ctx, "memory_isolate", map[string]any{
		"saved":      true,
		"whiteboard": slugInfo.Slug,
		"slug":       cardSlug,
	}, start)
}

func findCard(cards []whiteboard.CardData, slug string) *whiteboard.CardData {
	for i := range cards {
		if cards[i].CardSlug == slug {
			return &cards[i]
		}
	}
	return nil
}

func findNeighbors(state whiteboard.BoardState, slug string) map[string]any {
	index := map[string]whiteboard.CardData{}
	for _, c := range state.Cards {
		index[c.CardSlug] = c
	}
	neighbors := map[string]any{}
	for _, conn := range state.Connections {
		if conn.From == slug {
			if n, ok := index[conn.To]; ok {
				neighbors[conn.To] = map[string]any{
					"card_slug":  n.CardSlug,
					"title":      n.Title,
					"created_at": n.CreatedAt,
					"connection": map[string]any{"fromPort": conn.FromPort, "toPort": conn.ToPort},
				}
			}
		}
		if conn.To == slug {
			if n, ok := index[conn.From]; ok {
				neighbors[conn.From] = map[string]any{
					"card_slug":  n.CardSlug,
					"title":      n.Title,
					"created_at": n.CreatedAt,
					"connection": map[string]any{"fromPort": conn.FromPort, "toPort": conn.ToPort},
				}
			}
		}
	}
	return neighbors
}

func uniqueGroups(cards []whiteboard.CardData) []string {
	set := map[string]bool{}
	for _, c := range cards {
		g := c.Group
		if g == "" {
			g = "session"
		}
		set[g] = true
	}
	out := make([]string, 0, len(set))
	for g := range set {
		out = append(out, g)
	}
	return out
}

func filterByGroup(cards []whiteboard.CardData, group string) []whiteboard.CardData {
	out := make([]whiteboard.CardData, 0)
	for _, c := range cards {
		g := c.Group
		if g == "" {
			g = "session"
		}
		if g == group {
			out = append(out, c)
		}
	}
	return out
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = regexp.MustCompile(`^-+|-+$`).ReplaceAllString(s, "")
	return s
}
