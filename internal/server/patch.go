package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/toolresponse"
	"zen-mcp/internal/toolsuggestions"
)

// SummarizeParams renders call arguments for log messages, truncating to 400
// chars with the same suffix format as patch-mcp.ts.
func SummarizeParams(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		return fmt.Sprintf("%v", args)
	}
	if len(b) > 400 {
		return fmt.Sprintf("%s... [truncated %d chars]", b[:400], len(b)-400)
	}
	return string(b)
}

// WrapHandlerWithTimeout wraps a tool handler with the per-tool timeout,
// parameter validation, inflight accounting, and error wrapping that
// patch-mcp.ts applies at registration time.
func WrapHandlerWithTimeout(name string, inner toolregistry.Handler, getTimeout func(string) time.Duration) toolregistry.Handler {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		timeout := getTimeout(name)
		params := req.GetArguments()
		action, _ := params["action"].(string)

		ctx = toolresponse.WithToolContext(ctx, toolresponse.ToolContext{ToolName: name, Params: params})

		if action != "" && mcpcfg.Get().ToolSuggestionsEnabled {
			res := toolsuggestions.ValidateToolCall(name, action, params, toolresponse.GetToolSchema(name))
			if !res.Valid {
				msg := res.Suggestion
				if msg == "" {
					msg = fmt.Sprintf("Invalid parameters for tool %q action %q", name, action)
				}
				return toolresponse.WrapErrorWithContext(ctx, name, fmt.Errorf("%s", msg), start), nil
			}
		}

		if srv := PoolServerFrom(ctx); srv != nil {
			BeginToolCall(srv)
			defer EndToolCall(srv)
		}

		type result struct {
			res *mcp.CallToolResult
			err error
		}
		ch := make(chan result, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- result{res: toolresponse.WrapErrorWithContext(ctx, name, fmt.Errorf("panic: %v", r), start)}
				}
			}()
			res, err := inner(ctx, req)
			ch <- result{res: res, err: err}
		}()

		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case r := <-ch:
			if r.err != nil {
				return toolresponse.WrapErrorWithContext(ctx, name, r.err, start), nil
			}
			return r.res, nil
		case <-timer.C:
			elapsed := time.Since(start).Milliseconds()
			actionLabel := action
			if actionLabel == "" {
				actionLabel = "(none)"
			}
			logfilter.Errorf("[MCP] TIMER Tool '%s' timed out after %dms (elapsed %dms) — action %q params: %s",
				name, timeout.Milliseconds(), elapsed, actionLabel, SummarizeParams(params))
			return toolresponse.WrapErrorWithContext(ctx, name, fmt.Errorf("Tool '%s' timed out after %dms", name, timeout.Milliseconds()), start), nil
		}
	}
}
