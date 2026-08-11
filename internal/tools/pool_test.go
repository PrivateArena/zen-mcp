package tools

import (
	"context"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/pooling"
)

func withPoolingConfigForPoolTest(t *testing.T, cfg mcpcfg.PoolingConfig) {
	t.Helper()
	old := mcpcfg.Get()
	newCfg := *old
	newCfg.Pooling = cfg
	mcpcfg.Config.Store(&newCfg)
	t.Cleanup(func() { mcpcfg.Config.Store(old) })
}

func TestPoolDisabledReturnsError(t *testing.T) {
	withPoolingConfigForPoolTest(t, mcpcfg.PoolingConfig{Enabled: false})
	deps := testDepsForPool()
	res := HandlePoolAction(context.Background(), deps, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "pool", Arguments: map[string]any{"action": "poll"}},
	})
	text := res.Content[0].(mcp.TextContent).Text
	if !contains(text, "disabled") {
		t.Errorf("expected disabled error, got: %s", text)
	}
}

func TestPoolPollCompletedJobReplaysVerbatim(t *testing.T) {
	withPoolingConfigForPoolTest(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"shell"},
		ElapsedMs:  5000,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := pooling.NewRegistry(60*time.Minute, 60*time.Minute, 256)
	pooling.SetGlobalRegistryForTest(func() *pooling.Registry { return reg })
	defer pooling.ResetGlobalForTest()

	job := &pooling.Job{Done: make(chan struct{})}
	id, _ := reg.Register(job)
	stored := &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "stored-result"}}}
	reg.Complete(id, stored)

	deps := testDepsForPool()
	res := HandlePoolAction(context.Background(), deps, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "pool", Arguments: map[string]any{"action": "poll", "pool_id": id}},
	})
	text := res.Content[0].(mcp.TextContent).Text
	if text != "stored-result" {
		t.Errorf("poll completed job = %q, want stored-result", text)
	}
}

func TestPoolPollRunningAfterWait(t *testing.T) {
	withPoolingConfigForPoolTest(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"shell"},
		ElapsedMs:  5000,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := pooling.NewRegistry(60*time.Minute, 60*time.Minute, 256)
	pooling.SetGlobalRegistryForTest(func() *pooling.Registry { return reg })
	defer pooling.ResetGlobalForTest()

	job := &pooling.Job{Done: make(chan struct{})}
	id, _ := reg.Register(job)

	deps := testDepsForPool()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	res := HandlePoolAction(ctx, deps, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "pool", Arguments: map[string]any{"action": "poll", "pool_id": id}},
	})
	text := res.Content[0].(mcp.TextContent).Text
	if !contains(text, "running") {
		t.Errorf("expected running after timeout, got: %s", text)
	}
}

func TestPoolPollCancelled(t *testing.T) {
	withPoolingConfigForPoolTest(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"shell"},
		ElapsedMs:  5000,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := pooling.NewRegistry(60*time.Minute, 60*time.Minute, 256)
	pooling.SetGlobalRegistryForTest(func() *pooling.Registry { return reg })
	defer pooling.ResetGlobalForTest()

	job := &pooling.Job{Done: make(chan struct{})}
	id, _ := reg.Register(job)
	reg.Cancel(id)

	deps := testDepsForPool()
	res := HandlePoolAction(context.Background(), deps, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "pool", Arguments: map[string]any{"action": "poll", "pool_id": id}},
	})
	text := res.Content[0].(mcp.TextContent).Text
	if !contains(text, "cancelled") {
		t.Errorf("expected cancelled, got: %s", text)
	}
}

func TestPoolPollUnknownID(t *testing.T) {
	withPoolingConfigForPoolTest(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"shell"},
		ElapsedMs:  5000,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := pooling.NewRegistry(60*time.Minute, 60*time.Minute, 256)
	pooling.SetGlobalRegistryForTest(func() *pooling.Registry { return reg })
	defer pooling.ResetGlobalForTest()

	deps := testDepsForPool()
	res := HandlePoolAction(context.Background(), deps, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "pool", Arguments: map[string]any{"action": "poll", "pool_id": "pool-missing"}},
	})
	text := res.Content[0].(mcp.TextContent).Text
	if !contains(text, "unknown pool_id") {
		t.Errorf("expected unknown pool_id error, got: %s", text)
	}
}

func TestPoolListShape(t *testing.T) {
	withPoolingConfigForPoolTest(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"shell"},
		ElapsedMs:  5000,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := pooling.NewRegistry(60*time.Minute, 60*time.Minute, 256)
	pooling.SetGlobalRegistryForTest(func() *pooling.Registry { return reg })
	defer pooling.ResetGlobalForTest()

	job := &pooling.Job{Done: make(chan struct{})}
	reg.Register(job)

	deps := testDepsForPool()
	res := HandlePoolAction(context.Background(), deps, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "pool", Arguments: map[string]any{"action": "list"}},
	})
	text := res.Content[0].(mcp.TextContent).Text
	if !contains(text, "pool_id") || !contains(text, "status") || !contains(text, "age_ms") {
		t.Errorf("expected list with pool_id/status/age_ms, got: %s", text)
	}
}

func TestPoolCancelAction(t *testing.T) {
	withPoolingConfigForPoolTest(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"shell"},
		ElapsedMs:  5000,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := pooling.NewRegistry(60*time.Minute, 60*time.Minute, 256)
	pooling.SetGlobalRegistryForTest(func() *pooling.Registry { return reg })
	defer pooling.ResetGlobalForTest()

	job := &pooling.Job{Done: make(chan struct{})}
	id, _ := reg.Register(job)

	deps := testDepsForPool()
	res := HandlePoolAction(context.Background(), deps, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "pool", Arguments: map[string]any{"action": "cancel", "pool_id": id}},
	})
	text := res.Content[0].(mcp.TextContent).Text
	if !contains(text, "cancelled") {
		t.Errorf("expected cancelled after cancel action, got: %s", text)
	}
}

func TestPoolStatusAction(t *testing.T) {
	withPoolingConfigForPoolTest(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"shell"},
		ElapsedMs:  5000,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := pooling.NewRegistry(60*time.Minute, 60*time.Minute, 256)
	pooling.SetGlobalRegistryForTest(func() *pooling.Registry { return reg })
	defer pooling.ResetGlobalForTest()

	job := &pooling.Job{Done: make(chan struct{})}
	id, _ := reg.Register(job)

	deps := testDepsForPool()
	res := HandlePoolAction(context.Background(), deps, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "pool", Arguments: map[string]any{"action": "status", "pool_id": id}},
	})
	text := res.Content[0].(mcp.TextContent).Text
	if !contains(text, "running") {
		t.Errorf("expected running status, got: %s", text)
	}
}

func testDepsForPool() Deps {
	return Deps{
		Store:                 nil,
		Reg:                   nil,
		Gatekeeper:            nil,
		PendingCollaborations: NewCollaborationRegistry(),
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
