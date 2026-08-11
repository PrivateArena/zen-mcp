package telemetry

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

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

// TestLogToolCallRecordsDurationMs pins that the duration metric is persisted
// as a real column (not buried in prose) so timeouts/aborts are queryable.
func TestLogToolCallRecordsDurationMs(t *testing.T) {
	setupTempRoot(t)
	if err := LogToolCall("shell", "", false, "timed out (activity) after 600012ms", 600012); err != nil {
		t.Fatal(err)
	}
	d, err := sql.Open("sqlite", telemetryDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var dur sql.NullInt64
	var msg string
	if err := d.QueryRow(`SELECT duration_ms, error_message FROM tool_calls WHERE tool='shell'`).Scan(&dur, &msg); err != nil {
		t.Fatal(err)
	}
	if !dur.Valid || dur.Int64 != 600012 {
		t.Errorf("duration_ms = %v, want 600012", dur)
	}
	if !strings.Contains(msg, "activity") {
		t.Errorf("error_message = %q", msg)
	}
}

// TestLogToolCallMigratesExistingSchema pins that a telemetry.db created by an
// older version (no duration_ms column) is upgraded in place rather than
// failing, keeping past records and the write path intact.
func TestLogToolCallMigratesExistingSchema(t *testing.T) {
	setupTempRoot(t)
	root := mcpcfg.ProjectRoot
	dir := filepath.Join(root, "telemetry")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "telemetry.db")+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE tool_calls (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      tool TEXT NOT NULL,
      action TEXT,
      success INTEGER NOT NULL DEFAULT 1,
      error_message TEXT,
      timestamp TEXT NOT NULL DEFAULT (datetime('now'))
    )`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO tool_calls (tool, action, success, error_message) VALUES ('browser','navigate',0,'pre-existing failure')`); err != nil {
		t.Fatal(err)
	}
	_ = old.Close()
	_ = Close() // reset cached handle so getDb re-opens the migrated file

	if err := LogToolCall("shell", "", false, "client abort after 292621ms", 292621); err != nil {
		t.Fatal(err)
	}

	d, err := sql.Open("sqlite", telemetryDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tool_calls`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("rows = %d, want 2 (pre-existing + new)", count)
	}
	var dur sql.NullInt64
	var msg string
	if err := d.QueryRow(`SELECT duration_ms, error_message FROM tool_calls WHERE tool='shell'`).Scan(&dur, &msg); err != nil {
		t.Fatalf("duration_ms column missing/read failed: %v", err)
	}
	if !dur.Valid || dur.Int64 != 292621 {
		t.Errorf("duration_ms = %v, want 292621", dur)
	}
}

func telemetryDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(mcpcfg.ProjectRoot, "telemetry", "telemetry.db") + "?mode=ro"
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
