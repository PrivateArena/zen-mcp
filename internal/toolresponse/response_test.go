package toolresponse

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
)

func intPtr(v int) *int { return &v }

func IsCommandResult(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		cr, ok := v.(CommandResult)
		return ok && cr.Stdout != ""
	}
	_, hasStdout := m["stdout"].(string)
	_, hasExit := m["exitCode"]
	return hasStdout && hasExit
}

// SetTimeoutObserver registers a callback invoked whenever a tool result
// carries an exec-level timeout. Intended for tests; nil by default.
func SetTimeoutObserver(fn func(tool, kind string, elapsedMs int64)) {
	onCommandTimeout = fn
}

func TestRenderOutputStringPassthrough(t *testing.T) {
	if got := RenderOutput("json", "plain text"); got != "plain text" {
		t.Errorf("string data should pass through, got %q", got)
	}
}

func TestRenderOutputRawCommandResult(t *testing.T) {
	cr := CommandResult{Command: "echo hi", Stdout: "hi\n", ExitCode: 0}
	if got := RenderOutput("raw", cr); got != "hi" {
		t.Errorf("raw = %q", got)
	}
	cr.Stderr = "warn"
	if got := RenderOutput("raw", cr); !strings.Contains(got, "hi\nwarn") {
		t.Errorf("raw with stderr = %q", got)
	}
}

func TestRenderOutputRawEmptySuccess(t *testing.T) {
	if got := RenderOutput("raw", CommandResult{Stdout: "", ExitCode: 0}); got != "✓ Success" {
		t.Errorf("empty success = %q", got)
	}
	if got := RenderOutput("raw", CommandResult{Stdout: "", ExitCode: 2}); got != "× Failed (no output)" {
		t.Errorf("empty failure = %q", got)
	}
}

func TestRenderOutputRawTimeoutMessages(t *testing.T) {
	cr := CommandResult{Stdout: "partial", TimedOut: "activity", ActTimeoutMs: intPtr(30000)}
	got := RenderOutput("raw", cr)
	if !strings.Contains(got, "idle for 30000ms") {
		t.Errorf("activity timeout message missing: %q", got)
	}
	cr = CommandResult{Stdout: "partial", TimedOut: "hard", Timeout: intPtr(60000)}
	got = RenderOutput("raw", cr)
	if !strings.Contains(got, "hard ceiling 60000ms") {
		t.Errorf("hard timeout message missing: %q", got)
	}
	cr = CommandResult{Stdout: "", Aborted: true}
	if got := RenderOutput("raw", cr); got != "× Aborted by user" {
		t.Errorf("aborted message = %q", got)
	}
}

func TestRenderOutputMD(t *testing.T) {
	cr := CommandResult{Command: "ls", Stdout: "a\nb", ExitCode: 0}
	got := RenderOutput("md", cr)
	if !strings.Contains(got, "**Command:** `ls`") {
		t.Errorf("md command line missing: %s", got)
	}
	if !strings.Contains(got, "a\nb") {
		t.Errorf("md stdout missing: %s", got)
	}
	cr.ExitCode = 3
	got = RenderOutput("md", cr)
	if !strings.Contains(got, "**Exit Code:** `3`") {
		t.Errorf("md exit code missing: %s", got)
	}
}

func TestRenderOutputJSON(t *testing.T) {
	cr := CommandResult{Command: "ls", Stdout: "a", ExitCode: 0}
	got := RenderOutput("json", cr)
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("invalid json: %v (%s)", err, got)
	}
	if m["command"] != "ls" || m["exitCode"] != float64(0) {
		t.Errorf("json content wrong: %v", m)
	}
}

func TestRenderOutputFallbackJSON(t *testing.T) {
	got := RenderOutput("weird", CommandResult{Command: "x", Stdout: "o"})
	if !strings.HasPrefix(got, "{") {
		t.Errorf("unknown format should fall back to json: %q", got)
	}
}

func TestRenderOutputOtherData(t *testing.T) {
	got := RenderOutput("json", map[string]any{"status": "ok"})
	if got != "{\"status\":\"ok\"}" {
		t.Errorf("map json = %q", got)
	}
}

func TestRenderOutputMDMap(t *testing.T) {
	got := RenderOutput("md", map[string]any{"message": "Workspace -> /x", "path": "/x", "prev_path": "/x", "tools_changed": []any{}})
	if strings.Contains(got, "```json") {
		t.Errorf("md map should not be wrapped as json fence: %s", got)
	}
	if !strings.Contains(got, "**message**") || !strings.Contains(got, "Workspace -> /x") {
		t.Errorf("md map should render key/value markdown: %s", got)
	}
}

func TestIsCommandResult(t *testing.T) {
	if !IsCommandResult(CommandResult{Stdout: "x", ExitCode: 0}) {
		t.Error("CommandResult should be detected")
	}
	if IsCommandResult(map[string]any{"hello": 1}) {
		t.Error("plain map should not be a command result")
	}
	if !IsCommandResult(map[string]any{"stdout": "x", "exitCode": 1}) {
		t.Error("map with stdout string should be a command result")
	}
	if IsCommandResult(map[string]any{"exitCode": 1}) {
		t.Error("map without stdout should not be a command result")
	}
	if IsCommandResult(CommandResult{Stdout: ""}) {
		t.Error("empty stdout CommandResult should not count")
	}
}

func TestToolContext(t *testing.T) {
	ctx := WithToolContext(context.Background(), ToolContext{ToolName: "browser_navigate", Params: map[string]any{"action": "navigate"}})
	tc, ok := ToolContextFrom(ctx)
	if !ok || tc.ToolName != "browser_navigate" {
		t.Errorf("tool context not carried: %+v, %v", tc, ok)
	}
	if action := ToolActionFromContext(ctx); action != "navigate" {
		t.Errorf("action = %q", action)
	}
	if _, ok := ToolContextFrom(context.Background()); ok {
		t.Error("empty context should yield no tool context")
	}
}

func TestTimedOutKind(t *testing.T) {
	cases := []struct {
		name string
		data any
		want string
	}{
		{"map hard", map[string]any{"stdout": "x", "timedOut": "hard"}, "hard"},
		{"map activity", map[string]any{"stdout": "x", "timedOut": "activity"}, "activity"},
		{"map empty", map[string]any{"stdout": "x", "timedOut": ""}, ""},
		{"map nil timeout", map[string]any{"stdout": "x", "timedOut": nil}, ""},
		{"plain map", map[string]any{"a": 1}, ""},
		{"struct hard", CommandResult{Stdout: "x", TimedOut: "hard"}, "hard"},
		{"struct ptr activity", &CommandResult{Stdout: "x", TimedOut: "activity"}, "activity"},
		{"struct no timeout", CommandResult{Stdout: "x"}, ""},
		{"string payload", "hello", ""},
	}
	for _, c := range cases {
		if got := timedOutKind(c.data); got != c.want {
			t.Errorf("%s: timedOutKind = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWrapSuccessLogsExecTimeout(t *testing.T) {
	setupMCPConfig(t)
	var got []string
	SetTimeoutObserver(func(tool, kind string, _ int64) {
		got = append(got, tool+":"+kind)
	})
	t.Cleanup(func() { SetTimeoutObserver(nil) })

	ctx := WithToolContext(context.Background(), ToolContext{ToolName: "shell", Params: map[string]any{"action": "long.task"}})
	start := time.Now().Add(-3 * time.Minute)
	res := WrapSuccess(ctx, "shell", map[string]any{
		"command":  "long task",
		"stdout":   "partial",
		"stderr":   "",
		"exitCode": 0,
		"aborted":  true,
		"timedOut": "hard",
		"timeout":  nil,
	}, start)

	if len(got) != 1 || got[0] != "shell:hard" {
		t.Fatalf("timeout observer = %v, want [shell:hard]", got)
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(res.Content))
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "hard ceiling") {
		t.Errorf("timeout annotation missing from response: %q", text)
	}
}

func TestWrapSuccessNoTimeoutNoObserver(t *testing.T) {
	setupMCPConfig(t)
	fired := false
	SetTimeoutObserver(func(_, _ string, _ int64) { fired = true })
	t.Cleanup(func() { SetTimeoutObserver(nil) })

	ctx := WithToolContext(context.Background(), ToolContext{ToolName: "shell", Params: map[string]any{}})
	res := WrapSuccess(ctx, "shell", map[string]any{
		"command":  "ls",
		"stdout":   "a\nb",
		"stderr":   "",
		"exitCode": 0,
		"timedOut": nil,
	}, time.Now())

	if fired {
		t.Fatal("timeout observer fired for a non-timed-out result")
	}
	if len(res.Content) != 1 || res.Content[0].(mcp.TextContent).Text == "" {
		t.Fatalf("successful result should be rendered unchanged: %+v", res.Content)
	}
}

func TestOrphanFlagSemantics(t *testing.T) {
	flag := new(atomic.Bool)
	ctx := MarkWithOrphanFlag(context.Background(), flag)
	if isOrphaned(ctx) {
		t.Error("fresh orphan flag should be false")
	}
	flag.Store(true)
	if !isOrphaned(ctx) {
		t.Error("flag should reflect the stored value")
	}
	if isOrphaned(context.Background()) {
		t.Error("plain context must not be orphaned")
	}
}

func TestWrapSuccessOrphanedSuppressesTimeoutEvent(t *testing.T) {
	setupMCPConfig(t)
	fired := false
	SetTimeoutObserver(func(_, _ string, _ int64) { fired = true })
	t.Cleanup(func() { SetTimeoutObserver(nil) })

	flag := new(atomic.Bool)
	flag.Store(true)
	ctx := WithToolContext(MarkWithOrphanFlag(context.Background(), flag),
		ToolContext{ToolName: "shell", Params: map[string]any{"action": "run"}})

	res := WrapSuccess(ctx, "shell", map[string]any{
		"command":  "long task",
		"stdout":   "partial",
		"stderr":   "",
		"exitCode": 0,
		"timedOut": "hard",
		"timeout":  nil,
	}, time.Now())

	if fired {
		t.Fatal("timeout observer fired for an orphaned (already-abandoned) result")
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected rendered content, got %d items", len(res.Content))
	}
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "hard ceiling") {
		t.Errorf("orphaned result should still render the timeout annotation")
	}
}

func TestWrapSuccessOrphanedSuppressesSuccessEvent(t *testing.T) {
	setupMCPConfig(t)
	fired := false
	SetTimeoutObserver(func(_, _ string, _ int64) { fired = true })
	t.Cleanup(func() { SetTimeoutObserver(nil) })

	flag := new(atomic.Bool)
	flag.Store(true)
	ctx := WithToolContext(MarkWithOrphanFlag(context.Background(), flag),
		ToolContext{ToolName: "shell", Params: map[string]any{}})
	res := WrapSuccess(ctx, "shell", "rendered", time.Now())
	if fired {
		t.Fatal("timeout observer must not fire on a plain orphaned success")
	}
	if len(res.Content) != 1 || res.Content[0].(mcp.TextContent).Text != "rendered" {
		t.Fatalf("orphaned success should still render: %+v", res.Content)
	}
}

func setupMCPConfig(t *testing.T) {
	t.Helper()
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir()
	t.Cleanup(func() {
		mcpcfg.ProjectRoot = old
	})
	if err := mcpcfg.Load(); err != nil {
		t.Fatal(err)
	}
}

func setupMCPConfigWithBody(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = dir
	t.Cleanup(func() {
		mcpcfg.ProjectRoot = old
	})
	if err := mcpcfg.Load(); err != nil {
		t.Fatal(err)
	}
}

func TestWrapSuccess(t *testing.T) {
	setupMCPConfig(t)
	ctx := WithToolContext(context.Background(), ToolContext{ToolName: "raw-capture", Params: map[string]any{"action": "screenshot"}})
	res := WrapSuccess(ctx, "raw-capture", "screenshot data", time.Now())
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(res.Content))
	}
	if res.Content[0].(mcp.TextContent).Text != "screenshot data" {
		t.Errorf("wrap text = %q", res.Content[0].(mcp.TextContent).Text)
	}
}

func TestWrapError(t *testing.T) {
	setupMCPConfig(t)
	ctx := WithToolContext(context.Background(), ToolContext{ToolName: "browser", Params: map[string]any{"action": "navigate"}})
	res := WrapErrorWithContext(ctx, "browser", errors.New("boom failure"), time.Now())
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "❌ browser failed") {
		t.Errorf("error header missing: %s", text)
	}
	if !strings.Contains(text, "Action: navigate") {
		t.Errorf("action line missing: %s", text)
	}
	if !strings.Contains(text, "Error: boom failure") {
		t.Errorf("error message missing: %s", text)
	}
	if !strings.Contains(text, "suggestion") && !strings.Contains(text, "Tool: browser") {
		t.Errorf("suggestion missing: %s", text)
	}
	if res.IsError {
		t.Error("IsError should stay false (matches TS)")
	}
}

func TestWrapErrorSuggestionsDisabledIsBarebone(t *testing.T) {
	setupMCPConfigWithBody(t, `{"tool_suggestions_enabled":false}`)
	ctx := WithToolContext(context.Background(), ToolContext{ToolName: "browser", Params: map[string]any{"action": "navigate"}})
	res := WrapErrorWithContext(ctx, "browser", errors.New("boom failure"), time.Now())
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "❌ browser failed") {
		t.Errorf("error header missing: %s", text)
	}
	if !strings.Contains(text, "Action: navigate") {
		t.Errorf("action line missing: %s", text)
	}
	if !strings.Contains(text, "Error: boom failure") {
		t.Errorf("error message missing: %s", text)
	}
	if strings.Contains(text, "📌 **Tool:") || strings.Contains(text, "Example Usage") || strings.Contains(text, "Parameters for action") {
		t.Errorf("suggestion block should be suppressed when disabled: %s", text)
	}
}

func TestWrapErrorFormatMd(t *testing.T) {
	setupMCPConfigWithBody(t, `{"toolConfigs":{"browser":{"timeout":60000,"format":"md"}}}`)
	ctx := WithToolContext(context.Background(), ToolContext{ToolName: "browser", Params: map[string]any{"action": "navigate"}})
	res := WrapErrorWithContext(ctx, "browser", errors.New("boom failure"), time.Now())
	text := res.Content[0].(mcp.TextContent).Text
	if strings.Contains(text, "```json") {
		t.Errorf("md error should not be wrapped as json fence: %s", text)
	}
	if !strings.Contains(text, "boom failure") {
		t.Errorf("md error should contain message: %s", text)
	}
}

func TestWrapErrorFormatJSON(t *testing.T) {
	setupMCPConfigWithBody(t, `{"toolConfigs":{"browser":{"timeout":60000,"format":"json"}}}`)
	ctx := WithToolContext(context.Background(), ToolContext{ToolName: "browser", Params: map[string]any{"action": "navigate"}})
	res := WrapErrorWithContext(ctx, "browser", errors.New("boom failure"), time.Now())
	text := res.Content[0].(mcp.TextContent).Text
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("json error format should be valid json: %v (%s)", err, text)
	}
	if m["message"] != "boom failure" {
		t.Errorf("json error message wrong: %v", m)
	}
	if m["tool"] != "browser" {
		t.Errorf("json error tool wrong: %v", m)
	}
}
