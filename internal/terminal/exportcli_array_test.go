package terminal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// arraySampleSchema mirrors a real tool (browser) shape: scalar params plus
// string-array params declared with "type":"array".
var arraySampleSchema = map[string]any{
	"type":     "object",
	"required": []any{"action"},
	"properties": map[string]any{
		"action":       map[string]any{"type": "string", "description": "Action."},
		"upload_files": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Files"},
		"provider":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Providers"},
		"url":          map[string]any{"type": "string", "description": "URL"},
	},
}

func arraySampleTool() cliTool {
	return cliTool{name: "browser", description: "Array test tool.", schema: arraySampleSchema}
}

func TestCollectParamsArrayFlag(t *testing.T) {
	params := collectParams(arraySampleSchema)
	got := map[string]cliParam{}
	for _, p := range params {
		got[p.key] = p
	}
	if !got["upload_files"].isArray {
		t.Error("upload_files should be flagged isArray")
	}
	if !got["provider"].isArray {
		t.Error("provider should be flagged isArray")
	}
	if got["action"].isArray {
		t.Error("action is a scalar and must not be flagged isArray")
	}
	if got["url"].isArray {
		t.Error("url is a scalar and must not be flagged isArray")
	}
}

func TestBuildWrapperScriptArraySections(t *testing.T) {
	script := buildWrapperScript(arraySampleTool(), "http://127.0.0.1:2999")

	for _, want := range []string{
		`declare -A ARR_PARAMS`,
		`push_array() {`,
		`split_array() {`,
		`ARR_PARAMS["$key"]+=$'\x01'"$val"`,
		`        provider|upload_files)`,
		`  ARRS_JSON=$(`,
		`  ARGS_JSON=$(jq -n --argjson base "$ARGS_JSON" --argjson extra "$ARRS_JSON" '$base * $extra')`,
		`(repeatable, comma-separated)`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("array wrapper missing %q", want)
		}
	}
}

func TestBuildWrapperScriptArrayShortAliases(t *testing.T) {
	script := buildWrapperScriptOpt(arraySampleTool(), "http://127.0.0.1:2999", true)
	for _, want := range []string{
		`-p) key="provider"; split_array "$key" "$2"; shift 2 ;;`,
		`-up) key="upload_files"; split_array "$key" "$2"; shift 2 ;;`,
		`-ur) key="url"; PARAMS["$key"]="$2"; shift 2 ;;`,
		`echo "  -up, --upload_files (repeatable, comma-separated)  Files"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("short array wrapper missing %q", want)
		}
	}
}

func TestBuildWrapperScriptScalarToolHasNoArraySections(t *testing.T) {
	// Backward-compat guard: tools without array params must generate the
	// exact pre-change script (no ARR_PARAMS, no push_array, no merge).
	script := buildWrapperScript(sampleTool(), "http://127.0.0.1:2999")
	for _, forbid := range []string{
		`declare -A ARR_PARAMS`,
		`push_array`,
		`ARRS_JSON`,
		`$base * $extra`,
		`(repeatable, comma-separated)`,
		`IFS=',' read`,
	} {
		if strings.Contains(script, forbid) {
			t.Errorf("scalar-only wrapper must not contain %q", forbid)
		}
	}
}

// arrayHarness extracts the arg-parsing + JSON-building section of a generated
// wrapper and turns it into a runnable script that prints ARGS_JSON.
func arrayHarness(t *testing.T, tool cliTool, short bool) string {
	t.Helper()
	script := buildWrapperScriptOpt(tool, "http://127.0.0.1:2999", short)
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n")
	inBody := false
	for _, l := range strings.Split(script, "\n") {
		if l == `RAW_JSON=""` {
			inBody = true
		}
		if strings.HasPrefix(l, "PAYLOAD=") {
			break
		}
		if inBody {
			b.WriteString(l + "\n")
		}
	}
	b.WriteString(`printf '%s' "$ARGS_JSON"` + "\n")
	return b.String()
}

func runArrayHarness(t *testing.T, harness string, args ...string) map[string]any {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "harness")
	if err := os.WriteFile(path, []byte(harness), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("bash", append([]string{path}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("harness %v failed: %v\n%s", args, err, out)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("harness %v output %q not JSON: %v", args, out, err)
	}
	return got
}

func TestArrayHarnessScalarBackwardCompat(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--url", "https://example.com")
	if got["action"] != "chat" || got["url"] != "https://example.com" {
		t.Fatalf("scalar passthrough broken: %v", got)
	}
	if _, ok := got["upload_files"]; ok {
		t.Fatalf("upload_files should be absent: %v", got)
	}
}

func TestArrayHarnessRepeatedShortFlags(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), true)
	got := runArrayHarness(t, harness, "-a", "chat", "-up", "f1", "-up", "f2", "-up", "f3")
	files, ok := got["upload_files"].([]any)
	if !ok {
		t.Fatalf("upload_files should be an array, got %#v", got["upload_files"])
	}
	if len(files) != 3 || files[0] != "f1" || files[1] != "f2" || files[2] != "f3" {
		t.Fatalf("repeated flags must accumulate, got %v", files)
	}
	if got["action"] != "chat" {
		t.Fatalf("scalar action lost: %v", got)
	}
}

func TestArrayHarnessRepeatedLongFlags(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--upload_files", "x", "--upload_files", "y")
	files, ok := got["upload_files"].([]any)
	if !ok {
		t.Fatalf("upload_files should be an array, got %#v", got["upload_files"])
	}
	if len(files) != 2 || files[0] != "x" || files[1] != "y" {
		t.Fatalf("long-form repeated flags must accumulate, got %v", files)
	}
}

func TestArrayHarnessCommaSeparated(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--upload_files", "f1,f2,f3")
	files, ok := got["upload_files"].([]any)
	if !ok {
		t.Fatalf("upload_files should be an array, got %#v", got["upload_files"])
	}
	if len(files) != 3 || files[0] != "f1" || files[1] != "f2" || files[2] != "f3" {
		t.Fatalf("comma split failed, got %v", files)
	}
}

func TestArrayHarnessCommaPlusRepeated(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--upload_files", "a,b", "--upload_files", "c")
	files := got["upload_files"].([]any)
	if len(files) != 3 || files[0] != "a" || files[1] != "b" || files[2] != "c" {
		t.Fatalf("comma+repeat should merge, got %v", files)
	}
}

func TestArrayHarnessShortEqualStyle(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), true)
	got := runArrayHarness(t, harness, "-a", "chat", "-up=/tmp/f1,/tmp/f2")
	files, ok := got["upload_files"].([]any)
	if !ok {
		t.Fatalf("short equal-style must yield an array, got %#v", got["upload_files"])
	}
	if len(files) != 2 || files[0] != "/tmp/f1" || files[1] != "/tmp/f2" {
		t.Fatalf("short equal-style split failed, got %v", files)
	}
	if got["action"] != "chat" {
		t.Fatalf("scalar action lost: %v", got)
	}
}

func TestArrayHarnessShortEqualStyleSingleStaysString(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), true)
	got := runArrayHarness(t, harness, "-a=chat", "-up=/tmp/f1")
	if got["action"] != "chat" {
		t.Fatalf("scalar equal-style action failed: %v", got)
	}
	if got["upload_files"] != "/tmp/f1" {
		t.Fatalf("single equal-style value must stay a string, got %#v", got["upload_files"])
	}
}

func TestArrayHarnessLongEqualStyle(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--upload_files=/tmp/f1,/tmp/f2,/tmp/f3")
	files, ok := got["upload_files"].([]any)
	if !ok {
		t.Fatalf("long equal-style must yield an array, got %#v", got["upload_files"])
	}
	if len(files) != 3 {
		t.Fatalf("long equal-style split failed, got %v", files)
	}
}

func TestArrayHarnessEqualStyleValueKeepsEquals(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--url=https://example.com?a=b")
	if got["url"] != "https://example.com?a=b" {
		t.Fatalf("equal-style value with embedded '=' must be preserved, got %#v", got["url"])
	}
}

func TestArrayHarnessMixedStyles(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), true)
	got := runArrayHarness(t, harness, "-a=chat", "-p", "gemini", "-up=/tmp/f1,/tmp/f2", "--upload_files=/tmp/f3", "--provider=openai")
	files, ok := got["upload_files"].([]any)
	if !ok || len(files) != 3 {
		t.Fatalf("mixed-style arrays must accumulate, got %#v", got["upload_files"])
	}
	providers, ok := got["provider"].([]any)
	if !ok || len(providers) != 2 || providers[0] != "gemini" || providers[1] != "openai" {
		t.Fatalf("mixed-style provider should accumulate across styles, got %#v", got["provider"])
	}
	if got["action"] != "chat" {
		t.Fatalf("scalar action lost: %v", got)
	}
}

func TestArrayHarnessEmptyEqualValueOmitted(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), true)
	got := runArrayHarness(t, harness, "-a", "chat", "-up=")
	if _, ok := got["upload_files"]; ok {
		t.Fatalf("empty equal-style value must be omitted, got %v", got)
	}
}

func TestArrayWrapperHelpDocumentsEqualStyle(t *testing.T) {
	script := buildWrapperScriptOpt(arraySampleTool(), "http://127.0.0.1:2999", true)
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
		"--<param>=<value>",
		"-up=f1",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("equal-style help missing %q\n%s", want, out)
		}
	}
}

func TestArrayHarnessMultilineValuePreserved(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--upload_files", "line1\nline2")
	if got["upload_files"] != "line1\nline2" {
		t.Fatalf("multiline array value must survive as one element, got %#v", got["upload_files"])
	}
}

func TestArrayHarnessMultilineWithCommaSplit(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), true)
	got := runArrayHarness(t, harness, "-a", "chat", "-up", "a\nb,c")
	files, ok := got["upload_files"].([]any)
	if !ok || len(files) != 2 || files[0] != "a\nb" || files[1] != "c" {
		t.Fatalf("newlines preserved while commas split, got %#v", got["upload_files"])
	}
}

func TestArrayHarnessMissingValueGuard(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
	harness := arrayHarness(t, arraySampleTool(), true)
	dir := t.TempDir()
	path := filepath.Join(dir, "harness")
	if err := os.WriteFile(path, []byte(harness), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("bash", path, "-a", "chat", "-ur").CombinedOutput()
	if err == nil {
		t.Fatalf("missing value must fail, got success: %s", out)
	}
	if !strings.Contains(string(out), "missing value") {
		t.Fatalf("missing-value diagnostic expected, got: %s", out)
	}
}

func TestArrayHarnessMissingValueGuardLong(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
	harness := arrayHarness(t, arraySampleTool(), false)
	dir := t.TempDir()
	path := filepath.Join(dir, "harness")
	if err := os.WriteFile(path, []byte(harness), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("bash", path, "--action", "chat", "--url").CombinedOutput()
	if err == nil {
		t.Fatalf("missing long-flag value must fail, got success: %s", out)
	}
	if !strings.Contains(string(out), "missing value") {
		t.Fatalf("missing-value diagnostic expected, got: %s", out)
	}
}

func TestArrayHarnessSingleValueStaysString(t *testing.T) {
	// Backward-compat: a single array value is sent as a plain string, exactly
	// as every pre-change wrapper did.
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--upload_files", "f1")
	if got["upload_files"] != "f1" {
		t.Fatalf("single value must stay a string, got %#v", got["upload_files"])
	}
}

func TestArrayHarnessEmptyAndTrailingComma(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--upload_files", "")
	if _, ok := got["upload_files"]; ok {
		t.Fatalf("empty value must be omitted, got %v", got)
	}
	got = runArrayHarness(t, harness, "--action", "chat", "--upload_files", "f1,")
	if got["upload_files"] != "f1" {
		t.Fatalf("trailing comma must be dropped, got %#v", got["upload_files"])
	}
}

func TestArrayHarnessMixedArrayAndScalarParams(t *testing.T) {
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--upload_files", "f1,f2", "--provider", "p1", "--url", "u")
	if len(got["upload_files"].([]any)) != 2 {
		t.Fatalf("upload_files array wrong: %v", got["upload_files"])
	}
	if got["provider"] != "p1" {
		t.Fatalf("single provider should stay string, got %#v", got["provider"])
	}
	if got["url"] != "u" {
		t.Fatalf("scalar url lost: %v", got)
	}
}

func TestArrayHarnessScalarValueWithCommaKept(t *testing.T) {
	// Comma-splitting must only apply to array-typed params; a scalar value
	// containing a comma passes through untouched.
	harness := arrayHarness(t, arraySampleTool(), false)
	got := runArrayHarness(t, harness, "--action", "chat", "--url", "a,b")
	if got["url"] != "a,b" {
		t.Fatalf("scalar comma must be preserved, got %#v", got["url"])
	}
}

func TestArrayWrapperShellSyntax(t *testing.T) {
	script := buildWrapperScript(arraySampleTool(), "http://127.0.0.1:2999")
	dir := t.TempDir()
	path := filepath.Join(dir, "zen-browser")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("bash -n failed: %v\n%s", err, out)
	}
	// --help must work offline for array-enabled tools too.
	out, err := exec.Command(path, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"--upload_files (repeatable, comma-separated)",
		"--action (required)  Action.",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("array help missing %q\n%s", want, out)
		}
	}
}
