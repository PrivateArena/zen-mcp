package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	mcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/jang/zen-mcp/internal/shared"
	"github.com/jang/zen-mcp/internal/toolregistry"
)

func echoHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ok"}}}, nil
}

func testDeps() (RouteDeps, *http.ServeMux) {
	reg := toolregistry.Create()
	start := time.Date(2026, 8, 5, 2, 22, 34, 570000000, time.UTC)
	deps := RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			s := mcpserver.NewMCPServer("zen-tools", "2.4.1",
				mcpserver.WithToolCapabilities(true),
				mcpserver.WithResourceCapabilities(false, false),
				mcpserver.WithPromptCapabilities(false),
			)
			s.AddTool(mcp.NewTool("echo", mcp.WithString("text")), echoHandler)
			return s
		},
		Registry:              reg,
		Shared:                shared.NewStore(),
		PendingCollaborations: map[string]func(string){},
		StartTime:             start,
		Tag:                   "test",
	}
	mux := http.NewServeMux()
	SetupRoutes(mux, deps)
	return deps, mux
}

func doRequest(mux *http.ServeMux, method, target string, body []byte, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestHealthRoute(t *testing.T) {
	_, mux := testDeps()
	t.Setenv("MCP_TRANSPORT", "")
	rec := doRequest(mux, http.MethodGet, "/health", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["server"] != "zen-tools" || body["version"] != "2.4.1" {
		t.Errorf("health body wrong: %v", body)
	}
	if body["transport"] != "unknown" {
		t.Errorf("transport = %v", body["transport"])
	}
	if body["startedAt"] != "2026-08-05T02:22:34.570Z" {
		t.Errorf("startedAt = %v", body["startedAt"])
	}
	if _, ok := body["uptime"].(float64); !ok {
		t.Errorf("uptime missing: %v", body["uptime"])
	}
}

func TestMcpStatusRoute(t *testing.T) {
	_, mux := testDeps()
	rec := doRequest(mux, http.MethodGet, "/mcp-status", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	info, ok := body["serverInfo"].(map[string]any)
	if !ok || info["name"] != "zen-tools" || info["version"] != "2.4.1" {
		t.Errorf("serverInfo wrong: %v", body)
	}
	if body["status"] != "ok" {
		t.Errorf("status wrong: %v", body)
	}
}

func TestSharedSetGet(t *testing.T) {
	_, mux := testDeps()

	rec := doRequest(mux, http.MethodPost, "/shared/alpha", []byte(`{"value":"beta"}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("set status = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["key"] != "alpha" || body["value"] != "beta" {
		t.Errorf("set body wrong: %v", body)
	}

	rec = doRequest(mux, http.MethodGet, "/shared/alpha", nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "beta") {
		t.Errorf("get wrong: %d %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(mux, http.MethodGet, "/shared/missing", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing key status = %d", rec.Code)
	}
	var errBody map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &errBody)
	if !strings.Contains(errBody["error"].(string), "missing") {
		t.Errorf("error body wrong: %v", errBody)
	}

	rec = doRequest(mux, http.MethodPost, "/shared/alpha", []byte(`{"value":123}`), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-string value should 400, got %d", rec.Code)
	}

	rec = doRequest(mux, http.MethodGet, "/shared", nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "alpha") {
		t.Errorf("get-all wrong: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRejectionRoutes(t *testing.T) {
	_, mux := testDeps()
	cases := []struct {
		method, target, want string
	}{
		{http.MethodGet, "/sse", "SSE sessions not supported in stateless mode"},
		{http.MethodPost, "/sse/message", "SSE sessions not supported in stateless mode"},
		{http.MethodGet, "/mcp", "Streamable HTTP requires POST"},
		{http.MethodDelete, "/mcp", "Session termination not supported in stateless mode"},
	}
	for _, tc := range cases {
		rec := doRequest(mux, tc.method, tc.target, nil, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s status = %d", tc.method, tc.target, rec.Code)
		}
		if strings.TrimSpace(rec.Body.String()) != tc.want {
			t.Errorf("%s %s body = %q", tc.method, tc.target, rec.Body.String())
		}
	}
}

func TestRecoveryRoute(t *testing.T) {
	_, mux := testDeps()
	rec := doRequest(mux, http.MethodGet, "/recovery", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content-type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "MCP AUTO-RECOVERY") {
		t.Errorf("body missing banner: %s", rec.Body.String())
	}
}

func TestCollaborateRoute(t *testing.T) {
	_, mux := testDeps()

	rec := doRequest(mux, http.MethodPost, "/api/collaborate", []byte(`{"path":"/tmp/x"}`), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing id should 400, got %d", rec.Code)
	}

	rec = doRequest(mux, http.MethodPost, "/api/collaborate?id=abc", []byte(`{}`), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing path should 400, got %d", rec.Code)
	}

	rec = doRequest(mux, http.MethodPost, "/api/collaborate?id=nope", []byte(`{"path":"/tmp/x"}`), "application/json")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown session should 404, got %d", rec.Code)
	}
}

func TestCollaborateResolve(t *testing.T) {
	deps, mux := testDeps()
	received := make(chan string, 1)
	deps.PendingCollaborations["abc"] = func(p string) { received <- p }

	rec := doRequest(mux, http.MethodPost, "/api/collaborate?id=abc", []byte(`{"path":"/tmp/shot.png"}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	select {
	case p := <-received:
		if p != "/tmp/shot.png" {
			t.Errorf("callback path = %q", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callback not invoked")
	}
	if _, still := deps.PendingCollaborations["abc"]; still {
		t.Error("pending entry should be removed after resolve")
	}
}

func TestAutoDetectWorkspace(t *testing.T) {
	st := shared.NewStore()

	// 1. initialize rootUri
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"file:///tmp/proj1","capabilities":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(body)))
	if got := autoDetectWorkspace(req, st); got != "/tmp/proj1" {
		t.Errorf("rootUri detect = %q", got)
	}

	// 2. initialize workspaceFolders
	body = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"workspaceFolders":[{"uri":"file:///tmp/proj2"}]}}`
	req = httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(body)))
	if got := autoDetectWorkspace(req, st); got != "/tmp/proj2" {
		t.Errorf("workspaceFolders detect = %q", got)
	}

	// 3. query param
	req = httptest.NewRequest(http.MethodPost, "/mcp?projectPath=/tmp/proj3", nil)
	if got := autoDetectWorkspace(req, st); got != "/tmp/proj3" {
		t.Errorf("query detect = %q", got)
	}

	// 4. shared workspace-root
	st.Set("workspace-root", "/tmp/proj4")
	req = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	if got := autoDetectWorkspace(req, st); got != "/tmp/proj4" {
		t.Errorf("shared detect = %q", got)
	}

	// 5. header
	st.Set("workspace-root", "")
	req = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("X-Workspace-Root", "/tmp/proj5")
	if got := autoDetectWorkspace(req, st); got != "/tmp/proj5" {
		t.Errorf("header detect = %q", got)
	}
}

func mcpInitializeRequest(id int) []byte {
	return []byte(`{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0"}}}`)
}

func TestMCPInitializeHandshake(t *testing.T) {
	_, mux := testDeps()
	rec := doRequest(mux, http.MethodPost, "/mcp", mcpInitializeRequest(1), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize status = %d: %s", rec.Code, rec.Body.String())
	}
	var msg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	if msg["jsonrpc"] != "2.0" || msg["id"] != float64(1) {
		t.Errorf("jsonrpc envelope wrong: %v", msg)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	info, ok := result["serverInfo"].(map[string]any)
	if !ok || info["name"] != "zen-tools" || info["version"] != "2.4.1" {
		t.Errorf("serverInfo wrong: %v", result)
	}
	if _, ok := result["protocolVersion"].(string); !ok {
		t.Errorf("protocolVersion missing: %v", result)
	}
	capabilities, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities missing: %v", result)
	}
	if _, ok := capabilities["tools"]; !ok {
		t.Errorf("tools capability missing: %v", capabilities)
	}
}

func TestMCPToolsList(t *testing.T) {
	_, mux := testDeps()
	rec := doRequest(mux, http.MethodPost, "/mcp", mcpInitializeRequest(1), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize: %d", rec.Code)
	}
	rec = doRequest(mux, http.MethodPost, "/mcp",
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d: %s", rec.Code, rec.Body.String())
	}
	var msg map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &msg)
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("no tools listed: %v", result)
	}
	found := false
	for _, tl := range tools {
		if m, ok := tl.(map[string]any); ok && m["name"] == "echo" {
			found = true
		}
	}
	if !found {
		t.Errorf("echo tool not listed: %v", tools)
	}
}

func TestMCPBadJSON(t *testing.T) {
	_, mux := testDeps()
	rec := doRequest(mux, http.MethodPost, "/mcp", []byte("{not json"), "application/json")
	// mcp-go returns a JSON-RPC parse error response, not a 500.
	if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Errorf("parse error status = %d", rec.Code)
	}
}

func TestReadBodyOnce(t *testing.T) {
	_, mux := testDeps()
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"file:///tmp/p"}}`)
	rec := doRequest(mux, http.MethodPost, "/mcp?projectPath=/tmp/q", body, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize after body rewind failed: %d: %s", rec.Code, rec.Body.String())
	}
}
