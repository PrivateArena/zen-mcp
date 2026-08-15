package tools

import (
	"context"
	"fmt"
	"time"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// defColab is a helper function
func defColab(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "colab",
		Title:       "Google Colab Controller",
		Description: "Interact with Google Colab notebooks via Firefox bridge. Actions: get_status, get_cells, add_cell, update_cell, run_cell, delete_cell, clear_outputs, restart.",
		Schema: jsonSchema(map[string]any{
			"action":  strEnumProp("Colab notebook action", []string{"get_status", "get_cells", "add_cell", "update_cell", "run_cell", "delete_cell", "clear_outputs", "restart"}),
			"index":   numProp("0-based cell index (required update/run/delete/clear)"),
			"type":    strEnumProp("[add_cell] \"code\" or \"text\" (default \"code\")", []string{"code", "text"}),
			"text":    strProp("[update_cell] Source/markdown to write"),
			//"timeout": numProp("Bridge call timeout ms (default 60000)"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleColabAction(ctx, workspace, deps, req), nil
		},
	}
}

// HandleColabAction is a helper function
func HandleColabAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)

	if action == "" {
		return toolresponse.WrapErrorWithContext(ctx, "colab", fmt.Errorf("action is required"), start)
	}

	bridgeParams := map[string]any{}
	if v, ok := args["index"].(float64); ok {
		bridgeParams["index"] = int(v)
	}
	if v, ok := args["type"].(string); ok && v != "" {
		bridgeParams["type"] = v
	}
	if v, ok := args["text"].(string); ok && v != "" {
		bridgeParams["text"] = v
	}
	if v, ok := args["timeout"].(float64); ok && v > 0 {
		bridgeParams["timeout"] = int(v)
	}

	switch action {
	case "update_cell":
		if _, ok := args["index"]; !ok {
			return toolresponse.WrapErrorWithContext(ctx, "colab", fmt.Errorf("index is required for update_cell"), start)
		}
		if _, ok := args["text"]; !ok {
			return toolresponse.WrapErrorWithContext(ctx, "colab", fmt.Errorf("text is required for update_cell"), start)
		}
	case "run_cell", "delete_cell", "clear_outputs":
		if _, ok := args["index"]; !ok {
			return toolresponse.WrapErrorWithContext(ctx, "colab", fmt.Errorf("index is required for %s", action), start)
		}
	}

	bridgeAction := "colab_" + action
	res, err := bridge.CallBridge(ctx, bridgeAction, bridgeParams)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "colab", err, start)
	}

	if success, ok := res["success"].(bool); ok && !success {
		errMsg := "unknown bridge error"
		if errStr, ok := res["error"].(string); ok {
			errMsg = errStr
		}
		return toolresponse.WrapErrorWithContext(ctx, "colab", fmt.Errorf("%s", errMsg), start)
	}

	output := res
	if data, ok := res["data"]; ok {
		if m, ok2 := data.(map[string]any); ok2 {
			output = m
		}
	}
	return toolresponse.WrapSuccess(ctx, "colab", output, start)
}
