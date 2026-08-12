package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"zen-mcp/internal/gatekeeper"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/shared"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/toolstate"
	"zen-mcp/internal/tools"
)

// toolsModeMux builds the routes harness with every tool registered through
// RegisterAllTools. When hide is true it mirrors the agent-facing MCP server
// in mcp2cli mode: deps.HideTools is set and no tools capability is advertised.
func toolsModeMux(t *testing.T, hide bool) (*toolregistry.ToolRegistry, *http.ServeMux) {
	t.Helper()
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir()
	t.Cleanup(func() { mcpcfg.ProjectRoot = old })
	if err := mcpcfg.Load(); err != nil {
		t.Fatalf("mcpcfg.Load: %v", err)
	}

	reg := toolregistry.Create()
	deps := tools.Deps{
		Store:                 shared.NewStore(),
		Reg:                   reg,
		Gatekeeper:            gatekeeper.New(shared.NewStore()),
		PendingCollaborations: tools.NewCollaborationRegistry(),
		HideTools:             hide,
	}

	mux := http.NewServeMux()
	SetupRoutes(mux, RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			s := mcpserver.NewMCPServer(ServerName, ServerVersion,
				mcpserver.WithToolCapabilities(!hide),
				mcpserver.WithToolFilter(FilterEnabled(reg)),
				mcpserver.WithResourceCapabilities(true, true),
				mcpserver.WithPromptCapabilities(false),
			)
			if err := RegisterAllTools(context.Background(), s, reg, deps, ""); err != nil {
				t.Fatalf("RegisterAllTools: %v", err)
			}
			return s
		},
		Registry:              reg,
		Shared:                shared.NewStore(),
		PendingCollaborations: tools.NewCollaborationRegistry(),
		StartTime:             time.Now(),
		Tag:                   "test",
	})
	return reg, mux
}

// TestRegisterAllToolsHiddenHidesAllTools pins the mcp2cli contract: the
// agent-facing server returns an empty tools/list, tools/call is rejected, the
// registry tracks every tool as disabled and the catalog carries no tool
// sections — so no tool schema ever reaches the agent.
func TestRegisterAllToolsHiddenHidesAllTools(t *testing.T) {
	reg, mux := toolsModeMux(t, true)

	rec := doRequest(mux, http.MethodPost, "/mcp", mcpInitializeRequest(1), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(mux, http.MethodPost, "/mcp",
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d: %s", rec.Code, rec.Body.String())
	}
	var listMsg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listMsg); err != nil {
		t.Fatalf("tools/list body: %v", err)
	}
	lresult, _ := listMsg["result"].(map[string]any)
	if toolsArr, _ := lresult["tools"].([]any); len(toolsArr) != 0 {
		t.Errorf("tools/list must be empty, got %d tools: %v", len(toolsArr), toolsArr)
	}

	names := reg.ListToolNames()
	if len(names) == 0 {
		t.Fatal("registry must still track the full tool set")
	}
	for _, n := range names {
		if reg.IsToolEnabled(n) {
			t.Errorf("tool %q must be disabled in mcp2cli mode", n)
		}
	}

	if catalog := buildToolCatalog(reg); strings.Contains(catalog, "## ") {
		t.Errorf("catalog must not contain tool sections, got:\n%s", catalog)
	}
}

// TestRegisterAllToolsHiddenRejectsToolCall pins that a hidden tool is not
// callable: mcp2cli mode forces the agent onto the zen-* CLI wrappers, so a
// direct tools/call must fail instead of silently working.
func TestRegisterAllToolsHiddenRejectsToolCall(t *testing.T) {
	_, mux := toolsModeMux(t, true)
	doRequest(mux, http.MethodPost, "/mcp", mcpInitializeRequest(1), "application/json")

	rec := doRequest(mux, http.MethodPost, "/mcp",
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"shell","arguments":{}}}`),
		"application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/call status = %d: %s", rec.Code, rec.Body.String())
	}
	var msg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatalf("tools/call body: %v", err)
	}
	if _, hasResult := msg["result"]; hasResult {
		t.Errorf("hidden server must reject tools/call, got a result: %s", rec.Body.String())
	}
	if _, hasError := msg["error"]; !hasError {
		t.Errorf("hidden server must return a JSON-RPC error, got: %s", rec.Body.String())
	}
}

// TestRegisterAllToolsVisibleKeepsTools guards backward compatibility: with
// deps.HideTools unset the server still lists and enables every tool.
func TestRegisterAllToolsVisibleKeepsTools(t *testing.T) {
	reg, mux := toolsModeMux(t, false)
	doRequest(mux, http.MethodPost, "/mcp", mcpInitializeRequest(1), "application/json")

	rec := doRequest(mux, http.MethodPost, "/mcp",
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d: %s", rec.Code, rec.Body.String())
	}
	var msg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatalf("tools/list body: %v", err)
	}
	result, _ := msg["result"].(map[string]any)
	if toolsArr, _ := result["tools"].([]any); len(toolsArr) == 0 {
		t.Fatalf("visible server must list tools, got: %s", rec.Body.String())
	}
	for _, n := range reg.ListToolNames() {
		if !reg.IsToolEnabled(n) {
			t.Errorf("tool %q must stay enabled on the visible server", n)
		}
	}
}

// TestRegisterAllToolsHiddenStaysHiddenAfterToolstate pins the edge case where
// the workspace tool re-applies toolstate to the shared registry (reachable
// via the terminal's zen-workspace command, which shares the filteredReg used
// by the hidden server). Even if the registry re-enables entries, nothing may
// resurrect on the hidden MCP server because no tool was ever registered onto
// it.
func TestRegisterAllToolsHiddenStaysHiddenAfterToolstate(t *testing.T) {
	reg, mux := toolsModeMux(t, true)
	doRequest(mux, http.MethodPost, "/mcp", mcpInitializeRequest(1), "application/json")

	toolstate.ApplyToolStates("", reg)

	rec := doRequest(mux, http.MethodPost, "/mcp",
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`), "application/json")
	var msg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatalf("tools/list body: %v", err)
	}
	lresult, _ := msg["result"].(map[string]any)
	if toolsArr, _ := lresult["tools"].([]any); len(toolsArr) != 0 {
		t.Errorf("tools/list must stay empty after toolstate re-apply, got %d tools", len(toolsArr))
	}

	rec = doRequest(mux, http.MethodPost, "/mcp",
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"shell","arguments":{}}}`),
		"application/json")
	var callMsg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &callMsg); err != nil {
		t.Fatalf("tools/call body: %v", err)
	}
	if _, hasResult := callMsg["result"]; hasResult {
		t.Errorf("tools/call must stay rejected after toolstate re-apply: %s", rec.Body.String())
	}
}
