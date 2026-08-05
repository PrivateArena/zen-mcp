package toolresponse

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/jang/zen-mcp/internal/mcpcfg"
)

func intPtr(v int) *int { return &v }

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
