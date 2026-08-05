package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/jang/zen-mcp/internal/mcpcfg"
	"github.com/jang/zen-mcp/internal/toolresponse"
)

func defSkills(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "skill",
		Title:       "Agent Skill System",
		Description: "Retrieve a skill by its ID.",
		Schema: jsonSchema(map[string]any{
			"id": strProp("Skill ID"),
		}, []string{"id"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSkillsAction(ctx, workspace, deps, req), nil
		},
	}
}

func HandleSkillsAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	id, _ := args["id"].(string)

	if id == "" {
		return toolresponse.WrapErrorWithContext(ctx, "skill", fmt.Errorf("Skill ID is required"), start)
	}

	skillsDir := filepath.Join(mcpcfg.ProjectRoot, "resources", "skills")
	skillPath := filepath.Join(skillsDir, id+".md")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		skillPath = filepath.Join(skillsDir, id, "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			return toolresponse.WrapErrorWithContext(ctx, "skill", fmt.Errorf("Skill \"%s\" not found.", id), start)
		}
	}

	content, err := os.ReadFile(skillPath)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "skill", err, start)
	}

	markdown := fmt.Sprintf("# Skill: %s\n*Framework: unspecified*\n\n## Knowledge Content\n\n%s\n", id, string(content))
	return toolresponse.WrapSuccess(ctx, "skill", strings.TrimSpace(markdown), start)
}
