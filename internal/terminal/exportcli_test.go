package terminal

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"zen-mcp/internal/mcpcfg"
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
		`28) echo "Error: request timed out (${ZENMCP_TIMEOUT:-1200}s) — server may be busy" >&2 ;;`,
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

func TestShortAliasMap(t *testing.T) {
	cases := []struct {
		name   string
		params []cliParam
		want   map[string]string
	}{
		{
			name:   "single param becomes first char",
			params: []cliParam{{key: "message"}},
			want:   map[string]string{"message": "m"},
		},
		{
			name:   "colliding first letters disambiguate",
			params: []cliParam{{key: "message"}, {key: "model"}},
			want:   map[string]string{"message": "me", "model": "mo"},
		},
		{
			name:   "prefix key keeps full name",
			params: []cliParam{{key: "url"}, {key: "url2"}},
			want:   map[string]string{"url": "url", "url2": "url2"},
		},
		{
			name:   "h reserved for help",
			params: []cliParam{{key: "help"}},
			want:   map[string]string{"help": "he"},
		},
		{
			name:   "real browser param set",
			params: []cliParam{{key: "action"}, {key: "body"}, {key: "code"}, {key: "headers"}, {key: "message"}, {key: "method"}, {key: "provider"}, {key: "take_screenshot"}, {key: "upload_files"}, {key: "url"}},
			want: map[string]string{
				"action": "a", "body": "b", "code": "c", "headers": "he",
				"message": "mes", "method": "met", "provider": "p",
				"take_screenshot": "t", "upload_files": "up", "url": "ur",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shortAliasMap(tc.params)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d aliases, want %d: %v", len(got), len(tc.want), got)
			}
			for k, want := range tc.want {
				if got[k] != want {
					t.Errorf("alias[%q] = %q, want %q", k, got[k], want)
				}
			}
			seen := map[string]bool{}
			for k, a := range got {
				if seen[a] {
					t.Errorf("duplicate alias %q for keys %q", a, k)
				}
				if a == "h" {
					t.Errorf("reserved alias h assigned to %q", k)
				}
				seen[a] = true
			}
		})
	}
}

func TestShortAliasMapSkipsEmptyKeys(t *testing.T) {
	got := shortAliasMap([]cliParam{{key: ""}, {key: "message"}})
	if got["message"] != "m" {
		t.Errorf("alias[message] = %q, want m", got["message"])
	}
}

func TestBuildWrapperScriptShort(t *testing.T) {
	shortScript := buildWrapperScriptOpt(sampleTool(), "http://127.0.0.1:2999", true)
	defaultScript := buildWrapperScript(sampleTool(), "http://127.0.0.1:2999")

	for _, want := range []string{
		`    -a) key="action"; PARAMS["$key"]="$2"; shift 2 ;;`,
		`    -u) key="url"; PARAMS["$key"]="$2"; shift 2 ;;`,
		`    -c) key="count"; PARAMS["$key"]="$2"; shift 2 ;;`,
		`echo "  $0 -<short> <value>...   # short aliases"`,
		`echo "  -a, --action (required)  Browser action. [navigate|chat|screenshot]"`,
		`echo "  -c, --count  How many"`,
	} {
		if !strings.Contains(shortScript, want) {
			t.Errorf("short script missing %q", want)
		}
	}

	// Long form must still be accepted in short wrappers, and the default
	// (non-short) wrapper must be byte-identical in behavior to before.
	for _, forbid := range []string{
		`-a) key="action"`,
		`-c, --count`,
		`-<short>`,
	} {
		if strings.Contains(defaultScript, forbid) {
			t.Errorf("default script must not contain %q (short mode leaked)", forbid)
		}
	}
	for _, want := range []string{
		`echo "  --action (required)  Browser action. [navigate|chat|screenshot]"`,
		`key="${1#--}"; PARAMS["$key"]="$2"; shift 2 ;;`,
	} {
		if !strings.Contains(defaultScript, want) {
			t.Errorf("default script missing %q", want)
		}
	}
}

func TestBuildWrapperScriptShortShellSyntaxAndHelp(t *testing.T) {
	script := buildWrapperScriptOpt(sampleTool(), "http://127.0.0.1:2999", true)
	dir := t.TempDir()
	path := filepath.Join(dir, "zen-browser")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("bash -n failed: %v\n%s", err, out)
	}
	out, err := exec.Command(path, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"-a, --action (required)  Browser action. [navigate|chat|screenshot]",
		"-c, --count  How many",
		"-<short> <value>...",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("short help output missing %q\n%s", want, out)
		}
	}
}

func TestExportCLIWithShortEndToEnd(t *testing.T) {
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
	ExportCLIWithShort(&out, 2999, 3001, true)
	if !strings.Contains(out.String(), "Exported") {
		t.Fatalf("unexpected output: %s", out.String())
	}

	// Real browser tool: action -> a, message -> mes, headers -> he (not h).
	script, err := os.ReadFile(filepath.Join(work, "cli", "zen-browser"))
	if err != nil {
		t.Fatalf("zen-browser missing: %v", err)
	}
	for _, want := range []string{
		`-a) key="action"`,
		`-mes) key="message"`,
		`-he) key="headers"`,
		`-a, --action`,
		`-mes, --message`,
	} {
		if !strings.Contains(string(script), want) {
			t.Errorf("real wrapper missing %q", want)
		}
	}
	if strings.Contains(string(script), `-h) key=`) {
		t.Errorf("reserved -h must never be emitted as a param alias")
	}

	path := filepath.Join(work, "cli", "zen-browser")
	if out, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("bash -n failed: %v\n%s", err, out)
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

func withCliConfig(t *testing.T, prefix string, short bool) {
	t.Helper()
	old := mcpcfg.Get()
	t.Cleanup(func() { mcpcfg.Config.Store(old) })
	cfg := *old
	cfg.CliModePrefix = prefix
	cfg.CliModeShort = short
	mcpcfg.Config.Store(&cfg)
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origWd) })
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	return work
}

func TestExportCLIWithCustomPrefix(t *testing.T) {
	withCliConfig(t, "zn-", false)
	work := chdirTemp(t)

	var out bytes.Buffer
	ExportCLI(&out, 2999, 3001)
	if !strings.Contains(out.String(), "Exported") {
		t.Fatalf("unexpected output: %s", out.String())
	}

	// New-prefix wrappers are generated...
	if _, err := os.Stat(filepath.Join(work, "cli", "zn-browser")); err != nil {
		t.Fatalf("zn-browser missing: %v", err)
	}
	// ...and legacy zen-* wrappers are cleaned up by the prefix change.
	entries, _ := os.ReadDir(filepath.Join(work, "cli"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "zen-") {
			t.Errorf("legacy zen-* wrapper survived prefix change: %s", e.Name())
		}
	}
	link := filepath.Join(os.Getenv("HOME"), ".local", "bin", "zn-browser")
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("symlink zn-browser missing: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(work, "cli", "zn-browser")); err != nil {
		t.Errorf("zn-browser not readable: %v", err)
	}
}

func TestExportCLIWithShortRespectsCustomPrefix(t *testing.T) {
	withCliConfig(t, "zn-", false)
	work := chdirTemp(t)

	var out bytes.Buffer
	ExportCLIWithShort(&out, 2999, 3001, false)
	script, err := os.ReadFile(filepath.Join(work, "cli", "zn-browser"))
	if err != nil {
		t.Fatalf("zn-browser missing: %v", err)
	}
	if !strings.Contains(string(script), "#!/usr/bin/env bash") {
		t.Error("zn-browser not a valid wrapper")
	}
}

func TestExportCLIRespectsConfigShort(t *testing.T) {
	withCliConfig(t, "zen-", true)
	work := chdirTemp(t)

	var out bytes.Buffer
	// ExportCLI has no explicit short flag; the climode_short config must apply.
	ExportCLI(&out, 2999, 3001)
	script, err := os.ReadFile(filepath.Join(work, "cli", "zen-browser"))
	if err != nil {
		t.Fatalf("zen-browser missing: %v", err)
	}
	for _, want := range []string{
		`-a) key="action"`,
		`-mes) key="message"`,
		`-he) key="headers"`,
	} {
		if !strings.Contains(string(script), want) {
			t.Errorf("short alias %q missing when climode_short=true", want)
		}
	}
	if strings.Contains(string(script), `-h) key=`) {
		t.Errorf("reserved -h must never be emitted as a param alias")
	}
}

func TestRemoveExactStale(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"zen-browser", "zn-browser", "zn-shell", "zen-myscript", "unrelated"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	candidates := wrapperNameSet([]cliTool{{name: "browser"}, {name: "shell"}}, "zn-", mcpcfg.DefaultCliModePrefix)
	removeExactStale(dir, candidates, map[string]bool{"zn-browser": true})
	leftover, _ := os.ReadDir(dir)
	names := map[string]bool{}
	for _, e := range leftover {
		names[e.Name()] = true
	}
	if names["zen-browser"] {
		t.Error("legacy prefixed exact wrapper should be removed as stale")
	}
	if names["zn-shell"] {
		t.Error("current-prefix wrapper of a known tool not in keep should be removed")
	}
	if !names["zn-browser"] {
		t.Error("kept wrapper should survive")
	}
	if !names["zen-myscript"] {
		t.Error("user file sharing the wrapper prefix must not be removed")
	}
	if !names["unrelated"] {
		t.Error("non-wrapper file must not be touched")
	}
}

func TestExportCliCleanMixedPrefixes(t *testing.T) {
	withCliConfig(t, "zn-", false)
	work := chdirTemp(t)
	cliDir := filepath.Join(work, "cli")
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"zen-browser", "zn-browser", "zen-myscript", "keep.txt"} {
		if err := os.WriteFile(filepath.Join(cliDir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	ExportCliClean(&out)
	leftover, _ := os.ReadDir(cliDir)
	names := map[string]bool{}
	for _, e := range leftover {
		names[e.Name()] = true
	}
	if names["zen-browser"] || names["zn-browser"] {
		t.Errorf("wrapper artifacts not cleaned: %v", names)
	}
	if !names["zen-myscript"] {
		t.Error("user file sharing the wrapper prefix should be left untouched")
	}
	if !names["keep.txt"] {
		t.Error("non-wrapper file should be left untouched")
	}
}

func TestSymlinkTargetsInto(t *testing.T) {
	cliDir := filepath.Join(t.TempDir(), "cli")
	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{"absolute inside", filepath.Join(cliDir, "zen-browser"), true},
		{"nested inside", filepath.Join(cliDir, "sub", "zen-browser"), true},
		{"exact dir", cliDir, false},
		{"sibling prefix dir", cliDir + "-other", false},
		{"relative target", "cli/zen-browser", false},
		{"unrelated absolute", "/usr/bin/env", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := symlinkTargetsInto(tc.target, cliDir); got != tc.want {
				t.Errorf("symlinkTargetsInto(%q, %q) = %v, want %v", tc.target, cliDir, got, tc.want)
			}
		})
	}
}

func TestRemoveStaleBinLinksLeavesUserArtifacts(t *testing.T) {
	cliDir := t.TempDir()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cliDir, "zen-browser"), []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cliTarget := filepath.Join(cliDir, "zen-browser")

	// User's own regular file sharing the wrapper prefix.
	if err := os.WriteFile(filepath.Join(binDir, "zen-myscript"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// User's own symlink pointing somewhere unrelated.
	if err := os.Symlink("/usr/bin/env", filepath.Join(binDir, "zen-helper")); err != nil {
		t.Fatal(err)
	}
	// Stale wrapper link this generator created for a now-removed tool.
	if err := os.Symlink(cliTarget, filepath.Join(binDir, "zen-removed")); err != nil {
		t.Fatal(err)
	}
	// Current wrapper link, present in keep.
	if err := os.Symlink(cliTarget, filepath.Join(binDir, "zen-current")); err != nil {
		t.Fatal(err)
	}

	removed := removeStaleBinLinks(binDir, cliDir, map[string]bool{"zen-current": true})
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	for _, keep := range []string{"zen-myscript", "zen-helper", "zen-current"} {
		if _, err := os.Lstat(filepath.Join(binDir, keep)); err != nil {
			t.Errorf("user artifact %s should survive, err=%v", keep, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(binDir, "zen-removed")); !os.IsNotExist(err) {
		t.Errorf("stale wrapper link should be removed, err=%v", err)
	}
}

func TestExportCLIKeepsUserBinArtifacts(t *testing.T) {
	withCliConfig(t, "zen-", false)
	chdirTemp(t)
	binDir := filepath.Join(os.Getenv("HOME"), ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "zen-myscript"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/usr/bin/env", filepath.Join(binDir, "zen-helper")); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	ExportCLI(&out, 2999, 3001)
	for _, keep := range []string{"zen-myscript", "zen-helper"} {
		if _, err := os.Lstat(filepath.Join(binDir, keep)); err != nil {
			t.Errorf("user artifact %s deleted by export: %v", keep, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(binDir, "zen-browser")); err != nil {
		t.Errorf("generated symlink missing: %v", err)
	}

	ExportCliClean(&out)
	for _, keep := range []string{"zen-myscript", "zen-helper"} {
		if _, err := os.Lstat(filepath.Join(binDir, keep)); err != nil {
			t.Errorf("user artifact %s deleted by clean: %v", keep, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(binDir, "zen-browser")); !os.IsNotExist(err) {
		t.Errorf("generated symlink should be cleaned, err=%v", err)
	}
}

func TestExportCLIWithCustomPrefixRemovesStaleLinks(t *testing.T) {
	withCliConfig(t, "zn-", false)
	work := chdirTemp(t)
	cliDir := filepath.Join(work, "cli")
	binDir := filepath.Join(os.Getenv("HOME"), ".local", "bin")
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	absCli, err := filepath.Abs(cliDir)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a previous export under the legacy zen- prefix: wrapper in cli/
	// plus the symlink this generator created for it in ~/.local/bin.
	if err := os.WriteFile(filepath.Join(cliDir, "zen-shell"), []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(absCli, "zen-shell"), filepath.Join(binDir, "zen-shell")); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	ExportCLI(&out, 2999, 3001)

	if _, err := os.Lstat(filepath.Join(cliDir, "zen-shell")); !os.IsNotExist(err) {
		t.Errorf("legacy cli/ wrapper should be cleaned on prefix change, err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(binDir, "zen-shell")); !os.IsNotExist(err) {
		t.Errorf("stale legacy symlink should be cleaned on prefix change, err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(binDir, "zn-browser")); err != nil {
		t.Errorf("new-prefix symlink missing: %v", err)
	}
}
