package pooling

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
)

const poolEnabledConfig = `{
  "pooling": {"enabled": true, "tools": ["shell", "browser", "run", "ui-vision"], "elapsedMs": 50, "ttlMinutes": 5, "maxJobs": 8},
  "tool_suggestions_enabled": false
}`

func withPoolingConfig(t *testing.T, body string) {
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

func makeCall(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "shell",
			Arguments: args,
		},
	}
}

func extractRunningPayload(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if res == nil || len(res.Content) != 1 {
		t.Fatalf("expected single content item, got %+v", res)
	}
	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("running payload is not JSON: %q: %v", text.Text, err)
	}
	return payload
}

func assertResultText(t *testing.T, res *mcp.CallToolResult, want string) {
	t.Helper()
	if res == nil || len(res.Content) != 1 {
		t.Fatalf("result content wrong: %+v", res)
	}
	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	if text.Text != want {
		t.Errorf("result text = %q, want %q", text.Text, want)
	}
}

func TestWrapFastReturnsResultUntouched(t *testing.T) {
	withPoolingConfig(t, poolEnabledConfig)
	reg := NewRegistry(time.Minute, time.Minute, 4)
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return textResult("instant"), nil
	}
	res, err := Wrap("shell", reg, inner)(context.Background(), makeCall(map[string]any{"command": "echo hi"}))
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	assertResultText(t, res, "instant")
	if n := len(reg.List()); n != 0 {
		t.Errorf("fast path must not register a job, got %d", n)
	}
}

func TestWrapDisabledPassesThrough(t *testing.T) {
	withPoolingConfig(t, `{"pooling":{"enabled":false},"tool_suggestions_enabled":false}`)
	reg := NewRegistry(time.Minute, time.Minute, 4)
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return textResult("plain"), nil
	}
	res, err := Wrap("shell", reg, inner)(context.Background(), makeCall(map[string]any{"command": "echo hi"}))
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	assertResultText(t, res, "plain")
	if n := len(reg.List()); n != 0 {
		t.Errorf("disabled pooling must not register jobs, got %d", n)
	}
}

func TestWrapNotInToolsListPassesThrough(t *testing.T) {
	withPoolingConfig(t, poolEnabledConfig) // tools list = shell, browser, ...
	reg := NewRegistry(time.Minute, time.Minute, 4)
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return textResult("not-pooled"), nil
	}
	res, err := Wrap("capture", reg, inner)(context.Background(), makeCall(nil))
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	assertResultText(t, res, "not-pooled")
}

func TestWrapPoolToolNeverWrapped(t *testing.T) {
	// Even if a misconfigured config lists "pool", the pool tool must never be
	// wrapped: a wrapped pool poll would itself spawn a job and a SECOND
	// pool_id.
	withPoolingConfig(t, `{"pooling":{"enabled":true,"tools":["pool"],"elapsedMs":50},"tool_suggestions_enabled":false}`)
	reg := NewRegistry(time.Minute, time.Minute, 4)
	called := false
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return textResult("pool-body"), nil
	}
	res, err := Wrap("pool", reg, inner)(context.Background(), makeCall(map[string]any{"action": "poll", "pool_id": "pool-x"}))
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	if !called {
		t.Fatal("pool inner handler must run directly, unwrapped")
	}
	assertResultText(t, res, "pool-body")
	if n := len(reg.List()); n != 0 {
		t.Errorf("pool tool must never spawn jobs, got %d", n)
	}
}

func TestWrapSlowReturnsRunningWithPoolIDAndPolls(t *testing.T) {
	withPoolingConfig(t, poolEnabledConfig)
	reg := NewRegistry(time.Minute, time.Minute, 4)
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(150 * time.Millisecond)
		return textResult("long-finished"), nil
	}
	start := time.Now()
	res, err := Wrap("shell", reg, inner)(context.Background(), makeCall(map[string]any{"command": "sleep 1"}))
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Errorf("wrap returned in %v, must wait out the full elapsed window", elapsed)
	}
	payload := extractRunningPayload(t, res)
	if payload["status"] != "running" {
		t.Errorf("status = %v, want running", payload["status"])
	}
	id, _ := payload["pool_id"].(string)
	if id == "" {
		t.Fatal("running payload missing pool_id")
	}
	// Agents need a progress signal: elapsedMs must be present and roughly the
	// elapsed window (never 0, so the LLM doesn't think the job just started).
	elapsedMs, ok := payload["elapsedMs"].(float64)
	if !ok {
		t.Fatalf("running payload missing elapsedMs: %+v", payload)
	}
	if elapsedMs < 50 {
		t.Errorf("elapsedMs = %v, want >= 50 (conversion window)", elapsedMs)
	}
	if job, ok := reg.Get(id); !ok {
		t.Fatalf("job %q must exist in registry immediately after conversion", id)
	} else if job.ID != id {
		t.Errorf("stored job.ID = %q, want %q", job.ID, id)
	}

	out := reg.LongPoll(context.Background(), id, 2*time.Second)
	if out.State != StateDone {
		t.Fatalf("poll state = %q, want done", out.State)
	}
	assertResultText(t, out.Result, "long-finished")
}

func TestWrapClientAbortRegistersImmediately(t *testing.T) {
	withPoolingConfig(t, `{"pooling":{"enabled":true,"tools":["shell"],"elapsedMs":3600000},"tool_suggestions_enabled":false}`)
	reg := NewRegistry(time.Minute, time.Minute, 4)
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(100 * time.Millisecond)
		return textResult("late"), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	type wrapped struct {
		res *mcp.CallToolResult
		err error
	}
	resultCh := make(chan wrapped, 1)
	go func() {
		res, err := Wrap("shell", reg, inner)(ctx, makeCall(map[string]any{"command": "echo late"}))
		resultCh <- wrapped{res, err}
	}()
	time.Sleep(10 * time.Millisecond) // let goroutine A start
	cancel()

	select {
	case o := <-resultCh:
		if time.Since(start) > time.Second {
			t.Errorf("abort registration took %v, must NOT wait out the 1h window", time.Since(start))
		}
		payload := extractRunningPayload(t, o.res)
		if id, _ := payload["pool_id"].(string); id == "" {
			t.Fatal("abort path must still register and return a pool_id")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wrap did not return on client abort")
	}
	// Job must be registered almost immediately (~ms), not after the elapsed window.
	if n := len(reg.List()); n != 1 {
		t.Errorf("expected 1 registered job after abort, got %d", n)
	}
	// The detached background job still completes and is retrievable.
	id := reg.List()[0].ID
	pollOut := reg.LongPoll(context.Background(), id, 2*time.Second)
	if pollOut.State != StateDone || pollOut.Result == nil {
		t.Fatalf("background job should still complete, got state=%q result=%v", pollOut.State, pollOut.Result)
	}
	assertResultText(t, pollOut.Result, "late")
}

func TestWrapSinglePoolIDLifecycle(t *testing.T) {
	withPoolingConfig(t, poolEnabledConfig)
	reg := NewRegistry(time.Minute, time.Minute, 4)
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(150 * time.Millisecond)
		return textResult("final-result"), nil
	}
	res, err := Wrap("shell", reg, inner)(context.Background(), makeCall(map[string]any{"command": "echo x"}))
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	payload := extractRunningPayload(t, res)
	id, _ := payload["pool_id"].(string)
	if id == "" {
		t.Fatal("running payload missing pool_id")
	}

	// The running payload must reference exactly one DISTINCT pool id: every
	// "pool-" token (the pool_id field and the one inside the hint) must be the
	// same id that keys the job. A second, different id anywhere = the
	// "double pool_id" bug this test guards against.
	re := regexp.MustCompile(`pool-[0-9a-f]{16}`)
	seen := map[string]bool{}
	for _, tok := range re.FindAllString(res.Content[0].(mcp.TextContent).Text, -1) {
		seen[tok] = true
	}
	if len(seen) != 1 || !seen[id] {
		t.Errorf("running payload must reference exactly one pool id (= %q), saw %v", id, seen)
	}
	if job, ok := reg.Get(id); !ok || job.ID != id {
		t.Fatal("stored job id mismatch")
	}

	// After completion the replayed result must contain NO pool_id at all.
	out := reg.LongPoll(context.Background(), id, 2*time.Second)
	if out.State != StateDone {
		t.Fatalf("poll state = %q, want done", out.State)
	}
	assertResultText(t, out.Result, "final-result")
	if strings.Contains(out.Result.Content[0].(mcp.TextContent).Text, "pool-") {
		t.Error("completed result must not carry any pool_id")
	}
}

func TestWrapPanicStoresErrorResult(t *testing.T) {
	withPoolingConfig(t, poolEnabledConfig)
	reg := NewRegistry(time.Minute, time.Minute, 4)
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(100 * time.Millisecond)
		panic("boom")
	}
	res, err := Wrap("shell", reg, inner)(context.Background(), makeCall(map[string]any{"command": "x"}))
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	id, _ := extractRunningPayload(t, res)["pool_id"].(string)
	out := reg.LongPoll(context.Background(), id, 2*time.Second)
	if out.State != StateDone {
		t.Fatalf("poll state = %q, want done", out.State)
	}
	if out.Result == nil || !strings.Contains(out.Result.Content[0].(mcp.TextContent).Text, "panic: boom") {
		t.Errorf("stored panic result missing, got %+v", out.Result)
	}
}

func TestWrapInnerErrorStoredAsErrorResult(t *testing.T) {
	withPoolingConfig(t, poolEnabledConfig)
	reg := NewRegistry(time.Minute, time.Minute, 4)
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(100 * time.Millisecond)
		return nil, fmt.Errorf("inner exploded")
	}
	res, err := Wrap("shell", reg, inner)(context.Background(), makeCall(map[string]any{"command": "x"}))
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	id, _ := extractRunningPayload(t, res)["pool_id"].(string)
	out := reg.LongPoll(context.Background(), id, 2*time.Second)
	if out.State != StateDone {
		t.Fatalf("poll state = %q, want done", out.State)
	}
	if out.Result == nil || !strings.Contains(out.Result.Content[0].(mcp.TextContent).Text, "inner exploded") {
		t.Errorf("stored error result missing, got %+v", out.Result)
	}
}

func TestWrapRegistryFullReturnsErrorNotRunningPayload(t *testing.T) {
	withPoolingConfig(t, poolEnabledConfig)
	reg := NewRegistry(time.Minute, time.Minute, 1)
	// Occupies the only slot.
	id1, err := reg.Register(&Job{})
	if err != nil {
		t.Fatalf("seed register: %v", err)
	}
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(80 * time.Millisecond)
		return textResult("should-never-register"), nil
	}
	res, err := Wrap("shell", reg, inner)(context.Background(), makeCall(map[string]any{"command": "x"}))
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if strings.Contains(text, "pool_id") {
		t.Errorf("registry-full must NOT return a running/pool_id payload, got: %s", text)
	}
	if !strings.Contains(text, "pool registry full") {
		t.Errorf("expected explicit registry-full error, got: %s", text)
	}
	// The seed job is still retrievable.
	if out := reg.LongPoll(context.Background(), id1, time.Millisecond); out.State != StateRunning {
		t.Errorf("seed job state = %q", out.State)
	}
}

func TestWrapDetachesContext(t *testing.T) {
	withPoolingConfig(t, `{"pooling":{"enabled":true,"tools":["shell"],"elapsedMs":50},"tool_suggestions_enabled":false}`)
	reg := NewRegistry(time.Minute, time.Minute, 4)
	var sawCancel atomic.Bool
	entered := make(chan struct{})
	inner := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		close(entered)
		select {
		case <-ctx.Done():
			sawCancel.Store(true)
		case <-time.After(300 * time.Millisecond):
		}
		return textResult("survived"), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	resCh := make(chan *mcp.CallToolResult, 1)
	go func() {
		res, _ := Wrap("shell", reg, inner)(ctx, makeCall(map[string]any{"command": "x"}))
		resCh <- res
	}()
	<-entered
	cancel() // client aborts; inner must be detached
	<-resCh
	time.Sleep(50 * time.Millisecond)
	if sawCancel.Load() {
		t.Error("inner handler must not observe request cancellation (WithoutCancel)")
	}
}
