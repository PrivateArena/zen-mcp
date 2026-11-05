package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
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
	"zen-mcp/internal/toolstate"
)

func newMcpServer(id string, reg *toolregistry.ToolRegistry, deps tools.Deps) *mcpserver.MCPServer {
	workspace := resolveWorkspace(id)

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

// resolveWorkspace turns a server-cache id into the workspace its tools are bound
// to. "" and "default" mean the configured root, which is what a request that
// carried no workspace hint gets.
func resolveWorkspace(id string) string {
	if id != "" && id != "default" {
		return id
	}
	if ws := mcpcfg.Get().DefaultWorkspaceRoot; ws != "" {
		return ws
	}
	cwd, _ := os.Getwd()
	return cwd
}

// newWorkspaceServer builds the MCP server instance of one workspace with its OWN
// tool registry. One registry shared across the per-workspace server cache meant
// the last workspace registered decided enabled_tools - and held the handler
// closures - for every workspace still cached.
//
// catalog is the process-wide registry the terminal REPL and export_cli read; it
// is mirrored with tool definitions only, never with a handler, which belongs to
// the workspace that built it.
func newWorkspaceServer(id string, catalog *toolregistry.ToolRegistry, deps tools.Deps) (*mcpserver.MCPServer, *toolregistry.ToolRegistry) {
	reg := toolregistry.Create()
	deps.Reg = reg
	srv := newMcpServer(id, reg, deps)
	if catalog != nil {
		for _, entry := range reg.Definitions() {
			catalog.Track(toolregistry.ToolRegistration{
				Name:           entry.Name,
				DefaultEnabled: entry.DefaultEnabled,
				Description:    entry.Description,
				Schema:         entry.Schema,
			})
			catalog.SetToolEnabled(entry.Name, entry.Enabled)
		}
	}
	return srv, reg
}

// workspaceRegistries remembers the registry each cached workspace was built with
// so a config edit reaches the server that already exists for it. It is bounded by
// the server cache it shadows: a cached server that is evicted and rebuilt replaces
// its entry through the factory.
//
// The bound mirrors serverCacheMaxSize in internal/server.
type workspaceRegistries struct {
	mu    sync.Mutex
	limit int
	byID  map[string]*workspaceRegistryEntry
}

type workspaceRegistryEntry struct {
	workspace string
	reg       *toolregistry.ToolRegistry
	used      time.Time
}

func newWorkspaceRegistries(limit int) *workspaceRegistries {
	return &workspaceRegistries{limit: limit, byID: map[string]*workspaceRegistryEntry{}}
}

func (w *workspaceRegistries) put(id, workspace string, reg *toolregistry.ToolRegistry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.byID[id] = &workspaceRegistryEntry{workspace: workspace, reg: reg, used: time.Now()}
	for len(w.byID) > w.limit {
		oldest := ""
		var oldestAt time.Time
		for k, entry := range w.byID {
			if oldest == "" || entry.used.Before(oldestAt) {
				oldest, oldestAt = k, entry.used
			}
		}
		if oldest == "" {
			return
		}
		delete(w.byID, oldest)
	}
}

// refresh is the RouteDeps.RefreshWorkspace hook, called when a request arrives
// for an already-cached workspace. Re-applying that workspace's own tool states
// is what makes an edited enabled_tools - global or project tier - take effect
// without a restart or a cache eviction.
func (w *workspaceRegistries) refresh(id string) {
	w.mu.Lock()
	entry, ok := w.byID[id]
	if ok {
		entry.used = time.Now()
	}
	w.mu.Unlock()
	if !ok {
		return
	}
	toolstate.ApplyToolStates(entry.workspace, entry.reg)
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
	// One registry per cached workspace, remembered per listener so a config edit
	// can be re-applied to the server that is already cached for it.
	filteredRegs := newWorkspaceRegistries(64)
	unfilteredRegs := newWorkspaceRegistries(64)

	gk := gatekeeper.New(store)
	pendingCollabs := tools.NewCollaborationRegistry()
	deps := tools.Deps{
		Store:                 store,
		Reg:                   filteredReg,
		Gatekeeper:            gk,
		PendingCollaborations: pendingCollabs,
		// The wrapper generator lives in internal/terminal (which imports this
		// package), so it reaches the export_cli tool as an injected function.
		ExportCLI: terminal.RunExportCLIText,
	}

	filteredFactory := func(id string) *mcpserver.MCPServer {
		d := deps
		// mcp2cli mode: the agent-facing MCP server exposes no tools; the CLI
		// wrappers target the unfiltered server which keeps the full set.
		d.HideTools = mcpcfg.Get().Mcp2Cli
		srv, reg := newWorkspaceServer(id, filteredReg, d)
		filteredRegs.put(id, resolveWorkspace(id), reg)
		return srv
	}
	unfilteredFactory := func(id string) *mcpserver.MCPServer {
		srv, reg := newWorkspaceServer(id, unfilteredReg, deps)
		unfilteredRegs.put(id, resolveWorkspace(id), reg)
		return srv
	}

	mcpMux := http.NewServeMux()
	server.SetupRoutes(mcpMux, server.RouteDeps{
		CreateMCPServer:       filteredFactory,
		Registry:              filteredReg,
		RefreshWorkspace:      filteredRegs.refresh,
		Shared:                store,
		PendingCollaborations: pendingCollabs,
		StartTime:             startTime,
		Tag:                   fmt.Sprintf("%d", mcpPort),
	})

	cliMux := http.NewServeMux()
	server.SetupRoutes(cliMux, server.RouteDeps{
		CreateMCPServer:       unfilteredFactory,
		Registry:              unfilteredReg,
		RefreshWorkspace:      unfilteredRegs.refresh,
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
