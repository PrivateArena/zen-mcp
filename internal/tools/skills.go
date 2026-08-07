package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zen-mcp/internal/skills"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func defSkills(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "skill",
		Title:       "Agent Skill System",
		Description: "Retrieve a skill by its ID.",
		Schema: jsonSchema(map[string]any{
			"action": strEnumProp("Skill action.", []string{"list", "get"}),
			"id":     strProp("Skill ID"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSkillsAction(ctx, workspace, deps, req), nil
		},
	}
}

func HandleSkillsAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)
	id, _ := args["id"].(string)

	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		return handleSkillsList(ctx, start)
	case "get":
		return handleSkillsGet(ctx, id, start)
	default:
		return toolresponse.WrapErrorWithContext(ctx, "skill", fmt.Errorf("Unknown action: %s", action), start)
	}
}

func handleSkillsList(ctx context.Context, start time.Time) *mcp.CallToolResult {
	skillList, err := skills.LoadSkills()
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "skill", err, start)
	}

	var sb strings.Builder
	sb.WriteString("# Standardized Skills and Patterns\n\n")
	sb.WriteString("Below are the standardized skills and patterns you must follow. If a task matches a description, retrieve it.\n\n")
	sb.WriteString("## Available Skills\n\n")

	for _, s := range skillList {
		sb.WriteString(fmt.Sprintf("- **%s** (%s) - *%s*\n", s.ID, s.Title, s.Framework))
		sb.WriteString(fmt.Sprintf("  %s\n", s.Description))
		sb.WriteString(fmt.Sprintf("  *Usage:* `skill (action=\"get\", id=\"%s\")`\n\n", s.ID))
	}

	return toolresponse.WrapSuccess(ctx, "skill", strings.TrimSpace(sb.String()), start)
}

func handleSkillsGet(ctx context.Context, id string, start time.Time) *mcp.CallToolResult {
	if id == "" {
		return toolresponse.WrapErrorWithContext(ctx, "skill", fmt.Errorf("Skill ID is required for get"), start)
	}

	skill, err := skills.FindSkillByID(id)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "skill", err, start)
	}

	skillPath := skill.Path
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "skill", err, start)
	}

	skillDir := filepath.Dir(skillPath)
	resolved := skills.ResolveSkillContent(string(content), skillDir, id)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Skill: %s (%s)\n", skill.Title, skill.ID))
	sb.WriteString(fmt.Sprintf("*Framework: %s*\n", skill.Framework))
	sb.WriteString("\n## Knowledge Content\n\n")
	sb.WriteString(resolved.Enriched)

	return toolresponse.WrapSuccess(ctx, "skill", strings.TrimSpace(sb.String()), start)
}
