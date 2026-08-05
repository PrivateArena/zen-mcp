package exec

import (
	"testing"
)

func TestRunEcho(t *testing.T) {
	res := Run("echo hello", "/tmp", 0, 30000)
	if res.Stdout != "hello\n" && res.Stdout != "hello" {
		t.Errorf("Run(echo) stdout = %q, want hello", res.Stdout)
	}
	if res.ExitCode != 0 {
		t.Errorf("Run(echo) exitCode = %d, want 0", res.ExitCode)
	}
}

func TestRunBadCommand(t *testing.T) {
	res := Run("nonexistentcommand12345", "/tmp", 0, 30000)
	if res.ExitCode == 0 {
		t.Errorf("Run(bad) exitCode = %d, want non-zero", res.ExitCode)
	}
}
