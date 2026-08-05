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
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/jang/zen-mcp/internal/mcpcfg"
	"github.com/jang/zen-mcp/internal/toolresponse"
)

var pendingCollaborations = map[string]chan string{}

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
			return handleCaptureAction(ctx, workspace, deps, req), nil
		},
	}
}

func handleCaptureAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
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
		return handleCollaborateCapture(ctx, apiAddr, targetPath, start, deps)
	}

	return handleStandardCapture(ctx, apiAddr, targetPath, mode, args, start)
}

func handleCollaborateCapture(ctx context.Context, apiAddr, targetPath string, start time.Time, deps Deps) *mcp.CallToolResult {
	collabID := fmt.Sprintf("collab_%d_%06x", time.Now().Unix(), rand.Int31())
	port := mcpcfg.Get().McpPort
	if port == 0 {
		port = mcpcfg.Get().DaemonPort
	}
	collaborateURL := fmt.Sprintf("http://localhost:%d/api/collaborate?id=%s", port, collabID)
	_ = collaborateURL

	resolveCh := make(chan string, 1)
	if deps.PendingCollaborations != nil {
		deps.PendingCollaborations[collabID] = func(path string) {
			resolveCh <- path
		}
	}

	select {
	case savedPath := <-resolveCh:
		delete(deps.PendingCollaborations, collabID)
		return toolresponse.WrapSuccess(ctx, "capture", map[string]any{"path": savedPath, "mode": "collaborate"}, start)
	case <-time.After(60 * time.Second):
		delete(deps.PendingCollaborations, collabID)
		return toolresponse.WrapErrorWithContext(ctx, "capture", fmt.Errorf("Collaborate screenshot timed out after 60 seconds."), start)
	}
}

func handleStandardCapture(ctx context.Context, apiAddr, targetPath, mode string, args map[string]any, start time.Time) *mcp.CallToolResult {
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

func getZenCapAPIAddress() string {
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
