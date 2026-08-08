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

	"zen-mcp/internal/shared"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/toolstate"
)

const (
	ServerName    = "zen-tools"
	ServerVersion = "2.4.1"
)

type RouteDeps struct {
	CreateMCPServer       func(id string) *mcpserver.MCPServer
	Registry              *toolregistry.ToolRegistry
	Shared                *shared.Store
	PendingCollaborations map[string]func(string)
	StartTime             time.Time
	Tag                   string
	ServerCache           *serverCache
}

type serverCache struct {
	mu      sync.RWMutex
	servers map[string]*mcpserver.MCPServer
}

func (c *serverCache) getOrCreate(logicalID string, factory func(string) *mcpserver.MCPServer, registry *toolregistry.ToolRegistry) *mcpserver.MCPServer {
	c.mu.RLock()
	if srv, ok := c.servers[logicalID]; ok {
		c.mu.RUnlock()
		return srv
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if srv, ok := c.servers[logicalID]; ok {
		return srv
	}
	srv := factory(logicalID)
	if registry != nil {
		toolstate.ApplyToolStates("", registry)
	}
	c.servers[logicalID] = srv
	return srv
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
		deps.ServerCache = &serverCache{
			servers: make(map[string]*mcpserver.MCPServer),
		}
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
		resolve, ok := deps.PendingCollaborations[id]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("Session not found or expired: %s", id)})
			return
		}
		delete(deps.PendingCollaborations, id)
		resolve(body.Path)
		log.Printf("[API] Resolved collaborative capture session: %s with path: %s", id, body.Path)
		writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
	})
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		deps.postMCP(w, r)
	})
	mux.HandleFunc("GET /mcp", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusBadRequest, "Streamable HTTP requires POST")
	})
	mux.HandleFunc("DELETE /mcp", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusBadRequest, "Session termination not supported in stateless mode")
	})
}

func (d RouteDeps) postMCP(w http.ResponseWriter, r *http.Request) {
	workspace := autoDetectWorkspace(r, d.Shared)
	logicalID := workspace
	if logicalID == "" {
		logicalID = "default"
	}

	srv := d.ServerCache.getOrCreate(logicalID, d.CreateMCPServer, d.Registry)

	handler := mcpserver.NewStreamableHTTPServer(srv, mcpserver.WithStateLess(true))
	if jsonBodyMethod(r) == "tools/list" {
		bw := toolsListRewriter(w)
		handler.ServeHTTP(bw, r)
		_ = bw.finish()
		return
	}
	handler.ServeHTTP(w, r)
}

// autoDetectWorkspace resolves a workspace root from initialize params, query,
// shared state, then headers — in that order, matching routes.ts.
func autoDetectWorkspace(r *http.Request, st *shared.Store) string {
	var projectPath string

	body, _ := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	r.Body = io.NopCloser(bytes.NewReader(body))

	var msg struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	_ = json.Unmarshal(body, &msg)
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

func jsonBodyMethod(r *http.Request) string {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	r.Body = io.NopCloser(bytes.NewReader(body))
	var msg struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(body, &msg)
	if msg.Method == "" {
		return "(unknown)"
	}
	return msg.Method
}

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

func writeText(w http.ResponseWriter, code int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(text))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
