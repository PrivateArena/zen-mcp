package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
)

// bridgeRecorder is a fake Firefox bridge that captures the JSON body of every
// POST so tests can assert exactly which params reach the bridge.
type bridgeRecorder struct {
	mu  sync.Mutex
	req []map[string]any
}

func (r *bridgeRecorder) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("bridge decode: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		r.mu.Lock()
		r.req = append(r.req, body)
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"response":"ok","data":{"content":"ok"}}`))
	}
}

func (r *bridgeRecorder) last() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.req) == 0 {
		return nil
	}
	return r.req[len(r.req)-1]
}

func pointBridgeAt(t *testing.T, url string) {
	t.Helper()
	orig := mcpcfg.Get()
	cfg := *orig
	host, port := splitBridgeURL(url)
	cfg.Host = host
	cfg.FirefoxBridgePort = port
	mcpcfg.Config.Store(&cfg)
	t.Cleanup(func() { mcpcfg.Config.Store(orig) })
}

func splitBridgeURL(raw string) (string, int) {
	rest := strings.TrimPrefix(raw, "http://")
	host := rest
	port := 80
	if i := strings.LastIndex(rest, ":"); i >= 0 {
		host = rest[:i]
		port = 0
		for _, c := range rest[i+1:] {
			port = port*10 + int(c-'0')
		}
	}
	return host, port
}

func browserReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "browser",
			Arguments: args,
		},
	}
}

func TestBrowserChatUploadFilesReachesBridge(t *testing.T) {
	rec := &bridgeRecorder{}
	ts := httptest.NewServer(rec.handler(t))
	defer ts.Close()
	pointBridgeAt(t, ts.URL)

	workspace := t.TempDir()
	ctx := context.Background()
	res := HandleBrowserAction(ctx, workspace, Deps{}, browserReq(map[string]any{
		"action":       "chat",
		"message":      "CONTEXT: red-teaming the architecture",
		"provider":     "claude",
		"upload_files": []any{"PROJECT_OVERVIEW.md", "pkg/sfizz/bridge.c", filepath.Join("pkg", "sfizz", "engine.go")},
	}))
	if res == nil {
		t.Fatal("nil result")
	}

	got := rec.last()
	if got == nil {
		t.Fatalf("bridge never called; result=%v", res)
	}
	if got["action"] != "chat" {
		t.Fatalf("bridge action = %v, want chat", got["action"])
	}
	if got["message"] != "CONTEXT: red-teaming the architecture" {
		t.Fatalf("message not forwarded: %#v", got["message"])
	}
	if got["provider"] != "claude" {
		t.Fatalf("provider not forwarded: %#v", got["provider"])
	}

	// The uploaded files must reach the bridge as ABSOLUTE paths resolved
	// against the workspace root, not raw relative paths (which the Firefox
	// bridge cannot resolve — the CLI CWD differs from the bridge CWD).
	paths, ok := got["path"].([]any)
	if !ok {
		t.Fatalf("bridge path param missing/not an array; got %#v", got["path"])
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %d: %v", len(paths), paths)
	}
	for _, p := range paths {
		s, ok := p.(string)
		if !ok {
			t.Fatalf("path entry not a string: %#v", p)
		}
		if !filepath.IsAbs(s) {
			t.Errorf("path %q must be absolute", s)
		}
		if !strings.HasPrefix(s, workspace) {
			t.Errorf("path %q must be resolved against workspace %q", s, workspace)
		}
	}
	if strings.Contains(paths[0].(string), "PROJECT_OVERVIEW.md") != true {
		t.Errorf("first path %q should reference PROJECT_OVERVIEW.md", paths[0])
	}
	// upload_files must not leak into the bridge body once mapped to path.
	if _, leaked := got["upload_files"]; leaked {
		t.Errorf("upload_files must be consumed, leaked into bridge: %#v", got)
	}
}

func TestBrowserChatUploadFilesResolvedAbsolute(t *testing.T) {
	rec := &bridgeRecorder{}
	ts := httptest.NewServer(rec.handler(t))
	defer ts.Close()
	pointBridgeAt(t, ts.URL)

	workspace := t.TempDir()
	res := HandleBrowserAction(context.Background(), workspace, Deps{}, browserReq(map[string]any{
		"action":       "chat",
		"message":      "hello",
		"upload_files": []any{"a.md"},
	}))
	if res == nil {
		t.Fatal("nil result")
	}
	got := rec.last()
	if got == nil {
		t.Fatalf("bridge never called")
	}
	paths, ok := got["path"].([]any)
	if !ok || len(paths) != 1 {
		t.Fatalf("path = %#v, want one absolute path", got["path"])
	}
	want := filepath.Join(workspace, "a.md")
	if paths[0] != want {
		t.Errorf("path = %q, want %q", paths[0], want)
	}
}

func TestBrowserChatSingleStringUploadStaysString(t *testing.T) {
	// Backward-compat shape guard: a single-file upload expressed as a plain
	// string must reach the bridge as a resolved absolute STRING (not an array).
	rec := &bridgeRecorder{}
	ts := httptest.NewServer(rec.handler(t))
	defer ts.Close()
	pointBridgeAt(t, ts.URL)

	workspace := t.TempDir()
	res := HandleBrowserAction(context.Background(), workspace, Deps{}, browserReq(map[string]any{
		"action":       "chat",
		"message":      "hello",
		"upload_files": "a.md",
	}))
	if res == nil {
		t.Fatal("nil result")
	}
	got := rec.last()
	if got == nil {
		t.Fatalf("bridge never called")
	}
	p, ok := got["path"].(string)
	if !ok {
		t.Fatalf("single-file upload must stay a string, got %#v", got["path"])
	}
	if p != filepath.Join(workspace, "a.md") {
		t.Errorf("path = %q, want %q", p, filepath.Join(workspace, "a.md"))
	}
}

func TestBrowserChatAbsoluteUploadPassthrough(t *testing.T) {
	rec := &bridgeRecorder{}
	ts := httptest.NewServer(rec.handler(t))
	defer ts.Close()
	pointBridgeAt(t, ts.URL)

	abs := "/tmp/exists/abs.md"
	res := HandleBrowserAction(context.Background(), "/some/ws", Deps{}, browserReq(map[string]any{
		"action":       "chat",
		"message":      "hello",
		"upload_files": []any{abs},
	}))
	if res == nil {
		t.Fatal("nil result")
	}
	got := rec.last()
	if got == nil {
		t.Fatalf("bridge never called")
	}
	paths, ok := got["path"].([]any)
	if !ok || len(paths) != 1 || paths[0] != abs {
		t.Fatalf("absolute path must pass through untouched, got %#v", got["path"])
	}
}

func TestBrowserChatNoUploadNoPathParam(t *testing.T) {
	rec := &bridgeRecorder{}
	ts := httptest.NewServer(rec.handler(t))
	defer ts.Close()
	pointBridgeAt(t, ts.URL)

	res := HandleBrowserAction(context.Background(), t.TempDir(), Deps{}, browserReq(map[string]any{
		"action":  "chat",
		"message": "just a message",
	}))
	if res == nil {
		t.Fatal("nil result")
	}
	got := rec.last()
	if got == nil {
		t.Fatalf("bridge never called")
	}
	if got["message"] != "just a message" {
		t.Fatalf("message = %#v", got["message"])
	}
	if _, ok := got["path"]; ok {
		t.Errorf("message-only chat must not set path: %#v", got)
	}
	if _, ok := got["upload_files"]; ok {
		t.Errorf("upload_files must not leak: %#v", got)
	}
}

func TestBrowserChatUploadAndScreenshotConflict(t *testing.T) {
	rec := &bridgeRecorder{}
	ts := httptest.NewServer(rec.handler(t))
	defer ts.Close()
	pointBridgeAt(t, ts.URL)

	res := HandleBrowserAction(context.Background(), t.TempDir(), Deps{}, browserReq(map[string]any{
		"action":          "chat",
		"message":         "hello",
		"upload_files":    []any{"a.md"},
		"take_screenshot": true,
	}))
	if res == nil {
		t.Fatal("nil result")
	}
	if rec.last() != nil {
		t.Fatalf("conflicting chat must not reach the bridge: %#v", rec.last())
	}
	var text string
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
		}
	}
	if !strings.Contains(text, "cannot be used together") {
		t.Errorf("expected conflict error, got: %s", text)
	}
}

func TestResolveUploadFiles(t *testing.T) {
	root := "/ws"
	if resolveUploadFiles(nil, root) != nil {
		t.Error("nil input must return nil")
	}
	if got := resolveUploadFiles("a.md", root); got != "/ws/a.md" {
		t.Errorf("string case = %#v", got)
	}
	if got := resolveUploadFiles("/already/abs.md", root); got != "/already/abs.md" {
		t.Errorf("abs string case = %#v", got)
	}
	if got := resolveUploadFiles([]string{"a.md", "/abs/b.md"}, root); !reflect.DeepEqual(got, []string{"/ws/a.md", "/abs/b.md"}) {
		t.Errorf("[]string case = %#v", got)
	}
	got := resolveUploadFiles([]any{"a.md", 42, "/abs/c.md"}, root)
	want := []any{"/ws/a.md", "/abs/c.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("[]any case (non-strings dropped) = %#v, want %#v", got, want)
	}
	if got := resolveUploadFiles(5, root); got != 5 {
		t.Errorf("unsupported type must pass through = %#v", got)
	}
}

func TestBrowserNonChatUploadFilesPreserved(t *testing.T) {
	// Fall-through actions must keep upload_files (resolved absolute), not
	// rewrite it into path — only chat consumes the path mapping.
	rec := &bridgeRecorder{}
	ts := httptest.NewServer(rec.handler(t))
	defer ts.Close()
	pointBridgeAt(t, ts.URL)

	workspace := t.TempDir()
	res := HandleBrowserAction(context.Background(), workspace, Deps{}, browserReq(map[string]any{
		"action":       "screenshot",
		"upload_files": []any{"a.md"},
	}))
	if res == nil {
		t.Fatal("nil result")
	}
	got := rec.last()
	if got == nil {
		t.Fatalf("bridge never called")
	}
	if _, ok := got["path"]; ok {
		t.Errorf("non-chat action must not set path: %#v", got)
	}
	files, ok := got["upload_files"].([]any)
	if !ok || len(files) != 1 || files[0] != filepath.Join(workspace, "a.md") {
		t.Errorf("non-chat upload_files must be resolved absolute, got %#v", got["upload_files"])
	}
}
