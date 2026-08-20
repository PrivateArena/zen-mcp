package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/prompts"
	"zen-mcp/internal/terminal"
)

func setupPromptsTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = dir
	t.Cleanup(func() { mcpcfg.ProjectRoot = old })

	var buf strings.Builder
	oldOut := terminal.LogOut
	terminal.LogOut = &buf
	t.Cleanup(func() { terminal.LogOut = oldOut })

	return dir
}

func writePromptDefs(t *testing.T, dir string, content string) {
	t.Helper()
	dir = filepath.Join(dir, "resources", "prompts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pi-test.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConvertPiPlaceholders(t *testing.T) {
	cases := []struct {
		name     string
		template string
		args     []prompts.PromptArgument
		want     string
	}{
		{
			name:     "singleArgBecomesPositional",
			template: "Task: {{i}}",
			args:     []prompts.PromptArgument{{Name: "i"}},
			want:     "Task: $1",
		},
		{
			name:     "multipleArgsBecomePositions",
			template: "in={{i}} out={{o}} patch={{patch-path}} in-again={{i}}",
			args: []prompts.PromptArgument{
				{Name: "i"}, {Name: "o"}, {Name: "patch-path"},
			},
			want: "in=$1 out=$2 patch=$3 in-again=$1",
		},
		{
			name:     "unknownPlaceholderPreserved",
			template: "You are {{PERSONA}}. Task: {{i}}",
			args:     []prompts.PromptArgument{{Name: "i"}},
			want:     "You are {{PERSONA}}. Task: $1",
		},
		{
			name:     "noArgsLeavesTemplateUntouched",
			template: "Raw {{i}} with awk $1 $2",
			args:     nil,
			want:     "Raw {{i}} with awk $1 $2",
		},
		{
			name:     "unclosedPlaceholderPreserved",
			template: "open {{never closed",
			args:     []prompts.PromptArgument{{Name: "never"}},
			want:     "open {{never closed",
		},
		{
			name:     "malformedPlaceholderPreserved",
			template: `{{"{{var}}`,
			args:     []prompts.PromptArgument{{Name: "var"}},
			want:     `{{"{{var}}`,
		},
		{
			name:     "emptyTemplateStaysEmpty",
			template: "",
			args:     []prompts.PromptArgument{{Name: "i"}},
			want:     "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := convertPiPlaceholders(tc.template, tc.args); got != tc.want {
				t.Errorf("convertPiPlaceholders(%q) = %q, want %q", tc.template, got, tc.want)
			}
		})
	}
}

const piTestDefs = `- name: pi-test-multi
  description: Multi argument prompt
  arguments:
    - name: i
      description: The input
      required: true
    - name: o
      description: The output
      required: false
  template: |-
    Process {{i}} into {{o}}.
    bash: awk '{print $1}' input

- name: pi-test-noargs
  description: No argument prompt
  template: |-
    Just a fixed instruction.
`

func TestGeneratePiPromptsWritesFiles(t *testing.T) {
	dir := setupPromptsTest(t)
	writePromptDefs(t, dir, piTestDefs)

	if err := generatePiPrompts(); err != nil {
		t.Fatalf("generatePiPrompts() error = %v", err)
	}

	piDir := filepath.Join(dir, "resources", "prompts-pi")
	multiPath := filepath.Join(piDir, "pi-test-multi.md")
	data, err := os.ReadFile(multiPath)
	if err != nil {
		t.Fatalf("read %s: %v", multiPath, err)
	}
	content := string(data)

	if !strings.Contains(content, "description: Multi argument prompt") {
		t.Errorf("missing description in frontmatter:\n%s", content)
	}
	if !strings.Contains(content, "argument-hint: <i> [o]") {
		t.Errorf("argument-hint must mark required <i> and optional [o]:\n%s", content)
	}
	if strings.Contains(content, "{{i}}") || strings.Contains(content, "{{o}}") {
		t.Errorf("named placeholders must be converted to positional args:\n%s", content)
	}
	if !strings.Contains(content, "Process $1 into $2.") {
		t.Errorf("body must use $1/$2 positional args:\n%s", content)
	}
	if !strings.Contains(content, "awk '{print $1}' input") {
		t.Errorf("pre-existing regex-like $1 text must be preserved:\n%s", content)
	}

	noargsPath := filepath.Join(piDir, "pi-test-noargs.md")
	data, err = os.ReadFile(noargsPath)
	if err != nil {
		t.Fatalf("read %s: %v", noargsPath, err)
	}
	noargs := string(data)
	if strings.Contains(noargs, "argument-hint") {
		t.Errorf("no-argument prompt must not emit argument-hint:\n%s", noargs)
	}
	if !strings.Contains(noargs, "Just a fixed instruction.") {
		t.Errorf("no-argument template must be exported verbatim:\n%s", noargs)
	}
}

func TestGeneratePiPromptsMissingDirLogsError(t *testing.T) {
	dir := setupPromptsTest(t)
	_ = dir

	var buf strings.Builder
	oldOut := terminal.LogOut
	terminal.LogOut = &buf
	t.Cleanup(func() { terminal.LogOut = oldOut })

	if err := generatePiPrompts(); err != nil {
		t.Fatalf("generatePiPrompts() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Prompts directory not found") {
		t.Errorf("missing error log: %q", buf.String())
	}
}

func TestGeneratePiPromptsSkillBodyReplacesTemplate(t *testing.T) {
	dir := setupPromptsTest(t)
	if err := os.MkdirAll(filepath.Join(dir, "resources", "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Join(dir, "resources", "skills", "foo")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillContent := "# Skill: foo\n\n---\nname: foo\ndescription: Test skill\n---\n\nDo the foo thing carefully.\n"
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := generatePiPrompts(); err != nil {
		t.Fatalf("generatePiPrompts() error = %v", err)
	}

	path := filepath.Join(dir, "resources", "prompts-pi", "_foo.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(data)

	if !strings.Contains(content, "Do the foo thing carefully.") {
		t.Errorf("skill-enabled prompt body must be the skill content:\n%s", content)
	}
	if strings.Contains(content, "{{i}}") || strings.Contains(content, "Task: $1") {
		t.Errorf("skill body must replace the {{i}} template:\n%s", content)
	}
}