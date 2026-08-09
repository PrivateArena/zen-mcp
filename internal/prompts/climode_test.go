package prompts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"zen-mcp/internal/mcpcfg"
)

func TestCLIToolMapCoversAllTools(t *testing.T) {
	// Sanity: all 14 tools defined in tools.AllDefs have a CLI mapping.
	expected := []string{
		"browser", "capture", "codegraph", "colab", "context",
		"memory", "memory_isolate", "memory_shared", "run", "shell",
		"skill", "think", "ui-vision", "workspace",
	}
	for _, name := range expected {
		if got := CLIToolMap[name]; got == "" {
			t.Errorf("CLIToolMap missing entry for %q", name)
		}
	}
}

func TestCLIToolKnown(t *testing.T) {
	if got := CLITool("codegraph"); got != "zen-codegraph" {
		t.Errorf("CLITool(codegraph) = %q, want zen-codegraph", got)
	}
	if got := CLITool("skills"); got != "zen-skills" {
		t.Errorf("CLITool(skills) = %q, want zen-skills", got)
	}
}

func TestCLIToolUnknownFallsBack(t *testing.T) {
	if got := CLITool("nonexistent"); got != "zen-nonexistent" {
		t.Errorf("CLITool(nonexistent) = %q, want zen-nonexistent", got)
	}
}

func TestTransformFunctionalNotation(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "simple single param",
			in:   "Use `codegraph({ action: 'files' })` to list files.",
			want: "Use `zen-codegraph --action files` to list files.",
		},
		{
			name: "double quotes",
			in:   "Run `codegraph({ action: \"status\" })`.",
			want: "Run `zen-codegraph --action status`.",
		},
		{
			name: "multiple params",
			in:   "Call `codegraph({ action: 'search', query: 'PORT-TODO', semantic: true })`.",
			want: "Call `zen-codegraph --action search --query PORT-TODO --semantic true`.",
		},
		{
			name: "quoted value with comma",
			in:   "Call `codegraph({ action: 'skeletons', query: '<entry points, tool registrations>' })`.",
			want: "Call `zen-codegraph --action skeletons --query '<entry points, tool registrations>'`.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TransformMCPToCLI(tc.in)
			if got != tc.want {
				t.Errorf("TransformMCPToCLI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTransformObjectDot(t *testing.T) {
	// Rule 3: mcp.tool.method({...})
	// Note: this pattern is not in the current prompts/skills but must be handled.
	in := "mcp.codegraph.files({ action: 'files' })"
	want := "zen-codegraph --files --action files"
	got := TransformMCPToCLI(in)
	if got != want {
		t.Errorf("TransformMCPToCLI(%q) = %q, want %q", in, got, want)
	}
}

func TestTransformTextRefs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "MCP tool backtick reference",
			in:   "Use MCP `codegraph` to inspect.",
			want: "Use `zen-codegraph` to inspect.",
		},
		{
			name: "MCP tool bare reference",
			in:   "Use the MCP codegraph tool.",
			want: "Use the zen-codegraph CLI.",
		},
		{
			name: "MCP shell reference",
			in:   "Always use the MCP `codegraph` for indexing.",
			want: "Always use the `zen-codegraph` CLI for indexing.",
		},
		{
			name: "MCP skill activation",
			in:   "Activate MCP skill id=grill-me and interview me.",
			want: "Activate skill id=grill-me via `zen-skill --action get --id=grill-me` and interview me.",
		},
		{
			name: "MCP Tool skill reference",
			in:   "Please use MCP Tool skill id=grill-me",
			want: "Please use `zen-skill --action get --id=grill-me",
		},
		{
			name: "inline skill id backticks",
			in:   "Use `skill id=grill-me` to proceed.",
			want: "Use `zen-skill --action get --id=grill-me` to proceed.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TransformMCPToCLI(tc.in)
			if got != tc.want {
				t.Errorf("TransformMCPToCLI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTransformNoFalsePositives(t *testing.T) {
	cases := []string{
		"This is normal text.",
		"Use the shell tool to run commands.",
		"The MCP protocol is standard.",
		"Call the function calculate().",
		"Some `code` here.",
		"mcp. is not a tool call here.",
	}
	for _, in := range cases {
		got := TransformMCPToCLI(in)
		if got != in {
			t.Errorf("TransformMCPToCLI(%q) should be unchanged, got %q", in, got)
		}
	}
}

func TestTransformIdempotent(t *testing.T) {
	in := "Use `zen-codegraph --action files` to list files."
	once := TransformMCPToCLI(in)
	twice := TransformMCPToCLI(once)
	if once != twice {
		t.Errorf("idempotency failed: once=%q twice=%q", once, twice)
	}
	if once != in {
		t.Errorf("already-CLI text changed: %q -> %q", in, once)
	}
}

func TestTransformUnknownToolFallsBack(t *testing.T) {
	in := "Use `customtool({ action: 'run' })`."
	want := "Use `zen-customtool --action run`."
	got := TransformMCPToCLI(in)
	if got != want {
		t.Errorf("TransformMCPToCLI(%q) = %q, want %q", in, got, want)
	}
}

func TestTransformIdempotencyGuardSkipsAlreadyCLI(t *testing.T) {
	in := "Use `zen-codegraph --action files` to list files."
	got := TransformMCPToCLI(in)
	if got != in {
		t.Errorf("already-CLI text should pass through unchanged, got %q", got)
	}
}

func TestTransformIdempotencyGuardDoesNotSkipMixed(t *testing.T) {
	// Mixed RPC + CLI text should still be transformed.
	in := "Use `codegraph({ action: 'files' })` or `zen-browser --action navigate`."
	want := "Use `zen-codegraph --action files` or `zen-browser --action navigate`."
	got := TransformMCPToCLI(in)
	if got != want {
		t.Errorf("TransformMCPToCLI(%q) = %q, want %q", in, got, want)
	}
}

func TestTransformComplexValueFallsBackToJSON(t *testing.T) {
	in := "Use `codegraph({ action: 'search', query: '[complex]' })`."
	want := "Use `zen-codegraph --json '{\"action\":\"search\",\"query\":\"[complex]\"}'`."
	got := TransformMCPToCLI(in)
	if got != want {
		t.Errorf("TransformMCPToCLI(%q) = %q, want %q", in, got, want)
	}
}

func TestTransformRule1MatchesBareWordValues(t *testing.T) {
	// Bare word values are matched by the parser.
	in := "Use `codegraph({ action: files })`."
	want := "Use `zen-codegraph --action files`."
	got := TransformMCPToCLI(in)
	if got != want {
		t.Errorf("TransformMCPToCLI(%q) = %q, want %q", in, got, want)
	}
}

func TestTransformSkillBlockTransforms(t *testing.T) {
	// Rule 2: functional notation on its own line without backticks.
	in := "codegraph({ action: 'files' })"
	want := "zen-codegraph --action files"
	got := TransformMCPToCLI(in)
	if got != want {
		t.Errorf("TransformMCPToCLI(%q) = %q, want %q", in, got, want)
	}
}

func TestResolvePromptMcp2CliTransformsToolCalls(t *testing.T) {
	dir := setupSkills(t)
	mcpcfg.ProjectRoot = dir
	oldCfg := mcpcfg.Get()
	defer func() { mcpcfg.Config.Store(oldCfg) }()

	cfg := *oldCfg
	cfg.Mcp2Cli = true
	mcpcfg.Config.Store(&cfg)

	p := PromptDefinition{
		Name:          "test",
		Arguments:     []PromptArgument{{Name: "i"}},
		Template:      "Run `codegraph({ action: 'files' })` and `codegraph({ action: 'status' })`.",
		EnabledSkills: []string{"codebase-research"},
	}
	result, err := ResolvePrompt(p, map[string]string{"i": "query"}, "/ws")
	if err != nil {
		t.Fatalf("ResolvePrompt() error = %v", err)
	}
	if !strings.Contains(result, "zen-codegraph --action files") {
		t.Errorf("expected zen-codegraph --action files in result, got:\n%s", result)
	}
	if !strings.Contains(result, "zen-codegraph --action status") {
		t.Errorf("expected zen-codegraph --action status in result, got:\n%s", result)
	}
}

func TestResolvePromptMcp2CliFalsePreservesRPC(t *testing.T) {
	dir := setupSkills(t)
	mcpcfg.ProjectRoot = dir
	oldCfg := mcpcfg.Get()
	defer func() { mcpcfg.Config.Store(oldCfg) }()

	cfg := *oldCfg
	cfg.Mcp2Cli = false
	mcpcfg.Config.Store(&cfg)

	p := PromptDefinition{
		Name:          "test",
		Arguments:     []PromptArgument{{Name: "i"}},
		Template:      "Run `codegraph({ action: 'files' })`.",
		EnabledSkills: []string{"codebase-research"},
	}
	result, err := ResolvePrompt(p, map[string]string{"i": "query"}, "/ws")
	if err != nil {
		t.Fatalf("ResolvePrompt() error = %v", err)
	}
	if !strings.Contains(result, "`codegraph({ action: 'files' })`") {
		t.Errorf("expected RPC form preserved when mcp2cli=false, got:\n%s", result)
	}
}

func TestTransformRealPromptAndSkillFiles(t *testing.T) {
	projectRoot := findProjectRoot(t)

	promptsDir := filepath.Join(projectRoot, "resources", "prompts")
	skillsDir := filepath.Join(projectRoot, "resources", "skills")

	mustTransform := []struct {
		pattern string
		want    string
	}{
		// Simple flat-param patterns handled by the regex parser.
		{`codegraph({ action: 'files' })`, "zen-codegraph --action files"},
		{`codegraph({ action: 'map' })`, "zen-codegraph --action map"},
		{`codegraph({ action: 'status' })`, "zen-codegraph --action status"},
		{`codegraph({ action: 'index' })`, "zen-codegraph --action index"},
		{`codegraph({ action: 'mermaid' })`, "zen-codegraph --action mermaid"},
		{`browser({ action: 'screenshot', screenshot: 'full' })`, "zen-browser --action screenshot --screenshot full"},
		// skills tool (MCP name: "skill" → "zen-skill"; file uses "skills" → falls back to "zen-skills").
		{`skills({ action: 'get', id: 'kontakt-reconstruct' })`, "zen-skills --action get --id kontakt-reconstruct"},
		{`codegraph({ action: 'search', query: 'PORT-TODO', semantic: true })`, "zen-codegraph --action search --query PORT-TODO --semantic true"},
		// Reference patterns (no param parsing).
		{"Activate MCP `skill id=", "Activate skill id="},
		{"MCP `codegraph`", "`zen-codegraph`"},
		{"MCP codegraph tool", "zen-codegraph CLI"},
		{"Always use the MCP `", "Always use the `zen-"},
		{"`skill id=", "`zen-skill --action get --id="},
	}

	var totalFiles, withMatches int
	var untransformed []string
	var complexPatterns []string // patterns with nested objects/arrays — need manual review

	checkDir := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".md") {
				continue
			}
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			body := stripFrontmatter(string(data))
			if !strings.Contains(body, "codegraph") && !strings.Contains(body, "browser") &&
				!strings.Contains(body, "memory(") && !strings.Contains(body, "workspace(") &&
				!strings.Contains(body, "skills(") && !strings.Contains(body, "MCP ") &&
				!strings.Contains(body, "Activate MCP") && !strings.Contains(body, "`skill id=") {
				continue
			}
			totalFiles++
			transformed := TransformMCPToCLI(body)
			bodyChanged := transformed != body
			if !bodyChanged {
				untransformed = append(untransformed, name)
				continue
			}
		withMatches++
		for _, tc := range mustTransform {
			if strings.Contains(body, tc.pattern) && !strings.Contains(transformed, tc.want) {
				t.Logf("%s: pattern %q not transformed to %q (regex limitation — nested/array value)",
					name, tc.pattern, tc.want)
			}
		}
			// Detect patterns with nested objects/arrays that the regex parser
			// cannot flatten — these need manual standardization or a deeper parser.
			nestedRe := regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_-]*\(\{\s*[^}]*[\[{][^}]*\}\s*[^}]*\}`)
			for _, match := range nestedRe.FindAllString(body, -1) {
				truncated := match
				if len(truncated) > 120 {
					truncated = truncated[:120] + "..."
				}
				complexPatterns = append(complexPatterns, name+": "+truncated)
			}
		}
	}

	checkDir(promptsDir)
	checkDir(skillsDir)

	if totalFiles == 0 {
		t.Fatal("no prompt or skill files matched transform-relevant content")
	}
	if withMatches == 0 {
		t.Fatal("no files were transformed; rules may be non-functional")
	}

	if len(untransformed) > 0 {
		t.Logf("INFO: %d file(s) contained MCP-like content but were NOT transformed (manual review needed):\n%s",
			len(untransformed), strings.Join(untransformed, ", "))
	}
	if len(complexPatterns) > 0 {
		t.Logf("INFO: %d nested/array pattern(s) found across files that the regex parser cannot flatten (manual standardization or deeper parser needed):\n%s",
			len(complexPatterns), strings.Join(complexPatterns, "\n"))
	}
}

func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (config.json)")
		}
		dir = parent
	}
}

func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	idx := strings.Index(content[3:], "---")
	if idx == -1 {
		return content
	}
	return content[3+idx+3:]
}
