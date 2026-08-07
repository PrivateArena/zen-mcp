// Package exec ports the spawn + dual-timeout execution in src/tools/shell.ts.
package exec

import (
	"os/exec"
	"sync"
	"syscall"
	"time"

	"zen-mcp/internal/shell/processes"
)

// Result mirrors the TS promise resolution for a shell run.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Aborted  bool
	TimedOut string // "", "activity", or "hard"
}

// Run spawns `sh -c command` in cwd as a detached process group, enforces an
// optional hard timeout and an activity timeout (reset on output), and
// returns when the child closes.
func Run(command, cwd string, timeoutMs, activityTimeoutMs int) Result {
	cmd := exec.Command("sh", "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	var (
		mu       sync.Mutex
		stdout   []byte
		stderr   []byte
		timedOut = ""
	)

	activityCh := make(chan struct{}, 8)
	read := func(r interface{ Read([]byte) (int, error) }, dst *[]byte) {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				mu.Lock()
				*dst = append(*dst, buf[:n]...)
				mu.Unlock()
				select {
				case activityCh <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}

	kill := func() {
		if cmd.Process == nil {
			return
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err != nil {
			_ = cmd.Process.Kill()
		}
	}

	hardTimer := time.NewTimer(time.Hour)
	if timeoutMs > 0 {
		hardTimer.Reset(time.Duration(timeoutMs) * time.Millisecond)
	}
	activityTimer := time.NewTimer(time.Hour)
	if activityTimeoutMs > 0 {
		activityTimer.Reset(time.Duration(activityTimeoutMs) * time.Millisecond)
	}

	done := make(chan struct{})
	var closeOnce sync.Once
	finish := func(t string) {
		closeOnce.Do(func() {
			mu.Lock()
			timedOut = t
			mu.Unlock()
			close(done)
		})
	}

	if err := cmd.Start(); err != nil {
		// Mirrors the TS 'error' event: exitCode = err.status || 1.
		code := 1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return Result{
			Stdout:   "",
			Stderr:   err.Error(),
			ExitCode: code,
			Aborted:  false,
		}
	}
	processes.Register(cmd)
	defer processes.Unregister(cmd)

	go read(stdoutPipe, &stdout)
	go read(stderrPipe, &stderr)

	go func() {
		for {
			select {
			case <-hardTimer.C:
				kill()
				finish("hard")
			case <-activityTimer.C:
				kill()
				finish("activity")
			case <-activityCh:
				if activityTimeoutMs > 0 {
					activityTimer.Reset(time.Duration(activityTimeoutMs) * time.Millisecond)
				}
			case <-done:
				return
			}
		}
	}()

	err := cmd.Wait()
	hardTimer.Stop()
	activityTimer.Stop()
	finish("")

	var exitCode int
	signal130 := false
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				exitCode = 130
				signal130 = true
			}
		} else {
			exitCode = 1
		}
	}

	mu.Lock()
	defer mu.Unlock()
	aborted := timedOut != "" || signal130
	return Result{
		Stdout:   string(stdout),
		Stderr:   string(stderr),
		ExitCode: exitCode,
		Aborted:  aborted,
		TimedOut: timedOut,
	}
}

const maxOutput = 10 * 1024 * 1024
const truncationMark = "\n[output truncated at 10MB]"

// SandboxResult mirrors the run.ts promise result.
type SandboxResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut string // "", "activity", or "hard"
}

// RunSandbox ports registerSandboxReplTool's spawn block: runs an interpreter
// with args (no shell), optional stdin, hard + activity timers, and 10MB
// output truncation.
func RunSandbox(name string, args []string, cwd, stdin string, activityMs, hardMs int) SandboxResult {
	cmd := exec.Command(name, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()
	stdinPipe, _ := cmd.StdinPipe()

	var (
		mu       sync.Mutex
		stdout   []byte
		stderr   []byte
		timedOut = ""
	)

	activityCh := make(chan struct{}, 8)
	read := func(r interface{ Read([]byte) (int, error) }, dst *[]byte, trunc *bool) {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				mu.Lock()
				if !*trunc {
					if len(*dst) < maxOutput {
						*dst = append(*dst, buf[:n]...)
					} else {
						*dst = append(*dst, truncationMark...)
						*trunc = true
					}
				}
				mu.Unlock()
				select {
				case activityCh <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}

	kill := func() {
		if cmd.Process == nil {
			return
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err != nil {
			_ = cmd.Process.Kill()
		}
	}

	done := make(chan struct{})
	var closeOnce sync.Once
	finish := func(t string) {
		closeOnce.Do(func() {
			mu.Lock()
			timedOut = t
			mu.Unlock()
			close(done)
		})
	}

	var stdoutTrunc, stderrTrunc bool
	if err := cmd.Start(); err != nil {
		code := 1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return SandboxResult{Stdout: "", Stderr: err.Error(), ExitCode: code}
	}
	processes.Register(cmd)
	defer processes.Unregister(cmd)

	go func() {
		if stdinPipe != nil {
			_, _ = stdinPipe.Write([]byte(stdin))
			_ = stdinPipe.Close()
		}
	}()
	go read(stdoutPipe, &stdout, &stdoutTrunc)
	go read(stderrPipe, &stderr, &stderrTrunc)

	hardTimer := time.NewTimer(time.Hour)
	if hardMs > 0 {
		hardTimer.Reset(time.Duration(hardMs) * time.Millisecond)
	}
	activityTimer := time.NewTimer(time.Hour)
	if activityMs > 0 {
		activityTimer.Reset(time.Duration(activityMs) * time.Millisecond)
	}

	go func() {
		for {
			select {
			case <-hardTimer.C:
				kill()
				finish("hard")
			case <-activityTimer.C:
				kill()
				finish("activity")
			case <-activityCh:
				if activityMs > 0 {
					activityTimer.Reset(time.Duration(activityMs) * time.Millisecond)
				}
			case <-done:
				return
			}
		}
	}()

	err := cmd.Wait()
	hardTimer.Stop()
	activityTimer.Stop()
	finish("")

	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = 1
		}
	}

	mu.Lock()
	defer mu.Unlock()
	return SandboxResult{
		Stdout:   string(stdout),
		Stderr:   string(stderr),
		ExitCode: exitCode,
		TimedOut: timedOut,
	}
}
