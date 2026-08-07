package server

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/jang/zen-mcp/internal/toolregistry"
	"github.com/jang/zen-mcp/internal/toolstate"
)

const (
	MaxPoolSize               = 3
	IDLEReap                  = 10 * time.Minute
	DefaultAcquireTimeout     = 30 * time.Second
	DeferredAcquireTimeout    = 2_400_000 * time.Millisecond
	SwapMaxWait               = 45 * time.Minute
	SwapCloseMaxWait          = 40 * time.Minute
	ReaperInterval            = 60 * time.Second
	ReaperPollInterval        = 250 * time.Millisecond
	SwapSerializePollInterval = 25 * time.Millisecond
)

// Factory builds a fresh MCPServer for a logical workspace id.
type Factory func(id string) *mcpserver.MCPServer

type PoolEntry struct {
	Server   *mcpserver.MCPServer
	Registry *toolregistry.ToolRegistry
	Busy     bool
	LastUsed time.Time
	Inflight int
}

type Waiter struct {
	result chan *mcpserver.MCPServer
	err    chan error
	timer  *time.Timer
}

// Pool is never replaced once created for a key — only Entries/Waiters/Closing
// are mutated. This keeps every closure pointing at live state.
type Pool struct {
	mu       sync.Mutex
	Entries  []*PoolEntry
	Waiters  []*Waiter
	Closing  []*PoolEntry
	Swapping bool
}

var (
	poolsMu sync.Mutex
	pools   = map[string]*Pool{}
)

func cacheKey(tag, logicalID string) string { return tag + ":" + logicalID }

func getOrCreatePool(key string) *Pool {
	poolsMu.Lock()
	defer poolsMu.Unlock()
	p, ok := pools[key]
	if !ok {
		p = &Pool{}
		pools[key] = p
	}
	return p
}

func getPool(key string) *Pool {
	poolsMu.Lock()
	defer poolsMu.Unlock()
	return pools[key]
}

func (p *Pool) hasInflight() bool {
	for _, e := range p.Entries {
		if e.Inflight > 0 {
			return true
		}
	}
	return false
}

func (p *Pool) findIdle() *PoolEntry {
	for _, e := range p.Entries {
		if !e.Busy {
			return e
		}
	}
	return nil
}

// drainWaiters hands released entries to queued waiters. Caller holds p.mu.
func (p *Pool) drainWaiters() {
	for len(p.Waiters) > 0 {
		idle := p.findIdle()
		if idle == nil {
			return
		}
		w := p.Waiters[0]
		p.Waiters = p.Waiters[1:]
		if w.timer != nil {
			w.timer.Stop()
		}
		idle.Busy = true
		idle.LastUsed = time.Now()
		w.result <- idle.Server
	}
}

func (p *Pool) removeWaiter(w *Waiter) {
	for i, x := range p.Waiters {
		if x == w {
			p.Waiters = append(p.Waiters[:i], p.Waiters[i+1:]...)
			return
		}
	}
}

// findEntry locates an entry (active or closing) by server pointer.
func (p *Pool) findEntry(srv *mcpserver.MCPServer) *PoolEntry {
	for _, e := range p.Entries {
		if e.Server == srv {
			return e
		}
	}
	for _, e := range p.Closing {
		if e.Server == srv {
			return e
		}
	}
	return nil
}

// AcquireServer returns an idle pooled server, or creates one if the pool has
// room, or queues behind in-flight calls. Mirrors server-manager.ts.
func AcquireServer(cacheTag, logicalID string, factory Factory, registry *toolregistry.ToolRegistry, acquireTimeout ...time.Duration) (*mcpserver.MCPServer, error) {
	key := cacheKey(cacheTag, logicalID)
	p := getOrCreatePool(key)

	to := DefaultAcquireTimeout
	if len(acquireTimeout) > 0 {
		to = acquireTimeout[0]
	}

	p.mu.Lock()
	if idle := p.findIdle(); idle != nil {
		idle.Busy = true
		idle.LastUsed = time.Now()
		srv := idle.Server
		p.mu.Unlock()
		return srv, nil
	}

	if len(p.Entries) < MaxPoolSize && !p.hasInflight() {
		placeholder := &PoolEntry{Busy: true, LastUsed: time.Now()}
		p.Entries = append(p.Entries, placeholder)
		p.mu.Unlock()

		srv := factory(logicalID)
		if registry != nil {
			toolstate.ApplyToolStates("", registry)
		}

		p.mu.Lock()
		placeholder.Server = srv
		placeholder.Registry = registry
		p.drainWaiters()
		p.mu.Unlock()
		return srv, nil
	}

	deferred := p.hasInflight()
	timeout := to
	if deferred {
		timeout = DeferredAcquireTimeout
	}

	w := &Waiter{result: make(chan *mcpserver.MCPServer, 1), err: make(chan error, 1)}
	w.timer = time.AfterFunc(timeout, func() {
		p.mu.Lock()
		p.removeWaiter(w)
		p.mu.Unlock()
		w.err <- fmt.Errorf("[ServerManager] Timed out after %dms waiting for a free server in pool %q (all %d slots busy)", timeout.Milliseconds(), key, MaxPoolSize)
	})
	p.Waiters = append(p.Waiters, w)
	p.mu.Unlock()

	select {
	case srv := <-w.result:
		return srv, nil
	case err := <-w.err:
		return nil, err
	}
}

// ReleaseServer marks an entry idle and hands it to any queued waiter.
func ReleaseServer(cacheTag, logicalID string, srv *mcpserver.MCPServer) {
	p := getPool(cacheKey(cacheTag, logicalID))
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if e := p.findEntry(srv); e != nil {
		e.Busy = false
		e.LastUsed = time.Now()
	}
	p.drainWaiters()
}

// BeginToolCall tracks an in-flight tool call for inflight-aware pool logic.
func BeginToolCall(srv *mcpserver.MCPServer) {
	if srv == nil {
		return
	}
	for _, p := range snapshotPools() {
		p.mu.Lock()
		if e := p.findEntry(srv); e != nil {
			e.Inflight++
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
	}
}

func EndToolCall(srv *mcpserver.MCPServer) {
	if srv == nil {
		return
	}
	for _, p := range snapshotPools() {
		p.mu.Lock()
		if e := p.findEntry(srv); e != nil && e.Inflight > 0 {
			e.Inflight--
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
	}
}

func HasInflightCalls(cacheTag, logicalID string) bool {
	p := getPool(cacheKey(cacheTag, logicalID))
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hasInflight()
}

func HasAnyInflightCalls() bool {
	for _, p := range snapshotPools() {
		p.mu.Lock()
		inflight := p.hasInflight()
		p.mu.Unlock()
		if inflight {
			return true
		}
	}
	return false
}

// SwapServer rebuilds a server for a key, serializing concurrent swaps and
// deferring while tool calls are mid-flight.
func SwapServer(cacheTag, logicalID string, factory Factory, registry *toolregistry.ToolRegistry) (*mcpserver.MCPServer, error) {
	key := cacheKey(cacheTag, logicalID)
	p := getOrCreatePool(key)

	for {
		p.mu.Lock()
		if !p.Swapping {
			p.Swapping = true
			p.mu.Unlock()
			break
		}
		p.mu.Unlock()
		time.Sleep(SwapSerializePollInterval)
	}
	defer func() {
		p.mu.Lock()
		p.Swapping = false
		p.mu.Unlock()
	}()

	swapDeadline := time.Now().Add(SwapMaxWait)
	for {
		p.mu.Lock()
		inflight := p.hasInflight()
		p.mu.Unlock()
		if !inflight {
			break
		}
		if time.Now().After(swapDeadline) {
			log.Printf("[ServerManager] Swap on %q waited %v for in-flight calls — proceeding anyway.", key, SwapMaxWait)
			break
		}
		time.Sleep(ReaperPollInterval)
	}

	p.mu.Lock()
	oldEntries := p.Entries
	p.Entries = nil
	p.mu.Unlock()

	if registry != nil {
		registry.Reset()
	}
	srv := factory(logicalID)
	if registry != nil {
		toolstate.ApplyToolStates("", registry)
	}

	p.mu.Lock()
	p.Entries = append(p.Entries, &PoolEntry{Server: srv, Registry: registry, LastUsed: time.Now()})
	p.drainWaiters()
	p.mu.Unlock()

	for _, e := range oldEntries {
		if e.Server == nil {
			continue
		}
		p.mu.Lock()
		ready := !e.Busy && e.Inflight == 0
		if ready {
			p.mu.Unlock()
			continue
		}
		p.Closing = append(p.Closing, e)
		p.mu.Unlock()

		go closeWhenIdle(p, e)
	}
	return srv, nil
}

func closeWhenIdle(p *Pool, e *PoolEntry) {
	deadline := time.Now().Add(SwapCloseMaxWait)
	for {
		p.mu.Lock()
		ready := !e.Busy && e.Inflight == 0
		p.mu.Unlock()
		if ready || time.Now().After(deadline) {
			break
		}
		time.Sleep(ReaperPollInterval)
	}
	p.mu.Lock()
	for i, x := range p.Closing {
		if x == e {
			p.Closing = append(p.Closing[:i], p.Closing[i+1:]...)
			break
		}
	}
	p.mu.Unlock()
}

func GetCachedServer(cacheTag, logicalID string) *mcpserver.MCPServer {
	p := getPool(cacheKey(cacheTag, logicalID))
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if e := p.findIdle(); e != nil {
		return e.Server
	}
	return nil
}

func ClearServerCache() {
	poolsMu.Lock()
	ps := make([]*Pool, 0, len(pools))
	for _, p := range pools {
		ps = append(ps, p)
	}
	pools = map[string]*Pool{}
	poolsMu.Unlock()

	for _, p := range ps {
		p.mu.Lock()
		for _, w := range p.Waiters {
			if w.timer != nil {
				w.timer.Stop()
			}
			w.err <- fmt.Errorf("[ServerManager] Server cache cleared")
		}
		p.Waiters = nil
		p.mu.Unlock()
	}
}

func snapshotPools() []*Pool {
	poolsMu.Lock()
	defer poolsMu.Unlock()
	ps := make([]*Pool, 0, len(pools))
	for _, p := range pools {
		ps = append(ps, p)
	}
	return ps
}

func reapIdleOnce(now time.Time) {
	for _, p := range snapshotPools() {
		p.mu.Lock()
		var stale []*PoolEntry
		for _, e := range p.Entries {
			if !e.Busy && e.Inflight == 0 && now.Sub(e.LastUsed) > IDLEReap {
				stale = append(stale, e)
			}
		}
		if len(stale) > 0 && len(stale) < len(p.Entries) {
			for _, e := range stale {
				for i, x := range p.Entries {
					if x == e {
						p.Entries = append(p.Entries[:i], p.Entries[i+1:]...)
						break
					}
				}
			}
		}
		p.mu.Unlock()
	}
}

// StartIdleReaper launches the idle server reaper and returns a stop function.
func StartIdleReaper() func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(ReaperInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				reapIdleOnce(time.Now())
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}

// ---- per-request server carry (feeds BeginToolCall/EndToolCall in patch.go) ----

type poolServerKey struct{}

func WithPoolServer(ctx context.Context, srv *mcpserver.MCPServer) context.Context {
	return context.WithValue(ctx, poolServerKey{}, srv)
}

func PoolServerFrom(ctx context.Context) *mcpserver.MCPServer {
	srv, _ := ctx.Value(poolServerKey{}).(*mcpserver.MCPServer)
	return srv
}
