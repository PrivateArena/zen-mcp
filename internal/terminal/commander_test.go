package terminal

import (
	"bytes"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/shared"
	"zen-mcp/internal/telemetry"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/tools"
)

var errBoom = errors.New("boom")

func TestRegisterAndDispatch(t *testing.T) {
	var called []string
	Register("echo", func(args []string) error {
		called = append(called, strings.Join(args, " "))
		return nil
	})
	defer unregister("echo")

	var prompt bytes.Buffer
	runCommander(strings.NewReader("echo hello world\n"), &prompt)

	if len(called) != 1 || called[0] != "hello world" {
		t.Errorf("handler not called correctly: %v", called)
	}
	if prompt.String() != "> > " {
		t.Errorf("prompts = %q", prompt.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	var logOut bytes.Buffer
	old := LogOut
	LogOut = &logOut
	defer func() { LogOut = old }()

	var prompt bytes.Buffer
	runCommander(strings.NewReader("nosuchcmd arg\n"), &prompt)

	if !strings.Contains(logOut.String(), "Unknown command: nosuchcmd") {
		t.Errorf("missing unknown-command log: %q", logOut.String())
	}
}

func TestHandlerErrorLogged(t *testing.T) {
	Register("fail", func(_ []string) error {
		return errBoom
	})
	defer unregister("fail")

	var logOut bytes.Buffer
	old := LogOut
	LogOut = &logOut
	defer func() { LogOut = old }()

	var prompt bytes.Buffer
	runCommander(strings.NewReader("fail\n"), &prompt)

	if !strings.Contains(logOut.String(), "ERROR: boom") {
		t.Errorf("missing error log: %q", logOut.String())
	}
	if !strings.Contains(logOut.String(), "START: fail...") {
		t.Errorf("missing start log: %q", logOut.String())
	}
}

func TestExitStops(t *testing.T) {
	var prompt bytes.Buffer
	if !runCommander(strings.NewReader("hello\nhelp\nexit\n"), &prompt) {
		t.Error("exit should stop the REPL")
	}
	// No panic means exit/quit handled; help is registered.
	if !strings.Contains(prompt.String(), "> ") {
		t.Error("prompt expected")
	}
}

func TestHelpRegistered(t *testing.T) {
	var logOut bytes.Buffer
	old := LogOut
	LogOut = &logOut
	defer func() { LogOut = old }()

	var prompt bytes.Buffer
	runCommander(strings.NewReader("help\n"), &prompt)
	if !strings.Contains(logOut.String(), "Available commands:") {
		t.Errorf("help output missing: %q", logOut.String())
	}
}

func TestBlankLinesSkipped(t *testing.T) {
	var called bool
	Register("ping", func(_ []string) error {
		called = true
		return nil
	})
	defer unregister("ping")

	var prompt bytes.Buffer
	runCommander(strings.NewReader("\n  \nping\n"), &prompt)
	if !called {
		t.Error("ping should have been dispatched after blank lines")
	}
}

func TestFallbackPort(t *testing.T) {
	if got := FallbackPort(2999, 2998, true); got != 2999 {
		t.Errorf("cli available should use cliPort, got %d", got)
	}
	if got := FallbackPort(2999, 2998, false); got != 2998 {
		t.Errorf("cli unavailable should fall back to mcpPort, got %d", got)
	}
}

func unregister(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(handlers, name)
}

// TestExecuteToolRecordsActionTelemetry pins that the CLI dispatch boundary
// (terminal.ExecuteTool) also injects the request ToolContext so CLI-invoked
// tools record their action in telemetry, matching the MCP path.
func TestExecuteToolRecordsActionTelemetry(t *testing.T) {
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir()
	t.Cleanup(func() {
		mcpcfg.ProjectRoot = old
		_ = telemetry.Close()
	})
	_ = telemetry.Close()

	oldDeps := deps
	t.Cleanup(func() { deps = oldDeps })
	SetDeps(tools.Deps{
		Store: shared.NewStore(),
		Reg:   toolregistry.Create(),
	})

	out := ExecuteTool("skill", map[string]any{"action": "list"})
	if strings.Contains(out, "ERROR:") {
		t.Fatalf("ExecuteTool failed: %s", out)
	}

	_ = telemetry.Close()
	d, err := sql.Open("sqlite", "file:"+filepath.Join(mcpcfg.ProjectRoot, "telemetry", "telemetry.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var tool, action string
	if err := d.QueryRow(`SELECT tool, action FROM tool_calls ORDER BY id DESC LIMIT 1`).Scan(&tool, &action); err != nil {
		t.Fatalf("query: %v", err)
	}
	if tool != "skill" || action != "list" {
		t.Fatalf("telemetry row = %s.%s, want skill.list", tool, action)
	}
}
