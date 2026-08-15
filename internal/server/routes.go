package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/shared"
	"zen-mcp/internal/telemetry"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/tools"
)

const (
	ServerName    = "zen-tools"
	ServerVersion = "2.4.1"
)

type RouteDeps struct {
	CreateMCPServer       func(id string) *mcpserver.MCPServer
	Registry              *toolregistry.ToolRegistry
	Shared                *shared.Store
	PendingCollaborations *tools.CollaborationRegistry
	StartTime             time.Time
	Tag                   string
	ServerCache           *serverCache
}

type rpcMessage struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

// serverCache maps a logical workspace ID to a cached MCPServer, its
// per-server streamable HTTP handler, and its last-use time. It is bounded by
// serverCacheMaxSize (LRU eviction) and serverCacheTTL (idle reaping) so a
// client-controlled logicalID space cannot grow memory without bound (F2).
type serverCache struct {
	mu       sync.RWMutex
	servers  map[string]*mcpserver.MCPServer
	lastUsed map[string]time.Time
	handlers map[string]*mcpserver.StreamableHTTPServer
	maxSize  int
	ttl      time.Duration
}

// newServerCache is a helper function
func newServerCache() *serverCache {
	c := &serverCache{
		servers:  make(map[string]*mcpserver.MCPServer),
		lastUsed: make(map[string]time.Time),
		handlers: make(map[string]*mcpserver.StreamableHTTPServer),
		maxSize:  serverCacheMaxSize,
		ttl:      serverCacheTTL,
	}
	registerServerCache(c)
	return c
}

// getOrCreate is a helper function
func (c *serverCache) getOrCreate(logicalID string, factory func(string) *mcpserver.MCPServer, registry *toolregistry.ToolRegistry) *mcpserver.MCPServer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	if srv, ok := c.servers[logicalID]; ok {
		c.lastUsed[logicalID] = time.Now()
		return srv
	}
	srv := factory(logicalID)
	c.servers[logicalID] = srv
	c.lastUsed[logicalID] = time.Now()
	// F6/F13: construct the stateless streamable HTTP handler exactly once per
	// cached server instead of per request, so future StreamableHTTPOption
	// configuration cannot silently no-op.
	c.handlers[logicalID] = mcpserver.NewStreamableHTTPServer(srv, mcpserver.WithStateLess(true))
	c.evictLocked()
	return srv
}

// getOrCreateHandler is a helper function
func (c *serverCache) getOrCreateHandler(logicalID string, factory func(string) *mcpserver.MCPServer, registry *toolregistry.ToolRegistry) *mcpserver.StreamableHTTPServer {
	c.getOrCreate(logicalID, factory, registry)
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.handlers[logicalID]
}

// initLocked lazily initializes maps so struct literals created without the
// helper still work (kept for backward compatibility with existing callers).
func (c *serverCache) initLocked() {
	if c.servers == nil {
		c.servers = make(map[string]*mcpserver.MCPServer)
	}
	if c.lastUsed == nil {
		c.lastUsed = make(map[string]time.Time)
	}
	if c.handlers == nil {
		c.handlers = make(map[string]*mcpserver.StreamableHTTPServer)
	}
}

// evictLocked drops the least-recently-used entries while over maxSize.
// Caller holds c.mu.
func (c *serverCache) evictLocked() {
	if c.maxSize <= 0 {
		return
	}
	for len(c.servers) > c.maxSize {
		var oldest string
		var oldestT time.Time
		for id, t := range c.lastUsed {
			if oldest == "" || t.Before(oldestT) {
				oldest, oldestT = id, t
			}
		}
		if oldest == "" {
			return
		}
		delete(c.servers, oldest)
		delete(c.lastUsed, oldest)
		delete(c.handlers, oldest)
	}
}

// reapIdle evicts entries idle longer than ttl. No-op when ttl <= 0.
func (c *serverCache) reapIdle(now time.Time) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, t := range c.lastUsed {
		if now.Sub(t) > c.ttl {
			delete(c.servers, id)
			delete(c.lastUsed, id)
			delete(c.handlers, id)
		}
	}
}

// SetupRoutes registers the 12 stateless HTTP routes (Go 1.22 ServeMux).
func SetupRoutes(mux *http.ServeMux, deps RouteDeps) {
	if deps.Shared == nil {
		deps.Shared = shared.NewStore()
	}
	if deps.StartTime.IsZero() {
		deps.StartTime = time.Now()
	}
	if deps.ServerCache == nil {
		deps.ServerCache = newServerCache()
	}

	mux.HandleFunc("GET /sse", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusBadRequest, "SSE sessions not supported in stateless mode")
	})
	mux.HandleFunc("POST /sse/message", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusBadRequest, "SSE sessions not supported in stateless mode")
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		transport := os.Getenv("MCP_TRANSPORT")
		if transport == "" {
			transport = "unknown"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"server":    ServerName,
			"version":   ServerVersion,
			"transport": transport,
			"uptime":    time.Since(deps.StartTime).Seconds(),
			"startedAt": deps.StartTime.UTC().Format("2006-01-02T15:04:05.000Z"),
		})
	})
	mux.HandleFunc("GET /mcp-status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"uptime":     time.Since(deps.StartTime).Seconds(),
			"serverInfo": map[string]any{"name": ServerName, "version": ServerVersion},
		})
	})
	mux.HandleFunc("GET /recovery", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "\nMCP AUTO-RECOVERY\n=================\n\nThe server creates a fresh transport per request.\nIf you see connection errors, simply retry the request.\n\nIf all else fails, restart the server.\n  ")
	})
	mux.HandleFunc("GET /shared/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		value, ok := deps.Shared.Get(key)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("Key '%s' not found", key)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": value})
	})
	mux.HandleFunc("POST /shared/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		var raw map[string]any
		if err := json.NewDecoder(io.LimitReader(r.Body, 50<<20)).Decode(&raw); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Body must contain a string value field"})
			return
		}
		value, ok := raw["value"].(string)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Body must contain a string value field"})
			return
		}
		deps.Shared.Set(key, value)
		log.Printf("[Shared] %s = %s", key, value)
		writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": value})
	})
	mux.HandleFunc("GET /shared", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, deps.Shared.GetAll())
	})
	mux.HandleFunc("POST /api/collaborate", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Missing query parameter: id"})
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 50<<20)).Decode(&body); err != nil || body.Path == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Missing body parameter: path"})
			return
		}
		// F5/F11: Resolve claims the pending collaboration atomically; the
		// resolve callback fires exactly once and a duplicate or late POST
		// (after timeout) is answered with 404 instead of a spurious 200.
		if deps.PendingCollaborations.Resolve(id, body.Path) {
			log.Printf("[API] Resolved collaborative capture session: %s with path: %s", id, body.Path)
			writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("Session not found or expired: %s", id)})
	})
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		deps.postMCP(w, r)
	})
	mux.HandleFunc("GET /mcp", func(w http.ResponseWriter, _ *http.Request) {
		// F7: Streamable HTTP spec requires 405 (not 400) when GET has no SSE
		// support; the Allow header advertises the supported method.
		w.Header().Set("Allow", "POST")
		writeText(w, http.StatusMethodNotAllowed, "Method Not Allowed: Streamable HTTP requires POST")
	})
	mux.HandleFunc("DELETE /mcp", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusBadRequest, "Session termination not supported in stateless mode")
	})
}

// postMCP is a helper function
func (d RouteDeps) postMCP(w http.ResponseWriter, r *http.Request) {
	// F4: read the body exactly once. The parsed method/params drive both
	// workspace detection and the tools/list rewrite; the body is rewound once
	// for the mcp-go handler.
	body, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Failed to read request body"})
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	var msg rpcMessage
	_ = json.Unmarshal(body, &msg)

	workspace := detectWorkspace(msg, r, d.Shared)
	logicalID := workspace
	if logicalID == "" {
		logicalID = "default"
	}

	handler := d.ServerCache.getOrCreateHandler(logicalID, d.CreateMCPServer, d.Registry)
	if msg.Method == "tools/list" {
		bw := toolsListRewriter(w)
		serveWithAbortLog(r, msg, handler, bw)
		_ = bw.finish()
		return
	}
	serveWithAbortLog(r, msg, handler, w)
}

// serveWithAbortLog runs the streamable MCP handler while watching the client
// request context. If the client disconnects or cancels before the handler
// finishes (e.g., Opencode/Kilocode giving up on a slow handshake or tool
// call), the abort is logged with the method and elapsed duration so
// client-side timeouts are attributable. It never touches the ResponseWriter
// itself, so there is no race with the handler's own writes.
func serveWithAbortLog(r *http.Request, msg rpcMessage, handler *mcpserver.StreamableHTTPServer, w http.ResponseWriter) {
	start := time.Now()
	done := make(chan struct{})
	go func() {
		select {
		case <-r.Context().Done():
			select {
			case <-done:
				return
			default:
				elapsed := time.Since(start).Milliseconds()
				reason := abortReason(r.Context())
				logfilter.Errorf("[MCP] CLIENT-ABORT method %q after %dms reason=%s", msg.Method, elapsed, reason)
				// tools/call aborts are recorded at the tool-handler level
				// (WrapHandlerWithTimeout), which carries tool + action. Every
				// other MCP method aborted mid-handshake is recorded here so no
				// client-side timeout is invisible to telemetry.
				if msg.Method != "tools/call" {
					_ = telemetry.LogToolCall(reqTool(msg), "", false,
						fmt.Sprintf("client abort after %dms: %s", elapsed, reason), elapsed)
				}
				if onRequestAbort != nil {
					onRequestAbort(msg.Method, elapsed, reason)
				}
			}
		case <-done:
		}
	}()
	handler.ServeHTTP(w, r)
	close(done)
}

// reqTool extracts a tool name from a tools/call message body, empty otherwise.
func reqTool(msg rpcMessage) string {
	if msg.Method != "tools/call" {
		return msg.Method
	}
	if name, ok := msg.Params["name"].(string); ok {
		return name
	}
	return "tools/call"
}

// onRequestAbort is a test-only observer, invoked after a client-request abort
// is logged.
var onRequestAbort func(method string, elapsedMs int64, reason string)

// detectWorkspace resolves the workspace for an already-decoded request.
func detectWorkspace(msg rpcMessage, r *http.Request, st *shared.Store) string {
	var projectPath string

	if msg.Method == "initialize" {
		if params := msg.Params; params != nil {
			if rootURI, ok := params["rootUri"].(string); ok && strings.HasPrefix(rootURI, "file://") {
				if p := fileURLToPath(rootURI); p != "" {
					projectPath = p
				}
			}
			if projectPath == "" {
				if folders, ok := params["workspaceFolders"].([]any); ok && len(folders) > 0 {
					if m, ok := folders[0].(map[string]any); ok {
						if uri, ok := m["uri"].(string); ok && strings.HasPrefix(uri, "file://") {
							if p := fileURLToPath(uri); p != "" {
								projectPath = p
							}
						}
					}
				}
			}
		}
		if projectPath != "" && st != nil {
			// F3: carry the initialize-detected workspace to subsequent
			// stateless requests that arrive without an explicit hint, so a
			// follow-up tools/call binds to the same cached server instead of
			// silently falling back to "default".
			st.Set("workspace-root", projectPath)
		}
	}

	if projectPath == "" {
		for _, key := range []string{"projectPath", "project", "workspace", "root", "path"} {
			if v := r.URL.Query().Get(key); v != "" {
				projectPath = v
				break
			}
		}
	}

	if projectPath == "" {
		if v, ok := st.Get("workspace-root"); ok {
			projectPath = v
		}
	}

	if projectPath == "" {
		for _, header := range []string{"X-Project-Path", "X-Workspace-Root", "Mcp-Project-Path", "Project-Path"} {
			if v := r.Header.Get(header); v != "" {
				projectPath = v
				break
			}
		}
	}

	if projectPath != "" {
		if abs, err := filepath.Abs(projectPath); err == nil {
			return abs
		}
	}
	return ""
}

// fileURLToPath is a helper function
func fileURLToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	if p == "" {
		p = u.Opaque
	}
	if host := u.Host; host != "" {
		p = "//" + host + p
	}
	if dec, err := url.PathUnescape(p); err == nil {
		p = dec
	}
	return filepath.FromSlash(p)
}

// writeText is a helper function
func writeText(w http.ResponseWriter, code int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(text))
}

// writeJSON is a helper function
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
