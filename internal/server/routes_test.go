package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	_ "modernc.org/sqlite"

	"zen-mcp/internal/gatekeeper"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/shared"
	"zen-mcp/internal/telemetry"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/toolresponse"
	"zen-mcp/internal/tools"
)

func echoHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ok"}}}, nil
}


// setRequestAbortObserver registers a callback invoked whenever an in-flight
// MCP request is cancelled by the client. Intended for tests; nil by default.
func setRequestAbortObserver(fn func(method string, elapsedMs int64, reason string)) {
	onRequestAbort = fn
}

// autoDetectWorkspace resolves a workspace root from initialize params, query,
// shared state, then headers — in that order, matching routes.ts.
func autoDetectWorkspace(r *http.Request, st *shared.Store) string {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	r.Body = io.NopCloser(bytes.NewReader(body))

	var msg rpcMessage
	_ = json.Unmarshal(body, &msg)
	return detectWorkspace(msg, r, st)
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
		PendingCollaborations: tools.NewCollaborationRegistry(),
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
		wantCode             int
	}{
		{http.MethodGet, "/sse", "SSE sessions not supported in stateless mode", http.StatusBadRequest},
		{http.MethodPost, "/sse/message", "SSE sessions not supported in stateless mode", http.StatusBadRequest},
		{http.MethodGet, "/mcp", "Method Not Allowed: Streamable HTTP requires POST", http.StatusMethodNotAllowed},
		{http.MethodDelete, "/mcp", "Session termination not supported in stateless mode", http.StatusBadRequest},
	}
	for _, tc := range cases {
		rec := doRequest(mux, tc.method, tc.target, nil, "")
		if rec.Code != tc.wantCode {
			t.Errorf("%s %s status = %d, want %d", tc.method, tc.target, rec.Code, tc.wantCode)
		}
		if strings.TrimSpace(rec.Body.String()) != tc.want {
			t.Errorf("%s %s body = %q", tc.method, tc.target, rec.Body.String())
		}
	}
}

func TestGetMcpReturns405WithAllowHeader(t *testing.T) {
	_, mux := testDeps()
	rec := doRequest(mux, http.MethodGet, "/mcp", nil, "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /mcp status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "POST" {
		t.Errorf("Allow header = %q, want POST", allow)
	}
}

// TestClientDisconnectReachesWrappedHandler pins that a client disconnect
// cancels r.Context() and that cancellation propagates through mcp-go
// (streamable HTTP -> HandleMessage) into WrapHandlerWithTimeout's ctx.Done
// branch, so a client-side deadline surfaces as an "interrupted" tool result
// instead of the handler running to completion against a dead connection.
func TestClientDisconnectReachesWrappedHandler(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	inner := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		close(entered)
		<-ctx.Done()
		<-release
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "late"}}}, nil
	}
	wrapped := WrapHandlerWithTimeout("blocker", inner, func(string) time.Duration { return time.Hour })

	deps := RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			s := mcpserver.NewMCPServer("zen-tools", "2.4.1",
				mcpserver.WithToolCapabilities(true),
				mcpserver.WithResourceCapabilities(false, false),
				mcpserver.WithPromptCapabilities(false),
			)
			tool := mcp.NewTool("blocker", mcp.WithString("x"))
			s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return wrapped(ctx, req)
			})
			return s
		},
		Registry:              toolregistry.Create(),
		Shared:                shared.NewStore(),
		PendingCollaborations: tools.NewCollaborationRegistry(),
		StartTime:             time.Now(),
		Tag:                   "test",
	}
	mux := http.NewServeMux()
	SetupRoutes(mux, deps)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"blocker","arguments":{"x":"y"}}}`)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)

	clientDone := make(chan error, 1)
	go func() {
		_, err := ts.Client().Do(req)
		clientDone <- err
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("wrapped handler did not start")
	}

	select {
	case err := <-clientDone:
		t.Logf("client saw error after deadline: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("client request did not return after its own deadline")
	}

	close(release)
	time.Sleep(200 * time.Millisecond)
}

// TestClientAbortTelemetryFaithful pins the whole faithfulness fix: when a
// client aborts a long tool call, telemetry receives EXACTLY ONE failure row
// for the client-visible abort (with a real duration_ms metric), and the
// orphaned background handler's later result must NOT emit a second row.
func TestClientAbortTelemetryFaithful(t *testing.T) {
	// Isolate telemetry to a temp dir; reset any cached DB handle first.
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir()
	t.Cleanup(func() {
		mcpcfg.ProjectRoot = old
		_ = telemetry.Close()
	})
	_ = telemetry.Close()

	started := make(chan struct{})
	inner := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		select {
		case <-started:
		default:
			close(started)
		}
		// Long, ctx-agnostic work (mirrors HandleShellAction -> exec.Run).
		time.Sleep(800 * time.Millisecond)
		return toolresponse.WrapSuccess(ctx, "blocker", map[string]any{
			"command":  "sleep 900",
			"stdout":   "partial",
			"stderr":   "",
			"exitCode": 0,
			"timedOut": "hard",
			"timeout":  nil,
		}, time.Now()), nil
	}
	wrapped := WrapHandlerWithTimeout("blocker", inner, func(string) time.Duration { return time.Hour })

	deps := RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			s := mcpserver.NewMCPServer("zen-tools", "2.4.1",
				mcpserver.WithToolCapabilities(true),
				mcpserver.WithResourceCapabilities(false, false),
				mcpserver.WithPromptCapabilities(false),
			)
			s.AddTool(mcp.NewTool("blocker", mcp.WithString("x")), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return wrapped(ctx, req)
			})
			return s
		},
		Registry:              toolregistry.Create(),
		Shared:                shared.NewStore(),
		PendingCollaborations: tools.NewCollaborationRegistry(),
		StartTime:             time.Now(),
		Tag:                   "test",
	}
	mux := http.NewServeMux()
	SetupRoutes(mux, deps)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", bytes.NewReader(
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"blocker","arguments":{"x":"y"}}}`)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)

	clientDone := make(chan error, 1)
	go func() {
		_, err := ts.Client().Do(req)
		clientDone <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("blocker handler did not start")
	}
	select {
	case err := <-clientDone:
		if err == nil {
			t.Log("client call surfaced a response before cancel?")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not abort after its own deadline")
	}
	// Let the orphaned background handler finish so suppression is exercised.
	time.Sleep(900 * time.Millisecond)

	_ = telemetry.Close()
	d, err := sql.Open("sqlite", "file:"+filepath.Join(mcpcfg.ProjectRoot, "telemetry", "telemetry.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	rows, err := d.Query(`SELECT tool, success, error_message, duration_ms FROM tool_calls ORDER BY id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var tool string
		var success int
		var msg sql.NullString
		var dur sql.NullInt64
		if err := rows.Scan(&tool, &success, &msg, &dur); err != nil {
			t.Fatal(err)
		}
		durStr := "<nil>"
		if dur.Valid {
			durStr = strconv.FormatInt(dur.Int64, 10)
		}
		got = append(got, fmt.Sprintf("%s|%d|%s|%s", tool, success, msg.String, durStr))
	}
	if len(got) != 1 {
		t.Fatalf("telemetry rows = %v, want exactly 1 (abort), got %d", got, len(got))
	}
	if !strings.HasPrefix(got[0], "blocker|0|Tool 'blocker' interrupted: client_disconnect_cancel|") {
		t.Errorf("unexpected row: %q", got[0])
	}
}

// TestToolCallRecordsActionTelemetry pins the actions-telemetry faithfulness
// fix: the production registration boundary (RegisterAllTools) must inject the
// request ToolContext so that every tools/call carries its action into the
// telemetry action column. Before the fix, WithToolContext was only applied by
// the test-only WrapHandlerWithTimeout wrapper, so the action was NULL for
// nearly all tools (e.g. codegraph: 789 calls but only 4 actionable rows).
func TestToolCallRecordsActionTelemetry(t *testing.T) {
	old := mcpcfg.ProjectRoot
	mcpcfg.ProjectRoot = t.TempDir()
	t.Cleanup(func() {
		mcpcfg.ProjectRoot = old
		_ = telemetry.Close()
	})
	_ = telemetry.Close()

	ws := filepath.Join(mcpcfg.ProjectRoot, "workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	reg := toolregistry.Create()
	gk := gatekeeper.New(shared.NewStore())
	deps := tools.Deps{
		Store:                 shared.NewStore(),
		Reg:                   reg,
		Gatekeeper:            gk,
		PendingCollaborations: tools.NewCollaborationRegistry(),
	}
	depsWithReg := deps
	depsWithReg.Reg = reg

	depsRoute := RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			s := mcpserver.NewMCPServer("zen-tools", "2.4.1",
				mcpserver.WithToolCapabilities(true),
				mcpserver.WithResourceCapabilities(false, false),
				mcpserver.WithPromptCapabilities(false),
			)
			if err := RegisterAllTools(context.Background(), s, reg, depsWithReg, ws); err != nil {
				panic(err)
			}
			return s
		},
		Registry:              reg,
		Shared:                shared.NewStore(),
		PendingCollaborations: tools.NewCollaborationRegistry(),
		StartTime:             time.Now(),
		Tag:                   "test",
	}
	mux := http.NewServeMux()
	SetupRoutes(mux, depsRoute)

	do := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(body)))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		return rec
	}
	do(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{}}}`)

	res := do(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"skill","arguments":{"action":"list"}}}`)
	if res.Code != http.StatusOK {
		t.Fatalf("tools/call status = %d, body = %s", res.Code, res.Body.String())
	}

	_ = telemetry.Close()
	d, err := sql.Open("sqlite", "file:"+filepath.Join(mcpcfg.ProjectRoot, "telemetry", "telemetry.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var rows []string
	q, err := d.Query(`SELECT tool, action, success, error_message FROM tool_calls ORDER BY id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer q.Close()
	for q.Next() {
		var tool, action string
		var success int
		var msg sql.NullString
		if err := q.Scan(&tool, &action, &success, &msg); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, fmt.Sprintf("%s|%s|%d|%s", tool, action, success, msg.String))
	}
	var skillRows []string
	for _, r := range rows {
		if strings.HasPrefix(r, "skill|") {
			skillRows = append(skillRows, r)
		}
	}
	if len(skillRows) == 0 {
		t.Fatalf("no skill telemetry rows recorded, got %v", rows)
	}
	for _, r := range skillRows {
		if !strings.HasPrefix(r, "skill|list|") {
			t.Errorf("skill row did not record action 'list': %q", r)
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
	deps.PendingCollaborations.Register("abc", func(p string) { received <- p })

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
	if deps.PendingCollaborations.Contains("abc") {
		t.Error("pending entry should be removed after resolve")
	}

	// A duplicate POST must not double-resolve (F11): return 404, no second callback.
	rec = doRequest(mux, http.MethodPost, "/api/collaborate?id=abc", []byte(`{"path":"/tmp/shot.png"}`), "application/json")
	if rec.Code != http.StatusNotFound {
		t.Errorf("duplicate resolve status = %d, want 404", rec.Code)
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

func TestPostMCPNoAbortOnNormalCompletion(t *testing.T) {
	_, mux := testDeps()
	var mu sync.Mutex
	aborts := 0
	setRequestAbortObserver(func(string, int64, string) {
		mu.Lock()
		aborts++
		mu.Unlock()
	})
	t.Cleanup(func() { setRequestAbortObserver(nil) })

	rec := doRequest(mux, http.MethodPost, "/mcp", mcpInitializeRequest(1), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize failed: %d: %s", rec.Code, rec.Body.String())
	}

	mu.Lock()
	n := aborts
	mu.Unlock()
	if n != 0 {
		t.Fatalf("abort observer fired %d times on a normally-completing request, want 0", n)
	}
}

func TestPostMCPRequestAbortLogged(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	deps := RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			s := mcpserver.NewMCPServer("zen-tools", "2.4.1",
				mcpserver.WithToolCapabilities(true),
				mcpserver.WithResourceCapabilities(false, false),
				mcpserver.WithPromptCapabilities(false),
			)
			s.AddTool(mcp.NewTool("block", mcp.WithString("x")), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				close(started)
				<-release
				return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "done"}}}, nil
			})
			return s
		},
		Registry:              toolregistry.Create(),
		Shared:                shared.NewStore(),
		PendingCollaborations: tools.NewCollaborationRegistry(),
		StartTime:             time.Now(),
		Tag:                   "test",
	}
	mux := http.NewServeMux()
	SetupRoutes(mux, deps)

	var mu sync.Mutex
	aborts := make([]string, 0)
	setRequestAbortObserver(func(method string, _ int64, reason string) {
		if reason == "" {
			t.Error("abort observer received empty reason")
		}
		mu.Lock()
		aborts = append(aborts, method)
		mu.Unlock()
	})
	t.Cleanup(func() { setRequestAbortObserver(nil) })

	ctx, cancel := context.WithCancel(context.Background())
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"block","arguments":{}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	serveDone := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(serveDone)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking handler did not start")
	}
	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(aborts)
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	select {
	case <-serveDone:
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not finish after handler release")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(aborts) != 1 {
		t.Fatalf("abort observer fired %v, want exactly [tools/call]", aborts)
	}
	if aborts[0] != "tools/call" {
		t.Errorf("abort method = %q, want tools/call", aborts[0])
	}
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

// countingBody tracks total bytes consumed to pin F4: postMCP must materialize
// the request body exactly once, then hand a rewound buffer to the mcp-go
// handler (which reads the buffer, not the original body).
type countingBody struct {
	io.ReadCloser
	total int64
}

func (c *countingBody) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.total += int64(n)
	return n, err
}

func TestPostMCPReadsBodyOnce(t *testing.T) {
	_, mux := testDeps()
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"file:///tmp/p"}}`)
	cb := &countingBody{ReadCloser: io.NopCloser(bytes.NewReader(body))}
	req := httptest.NewRequest(http.MethodPost, "/mcp", cb)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize: %d: %s", rec.Code, rec.Body.String())
	}
	if cb.total != int64(len(body)) {
		t.Errorf("request body consumed %d bytes, want %d (materialized more than once, F4)", cb.total, len(body))
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
		PendingCollaborations: tools.NewCollaborationRegistry(),
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
		PendingCollaborations: tools.NewCollaborationRegistry(),
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
		PendingCollaborations: tools.NewCollaborationRegistry(),
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
		PendingCollaborations: tools.NewCollaborationRegistry(),
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

func newTestServerCache(maxSize int, ttl time.Duration) *serverCache {
	return &serverCache{
		servers:  make(map[string]*mcpserver.MCPServer),
		lastUsed: make(map[string]time.Time),
		handlers: make(map[string]*mcpserver.StreamableHTTPServer),
		maxSize:  maxSize,
		ttl:      ttl,
	}
}

func TestServerCacheCapEvictsLRU(t *testing.T) {
	cache := newTestServerCache(2, time.Hour)
	factory := func(id string) *mcpserver.MCPServer {
		return mcpserver.NewMCPServer("cap-"+id, "1.0")
	}

	cache.getOrCreate("a", factory, nil)
	time.Sleep(time.Millisecond)
	cache.getOrCreate("b", factory, nil)
	time.Sleep(time.Millisecond)
	cache.getOrCreate("c", factory, nil)

	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if len(cache.servers) != 2 {
		t.Fatalf("cache should hold %d entries, got %d", 2, len(cache.servers))
	}
	if _, ok := cache.servers["a"]; ok {
		t.Error("least-recently-used entry 'a' should have been evicted")
	}
	if _, ok := cache.servers["b"]; !ok {
		t.Error("entry 'b' should survive")
	}
	if _, ok := cache.servers["c"]; !ok {
		t.Error("entry 'c' should survive")
	}
}

func TestServerCacheIdleReap(t *testing.T) {
	cache := newTestServerCache(0, time.Hour)
	cache.getOrCreate("stale", func(id string) *mcpserver.MCPServer {
		return mcpserver.NewMCPServer("reap", "1.0")
	}, nil)
	cache.mu.Lock()
	cache.lastUsed["stale"] = time.Now().Add(-2 * time.Hour)
	cache.mu.Unlock()

	cache.reapIdle(time.Now())
	cache.mu.RLock()
	_, ok := cache.servers["stale"]
	cache.mu.RUnlock()
	if ok {
		t.Error("idle entry should be reaped")
	}
}

func TestServerCacheIdleReapDisabled(t *testing.T) {
	cache := newTestServerCache(0, 0)
	cache.getOrCreate("keep", func(id string) *mcpserver.MCPServer {
		return mcpserver.NewMCPServer("keep", "1.0")
	}, nil)
	cache.mu.Lock()
	cache.lastUsed["keep"] = time.Now().Add(-2 * time.Hour)
	cache.mu.Unlock()

	cache.reapIdle(time.Now())
	cache.mu.RLock()
	_, ok := cache.servers["keep"]
	cache.mu.RUnlock()
	if !ok {
		t.Error("reapIdle must no-op when ttl <= 0")
	}
}

func TestServerCacheHandlerReuse(t *testing.T) {
	cache := newTestServerCache(4, time.Hour)
	factory := func(id string) *mcpserver.MCPServer {
		return mcpserver.NewMCPServer("hdl-"+id, "1.0")
	}

	h1 := cache.getOrCreateHandler("ws", factory, nil)
	h2 := cache.getOrCreateHandler("ws", factory, nil)
	if h1 != h2 {
		t.Error("streamable handler should be constructed once per cached server (F6)")
	}
}

func TestGetOrCreateDoesNotClobberToolState(t *testing.T) {
	reg := toolregistry.Create()
	reg.Track(toolregistry.ToolRegistration{Name: "foo", DefaultEnabled: true})
	if !reg.SetToolEnabled("foo", false) {
		t.Fatal("failed to disable tool")
	}
	cache := newTestServerCache(4, time.Hour)
	cache.getOrCreate("ws", func(id string) *mcpserver.MCPServer {
		return mcpserver.NewMCPServer("t", "1.0")
	}, reg)

	if reg.IsToolEnabled("foo") {
		t.Error("getOrCreate must not clobber per-workspace tool state with a global re-apply (F14)")
	}
}

// TestPostMCPWorkspaceFollowUpSequence pins F3: an initialize carrying
// rootUri must bind subsequent hint-less calls to the same cached server
// instead of silently falling back to the "default" workspace.
func TestPostMCPWorkspaceFollowUpSequence(t *testing.T) {
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
		PendingCollaborations: tools.NewCollaborationRegistry(),
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

	initBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","rootUri":"file:///tmp/f3-ws","capabilities":{}}}`)
	if rec := doReq(initBody); rec.Code != http.StatusOK {
		t.Fatalf("initialize: %d: %s", rec.Code, rec.Body.String())
	}
	if factoryCalls != 1 {
		t.Fatalf("factory calls after initialize = %d, want 1", factoryCalls)
	}

	// No workspace hint on the follow-up: it must reuse the initialize server.
	rec := doReq([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list follow-up: %d: %s", rec.Code, rec.Body.String())
	}
	if factoryCalls != 1 {
		t.Fatalf("factory calls after follow-up = %d, want 1 (server must be reused, not re-created for 'default')", factoryCalls)
	}
}
