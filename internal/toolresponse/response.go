package toolresponse

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/markdown"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/telemetry"
	"zen-mcp/internal/toolsuggestions"
)

type CommandResult struct {
	Command      string  `json:"command,omitempty"`
	Stdout       string  `json:"stdout"`
	Stderr       string  `json:"stderr,omitempty"`
	ExitCode     int     `json:"exitCode"`
	Aborted      bool    `json:"aborted,omitempty"`
	TimedOut     string  `json:"timedOut,omitempty"`
	ActTimeoutMs *int    `json:"actTimeoutMs,omitempty"`
	Timeout      *int    `json:"timeout,omitempty"`
	Savings      *string `json:"savings,omitempty"`
}

func RenderOutput(format string, data any) string {
	if data == nil {
		return ""
	}
	if s, ok := data.(string); ok {
		return s
	}
	if cr, ok := asCommandResult(data); ok {
		return renderCommandResult(format, cr)
	}
	serialized, err := json.Marshal(data)
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	if len(serialized) > 64*1024 {
		logfilter.Debugf("[ToolResponse] Payload too large for structured formatting (%d bytes). Downgrading to raw.", len(serialized))
		return string(serialized)
	}
	switch format {
	case "json":
		return string(serialized)
	case "md":
		md := markdown.JSONToMarkdown(string(serialized))
		if md != "" {
			return md
		}
		fence := "```"
		if strings.Contains(string(serialized), "```") {
			fence = "````"
		}
		return fence + "json\n" + string(serialized) + "\n" + fence
	}
	return string(serialized)
}

func renderCommandResult(format string, cr CommandResult) string {
	if format == "raw" {
		outputText := strings.TrimSpace(cr.Stdout)
		cleanStderr := strings.TrimSpace(cr.Stderr)
		if cleanStderr != "" {
			if outputText != "" {
				outputText += "\n"
			}
			outputText += cleanStderr
		}
		switch cr.TimedOut {
		case "activity":
			ms := 30000
			if cr.ActTimeoutMs != nil {
				ms = *cr.ActTimeoutMs
			}
			msg := fmt.Sprintf("⏱ Timed out (idle for %dms with no output)", ms)
			if outputText != "" {
				outputText = outputText + "\n" + msg
			} else {
				outputText = msg
			}
		case "hard":
			ms := 60000
			if cr.Timeout != nil {
				ms = *cr.Timeout
			}
			msg := fmt.Sprintf("⏱ Timed out (hard ceiling %dms)", ms)
			if outputText != "" {
				outputText = outputText + "\n" + msg
			} else {
				outputText = msg
			}
		default:
			if cr.Aborted {
				msg := "× Aborted by user"
				if outputText != "" {
					outputText = outputText + "\n" + msg
				} else {
					outputText = msg
				}
			}
		}
		if outputText != "" {
			return outputText
		}
		if cr.ExitCode == 0 {
			return "✓ Success"
		}
		return "× Failed (no output)"
	}

	if format == "md" {
		var parts []string
		if cr.Command != "" {
			parts = append(parts, fmt.Sprintf("**Command:** `%s`", cr.Command))
		}
		cleanStdout := strings.TrimSpace(cr.Stdout)
		if cleanStdout != "" {
			fence := "```"
			if strings.Contains(cleanStdout, "```") {
				fence = "````"
			}
			parts = append(parts, fmt.Sprintf("**stdout:**\n%s\n%s\n%s", fence, cleanStdout, fence))
		}
		cleanStderr := strings.TrimSpace(cr.Stderr)
		if cleanStderr != "" {
			fence := "```"
			if strings.Contains(cleanStderr, "```") {
				fence = "````"
			}
			parts = append(parts, fmt.Sprintf("**stderr:**\n%s\n%s\n%s", fence, cleanStderr, fence))
		}
		if cr.ExitCode != 0 {
			parts = append(parts, fmt.Sprintf("**Exit Code:** `%d`", cr.ExitCode))
		}
		switch cr.TimedOut {
		case "activity":
			ms := 30000
			if cr.ActTimeoutMs != nil {
				ms = *cr.ActTimeoutMs
			}
			parts = append(parts, fmt.Sprintf("⏱ **Timed out:** Idle for %dms with no output.", ms))
		case "hard":
			ms := 60000
			if cr.Timeout != nil {
				ms = *cr.Timeout
			}
			parts = append(parts, fmt.Sprintf("⏱ **Timed out:** Hard ceiling %dms reached.", ms))
		default:
			if cr.Aborted {
				parts = append(parts, "× **Aborted:** Cancelled by user.")
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
		return "✓ Success"
	}

	// fallthrough for other formats on a CommandResult
	serialized, err := json.Marshal(cr)
	if err != nil {
		return fmt.Sprintf("%v", cr)
	}
	return string(serialized)
}

func asCommandResult(v any) (CommandResult, bool) {
	switch t := v.(type) {
	case CommandResult:
		return t, true
	case *CommandResult:
		if t != nil {
			return *t, true
		}
	case map[string]any:
		var cr CommandResult
		if s, ok := t["stdout"].(string); ok {
			cr.Stdout = s
		}
		if s, ok := t["stderr"].(string); ok {
			cr.Stderr = s
		}
		if s, ok := t["command"].(string); ok {
			cr.Command = s
		}
		if s, ok := t["timedOut"].(string); ok {
			cr.TimedOut = s
		}
		if b, ok := t["aborted"].(bool); ok {
			cr.Aborted = b
		}
		if n, ok := t["exitCode"].(float64); ok {
			cr.ExitCode = int(n)
		}
		if n, ok := t["exitCode"].(int); ok {
			cr.ExitCode = n
		}
		if _, hasStdout := t["stdout"]; hasStdout {
			return cr, true
		}
	}
	return CommandResult{}, false
}

// ---- Tool context (per-request, carries toolName + params) ----

type ToolContext struct {
	ToolName string
	Params   map[string]any
}

type toolContextKey struct{}

func WithToolContext(ctx context.Context, tc ToolContext) context.Context {
	return context.WithValue(ctx, toolContextKey{}, tc)
}

func ToolContextFrom(ctx context.Context) (ToolContext, bool) {
	tc, ok := ctx.Value(toolContextKey{}).(ToolContext)
	return tc, ok
}

func ToolActionFromContext(ctx context.Context) string {
	tc, ok := ToolContextFrom(ctx)
	if !ok {
		return ""
	}
	if action, ok := tc.Params["action"].(string); ok {
		return action
	}
	return ""
}

// ---- Orphan flag (abandoned tool calls) ----

type orphanFlagKey struct{}

// MarkWithOrphanFlag attaches a flag that records whether the tool call was
// abandoned (client abort or server timeout) before the handler returned. The
// handler goroutine may still be running in the background; result wrapping in
// that goroutine must not emit telemetry or error logs for a call the client
// never observed.
func MarkWithOrphanFlag(ctx context.Context, flag *atomic.Bool) context.Context {
	return context.WithValue(ctx, orphanFlagKey{}, flag)
}

// isOrphaned reports whether the tool call was abandoned before it completed.
func isOrphaned(ctx context.Context) bool {
	flag, ok := ctx.Value(orphanFlagKey{}).(*atomic.Bool)
	return ok && flag.Load()
}

// ---- Schema registry (mirrors toolSchemas Map in TS) ----

var (
	schemaMu    sync.RWMutex
	toolSchemas = make(map[string]map[string]any)
)

func SetToolSchema(name string, schema map[string]any) {
	schemaMu.Lock()
	defer schemaMu.Unlock()
	toolSchemas[name] = schema
}

func GetToolSchema(name string) map[string]any {
	schemaMu.RLock()
	defer schemaMu.RUnlock()
	return toolSchemas[name]
}

// ---- Virtualization hook (wired by tokenoptimizer in F5) ----

var virtualizeFunc func(tool, text string) (string, error)

func SetVirtualizer(fn func(tool, text string) (string, error)) {
	virtualizeFunc = fn
}

// ---- Result wrappers ----

func WrapSuccess(ctx context.Context, tool string, data any, start time.Time) *mcp.CallToolResult {
	toolCfg := mcpcfg.GetToolConfig(tool)
	text := RenderOutput(string(toolCfg.Format), data)

	action := ToolActionFromContext(ctx)
	if kind := timedOutKind(data); kind != "" && !isOrphaned(ctx) {
		reportCommandTimeout(tool, action, kind, time.Since(start).Milliseconds())
	} else if !isOrphaned(ctx) {
		_ = telemetry.LogToolCall(tool, action, true, "", time.Since(start).Milliseconds())
	}

	bypass := mcpcfg.Get().BypassTools
	if len(bypass) == 0 {
		bypass = []string{"session", "memory", "workspace", "think", "run"}
	}
	if !contains(bypass, tool) && virtualizeFunc != nil {
		if virtualized, err := virtualizeFunc(tool, text); err == nil {
			text = virtualized
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}},
	}
}

// timedOutKind extracts the exec-level timeout kind ("hard" or "activity")
// carried by command-style tool results, or "" when the payload is not a
// timed-out command.
func timedOutKind(data any) string {
	if m, ok := data.(map[string]any); ok {
		if s, ok := m["timedOut"].(string); ok && s != "" {
			return s
		}
	}
	if cr, ok := asCommandResult(data); ok && cr.TimedOut != "" {
		return cr.TimedOut
	}
	return ""
}

// reportCommandTimeout logs every exec-level tool timeout on the MCP side and
// records it in telemetry as a failure so timeouts are visible and queryable.
func reportCommandTimeout(tool, action, kind string, elapsedMs int64) {
	actionLabel := action
	if actionLabel == "" {
		actionLabel = "(none)"
	}
	logfilter.Errorf("[MCP] TIMER Tool '%s' TIMED OUT (%s) after %dms — action %q", tool, kind, elapsedMs, actionLabel)
	_ = telemetry.LogToolCall(tool, action, false, fmt.Sprintf("timed out (%s) after %dms", kind, elapsedMs), elapsedMs)
	if onCommandTimeout != nil {
		onCommandTimeout(tool, kind, elapsedMs)
	}
}

// onCommandTimeout is a test-only observer, invoked after a timeout is logged.
var onCommandTimeout func(tool, kind string, elapsedMs int64)

func WrapError(tool string, err error, start time.Time) *mcp.CallToolResult {
	return WrapErrorWithContext(context.Background(), tool, err, start)
}

func WrapErrorWithContext(ctx context.Context, tool string, err error, start time.Time) *mcp.CallToolResult {
	return wrapErrorCtx(ctx, tool, err, start)
}

func wrapErrorCtx(ctx context.Context, tool string, err error, start time.Time) *mcp.CallToolResult {
	errorMessage := err.Error()
	duration := time.Since(start).Milliseconds()

	action := ToolActionFromContext(ctx)
	if !isOrphaned(ctx) {
		logfilter.Errorf("[%s] FAILED after %dms: %s", tool, duration, errorMessage)
		_ = telemetry.LogToolCall(tool, action, false, errorMessage, duration)
	}

	schema := GetToolSchema(tool)
	stackTrace := errorStack(err)

	showSuggestion := mcpcfg.Get().ToolSuggestionsEnabled && mcpcfg.Get().ToolSuggestionStyle != "lite"
	suggestion := ""
	if showSuggestion {
		suggestion = toolsuggestions.FormatSuggestion(tool, errorMessage, action, schema)
	}

	actionLabel := action
	if actionLabel == "" {
		actionLabel = "(no action)"
	}

	toolCfg := mcpcfg.GetToolConfig(tool)
	if toolCfg.Format == "raw" {
		var lines []string
		lines = append(lines, fmt.Sprintf("❌ %s failed", tool))
		lines = append(lines, "Action: "+actionLabel)
		lines = append(lines, "Error: "+errorMessage)
		if stackTrace != "" {
			stackLines := strings.Split(stackTrace, "\n")
			var atParts []string
			for i := 1; i < len(stackLines) && len(atParts) < 2; i++ {
				if s := strings.TrimSpace(stackLines[i]); s != "" {
					atParts = append(atParts, s)
				}
			}
			if at := strings.Join(atParts, " → "); at != "" {
				lines = append(lines, "At: "+at)
			}
		}
		if suggestion != "" {
			lines = append(lines, "\n"+suggestion)
		}

		text := strings.Join(filterEmpty(lines), "\n")
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}},
			IsError: false,
		}
	}

	data := map[string]any{
		"error":   true,
		"tool":    tool,
		"action":  actionLabel,
		"message": errorMessage,
	}
	if stackTrace != "" {
		data["stack"] = stackTrace
	}
	if suggestion != "" {
		data["suggestion"] = suggestion
	}

	text := RenderOutput(string(toolCfg.Format), data)
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}},
		IsError: false,
	}
}

func filterEmpty(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func errorStack(err error) string {
	pcs := make([]uintptr, 8)
	n := runtime.Callers(3, pcs)
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])
	var lines []string
	for {
		f, more := frames.Next()
		lines = append(lines, fmt.Sprintf("    at %s (%s:%d)", f.Function, filepath.Base(f.File), f.Line))
		if !more || len(lines) >= 3 {
			break
		}
	}
	return "Error: " + err.Error() + "\n" + strings.Join(lines, "\n")
}
