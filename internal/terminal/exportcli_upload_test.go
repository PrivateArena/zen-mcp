package terminal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"zen-mcp/internal/tools"
)

// payloadHarness extracts the arg-parsing + PAYLOAD-build section of a
// generated wrapper and turns it into a runnable script that prints the final
// JSON-RPC request body.
func payloadHarness(t *testing.T, tool cliTool, short bool) string {
	t.Helper()
	script := buildWrapperScriptOpt(tool, "http://127.0.0.1:2999", short)
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n")
	inBody := false
	for _, l := range strings.Split(script, "\n") {
		if strings.HasPrefix(l, `TOOL="`) {
			inBody = true
		}
		if strings.HasPrefix(l, "# Dry-run:") {
			break
		}
		if inBody {
			b.WriteString(l + "\n")
		}
	}
	b.WriteString(`printf '%s' "$PAYLOAD"` + "\n")
	return b.String()
}

func runPayloadHarness(t *testing.T, harness string, args ...string) map[string]any {
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
	var payload struct {
		Params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("harness %v output %q not JSON-RPC: %v", args, out, err)
	}
	return payload.Params.Arguments
}

// browserTool returns the real browser tool def schema exactly as served over
// tools/list, so wrapper tests mirror production.
func browserTool() cliTool {
	defs := tools.AllDefs("", tools.Deps{})
	for _, d := range defs {
		if d.Name == "browser" {
			return cliTool{name: d.Name, description: d.Description, schema: d.Schema}
		}
	}
	panic("browser tool not found")
}

// TestBrowserWrapperChatUploadsReachPayload is the coverage regression for the
// cli_bug.txt / cli_bug2.txt reports: running the generated browser wrapper with
// the exact transcript flags (`-up <file> -up <file> ... --message "...") must
// put every uploaded file into the tools/call JSON-RPC body. Before this test,
// upload_files silently vanished from the request, so browser.chat ran with
// only the message.
func TestBrowserWrapperChatUploadsReachPayload(t *testing.T) {
	harness := payloadHarness(t, browserTool(), true)
	args := []string{
		"--action", "chat",
		"--provider", "claude",
		"-up", "PROJECT_OVERVIEW.md",
		"-up", "pkg/sfizz/bridge.c",
		"-up", "pkg/sfizz/engine.go",
		"-up", "pkg/sfizz/engine_stability.go",
		"-up", "pkg/sfizz/engine_synth.go",
		"-up", "pkg/sfizz/engine_render.go",
		"-up", "pkg/sfzspec/wrapper.go",
		"--message", "CONTEXT: red-teaming this architecture plan",
	}
	got := runPayloadHarness(t, harness, args...)

	if got["action"] != "chat" {
		t.Fatalf("action = %#v, want chat", got["action"])
	}
	if got["message"] != "CONTEXT: red-teaming this architecture plan" {
		t.Fatalf("message = %#v", got["message"])
	}
	if got["provider"] != "claude" {
		t.Fatalf("provider = %#v, want claude", got["provider"])
	}
	files, ok := got["upload_files"].([]any)
	if !ok {
		t.Fatalf("upload_files missing/not array in payload: %#v", got)
	}
	if len(files) != 7 {
		t.Fatalf("expected 7 uploaded files, got %d: %v", len(files), files)
	}
	for i, want := range []string{
		"PROJECT_OVERVIEW.md", "pkg/sfizz/bridge.c", "pkg/sfizz/engine.go",
		"pkg/sfizz/engine_stability.go", "pkg/sfizz/engine_synth.go",
		"pkg/sfizz/engine_render.go", "pkg/sfzspec/wrapper.go",
	} {
		if files[i] != want {
			t.Errorf("file[%d] = %q, want %q", i, files[i], want)
		}
	}
}

// TestBrowserWrapperChatLongFormUploads guards the same contract via the
// long-form --upload_files flag, which the non-short wrappers accept.
func TestBrowserWrapperChatLongFormUploads(t *testing.T) {
	harness := payloadHarness(t, browserTool(), false)
	got := runPayloadHarness(t, harness,
		"--action", "chat",
		"--upload_files", "a.md,b.md",
		"--message", "hello",
	)
	files, ok := got["upload_files"].([]any)
	if !ok || len(files) != 2 || files[0] != "a.md" || files[1] != "b.md" {
		t.Fatalf("long-form upload_files lost: %#v", got)
	}
}

// TestBrowserWrapperMessageOnlyNoUpload verifies the no-upload case stays clean:
// a message-only chat call must not fabricate an upload_files key.
func TestBrowserWrapperMessageOnlyNoUpload(t *testing.T) {
	harness := payloadHarness(t, browserTool(), true)
	got := runPayloadHarness(t, harness, "-a", "chat", "--message", "just a message")
	if _, ok := got["upload_files"]; ok {
		t.Fatalf("message-only call must not include upload_files: %#v", got)
	}
	if got["message"] != "just a message" {
		t.Fatalf("message = %#v", got["message"])
	}
}
