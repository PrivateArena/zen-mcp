package tools

import (
	"context"
	"os"
	"strings"
	"testing"

	"zen-mcp/internal/skills"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// Section retrieval is the difference between reading a 1200-line skill and the
// twenty lines that answer the question, so the handler has to keep the outline and
// the filter in step with the document on disk.

func skillReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "skill",
			Arguments: args,
		},
	}
}

func skillResultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("skill returned no content")
	}
	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return text.Text
}

// skillWithHeadings returns the id of the first bundled skill with at least three
// sections, so the test follows the real corpus instead of a fixture that can drift
// from it.
func skillWithHeadings(t *testing.T) (string, int) {
	t.Helper()
	entries, err := skills.LoadSkills()
	if err != nil || len(entries) == 0 {
		t.Skipf("no bundled skills available: %v", err)
	}
	for _, entry := range entries {
		count := len(skills.ParseSections(readSkillFile(t, entry.ID)))
		if count < 3 {
			continue
		}
		res := HandleSkillsAction(context.Background(), t.TempDir(), Deps{}, skillReq(map[string]any{
			"action": "outline", "id": entry.ID,
		}))
		if res.IsError {
			continue
		}
		return entry.ID, count
	}
	t.Skip("no bundled skill has at least 3 sections")
	return "", 0
}

func readSkillFile(t *testing.T, id string) string {
	t.Helper()
	entry, err := skills.FindSkillByID(id)
	if err != nil {
		t.Skipf("skill %q not found: %v", id, err)
	}
	data, err := os.ReadFile(entry.Path)
	if err != nil {
		t.Skipf("cannot read %s: %v", entry.Path, err)
	}
	return string(data)
}

func TestSkillsOutlineAction(t *testing.T) {
	id, _ := skillWithHeadings(t)

	res := HandleSkillsAction(context.Background(), t.TempDir(), Deps{}, skillReq(map[string]any{
		"action": "outline", "id": id,
	}))
	text := skillResultText(t, res)

	if !strings.Contains(text, "# Outline: "+id) {
		t.Errorf("outline header missing for %q:\n%s", id, text)
	}
	if !strings.Contains(text, "filter_sections") {
		t.Error("outline does not tell the caller how to fetch one section")
	}
	if !outlineLinesCarrySpans(text) {
		t.Error("outline lines do not carry a trailing start:end span")
	}
	for _, s := range skills.ParseSections(readSkillFile(t, id)) {
		if !strings.Contains(text, s.Title) {
			t.Errorf("outline is missing the section %q", s.Title)
		}
	}
}

// outlineLinesCarrySpans checks the body of an outline: "<idx> <level> <heading>
// <start:end>", one line per section, so a range is copy-pasteable into a filter or
// straight into sed.
func outlineLinesCarrySpans(text string) bool {
	lines := strings.Split(text, "\n")
	started := false
	for _, line := range lines {
		if strings.HasPrefix(line, "  1 ") || strings.HasPrefix(strings.TrimLeft(line, " "), "1 ") {
			started = true
		}
		if !started || strings.TrimSpace(line) == "" || strings.HasPrefix(line, "Fetch:") {
			continue
		}
		fields := strings.Fields(line)
		span := fields[len(fields)-1]
		colon := strings.Index(span, ":")
		if colon <= 0 || !isAllDigits(span[:colon]) || !isAllDigits(span[colon+1:]) {
			return false
		}
	}
	return started
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func TestSkillsFilteredReportNamesItsSpan(t *testing.T) {
	id, _ := skillWithHeadings(t)

	res := HandleSkillsAction(context.Background(), t.TempDir(), Deps{}, skillReq(map[string]any{
		"action": "get", "id": id, "filter_sections": []any{"1"},
	}))
	text := skillResultText(t, res)

	// The report line has to carry the span, so the agent can sed the file itself.
	if !strings.Contains(text, "*Sections: 1 of ") || !strings.Contains(text, " (") {
		t.Errorf("filter report lost its span:\n%s", text)
	}
	if !strings.Contains(text, "*File: ") {
		t.Error("filter report does not name the file the omitted spans live in")
	}
}

func TestSkillsGetNarrowsToRequestedSection(t *testing.T) {
	id, _ := skillWithHeadings(t)

	full := HandleSkillsAction(context.Background(), t.TempDir(), Deps{}, skillReq(map[string]any{
		"action": "get", "id": id,
	}))
	fullText := skillResultText(t, full)

	firstSection := ""
	for _, s := range skills.ParseSections(readSkillFile(t, id)) {
		if s.Level == 2 {
			firstSection = s.Title
			break
		}
	}
	if firstSection == "" {
		t.Skipf("skill %q has no level-2 section to filter", id)
	}

	filtered := HandleSkillsAction(context.Background(), t.TempDir(), Deps{}, skillReq(map[string]any{
		"action": "get", "id": id, "filter_sections": []any{firstSection},
	}))
	text := skillResultText(t, filtered)

	if !strings.Contains(text, "## Knowledge Content") {
		t.Error("filtered skill lost its content header")
	}
	if !strings.Contains(text, "*Sections:") {
		t.Error("filtered skill does not report what it kept")
	}
	if strings.Count(text, "\n") >= strings.Count(fullText, "\n") {
		t.Errorf("filter kept %d lines of %d: no savings", strings.Count(text, "\n")+1, strings.Count(fullText, "\n")+1)
	}
}

func TestSkillsGetReportsAnUnmatchedSection(t *testing.T) {
	id, _ := skillWithHeadings(t)

	res := HandleSkillsAction(context.Background(), t.TempDir(), Deps{}, skillReq(map[string]any{
		"action": "get", "id": id, "filter_sections": []any{"definitely not a heading anywhere"},
	}))
	text := skillResultText(t, res)
	if !strings.Contains(text, "skill failed") {
		t.Fatalf("an unmatchable section filter must fail rather than return an empty skill:\n%s", text)
	}
	if !strings.Contains(text, "No section of this skill matches definitely not a heading anywhere") {
		t.Error("unmatched filter error does not name the selector")
	}
	if !strings.Contains(text, "top-level sections:") {
		t.Error("unmatched filter error does not offer the real section names")
	}
}

func TestSkillsStringListArg(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"core actions", "core actions"},
		{"core, zoom ", "core|zoom"},
		{[]any{"a", "b,c"}, "a|b|c"},
		{[]string{"x"}, "x"},
		{nil, ""},
		{"", ""},
	}
	for _, c := range cases {
		got := strings.Join(skillStringListArg(c.in), "|")
		if got != c.want {
			t.Errorf("skillStringListArg(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}
