package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeWrapper writes a generated wrapper to disk and returns its path.
func writeWrapper(t *testing.T, tool cliTool, short bool) string {
	t.Helper()
	script := buildWrapperScriptOpt(tool, "http://127.0.0.1:2999", short)
	dir := t.TempDir()
	path := filepath.Join(dir, "zen-"+tool.name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("bash -n failed: %v\n%s", err, out)
	}
	return path
}

func TestDryRunPrintsPayloadAndSkipsCurl(t *testing.T) {
	path := writeWrapper(t, browserTool(), true)
	out, err := exec.Command(path, "--action", "chat", "--provider", "claude",
		"-up", "a.md", "-up", "b.md", "--message", "hello", "--dry-run").CombinedOutput()
	if err != nil {
		t.Fatalf("--dry-run failed: %v\n%s", err, out)
	}
	text := string(out)

	// The full JSON-RPC body must be shown with every uploaded file present.
	for _, want := range []string{
		"=== JSON-RPC REQUEST (dry-run, NOT sent) ===",
		`"method": "tools/call"`,
		`"name": "browser"`,
		`"action": "chat"`,
		`"message": "hello"`,
		`"upload_files": [`,
		`"a.md"`,
		`"b.md"`,
		"=== METRICS ===",
		"tool          : browser",
		"upload_files  : 2",
		"payload_bytes : ",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("dry-run output missing %q\n%s", want, text)
		}
	}
}

func TestDryRunIsIndependentOfArgOrder(t *testing.T) {
	path := writeWrapper(t, browserTool(), true)
	out, err := exec.Command(path, "--dry-run", "--action", "chat", "-up", "x.md", "--message", "hi").CombinedOutput()
	if err != nil {
		t.Fatalf("--dry-run first arg failed: %v\n%s", err, out)
	}
	text := string(out)
	// A single repeated -up value is kept as a plain string by the wrapper
	// (documented single-value behavior); the body must still contain it.
	if !strings.Contains(text, `"upload_files": "x.md"`) {
		t.Errorf("dry-run body missing upload_files:\n%s", text)
	}
	if !strings.Contains(text, "upload_files  : 1") {
		t.Errorf("dry-run metric upload_files should be 1:\n%s", text)
	}
}

func TestDryRunNoValueConsumed(t *testing.T) {
	// --dry-run must not swallow the following flag as its value.
	path := writeWrapper(t, browserTool(), true)
	out, err := exec.Command(path, "--dry-run", "--action", "chat", "--message", "hi").CombinedOutput()
	if err != nil {
		t.Fatalf("--dry-run with following flags failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"action": "chat"`) {
		t.Errorf("action flag lost after --dry-run:\n%s", out)
	}
}

func TestDryRunSoleFlagNoMissingValueError(t *testing.T) {
	path := writeWrapper(t, browserTool(), true)
	out, err := exec.Command(path, "--dry-run").CombinedOutput()
	if err != nil {
		t.Fatalf("bare --dry-run must not fail: %v\n%s", err, out)
	}
	text := string(out)
	if strings.Contains(text, "missing value") {
		t.Errorf("bare --dry-run triggered missing-value guard:\n%s", text)
	}
	if !strings.Contains(text, "=== JSON-RPC REQUEST (dry-run, NOT sent) ===") {
		t.Errorf("bare --dry-run should print the request:\n%s", text)
	}
}

func TestDryRunMessageOnlyCall(t *testing.T) {
	// The debugging use case from the bug report: a chat call that previously
	// dropped its uploads must visibly show upload_files: 0 in dry-run metrics.
	path := writeWrapper(t, browserTool(), true)
	out, err := exec.Command(path, "-a", "chat", "--message", "no files here", "--dry-run").CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "upload_files  : 0") {
		t.Errorf("message-only dry-run must report upload_files : 0:\n%s", text)
	}
	if !strings.Contains(text, "param_count   : 2") {
		t.Errorf("expected param_count 2 (action, message):\n%s", text)
	}
}

func TestDryRunHelpDocumentsFlag(t *testing.T) {
	path := writeWrapper(t, browserTool(), true)
	out, err := exec.Command(path, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "--dry-run") {
		t.Errorf("--help must document --dry-run:\n%s", out)
	}
}

func TestDryRunLongFormOnlyWrapper(t *testing.T) {
	// Non-short wrappers must also support --dry-run.
	path := writeWrapper(t, browserTool(), false)
	out, err := exec.Command(path, "--action", "chat", "--upload_files", "a.md", "--message", "hi", "--dry-run").CombinedOutput()
	if err != nil {
		t.Fatalf("long-only wrapper dry-run failed: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "upload_files  : 1") || !strings.Contains(text, `"a.md"`) {
		t.Errorf("long-only dry-run missing upload_files:\n%s", text)
	}
}
