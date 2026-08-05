package terminal

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

var errBoom = errors.New("boom")

func TestRegisterAndDispatch(t *testing.T) {
	var called []string
	Register("echo", func(args []string, sessionID string) error {
		called = append(called, sessionID, strings.Join(args, " "))
		return nil
	})
	defer unregister("echo")

	var prompt bytes.Buffer
	runCommander(strings.NewReader("echo hello world\n"), "s1", &prompt)

	if len(called) != 2 || called[0] != "s1" || called[1] != "hello world" {
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
	runCommander(strings.NewReader("nosuchcmd arg\n"), "s1", &prompt)

	if !strings.Contains(logOut.String(), "Unknown command: nosuchcmd") {
		t.Errorf("missing unknown-command log: %q", logOut.String())
	}
}

func TestHandlerErrorLogged(t *testing.T) {
	Register("fail", func(_ []string, _ string) error {
		return errBoom
	})
	defer unregister("fail")

	var logOut bytes.Buffer
	old := LogOut
	LogOut = &logOut
	defer func() { LogOut = old }()

	var prompt bytes.Buffer
	runCommander(strings.NewReader("fail\n"), "s1", &prompt)

	if !strings.Contains(logOut.String(), "ERROR: boom") {
		t.Errorf("missing error log: %q", logOut.String())
	}
	if !strings.Contains(logOut.String(), "START: fail...") {
		t.Errorf("missing start log: %q", logOut.String())
	}
}

func TestExitStops(t *testing.T) {
	var prompt bytes.Buffer
	runCommander(strings.NewReader("hello\nhelp\nexit\n"), "s1", &prompt)
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
	runCommander(strings.NewReader("help\n"), "s1", &prompt)
	if !strings.Contains(logOut.String(), "Available commands:") {
		t.Errorf("help output missing: %q", logOut.String())
	}
}

func TestBlankLinesSkipped(t *testing.T) {
	var called bool
	Register("ping", func(_ []string, _ string) error {
		called = true
		return nil
	})
	defer unregister("ping")

	var prompt bytes.Buffer
	runCommander(strings.NewReader("\n  \nping\n"), "s1", &prompt)
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
