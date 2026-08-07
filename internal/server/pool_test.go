package server

import (
	"strings"
	"sync"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"zen-mcp/internal/toolregistry"
)

func newTestFactory(counter *int, mu *sync.Mutex) Factory {
	return func(id string) *mcpserver.MCPServer {
		if counter != nil {
			mu.Lock()
			*counter++
			mu.Unlock()
		}
		return mcpserver.NewMCPServer("zen-tools", "2.4.1", mcpserver.WithToolCapabilities(true))
	}
}

func TestAcquireReleaseReuses(t *testing.T) {
	var n int
	var mu sync.Mutex
	tag := t.Name()
	reg := toolregistry.Create()

	a, err := AcquireServer(tag, "default", newTestFactory(&n, &mu), reg)
	if err != nil {
		t.Fatal(err)
	}
	ReleaseServer(tag, "default", a)

	b, err := AcquireServer(tag, "default", newTestFactory(&n, &mu), reg)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("acquire after release should reuse the same server instance")
	}
	if n != 1 {
		t.Errorf("factory should have created exactly 1 server, got %d", n)
	}
}

func TestAcquireMaxPoolAndQueuing(t *testing.T) {
	var n int
	var mu sync.Mutex
	tag := t.Name()
	factory := newTestFactory(&n, &mu)

	a, _ := AcquireServer(tag, "default", factory, nil)
	b, _ := AcquireServer(tag, "default", factory, nil)
	c, _ := AcquireServer(tag, "default", factory, nil)
	if n != 3 {
		t.Fatalf("expected 3 servers created, got %d", n)
	}
	_, _, _ = a, b, c

	// 4th acquire queues behind the 3 busy slots.
	done := make(chan *mcpserver.MCPServer, 1)
	errCh := make(chan error, 1)
	go func() {
		s, err := AcquireServer(tag, "default", factory, nil, 5*time.Second)
		if err != nil {
			errCh <- err
			return
		}
		done <- s
	}()

	ReleaseServer(tag, "default", c)
	select {
	case s := <-done:
		if s != c {
			t.Error("queued waiter should receive the released server")
		}
	case err := <-errCh:
		t.Fatalf("acquire failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("queued acquire never completed")
	}
	if n != 3 {
		t.Errorf("no new server should be created while queued, factory count %d", n)
	}
}

func TestAcquireTimeout(t *testing.T) {
	var n int
	var mu sync.Mutex
	tag := t.Name()
	factory := newTestFactory(&n, &mu)

	for i := 0; i < 3; i++ {
		if _, err := AcquireServer(tag, "default", factory, nil); err != nil {
			t.Fatal(err)
		}
	}

	_, err := AcquireServer(tag, "default", factory, nil, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "[ServerManager] Timed out after 50ms") {
		t.Errorf("unexpected timeout message: %v", msg)
	}
	if !strings.Contains(msg, "(all 3 slots busy)") {
		t.Errorf("timeout message should mention slot count: %v", msg)
	}
}

func TestInflightDefersCreation(t *testing.T) {
	var n int
	var mu sync.Mutex
	tag := t.Name()
	factory := newTestFactory(&n, &mu)

	a, _ := AcquireServer(tag, "default", factory, nil)
	BeginToolCall(a)

	// With in-flight calls, a second acquire must queue, not create.
	done := make(chan *mcpserver.MCPServer, 1)
	go func() {
		s, _ := AcquireServer(tag, "default", factory, nil, 5*time.Second)
		done <- s
	}()
	time.Sleep(100 * time.Millisecond)
	if n != 1 {
		t.Fatalf("server creation should be deferred while inflight, count %d", n)
	}

	EndToolCall(a)
	ReleaseServer(tag, "default", a)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("queued acquire never completed")
	}
	if n != 1 {
		t.Errorf("factory count should stay 1, got %d", n)
	}
}

func TestHasInflightCalls(t *testing.T) {
	tag := t.Name()
	a, _ := AcquireServer(tag, "default", newTestFactory(nil, nil), nil)
	if HasInflightCalls(tag, "default") {
		t.Error("no calls in flight yet")
	}
	BeginToolCall(a)
	if !HasInflightCalls(tag, "default") {
		t.Error("should report inflight")
	}
	if !HasAnyInflightCalls() {
		t.Error("HasAnyInflightCalls should be true")
	}
	EndToolCall(a)
	if HasInflightCalls(tag, "default") {
		t.Error("inflight should clear after end")
	}
}

func TestSwapServerReplaces(t *testing.T) {
	var n int
	var mu sync.Mutex
	tag := t.Name()
	factory := newTestFactory(&n, &mu)
	reg := toolregistry.Create()

	a, _ := AcquireServer(tag, "default", factory, reg)
	ReleaseServer(tag, "default", a)

	b, err := SwapServer(tag, "default", factory, reg)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("swap should produce a fresh server")
	}
	if n != 2 {
		t.Errorf("factory should have created 2 servers, got %d", n)
	}
	if got := GetCachedServer(tag, "default"); got != b {
		t.Error("cached server should be the swapped-in one")
	}
	// Old server's inflight/busy references are dropped; new one works.
	c, _ := AcquireServer(tag, "default", factory, reg)
	if c != b {
		t.Error("acquire should reuse swapped server")
	}
}

func TestSwapWaitsForInflight(t *testing.T) {
	tag := t.Name()
	factory := newTestFactory(nil, nil)
	a, _ := AcquireServer(tag, "default", factory, nil)
	ReleaseServer(tag, "default", a)

	BeginToolCall(a)
	swapDone := make(chan *mcpserver.MCPServer, 1)
	go func() {
		s, _ := SwapServer(tag, "default", factory, nil)
		swapDone <- s
	}()
	time.Sleep(100 * time.Millisecond)
	select {
	case <-swapDone:
		t.Fatal("swap should block while a call is in flight")
	default:
	}
	EndToolCall(a)
	select {
	case <-swapDone:
	case <-time.After(3 * time.Second):
		t.Fatal("swap should complete after in-flight call drains")
	}
}

func TestClearServerCache(t *testing.T) {
	tag := t.Name()
	_, _ = AcquireServer(tag, "default", newTestFactory(nil, nil), nil)
	ClearServerCache()
	if GetCachedServer(tag, "default") != nil {
		t.Error("cache should be empty after clear")
	}
}

func TestReapIdleOnce(t *testing.T) {
	var n int
	var mu sync.Mutex
	tag := t.Name()
	factory := newTestFactory(&n, &mu)

	a, _ := AcquireServer(tag, "default", factory, nil)
	b, _ := AcquireServer(tag, "default", factory, nil)
	ReleaseServer(tag, "default", a)
	ReleaseServer(tag, "default", b)

	// Make only one entry stale; the fresh one must be kept.
	p := getPool(cacheKey(tag, "default"))
	p.mu.Lock()
	p.Entries[0].LastUsed = time.Now().Add(-2 * time.Hour)
	p.mu.Unlock()

	reapIdleOnce(time.Now())

	p.mu.Lock()
	var remain []*PoolEntry
	for _, e := range p.Entries {
		if e.Busy == false {
			remain = append(remain, e)
		}
	}
	busy := len(p.Entries) - len(remain)
	p.mu.Unlock()

	if len(remain) != 1 || busy != 0 {
		t.Fatalf("expected exactly 1 idle entry to remain, got %d idle, %d busy", len(remain), busy)
	}
	if remain[0].Server != b {
		t.Error("the fresh entry should be the one kept")
	}

	c, _ := AcquireServer(tag, "default", factory, nil)
	if c != b {
		t.Error("acquire should reuse the surviving entry")
	}
	if n != 2 {
		t.Errorf("factory should not create a new server after reaping one stale, count %d", n)
	}
}

func TestReapKeepsAllWhenAllStale(t *testing.T) {
	var n int
	var mu sync.Mutex
	tag := t.Name()
	factory := newTestFactory(&n, &mu)

	a, _ := AcquireServer(tag, "default", factory, nil)
	b, _ := AcquireServer(tag, "default", factory, nil)
	ReleaseServer(tag, "default", a)
	ReleaseServer(tag, "default", b)

	// All entries stale: reaper must not touch the pool (TS behavior).
	p := getPool(cacheKey(tag, "default"))
	p.mu.Lock()
	for _, e := range p.Entries {
		e.LastUsed = time.Now().Add(-2 * time.Hour)
	}
	p.mu.Unlock()

	reapIdleOnce(time.Now())
	p.mu.Lock()
	left := len(p.Entries)
	p.mu.Unlock()
	if left != 2 {
		t.Errorf("reaper must keep everything when all entries are stale, got %d", left)
	}
}

func TestConcurrentAcquireRelease(t *testing.T) {
	var n int
	var mu sync.Mutex
	tag := t.Name()
	factory := newTestFactory(&n, &mu)

	const workers = 12
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				s, err := AcquireServer(tag, "default", factory, nil, 10*time.Second)
				if err != nil {
					t.Errorf("acquire: %v", err)
					return
				}
				BeginToolCall(s)
				time.Sleep(time.Millisecond)
				EndToolCall(s)
				ReleaseServer(tag, "default", s)
			}
		}()
	}
	wg.Wait()
	if n > 3 {
		t.Errorf("pool should never create more than %d servers, got %d", MaxPoolSize, n)
	}
	if n == 0 {
		t.Error("factory should have created servers")
	}
}

func TestSwapSerializes(t *testing.T) {
	var n int
	var mu sync.Mutex
	tag := t.Name()
	factory := newTestFactory(&n, &mu)

	_, _ = AcquireServer(tag, "default", factory, nil)
	ReleaseServer(tag, "default", GetCachedServer(tag, "default"))

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := SwapServer(tag, "default", factory, nil)
			if err != nil {
				t.Errorf("swap: %v", err)
			}
		}()
	}
	wg.Wait()

	p := getPool(cacheKey(tag, "default"))
	p.mu.Lock()
	swapping := p.Swapping
	p.mu.Unlock()
	if swapping {
		t.Error("swapping flag should be cleared after swaps complete")
	}
}
