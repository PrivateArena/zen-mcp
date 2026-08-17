package terminal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// memorySampleSchema mirrors the real memory tool: scalar params that routinely
// carry large markdown bodies (notes, objective) where shell quoting
// and escaping cause agent errors.
var memorySampleSchema = map[string]any{
	"type":     "object",
	"required": []any{"action"},
	"properties": map[string]any{
		"action":    map[string]any{"type": "string", "description": "Memory action.", "enum": []any{"save", "load"}},
		"title":     map[string]any{"type": "string", "description": "One-line label"},
		"objective": map[string]any{"type": "string", "description": "1-2 sentence goal"},
		"notes":     map[string]any{"type": "string", "description": "Markdown notes"},
	},
}

func memorySampleTool() cliTool {
	return cliTool{name: "memory", description: "Save/load project memory.", schema: memorySampleSchema}
}

// runJsonHarness runs the extracted arg-parsing + JSON-build section of a
// generated wrapper, feeding optional stdin (for --json -), and returns the
// final ARGS_JSON as a map.
func runJsonHarness(t *testing.T, harness, stdin string, args ...string) map[string]any {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "harness")
	if err := os.WriteFile(path, []byte(harness), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{path}, args...)...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("harness %v failed: %v\n%s", args, err, out)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("harness %v output %q not JSON: %v", args, out, err)
	}
	return got
}

// runJsonHarnessErr runs the harness expecting failure, returning its combined
// output and error (nil error means the harness unexpectedly succeeded).
func runJsonHarnessErr(t *testing.T, harness, stdin string, args ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "harness")
	if err := os.WriteFile(path, []byte(harness), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{path}, args...)...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestJsonStdinDashHandlesParensAndQuotes reproduces the exact failure from
// .scratch/mistake_cli.txt: massive JSON with parens and embedded quotes must
// flow through STDIN without a bash syntax error or jq parse failure.
func TestJsonStdinDashHandlesParensAndQuotes(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), false)
	base := `{"notes": "UpsertFile'd (with parens) and \"quotes\" work", "objective": "fix C/C++ plugins"}`
	got := runJsonHarness(t, harness, base, "--json", "-", "--action", "save")
	if got["action"] != "save" {
		t.Fatalf("merged action flag lost: %v", got)
	}
	if got["objective"] != "fix C/C++ plugins" {
		t.Fatalf("objective from stdin base missing: %v", got)
	}
	notes, _ := got["notes"].(string)
	if !strings.Contains(notes, "(with parens)") || !strings.Contains(notes, `"quotes"`) {
		t.Fatalf("notes from stdin base mangled: %q", notes)
	}
}

// TestJsonStdinDashMergesScalarFlags: the -a save style flag must be detected
// even when the JSON body comes from STDIN (previously flags were ignored).
func TestJsonStdinDashMergesScalarFlags(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), true)
	got := runJsonHarness(t, harness, `{"objective": "x"}`, "--json", "-", "-a", "save")
	if got["action"] != "save" || got["objective"] != "x" {
		t.Fatalf("stdin base + flag merge broken: %v", got)
	}
}

// TestJsonStdinDashEqualStyle: --json=- is accepted as the STDIN sentinel.
func TestJsonStdinDashEqualStyle(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), false)
	got := runJsonHarness(t, harness, `{"title": "t"}`, "--json=-", "--action", "load")
	if got["action"] != "load" || got["title"] != "t" {
		t.Fatalf("equal-style --json=- merge broken: %v", got)
	}
}

// TestJsonRawStringMergesFlags: --json 'base' + individual flags merges with
// flags winning — the reported `zmemory -a save --json '...'` case.
func TestJsonRawStringMergesFlags(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), true)
	got := runJsonHarness(t, harness, "", "--json", `{"objective": "base goal"}`, "-a", "save")
	if got["action"] != "save" || got["objective"] != "base goal" {
		t.Fatalf("raw json + flag merge broken: %v", got)
	}
}

// TestJsonRawStringFlagsWin: when base and flag share a key, the CLI flag wins.
func TestJsonRawStringFlagsWin(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), false)
	got := runJsonHarness(t, harness, "", "--json", `{"action": "load"}`, "--action", "save")
	if got["action"] != "save" {
		t.Fatalf("CLI flag must override base, got %v", got)
	}
}

// TestJsonRawStringAlone: --json with a complete body and no flags must pass
// through untouched (pre-change behavior preserved).
func TestJsonRawStringAlone(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), false)
	got := runJsonHarness(t, harness, "", "--json", `{"action": "load", "notes": "only base"}`)
	if len(got) != 2 || got["action"] != "load" || got["notes"] != "only base" {
		t.Fatalf("bare --json body must pass through unchanged: %v", got)
	}
}

// TestJsonFileReadsBase: --json-file feeds the base JSON from disk.
func TestJsonFileReadsBase(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), false)
	file := filepath.Join(t.TempDir(), "base.json")
	if err := os.WriteFile(file, []byte(`{"objective": "from file"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runJsonHarness(t, harness, "", "--json-file", file, "--action", "save")
	if got["action"] != "save" || got["objective"] != "from file" {
		t.Fatalf("--json-file base + flag merge broken: %v", got)
	}
}

// TestJsonFileMissing: --json-file with a missing path fails cleanly.
func TestJsonFileMissing(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), false)
	out, err := runJsonHarnessErr(t, harness, "", "--json-file", "/nonexistent/base.json")
	if err == nil {
		t.Fatalf("missing --json-file must fail, got: %s", out)
	}
	if !strings.Contains(out, "file not found") {
		t.Fatalf("expected file-not-found diagnostic, got: %s", out)
	}
}

// TestJsonInvalidBaseDiagnostic: an invalid JSON body must produce a clear
// wrapper-level error instead of the cryptic jq "expected digit" failure.
func TestJsonInvalidBaseDiagnostic(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), false)
	out, err := runJsonHarnessErr(t, harness, "", "--json", "not json")
	if err == nil {
		t.Fatalf("invalid JSON must fail, got: %s", out)
	}
	if !strings.Contains(out, "invalid JSON passed via --json/--json-file") {
		t.Fatalf("expected clear invalid-JSON diagnostic, got: %s", out)
	}
}

// TestAtFileScalarReadsDisk: --notes @file streams markdown from disk
// instead of passing multiline text through shell quotes.
func TestAtFileScalarReadsDisk(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), false)
	notes := "## Core Concepts\n\nUpsertFile'd (with parens) works cleanly now!\n"
	file := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(file, []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runJsonHarness(t, harness, "", "--action", "save", "--notes", "@"+file)
	if got["action"] != "save" {
		t.Fatalf("action lost: %v", got)
	}
	n, _ := got["notes"].(string)
	if n != "## Core Concepts\n\nUpsertFile'd (with parens) works cleanly now!\n" {
		t.Fatalf("notes from @file mangled: %q", n)
	}
}

// TestAtFileMissing: a @file that does not exist fails with the param name so
// agents immediately catch typos instead of sending junk payloads.
func TestAtFileMissing(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), false)
	out, err := runJsonHarnessErr(t, harness, "", "--action", "save", "--notes", "@/nonexistent/notes.md")
	if err == nil {
		t.Fatalf("missing @file must fail, got: %s", out)
	}
	if !strings.Contains(out, "file not found for parameter notes: /nonexistent/notes.md") {
		t.Fatalf("expected param-aware diagnostic, got: %s", out)
	}
}

// TestAtFileEqualStyle: --notes=@file equal-style form resolves too.
func TestAtFileEqualStyle(t *testing.T) {
	harness := arrayHarness(t, memorySampleTool(), false)
	file := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(file, []byte("equal style content"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runJsonHarness(t, harness, "", "--action", "save", "--notes=@"+file)
	if got["notes"] != "equal style content" {
		t.Fatalf("equal-style @file broken: %v", got)
	}
}

// TestAtFileArrayParam: array-typed params also accept @file (single element
// from disk, kept as a plain string like any single array value).
func TestAtFileArrayParam(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), true)
	file := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(file, []byte("multi\nline payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runJsonHarness(t, harness, "", "-a", "chat", "-up", "@"+file)
	if got["action"] != "chat" {
		t.Fatalf("action lost: %v", got)
	}
	if got["upload_files"] != "multi\nline payload" {
		t.Fatalf("@file array value mangled: %#v", got["upload_files"])
	}
}

// TestJsonStdinDashArrayMerge: base from STDIN merges with repeated array flags.
func TestJsonStdinDashArrayMerge(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), true)
	got := runJsonHarness(t, harness, `{"action": "load"}`, "--json", "-", "-up", "a.md", "-up", "b.md")
	files, ok := got["upload_files"].([]any)
	if !ok || len(files) != 2 || files[0] != "a.md" || files[1] != "b.md" {
		t.Fatalf("array flags not merged over stdin base: %v", got)
	}
	if got["action"] != "load" {
		t.Fatalf("base action should survive array merge, got %v", got)
	}
}

// TestMemoryWrapperJsonStdinDryRun exercises the full generated memory wrapper
// with a real heredoc body: dry-run must show the merged action + notes.
func TestMemoryWrapperJsonStdinDryRun(t *testing.T) {
	var tool cliTool
	for _, tr := range collectTools() {
		if tr.name == "memory" {
			tool = tr
			break
		}
	}
	if tool.name == "" {
		t.Skip("memory tool not in fallback tool set")
	}
	path := writeWrapper(t, tool, true)
	cmd := exec.Command(path, "--json", "-", "-a", "save", "--dry-run")
	cmd.Stdin = strings.NewReader(`{"notes": "UpsertFile'd (with parens)"}`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("memory wrapper --json - dry-run failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{
		`"action": "save"`,
		"UpsertFile'd (with parens)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("dry-run body missing %q\n%s", want, text)
		}
	}
}

// TestBuildWrapperScriptDocumentsJsonFeatures: --help must advertise --json -,
// --json-file, and the @file convention so agents know they exist.
func TestBuildWrapperScriptDocumentsJsonFeatures(t *testing.T) {
	script := buildWrapperScript(memorySampleTool(), "http://127.0.0.1:2999")
	dir := t.TempDir()
	path := filepath.Join(dir, "zen-memory")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(path, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"'-' reads JSON from STDIN",
		"--json-file <path>",
		"@<file>",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("help output missing %q\n%s", want, out)
		}
	}
}
