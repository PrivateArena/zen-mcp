# Architecture Plan — Tool-Call Pooling (long-running sub-agent jobs)

## Summary

The MCP server (`zen-mcp`) delegates to long-running sub-agents through 4 tools — `shell`, `browser`, `run`, `ui-vision` — that can take 1–30 minutes. Client-side tool-call timeouts are uncontrollable and undocumented, so today a job that outlives the client's patience is either **killed** (`ui-vision` passes the request ctx into `CommandContext`/`http.NewRequestWithContext`/`DelegateToWebAgent`) or **orphaned** (`browser`/`shell`/`run` continue in the background but their result is lost). This plan adds an opt-in, config-driven **tool-level job pool**: a wrapped tool call that exceeds a configurable elapsed window returns a `pool_id` payload *after the full elapsed window* (never immediately); a new `pool` tool server-side long-polls the in-memory job registry and replays the eventual result. No client/protocol dependence (no MCP Tasks, no undocumented timeouts) — purely portable, "install and forget", and kill-switchable from `config.json`.

System boundaries are: MCP HTTP surface (`tools/call`) → registration-time wrapping in `RegisterAllTools` → a new `internal/pooling` package (wrapper + registry) → the existing tool handlers (unchanged) → a new `pool` MCP tool for status/poll/cancel/list.

**Interview decisions baked in:** single `elapsedMs` knob (default 60000, configurable) governs both the first-window conversion and each poll's block window; `pool` tool is always registered; `cancel` is soft (mark + evict, process not killed); context is detached with `context.WithoutCancel` (which also fixes the `ui-vision` client-abort-kills-job bug for free).

---

## System boundaries & components

```
                      ┌────────────────────────────────────────────────────┐
 MCP client           │  zen-mcp (Go, stateless HTTP POST /mcp, mcp-go)   │
 (opencode etc.)      │                                                    │
   │ tools/call       │  internal/server/routes.go → StreamableHTTPServer  │
   ▼                  │   → handleMessage → tool handler (synchronous)     │
 ┌──────────┐         │         │                                          │
 │ tools:catalog      │  internal/server/tools.go RegisterAllTools         │
 │ tools/list         │   (both listeners: mcpPort + cliPort)              │
 └──────────┘         │            │                                       │
                      │            v                                       │
                      │   [pooling.Wrap]  ── inner(Context{WithoutCancel}) ─┐
                      │     │  completes ≤ elapsed → return result          │
                      │     │  exceeds elapsed   → {status:running,pool_id} │
                      │     ▼                                               │
                      │   internal/pooling.Registry (in-memory, cap+TTL)    │
                      │     ▲                                               │
                      │   internal/tools/pool.go  defPool (poll/status/     │
                      │   cancel/list)  → long-polls Registry ≤ elapsed     │
                      └────────────────────────────────────────────────────┘
   unchanged handlers: shell.go(H41) browser.go(H61) run.go(H53) uivision.go(H37)
   config source:      mcpcfg.ZenConfig.Pooling  (hot-reload atomic.Pointer)
```

### New components (with homes)

| Component | Home | Role |
|---|---|---|
| `Job` / `Registry` / `Global()` | `internal/pooling/registry.go` (new) | In-memory job store, `LongPoll`, TTL reaper, cap, soft cancel |
| `Wrap` | `internal/pooling/wrap.go` (new) | Black-box handler wrapper: detach ctx, elapsed-window conversion |
| `defPool` / `HandlePoolAction` | `internal/tools/pool.go` (new) | The `pool` MCP tool (always registered) |
| Pooling config | `internal/mcpcfg/config.go` (modify) | `ZenConfig.Pooling`, defaults, deep-merge key |
| Wiring | `internal/server/tools.go` (modify) | Wrap `def.Handler` when tool ∈ `Pooling.Tools` |
| Tool descriptions | `internal/tools/shell.go`, `browser.go`, `run.go`, `uivision.go` (modify) | State the `pool_id` contract in each Description |
| Registration | `internal/tools/types.go` (modify) | Add `defPool(deps)` to `AllDefs` |

**Verified constraints that shape the design:**
- Production applies **no** wrapper today: `RegisterAllTools` (`server/tools.go:45`) registers `def.Handler` raw; `WrapHandlerWithTimeout` (`patch.go:38`) is test-only.
- `mcp-go` v0.48.0 runs tool handlers synchronously in the HTTP goroutine and, after `HandleMessage` returns, skips the response write if `ctx.Err() != nil` (`streamable_http.go:521`) — so a client abort can never cause a write-on-closed-connection; each request has a fresh `r.Context()` even on keep-alive. Cancellation is purely cooperative.
- `bridge.CallBridge` already detaches with `context.WithoutCancel` (`bridge.go:56`); `exec.Run`/`RunSandbox` take no ctx; only `ui-vision` uses the request ctx destructively.
- `toolresponse.WrapSuccess` (`response.go:300`) renders text, logs a telemetry row, and applies the virtualizer **at construction**; `WrapErrorWithContext` (`response.go:368`) logs a failure row at construction. Replay of a pre-built `*mcp.CallToolResult` therefore does **not** double-log — the interim "running" response is the only one that must suppress its telemetry row (via `toolresponse.MarkWithOrphanFlag`).
- REPL: `terminal/commander.go:427-451` `ExecuteTool` calls `tools.HandleXAction` **directly**, bypassing `RegisterAllTools` — REPL is out of scope (human-driven, no benefit).

---

## Data flow & state management

```
 wrapped tool call (tools/call: shell|browser|run|ui-vision)
   │ pooling.Enabled(name)?
   │   ├─ no  → inner(ctx, req)  → result  (today's behavior, exactly)
   │   └─ yes ─► goroutine A: out := inner(context.WithoutCancel(ctx), req); ch <- out
   │             select {
   │               // fast path
   │               case out := <-ch:  return out                      (result immediately)
   │               // conversion / client-abort (Q1 fix): register NOW, never wait out a dead window
   │               case <-timer | <-ctx.Done():
   │                   id := reg.Register(job)
   │                   go goroutine B: out := <-ch; job.Result = out; close(job.Done)  // assign BEFORE close
   │                   select { case <-job.Done: return job.Result   // boundary race
   │                            default:        return {status:running, pool_id:id} }
   │             }
   ▼
 pool tool (action=poll, pool_id)
   │ reg.LongPoll(ctx,id,elapsed)  → select {
   │    case <-job.Done:   return job.Result (stored pre-wrapped, replay verbatim)
   │    case <-timer:      return {status:running,pool_id}
   │    case <-ctx.Done(): return (aborted; nothing written by mcp-go)
   │    cancelled:         return {status:cancelled}
   │    not found:         error "unknown pool_id (job expired or server restarted)"
   │ }
   ▼
 registry lifecycle (internal/pooling/registry.go)
   Register   id = "pool-" + 16-hex-from-crypto/rand   Job{CreatedAt: now}
   Complete   job.Result set, close(job.Done)   FinishedAt: now
   Cancel     job.Cancelled = true (soft; underlying process NOT killed)
   Retrieve   (poll returns held *Job pointer — eviction never invalidates it)
   Reaper     every 60s, sync.Once-started:
                running/cancelled-unevicted → evict when now-CreatedAt > TTL
                done → evict when now-FinishedAt > TTL   // post-completion grace, NOT creation-based
              cap: MaxJobs (256) — Register returns error when full
```

State is entirely in-memory and process-local. **Losing jobs on server restart (incl. `.air` dev auto-restart) is accepted** and surfaced as a distinct `unknown pool_id (expired or restarted)` error so the LLM re-issues the original tool call instead of re-polling a black hole.

---

## Implementation Blueprint

Ordered so any agent can build one step in isolation. `Done when` gates are the acceptance criteria.

**Step 0 — `config.json` (optional, user side)**
```json
"pooling": { "enabled": true, "tools": ["shell","browser","run","ui-vision"], "elapsedMs": 60000, "ttlMinutes": 60, "maxJobs": 256 }
```

---

| # | File (path) | Action | Concrete signature / schema | Depends on | Done when |
|---|---|---|---|---|---|
| 1 | `internal/mcpcfg/config.go` | modify | Add to `ZenConfig` (after `ToolTimeouts`, line ~108): `Pooling PoolingConfig \`json:"pooling"\``. Add type: `type PoolingConfig struct { Enabled bool \`json:"enabled"\`; Tools []string \`json:"tools,omitempty"\`; ElapsedMs int \`json:"elapsedMs"\`; TTLMinutes int \`json:"ttlMinutes,omitempty"\`; MaxJobs int \`json:"maxJobs,omitempty"\` }`. In `defaultConfig()`: `Pooling: PoolingConfig{Enabled:false, Tools:[]string{"shell","browser","run","ui-vision"}, ElapsedMs:60000, TTLMinutes:60, MaxJobs:256}`. In `mergeConfig` add `"pooling"` to the deep-merge key slice (line 380). | none | `go test -tags fts5 ./internal/mcpcfg` green with new `TestPoolingConfigDefaults` (assert defaults + a partial user override merges: tools replaced, elapsedMs defaults preserved when omitted) |
| 2 | `internal/pooling/registry.go` | create | `package pooling`; `type Job struct { ID string; Cancelled bool; Done chan struct{}; Result *mcp.CallToolResult; CreatedAt, FinishedAt time.Time }`; `type Registry struct { mu sync.Mutex; jobs map[string]*Job; ttl, grace time.Duration; max int }`; `func NewRegistry(ttl, grace time.Duration, max int) *Registry`; `func (r *Registry) Register(job *Job) (id string, err error)` (caps at max, evicts oldest expired first); `func (r *Registry) Complete(id string, res *mcp.CallToolResult)` (assign then `close(Done)`); `func (r *Registry) Cancel(id string) bool`; `func (r *Registry) Get(id string) (*Job, bool)`; `func (r *Registry) LongPoll(ctx context.Context, id string, wait time.Duration) PollOutcome` where `type PollOutcome struct { State string; Result *mcp.CallToolResult }`, State ∈ `{"done","running","cancelled","unknown"}` (select on `<-job.Done`, `<-time.After(wait)`, `<-ctx.Done()`, checked against `job.Cancelled` first); `func (r *Registry) List() []JobInfo` (`type JobInfo struct { ID string; State string; AgeMs int64 }`); `func (r *Registry) EvictExpired(now time.Time) int` (running: age-from-CreatedAt > ttl; done/cancelled: age-from-FinishedAt > grace); `var globalOnce sync.Once; func Global() *Registry` (initializes NewRegistry(60m TTL, 60m grace, 256) from `mcpcfg.Get().Pooling` and starts a 60s reaper goroutine). | Step 1 | `go test -tags fts5 ./internal/pooling` — `registry_test.go`: register/complete/close-order, duplicate polls (two receivers on one closed `Done` both get the result), LongPoll running→done transition, cancel→`cancelled`, `unknown` for missing id, EvictExpired keeps done-but-unpolled for `grace` and evicts after, cap rejects registration at max |
| 3 | `internal/pooling/wrap.go` | create | `func Wrap(name string, inner toolregistry.Handler) toolregistry.Handler` (reads `mcpcfg.Get().Pooling` at **call time** for live toggle); behavior per data-flow diagram: goroutine A `inner(context.WithoutCancel(ctx), req)`; first select on `ch` / `elapsedTimer` / `<-ctx.Done()`; on timer-or-abort: `reg.Register` (propagate error if full via `toolresponse.WrapErrorWithContext`), start goroutine B to drain `ch` into `job.Result` then `close(job.Done)`, recheck `<-job.Done` vs `default`, build running response via `toolresponse.WrapSuccess(ctx, name, map{"status":"running","pool_id":id,"hint":"poll via pool tool: {\"action\":\"poll\",\"pool_id\":\"<id>\"}"}, start)` on a ctx carrying `toolresponse.MarkWithOrphanFlag` (suppresses the interim telemetry row). Add panic-recover in goroutine A. Add private `func summarize(args map[string]any) string` (400-char truncation, mirrors `server.SummarizeParams`) for any pooling log lines. | Steps 1, 2 | `go test -tags fts5 ./internal/pooling` — `wrap_test.go`: fast handler (< elapsed) returns result untouched; slow handler (> elapsed) returns payload with `pool_id` and elapsed ≥ configured; client-cancel (`ctx` canceled before timer) registers the job immediately (assert job exists in registry after short wait, ~0ms not ~elapsed); returned `pool_id` then resolves via `LongPoll`; panic in inner yields a stored error result; registry-full returns error not a running payload |
| 4 | `internal/server/tools.go` | modify | Around line 44 (`handler := def.Handler`): `if mcpcfg.Get().Pooling.Enabled && slices.Contains(mcpcfg.Get().Pooling.Tools, def.Name) { handler = pooling.Wrap(def.Name, handler) }`. Ensure exactly one wrapped `handler` flows into both `srv.AddTool` closure and `reg.Track`. The `pool` tool name never appears in `Pooling.Tools` so it is never self-wrapped. | Steps 1, 2, 3 | `go build -tags fts5 .` green; existing `routes_test.go`/`patch_test.go` still pass; manual: enable pooling with a stubbed slow def in a throwaway test, `tools/call` returns running payload after elapsed |
| 5 | `internal/tools/pool.go` | create | `func defPool(deps Deps) ToolDef` — Name `"pool"`, Title `"Long-Running Job Pool"`, Description describes actions + plain-text `pool_id` contract; Schema `jsonSchema(map{ "action": strEnumProp("poll|status|cancel|list", []string{"poll","status","cancel","list"}), "pool_id": strProp("pool id returned by a pooled tool call (required for poll/status/cancel)") }, []string{"action"})`; `func HandlePoolAction(ctx context.Context, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult` — reads `mcpcfg.Get().Pooling`; **disabled → error** "pooling is disabled in config.json for this server"; `poll` → `reg.LongPoll(ctx, id, elapsed)`; `done` → return `outcome.Result` verbatim (no re-wrap, no telemetry); `running` → `WrapSuccess` with running payload; `cancelled` → cancelled payload; `unknown` → error "unknown pool_id \"<id>\" (job expired or server restarted — re-issue the original call)"; `status` → immediate `Get`; `cancel` → `reg.Cancel`, respond cancelled; `list` → `reg.List` JSON. | Step 2 | `go test -tags fts5 ./internal/tools` — `pool_test.go`: completed job replays stored result verbatim, running after wait, cancelled, unknown after expiry/restart text, list shape; `pooling disabled` error when `Enabled=false` |
| 6 | `internal/tools/types.go` | modify | Add `defPool(deps)` to `AllDefs` return list (after `defUiVision`, keeping TS registration order stable otherwise). | Step 5 | `tools/list` (via catalog/`FilterEnabled`) includes `pool`; existing `types` tests/build green |
| 7 | `internal/tools/shell.go`, `browser.go`, `run.go`, `uivision.go` | modify | Append one paragraph to each `Description` string: `Long-running jobs (> Ns) return {"status":"running","pool_id":...} — poll with the pool tool (action="poll").` (N from `defaultConfig` `ElapsedMs`). No other logic changes. | Step 1 | Descriptions mention the pool contract; `go build -tags fts5 .` green; no behavioral change when pooling disabled |
| 8 | `internal/mcpcfg/config_test.go` | modify | Extend with pooling defaults/merge cases (see Step 1 gate). | Step 1 | Covered by Step 1 acceptance |
| 9 | Integration pass | verify | `go vet -tags fts5 ./...`; `go test -tags fts5 ./...`; `go build -tags fts5 -o zen-mcp .`; manual smoke with `.air` running: enabling pooling in `config.json` works **without restart** (wrapper re-reads config per call); toggling `enabled:false` restores today's behavior exactly. | Steps 1–8 | full suite green; live-toggle verified by `/mcp` `tools/call` against a long `sleep 90` shell command, then two `pool` polls |

---

## Failure modes & guards

| Failure mode | Guard (file / function) |
|---|---|
| Client aborts during first window; job kept waiting 60s pointlessly, registration delayed | `wrap.go`: third select case `<-ctx.Done()` registers immediately (red-team Q1 fix); mcp-go skips the write on canceled ctx (`streamable_http.go:521`) — nothing else to do |
| Client disconnect after job completes — response write race on keep-alive conn | Non-issue by construction: each request gets a fresh `r.Context()`; verified against mcp-go source (red-team Q1) |
| Completion races the conversion timer (job finishes right as we register) | `wrap.go`: post-register recheck via `select { <-job.Done … }`; goroutine B always assigns `job.Result` **before** `close(job.Done)` (channel happens-before), so any `Done` observer sees the result (red-team Q2b) |
| Duplicate/simultaneous polls of one pool_id | `registry.go`: multiple receivers on a closed channel is safe; `LongPoll` holds the `*Job` pointer (red-team Q2a/Q2c) |
| TTL reaper deletes a finished-but-unpolled 25-min job before the LLM polls | `registry.go`: reaper evicts done jobs by age-from-`FinishedAt` (grace `TTLMinutes`), running jobs by age-from-`CreatedAt` (red-team Q2c) |
| Registry exhausted by concurrent long jobs | `wrap.go` + `registry.go`: `MaxJobs` (256) cap; `Register` error → `WrapErrorWithContext` explicit, retryable (red-team Q3) |
| Server/.air restart mid-job loses all jobs | `pool.go`: distinct error text `unknown pool_id (job expired or server restarted — re-issue the original call)` (red-team Q8) |
| Double telemetry / re-virtualizer on replay | `wrap.go`: interim running response built on an orphan-flagged ctx (suppresses its row); stored result is the already-wrapped `*mcp.CallToolResult`, replayed verbatim by `pool poll` with no re-wrap — verified `WrapSuccess` side effects happen once at construction (red-team Q5) |
| Underlying tool returns an error result or panics mid-job | `wrap.go`: panic-recover in goroutine A; error results stored and retrievable via poll like successes |
| `pool` called with unknown/expired id | `pool.go`: `unknown` state → actionable error, never ambiguous silence |
| CLI/REPL path without wrapping | Documented scope: REPL `ExecuteTool` (`commander.go:427`) calls handlers directly and stays unwrapped; CLI **wrapper scripts** hit the HTTP `cliPort` listener so they ARE wrapped (both listeners share the singleton `Global()` registry) (red-team FM1) |
| Config hot-reloaded mid-job changes polling cadence | `wrap.go`/`pool.go`: config re-read per call at invocation; doc: a poll started under new `elapsedMs` finishes under it (red-team FM2) |
| Sensitive args (shell command, ui-vision message) leaked in pool logs | `wrap.go` private `summarize()` 400-char truncation, mirroring `SummarizeParams`; only `pool_id` + status logged (red-team FM3) |

---

## Key decisions & alternatives

1. **Bespoke `internal/pooling` vs MCP-protocol Tasks (SEP-1686 / mark3labs native `TaskSupport`)** — Rejected for this increment: verified natively (`tools.go:41` sets `TaskSupportForbidden`; `executeRegularToolAsTask` derives `taskCtx` via `context.WithCancel` off the request ctx with no detach) so the library has the *same* context-lifetime bug and still needs our `WithoutCancel` fix; and flipping `TaskSupport` changes `tools/list` semantics per client — the user's own goal is client portability ("install and forget"). If clients later standardize on MCP Tasks, the wrapper is a thin shim away.
2. **Live config toggle (per-call read) vs baked-in at registration** — Chose per-call read of `mcpcfg.Get().Pooling`: toggling `enabled`/`elapsedMs` in `config.json` takes effect without a server restart, matching the project's hot-reload convention.
3. **Store pre-wrapped `*mcp.CallToolResult` vs store raw `(data, err)` and re-wrap at poll time** — Chose pre-wrapped replay: re-wrapping at poll time would emit a second telemetry row with a wrong (poll-time) elapsed and lose the completion row if never polled. Verified against `response.go:300`.
4. **`elapsedMs` as a single knob (60s) for both conversion and poll-wait vs separate `pollWaitMs` (longer) or 45s default** — Kept the single 60000ms knob per user decision, re-confirmed after the red-team presented the ~8-turn savings of a 4–5 min poll window against the just-observed 300s client abort. Rationale: the design premise is "return before *any* client's undocumented timeout"; 60s is the safe convergent point. Configurable in `config.json` if a given client is known to tolerate longer.
5. **Soft cancel (mark + evict) vs process SIGKILL** — Chose soft: the wrapper is black-box with no process handle; hard-kill would require plumbing `internal/shell/processes` tracking per-job into 4 tools for marginal benefit. Underlying shell/bridge work continues to completion.
6. **Detach ctx for ALL wrapped calls (`context.WithoutCancel`), not only on conversion** — Accepted the minor loss (a fast call can no longer be client-cancelled once started) because shell/run were never cancellable anyway, browser already detaches, and the win — `ui-vision` jobs and the launched app surviving client abort — is the core fix this feature exists for.
7. **Always register `pool` tool vs gate on `enabled`** — Always registered (user decision): harmless shell when disabled, allows manual probing, keeps `tools/list` stable when toggling.

---

## Red-team critique summary (browser.chat → claude, source-verified vs mark3labs/mcp-go v0.48.0)

| Point | Verdict | Disposition |
|---|---|---|
| Q1: no write-on-closed-stream risk (verified streamable_http.go:521); wrapper must select on `ctx.Done()` in first window | ACCEPT-WITH-CHANGE | **Folded in**: third select case in `wrap.go`; register on abort |
| Q2a/Q2b: duplicate polls safe; assign-before-close; recheck via `<-Done` not bare map read | ACCEPT-WITH-CHANGE | **Folded in**: goroutine B ordering + post-register `select` |
| Q2c: creation-based TTL would reap completed-unpolled jobs | ACCEPT-WITH-CHANGE | **Folded in**: post-completion grace from `FinishedAt` |
| Q3: goroutine-per-job unbounded | REJECT premise (single-user/loopback) | **Folded in as cheap insurance**: `MaxJobs` cap + explicit error |
| Q4: raise poll window to 4–5 min to cut turns (~8 not ~30) | REJECT | **Resolved via follow-up interview**: user kept 60000ms single configurable knob |
| Q5: verbatim replay may double-log telemetry / re-run virtualizer | ACCEPT-WITH-CHANGE + asked for response.go | **Folded in**: read `response.go` locally; replay verbatim is correct because construction side effects happen once; orphan-flag only the interim running response |
| Q6: LLM must learn contract from `tools/list`; keep running payload rendering consistent | ACCEPT + missed piece | **Folded in**: 4 Description edits (Step 7); running payload via `WrapSuccess` (same render path) |
| Q7: "wrapper not needed" + discovered native Tasks | REJECT | Rejected per decision #1 (client portability + same ctx bug in library) |
| Q8: `.air` restart realism; singleton registry intent; CLI listener | ACCEPT-WITH-CHANGE | **Folded in**: distinct `unknown pool_id (expired or restarted)` error; explicit process-wide `Global()` singleton |
| FM1: REPL `ExecuteTool` bypasses wrapping | NEW | **Resolved via local verification**: real (commander.go:427); documented as out-of-scope (human-driven REPL) |
| FM2: hot-reload mid-poll cadence inconsistency | NEW | **Folded in**: per-call config read, documented |
| FM3: sensitive args in new logs | NEW | **Folded in**: private 400-char `summarize()` in `wrap.go` |

---

## Open questions

None. The only red-team point routed to you (Q4) was resolved in the follow-up interview; all facts (mcp-go behavior, `response.go` side effects, REPL path) were verified against the local codebase and library source rather than deferred.