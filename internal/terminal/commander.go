package terminal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	mcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/jang/zen-mcp/internal/tools"
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

// showResult formats and prints a result object as JSON.
func showResult(data any) {
	b, _ := json.MarshalIndent(data, "", "  ")
	if len(b) == 0 {
		b = []byte("null")
	}
	fmt.Fprintf(LogOut, "[ZEN-CLI] RESULT:\n%s\n", string(b))
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

// parseCodegraphArgs parses codegraph command arguments.
func parseCodegraphArgs(args []string) ParsedCodegraphArgs {
	result := ParsedCodegraphArgs{Isolate: 0}
	for _, a := range args {
		switch {
		case isNumeric(a):
			i := parseInt(a)
			result.Limit = &i
		case strings.HasPrefix(a, "isolate="):
			result.Isolate = parseInt(strings.TrimPrefix(a, "isolate="))
		case a == "--json":
			result.FormatJson = true
		case a == "--dry-run":
			result.DryRun = true
		case a != "--force" && a != "---force" && !strings.HasPrefix(a, "--"):
			result.Query = a
		}
	}
	return result
}

// runCommander is the line REPL loop. Split for testability.
func runCommander(r io.Reader, prompt io.Writer) {
	Logf("Human-in-the-Loop Active. Available: type help for commands.")
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
			Logf("Shutting down terminal commander.")
			return
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

// StartTerminalCommander launches the REPL in a background goroutine.
func StartTerminalCommander() {
	go runCommander(os.Stdin, os.Stdout)
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

	var result *mcp.CallToolResult
	switch name {
	case "codegraph":
		result = tools.HandleCodegraphAction(ctx, "", d, req)
	case "memory":
		result = tools.HandleMemoryAction(ctx, "", d, req)
	case "memory_isolate":
		result = tools.HandleMemoryIsolateAction(ctx, "", d, req)
	case "memory_shared":
		result = tools.HandleMemorySharedAction(ctx, "", d, req)
	case "shell":
		result = tools.HandleShellAction(ctx, "", d, req)
	case "browser":
		result = tools.HandleBrowserAction(ctx, "", d, req)
	case "skill":
		result = tools.HandleSkillsAction(ctx, "", d, req)
	case "context":
		result = tools.HandleContextAction(ctx, "", d, req)
	case "ui-vision":
		result = tools.HandleUiVisionAction(ctx, "", d, req)
	case "workspace":
		path, _ := args["path"].(string)
		result = tools.HandleWorkspaceAction(ctx, path, "", d)
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
