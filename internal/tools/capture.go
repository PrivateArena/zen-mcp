package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// CollaborationState is the lifecycle of a pending collaborative capture.
// A collaboration is settled exactly once: either the HTTP /api/collaborate
// POST resolves it, or the capture tool's 60s timeout expires it. Any later
// Resolve call is a no-op returning false.
type CollaborationState int

const (
	CollabPending CollaborationState = iota
	CollabResolved
	CollabExpired
)

type collabEntry struct {
	resolve func(string)
	state   CollaborationState
}

// CollaborationRegistry is a mutex-guarded registry of pending collaborative
// capture sessions shared by the capture tool (writer) and the /api/collaborate
// route (reader/deleter). It replaces the racy raw map (F5) and guarantees a
// single-owner resolve so the timeout-vs-HTTP race cannot double-resolve or
// drop a late path (F11).
type CollaborationRegistry struct {
	mu    sync.Mutex
	items map[string]*collabEntry
}

func NewCollaborationRegistry() *CollaborationRegistry {
	return &CollaborationRegistry{items: make(map[string]*collabEntry)}
}

// Register records a pending collaboration for id. A later Resolve invokes
// resolve exactly once. Nil-receiver and nil resolve are tolerated.
func (r *CollaborationRegistry) Register(id string, resolve func(string)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = make(map[string]*collabEntry)
	}
	r.items[id] = &collabEntry{resolve: resolve, state: CollabPending}
}

// Resolve claims a pending collaboration and invokes its callback with path.
// It returns true only for the single caller that wins the claim; unknown,
// already-resolved, and expired ids return false.
func (r *CollaborationRegistry) Resolve(id, path string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	e, ok := r.items[id]
	if !ok || e.state != CollabPending {
		r.mu.Unlock()
		return false
	}
	e.state = CollabResolved
	delete(r.items, id)
	resolve := e.resolve
	r.mu.Unlock()

	if resolve != nil {
		resolve(path)
	}
	return true
}

// Expire settles a pending collaboration as expired (e.g. tool timeout),
// making any late Resolve a no-op. Returns true if it claimed the entry.
func (r *CollaborationRegistry) Expire(id string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.items[id]
	if !ok || e.state != CollabPending {
		return false
	}
	e.state = CollabExpired
	delete(r.items, id)
	return true
}

// Contains reports whether id is currently pending.
func (r *CollaborationRegistry) Contains(id string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.items[id]
	return ok && e.state == CollabPending
}


func defCapture(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "capture",
		Title:       "Zen-Cap Screenshot Capture",
		Description: "Capture screenshots via Zen-Cap HTTP API. Modes: full, region, window, collaborate. Prefer window mode.",
		Schema: jsonSchema(map[string]any{
			"action":     strEnumProp("Action", []string{"screenshot"}),
			"mode":       strEnumProp("Capture mode", []string{"full", "region", "window", "collaborate"}),
			"region":     strProp("Coordinates (X,Y,W,H) or \"interactive\""),
			"window":     strProp("\"active\", \"interactive\", ID, or class/title substring"),
			"pid":        numProp("Window PID"),
			"class":      strProp("WM_CLASS match"),
			"title":      strProp("Window title match"),
			"launch":     strProp("App to launch before capture"),
			"clipMode":   strEnumProp("Post-capture clipboard action", []string{"image", "path", "ocr", "translate", "none"}),
			"outputPath": strProp("Save path"),
			"delay":      numProp("Delay seconds before capture"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleCaptureAction(ctx, workspace, deps, req), nil
		},
	}
}

func HandleCaptureAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)

	if action != "screenshot" {
		return toolresponse.WrapErrorWithContext(ctx, "capture", fmt.Errorf("Unknown action: %s", action), start)
	}

	apiAddr := getZenCapAPIAddress()
	targetPath, _ := args["outputPath"].(string)
	if targetPath == "" {
		targetPath = fmt.Sprintf("/tmp/screenshot_%d.png", time.Now().UnixNano())
	}

	mode, _ := args["mode"].(string)
	if mode == "" {
		mode = "full"
	}

	if delay, ok := args["delay"].(float64); ok && delay > 0 {
		time.Sleep(time.Duration(delay) * time.Second)
	}

	if mode == "collaborate" {
		return HandleCollaborateCapture(ctx, apiAddr, targetPath, start, deps)
	}

	return HandleStandardCapture(ctx, apiAddr, targetPath, mode, args, start)
}

func HandleCollaborateCapture(ctx context.Context, apiAddr, targetPath string, start time.Time, deps Deps) *mcp.CallToolResult {
	collabID := fmt.Sprintf("collab_%d_%06x", time.Now().Unix(), rand.Int31())
	port := mcpcfg.Get().McpPort
	if port == 0 {
		port = mcpcfg.Get().DaemonPort
	}
	collaborateURL := fmt.Sprintf("http://localhost:%d/api/collaborate?id=%s", port, collabID)
	_ = collaborateURL

	resolveCh := make(chan string, 1)
	if deps.PendingCollaborations != nil {
		deps.PendingCollaborations.Register(collabID, func(path string) {
			resolveCh <- path
		})
	}
	// Expire is idempotent: if the HTTP POST already resolved this id, the
	// entry is gone and this is a no-op (F5/F11 single-owner state machine).
	defer deps.PendingCollaborations.Expire(collabID)

	select {
	case savedPath := <-resolveCh:
		return toolresponse.WrapSuccess(ctx, "capture", map[string]any{"path": savedPath, "mode": "collaborate"}, start)
	case <-time.After(60 * time.Second):
		return toolresponse.WrapErrorWithContext(ctx, "capture", fmt.Errorf("Collaborate screenshot timed out after 60 seconds."), start)
	}
}

func HandleStandardCapture(ctx context.Context, apiAddr, targetPath, mode string, args map[string]any, start time.Time) *mcp.CallToolResult {
	region, _ := args["region"].(string)
	window, _ := args["window"].(string)
	pid, _ := args["pid"].(float64)
	class, _ := args["class"].(string)
	title, _ := args["title"].(string)
	launch, _ := args["launch"].(string)
	clipMode, _ := args["clipMode"].(string)

	if region == "" && mode == "region" {
		region = "interactive"
	}
	if window == "" && mode == "window" {
		window = "active"
	}

	body := map[string]any{
		"output":    targetPath,
		"region":    region,
		"window":    window,
		"pid":       int(pid),
		"class":     class,
		"title":     title,
		"launch":    launch,
		"clip_mode": clipMode,
	}

	bodyBytes, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s/screenshot", apiAddr), strings.NewReader(string(bodyBytes)))
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "capture", err, start)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "capture", fmt.Errorf("Zen-Cap service is not running or not listening on %s. Error: %s", apiAddr, err.Error()), start)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]any
		json.NewDecoder(resp.Body).Decode(&errBody)
		errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if em, ok := errBody["error"].(string); ok {
			errMsg = em
		}
		return toolresponse.WrapErrorWithContext(ctx, "capture", fmt.Errorf("Zen-Cap screenshot failed: %s", errMsg), start)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "capture", err, start)
	}

	return toolresponse.WrapSuccess(ctx, "capture", map[string]any{"path": data["path"], "status": "success"}, start)
}

var (
	zenCapAddrMu       sync.Mutex
	zenCapAddrCache    string
	zenCapAddrLoadedAt time.Time
	zenCapAddrCacheTTL = time.Minute
)

// getZenCapAPIAddress returns the zen-cap API address, re-reading the config
// at most once per minute (F12: the old code hit disk on every capture call).
func getZenCapAPIAddress() string {
	zenCapAddrMu.Lock()
	defer zenCapAddrMu.Unlock()
	if zenCapAddrCache != "" && time.Since(zenCapAddrLoadedAt) < zenCapAddrCacheTTL {
		return zenCapAddrCache
	}
	addr := readZenCapAPIAddress()
	zenCapAddrCache = addr
	zenCapAddrLoadedAt = time.Now()
	return addr
}

func readZenCapAPIAddress() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "localhost:4444"
	}
	configPath := filepath.Join(home, ".config", "zen-cap", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "localhost:4444"
	}
	var cfg map[string]any
	if json.Unmarshal(data, &cfg) != nil {
		return "localhost:4444"
	}
	if addr, ok := cfg["api_address"].(string); ok && addr != "" {
		return strings.TrimSpace(addr)
	}
	return "localhost:4444"
}
