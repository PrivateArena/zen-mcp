package terminal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
)

// Handler is a terminal command handler (mirrors the handlers group in TS).
type Handler func(args []string, sessionID string) error

var (
	mu       sync.RWMutex
	handlers = map[string]Handler{}
)

// LogOut is where [ZEN-CLI] log lines are written (stderr by default).
var LogOut io.Writer = os.Stderr

func init() {
	Register("help", func(_ []string, _ string) error {
		logf("Available commands: %s", strings.Join(List(), ", "))
		return nil
	})
}

// Register installs a command handler.
func Register(name string, h Handler) {
	mu.Lock()
	defer mu.Unlock()
	handlers[name] = h
}

// Get returns the handler for a command, if any.
func Get(name string) (Handler, bool) {
	mu.RLock()
	defer mu.RUnlock()
	h, ok := handlers[name]
	return h, ok
}

// List returns registered command names sorted.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(handlers))
	for n := range handlers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// logf mirrors the TS terminal `log()` helper ([ZEN-CLI] prefix).
func logf(format string, args ...any) {
	fmt.Fprintf(LogOut, "[ZEN-CLI] "+format+"\n", args...)
}

// FallbackPort resolves the export-cli port when the CLI listener failed with
// EADDRINUSE: prefer cliPort, otherwise fall back to mcpPort (index.ts).
func FallbackPort(cliPort, mcpPort int, cliAvailable bool) int {
	if cliAvailable {
		return cliPort
	}
	return mcpPort
}

// runCommander is the line REPL loop. Split for testability.
func runCommander(r io.Reader, sessionID string, prompt io.Writer) {
	logf("Human-in-the-Loop Active. Available: type help for commands.")
	scanner := bufio.NewScanner(r)
	for {
		fmt.Fprint(prompt, "> ")
		if !scanner.Scan() {
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		cmd := parts[0]
		if cmd == "exit" || cmd == "quit" {
			logf("Shutting down terminal commander.")
			return
		}
		h, ok := Get(cmd)
		if !ok {
			logf("Unknown command: %s (type help)", cmd)
			continue
		}
		logf("START: %s...", cmd)
		if err := h(parts[1:], sessionID); err != nil {
			logf("ERROR: %v", err)
		}
	}
}

// StartTerminalCommander launches the REPL in a background goroutine.
func StartTerminalCommander(sessionID string) {
	go runCommander(os.Stdin, sessionID, os.Stdout)
}
