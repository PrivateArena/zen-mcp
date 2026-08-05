package toolregistry

import (
	"context"
	"testing"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func handler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ok"}}}, nil
}

func TestTrackAndGet(t *testing.T) {
	r := Create()
	r.Track(ToolRegistration{Name: "a", DefaultEnabled: true, Description: "desc a", Handler: handler})
	entry, ok := r.GetTool("a")
	if !ok {
		t.Fatal("tool a not found")
	}
	if entry.Name != "a" || !entry.Enabled || entry.Handler == nil {
		t.Errorf("registration wrong: %+v", entry)
	}
	if !r.IsToolEnabled("a") {
		t.Error("default-enabled tool should be enabled")
	}
	if _, ok := r.GetTool("missing"); ok {
		t.Error("missing tool should not be found")
	}
	if r.IsToolEnabled("missing") {
		t.Error("missing tool should not be enabled")
	}
}

func TestDefaultDisabled(t *testing.T) {
	r := Create()
	r.Track(ToolRegistration{Name: "b"})
	if r.IsToolEnabled("b") {
		t.Error("tool without DefaultEnabled should start disabled")
	}
}

func TestListTools(t *testing.T) {
	r := Create()
	r.Track(ToolRegistration{Name: "a"})
	r.Track(ToolRegistration{Name: "b"})
	if got := r.ListToolNames(); len(got) != 2 {
		t.Errorf("ListToolNames = %v", got)
	}
	if got := r.ListTools(); len(got) != 2 {
		t.Errorf("ListTools = %d", len(got))
	}
}

func TestSetToolEnabled(t *testing.T) {
	r := Create()
	r.Track(ToolRegistration{Name: "a"})
	if !r.SetToolEnabled("a", true) {
		t.Error("enabling existing tool should succeed")
	}
	if !r.IsToolEnabled("a") {
		t.Error("tool should be enabled")
	}
	if r.SetToolEnabled("missing", true) {
		t.Error("enabling missing tool should fail")
	}
}

func TestReset(t *testing.T) {
	r := Create()
	r.Track(ToolRegistration{Name: "a"})
	r.Reset()
	if _, ok := r.GetTool("a"); ok {
		t.Error("Reset should clear tools")
	}
}
