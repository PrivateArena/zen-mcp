package server

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var shutdownOnce sync.Once

var shutdownCh chan struct{}

// SetupShutdownHandlers installs SIGINT/SIGTERM handlers. Idempotent.
func SetupShutdownHandlers(mode string, logf func(format string, args ...any)) chan struct{} {
	shutdownOnce.Do(func() {
		label := "SSE"
		if mode == "stdio" {
			label = "Stdio"
		}
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		shutdownCh = make(chan struct{})
		go func() {
			<-ch
			if logf != nil {
				logf("\n🛑 Graceful Shutdown (%s)...", label)
			}
			close(shutdownCh)
		}()
	})
	return shutdownCh
}
