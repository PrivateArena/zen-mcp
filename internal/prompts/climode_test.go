package prompts

import (
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
