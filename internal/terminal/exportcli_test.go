package terminal

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var sampleSchema = map[string]any{
	"type":     "object",
	"required": []any{"action"},
	"properties": map[string]any{
		"action": map[string]any{"type": "string", "description": "Browser action.", "enum": []any{"navigate", "chat", "screenshot"}},
		"url":    map[string]any{"type": "string", "description": "[nav] URL"},
		"count":  map[string]any{"type": "number", "description": "How many"},
	},
}

func sampleTool() cliTool {
	return cliTool{name: "browser", description: "Control Firefox via userChrome.js MCP Bridge.", schema: sampleSchema}
}

func TestCollectParams(t *testing.T) {
	params := collectParams(sampleSchema)
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}
	if params[0].key != "action" {
		t.Fatalf("expected sorted first key action, got %s", params[0].key)
	}
	if !params[0].required {
		t.Error("action should be required")
	}
	if params[0].desc != "Browser action." {
		t.Errorf("desc = %q", params[0].desc)
	}
	if len(params[0].values) != 3 || params[0].values[1] != "chat" {
		t.Errorf("values = %v", params[0].values)
	}
	if params[1].key != "count" || params[1].required {
		t.Errorf("count param = %+v", params[1])
	}
	if params[2].key != "url" {
		t.Errorf("url param = %+v", params[2])
	}
}

func TestCollectParamsStringSlices(t *testing.T) {
	// Real defs build schemas with []string (not []any) for required/enum.
	schema := map[string]any{
		"type":     "object",
		"required": []string{"action"},
		"properties": map[string]any{
			"action": map[string]any{"type": "string", "description": "Action", "enum": []string{"navigate", "chat"}},
			"other":  map[string]any{"type": "string", "description": "Other"},
		},
	}
	params := collectParams(schema)
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}
	if !params[0].required {
		t.Error("action should be required")
	}
	if got := params[0].values; len(got) != 2 || got[0] != "navigate" || got[1] != "chat" {
		t.Errorf("enum values = %v", got)
	}
}

func TestQEscapes(t *testing.T) {
	if got := q(`a"b$c\d`); got != `a\"b\$c\\d` {
		t.Errorf("q() = %q", got)
	}
}

func TestBuildWrapperScriptContent(t *testing.T) {
	script := buildWrapperScript(sampleTool(), "http://127.0.0.1:2999")

	for _, want := range []string{
		"#!/usr/bin/env bash",
		"# browser — generated wrapper for MCP tool: browser",
		"# Parameters (from JSON Schema):",
		"#   --action (required)  Browser action. [navigate|chat|screenshot]",
		"# Server: http://127.0.0.1:2999",
		"# Generated: ",
		"set -euo pipefail",
		`SESSION_ID="${ZENMCP_SESSION_ID:-zen-cli-$$}"`,
		`SHARED_WS=$(curl -sf "http://127.0.0.1:2999/shared/workspace-root"`,
		`WORKSPACE="${ZENMCP_WORKSPACE_ROOT:-${SHARED_WS:-$(pwd)}}"`,
		`TOOL="browser"`,
		`--json)  RAW_JSON="$2"; shift 2 ;;`,
		"echo \"browser — Control Firefox via userChrome.js MCP Bridge.\"",
		`echo "  --action (required)  Browser action. [navigate|chat|screenshot]"`,
		"ACTION VALUES: navigate, chat, screenshot",
		`key="${1#--}"; PARAMS["$key"]="$2"; shift 2 ;;`,
		`printf "%s\0%s\0" "$k" "${PARAMS[$k]}"; done \`,
		`split("\u0000")`,
		`-X POST "http://127.0.0.1:2999/mcp" \`,
		`7)  echo "Error: connection refused — is zenmcp running at http://127.0.0.1:2999?" >&2 ;;`,
		`28) echo "Error: request timed out (${ZENMCP_TIMEOUT:-60}s) — server may be busy" >&2 ;;`,
		`.result.content[] | if .type == "text" then .text`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("generated script missing %q", want)
		}
	}
}

func TestBuildWrapperScriptShellSyntax(t *testing.T) {
	script := buildWrapperScript(sampleTool(), "http://127.0.0.1:2999")
	dir := t.TempDir()
	path := filepath.Join(dir, "zen-browser")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("bash -n failed: %v\n%s", err, out)
	}
}

func TestBuildWrapperScriptHelp(t *testing.T) {
	script := buildWrapperScript(sampleTool(), "http://127.0.0.1:2999")
	dir := t.TempDir()
	path := filepath.Join(dir, "zen-browser")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(path, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"browser — Control Firefox via userChrome.js MCP Bridge.",
		"POST http://127.0.0.1:2999/mcp",
		"--action (required)  Browser action. [navigate|chat|screenshot]",
		"ACTION VALUES: navigate, chat, screenshot",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("help output missing %q\n%s", want, out)
		}
	}
}

func TestCollectToolsFallback(t *testing.T) {
	// With no deps the registry is empty; collectTools must fall back to
	// the static tools.AllDefs schema set (non-empty, deterministic order).
	tools := collectTools()
	if len(tools) == 0 {
		t.Fatal("expected fallback tool set")
	}
	for i := 1; i < len(tools); i++ {
		if tools[i-1].name > tools[i].name {
			t.Errorf("tools not sorted: %s before %s", tools[i-1].name, tools[i].name)
		}
	}
	names := map[string]bool{}
	for _, tr := range tools {
		if tr.name == "" {
			t.Error("empty tool name")
		}
		if names[tr.name] {
			t.Errorf("duplicate tool %s", tr.name)
		}
		names[tr.name] = true
	}
}

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zen-test")
	if err := writeAtomic(path, "hello"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "hello" {
		t.Fatalf("read back = %q, err=%v", data, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file should be cleaned up, stat err=%v", err)
	}
}

func TestExportCLIEndToEnd(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out bytes.Buffer
	ExportCLI(&out, 2999, 3001)

	entries, err := os.ReadDir(filepath.Join(work, "cli"))
	if err != nil {
		t.Fatalf("cli dir missing: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no wrappers exported")
	}
	byName := map[string]bool{}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "zen-") {
			t.Errorf("unexpected non-zen file %s", e.Name())
		}
		byName[e.Name()] = true
		info, err := e.Info()
		if err != nil || info.Mode()&0o111 == 0 {
			t.Errorf("wrapper %s not executable: %v", e.Name(), err)
		}
	}
	if !byName["zen-browser"] || !byName["zen-workspace"] || !byName["zen-shell"] {
		t.Errorf("expected core wrappers, got %v", byName)
	}

	// Symlink present under the temp HOME and resolves to an absolute, real file.
	link := filepath.Join(home, ".local", "bin", "zen-browser")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink %s failed: %v", link, err)
	}
	if !filepath.IsAbs(target) {
		t.Fatalf("symlink target must be absolute, got %q", target)
	}
	if info, err := os.Stat(target); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("symlink target %q not executable: %v", target, err)
	}

	// The generated wrapper's --help works offline.
	browser := filepath.Join(work, "cli", "zen-browser")
	helpOut, err := exec.Command(browser, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, helpOut)
	}
	if !strings.Contains(string(helpOut), "PARAMETERS:") {
		t.Errorf("help output missing PARAMETERS:\n%s", helpOut)
	}

	// Unknown arg path fails cleanly.
	if _, err := exec.Command(browser, "bogus").CombinedOutput(); err == nil {
		t.Error("expected non-zero exit for unknown arg")
	}

	// ExportCliClean removes both cli/ artifacts and the symlink.
	ExportCliClean(&out)
	leftover, _ := os.ReadDir(filepath.Join(work, "cli"))
	if len(leftover) != 0 {
		t.Errorf("cli/ not cleaned, left: %v", leftover)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("symlink %s should be removed, stat err=%v", link, err)
	}
}
