package terminal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/tools"
)

// withRunConfig installs the config values RunExportCLI falls back to, so a test
// can prove the command resolves prefix/ports/short from the config instead of
// requiring its caller to know them.
func withRunConfig(t *testing.T, prefix string, short bool, cliPort, mcpPort int) {
	t.Helper()
	old := mcpcfg.Get()
	t.Cleanup(func() { mcpcfg.Config.Store(old) })
	cfg := *old
	cfg.CliModePrefix = prefix
	cfg.CliModeShort = short
	cfg.CliPort = cliPort
	cfg.McpPort = mcpPort
	mcpcfg.Config.Store(&cfg)
}

func TestRunExportCLIUsesConfigShortDefault(t *testing.T) {
	withRunConfig(t, "zen-", true, 2999, 3001)
	work := chdirTemp(t)

	// Short nil => the climode_short config value decides, exactly like the terminal
	// command with no flag.
	var out bytes.Buffer
	if err := RunExportCLI(&out, tools.ExportCLIOptions{}); err != nil {
		t.Fatalf("export failed: %v (%s)", err, out.String())
	}
	script, err := os.ReadFile(filepath.Join(work, "cli", "zen-browser"))
	if err != nil {
		t.Fatalf("zen-browser missing: %v", err)
	}
	if !strings.Contains(string(script), `-a) key="action"`) {
		t.Errorf("config short must produce short aliases:\n%s", probeWrapper(script))
	}

	// An explicit false must override the config default.
	off := false
	out.Reset()
	if err := RunExportCLI(&out, tools.ExportCLIOptions{Short: &off}); err != nil {
		t.Fatalf("export failed: %v (%s)", err, out.String())
	}
	script, err = os.ReadFile(filepath.Join(work, "cli", "zen-browser"))
	if err != nil {
		t.Fatalf("zen-browser missing: %v", err)
	}
	if strings.Contains(string(script), `-a) key="action"`) {
		t.Errorf("short=false must produce long-only flags:\n%s", probeWrapper(script))
	}
	if !strings.Contains(string(script), `--action`) {
		t.Errorf("long flags must always be accepted:\n%s", probeWrapper(script))
	}
}

func TestRunExportCLIUsesConfigPorts(t *testing.T) {
	withRunConfig(t, "zen-", false, 2777, 3001)
	work := chdirTemp(t)

	var out bytes.Buffer
	if err := RunExportCLI(&out, tools.ExportCLIOptions{}); err != nil {
		t.Fatalf("export failed: %v (%s)", err, out.String())
	}
	script, err := os.ReadFile(filepath.Join(work, "cli", "zen-browser"))
	if err != nil {
		t.Fatalf("zen-browser missing: %v", err)
	}
	// The wrapper points at the CLI port, so the injected tool never has to know the
	// port layout.
	if !strings.Contains(string(script), "http://127.0.0.1:2777") {
		t.Errorf("wrapper missing the configured CLI port:\n%s", probeWrapper(script))
	}
}

func TestRunExportCLICleanRemovesWrappers(t *testing.T) {
	withRunConfig(t, "zen-", false, 2999, 3001)
	work := chdirTemp(t)
	home := os.Getenv("HOME")

	var out bytes.Buffer
	if err := RunExportCLI(&out, tools.ExportCLIOptions{}); err != nil {
		t.Fatalf("export failed: %v", err)
	}
	wrapper := filepath.Join(work, "cli", "zen-browser")
	if _, err := os.Stat(wrapper); err != nil {
		t.Fatalf("export wrote nothing: %v", err)
	}
	link := filepath.Join(home, ".local", "bin", "zen-browser")
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("export left no PATH link: %v", err)
	}

	out.Reset()
	if err := RunExportCLI(&out, tools.ExportCLIOptions{Clean: true}); err != nil {
		t.Fatalf("clean failed: %v", err)
	}
	if !strings.Contains(out.String(), "Cleaned") {
		t.Errorf("clean report = %q", out.String())
	}
	if _, err := os.Stat(wrapper); !os.IsNotExist(err) {
		t.Errorf("wrapper survived clean: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("PATH link survived clean: %v", err)
	}
}

func TestRunExportCLISurfacesAFatalWriteError(t *testing.T) {
	withRunConfig(t, "zen-", false, 2999, 3001)
	work := chdirTemp(t)

	// A cli path that is not a directory can never hold wrappers, so the run is a
	// failure and must report one instead of printing a happy summary.
	if err := os.WriteFile(filepath.Join(work, "cli"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := RunExportCLI(&out, tools.ExportCLIOptions{})
	if err == nil {
		t.Fatalf("expected an error, got report %q", out.String())
	}
	if !strings.Contains(err.Error(), "could not create") {
		t.Errorf("error = %v", err)
	}
	if !strings.Contains(out.String(), "ERROR: could not create") {
		t.Errorf("report must keep the human-readable line, got %q", out.String())
	}
}

func TestRunExportCLIAcceptsANilWriter(t *testing.T) {
	withRunConfig(t, "zen-", false, 2999, 3001)
	chdirTemp(t)

	// A tool caller may legitimately want no report stream; the run still has to
	// work rather than nil-panic.
	if err := RunExportCLI(nil, tools.ExportCLIOptions{}); err != nil {
		t.Fatalf("export with a nil writer failed: %v", err)
	}
}

func TestRunExportCLISerializesConcurrentRuns(t *testing.T) {
	withRunConfig(t, "zen-", false, 2999, 3001)
	chdirTemp(t)

	// Wrapper generation rewrites cli/ and the PATH links as one unit. While the lock
	// is held, a second run must wait instead of racing over the same files.
	exportMu.Lock()
	type outcome struct {
		text string
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		text, err := RunExportCLIText(tools.ExportCLIOptions{})
		done <- outcome{text: text, err: err}
	}()

	select {
	case got := <-done:
		t.Fatalf("export finished while another run held the export lock: %q (%v)", got.text, got.err)
	case <-time.After(150 * time.Millisecond):
	}

	exportMu.Unlock()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("queued export failed once the lock was released: %v", got.err)
		}
		if !strings.Contains(got.text, "Exported") {
			t.Errorf("queued export report = %q", got.text)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("queued export never ran")
	}
}

// probeWrapper shortens a wrapper script for failure messages.
func probeWrapper(script []byte) string {
	const max = 400
	if len(script) <= max {
		return string(script)
	}
	return string(script[:max]) + "..."
}
