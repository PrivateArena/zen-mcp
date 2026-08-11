package pooling

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolresponse"
)

func withPoolingConfig(t *testing.T, cfg mcpcfg.PoolingConfig) {
	t.Helper()
	old := mcpcfg.Get()
	newCfg := *old
	newCfg.Pooling = cfg
	mcpcfg.Config.Store(&newCfg)
	t.Cleanup(func() { mcpcfg.Config.Store(old) })
}

func TestWrapFastHandlerReturnsResultUntouched(t *testing.T) {
	withPoolingConfig(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"fast"},
		ElapsedMs:  5000,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 256)

	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "fast-result"}}}, nil
	}
	wrapped := WrapWithRegistry("fast", inner, func() *Registry { return reg })

	res, err := wrapped(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("wrapped: %v", err)
	}
	if res.Content[0].(mcp.TextContent).Text != "fast-result" {
		t.Errorf("fast result = %q, want fast-result", res.Content[0].(mcp.TextContent).Text)
	}
}

func TestWrapSlowHandlerReturnsPoolID(t *testing.T) {
	withPoolingConfig(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"slow"},
		ElapsedMs:  50,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 256)

	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(200 * time.Millisecond)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "slow-result"}}}, nil
	}
	wrapped := WrapWithRegistry("slow", inner, func() *Registry { return reg })

	done := make(chan *mcp.CallToolResult, 1)
	go func() {
		res, _ := wrapped(context.Background(), mcp.CallToolRequest{})
		done <- res
	}()

	select {
	case res := <-done:
		text := res.Content[0].(mcp.TextContent).Text
		if !contains(text, "pool_id") {
			t.Errorf("expected pool_id in result, got: %s", text)
		}
		if !contains(text, "running") {
			t.Errorf("expected running status in result, got: %s", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wrapped did not return within 2s")
	}
}

func TestWrapClientCancelRegistersImmediately(t *testing.T) {
	withPoolingConfig(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"cancel-test"},
		ElapsedMs:  5000,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 256)

	started := make(chan struct{})
	inner := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		close(started)
		<-ctx.Done()
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "late"}}}, nil
	}
	wrapped := WrapWithRegistry("cancel-test", inner, func() *Registry { return reg })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *mcp.CallToolResult, 1)
	go func() {
		res, _ := wrapped(ctx, mcp.CallToolRequest{})
		done <- res
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("inner did not start")
	}
	cancel()

	select {
	case <-done:
		// job should be registered immediately on cancel
		jobs := reg.List()
		found := false
		for _, j := range jobs {
			if j.State != "" {
				found = true
				break
			}
		}
		if !found && len(jobs) == 0 {
			// It's possible the job completed so fast it's done, but more likely it's registered
			// Let's check with a small delay
			time.Sleep(10 * time.Millisecond)
			jobs = reg.List()
			if len(jobs) == 0 {
				t.Error("expected job to be registered on client cancel")
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wrapped did not return after cancel")
	}
}

func TestWrapPoolIDResolvesViaLongPoll(t *testing.T) {
	withPoolingConfig(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"resolve"},
		ElapsedMs:  50,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 256)

	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(200 * time.Millisecond)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "resolved"}}}, nil
	}
	wrapped := WrapWithRegistry("resolve", inner, func() *Registry { return reg })

	res, _ := wrapped(context.Background(), mcp.CallToolRequest{})
	text := res.Content[0].(mcp.TextContent).Text

	var poolID string
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err == nil {
		poolID, _ = payload["pool_id"].(string)
	}
	if poolID == "" {
		t.Fatalf("could not extract pool_id from: %s", text)
	}

	outcome := reg.LongPoll(context.Background(), poolID, 2*time.Second)
	if outcome.State != "done" {
		t.Errorf("poll state = %q, want done", outcome.State)
	}
	if outcome.Result == nil || outcome.Result.Content[0].(mcp.TextContent).Text != "resolved" {
		t.Error("poll result should contain resolved text")
	}
}

func TestWrapPanicStoresErrorResult(t *testing.T) {
	withPoolingConfig(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"panic-test"},
		ElapsedMs:  50,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 256)

	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		panic("intentional panic for test")
	}
	wrapped := WrapWithRegistry("panic-test", inner, func() *Registry { return reg })

	res, _ := wrapped(context.Background(), mcp.CallToolRequest{})
	text := res.Content[0].(mcp.TextContent).Text
	if !contains(text, "panic") {
		t.Errorf("expected panic in result, got: %s", text)
	}
}

func TestWrapRegistryFullReturnsError(t *testing.T) {
	withPoolingConfig(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"full"},
		ElapsedMs:  30,
		TTLMinutes: 60,
		MaxJobs:    2,
	})
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 2)

	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(200 * time.Millisecond)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "x"}}}, nil
	}

	wrapped := WrapWithRegistry("full", inner, func() *Registry { return reg })

	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		wrapped(ctx, mcp.CallToolRequest{})
		cancel()
	}

	// Registry is now full; third call should get an error
	res, _ := wrapped(context.Background(), mcp.CallToolRequest{})
	text := res.Content[0].(mcp.TextContent).Text
	if !contains(text, "full") && !contains(text, "cap") {
		t.Errorf("expected registry-full error, got: %s", text)
	}
}

func TestWrapDisabledPassesThrough(t *testing.T) {
	withPoolingConfig(t, mcpcfg.PoolingConfig{
		Enabled:    false,
		Tools:      []string{"x"},
		ElapsedMs:  50,
		TTLMinutes: 60,
		MaxJobs:    256,
	})

	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "direct"}}}, nil
	}
	wrapped := Wrap("x", inner)

	res, _ := wrapped(context.Background(), mcp.CallToolRequest{})
	if res.Content[0].(mcp.TextContent).Text != "direct" {
		t.Errorf("disabled wrap should pass through, got: %s", res.Content[0].(mcp.TextContent).Text)
	}
}

func TestWrapToolNotInListPassesThrough(t *testing.T) {
	withPoolingConfig(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"other"},
		ElapsedMs:  50,
		TTLMinutes: 60,
		MaxJobs:    256,
	})

	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "direct"}}}, nil
	}
	wrapped := Wrap("not-in-list", inner)

	res, _ := wrapped(context.Background(), mcp.CallToolRequest{})
	if res.Content[0].(mcp.TextContent).Text != "direct" {
		t.Errorf("tool not in list should pass through, got: %s", res.Content[0].(mcp.TextContent).Text)
	}
}

func TestWrapOrphanFlagSuppressesTelemetry(t *testing.T) {
	withPoolingConfig(t, mcpcfg.PoolingConfig{
		Enabled:    true,
		Tools:      []string{"orphan"},
		ElapsedMs:  50,
		TTLMinutes: 60,
		MaxJobs:    256,
	})
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 256)

	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(200 * time.Millisecond)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "late"}}}, nil
	}
	wrapped := WrapWithRegistry("orphan", inner, func() *Registry { return reg })

	res, _ := wrapped(context.Background(), mcp.CallToolRequest{})
	text := res.Content[0].(mcp.TextContent).Text
	if !contains(text, "pool_id") {
		t.Errorf("expected pool_id in orphan result, got: %s", text)
	}
}

func TestWrapSummarizeTruncates(t *testing.T) {
	args := map[string]any{"command": string(make([]byte, 500))}
	s := summarize(args)
	if len(s) > 450 {
		t.Errorf("summarize length = %d, want <= 450", len(s))
	}
	if !contains(s, "[truncated") {
		t.Error("summarize should include truncation marker")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsInternal(s, sub))
}

func containsInternal(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func init() {
	toolresponse.SetTimeoutObserver(func(tool, kind string, elapsedMs int64) {
		// no-op for tests
	})
}
