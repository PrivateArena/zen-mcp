# REPORT: parallel zfetch / zsearch returned the wrong page (Firefox bridge concurrency)

Scope: why two bridge-backed tool calls issued at the same time (zfetch × N, zsearch, zbrowser read) came back with **one URL's provenance line over another URL's content**, and what was changed so the calls are served one at a time in arrival order.

Measured on: `zen-mcp` @ `be4bc72` (before) and the queue patch (after), live Zen bridge `127.0.0.1:9999`, 2026-10-24.
Fixture: `.scratch/bridge-race/` (gitignored — keep a copy). `pages.py` is a two-line Python server that tags every response with its own path (`/one` → `PAGE one`, `marker-one-…`), `race.sh` fires two `fetch` calls simultaneously through the MCP endpoint and reports the tag that actually came back.

## TL;DR — verdict

| # | Finding | Where the damage happens | Fix |
|---|---|---|---|
| 1 | Every bridge action addresses the **active tab**; there is no tab handle in the request or the response, so the last `navigate` wins for every reader | Bridge protocol (`userChrome.js`) | Cannot be fixed from the MCP side — mitigated by never overlapping callers |
| 2 | `fetch` / `search` / `browser read` are **multi-step** flows (`navigate → wait → read`), and nothing made the sequence atomic | **zen-mcp** (`internal/tools/fetch.go`, `search.go`, `browser.go`) | `bridge.WithTurn` reserves the bridge for the whole sequence |
| 3 | `bridge.CallBridge` was fully unbounded — no queue, no limit, one throw-away `http.Client` per call, so N concurrent tools = N racing navigations | **zen-mcp** (`internal/bridge/bridge.go`) | FIFO `Gate` in `internal/bridge/gate.go`, one turn by default |
| 4 | A queue with no ceiling would turn one wedged bridge POST into a hang for every bridge tool (`CallBridge` deliberately has no client deadline) | **zen-mcp** (new) | `bridge.queueTimeoutMs` (default 120 s) → `ErrBridgeBusy` naming the holder |
| 5 | `currentContainer` was a package-level `string` written by every `browser` call — an unsynchronized global, i.e. a data race under `-race` | **zen-mcp** (`internal/tools/browser.go`) | Removed (it was never read) |

Findings 2–5 are fixed. Finding 1 is the reason the fix has to be a queue rather than a retry: the client cannot tell the bridge *which* tab it means.

---

## Evidence `[VERIFIED]`

### Before — the fixture, through the real bridge and the real server

`python3 pages.py 8901` then 5 rounds of `race.sh` against `be4bc72` (same test done with the `zfetch` wrapper, `-i 300`):

```
round=1 call=a expected=1 got=2      # zfetch(/1) returned PAGE 2
round=1 call=b expected=2 got=2
round=2 call=a expected=1 got=2
round=2 call=b expected=2 got=2
round=3 call=a expected=1 got=2
round=4 call=a expected=1 got=2
round=5 call=a expected=1 got=1
round=5 call=b expected=2 got=1      # and now the other way round
```

The damage is silent because the response *looks* right — the provenance line is built from the `navigate` report, the body from the `read` of whatever tab won:

```
url: http://127.0.0.1:8899/1 | title: Page 1 | status: 200

# PAGE 2

marker-2-xxxxxxx...
```

This is the worst failure mode for an agent: 9 of 10 parallel fetches returned content that is attributed to the wrong URL, and nothing in the payload says so.

### Root cause

`HandleFetchAction` is three independent bridge POSTs:

```go
nav, err := bridge.CallBridge(ctx, "navigate", navigateParams)          // opens the tab
bridge.CallBridge(ctx, "wait_for_network_idle", ...)                     // best effort
res, err := bridge.CallBridge(ctx, "read", readParams)                   // reads the ACTIVE tab
```

`CallBridge` just POSTs — no lock, no queue, and the MCP server happily runs two `tools/call` requests concurrently (each request is its own stateless session, and `pooling` can detach a bridge tool into a background job that keeps navigating). With two callers the interleaving is `navA navB readA readB`, and both reads see page B. `zsearch` (`navigate → wait_for_network_idle → wait_for_selector → get_text`) and `zbrowser -a read|search` have the same shape. `zbrowser -a chat` was the only action that survived this, because the *bridge* serializes chat runs internally — which is exactly the asymmetry reported.

### After — same fixture, patched server

`race.sh` (2 concurrent `fetch` calls, 6 rounds):

```
round=1 url=/one got=one OK     round=1 url=/two got=two OK
...
round=6 url=/one got=one OK     round=6 url=/two got=two OK
```

5-way fan-out (4 × zfetch + 1 × zrequest at once): every response carried its own page tag.

FIFO in practice — one holder (`-i 6000`) plus three callers that arrive 300 ms apart (`sec.nanos` start/end stamps):

```
holder  start=21.024 end=27.568 content=PAGE holder
second  start=21.625 end=28.223 content=PAGE second
third   start=21.926 end=28.886 content=PAGE third
fourth  start=22.227 end=29.527 content=PAGE fourth
```

Arrival order preserved, ~0.65 s apart, and every page correct.

---

## The fix

### 1. `internal/bridge/gate.go` — a FIFO turnstile, not a mutex

`Gate` is a ticket queue: tickets are handed out in arrival order and admitted strictly in ticket order. A mutex or a buffered-channel semaphore would fix the overlap but keep the *fairness* bug — an unordered wake-up means a losing caller keeps reading a stranger's tab, and under a burst it can lose forever.

```go
func (g *Gate) Acquire(ctx context.Context, label string) (release func(), err error)
func (g *Gate) Status() Status            // limit / active / waiting / holder / heldFor
type  ErrBridgeBusy struct{ ... }         // queue ceiling hit; names the holder
func DefaultGate() *Gate                  // what CallBridge goes through
```

* waiters are cancellable — a client that disconnects leaves the queue and the next caller is still admitted;
* `release` is idempotent (`sync.Once` + a ticket scan), so a `defer release()` plus a caller-side release cannot over-admit;
* a queue ceiling (`bridge.queueTimeoutMs`, default 120 s) converts "the bridge wedged while holding the turn" into a readable error instead of a stall of every bridge tool — `CallBridge` runs without a client deadline on purpose (see `TestCallBridgeNoClientDeadline`), so the turn really can be held for minutes.

### 2. `bridge.WithTurn` — atomicity for multi-step flows

```go
ctx, releaseTurn, err := bridge.WithTurn(ctx, "fetch "+targetURL)
if err != nil { return toolresponse.WrapErrorWithContext(ctx, "fetch", err, start) }
defer releaseTurn()
```

`CallBridge` reuses a turn already carried by `ctx` instead of queueing again, so `navigate → wait → read` is one indivisible unit, and nesting (`browser read` inside a pooled job, a tool calling a tool) can never deadlock on the queue. Callers that do not opt in still get per-call serialization, because `CallBridge` takes the turn itself — safety by default, so a new bridge tool cannot reintroduce the race by forgetting to queue.

Applied to: `HandleFetchAction`, `HandleSearchAction`, and the `read` / `search` actions of `HandleBrowserAction`.

### 3. Deliberate exemption: the chat-like actions

`chat`, `brainstorm` and `brainstorm_status` bypass the queue by default. They drive an AI page for up to the 900 s browser budget, so putting them in the lane would starve every fetch behind a conversation — and they are the one action the bridge already serializes internally. `"bridge": {"queueAgentActions": true}` opts into strict one-by-one for them too.

### 4. Config (`bridge` block, live-reloaded)

| Key | Default | Meaning |
|---|---|---|
| `maxConcurrent` | `1` | how many bridge operations may be in flight; `1` = the one-by-one pooling asked for |
| `queueTimeoutMs` | `120000` | how long a caller waits for its turn before `bridge busy` |
| `queueAgentActions` | `false` | also queue chat / brainstorm |

Read per `Acquire`, so a config edit takes effect without a restart (`TestGateFollowsConfigConcurrency`).

---

## Tests added

`internal/bridge/gate_test.go`

| Test | Guards |
|---|---|
| `TestGateAdmitsOneCallerAtATime` | 8 concurrent acquirers → peak overlap is exactly 1, gate left clean |
| `TestGateAdmitsInArrivalOrder` | FIFO: `first,second,third` — the fairness contract |
| `TestGateLimitAllowsBoundedParallelism` | `maxConcurrent: 3` is honoured and never exceeded |
| `TestGateCanceledWaiterLeavesTheQueue` | a canceled caller cannot wedge the queue behind it |
| `TestGateQueueCeilingReportsTheHolder` | `*ErrBridgeBusy` and the holder label in the message |
| `TestGateReleaseIsIdempotent` | double release cannot over-admit |
| `TestGateFollowsConfigConcurrency` | the config block is live |
| `TestCallBridgeSerializesConcurrentCalls` | 6 parallel `CallBridge` → the stub server never sees 2 requests at once |
| `TestCallBridgeTurnIsReentrantAcrossASequence` | the navigate/wait/read sequence is not interrupted |
| `TestWithTurnNestsWithoutDeadlock` | nested reservations take one turn, inner release is a no-op |
| `TestQueueAgentActionsDefaultOff` / `…CanBeMadeStrict` | the chat exemption and its opt-out |

`internal/tools/bridge_race_test.go` — drives the real handlers against a **stateful** bridge fake (`fakeBrowser`: `navigate` moves the active tab, `read`/`get_text` answer from it)

| Test | Guards |
|---|---|
| `TestParallelFetchesReadTheirOwnPages` | the reported bug: 3 rounds × 2 concurrent zfetch, each response must carry its own marker, never the other's, and the fake must never see 2 overlapping actions |
| `TestParallelSearchesReadTheirOwnOverviews` | same for the `navigate → wait → wait_for_selector → get_text` sequence |
| `TestFetchAndBrowserReadDoNotShareATab` | mixed workload: `browser read` must not move the tab under a running `fetch` |
| `TestFetchReportsABusyBridgeInsteadOfHanging` | a stuck bridge surfaces `bridge busy … turn held by stuck-browser.chat`, not a wrong page |

`internal/tools/pool_test.go`

| Test | Guards |
|---|---|
| `TestPoolListReportsBusyBridge` | `pool -a list` names the bridge turn holder and the queue depth when the bridge is busy, and stays a plain job list when it is idle — "who owns the browser" is one call away |

Mutation check `[VERIFIED]`: with the two `WithTurn` calls stubbed out, `TestParallelFetchesReadTheirOwnPages` fails with `fetch(.../two) returned another page's content: "# PAGE one …"` and `TestFetchAndBrowserReadDoNotShareATab` fails the same way — the new tests do guard the race, they do not merely pass alongside it.

---

## Verification ladder

1. `go build ./...`, `go vet ./...` — clean.
2. `go test -race ./internal/bridge/... ./internal/tools/... ./internal/mcpcfg/... ./internal/server/... ./internal/toolresponse/... ./internal/pooling/... -count=1` — all `ok`.
3. Live: fixture server + real bridge + the running server, `race.sh` and a 5-way fan-out — every call returned its own page; arrival order preserved.
4. Pre-existing failures, untouched by this change and failing identically at `be4bc72` (`internal/terminal`): `TestBuildWrapperScriptShort`, `TestGeneratePiPromptsWritesFiles`.

---

## Residual risk / follow-ups (not in this diff)

1. **Tab-scoped bridge API** `[HYPOTHESIS]` — the real cure for finding 1 is a `tab_id` on `read`/`get_text`/`wait_*` in `userChrome.js`, so two callers can work in parallel tabs instead of waiting. The Go side is then free to raise `maxConcurrent`; the gate already supports it.
2. **Long turn holders** — `CallBridge` has no client deadline by design, so a wedged bridge action holds the turn until the *bridge* answers; `queueTimeoutMs` only protects the waiters. A keep-alive/abort for the POST itself would shorten worst-case queueing.
3. **Queue observability** — `[VERIFIED]` `pool -a list` now carries a `bridge` field (`limit / active / waiting / holder / heldForMs / queue[]`) whenever the turn is occupied, so an agent can ask who owns the browser instead of guessing. Not yet exposed: a per-call `bridge wait time` in the tool's own provenance line, which would make slow queues visible without a second call.
