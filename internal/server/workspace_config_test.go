package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"zen-mcp/internal/gatekeeper"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/shared"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/tools"
	"zen-mcp/internal/toolstate"
)

// wsWithTier makes a workspace root whose project tier declares enabled_tools.
func wsWithTier(t *testing.T, tier string) string {
	t.Helper()
	ws := t.TempDir()
	withProjectTier(t, ws, tier)
	return ws
}

// registerWorkspaceServer is the shape of main.go's newWorkspaceServer: one
// registry per workspace, tools registered and tool states applied for it.
func registerWorkspaceServer(t *testing.T, workspace string) *toolregistry.ToolRegistry {
	t.Helper()
	reg := toolregistry.Create()
	buildWorkspaceServer(t, workspace, reg)
	return reg
}

func buildWorkspaceServer(t *testing.T, workspace string, reg *toolregistry.ToolRegistry) *mcpserver.MCPServer {
	t.Helper()
	deps := tools.Deps{
		Store:                 shared.NewStore(),
		Reg:                   reg,
		Gatekeeper:            gatekeeper.New(shared.NewStore()),
		PendingCollaborations: tools.NewCollaborationRegistry(),
	}
	srv := mcpserver.NewMCPServer(ServerName, ServerVersion,
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithToolFilter(FilterEnabled(reg)),
		mcpserver.WithResourceCapabilities(true, true),
		mcpserver.WithPromptCapabilities(false),
	)
	if err := RegisterAllTools(context.Background(), srv, reg, deps, workspace); err != nil {
		t.Fatalf("RegisterAllTools: %v", err)
	}
	return srv
}

// The per-workspace server cache used to share one registry, so the last
// workspace that registered decided enabled_tools for every workspace still
// cached. Two projects, two states: neither may see the other's.
func TestWorkspaceRegistriesKeepTheirOwnToolStates(t *testing.T) {
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir()
	t.Cleanup(func() { mcpcfg.ProjectRoot = old })
	if err := mcpcfg.Load(); err != nil {
		t.Fatalf("mcpcfg.Load: %v", err)
	}

	a := wsWithTier(t, `{"enabled_tools":{"pool":false}}`)
	b := wsWithTier(t, `{"enabled_tools":{"pool":true}}`)

	regA := registerWorkspaceServer(t, a)
	regB := registerWorkspaceServer(t, b)

	if regA.IsToolEnabled("pool") {
		t.Error("workspace A: the project tier did not disable pool")
	}
	if !regB.IsToolEnabled("pool") {
		t.Error("workspace B: pool was disabled by workspace A's tier")
	}

	// tools/list must agree with the registry each server filters with.
	visible := func(reg *toolregistry.ToolRegistry) map[string]bool {
		out := map[string]bool{}
		for _, tool := range FilterEnabled(reg)(context.Background(), []mcp.Tool{
			{Name: "pool"}, {Name: "capture"},
		}) {
			out[tool.Name] = true
		}
		return out
	}
	if visible(regA)["pool"] {
		t.Error("workspace A advertises pool")
	}
	if !visible(regB)["pool"] {
		t.Error("workspace B does not advertise pool")
	}

	// Switching to B reports what would change on A, and rewrites nothing: the B
	// server applies its own state when it is built.
	changed := toolstate.DiffToolStates(b, regA)
	found := false
	for _, name := range changed {
		if name == "pool" {
			found = true
		}
	}
	if !found {
		t.Errorf("DiffToolStates(B, regA) = %v, want pool listed", changed)
	}
	if regA.IsToolEnabled("pool") {
		t.Error("DiffToolStates mutated the registry")
	}
}

// A cached server was built from the config as it was then, so an edited project
// tier must reach it through the refresh hook - not through a restart, and not
// after the cache happens to evict that workspace.
func TestTierEditReachesTheCachedServer(t *testing.T) {
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir()
	t.Cleanup(func() { mcpcfg.ProjectRoot = old })
	if err := mcpcfg.Load(); err != nil {
		t.Fatalf("mcpcfg.Load: %v", err)
	}

	ws := wsWithTier(t, `{"enabled_tools":{"pool":false}}`)
	built := 0
	regs := map[string]*toolregistry.ToolRegistry{}
	cache := newTestServerCache(4, time.Hour)
	factory := func(id string) *mcpserver.MCPServer {
		built++
		reg := toolregistry.Create()
		regs[id] = reg
		return buildWorkspaceServer(t, id, reg)
	}
	refresh := func(id string) {
		if reg, ok := regs[id]; ok {
			toolstate.ApplyToolStates(id, reg)
		}
	}

	cache.getOrCreate(ws, factory, refresh)
	if regs[ws].IsToolEnabled("pool") {
		t.Fatal("the project tier did not disable pool at registration")
	}

	withProjectTier(t, ws, `{"enabled_tools":{"pool":true}}`)
	cache.getOrCreate(ws, factory, refresh)
	if !regs[ws].IsToolEnabled("pool") {
		t.Error("the cached server kept a tier that has since been edited")
	}
	if built != 1 {
		t.Errorf("servers built = %d, want the one cached instance refreshed", built)
	}
}

func withProjectTier(t *testing.T, workspaceRoot, tier string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workspaceRoot, ".zenmcp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, ".zenmcp", "config.json"), []byte(tier), 0o644); err != nil {
		t.Fatal(err)
	}
}
