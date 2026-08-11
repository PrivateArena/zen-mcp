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

func defPool(deps Deps) ToolDef {
	return ToolDef{
		Name:        "pool",
		Title:       "Long-Running Job Pool",
		Description: "Poll or manage long-running jobs that returned a pool_id. Actions: poll (block until done), status (immediate snapshot), cancel (soft cancel), list (all tracked jobs).",
		Schema: jsonSchema(map[string]any{
			"action":  strEnumProp("poll|status|cancel|list", []string{"poll", "status", "cancel", "list"}),
			"pool_id": strProp("pool id returned by a pooled tool call (required for poll/status/cancel)"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandlePoolAction(ctx, deps, req), nil
		},
	}
}

func HandlePoolAction(ctx context.Context, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	cfg := mcpcfg.Get().Pooling
	if !cfg.Enabled {
		return toolresponse.WrapErrorWithContext(ctx, "pool", fmt.Errorf("pooling is disabled in config.json for this server"), start)
	}

	args := req.GetArguments()
	action, _ := args["action"].(string)

	switch action {
	case "poll":
		poolID, _ := args["pool_id"].(string)
		if poolID == "" {
			return toolresponse.WrapErrorWithContext(ctx, "pool", fmt.Errorf("pool_id is required for action 'poll'"), start)
		}
		elapsed := time.Duration(cfg.ElapsedMs) * time.Millisecond
		if elapsed <= 0 {
			elapsed = 60 * time.Second
		}
		outcome := pooling.GlobalRegistry().LongPoll(ctx, poolID, elapsed)
		switch outcome.State {
		case "done":
			return outcome.Result
		case "running":
			return toolresponse.WrapSuccess(ctx, "pool", map[string]any{
				"status":  "running",
				"pool_id": poolID,
			}, start)
		case "cancelled":
			return toolresponse.WrapSuccess(ctx, "pool", map[string]any{
				"status":  "cancelled",
				"pool_id": poolID,
			}, start)
		default:
			return toolresponse.WrapErrorWithContext(ctx, "pool", fmt.Errorf("unknown pool_id %q (job expired or server restarted — re-issue the original call)", poolID), start)
		}
	case "status":
		poolID, _ := args["pool_id"].(string)
		if poolID == "" {
			return toolresponse.WrapErrorWithContext(ctx, "pool", fmt.Errorf("pool_id is required for action 'status'"), start)
		}
		job, ok := pooling.GlobalRegistry().Get(poolID)
		if !ok {
			return toolresponse.WrapErrorWithContext(ctx, "pool", fmt.Errorf("unknown pool_id %q (job expired or server restarted — re-issue the original call)", poolID), start)
		}
		state := "running"
		if job.Cancelled {
			state = "cancelled"
		}
		select {
		case <-job.Done:
			state = "done"
		default:
		}
		return toolresponse.WrapSuccess(ctx, "pool", map[string]any{
			"status":  state,
			"pool_id": poolID,
		}, start)
	case "cancel":
		poolID, _ := args["pool_id"].(string)
		if poolID == "" {
			return toolresponse.WrapErrorWithContext(ctx, "pool", fmt.Errorf("pool_id is required for action 'cancel'"), start)
		}
		if pooling.GlobalRegistry().Cancel(poolID) {
			return toolresponse.WrapSuccess(ctx, "pool", map[string]any{
				"status":  "cancelled",
				"pool_id": poolID,
			}, start)
		}
		return toolresponse.WrapErrorWithContext(ctx, "pool", fmt.Errorf("unknown pool_id %q (job expired or server restarted — re-issue the original call)", poolID), start)
	case "list":
		jobs := pooling.GlobalRegistry().List()
		out := make([]map[string]any, 0, len(jobs))
		for _, j := range jobs {
			out = append(out, map[string]any{
				"pool_id": j.ID,
				"status":  j.State,
				"age_ms":  j.AgeMs,
			})
		}
		return toolresponse.WrapSuccess(ctx, "pool", map[string]any{
			"jobs": out,
		}, start)
	default:
		return toolresponse.WrapErrorWithContext(ctx, "pool", fmt.Errorf("unknown action %q (poll|status|cancel|list)", action), start)
	}
}
