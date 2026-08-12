package pooling

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/toolresponse"
)

// Wrap returns a black-box handler wrapper. When pooling is enabled at call
// time and name is in the configured Tools list, a call that exceeds the
// elapsed window is converted to a {status:"running",pool_id} payload instead
// of blocking forever; the pool tool long-polls the same registry. The request
// context is detached with context.WithoutCancel so background work survives
// client abort (this also fixes the ui-vision client-abort-kills-app bug).
//
// When pooling is disabled or name is not pooled, inner runs exactly as today.
//
// pool_id CONTRACT: exactly one id exists per job — the id returned by
// Register is simultaneously the registry map key, job.ID, and the value in
// the running payload. Nothing in this file (or the pool tool) ever mints a
// second id for an existing job. The "pool" tool is never wrapped, so polling
// can never itself spawn a job and a second pool_id.
func Wrap(name string, reg *Registry, inner toolregistry.Handler) toolregistry.Handler {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pc := mcpcfg.Get().Pooling
		if !pc.Enabled || name == "pool" || !containsStr(pc.Tools, name) {
			return inner(ctx, req)
		}

		elapsed := time.Duration(pc.ElapsedMs) * time.Millisecond
		if elapsed <= 0 {
			elapsed = 60000 * time.Millisecond
		}
		start := time.Now()

		type out struct {
			res *mcp.CallToolResult
			err error
		}
		ch := make(chan out, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- out{res: toolresponse.WrapErrorWithContext(ctx, name, fmt.Errorf("panic: %v", r), start)}
				}
			}()
			res, err := inner(context.WithoutCancel(ctx), req)
			ch <- out{res: res, err: err}
		}()

		timer := time.NewTimer(elapsed)
		defer timer.Stop()

		select {
		case r := <-ch:
			if r.err != nil {
				return toolresponse.WrapErrorWithContext(ctx, name, r.err, start), nil
			}
			return r.res, nil
		case <-timer.C:
		case <-ctx.Done():
		}

		// Conversion: register NOW so the job is never lost, then drain the
		// inner result in the background and replay it through the pool tool.
		id, err := reg.Register(name, &Job{})
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, name, err, start), nil
		}
		logfilter.Infof("[pooling] %s exceeded %dms — %s running", name, elapsed.Milliseconds(), id)

		go func() {
			r := <-ch
			if r.err != nil {
				reg.Complete(id, toolresponse.WrapErrorWithContext(ctx, name, r.err, start))
				return
			}
			reg.Complete(id, r.res)
		}()

		// Boundary race: the job may have finished between Register and here.
		// If so, hand back the real result instead of a pool_id.
		select {
		case <-jobDone(reg, id):
			job, _ := reg.Get(id)
			return job.Result, nil
		default:
		}

		// Interim running payload. Built on an orphan-flagged context so the
		// running row is NOT telemetry-logged; the eventual stored result is
		// the already-wrapped *CallToolResult and logs exactly one row at
		// construction (replayed verbatim, never re-wrapped).
		orphaned := new(atomic.Bool)
		orphaned.Store(true)
		interimCtx := toolresponse.MarkWithOrphanFlag(ctx, orphaned)
		payload := map[string]any{
			"status":    StateRunning,
			"pool_id":   id,
			"elapsedMs": time.Since(start).Milliseconds(),
			"hint":      fmt.Sprintf(`poll: pool("%s",%q)`, "poll", id),
		}
		return toolresponse.WrapSuccess(interimCtx, name, payload, start), nil
	}
}

// jobDone returns the job's Done channel when the job is still live, or a
// closed channel so the caller's non-blocking select never hangs. Kept inside
// the pooling package to avoid exposing Job mutation from Wrap.
func jobDone(reg *Registry, id string) <-chan struct{} {
	if job, ok := reg.Get(id); ok {
		return job.Done
	}
	closed := make(chan struct{})
	close(closed)
	return closed
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
