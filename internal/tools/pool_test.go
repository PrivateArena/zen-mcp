package tools

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

const poolToolConfig = `{
  "pooling": {"enabled": true, "tools": ["shell"], "elapsedMs": 40, "ttlMinutes": 5, "maxJobs": 8},
  "tool_suggestions_enabled": false
}`

func withPoolToolConfig(t *testing.T, body string) {
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

func poolRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "pool",
			Arguments: args,
		},
	}
}

func poolPayload(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if res == nil || len(res.Content) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text.Text), &m); err != nil {
		return nil
	}
	return m
}

func TestPoolDisabledReturnsError(t *testing.T) {
	withPoolToolConfig(t, `{"pooling":{"enabled":false},"tool_suggestions_enabled":false}`)
	reg := pooling.NewRegistry(time.Minute, time.Minute, 4)
	res := HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "list"}))
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "pooling is disabled") {
		t.Errorf("expected disabled error, got: %s", res.Content[0].(mcp.TextContent).Text)
	}
}

func TestPoolPollReplaysStoredResultVerbatim(t *testing.T) {
	withPoolToolConfig(t, poolToolConfig)
	reg := pooling.NewRegistry(time.Minute, time.Minute, 4)
	stored := &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "original-job-output"}}}
	id, err := 	reg.Register("shell", &pooling.Job{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	reg.Complete(id, stored)
	res := HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "poll", "pool_id": id}))
	if res != stored {
		t.Error("poll on a done job must return the stored *CallToolResult verbatim (same pointer), no re-wrap")
	}
}

func TestPoolPollRunningReturnsSamePoolID(t *testing.T) {
	withPoolToolConfig(t, poolToolConfig)
	reg := pooling.NewRegistry(time.Minute, time.Minute, 4)
	id, err := 	reg.Register("shell", &pooling.Job{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	start := time.Now()
	res := HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "poll", "pool_id": id}))
	if time.Since(start) < 40*time.Millisecond {
		t.Error("poll must block for at least the elapsed window")
	}
	m := poolPayload(t, res)
	if m["status"] != "running" {
		t.Errorf("status = %v, want running", m["status"])
	}
	if m["pool_id"] != id {
		t.Errorf("polled pool_id = %v, want the SAME id %q (never a new one)", m["pool_id"], id)
	}
	if em, ok := m["elapsedMs"].(float64); !ok || em < 40 {
		t.Errorf("running payload must carry elapsedMs >= 40ms, got %v", m["elapsedMs"])
	}
}

func TestPoolPollCancelled(t *testing.T) {
	withPoolToolConfig(t, poolToolConfig)
	reg := pooling.NewRegistry(time.Minute, time.Minute, 4)
	id, _ := 	reg.Register("shell", &pooling.Job{})
	reg.Cancel(id)
	res := HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "poll", "pool_id": id}))
	m := poolPayload(t, res)
	if m["status"] != "cancelled" || m["pool_id"] != id {
		t.Errorf("cancelled payload wrong: %v", m)
	}
}

func TestPoolPollUnknownNeverCreatesJob(t *testing.T) {
	withPoolToolConfig(t, poolToolConfig)
	reg := pooling.NewRegistry(time.Minute, time.Minute, 4)
	res := HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "poll", "pool_id": "shell-nonexistent"}))
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "unknown pool_id \"shell-nonexistent\"") || !strings.Contains(text, "re-issue the original call") {
		t.Errorf("expected actionable unknown-id error, got: %s", text)
	}
	if n := len(reg.List()); n != 0 {
		t.Errorf("poll of unknown id must NOT create a job, registry has %d", n)
	}
}

func TestPoolPollMissingIDExplicitError(t *testing.T) {
	withPoolToolConfig(t, poolToolConfig)
	reg := pooling.NewRegistry(time.Minute, time.Minute, 4)
	res := HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "poll"}))
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "pool_id is required") {
		t.Errorf("expected explicit pool_id-required error, got: %s", res.Content[0].(mcp.TextContent).Text)
	}
	if n := len(reg.List()); n != 0 {
		t.Errorf("empty pool_id must not create a job, registry has %d", n)
	}
}

func TestPoolStatus(t *testing.T) {
	withPoolToolConfig(t, poolToolConfig)
	reg := pooling.NewRegistry(time.Minute, time.Minute, 4)
	id, _ := 	reg.Register("shell", &pooling.Job{})
	if m := poolPayload(t, HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "status", "pool_id": id}))); m["status"] != "running" {
		t.Errorf("running status wrong: %v", m)
	} else if em, ok := m["elapsedMs"].(float64); !ok || em < 0 {
		t.Errorf("status payload missing elapsedMs: %v", m)
	}
	reg.Complete(id, &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "x"}}})
	if m := poolPayload(t, HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "status", "pool_id": id}))); m["status"] != "done" {
		t.Errorf("done status wrong: %v", m)
	}
	reg.Cancel(id)
	if m := poolPayload(t, HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "status", "pool_id": id}))); m["status"] != "cancelled" {
		t.Errorf("cancelled status wrong: %v", m)
	}
	res := HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "status", "pool_id": "shell-missing"}))
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "unknown pool_id") {
		t.Errorf("unknown status should error, got: %s", res.Content[0].(mcp.TextContent).Text)
	}
}

func TestPoolCancel(t *testing.T) {
	withPoolToolConfig(t, poolToolConfig)
	reg := pooling.NewRegistry(time.Minute, time.Minute, 4)
	id, _ := 	reg.Register("shell", &pooling.Job{})
	m := poolPayload(t, HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "cancel", "pool_id": id})))
	if m["status"] != "cancelled" || m["pool_id"] != id {
		t.Errorf("cancel payload wrong: %v", m)
	}
	if reg.State(id) != pooling.StateCancelled {
		t.Error("registry should mark job cancelled")
	}
	res := HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "cancel", "pool_id": "shell-missing"}))
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "unknown pool_id") {
		t.Errorf("cancel unknown should error, got: %s", res.Content[0].(mcp.TextContent).Text)
	}
}

func TestPoolListShape(t *testing.T) {
	withPoolToolConfig(t, poolToolConfig)
	reg := pooling.NewRegistry(time.Minute, time.Minute, 4)
	id1, _ := 	reg.Register("shell", &pooling.Job{})
	id2, _ := 	reg.Register("shell", &pooling.Job{})
	reg.Complete(id2, &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "x"}}})
	res := HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "list"}))
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, id1) || !strings.Contains(text, id2) {
		t.Errorf("list missing jobs: %s", text)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(text), &m)
	jobs, ok := m["jobs"].([]any)
	if !ok || len(jobs) != 2 {
		t.Errorf("jobs array shape wrong: %v", m)
	}
}

func TestPoolUnknownAction(t *testing.T) {
	withPoolToolConfig(t, poolToolConfig)
	reg := pooling.NewRegistry(time.Minute, time.Minute, 4)
	res := HandlePoolAction(context.Background(), reg, poolRequest(map[string]any{"action": "explode"}))
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "unknown pool action") {
		t.Errorf("expected unknown-action error, got: %s", res.Content[0].(mcp.TextContent).Text)
	}
}
