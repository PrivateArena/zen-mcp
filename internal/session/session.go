package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jang/zen-mcp/internal/logfilter"
	"github.com/jang/zen-mcp/internal/mcpcfg"
	"github.com/jang/zen-mcp/internal/shared"
)

var sessionStatePath = filepath.Join(".zenmcp", "sessions.json")

type Manager struct {
	mu                  sync.RWMutex
	store               *shared.Store
	cwd                 string
	sessionWorkspaces   map[string]string
	sessionTimestamps   map[string]time.Time
	knownSessions       map[string]bool
	lastActiveSessionId string
	aliasMap            map[string]string
	pathResolver        *PathResolver
}

func New(store *shared.Store) *Manager {
	return newWithCwd(store, mustGetwd())
}

func newWithCwd(store *shared.Store, cwd string) *Manager {
	m := &Manager{
		store:             store,
		cwd:               cwd,
		sessionWorkspaces: map[string]string{},
		sessionTimestamps: map[string]time.Time{},
		knownSessions:     map[string]bool{},
		aliasMap:          map[string]string{},
	}
	m.loadAliasMap()
	m.pathResolver = NewPathResolver(m.aliasMap)
	return m
}

func mustGetwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func (m *Manager) loadAliasMap() {
	data, err := os.ReadFile(mcpcfg.MapFilePath())
	if err != nil {
		return
	}
	for _, fullPath := range orderedMapKeys(data) {
		m.aliasMap[fullPath] = fullPath
		base := filepath.Base(fullPath)
		if base != "" {
			m.aliasMap[base] = fullPath
		}
	}
}

func orderedMapKeys(data []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil
	}
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			break
		}
		if k, ok := key.(string); ok {
			keys = append(keys, k)
		}
		var v any
		if err := dec.Decode(&v); err != nil {
			break
		}
	}
	return keys
}

func (m *Manager) SetLastActiveSessionId(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastActiveSessionId = id
}

func (m *Manager) GetLastActiveSessionId() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastActiveSessionId
}

func (m *Manager) GetActiveWorkspaceRoot() string {
	m.mu.RLock()
	sid := m.lastActiveSessionId
	m.mu.RUnlock()
	if sid != "" {
		if ws := m.GetSessionWorkspaceRoot(sid); ws != "" {
			return ws
		}
	}
	if ws := m.GetSessionWorkspaceRoot("zen-persistent-session"); ws != "" {
		return ws
	}

	m.mu.RLock()
	for _, ws := range m.sessionWorkspaces {
		if ws != "" && exists(ws) {
			m.mu.RUnlock()
			return ws
		}
	}
	m.mu.RUnlock()

	if m.store != nil {
		if sharedWs, ok := m.store.Get("workspace-root"); ok && sharedWs != "" {
			return sharedWs
		}
	}
	return os.Getenv("MCP_WORKSPACE_ROOT")
}

func (m *Manager) UpdateSessionActivity(sessionId string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionTimestamps[sessionId] = time.Now()
}

func (m *Manager) SetSessionWorkspaceRoot(sessionId, path string) {
	resolved := m.resolvePath(path)
	m.mu.Lock()
	current := m.sessionWorkspaces[sessionId]
	shouldSet := resolved != current && exists(resolved)
	if shouldSet {
		m.sessionWorkspaces[sessionId] = resolved
	}
	m.mu.Unlock()
}

func (m *Manager) GetSessionWorkspaceRoot(sessionId string) string {
	m.mu.Lock()
	raw := m.sessionWorkspaces[sessionId]
	m.mu.Unlock()
	if raw == "" {
		raw = os.Getenv("MCP_WORKSPACE_ROOT")
	}
	if raw == "" {
		return ""
	}
	if exists(raw) {
		return raw
	}
	m.mu.Lock()
	delete(m.sessionWorkspaces, sessionId)
	m.mu.Unlock()
	return ""
}

func (m *Manager) ResolveWorkspacePath(input string) (string, bool) {
	resolved := m.resolvePath(input)
	return resolved, resolved != ""
}

func (m *Manager) resolvePath(input string) string {
	if p, ok := m.pathResolver.Resolve(input); ok {
		return p
	}
	if filepath.IsAbs(input) {
		return input
	}
	return filepath.Join(m.cwd, input)
}

func (m *Manager) KnownSessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.knownSessions))
	for id := range m.knownSessions {
		out = append(out, id)
	}
	return out
}

func (m *Manager) Save() error {
	m.mu.Lock()
	type tsEntry struct {
		id string
		t  time.Time
	}
	entries := make([]tsEntry, 0, len(m.sessionTimestamps))
	for id, t := range m.sessionTimestamps {
		entries = append(entries, tsEntry{id: id, t: t})
	}
	// insertion-order-independent desc sort
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j-1].t.Before(entries[j].t); j-- {
			entries[j-1], entries[j] = entries[j], entries[j-1]
		}
	}
	sorted := make([]string, 0, len(entries))
	for _, e := range entries {
		sorted = append(sorted, e.id)
	}
	if len(sorted) > 100 {
		sorted = sorted[:100]
	}
	known := make(map[string]bool, len(sorted))
	for _, id := range sorted {
		known[id] = true
	}
	m.knownSessions = known

	timestamps := map[string]string{}
	for id, t := range m.sessionTimestamps {
		timestamps[id] = t.UTC().Format(time.RFC3339Nano)
	}
	state := map[string]any{
		"workspaces":    m.sessionWorkspaces,
		"timestamps":    timestamps,
		"knownSessions": sorted,
		"lastUpdated":   time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	m.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(m.cwd, ".zenmcp"), 0o755); err != nil {
		logfilter.Debugf("[MCP] Failed to save session state: %v", err)
		return err
	}
	if err := os.WriteFile(filepath.Join(m.cwd, sessionStatePath), data, 0o644); err != nil {
		logfilter.Debugf("[MCP] Failed to save session state: %v", err)
		return err
	}
	logfilter.Info("[MCP] Session state saved to " + sessionStatePath)
	return nil
}

func (m *Manager) Load() error {
	data, err := os.ReadFile(filepath.Join(m.cwd, sessionStatePath))
	if err != nil {
		return nil
	}
	var state struct {
		Workspaces    map[string]string `json:"workspaces"`
		Timestamps    map[string]string `json:"timestamps"`
		KnownSessions []string          `json:"knownSessions"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		logfilter.Debugf("[MCP] Failed to load session state: %v", err)
		return err
	}

	m.mu.Lock()
	if state.Workspaces != nil {
		m.sessionWorkspaces = state.Workspaces
	}
	if state.Timestamps != nil {
		ts := map[string]time.Time{}
		for k, v := range state.Timestamps {
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				ts[k] = t
			}
		}
		m.sessionTimestamps = ts
	}
	if len(state.KnownSessions) > 0 {
		known := map[string]bool{}
		for _, id := range state.KnownSessions {
			known[id] = true
		}
		m.knownSessions = known
	} else if state.Timestamps != nil {
		known := map[string]bool{}
		for k := range state.Timestamps {
			known[k] = true
		}
		m.knownSessions = known
	}
	m.mu.Unlock()

	logfilter.Infof("[MCP] Restored %d session workspace mappings", len(state.Workspaces))
	logfilter.Infof("[MCP] Restored %d known sessions for recovery", len(m.KnownSessions()))
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
