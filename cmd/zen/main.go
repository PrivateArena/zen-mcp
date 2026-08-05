package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jang/zen-mcp/internal/logfilter"
	"github.com/jang/zen-mcp/internal/mcpcfg"
)

func main() {
	isStdio := false
	for _, a := range os.Args[1:] {
		if a == "--stdio" {
			isStdio = true
		}
	}

	if isStdio {
		logPath := "/tmp/mcp-server-debug.log"
		startup := fmt.Sprintf("[%s] Server starting in STDIO mode\n", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
		if err := os.WriteFile(logPath, []byte(startup), 0o644); err != nil {
			logfilter.Debugf("[MCP] Could not write stdio log: %v", err)
		} else if err := logfilter.SetStdioFile(logPath); err != nil {
			logfilter.Debugf("[MCP] Could not open stdio log: %v", err)
		}
	}

	if err := mcpcfg.Load(); err != nil {
		logfilter.Debugf("[MCP] Failed to parse config.json, using defaults: %v", err)
	}

	cfg := mcpcfg.Get()
	logfilter.Setup(cfg.LogLevel)

	startTime := time.Now()

	stop := mcpcfg.WatchConfig(func() {
		if err := mcpcfg.Load(); err != nil {
			logfilter.Debugf("[Config] Failed to reload config.json: %v", err)
			return
		}
		logfilter.Setup(mcpcfg.Get().LogLevel)
		logfilter.Info("[Config] Live-reloaded config.json successfully.")
	})
	defer stop()

	logfilter.Info(`
╔════════════════════════════════════════════════════════════╗
║  Zen Tools MCP Server v2.4.1 - STABLE EDITION              ║
╠════════════════════════════════════════════════════════════╣
║  Started: ` + startTime.Format("15:04:05") + `                                            ║
║  Status: READY                                             ║
╚════════════════════════════════════════════════════════════╝`)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
