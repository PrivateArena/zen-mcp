package gatekeeper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jang/zen-mcp/internal/mcpcfg"
	"github.com/jang/zen-mcp/internal/shared"
)

type fakeWS string

func (f fakeWS) GetActiveWorkspaceRoot() string { return string(f) }

func setupGatekeeper(t *testing.T) (*Gatekeeper, string) {
	t.Helper()
	ws := t.TempDir()
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir()
	t.Cleanup(func() {
		mcpcfg.ProjectRoot = old
		if err := mcpcfg.Load(); err != nil {
			t.Logf("restore config: %v", err)
		}
	})
	if err := mcpcfg.Load(); err != nil {
		t.Fatal(err)
	}
	c := mcpcfg.Get()
	c.GatekeeperEnabled = true
	c.GatekeeperInteractive = false
	c.GatekeeperInteractiveAuto = "reject"
	c.GatekeeperInteractiveTimeout = 0
	c.GatekeeperRemember = true
	c.GatekeeperRememberPaths = nil

	store := shared.NewStore()
	store.Set("workspace-root", ws)
	return New(store), ws
}

func TestIsLikelyFilePath(t *testing.T) {
	cases := []struct {
		token string
		want  bool
	}{
		{"/etc/passwd", true},
		{"../secret", true},
		{"C:\\Windows\\System32", true},
		{"/media/jang/home", true},
		{"src/index.ts", false},
		{"npm run build", false},
		{"http://example.com", false},
		{"hello.txt", false},
		{"/home/user/.ssh/id_rsa", true},
	}
	for _, tc := range cases {
		if got := IsLikelyFilePath(tc.token); got != tc.want {
			t.Errorf("IsLikelyFilePath(%q) = %v, want %v", tc.token, got, tc.want)
		}
	}
}

func TestNonInteractiveAutoReject(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	c := mcpcfg.Get()
	c.GatekeeperInteractive = false
	c.GatekeeperInteractiveAuto = "reject"

	if err := gk.ValidatePathSafety("/etc/passwd", "read"); err == nil {
		t.Error("expected block for /etc/passwd in non-interactive reject mode")
	} else if !strings.Contains(err.Error(), "Security block") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNonInteractiveAutoAccept(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	c := mcpcfg.Get()
	c.GatekeeperInteractive = false
	c.GatekeeperInteractiveAuto = "accept"

	if err := gk.ValidatePathSafety("/etc/passwd", "read"); err != nil {
		t.Errorf("expected accept in non-interactive accept mode, got %v", err)
	}
}

func TestGatekeeperDisabled(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	mcpcfg.Get().GatekeeperEnabled = false
	if err := gk.ValidatePathSafety("/etc/passwd", "read"); err != nil {
		t.Errorf("disabled gatekeeper should pass everything, got %v", err)
	}
	if err := gk.ValidatePathSafetySync("/etc/passwd", "read"); err != nil {
		t.Errorf("disabled sync check should pass, got %v", err)
	}
}

func TestValidatePathSafetySyncBlocksRestricted(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	err := gk.ValidatePathSafetySync("/etc/passwd", "open")
	if err == nil {
		t.Fatal("expected DANGEROUS PATH error")
	}
	if !strings.Contains(err.Error(), "DANGEROUS PATH DETECTED") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidatePathSafetySyncAllowsTemp(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	tmp := filepath.Join(t.TempDir(), "zen-run-script.py")
	if err := gk.ValidatePathSafetySync(tmp, "run"); err != nil {
		t.Errorf("zen-run-* temp should be allowed, got %v", err)
	}
}

func runAsync(fn func() error) <-chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- fn()
	}()
	return ch
}

func waitForPending(t *testing.T, gk *Gatekeeper) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(gk.GetPendingConfirmations()) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no pending confirmation appeared")
}

func TestInteractiveAccept(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	c := mcpcfg.Get()
	c.GatekeeperInteractive = true
	c.GatekeeperInteractiveAuto = "reject"

	result := runAsync(func() error {
		return gk.ValidatePathSafety("/etc/passwd", "read")
	})
	waitForPending(t, gk)

	if !gk.AcceptConfirmation("") {
		t.Fatal("AcceptConfirmation failed")
	}
	if err := <-result; err != nil {
		t.Errorf("expected accept, got %v", err)
	}
}

func TestInteractiveReject(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	c := mcpcfg.Get()
	c.GatekeeperInteractive = true
	c.GatekeeperInteractiveAuto = "accept"

	result := runAsync(func() error {
		return gk.ValidatePathSafety("/etc/passwd", "read")
	})
	waitForPending(t, gk)

	if !gk.RejectConfirmation("", "don't touch") {
		t.Fatal("RejectConfirmation failed")
	}
	err := <-result
	if err == nil {
		t.Fatal("expected block on reject")
	}
	if !strings.Contains(err.Error(), "Suggestion: don't touch") {
		t.Errorf("missing suggestion in error: %v", err)
	}
}

func TestInteractiveTimeoutAutoReject(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	c := mcpcfg.Get()
	c.GatekeeperInteractive = true
	c.GatekeeperInteractiveAuto = "reject"
	c.GatekeeperInteractiveTimeout = 150

	err := gk.ValidatePathSafety("/etc/passwd", "read")
	if err == nil {
		t.Fatal("expected timeout reject")
	}
	if !strings.Contains(err.Error(), "rejected or timed out") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRememberPathAutoAccepts(t *testing.T) {
	gk, ws := setupGatekeeper(t)
	c := mcpcfg.Get()
	c.GatekeeperInteractive = true
	c.GatekeeperInteractiveAuto = "reject"

	// outside-workspace path
	outside := filepath.Join(filepath.Dir(ws), "other")
	target := filepath.Join(outside, "file.txt")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	result := runAsync(func() error {
		return gk.ValidatePathSafety(target, "write")
	})
	waitForPending(t, gk)
	if !gk.AcceptConfirmation("") {
		t.Fatal("AcceptConfirmation failed")
	}
	if err := <-result; err != nil {
		t.Fatalf("first access should succeed: %v", err)
	}

	// remembered now -> no confirmation
	if err := gk.ValidatePathSafety(target, "write"); err != nil {
		t.Errorf("remembered path should auto-accept, got %v", err)
	}
	if len(gk.GetPendingConfirmations()) != 0 {
		t.Error("no confirmation should be pending for remembered path")
	}
}

func TestValidateCommandPayloadBlocked(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	c := mcpcfg.Get()
	c.GatekeeperInteractive = false
	c.GatekeeperInteractiveAuto = "reject"

	err := gk.ValidateCommandPayload("cat /etc/passwd", "/tmp")
	if err == nil {
		t.Fatal("expected command payload block")
	}
	if !strings.Contains(err.Error(), "Shell Guard") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateCommandPayloadSkipsSafeTokens(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	c := mcpcfg.Get()
	c.GatekeeperInteractive = false
	c.GatekeeperInteractiveAuto = "reject"

	err := gk.ValidateCommandPayload("npm run build --watch", "/tmp")
	if err != nil {
		t.Errorf("safe command should pass, got %v", err)
	}
	err = gk.ValidateCommandPayload("curl https://example.com", "/tmp")
	if err != nil {
		t.Errorf("url token should be skipped, got %v", err)
	}
}

func TestGetPendingConfirmations(t *testing.T) {
	gk, _ := setupGatekeeper(t)
	c := mcpcfg.Get()
	c.GatekeeperInteractive = true

	done := runAsync(func() error {
		return gk.ValidatePathSafety("/etc/passwd", "read")
	})
	waitForPending(t, gk)

	pending := gk.GetPendingConfirmations()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].ID == "" || pending[0].Description == "" {
		t.Errorf("pending info incomplete: %+v", pending[0])
	}
	gk.AcceptConfirmation(pending[0].ID)
	_ = <-done
}
