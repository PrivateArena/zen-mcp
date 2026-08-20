package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zen-mcp/internal/mcpcfg"
)

func setupSkills(t *testing.T) string {
	t.Helper()
	origRoot := mcpcfg.ProjectRoot
	t.Cleanup(func() { mcpcfg.ProjectRoot = origRoot })

	tmpRoot := t.TempDir()
	mcpcfg.ProjectRoot = tmpRoot
	skillsDir := filepath.Join(tmpRoot, "resources", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	content := "# Skill: Codebase Research\n\nStatic research content.\n"
	if err := os.WriteFile(filepath.Join(skillsDir, "codebase-research.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	return tmpRoot
}

func TestResolvePromptSuggestSkillsAppendsSuggestions(t *testing.T) {
	setupSkills(t)

	p := PromptDefinition{
		Name:          "test",
		Arguments:     []PromptArgument{{Name: "i"}},
		Template:      "Task: {{i}}",
		EnabledSkills: []string{"codebase-research"},
		SuggestSkills: boolPtr(true),
	}
	result, err := ResolvePrompt(p, map[string]string{"i": "hello"}, "/ws")
	if err != nil {
		t.Fatalf("ResolvePrompt() error = %v", err)
	}
	if !strings.Contains(result, "SKILL ACTIVATION") {
		t.Errorf("suggest mode missing SKILL ACTIVATION block, got:\n%s", result)
	}
	if !strings.Contains(result, "skill id=codebase-research") {
		t.Errorf("suggest mode missing skill id reminder, got:\n%s", result)
	}
	if strings.Contains(result, "STATIC SKILL CONTEXT") {
		t.Errorf("suggest mode must NOT inject static content, got:\n%s", result)
	}
	if strings.Contains(result, "Static research content.") {
		t.Errorf("suggest mode must NOT inject skill body, got:\n%s", result)
	}
}

func TestResolvePromptStaticInjectionIsDefault(t *testing.T) {
	setupSkills(t)

	p := PromptDefinition{
		Name:          "test",
		Arguments:     []PromptArgument{{Name: "i"}},
		Template:      "Task: {{i}}",
		EnabledSkills: []string{"codebase-research"},
	}
	result, err := ResolvePrompt(p, map[string]string{"i": "hello"}, "/ws")
	if err != nil {
		t.Fatalf("ResolvePrompt() error = %v", err)
	}
	if !strings.Contains(result, "STATIC SKILL CONTEXT") {
		t.Errorf("default mode missing static skill context, got:\n%s", result)
	}
	if !strings.Contains(result, "Static research content.") {
		t.Errorf("default mode missing skill body, got:\n%s", result)
	}
	if strings.Contains(result, "SKILL ACTIVATION") {
		t.Errorf("default mode must not emit suggestion block, got:\n%s", result)
	}
}

func TestSkillActivationBlock(t *testing.T) {
	if got := SkillActivationBlock("skill id=%s", "Use MCP skill id=skill_id to activate following knowledge:", []string{"a", "b"}); got != "\n\n---\n**SKILL ACTIVATION**\n[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:\n- `skill id=a`\n- `skill id=b`" {
		t.Errorf("MCP form mismatch:\n%q", got)
	}
	if got := SkillActivationBlock("zskill -a get -i %s", "Load required skills:", []string{"foo"}); got != "\n\n---\n**SKILL ACTIVATION**\n[IMPORTANT] Load required skills:\n- `zskill -a get -i foo`" {
		t.Errorf("CLI form mismatch:\n%q", got)
	}
	if got := SkillActivationBlock("zskill -a get -i %s", "Load required skills:", nil); got != "" {
		t.Errorf("empty skill list must return empty, got: %q", got)
	}
	if got := SkillActivationBlock("zskill -a get -i %s", "Load required skills:", []string{}); got != "" {
		t.Errorf("empty skill slice must return empty, got: %q", got)
	}
}

func TestResolvePromptSuggestSkillsEmptyList(t *testing.T) {
	p := PromptDefinition{
		Arguments:     []PromptArgument{{Name: "i"}},
		Template:      "Task: {{i}}",
		SuggestSkills: boolPtr(true),
	}
	result, err := ResolvePrompt(p, map[string]string{"i": "hello"}, "/ws")
	if err != nil {
		t.Fatalf("ResolvePrompt() error = %v", err)
	}
	if strings.Contains(result, "SKILL ACTIVATION") {
		t.Errorf("empty enabledSkills must not emit suggestion block, got:\n%s", result)
	}
	if result != "Task: hello" {
		t.Errorf("unexpected result: %q", result)
	}
}

func writeTimeline(t *testing.T, workspace string) {
	t.Helper()
	zenDir := filepath.Join(workspace, ".zenmcp")
	if err := os.MkdirAll(zenDir, 0o755); err != nil {
		t.Fatalf("mkdir .zenmcp: %v", err)
	}
	line := `{"schema_version":3,"timestamp":"2024-01-02T00:00:00.000Z","title":"Port memory","objective":"finish prompts","notes":"## Progress\n- Resolver ported"}`
	if err := os.WriteFile(filepath.Join(zenDir, "brain_timeline.jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write timeline: %v", err)
	}
}

func TestResolvePromptEnableMemoryContext(t *testing.T) {
	ws := t.TempDir()
	writeTimeline(t, ws)

	p := PromptDefinition{
		Name:                "test",
		Template:            "Question: {{i}}",
		Arguments:           []PromptArgument{{Name: "i"}},
		EnableMemoryContext: boolPtr(true),
	}
	result, err := ResolvePrompt(p, map[string]string{"i": "query text"}, ws)
	if err != nil {
		t.Fatalf("ResolvePrompt() error = %v", err)
	}
	if !strings.Contains(result, "Question: query text") {
		t.Errorf("template substitution lost, got:\n%s", result)
	}
	if !strings.Contains(result, "RETRIEVED MEMORY") {
		t.Errorf("memory marker missing, got:\n%s", result)
	}
	if !strings.Contains(result, "Port memory") {
		t.Errorf("latest event title missing, got:\n%s", result)
	}
	if !strings.Contains(result, "finish prompts") {
		t.Errorf("latest event objective missing, got:\n%s", result)
	}
}

func TestResolvePromptEnableMemoryContextNoArgs(t *testing.T) {
	ws := t.TempDir()
	writeTimeline(t, ws)

	p := PromptDefinition{
		Name:                "test",
		Template:            "Empty args",
		EnableMemoryContext: boolPtr(true),
	}
	result, err := ResolvePrompt(p, map[string]string{}, ws)
	if err != nil {
		t.Fatalf("ResolvePrompt() error = %v", err)
	}
	if result != "Empty args" {
		t.Errorf("memory appended with empty args, got:\n%s", result)
	}
}

func TestResolvePromptEnableMemoryContextNoTimeline(t *testing.T) {
	p := PromptDefinition{
		Name:                "test",
		Template:            "Question: {{i}}",
		Arguments:           []PromptArgument{{Name: "i"}},
		EnableMemoryContext: boolPtr(true),
	}
	result, err := ResolvePrompt(p, map[string]string{"i": "query"}, t.TempDir())
	if err != nil {
		t.Fatalf("ResolvePrompt() error = %v", err)
	}
	if strings.Contains(result, "RETRIEVED MEMORY") {
		t.Errorf("memory appended without timeline, got:\n%s", result)
	}
	if result != "Question: query" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestResolvePromptEnableMemoryContextEmptyWorkspace(t *testing.T) {
	p := PromptDefinition{
		Name:                "test",
		Template:            "Question: {{i}}",
		Arguments:           []PromptArgument{{Name: "i"}},
		EnableMemoryContext: boolPtr(true),
	}
	result, err := ResolvePrompt(p, map[string]string{"i": "query"}, "")
	if err != nil {
		t.Fatalf("ResolvePrompt() error = %v", err)
	}
	if strings.Contains(result, "RETRIEVED MEMORY") {
		t.Errorf("memory appended with empty workspace, got:\n%s", result)
	}
}

func TestResolvePromptEnableMemoryContextEmptyEventNotInjected(t *testing.T) {
	ws := t.TempDir()
	zenDir := filepath.Join(ws, ".zenmcp")
	if err := os.MkdirAll(zenDir, 0o755); err != nil {
		t.Fatalf("mkdir .zenmcp: %v", err)
	}
	line := `{"schema_version":3,"timestamp":"2024-01-02T00:00:00.000Z"}`
	if err := os.WriteFile(filepath.Join(zenDir, "brain_timeline.jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write timeline: %v", err)
	}

	p := PromptDefinition{
		Name:                "test",
		Template:            "Question: {{i}}",
		Arguments:           []PromptArgument{{Name: "i"}},
		EnableMemoryContext: boolPtr(true),
	}
	result, err := ResolvePrompt(p, map[string]string{"i": "query"}, ws)
	if err != nil {
		t.Fatalf("ResolvePrompt() error = %v", err)
	}
	if strings.Contains(result, "RETRIEVED MEMORY") {
		t.Errorf("empty event injected into prompt, got:\n%s", result)
	}
}
