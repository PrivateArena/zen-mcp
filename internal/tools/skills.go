package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/prompts"
	"zen-mcp/internal/skills"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func defSkills(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "skill",
		Title:       "Agent Skill System",
		Description: "Retrieve a skill by its ID, whole or narrowed down to the markdown sections that were asked for.",
		Schema: jsonSchema(map[string]any{
			"action":          strEnumProp("Skill action. list: every skill with its description. get: the skill body, optionally narrowed by filter_sections. outline: one line per section of a skill - <idx> <level> <heading> <start:end> - to pick a section, or to read a span off the file with sed.", []string{"list", "get", "outline"}),
			"id":              strProp("Skill ID"),
			"filter_sections": arrayStringProp("[get] Return only the matching sections instead of the whole skill. A selector is a heading phrase (\"core actions\"), a heading path (\"navigation/navigate\"), an outline number or a start:end line range from action=outline; a match includes that section's subsections. Comma-separated list or repeat the flag."),
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
	sections := skillStringListArg(args["filter_sections"])

	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		if len(sections) > 0 {
			return toolresponse.WrapErrorWithContext(ctx, "skill", fmt.Errorf("filter_sections needs action=get: list returns the skill catalogue, which has no sections"), start)
		}
		return handleSkillsList(ctx, start)
	case "get":
		return handleSkillsGet(ctx, id, sections, start)
	case "outline":
		if len(sections) > 0 {
			return toolresponse.WrapErrorWithContext(ctx, "skill", fmt.Errorf("filter_sections needs action=get: outline already lists every section"), start)
		}
		return handleSkillsOutline(ctx, id, start)
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
	sb.WriteString("A long skill does not have to come back whole: `outline` lists its sections, `get` with `filter_sections` returns only the ones you need.\n\n")
	sb.WriteString("## Available Skills\n\n")

	for _, s := range skillList {
		sb.WriteString(fmt.Sprintf("- **%s** (%s) - *%s*\n", s.ID, s.Title, s.Framework))
		sb.WriteString(fmt.Sprintf("  %s\n", s.Description))
		sb.WriteString(fmt.Sprintf("  *Usage:* `skill (action=\"get\", id=\"%s\")`\n\n", s.ID))
	}

	return toolresponse.WrapSuccess(ctx, "skill", strings.TrimSpace(sb.String()), start)
}

// loadSkill reads one skill's registry entry and its markdown file.
func loadSkill(id, action string) (skills.SkillRegistryEntry, string, error) {
	if id == "" {
		return skills.SkillRegistryEntry{}, "", fmt.Errorf("Skill ID is required for %s", action)
	}
	skill, err := skills.FindSkillByID(id)
	if err != nil {
		return skill, "", err
	}
	raw, err := os.ReadFile(skill.Path)
	if err != nil {
		return skill, "", err
	}
	return skill, string(raw), nil
}

func handleSkillsGet(ctx context.Context, id string, sections []string, start time.Time) *mcp.CallToolResult {
	skill, content, err := loadSkill(id, "get")
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "skill", err, start)
	}

	// Filter before enrichment, so the "Referenced Files" block the resolver appends
	// survives a filter that never named it.
	var filter *skills.SectionFilterResult
	if len(sections) > 0 {
		result := skills.FilterSections(content, sections)
		if len(result.Blocks) == 0 {
			return toolresponse.WrapErrorWithContext(ctx, "skill", skillNoSectionError(content, sections), start)
		}
		content = result.Content
		filter = &result
	}

	skillDir := filepath.Dir(skill.Path)
	resolved := skills.ResolveSkillContent(content, skillDir, id)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Skill: %s (%s)\n", skill.Title, skill.ID))
	sb.WriteString(fmt.Sprintf("*Framework: %s*\n", skill.Framework))
	if filter != nil {
		sb.WriteString(skillFilterLine(skill.Path, filter) + "\n")
	}
	sb.WriteString("\n## Knowledge Content\n\n")
	sb.WriteString(resolved.Enriched)

	result := strings.TrimSpace(sb.String())
	if mcpcfg.Get().Mcp2Cli {
		result = prompts.TransformMCPToCLI(result)
	}

	return toolresponse.WrapSuccess(ctx, "skill", result, start)
}

func handleSkillsOutline(ctx context.Context, id string, start time.Time) *mcp.CallToolResult {
	skill, content, err := loadSkill(id, "outline")
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "skill", err, start)
	}

	sections := skills.ParseSections(content)
	outline := skills.SectionOutline(sections)
	if outline == "" {
		return toolresponse.WrapSuccess(ctx, "skill", fmt.Sprintf("Skill %s has no markdown headings; get it whole with skill (action=\"get\", id=%q).", skill.ID, skill.ID), start)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Outline: %s (%s)\n", skill.Title, skill.ID))
	sb.WriteString(fmt.Sprintf("*%d sections · %d lines · %s*\n", len(sections), skills.LineCount(content), skill.Path))
	sb.WriteString("*<idx> <level> <heading> <start:end>*\n\n")
	sb.WriteString(outline)
	sb.WriteString(fmt.Sprintf("\nFetch: skill (action=\"get\", id=%q, filter_sections=[\"<idx | heading | start:end>\"], or sed -n '<start>,<end>p' %s)", skill.ID, skill.Path))

	return toolresponse.WrapSuccess(ctx, "skill", strings.TrimSpace(sb.String()), start)
}

// skillFilterLine reports which sections came back and where they sit in the file - so
// a narrowed skill tells the agent how much of the document it did not see, and names
// the file the omitted spans can be read out of with `sed`.
func skillFilterLine(path string, filter *skills.SectionFilterResult) string {
	line := fmt.Sprintf("*Sections: %d of %d · %d (%s) of %d lines · %s*",
		filter.Kept, filter.Sections, filter.Lines, strings.Join(filter.Spans, " "), filter.Total,
		strings.Join(filter.Blocks, " | "))
	if len(filter.Unmatched) > 0 {
		line += fmt.Sprintf("\n*No section matched: %s*", strings.Join(filter.Unmatched, ", "))
	}
	return line + fmt.Sprintf("\n*File: %s*", path)
}

func skillNoSectionError(content string, selectors []string) error {
	msg := fmt.Sprintf("No section of this skill matches %s", strings.Join(selectors, ", "))
	sections := skills.ParseSections(content)
	if len(sections) == 0 {
		return fmt.Errorf("%s: this skill has no markdown sections, get it whole with filter_sections omitted", msg)
	}
	if suggestions := skills.SuggestSections(sections, selectors, 5); len(suggestions) > 0 {
		msg += "; closest sections: " + strings.Join(suggestions, " | ")
	}
	titles := skills.TopSectionTitles(sections)
	if len(titles) > 12 {
		titles = titles[:12]
	}
	if len(titles) > 0 {
		msg += "; top-level sections: " + strings.Join(titles, " | ")
	}
	return fmt.Errorf("%s", msg)
}

// skillStringListArg reads a param that accepts one string, a []string, or a JSON
// array of them, and splits comma-separated values so a shell flag works unchanged.
func skillStringListArg(v any) []string {
	var items []string
	switch val := v.(type) {
	case string:
		items = []string{val}
	case []string:
		items = val
	case []any:
		for _, e := range val {
			if s, ok := e.(string); ok {
				items = append(items, s)
			}
		}
	}

	var out []string
	for _, item := range items {
		for _, part := range strings.Split(item, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}
