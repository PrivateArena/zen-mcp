package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"zen-mcp/internal/shared"
	"zen-mcp/internal/toolregistry"
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

func TestServerCacheReusesSameInstance(t *testing.T) {
	cache := &serverCache{servers: make(map[string]*mcpserver.MCPServer)}
	factory := func(id string) *mcpserver.MCPServer {
		return mcpserver.NewMCPServer("test", "1.0")
	}

	a := cache.getOrCreate("ws-a", factory, nil)
	b := cache.getOrCreate("ws-a", factory, nil)
	if a != b {
		t.Fatal("expected same server instance for same logicalID")
	}
}

func TestServerCacheCreatesSeparateInstances(t *testing.T) {
	cache := &serverCache{servers: make(map[string]*mcpserver.MCPServer)}
	factory := func(id string) *mcpserver.MCPServer {
		return mcpserver.NewMCPServer("test-"+id, "1.0")
	}

	a := cache.getOrCreate("ws-a", factory, nil)
	b := cache.getOrCreate("ws-b", factory, nil)
	if a == b {
		t.Fatal("expected different server instances for different logicalIDs")
	}
}

func TestServerCacheConcurrentAccess(t *testing.T) {
	cache := &serverCache{servers: make(map[string]*mcpserver.MCPServer)}
	factory := func(id string) *mcpserver.MCPServer {
		return mcpserver.NewMCPServer("test-concurrent", "1.0")
	}

	const goroutines = 50
	const repeats = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < repeats; j++ {
				srv := cache.getOrCreate("shared-ws", factory, nil)
				if srv == nil {
					t.Error("nil server returned")
				}
			}
		}()
	}
	wg.Wait()
}

func TestServerCacheFactoryCalledOncePerLogicalID(t *testing.T) {
	cache := &serverCache{servers: make(map[string]*mcpserver.MCPServer)}
	count := 0
	factory := func(id string) *mcpserver.MCPServer {
		count++
		return mcpserver.NewMCPServer("test-count-"+id, "1.0")
	}

	for i := 0; i < 5; i++ {
		cache.getOrCreate("ws-once", factory, nil)
	}
	if count != 1 {
		t.Fatalf("factory called %d times, expected 1", count)
	}
}

func TestServerCacheConcurrentDifferentIDs(t *testing.T) {
	cache := &serverCache{servers: make(map[string]*mcpserver.MCPServer)}
	factory := func(id string) *mcpserver.MCPServer {
		return mcpserver.NewMCPServer("test-"+id, "1.0")
	}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		id := fmt.Sprintf("ws-%d", i)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				cache.getOrCreate(id, factory, nil)
			}
		}()
	}
	wg.Wait()
}

func TestPostMCPUsesCachedServer(t *testing.T) {
	_, mux := testDeps()

	req := func(body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		return rec
	}

	first := req(mcpInitializeRequest(1))
	if first.Code != http.StatusOK {
		t.Fatalf("initialize failed: %d: %s", first.Code, first.Body.String())
	}

	second := req([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	if second.Code != http.StatusOK {
		t.Fatalf("tools/list failed: %d: %s", second.Code, second.Body.String())
	}

	var msg map[string]any
	_ = json.Unmarshal(second.Body.Bytes(), &msg)
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("no tools listed: %v", result)
	}
}

func TestPostMCPLongRunningRequestDoesNotBlockNewRequests(t *testing.T) {
	longCallStarted := make(chan struct{})
	longCallFinished := make(chan struct{})

	deps := RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			s := mcpserver.NewMCPServer("zen-tools", "2.4.1",
				mcpserver.WithToolCapabilities(true),
				mcpserver.WithResourceCapabilities(false, false),
				mcpserver.WithPromptCapabilities(false),
			)
			s.AddTool(mcp.NewTool("long-call", mcp.WithString("duration")), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				durationMs := 1500
				if args, ok := req.GetArguments()["duration"].(float64); ok {
					durationMs = int(args)
				}
				select {
				case longCallStarted <- struct{}{}:
				default:
				}
				time.Sleep(time.Duration(durationMs) * time.Millisecond)
				select {
				case longCallFinished <- struct{}{}:
				default:
				}
				return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "long-done"}}}, nil
			})
			s.AddTool(mcp.NewTool("quick", mcp.WithString("text")), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				text, _ := req.GetArguments()["text"].(string)
				if text == "" {
					text = "ok"
				}
				return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}}}, nil
			})
			return s
		},
		Registry:              toolregistry.Create(),
		Shared:                shared.NewStore(),
		PendingCollaborations: map[string]func(string){},
		StartTime:             time.Now(),
		Tag:                   "test",
	}
	mux := http.NewServeMux()
	SetupRoutes(mux, deps)

	doReq := func(body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		return rec
	}

	initRec := doReq(mcpInitializeRequest(1))
	if initRec.Code != http.StatusOK {
		t.Fatalf("initialize failed: %d: %s", initRec.Code, initRec.Body.String())
	}

	longBody := []byte(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"long-call","arguments":{"duration":1500}}}`)
	go func() {
		doReq(longBody)
	}()

	select {
	case <-longCallStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("long-call did not start")
	}

	type subTest struct {
		name string
		run  func(t *testing.T)
	}
	tests := []subTest{
		{
			name: "another client initialize",
			run: func(t *testing.T) {
				rec := doReq(mcpInitializeRequest(2))
				if rec.Code != http.StatusOK {
					t.Errorf("second initialize failed: %d: %s", rec.Code, rec.Body.String())
				}
			},
		},
		{
			name: "another client tools/list",
			run: func(t *testing.T) {
				rec := doReq([]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`))
				if rec.Code != http.StatusOK {
					t.Errorf("tools/list during long call failed: %d: %s", rec.Code, rec.Body.String())
				}
			},
		},
		{
			name: "another client tool call",
			run: func(t *testing.T) {
				rec := doReq([]byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"quick","arguments":{"text":"hello"}}}`))
				if rec.Code != http.StatusOK {
					t.Errorf("quick tool call during long call failed: %d: %s", rec.Code, rec.Body.String())
				}
			},
		},
		{
			name: "ping during long call",
			run: func(t *testing.T) {
				rec := doReq([]byte(`{"jsonrpc":"2.0","id":5,"method":"ping","params":{}}`))
				if rec.Code != http.StatusOK {
					t.Errorf("ping during long call failed: %d: %s", rec.Code, rec.Body.String())
				}
			},
		},
	}

	var clientWg sync.WaitGroup
	clientWg.Add(len(tests))
	errors := make(chan struct{}, len(tests))

	for _, tc := range tests {
		tc := tc
		go func() {
			defer clientWg.Done()
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				tc.run(t)
			}()

			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				t.Errorf("%s: request timed out", tc.name)
				select {
				case errors <- struct{}{}:
				default:
				}
			}
		}()
	}

	clientWg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Fatalf("%d concurrent requests failed during long call", len(errors))
	}

	select {
	case <-longCallFinished:
	case <-time.After(3 * time.Second):
		t.Fatal("long-call did not finish")
	}
}

func TestPostMCPExtremeConcurrency200Clients(t *testing.T) {
	factoryCalls := 0
	deps := RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			factoryCalls++
			s := mcpserver.NewMCPServer("zen-tools", "2.4.1",
				mcpserver.WithToolCapabilities(true),
				mcpserver.WithResourceCapabilities(false, false),
				mcpserver.WithPromptCapabilities(false),
			)
			s.AddTool(mcp.NewTool("quick", mcp.WithString("text")), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ok"}}}, nil
			})
			return s
		},
		Registry:              toolregistry.Create(),
		Shared:                shared.NewStore(),
		PendingCollaborations: map[string]func(string){},
		StartTime:             time.Now(),
		Tag:                   "test",
	}
	mux := http.NewServeMux()
	SetupRoutes(mux, deps)

	const clients = 200
	var wg sync.WaitGroup
	wg.Add(clients)
	errors := make(chan struct{}, clients)

	doReq := func(id int) {
		defer wg.Done()
		var body []byte
		switch id % 3 {
		case 0:
			body = fmt.Appendf([]byte{}, `{"jsonrpc":"2.0","id":%d,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{}}}`, id)
		case 1:
			body = []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
		case 2:
			body = fmt.Appendf([]byte{}, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"quick","arguments":{"text":"hello"}}}`, id)
		}
		r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			select {
			case errors <- struct{}{}:
			default:
			}
		}
	}

	for i := 0; i < clients; i++ {
		go doReq(i)
	}
	wg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Fatalf("%d requests failed under 200-client concurrency", len(errors))
	}
	if factoryCalls != 1 {
		t.Fatalf("factory called %d times, expected 1 under 200-client concurrency", factoryCalls)
	}
}

func TestPostMCP500ConcurrentToolsList(t *testing.T) {
	factoryCalls := 0
	deps := RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			factoryCalls++
			return mcpserver.NewMCPServer("zen-tools", "2.4.1",
				mcpserver.WithToolCapabilities(true),
				mcpserver.WithResourceCapabilities(false, false),
				mcpserver.WithPromptCapabilities(false),
			)
		},
		Registry:              toolregistry.Create(),
		Shared:                shared.NewStore(),
		PendingCollaborations: map[string]func(string){},
		StartTime:             time.Now(),
		Tag:                   "test",
	}
	mux := http.NewServeMux()
	SetupRoutes(mux, deps)

	const clients = 500
	var wg sync.WaitGroup
	wg.Add(clients)
	errors := make(chan struct{}, clients)

	for i := 0; i < clients; i++ {
		go func(id int) {
			defer wg.Done()
			body := fmt.Appendf([]byte{}, `{"jsonrpc":"2.0","id":%d,"method":"tools/list","params":{}}`, id)
			r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, r)
			if rec.Code != http.StatusOK {
				select {
				case errors <- struct{}{}:
				default:
				}
			}
		}(i)
	}
	wg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Fatalf("%d requests failed under 500-client tools/list concurrency", len(errors))
	}
	if factoryCalls != 1 {
		t.Fatalf("factory called %d times, expected 1 under 500-client concurrency", factoryCalls)
	}
}

func TestPostMCPLongCallWith200ConcurrentClients(t *testing.T) {
	longCallStarted := make(chan struct{})
	longCallFinished := make(chan struct{})

	factoryCalls := 0
	deps := RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			factoryCalls++
			s := mcpserver.NewMCPServer("zen-tools", "2.4.1",
				mcpserver.WithToolCapabilities(true),
				mcpserver.WithResourceCapabilities(false, false),
				mcpserver.WithPromptCapabilities(false),
			)
			s.AddTool(mcp.NewTool("long-call", mcp.WithString("duration")), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				durationMs := 5000
				if args, ok := req.GetArguments()["duration"].(float64); ok {
					durationMs = int(args)
				}
				select {
				case longCallStarted <- struct{}{}:
				default:
				}
				time.Sleep(time.Duration(durationMs) * time.Millisecond)
				select {
				case longCallFinished <- struct{}{}:
				default:
				}
				return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "long-done"}}}, nil
			})
			s.AddTool(mcp.NewTool("quick", mcp.WithString("text")), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				text, _ := req.GetArguments()["text"].(string)
				if text == "" {
					text = "ok"
				}
				return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}}}, nil
			})
			return s
		},
		Registry:              toolregistry.Create(),
		Shared:                shared.NewStore(),
		PendingCollaborations: map[string]func(string){},
		StartTime:             time.Now(),
		Tag:                   "test",
	}
	mux := http.NewServeMux()
	SetupRoutes(mux, deps)

	doReq := func(body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		return rec
	}

	initRec := doReq(mcpInitializeRequest(1))
	if initRec.Code != http.StatusOK {
		t.Fatalf("initialize failed: %d: %s", initRec.Code, initRec.Body.String())
	}

	longBody := []byte(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"long-call","arguments":{"duration":5000}}}`)
	go func() {
		doReq(longBody)
	}()

	select {
	case <-longCallStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("long-call did not start")
	}

	const clients = 200
	var wg sync.WaitGroup
	wg.Add(clients)
	errors := make(chan struct{}, clients)

	for i := 0; i < clients; i++ {
		go func(id int) {
			defer wg.Done()
			var body []byte
			switch id % 4 {
			case 0:
				body = fmt.Appendf([]byte{}, `{"jsonrpc":"2.0","id":%d,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{}}}`, id)
			case 1:
				body = []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
			case 2:
				body = fmt.Appendf([]byte{}, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"quick","arguments":{"text":"hello"}}}`, id)
			case 3:
				body = []byte(`{"jsonrpc":"2.0","id":5,"method":"ping","params":{}}`)
			}
			r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, r)
			if rec.Code != http.StatusOK {
				select {
				case errors <- struct{}{}:
				default:
				}
			}
		}(i)
	}
	wg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Fatalf("%d requests failed during 5s long-call with 200 concurrent clients", len(errors))
	}

	select {
	case <-longCallFinished:
	case <-time.After(6 * time.Second):
		t.Fatal("long-call did not finish")
	}

	if factoryCalls != 1 {
		t.Fatalf("factory called %d times, expected 1", factoryCalls)
	}
}
