package server

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var shutdownOnce sync.Once

// SetupShutdownHandlers installs SIGINT/SIGTERM handlers that save session
// state before exit, matching shutdown.ts. Idempotent.
func SetupShutdownHandlers(mode string, save func() error, logf func(format string, args ...any)) {
	shutdownOnce.Do(func() {
		label := "SSE"
		if mode == "stdio" {
			label = "Stdio"
		}
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-ch
			if logf != nil {
				logf("\n🛑 Graceful Shutdown (%s)...", label)
			}
			if save != nil {
				_ = save()
			}
			os.Exit(0)
		}()
	})
}
