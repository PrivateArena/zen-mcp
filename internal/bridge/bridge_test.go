package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"zen-mcp/internal/mcpcfg"
)

func testServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, func()) {
	t.Helper()
	ts := httptest.NewServer(handler)

	orig := mcpcfg.Get()
	cfg := *orig
	host, port := splitURL(ts.URL)
	cfg.Host = host
	cfg.FirefoxBridgePort = port
	mcpcfg.Config.Store(&cfg)

	return ts, func() {
		ts.Close()
		mcpcfg.Config.Store(orig)
	}
}

func splitURL(raw string) (string, int) {
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

func echoHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"action": body["action"],
			"echo":   body["ping"],
		})
	}
}

func TestCallBridgeSucceeds(t *testing.T) {
	_, restore := testServer(t, echoHandler(t))
	defer restore()

	out, err := CallBridge(context.Background(), "ping", map[string]any{"ping": "pong"})
	if err != nil {
		t.Fatalf("CallBridge: %v", err)
	}
	if out["echo"] != "pong" {
		t.Fatalf("expected echo pong, got %v", out["echo"])
	}
	if out["action"] != "ping" {
		t.Fatalf("expected action ping, got %v", out["action"])
	}
}

// TestCallBridgeIgnoresCanceledParent proves the bridge POST is detached from
// the MCP request context: canceling the parent mid-flight must not kill the
// in-flight bridge operation (the regression seen as "context canceled" after
// ~243s in browser.chat).
func TestCallBridgeIgnoresCanceledParent(t *testing.T) {
	slow := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
	_, restore := testServer(t, slow)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	out, err := CallBridge(ctx, "slow", nil)
	if err != nil {
		t.Fatalf("CallBridge must survive parent cancel, got: %v", err)
	}
	if out["ok"] != true {
		t.Fatalf("expected ok=true, got %v", out["ok"])
	}
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Fatalf("handler returned in %v - did the POST actually finish?", elapsed)
	}
}

// TestCallBridgeNoClientDeadline proves no finite client timeout can cut off a
// slow bridge POST even when the configured browser timeout is tiny.
func TestCallBridgeNoClientDeadline(t *testing.T) {
	slow := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
	_, restore := testServer(t, slow)
	defer restore()

	orig := mcpcfg.Get()
	cfg := *orig
	raw, _ := json.Marshal(mcpcfg.ToolConfig{Timeout: 1})
	cfg.ToolConfigs = map[string]json.RawMessage{"browser": raw}
	cfg.McpTimeoutMs = 0
	mcpcfg.Config.Store(&cfg)
	defer mcpcfg.Config.Store(orig)

	start := time.Now()
	out, err := CallBridge(context.Background(), "slow", nil)
	if err != nil {
		t.Fatalf("CallBridge with browser timeout 1ms must not fail, got: %v", err)
	}
	if out["ok"] != true {
		t.Fatalf("expected ok=true, got %v", out["ok"])
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("completed in %v despite 300ms handler - client deadline still applied?", elapsed)
	}
}

func TestCallBridgeSurfacesBridgeErrors(t *testing.T) {
	_, restore := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	defer restore()

	_, err := CallBridge(context.Background(), "fail", nil)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	bf, ok := err.(*ErrBridgeFailure)
	if !ok {
		t.Fatalf("expected *ErrBridgeFailure, got %T", err)
	}
	if bf.Status != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", bf.Status)
	}
}

func TestCallBridgeConnectionRefused(t *testing.T) {
	_, restore := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer restore()

	// Point at a port that is not accepting connections.
	cfg := *mcpcfg.Get()
	cfg.FirefoxBridgePort++
	mcpcfg.Config.Store(&cfg)

	_, err := CallBridge(context.Background(), "dead", nil)
	if err == nil {
		t.Fatal("expected error when bridge is unreachable")
	}
}
