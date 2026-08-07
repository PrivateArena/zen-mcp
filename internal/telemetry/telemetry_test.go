package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zen-mcp/internal/mcpcfg"
)

func setupTempRoot(t *testing.T) {
	t.Helper()
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir()
	t.Cleanup(func() {
		mcpcfg.ProjectRoot = old
		_ = Close()
	})
	if err := mcpcfg.Load(); err != nil {
		t.Fatal(err)
	}
}

func TestLogToolCallAndSummary(t *testing.T) {
	setupTempRoot(t)
	if err := LogToolCall("browser", "navigate", true, ""); err != nil {
		t.Fatal(err)
	}
	if err := LogToolCall("shell", "run", false, "command failed"); err != nil {
		t.Fatal(err)
	}
	sum := QueryTelemetry(nil)
	for _, want := range []string{"Status: ON", "Total calls: 2", "Success rate: 50.0%", "Failures: 1"} {
		if !strings.Contains(sum, want) {
			t.Errorf("summary missing %q:\n%s", want, sum)
		}
	}
	tools := QueryTelemetry([]string{"tools"})
	if !strings.Contains(tools, "browser: 1 calls") || !strings.Contains(tools, "shell: 1 calls") {
		t.Errorf("tools query wrong:\n%s", tools)
	}
	actions := QueryTelemetry([]string{"actions"})
	if !strings.Contains(actions, "browser.navigate") || !strings.Contains(actions, "shell.run") {
		t.Errorf("actions query wrong:\n%s", actions)
	}
	fail := QueryTelemetry([]string{"failures"})
	if !strings.Contains(fail, "shell.run") || !strings.Contains(fail, "command failed") {
		t.Errorf("failures query wrong:\n%s", fail)
	}
	if got := QueryTelemetry([]string{"reset"}); !strings.Contains(got, "Cleared") {
		t.Errorf("reset wrong: %s", got)
	}
	if got := QueryTelemetry([]string{"summary"}); !strings.Contains(got, "Total calls: 0") {
		t.Errorf("post-reset summary wrong:\n%s", got)
	}
}

func TestTelemetryDbPath(t *testing.T) {
	setupTempRoot(t)
	if err := LogToolCall("workspace", "set", true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mcpcfg.ProjectRoot, "telemetry", "telemetry.db")); err != nil {
		t.Errorf("db not created at expected path: %v", err)
	}
}

func TestTelemetryDisabled(t *testing.T) {
	setupTempRoot(t)
	mcpcfg.Get().TelemetryEnabled = false
	if err := LogToolCall("think", "create_plan", true, ""); err != nil {
		t.Fatal(err)
	}
	if got := QueryTelemetry([]string{"summary"}); !strings.Contains(got, "Total calls: 0") {
		t.Errorf("disabled telemetry should not record:\n%s", got)
	}
}

func TestEnableDisableToggles(t *testing.T) {
	setupTempRoot(t)
	mcpcfg.Get().TelemetryEnabled = false
	if got := QueryTelemetry([]string{"enable"}); !strings.Contains(got, "Enabled") {
		t.Errorf("enable: %s", got)
	}
	if err := LogToolCall("run", "exec", true, ""); err != nil {
		t.Fatal(err)
	}
	if got := QueryTelemetry([]string{"disable"}); !strings.Contains(got, "Disabled") {
		t.Errorf("disable: %s", got)
	}
	if got := QueryTelemetry([]string{"summary"}); !strings.Contains(got, "Status: OFF") {
		t.Errorf("summary should be OFF:\n%s", got)
	}
}
