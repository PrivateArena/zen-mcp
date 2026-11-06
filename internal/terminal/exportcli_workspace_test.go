package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A wrapper run from inside a project must bind to that project, the way a
// harness picks up its project-local .pi/ folder: the nearest directory upward
// that declares .zenmcp/config.json wins over the workspace the server was last
// pointed at. ZENMCP_WORKSPACE_ROOT still outranks everything, and a directory
// that declares nothing leaves the old behaviour alone.
func TestWrapperResolvesWorkspaceFromProjectTier(t *testing.T) {
	path := writeWrapper(t, browserTool(), true)

	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, ".zenmcp"), 0o755); err != nil {
		t.Fatal(err)
	}
	tier := filepath.Join(proj, ".zenmcp", "config.json")
	if err := os.WriteFile(tier, []byte(`{"youtube":{"container":"chan-b"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// The path bash prints is the physical one, and TMPDIR may be a symlink.
	proj, err := filepath.EvalSymlinks(proj)
	if err != nil {
		t.Fatal(err)
	}

	dryRun := func(dir string, env ...string) string {
		t.Helper()
		cmd := exec.Command(path, "-a", "chat", "--message", "hi", "--dry-run")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("dry-run in %s: %v\n%s", dir, err, out)
		}
		return string(out)
	}
	workspaceOf := func(t *testing.T, out string) string {
		t.Helper()
		const prefix = "workspace     : "
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, prefix) {
				return strings.TrimSpace(line[len(prefix):])
			}
		}
		t.Fatalf("dry-run printed no workspace:\n%s", out)
		return ""
	}

	// From the project folder itself, and from any folder below it.
	for _, dir := range []string{proj, filepath.Join(proj, "src"), filepath.Join(proj, "src", "deep")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		out := dryRun(dir)
		if got := workspaceOf(t, out); got != proj {
			t.Errorf("run from %s bound to %q, want the project root %q", dir, got, proj)
		}
		if !strings.Contains(out, "workspace_src : project-tier") {
			t.Errorf("run from %s did not report the tier as its source:\n%s", dir, out)
		}
	}

	// An explicit override still wins, so an agent can act on another project.
	if out := dryRun(proj, "ZENMCP_WORKSPACE_ROOT=/tmp/elsewhere"); !strings.Contains(out, "workspace_src : env:ZENMCP_WORKSPACE_ROOT") {
		t.Errorf("ZENMCP_WORKSPACE_ROOT did not win over the project tier:\n%s", out)
	}

	// Without a tier, nothing is discovered and the server's workspace decides.
	plain, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out := dryRun(plain); strings.Contains(out, "workspace_src : project-tier") {
		t.Errorf("a folder without a tier claimed one:\n%s", out)
	}

	// An empty tier is still a project - it declares which project this is, even
	// while it overrides nothing.
	if err := os.WriteFile(tier, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := dryRun(proj); !strings.Contains(out, "workspace_src : project-tier") {
		t.Errorf("an empty tier was not treated as a project:\n%s", out)
	}
}

// The tier discovery must not turn into a broken script for a caller whose path
// has spaces, and must terminate at the filesystem root.
func TestWrapperWorkspaceDiscoveryIsSafe(t *testing.T) {
	path := writeWrapper(t, browserTool(), true)

	proj := filepath.Join(t.TempDir(), "a project with spaces")
	if err := os.MkdirAll(filepath.Join(proj, ".zenmcp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".zenmcp", "config.json"), []byte(`{"log_level":"debug"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	proj, err := filepath.EvalSymlinks(proj)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(path, "-a", "chat", "--message", "hi", "--dry-run")
	cmd.Dir = proj
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper failed on a path with spaces: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "workspace_src : project-tier") {
		t.Errorf("tier not found on a path with spaces:\n%s", out)
	}
}
