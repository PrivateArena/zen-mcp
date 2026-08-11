package pooling

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func TestRegistryRegisterAndComplete(t *testing.T) {
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 10)
	job := &Job{Done: make(chan struct{})}
	id, err := reg.Register(job)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if id == "" {
		t.Fatal("Register returned empty id")
	}

	res := &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "done"}}}
	if !reg.Complete(id, res) {
		t.Fatal("Complete failed")
	}

	got, ok := reg.Get(id)
	if !ok {
		t.Fatal("Get failed after Complete")
	}
	if got.Result == nil {
		t.Fatal("Result not set after Complete")
	}
	if got.Result.Content[0].(mcp.TextContent).Text != "done" {
		t.Errorf("Result text = %q, want done", got.Result.Content[0].(mcp.TextContent).Text)
	}
}

func TestRegistryDuplicatePolls(t *testing.T) {
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 10)
	job := &Job{Done: make(chan struct{})}
	id, _ := reg.Register(job)

	res := &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ok"}}}
	reg.Complete(id, res)

	out1 := reg.LongPoll(context.Background(), id, 0)
	out2 := reg.LongPoll(context.Background(), id, 0)

	if out1.State != "done" {
		t.Errorf("first poll state = %q, want done", out1.State)
	}
	if out2.State != "done" {
		t.Errorf("second poll state = %q, want done", out2.State)
	}
	if out1.Result == nil {
		t.Error("first poll must return a result")
	}
	if out2.Result == nil {
		t.Error("second poll must return a result")
	}
}

func TestRegistryLongPollRunningToDone(t *testing.T) {
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 10)
	job := &Job{Done: make(chan struct{})}
	id, _ := reg.Register(job)

	out := reg.LongPoll(context.Background(), id, 50*time.Millisecond)
	if out.State != "running" {
		t.Fatalf("initial poll state = %q, want running", out.State)
	}

	res := &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "late"}}}
	reg.Complete(id, res)

	out = reg.LongPoll(context.Background(), id, 0)
	if out.State != "done" {
		t.Fatalf("poll after complete state = %q, want done", out.State)
	}
	if out.Result == nil || out.Result.Content[0].(mcp.TextContent).Text != "late" {
		t.Error("poll after complete must return stored result")
	}
}

func TestRegistryCancel(t *testing.T) {
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 10)
	job := &Job{Done: make(chan struct{})}
	id, _ := reg.Register(job)

	if !reg.Cancel(id) {
		t.Fatal("Cancel failed for existing job")
	}
	if reg.Cancel(id) {
		t.Error("Cancel should return false when already cancelled")
	}

	out := reg.LongPoll(context.Background(), id, 0)
	if out.State != "cancelled" {
		t.Errorf("poll after cancel state = %q, want cancelled", out.State)
	}
}

func TestRegistryLongPollUnknown(t *testing.T) {
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 10)
	out := reg.LongPoll(context.Background(), "pool-nonexistent", 0)
	if out.State != "unknown" {
		t.Errorf("poll unknown state = %q, want unknown", out.State)
	}
}

func TestRegistryEvictExpiredDoneKeepsGrace(t *testing.T) {
	reg := NewRegistry(60*time.Minute, 5*time.Minute, 10)
	job := &Job{Done: make(chan struct{})}
	id, _ := reg.Register(job)

	res := &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "x"}}}
	reg.Complete(id, res)

	now := time.Now()
	if reg.EvictExpired(now) != 0 {
		t.Fatal("done job should not be evicted immediately")
	}

	afterGrace := now.Add(6 * time.Minute)
	if reg.EvictExpired(afterGrace) != 1 {
		t.Fatal("done job should be evicted after grace")
	}

	if _, ok := reg.Get(id); ok {
		t.Error("evicted job should not be retrievable")
	}
}

func TestRegistryEvictExpiredRunningByTTL(t *testing.T) {
	reg := NewRegistry(5*time.Minute, 60*time.Minute, 10)
	job := &Job{Done: make(chan struct{}), CreatedAt: time.Now().Add(-6 * time.Minute)}
	id, _ := reg.Register(job)

	if reg.EvictExpired(time.Now()) != 1 {
		t.Fatal("running job past TTL should be evicted")
	}
	if _, ok := reg.Get(id); ok {
		t.Error("evicted running job should not be retrievable")
	}
}

func TestRegistryCapRejectsRegistration(t *testing.T) {
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 2)
	for i := 0; i < 2; i++ {
		if _, err := reg.Register(&Job{Done: make(chan struct{})}); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}
	_, err := reg.Register(&Job{Done: make(chan struct{})})
	if err == nil {
		t.Fatal("expected error when registry is full")
	}
	if !errors.Is(err, ErrRegistryFull) {
		t.Errorf("error type = %T, want ErrRegistryFull", err)
	}
}

func TestRegistryCapEvictsOldestExpired(t *testing.T) {
	reg := NewRegistry(5*time.Minute, 60*time.Minute, 2)
	old := &Job{Done: make(chan struct{}), CreatedAt: time.Now().Add(-10 * time.Minute)}
	newJ := &Job{Done: make(chan struct{}), CreatedAt: time.Now()}
	reg.Register(old)
	reg.Register(newJ)

	_, err := reg.Register(&Job{Done: make(chan struct{})})
	if err != nil {
		t.Fatalf("register after eviction: %v", err)
	}
	if _, ok := reg.Get(old.ID); ok {
		t.Error("oldest expired job should have been evicted")
	}
	if _, ok := reg.Get(newJ.ID); !ok {
		t.Error("new job should survive eviction")
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	reg := NewRegistry(60*time.Minute, 60*time.Minute, 100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job := &Job{Done: make(chan struct{})}
			id, err := reg.Register(job)
			if err != nil {
				return
			}
			reg.Complete(id, &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "c"}}})
			reg.Get(id)
			reg.LongPoll(context.Background(), id, 0)
			reg.Cancel(id)
			reg.List()
		}()
	}
	wg.Wait()
}

