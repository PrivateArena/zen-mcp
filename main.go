package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"zen-mcp/internal/gatekeeper"
	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/server"
	"zen-mcp/internal/shared"
	"zen-mcp/internal/shell/tokenoptimizer"
	"zen-mcp/internal/terminal"
	_ "zen-mcp/internal/terminal/handlers"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/toolresponse"
	"zen-mcp/internal/tools"
)

func newMcpServer(id string, reg *toolregistry.ToolRegistry, deps tools.Deps) *mcpserver.MCPServer {
	cfg := mcpcfg.Get()
	workspace := id
	if id == "" || id == "default" {
		workspace = cfg.DefaultWorkspaceRoot
		if workspace == "" {
			cwd, _ := os.Getwd()
			workspace = cwd
		}
	}

	srv := mcpserver.NewMCPServer(server.ServerName, server.ServerVersion,
		mcpserver.WithToolCapabilities(!deps.HideTools),
		mcpserver.WithToolFilter(server.FilterEnabled(reg)),
		mcpserver.WithResourceCapabilities(true, true),
		mcpserver.WithPromptCapabilities(false),
	)

	if err := server.RegisterAllTools(context.Background(), srv, reg, deps, workspace); err != nil {
		logfilter.Debugf("[MCP] Failed to register tools: %v", err)
	}
	return srv
}

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

	stopWatch := mcpcfg.WatchConfig(func() {
		if err := mcpcfg.Load(); err != nil {
			logfilter.Debugf("[Config] Failed to reload config.json: %v", err)
			return
		}
		logfilter.Setup(mcpcfg.Get().LogLevel)
		logfilter.Info("[Config] Live-reloaded config.json successfully.")
	})
	defer stopWatch()

	store := shared.NewStore()

	toolresponse.SetVirtualizer(func(tool, text string) (string, error) {
		ws, _ := store.Get("workspace-root")
		return tokenoptimizer.CheckAndVirtualizeOutput(tool, text, ws), nil
	})

	mode := "streamable-http"
	if isStdio {
		mode = "stdio"
	}
	os.Setenv("MCP_TRANSPORT", mode)
	shutdownCh := server.SetupShutdownHandlers(mode, func(format string, args ...any) {
		logfilter.Info(fmt.Sprintf(format, args...))
	})

	if isStdio {
		logfilter.Info("[MCP] STDIO mode: session transport wiring lands in M4.")
		<-shutdownCh
		return
	}

	runHTTPServers(startTime, cfg, store, shutdownCh)
}

func runHTTPServers(startTime time.Time, cfg *mcpcfg.ZenConfig, store *shared.Store, shutdownCh chan struct{}) {
	mcpPort := cfg.McpPort
	cliPort := cfg.CliPort
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			mcpPort = n
		}
	}
	if p := os.Getenv("CLI_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			cliPort = n
		}
	}

	filteredReg := toolregistry.Create()
	unfilteredReg := toolregistry.Create()

	gk := gatekeeper.New(store)
	pendingCollabs := tools.NewCollaborationRegistry()
	deps := tools.Deps{
		Store:                 store,
		Reg:                   filteredReg,
		Gatekeeper:            gk,
		PendingCollaborations: pendingCollabs,
	}

	filteredFactory := func(id string) *mcpserver.MCPServer {
		d := deps
		d.Reg = filteredReg
		// mcp2cli mode: the agent-facing MCP server exposes no tools; the CLI
		// wrappers target the unfiltered server which keeps the full set.
		d.HideTools = mcpcfg.Get().Mcp2Cli
		return newMcpServer(id, filteredReg, d)
	}
	unfilteredFactory := func(id string) *mcpserver.MCPServer {
		d := deps
		d.Reg = unfilteredReg
		return newMcpServer(id, unfilteredReg, d)
	}

	mcpMux := http.NewServeMux()
	server.SetupRoutes(mcpMux, server.RouteDeps{
		CreateMCPServer:       filteredFactory,
		Registry:              filteredReg,
		Shared:                store,
		PendingCollaborations: pendingCollabs,
		StartTime:             startTime,
		Tag:                   fmt.Sprintf("%d", mcpPort),
	})

	cliMux := http.NewServeMux()
	server.SetupRoutes(cliMux, server.RouteDeps{
		CreateMCPServer:       unfilteredFactory,
		Registry:              unfilteredReg,
		Shared:                store,
		PendingCollaborations: pendingCollabs,
		StartTime:             startTime,
		Tag:                   fmt.Sprintf("%d", cliPort),
	})
	// codegraph live viewer — CLI port only
	server.SetupLiveGraphRoutes(cliMux, store)

	// F9: bind the configured host explicitly. config.json's "host" defaults
	// to 127.0.0.1 so both listeners stay loopback-only unless widened.
	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	mcpAddr := net.JoinHostPort(host, strconv.Itoa(mcpPort))
	cliAddr := net.JoinHostPort(host, strconv.Itoa(cliPort))
	mcpSrv := &http.Server{Addr: mcpAddr, Handler: mcpMux}
	cliSrv := &http.Server{Addr: cliAddr, Handler: cliMux}

	mcpLn, err := net.Listen("tcp", mcpSrv.Addr)
	if err != nil {
		logfilter.Debugf("[MCP] Fatal: could not bind filtered port %d: %v", mcpPort, err)
		os.Exit(1)
	}

	cliLn, cliErr := net.Listen("tcp", cliSrv.Addr)
	cliAvailable := true
	if cliErr != nil && isAddrInUse(cliErr) {
		logfilter.Warnf("[MCP] Port %d already in use — unfiltered server disabled. CLI export will fall back to port %d.", cliPort, mcpPort)
		cliAvailable = false
	} else if cliErr != nil {
		logfilter.Debugf("[MCP] Unfiltered server error: %v", cliErr)
		cliAvailable = false
	}
	exportPort := terminal.FallbackPort(cliPort, mcpPort, cliAvailable)

	logfilter.Info(fmt.Sprintf(`
╔════════════════════════════════════════════════════════════╗
║  Zen Tools MCP Server v2.4.1 - STABLE EDITION              ║
╠════════════════════════════════════════════════════════════╣
║  Filtered Port: %-42d║
║  Started: %-41s║
║  Status: READY                                             ║
╚════════════════════════════════════════════════════════════╝`, mcpPort, startTime.Format("15:04:05")))

	go func() {
		if err := mcpSrv.Serve(mcpLn); err != nil && err != http.ErrServerClosed {
			logfilter.Debugf("[MCP] Filtered server error: %v", err)
		}
	}()
	if cliAvailable {
		go func() {
			if err := cliSrv.Serve(cliLn); err != nil && err != http.ErrServerClosed {
				logfilter.Debugf("[MCP] Unfiltered server error: %v", err)
			}
		}()
	}

	stopReaper := server.StartIdleReaper()
	defer stopReaper()

	logfilter.Info(fmt.Sprintf("[MCP] Terminal commander started. Export port: %d.", exportPort))
	terminal.SetDeps(deps)
	terminal.StartTerminalCommander(shutdownCh)

	<-shutdownCh
	terminal.WaitTerminalCommander()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mcpSrv.Shutdown(ctx); err != nil {
		logfilter.Debugf("[MCP] Filtered server shutdown error: %v", err)
	}
	if cliAvailable {
		if err := cliSrv.Shutdown(ctx); err != nil {
			logfilter.Debugf("[MCP] Unfiltered server shutdown error: %v", err)
		}
	}
}

func isAddrInUse(err error) bool {
	return strings.Contains(err.Error(), "address already in use")
}
