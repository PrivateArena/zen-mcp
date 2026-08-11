package pooling

import (
	"context"
	"sync"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: s}}}
}

func TestRegisterReturnsSameIDThatKeysTheJob(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	id, err := r.Register(&Job{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if id == "" {
		t.Fatal("Register returned empty id")
	}
	job, ok := r.Get(id)
	if !ok {
		t.Fatalf("job %q not found", id)
	}
	if job.ID != id {
		t.Errorf("job.ID = %q, want %q (single id must be the map key)", job.ID, id)
	}
	if job.Done == nil {
		t.Error("Register must initialize job.Done")
	}
}

func TestRegisterHonorsPreassignedID(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	id, err := r.Register(&Job{ID: "pool-custom"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if id != "pool-custom" {
		t.Errorf("returned id = %q, want pool-custom", id)
	}
	if _, err := r.Register(&Job{ID: "pool-custom"}); err == nil {
		t.Error("duplicate preassigned id should error")
	}
}

func TestCompleteSetsResultBeforeClosingDone(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	id, err := r.Register(&Job{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	want := textResult("done-payload")
	if !r.Complete(id, want) {
		t.Fatal("Complete returned false for live job")
	}
	job, _ := r.Get(id)
	<-job.Done
	if job.Result != want {
		t.Error("polled result must be the exact stored *CallToolResult (verbatim replay)")
	}
	if job.FinishedAt.IsZero() {
		t.Error("FinishedAt should be set on completion")
	}
	if r.Complete(id, textResult("again")) {
		t.Error("second Complete on a closed job should return false")
	}
}

func TestCompleteOnEvictedJobIsNoop(t *testing.T) {
	r := NewRegistry(10*time.Millisecond, time.Millisecond, 4)
	id, err := r.Register(&Job{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if n := r.EvictExpired(time.Now()); n != 1 {
		t.Fatalf("expected 1 eviction, got %d", n)
	}
	if r.Complete(id, textResult("late")) {
		t.Error("Complete on evicted job must return false")
	}
	if _, ok := r.Get(id); ok {
		t.Error("job should be gone after eviction")
	}
}

func TestLongPollRunningThenDone(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	id, _ := r.Register(&Job{})
	if out := r.LongPoll(context.Background(), id, 20*time.Millisecond); out.State != StateRunning {
		t.Fatalf("before completion state = %q, want running", out.State)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		r.Complete(id, textResult("finished"))
	}()
	out := r.LongPoll(context.Background(), id, time.Second)
	if out.State != StateDone {
		t.Fatalf("after completion state = %q, want done", out.State)
	}
	if got := out.Result.Content[0].(mcp.TextContent).Text; got != "finished" {
		t.Errorf("result text = %q", got)
	}
}

func TestLongPollCancelled(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	id, _ := r.Register(&Job{})
	if !r.Cancel(id) {
		t.Fatal("Cancel returned false")
	}
	if out := r.LongPoll(context.Background(), id, time.Millisecond); out.State != StateCancelled {
		t.Errorf("state = %q, want cancelled", out.State)
	}
}

func TestLongPollUnknownNeverCreatesJob(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	out := r.LongPoll(context.Background(), "pool-missing", 10*time.Millisecond)
	if out.State != StateUnknown {
		t.Fatalf("state = %q, want unknown", out.State)
	}
	if _, ok := r.Get("pool-missing"); ok {
		t.Error("LongPoll must NOT create a job for an unknown id")
	}
	if n := len(r.List()); n != 0 {
		t.Errorf("registry should stay empty after unknown poll, has %d jobs", n)
	}
}

func TestLongPollContextCancel(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	id, _ := r.Register(&Job{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := r.LongPoll(ctx, id, time.Hour)
	if out.State != StateRunning {
		t.Errorf("aborted poll state = %q, want running", out.State)
	}
}

func TestDuplicatePollsBothGetResult(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	id, _ := r.Register(&Job{})
	const n = 5
	var wg sync.WaitGroup
	results := make([]PollOutcome, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = r.LongPoll(context.Background(), id, 2*time.Second)
		}(i)
	}
	time.Sleep(50 * time.Millisecond)
	r.Complete(id, textResult("multi"))
	wg.Wait()
	for i := 0; i < n; i++ {
		if results[i].State != StateDone {
			t.Errorf("receiver %d state = %q, want done", i, results[i].State)
		}
	}
}

func TestCancelThenCompleteReportsCancelled(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	id, _ := r.Register(&Job{})
	r.Cancel(id)
	go r.Complete(id, textResult("late"))
	// Poll is checked against Cancelled first, so cancel wins.
	time.Sleep(10 * time.Millisecond)
	if out := r.LongPoll(context.Background(), id, 50*time.Millisecond); out.State != StateCancelled {
		t.Errorf("state = %q, want cancelled", out.State)
	}
}

func TestStateNonBlocking(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	id, _ := r.Register(&Job{})
	if got := r.State(id); got != StateRunning {
		t.Errorf("State before completion = %q, want running", got)
	}
	if got := r.State("pool-nope"); got != StateUnknown {
		t.Errorf("State for missing id = %q, want unknown", got)
	}
	r.Complete(id, textResult("ok"))
	if got := r.State(id); got != StateDone {
		t.Errorf("State after completion = %q, want done", got)
	}
	r.Cancel(id)
	if got := r.State(id); got != StateCancelled {
		t.Errorf("State after cancel = %q, want cancelled", got)
	}
}

func TestEvictExpiredKeepsDoneWithinGrace(t *testing.T) {
	r := NewRegistry(10*time.Millisecond, time.Hour, 4)
	id, _ := r.Register(&Job{})
	r.Complete(id, textResult("ok"))
	now := time.Now().Add(2 * time.Second) // far past TTL but within grace
	if n := r.EvictExpired(now); n != 0 {
		t.Fatalf("done job within grace evicted (%d)", n)
	}
	if _, ok := r.Get(id); !ok {
		t.Fatal("done-but-unpolled job must survive until grace expiry")
	}
	if n := r.EvictExpired(now.Add(time.Hour + time.Minute)); n != 1 {
		t.Fatalf("done job past grace should evict, got %d", n)
	}
}

func TestEvictExpiredEvictsRunningAfterTTL(t *testing.T) {
	r := NewRegistry(10*time.Millisecond, time.Hour, 4)
	id, _ := r.Register(&Job{})
	now := time.Now().Add(time.Second)
	if n := r.EvictExpired(now); n != 1 {
		t.Fatalf("running job past TTL should evict, got %d", n)
	}
	if _, ok := r.Get(id); ok {
		t.Error("running job should be evicted after TTL")
	}
}

func TestRegisterRejectsAtCap(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 2)
	for i := 0; i < 2; i++ {
		if _, err := r.Register(&Job{}); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}
	if _, err := r.Register(&Job{}); err != ErrRegistryFull {
		t.Fatalf("third register err = %v, want ErrRegistryFull", err)
	}
}

func TestRegisterAtCapReclaimsExpiredSlot(t *testing.T) {
	r := NewRegistry(10*time.Millisecond, time.Minute, 1)
	if _, err := r.Register(&Job{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	id, err := r.Register(&Job{})
	if err != nil {
		t.Fatalf("register after expired slot should reclaim, got %v", err)
	}
	if id == "" {
		t.Error("empty id")
	}
	if n := len(r.List()); n != 1 {
		t.Errorf("registry should hold exactly 1 job, got %d", n)
	}
}

func TestListShapeAndOrder(t *testing.T) {
	r := NewRegistry(time.Minute, time.Minute, 4)
	id1, _ := r.Register(&Job{})
	id2, _ := r.Register(&Job{})
	r.Cancel(id1)
	r.Complete(id2, textResult("done"))
	time.Sleep(2 * time.Millisecond)
	ids := make(map[string]bool)
	for _, info := range r.List() {
		ids[info.ID] = true
		if info.AgeMs < 0 {
			t.Errorf("negative age for %s", info.ID)
		}
	}
	if !ids[id1] || !ids[id2] {
		t.Errorf("list missing jobs: %v", ids)
	}
	byState := map[string]string{}
	for _, info := range r.List() {
		byState[info.ID] = info.State
	}
	if byState[id1] != StateCancelled || byState[id2] != StateDone {
		t.Errorf("list states wrong: %+v", byState)
	}
}
