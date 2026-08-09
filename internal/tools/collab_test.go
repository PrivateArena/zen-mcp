package tools

import (
	"sync"
	"testing"
)

func TestCollabRegisterResolveOnce(t *testing.T) {
	r := NewCollaborationRegistry()
	called := 0
	r.Register("id1", func(p string) { called++ })
	if !r.Contains("id1") {
		t.Fatal("entry should be pending after Register")
	}
	if !r.Resolve("id1", "/tmp/a.png") {
		t.Fatal("first Resolve should succeed")
	}
	if called != 1 {
		t.Errorf("resolve callback called %d times, want 1", called)
	}
	if r.Contains("id1") {
		t.Error("entry should be removed after resolve")
	}
}

func TestCollabResolveTwiceRejects(t *testing.T) {
	r := NewCollaborationRegistry()
	called := 0
	r.Register("id2", func(p string) { called++ })
	if !r.Resolve("id2", "/a") {
		t.Fatal("first resolve failed")
	}
	if r.Resolve("id2", "/b") {
		t.Error("second resolve must be rejected (single-owner)")
	}
	if called != 1 {
		t.Errorf("resolve callback called %d times, want 1", called)
	}
}

func TestCollabResolveAfterExpireRejects(t *testing.T) {
	r := NewCollaborationRegistry()
	r.Register("id3", func(p string) {})
	if !r.Expire("id3") {
		t.Fatal("expire should claim pending entry")
	}
	if r.Resolve("id3", "/c") {
		t.Error("resolve after expire must be rejected")
	}
}

func TestCollabResolveUnknownRejects(t *testing.T) {
	r := NewCollaborationRegistry()
	if r.Resolve("nope", "/d") {
		t.Error("resolve of unknown id must be rejected")
	}
	if r.Expire("nope") {
		t.Error("expire of unknown id must be rejected")
	}
}

// TestCollabConcurrentResolveExactlyOnce is the -race guard for F5/F11: many
// concurrent HTTP POSTs racing a timeout must claim the collaboration exactly
// once and invoke the resolve callback exactly once.
func TestCollabConcurrentResolveExactlyOnce(t *testing.T) {
	r := NewCollaborationRegistry()
	called := 0
	var mu sync.Mutex
	r.Register("race", func(p string) {
		mu.Lock()
		called++
		mu.Unlock()
	})

	const workers = 64
	var wg sync.WaitGroup
	success := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			success <- r.Resolve("race", "/x")
		}()
	}
	wg.Wait()
	close(success)

	wins := 0
	for ok := range success {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Errorf("exactly one Resolve should win, got %d", wins)
	}
	mu.Lock()
	defer mu.Unlock()
	if called != 1 {
		t.Errorf("resolve callback called %d times, want 1", called)
	}
}

func TestCollabNilRegistrySafe(t *testing.T) {
	var r *CollaborationRegistry
	r.Register("a", func(string) {})
	if r.Resolve("a", "/x") {
		t.Error("nil registry resolve must be rejected")
	}
	if r.Expire("a") {
		t.Error("nil registry expire must be rejected")
	}
	if r.Contains("a") {
		t.Error("nil registry contains must be false")
	}
}
