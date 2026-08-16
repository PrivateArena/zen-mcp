package terminal

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
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

// runLoopWithBytes drives runCommanderLoop directly over an os.Pipe, feeding
// input bytes (a raw-mode byte stream) before closing the write end so the
// loop terminates on EOF. It returns the loop's exit flag, the prompt output,
// and the LogOut output.
func runLoopWithBytes(t *testing.T, input string) (exited bool, prompt, logOut *bytes.Buffer) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	var p, l bytes.Buffer
	old := LogOut
	LogOut = &l
	defer func() { LogOut = old }()

	if _, err := w.WriteString(input); err != nil {
		w.Close()
		t.Fatal(err)
	}
	w.Close()

	var history []string
	historyIdx := -1
	var currentLine []byte
	buf := make([]byte, 1)

	exited = runCommanderLoop(&p, r, &history, &historyIdx, &currentLine, buf)
	return exited, &p, &l
}

// TestRunCommanderLoopCharacterization pins the CURRENT observable behavior
// of the raw-mode loop, so that readline edits (TAB completion) cannot
// silently regress non-TAB key handling. Each subtest asserts pre-existing
// behavior that must remain identical after the feature lands.
func TestRunCommanderLoopCharacterization(t *testing.T) {
	t.Run("printableAndEnterDispatch", func(t *testing.T) {
		var called []string
		Register("abb", func(args []string) error {
			called = append(called, strings.Join(args, " "))
			return nil
		})
		defer unregister("abb")

		exited, _, _ := runLoopWithBytes(t, "abc\x7fb\n")
		if exited {
			t.Error("exit flag should be false")
		}
		if len(called) != 1 || called[0] != "" {
			t.Errorf("handler called = %v, want one call with empty args", called)
		}
	})

	t.Run("tabBeforeWhitespaceIsNoop", func(t *testing.T) {
		// TAB in argument position must not mangle the line nor trigger
		// completion of argument tokens.
		var got []string
		Register("acmd", func(args []string) error {
			got = append(got, args...)
			return nil
		})
		defer unregister("acmd")

		exited, _, _ := runLoopWithBytes(t, "acmd x\x09\n")
		if exited {
			t.Error("exit flag should be false")
		}
		if len(got) != 1 || got[0] != "x" {
			t.Errorf("args = %v, want [x]", got)
		}
	})

	t.Run("tabWithNoMatchIsNoop", func(t *testing.T) {
		Register("abb", func([]string) error { return nil })
		defer unregister("abb")

		exited, prompt, _ := runLoopWithBytes(t, "zz\x09\n")
		if exited {
			t.Error("exit flag should be false")
		}
		// In the raw loop dispatch redirects LogOut to the prompt stream,
		// so the unknown-command log appears there, not on LogOut.
		if !strings.Contains(prompt.String(), "Unknown command: zz") {
			t.Errorf("missing unknown-command log: %q", prompt.String())
		}
		if strings.Contains(prompt.String(), "abb") {
			t.Errorf("no-match TAB must not print suggestions: %q", prompt.String())
		}
	})

	t.Run("ctrlDWithContentDoesNotExit", func(t *testing.T) {
		exited, prompt, _ := runLoopWithBytes(t, "abo\x04\n")
		if exited {
			t.Error("ctrl-D with non-empty line must not exit")
		}
		if !strings.Contains(prompt.String(), "Unknown command: abo") {
			t.Errorf("missing unknown-command log: %q", prompt.String())
		}
	})

	t.Run("ctrlCExits", func(t *testing.T) {
		exited, _, logOut := runLoopWithBytes(t, "\x03")
		if !exited {
			t.Error("ctrl-C should exit the loop")
		}
		if !strings.Contains(logOut.String(), "Shutting down terminal commander.") {
			t.Errorf("missing shutdown log: %q", logOut.String())
		}
	})

	t.Run("ctrlDOnEmptyExits", func(t *testing.T) {
		exited, _, logOut := runLoopWithBytes(t, "\x04")
		if !exited {
			t.Error("ctrl-D on empty line should exit the loop")
		}
		if !strings.Contains(logOut.String(), "Shutting down terminal commander.") {
			t.Errorf("missing shutdown log: %q", logOut.String())
		}
	})

	t.Run("exitCommandExits", func(t *testing.T) {
		exited, prompt, _ := runLoopWithBytes(t, "exit\n")
		if !exited {
			t.Error("exit command should exit the loop")
		}
		if !strings.Contains(prompt.String(), "Shutting down terminal commander.") {
			t.Errorf("missing shutdown log: %q", prompt.String())
		}
	})

	t.Run("helpCommandDispatches", func(t *testing.T) {
		exited, prompt, _ := runLoopWithBytes(t, "help\n")
		if exited {
			t.Error("help should not exit the loop")
		}
		if !strings.Contains(prompt.String(), "Available commands:") {
			t.Errorf("missing help output: %q", prompt.String())
		}
	})
}

// TestRunCommanderLoopTabCompletion covers the new TAB suggestion/autocomplete
// behavior of the raw-mode loop.
func TestRunCommanderLoopTabCompletion(t *testing.T) {
	t.Run("uniqueMatchAutocompletesLine", func(t *testing.T) {
		called := false
		Register("shell", func(_ []string) error {
			called = true
			return nil
		})
		defer unregister("shell")

		exited, prompt, _ := runLoopWithBytes(t, "sh\x09\n")
		if exited {
			t.Error("exit flag should be false")
		}
		if !called {
			t.Error("completed line should dispatch the handler")
		}
		if !strings.Contains(prompt.String(), "shell ") {
			t.Errorf("line was not completed to 'shell ': %q", prompt.String())
		}
	})

	t.Run("autocompleteThenTypeArgs", func(t *testing.T) {
		var got []string
		Register("shell", func(args []string) error {
			got = append(got, args...)
			return nil
		})
		defer unregister("shell")

		exited, _, _ := runLoopWithBytes(t, "sh\x09env\n")
		if exited {
			t.Error("exit flag should be false")
		}
		if len(got) != 1 || got[0] != "env" {
			t.Errorf("handler args = %v, want [env]", got)
		}
	})

	t.Run("multipleMatchesList", func(t *testing.T) {
		Register("alpha", func([]string) error { return nil })
		Register("alpine", func([]string) error { return nil })
		defer unregister("alpha")
		defer unregister("alpine")

		// First TAB advances to the common prefix "alp"; a second TAB (no
		// further shared chars) lists both candidates.
		exited, prompt, _ := runLoopWithBytes(t, "al\x09\x09\n")
		if exited {
			t.Error("exit flag should be false")
		}
		if !strings.Contains(prompt.String(), "alpha") || !strings.Contains(prompt.String(), "alpine") {
			t.Errorf("multi-match TAB must list both suggestions: %q", prompt.String())
		}
		if !strings.Contains(prompt.String(), "Unknown command: alp") {
			t.Errorf("line must dispatch uncompleted after listing: %q", prompt.String())
		}
	})

	t.Run("multipleMatchesAdvanceToCommonPrefix", func(t *testing.T) {
		Register("export-cli", func([]string) error { return nil })
		Register("export-commands", func([]string) error { return nil })
		defer unregister("export-cli")
		defer unregister("export-commands")

		// A single TAB fills the longest common prefix instead of just
		// listing: "ex" + TAB -> "export-c".
		exited, prompt, _ := runLoopWithBytes(t, "ex\x09\n")
		if exited {
			t.Error("exit flag should be false")
		}
		if !strings.Contains(prompt.String(), "export-c") {
			t.Errorf("TAB must advance to common prefix 'export-c': %q", prompt.String())
		}
		if strings.Contains(prompt.String(), "export-cli") {
			t.Errorf("candidate list must not be shown on first TAB: %q", prompt.String())
		}
		if !strings.Contains(prompt.String(), "Unknown command: export-c") {
			t.Errorf("line must dispatch the common-prefix command: %q", prompt.String())
		}
	})

	t.Run("emptyLineListsAll", func(t *testing.T) {
		Register("alpha", func([]string) error { return nil })
		Register("alpine", func([]string) error { return nil })
		defer unregister("alpha")
		defer unregister("alpine")

		exited, prompt, _ := runLoopWithBytes(t, "\x09\n")
		if exited {
			t.Error("exit flag should be false")
		}
		// Empty prefix matches every registered command, including the
		// statically-registered help command.
		for _, name := range []string{"alpha", "alpine", "help"} {
			if !strings.Contains(prompt.String(), name) {
				t.Errorf("empty-line TAB must list %q: %q", name, prompt.String())
			}
		}
	})
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
