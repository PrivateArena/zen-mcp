package terminal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"zen-mcp/internal/tools"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"golang.org/x/sys/unix"
)

// Handler is a terminal command handler.
type Handler func(args []string) error

var (
	mu       sync.RWMutex
	handlers = map[string]Handler{}
	deps     tools.Deps
)

// SetDeps sets the dependencies for terminal handlers.
func SetDeps(d tools.Deps) {
	deps = d
}

// GetDeps returns the dependencies for terminal handlers.
func GetDeps() tools.Deps {
	return deps
}

// LogOut is where [ZEN-CLI] log lines are written (stderr by default).
var LogOut io.Writer = os.Stderr

func init() {
	Register("help", func(_ []string) error {
		Logf("Available commands: %s", strings.Join(List(), ", "))
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

// Logf mirrors the TS terminal `log()` helper ([ZEN-CLI] prefix).
func Logf(format string, args ...any) {
	now := time.Now()
	ts := fmt.Sprintf("[%02d:%02d:%02d.%03d]", now.Hour(), now.Minute(), now.Second(), now.Nanosecond()/1e6)
	fmt.Fprintf(LogOut, "%s [ZEN-CLI] "+format+"\n", append([]any{ts}, args...)...)
}

// FallbackPort resolves the export-cli port when the CLI listener failed with
// EADDRINUSE: prefer cliPort, otherwise fall back to mcpPort (index.ts).
func FallbackPort(cliPort, mcpPort int, cliAvailable bool) int {
	if cliAvailable {
		return cliPort
	}
	return mcpPort
}

// Ws returns the workspace root for the given session, falling back to cwd.
func Ws() string {
	d := GetDeps()
	if d.Store != nil {
		if w, ok := d.Store.Get("workspace-root"); ok && w != "" {
			return w
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

// isNumeric checks if a string consists only of digits.
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// parseInt parses an integer from string, returning 0 on failure.
func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

// ParsedCodegraphArgs holds parsed codegraph-style CLI arguments.
type ParsedCodegraphArgs struct {
	Query      string
	Limit      *int
	Isolate    int
	FormatJson bool
	DryRun     bool
}

// ParseCodegraphArgs parses codegraph command arguments.
func ParseCodegraphArgs(args []string) ParsedCodegraphArgs {
	result := ParsedCodegraphArgs{Isolate: 0}
	for _, a := range args {
		switch {
		case isNumeric(a):
			if result.Limit == nil {
				i := parseInt(a)
				result.Limit = &i
			}
		case strings.HasPrefix(a, "isolate="):
			result.Isolate = parseInt(strings.TrimPrefix(a, "isolate="))
		case a == "--json":
			result.FormatJson = true
		case a == "--dry-run":
			result.DryRun = true
		case a != "--force" && a != "---force" && !strings.HasPrefix(a, "--"):
			if result.Query == "" {
				result.Query = a
			}
		}
	}
	return result
}

// runCommander is the line REPL loop. Split for testability.
func runCommander(r io.Reader, prompt io.Writer) bool {
	Logf("Human-in-the-Loop Active. Available: type help for commands.")

	var useRaw bool
	var rawFile *os.File
	if f, ok := r.(*os.File); ok {
		useRaw = isTerminal(int(f.Fd()))
		if useRaw {
			rawFile = f
		}
	}

	if !useRaw {
		return runCommanderScanner(r, prompt)
	}

	fd := rawFile.Fd()
	oldTermios, err := setRawMode(int(fd))
	if err != nil {
		Logf("WARN: failed to set raw mode: %v", err)
		return runCommanderScanner(r, prompt)
	}
	defer func() {
		if err := restoreTerminal(int(fd), oldTermios); err != nil {
			Logf("WARN: failed to restore terminal: %v", err)
		}
	}()

	history := []string{}
	historyIdx := -1
	var currentLine []byte
	buf := make([]byte, 1)

	fmt.Fprint(prompt, "> ")
	for {
		n, err := rawFile.Read(buf)
		if err != nil || n == 0 {
			fmt.Fprint(prompt, "\r\n")
			return false
		}
		b := buf[0]

		switch b {
		case '\r', '\n':
			line := strings.TrimSpace(string(currentLine))
			if line == "" {
				fmt.Fprint(prompt, "\r\n> ")
				continue
			}
			history = append(history, line)
			historyIdx = len(history)
			currentLine = nil
			fmt.Fprint(prompt, "\r\n")
			oldLogOut := LogOut
			LogOut = prompt
			shouldExit := dispatch(line, prompt)
			LogOut = oldLogOut
			if shouldExit {
				return true
			}
			fmt.Fprint(prompt, "> ")
		case 127, 8:
			if len(currentLine) > 0 {
				currentLine = currentLine[:len(currentLine)-1]
				redraw(prompt, currentLine)
			}
		case 3:
			fmt.Fprint(prompt, "\r\n")
			Logf("Shutting down terminal commander.")
			return true
		case 4:
			if len(currentLine) == 0 {
				fmt.Fprint(prompt, "\r\n")
				Logf("Shutting down terminal commander.")
				return true
			}
		case '\x1b':
			seq := make([]byte, 2)
			_, err := rawFile.Read(seq)
			if err != nil {
				return false
			}
			if seq[0] == '[' {
				switch seq[1] {
				case 'A':
					if len(history) == 0 {
					} else if historyIdx == -1 {
						historyIdx = len(history) - 1
						currentLine = []byte(history[historyIdx])
						redraw(prompt, currentLine)
					} else if historyIdx > 0 {
						historyIdx--
						currentLine = []byte(history[historyIdx])
						redraw(prompt, currentLine)
					}
				case 'B':
					if historyIdx >= 0 && historyIdx < len(history)-1 {
						historyIdx++
						currentLine = []byte(history[historyIdx])
						redraw(prompt, currentLine)
					} else if historyIdx >= 0 {
						historyIdx = -1
						currentLine = nil
						redraw(prompt, currentLine)
					}
				}
			}
		default:
			if b >= 32 && b <= 126 {
				currentLine = append(currentLine, b)
				redraw(prompt, currentLine)
			}
		}
	}
}

// runCommanderScanner handles non-TTY input using bufio.Scanner.
func runCommanderScanner(r io.Reader, prompt io.Writer) bool {
	scanner := bufio.NewScanner(r)
	for {
		fmt.Fprint(prompt, "> ")
		if !scanner.Scan() {
			return false
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		cmd := parts[0]
		if cmd == "exit" || cmd == "quit" {
			Logf("Shutting down terminal commander.")
			return true
		}
		h, ok := Get(cmd)
		if !ok {
			Logf("Unknown command: %s (type help)", cmd)
			continue
		}
		Logf("START: %s...", cmd)
		if err := h(parts[1:]); err != nil {
			Logf("ERROR: %v", err)
		}
	}
}

// dispatch executes a command line and reports whether the REPL should exit.
func dispatch(line string, prompt io.Writer) bool {
	parts := strings.Fields(line)
	cmd := parts[0]
	if cmd == "exit" || cmd == "quit" {
		Logf("Shutting down terminal commander.")
		return true
	}
	h, ok := Get(cmd)
	if !ok {
		Logf("Unknown command: %s (type help)", cmd)
		return false
	}
	Logf("START: %s...", cmd)
	if err := h(parts[1:]); err != nil {
		Logf("ERROR: %v", err)
	}
	return false
}

// redraw clears the current line and rewrites the prompt with the current input.
func redraw(prompt io.Writer, line []byte) {
	fmt.Fprint(prompt, "\r")
	fmt.Fprint(prompt, "\033[2K")
	fmt.Fprint(prompt, "> ")
	fmt.Fprint(prompt, string(line))
}

// isTerminal reports whether fd is a terminal.
func isTerminal(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	return err == nil
}

// setRawMode puts the terminal into raw mode.
func setRawMode(fd int) (*unix.Termios, error) {
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}
	new := *old
	new.Iflag &^= unix.ICRNL | unix.INLCR | unix.IGNCR
	new.Iflag &^= unix.IXON
	new.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
	new.Cc[unix.VMIN] = 1
	new.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &new); err != nil {
		return nil, err
	}
	return old, nil
}

// restoreTerminal restores the terminal to its previous settings.
func restoreTerminal(fd int, old *unix.Termios) error {
	if old != nil {
		return unix.IoctlSetTermios(fd, unix.TCSETS, old)
	}
	return nil
}

// StartTerminalCommander launches the REPL in a background goroutine.
func StartTerminalCommander() {
	go func() {
		if runCommander(os.Stdin, os.Stdout) {
			p, _ := os.FindProcess(os.Getpid())
			if p != nil {
				_ = p.Signal(syscall.SIGINT)
			}
		}
	}()
}

// MakeFakeRequest creates a fake mcp.CallToolRequest from args.
func MakeFakeRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "",
			Arguments: args,
		},
	}
}

// ExecuteTool executes a tool by name with arguments and returns the result text.
func ExecuteTool(name string, args map[string]any) string {
	d := GetDeps()
	if d.Store == nil && d.Reg == nil {
		return "ERROR: terminal handlers not initialized with deps"
	}

	ctx := context.Background()
	req := MakeFakeRequest(args)
	ws := Ws()

	var result *mcp.CallToolResult
	switch name {
	case "codegraph":
		result = tools.HandleCodegraphAction(ctx, ws, d, req)
	case "memory":
		result = tools.HandleMemoryAction(ctx, ws, d, req)
	case "memory_isolate":
		result = tools.HandleMemoryIsolateAction(ctx, ws, d, req)
	case "memory_shared":
		result = tools.HandleMemorySharedAction(ctx, ws, d, req)
	case "shell":
		result = tools.HandleShellAction(ctx, ws, d, req)
	case "browser":
		result = tools.HandleBrowserAction(ctx, ws, d, req)
	case "skill":
		result = tools.HandleSkillsAction(ctx, ws, d, req)
	case "context":
		result = tools.HandleContextAction(ctx, ws, d, req)
	case "ui-vision":
		result = tools.HandleUiVisionAction(ctx, ws, d, req)
	case "workspace":
		path, _ := args["path"].(string)
		result = tools.HandleWorkspaceAction(ctx, path, ws, d)
	default:
		return "ERROR: unknown tool " + name
	}

	if result == nil {
		return "ERROR: nil result"
	}

	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return "ERROR: no text content"
}
