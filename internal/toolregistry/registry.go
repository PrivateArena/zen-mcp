package toolregistry

import (
	"context"
	"sync"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

type Handler func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

type ToolRegistration struct {
	Name           string
	DefaultEnabled bool
	Description    string
	Schema         map[string]any
	Handler        Handler
	Enabled        bool
}

type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]*ToolRegistration
}

func Create() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]*ToolRegistration)}
}

func (r *ToolRegistry) Track(reg ToolRegistration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reg.DefaultEnabled {
		reg.Enabled = true
	}
	r.tools[reg.Name] = &reg
}

func (r *ToolRegistry) GetTool(name string) (*ToolRegistration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.tools[name]
	return entry, ok
}

func (r *ToolRegistry) ListToolNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	return out
}

func (r *ToolRegistry) ListTools() []*ToolRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ToolRegistration, 0, len(r.tools))
	for _, entry := range r.tools {
		out = append(out, entry)
	}
	return out
}

func (r *ToolRegistry) SetToolEnabled(name string, enabled bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.tools[name]
	if !ok {
		return false
	}
	entry.Enabled = enabled
	return true
}

func (r *ToolRegistry) IsToolEnabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.tools[name]
	if !ok {
		return false
	}
	return entry.Enabled
}

func (r *ToolRegistry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools = make(map[string]*ToolRegistration)
}
