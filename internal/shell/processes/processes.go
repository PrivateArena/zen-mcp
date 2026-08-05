// Package processes ports src/lib/shell/process-registry.ts: a registry of
// in-flight shell child processes that can be aborted wholesale.
package processes

import (
	"os/exec"
	"sync"
	"syscall"
)

var (
	mu  sync.Mutex
	set = map[*exec.Cmd]struct{}{}
)

// Register tracks cmd and unregisters it when it exits.
func Register(cmd *exec.Cmd) {
	mu.Lock()
	set[cmd] = struct{}{}
	mu.Unlock()

	go func() {
		_ = cmd.Wait()
		mu.Lock()
		delete(set, cmd)
		mu.Unlock()
	}()
}

// AbortAll kills every registered child process group (SIGKILL) and waits.
func AbortAll() {
	mu.Lock()
	cmds := make([]*exec.Cmd, 0, len(set))
	for c := range set {
		cmds = append(cmds, c)
	}
	mu.Unlock()

	for _, cmd := range cmds {
		killGroup(cmd)
	}
	for _, cmd := range cmds {
		_ = cmd.Wait()
	}
}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if pid <= 0 {
		return
	}
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if err != nil {
		_ = cmd.Process.Kill()
	}
}
