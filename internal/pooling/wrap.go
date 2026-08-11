package pooling

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/toolresponse"
)

type result struct {
	res *mcp.CallToolResult
	err error
}

func Wrap(name string, inner toolregistry.Handler) toolregistry.Handler {
	return WrapWithRegistry(name, inner, Global)
}

func WrapWithRegistry(name string, inner toolregistry.Handler, regFn func() *Registry) toolregistry.Handler {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		cfg := mcpcfg.Get().Pooling
		if !cfg.Enabled || !toolInList(name, cfg.Tools) {
			return inner(ctx, req)
		}

		elapsed := time.Duration(cfg.ElapsedMs) * time.Millisecond
		if elapsed <= 0 {
			elapsed = 60 * time.Second
		}

		params := req.GetArguments()
		ch := make(chan result, 1)
		innerCtx := context.WithoutCancel(ctx)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- result{res: toolresponse.WrapErrorWithContext(innerCtx, name, fmt.Errorf("panic: %v", r), start), err: nil}
				}
			}()
			out, err := inner(innerCtx, req)
			ch <- result{res: out, err: err}
		}()

		timer := time.NewTimer(elapsed)
		defer timer.Stop()

		select {
		case r := <-ch:
			if r.err != nil {
				return toolresponse.WrapErrorWithContext(innerCtx, name, r.err, start), nil
			}
			return r.res, nil
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}

		orphaned := new(atomic.Bool)
		ctx = toolresponse.MarkWithOrphanFlag(ctx, orphaned)

		reg := regFn()
		job := &Job{Done: make(chan struct{})}
		id, err := reg.Register(job)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, name, err, start), nil
		}

		go func() {
			r := <-ch
			if r.err != nil {
				job.Result = toolresponse.WrapErrorWithContext(innerCtx, name, r.err, start)
			} else {
				job.Result = r.res
			}
			close(job.Done)
		}()

		select {
		case <-job.Done:
			return job.Result, nil
		default:
		}

		_ = summarize(params)
		return toolresponse.WrapSuccess(ctx, name, map[string]any{
			"status":  "running",
			"pool_id": id,
			"hint":    "poll via pool tool: {\"action\":\"poll\",\"pool_id\":\"" + id + "\"}",
		}, start), nil
	}
}

func toolInList(name string, list []string) bool {
	for _, t := range list {
		if t == name {
			return true
		}
	}
	return false
}

func summarize(args map[string]any) string {
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
