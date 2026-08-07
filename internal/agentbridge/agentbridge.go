package agentbridge

import (
	"context"
	"fmt"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/mcpcfg"
)

// AgentChatParams mirrors the TS AgentChatParams shape.
type AgentChatParams struct {
	Provider  string
	Message   string
	Path      any
	Container string
}

// DelegateToWebAgent sends a chat request through the Firefox bridge.
func DelegateToWebAgent(ctx context.Context, params AgentChatParams) (string, error) {
	provider := params.Provider
	if provider == "" {
		provider = "gemini"
	}
	container := params.Container
	if container == "" {
		container = "Personal"
	}

	timeoutMs := mcpcfg.GetToolConfig("browser").Timeout
	if timeoutMs <= 0 {
		timeoutMs = 900_000
	}

	sanitized := bridge.DecodeHTMLEntities(bridge.FixMojibake(params.Message))

	bridgeParams := map[string]any{
		"action":    "chat",
		"provider":  provider,
		"path":      params.Path,
		"message":   sanitized,
		"container": container,
		"timeout":   timeoutMs,
	}

	resp, err := bridge.CallBridge(ctx, "chat", bridgeParams)
	if err != nil {
		return "", fmt.Errorf("agent bridge chat failed: %w", err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		data = resp
	}
	if success, ok := data["success"].(bool); ok && !success {
		errMsg := "unknown bridge error"
		if errStr, ok := data["error"].(string); ok {
			errMsg = errStr
		}
		return "", fmt.Errorf("%s", errMsg)
	}

	response, ok := data["response"].(string)
	if !ok {
		content, ok := data["content"].(string)
		if !ok {
			return "", fmt.Errorf("empty or invalid response from browser agent")
		}
		response = content
	}
	return bridge.DecodeHTMLEntities(bridge.FixMojibake(response)), nil
}
