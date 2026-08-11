package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/pooling"
)

func withPoolingWireConfig(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = dir
	t.Cleanup(func() { mcpcfg.ProjectRoot = old })
	if err := mcpcfg.Load(); err != nil {
		t.Fatal(err)
	}
}

func instantInner(text string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}}}, nil
	}
}

func TestWrapIfPooledEnabledAndListedWraps(t *testing.T) {
	withPoolingWireConfig(t, `{"pooling":{"enabled":true,"tools":["shell"],"elapsedMs":40},"tool_suggestions_enabled":false}`)
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(120 * time.Millisecond)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "wired-done"}}}, nil
	}
	handler := wrapIfPooled("shell", inner)
	res, err := handler(context.Background(), makePatchRequest(map[string]any{"command": "sleep 1"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"status":"running"`) {
		t.Fatalf("expected running payload, got: %s", text)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	id, _ := payload["pool_id"].(string)
	if id == "" {
		t.Fatal("running payload missing pool_id")
	}
	// The job lives in the process-wide registry (same one the pool tool uses).
	out := pooling.Global().LongPoll(context.Background(), id, 2*time.Second)
	if out.State != pooling.StateDone {
		t.Fatalf("poll state = %q, want done", out.State)
	}
	if got := out.Result.Content[0].(mcp.TextContent).Text; got != "wired-done" {
		t.Errorf("polled result = %q", got)
	}
}

func TestWrapIfPooledDisabledPassthrough(t *testing.T) {
	withPoolingWireConfig(t, `{"pooling":{"enabled":false},"tool_suggestions_enabled":false}`)
	handler := wrapIfPooled("shell", instantInner("plain"))
	res, err := handler(context.Background(), makePatchRequest(map[string]any{"command": "echo"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := res.Content[0].(mcp.TextContent).Text; got != "plain" {
		t.Errorf("result = %q, want plain (no wrap when disabled)", got)
	}
}

func TestWrapIfPooledLiveEnableAfterRegistration(t *testing.T) {
	// Register the handler while pooling is disabled (a server cached before the
	// user flipped enabled:true). Because the gate is per-call, the same handler
	// must start pooling the moment config enables it — no restart needed.
	withPoolingWireConfig(t, `{"pooling":{"enabled":false},"tool_suggestions_enabled":false}`)
	handler := wrapIfPooled("shell", func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(120 * time.Millisecond)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "toggled-done"}}}, nil
	})

	// Still disabled → blocking, synchronous result.
	res, err := handler(context.Background(), makePatchRequest(map[string]any{"command": "echo"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := res.Content[0].(mcp.TextContent).Text; got != "toggled-done" {
		t.Errorf("disabled result = %q", got)
	}

	// Hot-enable pooling without any restart.
	withPoolingWireConfig(t, `{"pooling":{"enabled":true,"tools":["shell"],"elapsedMs":40},"tool_suggestions_enabled":false}`)
	start := time.Now()
	res, err = handler(context.Background(), makePatchRequest(map[string]any{"command": "echo"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if time.Since(start) >= 120*time.Millisecond {
		t.Errorf("after enable the call must convert early, took %v", time.Since(start))
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"status":"running"`) {
		t.Fatalf("expected running payload after live enable, got: %s", text)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	id, _ := payload["pool_id"].(string)
	out := pooling.Global().LongPoll(context.Background(), id, 2*time.Second)
	if out.State != pooling.StateDone || out.Result == nil {
		t.Fatalf("poll state = %q", out.State)
	}
	if got := out.Result.Content[0].(mcp.TextContent).Text; got != "toggled-done" {
		t.Errorf("polled result = %q", got)
	}
}

func TestWrapIfPooledNotListedPassthrough(t *testing.T) {
	withPoolingWireConfig(t, `{"pooling":{"enabled":true,"tools":["shell"]},"tool_suggestions_enabled":false}`)
	handler := wrapIfPooled("browser", instantInner("unwrapped"))
	res, err := handler(context.Background(), makePatchRequest(map[string]any{"action": "navigate"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := res.Content[0].(mcp.TextContent).Text; got != "unwrapped" {
		t.Errorf("result = %q, want unwrapped", got)
	}
}

func TestWrapIfPooledPoolToolNeverWrapped(t *testing.T) {
	// The pool tool is never wrapped even when misconfigured into the list: a
	// wrapped pool poll would spawn a job and a second pool_id.
	withPoolingWireConfig(t, `{"pooling":{"enabled":true,"tools":["pool"]},"tool_suggestions_enabled":false}`)
	handler := wrapIfPooled("pool", instantInner("pool-handler-body"))
	res, err := handler(context.Background(), makePatchRequest(map[string]any{"action": "list"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := res.Content[0].(mcp.TextContent).Text; got != "pool-handler-body" {
		t.Errorf("pool tool must run unwrapped, got %q", got)
	}
}
