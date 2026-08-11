package pooling

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	"zen-mcp/internal/mcpcfg"
)

func TestPoolToolReturnsShellResultNotPoolID(t *testing.T) {
	mcpcfg.Config.Store(&mcpcfg.ZenConfig{
		Pooling: mcpcfg.PoolingConfig{
			Enabled:    true,
			Tools:      []string{"shell"},
			ElapsedMs:  1000,
			TTLMinutes: 10,
			MaxJobs:    32,
		},
	})
	defer mcpcfg.Config.Store(mcpcfg.Get())

	reg := NewRegistry(10*time.Minute, 10*time.Minute, 32)
	SetGlobalRegistryForTest(func() *Registry { return reg })
	defer ResetGlobalForTest()

	shellInner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(2 * time.Second)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "echo-DONE"}}}, nil
	}
	shellWrapped := WrapWithRegistry("shell", shellInner, func() *Registry { return reg })

	res, _ := shellWrapped(context.Background(), mcp.CallToolRequest{})
	shellText := res.Content[0].(mcp.TextContent).Text
	t.Logf("shell response: %s", shellText)

	var poolID string
	var payload map[string]any
	if err := json.Unmarshal([]byte(shellText), &payload); err == nil {
		poolID, _ = payload["pool_id"].(string)
	}
	if poolID == "" {
		t.Fatal("could not extract pool_id from shell response")
	}
	t.Logf("extracted pool_id: %s", poolID)

	time.Sleep(3 * time.Second)

	poolInner := func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		poolIDArg, _ := req.GetArguments()["pool_id"].(string)
		if poolIDArg == "" {
			return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "error: pool_id required"}}}, nil
		}
		outcome := reg.LongPoll(context.Background(), poolIDArg, 5*time.Second)
		switch outcome.State {
		case "done":
			return outcome.Result, nil
		case "running":
			return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: `{"status":"running","pool_id":"` + poolIDArg + `"}`}}}, nil
		default:
			return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "error: unknown pool_id"}}}, nil
		}
	}
	poolWrapped := WrapWithRegistry("pool", poolInner, func() *Registry { return reg })

	res, _ = poolWrapped(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"action": "poll", "pool_id": poolID},
		},
	})
	poolText := res.Content[0].(mcp.TextContent).Text
	t.Logf("pool poll response: %s", poolText)

	if poolText != "echo-DONE" {
		t.Errorf("pool poll should return shell result 'echo-DONE', got: %s", poolText)
	}
}
