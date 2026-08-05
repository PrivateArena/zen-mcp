package processes

import (
	"os/exec"
	"testing"
	"time"
)

func TestRegisterAndAbortAll(t *testing.T) {
	// Register a short-lived process
	cmd := exec.Command("sleep", "0.1")
	_ = cmd.Start()
	Register(cmd)

	// Give it a moment to register
	time.Sleep(10 * time.Millisecond)

	// AbortAll should not panic even with no processes
	AbortAll()

	// Verify the process finished
	_ = cmd.Wait()
}

func TestRegisterAutoUnregister(t *testing.T) {
	cmd := exec.Command("true")
	_ = cmd.Start()
	Register(cmd)

	// Wait for auto-unregister via Wait goroutine
	_ = cmd.Wait()
	time.Sleep(50 * time.Millisecond)

	// AbortAll should be safe with empty registry
	AbortAll()
}
