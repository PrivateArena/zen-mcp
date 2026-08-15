package tools

import (
	"context"
	"fmt"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/pooling"
	"zen-mcp/internal/toolresponse"
)

// defPool is a helper function
func defPool(deps Deps) ToolDef {
	return ToolDef{
		Name:  "pool",
		Title: "Long-Running Job Pool",
		Description: "Poll or manage pooled jobs. Wrapped tools (shell, browser, run, ui-vision) " +
			"return {status,pool_id} when they exceed the pooling window; use this tool " +
			"to poll, check status, cancel, or list live jobs.",
		Schema: jsonSchema(map[string]any{
			"action":  strEnumProp("Pool action.", []string{"poll", "status", "cancel", "list"}),
			"pool_id": strProp("Pool id returned by a pooled tool call (required for poll/status/cancel)"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandlePoolAction(ctx, pooling.Global(), req), nil
		},
	}
}

// HandlePoolAction implements the pool tool. It NEVER creates a job: poll and
// status only look up an existing pool_id, so polling can never mint a second
// pool_id. A missing/unknown id yields an explicit error directing the LLM to
// re-issue the original call.
func HandlePoolAction(ctx context.Context, reg *pooling.Registry, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)
	id, _ := args["pool_id"].(string)

	pc := mcpcfg.Get().Pooling
	if !pc.Enabled {
		return toolresponse.WrapErrorWithContext(ctx, "pool",
			fmt.Errorf("pooling is disabled in config.json for this server"), start)
	}
	elapsed := time.Duration(pc.ElapsedMs) * time.Millisecond
	if elapsed <= 0 {
		elapsed = 60000 * time.Millisecond
	}

	switch action {
	case "poll":
		if id == "" {
			return toolresponse.WrapErrorWithContext(ctx, "pool",
				fmt.Errorf("pool_id is required for action %q", action), start)
		}
		outcome := reg.LongPoll(ctx, id, elapsed)
		switch outcome.State {
		case pooling.StateDone:
			// Replay the pre-wrapped result verbatim: telemetry and rendering
			// side effects already happened once at construction.
			return outcome.Result
		case pooling.StateCancelled:
			return toolresponse.WrapSuccess(ctx, "pool",
				statusPayload(pooling.StateCancelled, id, reg), start)
		case pooling.StateUnknown:
			return toolresponse.WrapErrorWithContext(ctx, "pool", unknownPoolIDError(id), start)
		default:
			return toolresponse.WrapSuccess(ctx, "pool",
				statusPayload(pooling.StateRunning, id, reg), start)
		}
	case "status":
		if id == "" {
			return toolresponse.WrapErrorWithContext(ctx, "pool",
				fmt.Errorf("pool_id is required for action %q", action), start)
		}
		state := reg.State(id)
		if state == pooling.StateUnknown {
			return toolresponse.WrapErrorWithContext(ctx, "pool", unknownPoolIDError(id), start)
		}
		return toolresponse.WrapSuccess(ctx, "pool", statusPayload(state, id, reg), start)
	case "cancel":
		if id == "" {
			return toolresponse.WrapErrorWithContext(ctx, "pool",
				fmt.Errorf("pool_id is required for action %q", action), start)
		}
		if reg.Cancel(id) {
			return toolresponse.WrapSuccess(ctx, "pool",
				statusPayload(pooling.StateCancelled, id, reg), start)
		}
		return toolresponse.WrapErrorWithContext(ctx, "pool", unknownPoolIDError(id), start)
	case "list":
		return toolresponse.WrapSuccess(ctx, "pool", map[string]any{"jobs": reg.List()}, start)
	default:
		return toolresponse.WrapErrorWithContext(ctx, "pool",
			fmt.Errorf("unknown pool action %q (poll|status|cancel|list)", action), start)
	}
}

// unknownPoolIDError is a helper function
func unknownPoolIDError(id string) error {
	return fmt.Errorf("unknown pool_id %q (job expired or server restarted — re-issue the original call)", id)
}

// statusPayload builds a status payload with the elapsed time (ms) since the
// job was registered, so agents see a monotonic progress signal instead of
// guessing how long the job has been running.
func statusPayload(state, id string, reg *pooling.Registry) map[string]any {
	return map[string]any{
		"status":    state,
		"pool_id":   id,
		"elapsedMs": jobElapsedMs(reg, id),
	}
}

// jobElapsedMs is a helper function
func jobElapsedMs(reg *pooling.Registry, id string) int64 {
	if job, ok := reg.Get(id); ok {
		return time.Since(job.CreatedAt).Milliseconds()
	}
	return 0
}
