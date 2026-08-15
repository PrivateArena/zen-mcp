package server

import (
	"sync"
	"time"
)

// F1: The legacy AcquireServer/SwapServer/inflight pool machinery was removed.
// It had zero production callers (only pool_test.go exercised it) and its idle
// reaper therefore reaped nothing. The serverCache map is now the single source
// of cached MCPServer instances; this file owns the cache registry and the idle
// reaper that enforces the cap + TTL eviction (F2) in production.

const (
	serverCacheMaxSize      = 32
	serverCacheTTL          = 10 * time.Minute
	serverCacheReapInterval = 60 * time.Second
)

var (
	cacheRegMu sync.Mutex
	cacheReg   = map[*serverCache]struct{}{}
)

// registerServerCache is a helper function
func registerServerCache(c *serverCache) {
	cacheRegMu.Lock()
	defer cacheRegMu.Unlock()
	cacheReg[c] = struct{}{}
}

// unregisterServerCache is a helper function
func unregisterServerCache(c *serverCache) {
	cacheRegMu.Lock()
	defer cacheRegMu.Unlock()
	delete(cacheReg, c)
}

// snapshotServerCaches is a helper function
func snapshotServerCaches() []*serverCache {
	cacheRegMu.Lock()
	defer cacheRegMu.Unlock()
	out := make([]*serverCache, 0, len(cacheReg))
	for c := range cacheReg {
		out = append(out, c)
	}
	return out
}

// StartIdleReaper launches the idle server reaper over every registered
// serverCache and returns a stop function.
func StartIdleReaper() func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(serverCacheReapInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				for _, c := range snapshotServerCaches() {
					c.reapIdle(time.Now())
				}
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}
