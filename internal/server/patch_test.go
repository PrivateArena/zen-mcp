package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
)

func makePatchRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "browser",
			Arguments: args,
		},
	}
}

func withSuggestionConfig(t *testing.T, enabled bool) {
	t.Helper()
	dir := t.TempDir()
	body := "{}"
	if !enabled {
		body = `{"tool_suggestions_enabled":false}`
	}
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

func TestWrapHandlerValidationInterceptsWhenEnabled(t *testing.T) {
	withSuggestionConfig(t, true)
	called := false
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ran"}},
		}, nil
	}
	wrapped := WrapHandlerWithTimeout("browser", inner, func(string) time.Duration { return time.Minute })
	res, err := wrapped(context.Background(), makePatchRequest(map[string]any{"action": "navigate"}))
	if err != nil {
		t.Fatalf("wrapped: %v", err)
	}
	if called {
		t.Error("inner handler should not run when validation intercepts")
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Missing required parameter") {
		t.Errorf("expected missing-parameter error, got: %s", text)
	}
}

func TestWrapHandlerSkipsValidationWhenDisabled(t *testing.T) {
	withSuggestionConfig(t, false)
	called := false
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ran"}},
		}, nil
	}
	wrapped := WrapHandlerWithTimeout("browser", inner, func(string) time.Duration { return time.Minute })
	res, err := wrapped(context.Background(), makePatchRequest(map[string]any{"action": "navigate"}))
	if err != nil {
		t.Fatalf("wrapped: %v", err)
	}
	if !called {
		t.Fatal("inner handler should run when suggestions are disabled")
	}
	if got := res.Content[0].(mcp.TextContent).Text; got != "ran" {
		t.Errorf("expected inner result text, got %q", got)
	}
}

func TestWrapHandlerValidationPassesWithValidParams(t *testing.T) {
	withSuggestionConfig(t, true)
	called := false
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ran"}},
		}, nil
	}
	wrapped := WrapHandlerWithTimeout("browser", inner, func(string) time.Duration { return time.Minute })
	_, err := wrapped(context.Background(), makePatchRequest(map[string]any{"action": "navigate", "url": "https://example.com"}))
	if err != nil {
		t.Fatalf("wrapped: %v", err)
	}
	if !called {
		t.Error("valid call should reach inner handler")
	}
}
