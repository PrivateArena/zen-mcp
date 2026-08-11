package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"zen-mcp/internal/agentbridge"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sys/unix"
)

func defUiVision(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "ui-vision",
		Title:       "UI Vision (Gemini via zen-cap)",
		Description: "Launch a GUI app, capture its window via zen-cap, and get a Gemini description of the UI. Requires zen-cap running at the configured port.",
		Schema: jsonSchema(map[string]any{
			"path":    strProp("Absolute path to the GUI executable to launch and capture"),
			"message": strProp("Natural-language prompt for Gemini to describe the GUI state"),
		}, []string{"path", "message"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleUiVisionAction(ctx, workspace, deps, req), nil
		},
	}
}

func HandleUiVisionAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	appPath, _ := args["path"].(string)
	message, _ := args["message"].(string)

	if appPath == "" || message == "" {
		return toolresponse.WrapErrorWithContext(ctx, "ui-vision", fmt.Errorf("path and message are required"), start)
	}

	cmd := exec.CommandContext(ctx, appPath)
	cmd.SysProcAttr = setPgidSysProcAttr()
	if err := cmd.Start(); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "ui-vision", fmt.Errorf("failed to launch app: %s", err.Error()), start)
	}

	select {
	case <-time.After(3 * time.Second):
	case <-ctx.Done():
		return toolresponse.WrapErrorWithContext(ctx, "ui-vision", ctx.Err(), start)
	}

	outputDir := "/tmp/zen-mcp/ui-vision"
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "ui-vision", err, start)
	}

	timestamp := time.Now().UTC().Format("2006-01-02-150405")
	outputPath := filepath.Join(outputDir, fmt.Sprintf("ui-vision-%s.png", timestamp))

	zenCapURL := mcpcfg.ZenCapURL()
	if !strings.HasSuffix(zenCapURL, "/") {
		zenCapURL += "/"
	}
	screenshotURL := zenCapURL + "screenshot"

	screenshotBody := map[string]any{
		"window": "active",
		"output": outputPath,
	}
	bodyBytes, _ := json.Marshal(screenshotBody)
	screenshotReq, err := http.NewRequestWithContext(ctx, "POST", screenshotURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "ui-vision", err, start)
	}
	screenshotReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	screenshotResp, err := client.Do(screenshotReq)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "ui-vision", fmt.Errorf("zen-cap screenshot failed: %s", err.Error()), start)
	}
	defer screenshotResp.Body.Close()

	var screenshotResult map[string]any
	if err := json.NewDecoder(screenshotResp.Body).Decode(&screenshotResult); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "ui-vision", err, start)
	}

	screenshotPath, _ := screenshotResult["path"].(string)
	if screenshotPath == "" {
		return toolresponse.WrapErrorWithContext(ctx, "ui-vision", fmt.Errorf("zen-cap screenshot failed: no path returned"), start)
	}

	response, err := agentbridge.DelegateToWebAgent(ctx, agentbridge.AgentChatParams{
		Provider: "gemini",
		Path:     screenshotPath,
		Message:  message,
	})
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "ui-vision", err, start)
	}

	return toolresponse.WrapSuccess(ctx, "ui-vision", map[string]any{"path": screenshotPath, "response": response}, start)
}

func setPgidSysProcAttr() *unix.SysProcAttr {
	return &unix.SysProcAttr{Setpgid: true}
}
