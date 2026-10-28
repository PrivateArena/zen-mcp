# ARCHITECTURE PLAN — `tools/youtube.go`: an autonomous YouTube channel tool

## Summary

A new MCP tool `youtube` that lets an agent run a real YouTube channel end to end: identify
the channel, list/read videos, read comment threads with pagination, post comments and
replies, upload a video through YouTube's own Studio wizard, edit metadata and visibility,
schedule publication, and delete. Every capability rides the **user's authenticated Firefox
container session** through the existing Firefox bridge (`internal/bridge.CallBridge`) — no
API keys, no OAuth, no `yt-dlp`, no cookie export. Structured reads and writes are performed
by **JS programs executed inside an authenticated youtube.com document via `web_eval`**,
which call YouTube's own `youtubei/v1` endpoints same-origin and return only the decoded
rows (so nothing approaches the bridge's 1 MB response cap). Media upload uses the
bridge's existing `upload` action against the Studio wizard, because the browser must do
the byte transfer itself. Long work is decomposed into **short tool calls joined by an
on-disk upload journal**, so the pooling subsystem is not touched and no MCP call blocks
for minutes. Upload policy is one video per day, dry-run-first, and a hard stop after any
failed attach.

Verified live during this planning session (container `pachoigame` = userContextId 20,
`SAPISID`/`SID` present ⇒ logged in):

| Probe | Evidence |
|---|---|
| `navigate back_tab container:pachoigame` | returns TabRegistry id `t_mvs58xbt_625`, `containerId:20` |
| `web_eval` world | `Cu.Sandbox(win,{sandboxPrototype:win})` (`JS/bridge/frame.js:222`) → page globals (`ytcfg`, `ytInitialData`) invisible; top-level `await` works (`frame.js:231`) and the returned promise is awaited (`frame.js:232`) |
| same-origin `fetch` from that sandbox | `POST https://www.youtube.com/youtubei/v1/next` → **HTTP 200** |
| comment continuation | token is in the watch HTML **before** `"sectionIdentifier":"comment-item-section"`; `POST /youtubei/v1/next` → 200, **21 `commentThreadRenderer`, 20 `commentEntityPayload`** |
| response shape | `onResponseReceivedEndpoints[].reloadContinuationItemsCommand.continuationItems[].commentThreadRenderer.commentViewModel` + `frameworkUpdates.entityBatchUpdate.mutations[].payload` (mutations are `{entityKey,type,payload}`, **not** `{commentEntityPayload}`) |
| innertube bootstrap from HTML | `"INNERTUBE_API_KEY":"AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"`, `WEB` / `2.20261027.01.00` |
| bridge `request` action | works but defaults to the **userContextId 0** jar (`action_handlers.mjs:141-151`), and `postBridge` caps replies at 1 MB (`internal/bridge/bridge.go`) → a 2.1 MB watch page arrives truncated. Rejected as the transport. |
| bridge `upload` action | `<input type=file>` only, crosses closed shadow roots, background-tab safe, fires `input`+`change`, optional `triggerDrop`, returns no progress (`action_handlers.mjs:588`, `frame.js:416-442`) |
| unknown bridge action | replies `success:true, data:null` HTTP 200 — must be treated as an error |

---

## System boundaries

```mermaid
graph TD
  subgraph zen-mcp
    T["tools/youtube.go<br/>defYoutube + HandleYoutubeAction"]
    Y["internal/youtube<br/>session · innertube · wizard · journal · errors · scripts"]
    BR["internal/bridge<br/>CallBridge · WithTabTurn · TabID"]
    TR["internal/toolresponse<br/>WrapSuccess · WrapErrorWithContext"]
    CFG["internal/mcpcfg<br/>YoutubeConfig · GetToolConfig"]
  end
  subgraph "Firefox (userChrome bridge, 127.0.0.1:9999)"
    NAV["navigate · close_tab"]
    EV["web_eval (JS program)"]
    UP["upload · trusted_click · type · get_interactive_map · screenshot"]
  end
  subgraph "YouTube (same-origin, container 20)"
    IN["youtubei/v1/next · browse · comments.* · video-manager.*"]
    STU["studio.youtube.com wizard"]
  end
  T --> Y --> BR --> NAV & EV & UP
  EV -->|"in-page fetch, cookies attached"| IN
  UP --> STU
  T --> TR
  Y --> CFG
  Y --> J[(".zenmcp/youtube/&lt;container&gt;.json<br/>upload journal + dedupe ledger")]
```

Boundaries and what crosses them:

* **zen-mcp never talks to YouTube directly.** Every byte goes through `bridge.CallBridge`
  (`internal/bridge/bridge.go:39`), so the gate/queue semantics of
  `internal/bridge/address.go` apply for free.
* **`internal/youtube` never touches `mcp.CallToolRequest`** — it is a plain domain package
  with typed inputs/outputs; only `internal/tools/youtube.go` speaks MCP. This is the
  `colab.go` pattern (`internal/tools/colab.go:16` thin face over `bridge.CallBridge`),
  not the `codegraph.go` pattern (1320-line in-package engine).
* **The only long-lived state is on disk** (the journal). Nothing is cached in process
  memory across calls: no session tab, no channel identity cache. Per-call background tabs,
  as decided.
* **Two origins, therefore two tabs per upload**: `www.youtube.com` (innertube bootstrap +
  verification) and `studio.youtube.com` (wizard). Cross-origin `fetch` between them is not
  possible, so a `Session` is always origin-scoped.

---

## Data flow and state

### Read/write of structured data (one call, one tab, closed on the way out)

```mermaid
sequenceDiagram
  participant A as Agent
  participant H as HandleYoutubeAction
  participant S as youtube.Session
  participant B as bridge
  participant P as JS program (in-page)
  A->>H: youtube -a comments -v <videoID> --max 40
  H->>S: Open(ctx, container, OriginWatch)
  S->>B: navigate {url: BootstrapURL, back_tab, container}
  B-->>S: id = t_xxx
  S->>B: WithTabTurn(t_xxx)   %% per-tab queue, not the global Gate
  S->>B: web_eval {index:t_xxx, code: programProbe}
  B-->>S: {ok, apiKey, clientVersion, channel, login}
  H->>S: Comments(videoID, opts)
  S->>B: web_eval {index:t_xxx, code: programComments}
  P->>P: fetch(watch) → continuation token
  P->>P: POST youtubei/v1/next → decode rows (+ loop until max/pages)
  P-->>B: {ok:true,data:{comments:[…], nextToken}}   %% only decoded rows cross
  B-->>S: {success, data:{result,logs}, id:t_xxx}
  S->>S: verifyTab provenance echo
  S->>B: close_tab {index:t_xxx}
  H-->>A: WrapSuccess{comments,nextToken}
```

Failure handling in this flow is uniform: an error carries a `Code` (see *Failure modes*),
the tab is closed by the `release` returned from `Open`, and `data:null` from an
unrecognised action is a hard error, never an empty success.

### Upload: a journal-joined state machine, never one long call

The upload is deliberately **three tool calls** so no MCP call runs for minutes and the
pooling subsystem is untouched (`internal/pooling/*` is not modified — hard constraint).

```mermaid
stateDiagram-v2
  [*] --> Planned: youtube -a upload --dry_run=true
  Planned --> Attached: youtube -a upload (attach + fill details + set visibility)
  Planned --> [*]: refused (quota policy / file invalid / not logged in)
  Attached --> Attached: youtube -a upload_status  (read-only probe, repeatable)
  Attached --> Processing: script reports transfer 100%
  Processing --> Ready: script reports processing complete
  Ready --> Published: youtube -a upload_publish
  Published --> [*]: journal row closed with videoID
  Attached --> Failed: tab lost / wizard error
  Processing --> Failed
  Failed --> [*]: hard stop, screenshot + interactive map in payload
```

Journal state (`<ProjectRoot>/.zenmcp/youtube/<container>.json`) is the only thing that
links the calls — the wizard tab survives because TabRegistry ids are stable and every
step addresses the tab by id, so a second agent, a restart, or a user clicking elsewhere
cannot silently redirect a step to the wrong page.

```mermaid
graph LR
  U1["upload<br/>writes JournalEntry{stage:attached, tabID, file, sha256}"] --> J[(journal json)]
  U2["upload_status<br/>reads entry · probes progress · writes stage"] --> J
  U3["upload_publish<br/>reads entry · publishes · writes videoID"] --> J
  U4["videos --filter=recent<br/>diff vs newestID → videoID"] --> J
```

Progress reporting is therefore **pull, in-band, from the page's own DOM**:
`upload_status` runs `scripts/upload-progress.js`, which reads Studio's progress bar
(`aria-valuenow`) plus the wizard step header, and returns `{stage, percent, detail}`.
The skill (`resources/skills/youtube-automation.md`) teaches the poll pattern — call
`upload_status`, `sleep`, repeat — instead of pushing progress into the pool.

---

## Implementation Blueprint

Signatures are contracts: bodies are `// TODO` and the algorithm notes describe intent.

| # | File path | Action | Concrete signature / schema (pseudocode) | Depends on | Acceptance criteria ("done when…") |
|---|---|---|---|---|---|
| 1 | `internal/youtube/errors.go` | create | `type Error struct{ Code, Message, Hint string; Detail string }` with `func (e *Error) Error() string`; `func New(code, msg string) *Error`, `func Of(err error) (*Error, bool)`, `func IsCode(code string, err error) bool`; `const ( ErrNotLoggedIn="not_logged_in"; ErrContainerMismatch="container_mismatch"; ErrSchemaDrift="schema_drift"; ErrInnertubeHTTP="innertube_http"; ErrInnertube="innertube_error"; ErrQuotaPolicy="upload_quota_policy"; ErrAttachFailed="upload_attach_failed"; ErrTabLost="upload_tab_lost"; ErrFileInvalid="file_invalid"; ErrNeedsConfirm="confirm_required"; ErrDuplicate="duplicate_post"; ErrBridgeDown="bridge_unavailable"; ErrTimeout="timeout" )` | — | `go test ./internal/youtube` compiles; `Of` recovers a `*Error` wrapped by `fmt.Errorf("%w")` |
| 2 | `internal/youtube/scripts/` (`probe-identity.js`, `comments-read.js`, `replies-read.js`, `comment-post.js`, `videos-list.js`, `video-get.js`, `video-edit.js`, `video-delete.js`, `channel-get.js`, `upload-progress.js`) | create | One file per innertube/DOM program. **Program contract** (identical for all): the file is the *body of an async function*; it may use top-level `await`, `fetch`, `document`, `crypto`, `JSON`; the single argument `ARGS` is a JSON value already injected; it **must** `return {ok:true,data:<compact rows>}` or `{ok:false,error:"<Error.Code>",detail?:string}`. Never `return` a DOM node or a >200 KB object. | 1 | `go:embed` of the dir succeeds; each file parses under `node --check` (wrapped as `(async()=>{…})()`) |
| 3 | `internal/youtube/session.go` | create | `type Origin int; const ( OriginWatch Origin = iota; OriginStudio )`<br>`const BootstrapURL = "https://www.youtube.com/robots.txt"` *(same-origin, ~0.3 KB, carries container cookies; fall back to `https://www.youtube.com/` if its CSP blocks `connect-src` to `/youtubei/`)*<br>`type Session struct{ TabID, Container string; APIKey, ClientName, ClientVersion string; ExternalChannelID, PageOwnerID string; LoginOK bool; origin Origin }`<br>`func Open(ctx context.Context, container string, origin Origin) (_ *Session, release func(), err error)`<br>`func (s *Session) Run(ctx context.Context, script string, args any) (json.RawMessage, error)`<br>`func script(name string) (string, error)` //go:embed scripts → `scriptsFS` + `map[string]string` cache | 1, 2 | With `installBridgeStub` canned `navigate`+`web_eval` responses, `Open` issues exactly `navigate{back_tab:true,container}` then `web_eval{index:<id>}`, and `release` issues `close_tab{index:<id>}` (`bridge_stub_test.go:151`); `Open` with a stub that omits the tab id returns `ErrBridgeDown` naming the registry (`requireTabID`, `internal/tools/tabflow.go:41`) |
| 4 | `internal/youtube/journal.go` | create | `type Entry struct{ ID, Kind, Stage, TabID, VideoID, File, SHA256, Title, Visibility, PublishAt string; AttachedAt, FinishedAt time.Time; Error string }` (`Kind`: `attach` \| `post`)<br>`type Journal struct{ path string; mu sync.Mutex }`<br>`func Open(container string) (*Journal, error)` — path `filepath.Join(mcpcfg.ProjectRoot, ".zenmcp", "youtube", container+".json")`, 0600, load-on-open<br>`func (j *Journal) Append(e Entry) error` · `func (j *Journal) Active() (Entry, bool)` (stage not in {published,failed}) · `func (j *Journal) Update(id string, fn func(*Entry)) error` · `func (j *Journal) UploadsOn(day time.Time) int` · `func (j *Journal) HasPost(dedupeKey string) bool` | 1 | Round-trips in a temp dir; two processes writing interleaved `Append`s leave valid JSON; `UploadsOn` counts only `Kind=="attach"` |
| 5 | `internal/youtube/innertube.go` | create | `type Comment struct{ ID, ParentID, AuthorName, AuthorChannelID, Text string; Likes int; PublishedText, PublishedAt string; ReplyCount int; IsOwner, HasReply bool; Continuation string }`<br>`type CommentPage struct{ Comments []Comment; NextToken string; CountText string }`<br>`type Video struct{ ID, Title, Description, URL, Visibility, PublishedText, DurationText, Thumbnail string; Views, Comments int }`<br>`type Channel struct{ ExternalID, Title, Handle string; LoggedInAs []string; VideoCountText string }`<br>`type CommentOpts struct{ Max int; Sort string; PageToken string }`<br>`type VideoPatch struct{ Title, Description, Thumbnail, Visibility, PublishAt *string; Tags []string }`<br>`func (s *Session) Channel(ctx) (*Channel, error)` · `Videos(ctx, filter string, max int) ([]Video, error)` · `Video(ctx, videoID string) (*Video, error)` · `Comments(ctx, videoID string, o CommentOpts) (*CommentPage, error)` · `Replies(ctx, videoID, commentID string, o CommentOpts) (*CommentPage, error)` · `Post(ctx, videoID, parentID, text string) (*Comment, error)` · `Edit(ctx, videoID string, p VideoPatch) (*Video, error)` · `Delete(ctx, videoID string) error` | 1, 2, 3 | Given the real `next` response fixture captured this session, `Comments` returns 20 decoded comments with non-empty `ID`s (`Ug…`/`ccd…`) and a non-empty `NextToken`; given a mutated fixture with the paths removed it returns `ErrSchemaDrift` naming `onResponseReceivedEndpoints[].reloadContinuationItemsCommand` — **never** an empty success |
| 6 | `internal/youtube/wizard.go` | create | `type UploadRequest struct{ File, Title, Description, Category, Thumbnail string; Tags []string; Visibility, PublishAt string; DryRun bool }`<br>`type UploadResult struct{ UploadID, TabID, Stage, VideoID string; Progress Progress }`<br>`type Progress struct{ Stage string; Percent int; Detail string; Ready bool }`<br>`func Attach(ctx, container string, req UploadRequest, j *Journal, newestIDBefore string) (*UploadResult, error)` — steps: assert quota → open Studio session → `bridge upload {index, selector:"input[type=file]", path, triggerDrop:true}` → fill details step → set visibility/schedule → **return without publishing**<br>`func Status(ctx, container, tabID string) (*Progress, error)`<br>`func Publish(ctx, container, tabID, uploadID string, j *Journal, newestIDBefore string) (videoID string, err error)`<br>`var wizard = map[string]Locator{ "title":{ByLabel:"Title"}, "description":{ByLabel:"Description"}, "next":{ByRole:"button",Name:"Next"}, "publish":{ByRole:"button",Name:"Publish"}, "schedule":{ByRole:"radio",Name:"Schedule"}, "category":{ByLabel:"Category"} }`<br>`type Locator struct{ Selector, ByRole, ByText, ByLabel, Name string }` | 1, 3, 4 | Unit: with a stub, `Attach` refuses before any bridge call when the journal already has `maxUploadsPerDay` attaches today (`ErrQuotaPolicy`), and issues **no** `upload` action when `DryRun`. Live: one scheduled upload produces a `videoID` that `videos --filter=scheduled` then lists |
| 7 | `internal/mcpcfg/config.go` | modify | Add `type YoutubeConfig struct{ Container string \`json:"container"\`; MaxUploadsPerDay int \`json:"maxUploadsPerDay"\`; DefaultVisibility string \`json:"defaultVisibility"\`; Timezone string \`json:"timezone"\`; WizardTimeoutMs int \`json:"wizardTimeoutMs"\`; ProcessTimeoutMs int \`json:"processTimeoutMs"\`; BootstrapURL string \`json:"bootstrapUrl,omitempty"\` }`; field `Youtube YoutubeConfig \`json:"youtube"\`` in `ZenConfig` (after `Bridge`, `internal/mcpcfg/config.go:119`); defaults in `defaultConfig()` (`:178`): container `"pachoigame"`, max 1, visibility `"private"`, tz `"UTC"`, wizard 900000, process 1800000 | — | `Load()` on the current `config.json` yields `Get().Youtube.Container=="pachoigame"`; an absent `youtube` block still yields the defaults |
| 8 | `internal/tools/youtube.go` | create | `func defYoutube(workspace string, deps Deps) ToolDef` — `Name:"youtube"`, schema `action` enum `["channel","videos","video","comments","replies","comment","reply","upload","upload_status","upload_publish","edit","delete"]`, params `video_id, comment_id, text, file, title, description, tags[], category, visibility(private|unlisted|public|schedule), publish_at, max, sort(top|newest), page_token, filter(published|scheduled|recent), dry_run, force, confirm, keep_tab, container`<br>`func HandleYoutubeAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`<br>`type actionSpec struct{ required []string; needsSession bool; origin youtube.Origin; writes bool }`<br>`var actionSpecs map[string]actionSpec`<br>`func resolveContainer(args map[string]any) string` — arg else `mcpcfg.Get().Youtube.Container`<br>`func youtubeErrResult(ctx, start, err error) *mcp.CallToolResult` — `WrapErrorWithContext` with `Error.Code`/`Hint` in the message | 3–6 | `AllDefs` exposes `youtube`; a call with an unknown action returns a validation error listing the enum; each action's missing-required case returns `"<param> is required for <action>"` (the `colab.go:57` style) |
| 9 | `internal/tools/types.go` | modify | insert `defYoutube(workspace, deps),` after `defBrowser(...)` (`internal/tools/types.go:46`) so the CLI wrapper ordering follows the browser tool | 8 | `AllDefs` length +1; `zexport_cli` emits `cli/zyoutube` |
| 10 | `config.json` | modify | `"youtube": { "container": "pachoigame", "maxUploadsPerDay": 1, "defaultVisibility": "private", "timezone": "UTC", "wizardTimeoutMs": 900000, "processTimeoutMs": 1800000 }`; `"toolConfigs": { "youtube": { "timeout": 900000, "format": "raw" } }`; `"bypassTools": += "youtube"` | 7, 8 | Server restarts (auto via `run.sh`) and `zyoutube -a channel` answers |
| 11 | `internal/youtube/session_test.go`, `innertube_test.go`, `journal_test.go`, `wizard_test.go`, `internal/tools/youtube_test.go` | create | Use `installBridgeStub(t, map[string]string{ "navigate":…, "web_eval":… })` from `internal/tools/bridge_stub_test.go:151` (same package for the tools test; the `internal/youtube` tests need a local copy of the stub — extract nothing, keep it test-local). Fixtures: `testdata/next-comments.json` (the live `next` response), `testdata/watch.html` (comment-token region only, <300 KB). | 3–6, 8 | `go test ./internal/youtube ./internal/tools` green; `youtube_test.go` asserts the exact bridge call sequence per action and that `close_tab` always happens |
| 12 | `resources/skills/youtube-automation.md` | create | frontmatter `name: youtube-automation`, `description`, `trigger: youtube channel`; body: the action table, the **short-call pattern** (`upload` → loop `upload_status` → `upload_publish`), 1-video/day policy, dry-run-first, hard-stop-on-attach-failure, journal location, scheduled-backlog workflow (`videos --filter=scheduled`), and the container/identity rule | 8–11, after live verification | `zskill -a get -i youtube-automation` returns it; a fresh session can upload+publish using only the skill text |
| 13 | `docs/investigation-notes-youtube-innertube.md` *(append to `docs/`)* | create | Raw evidence: the verified JSON paths, the continuation-token anchor rule (token precedes `"sectionIdentifier":"comment-item-section"`), the `mutations[].payload` correction, the `web_eval` sandbox limitations, the `data:null` unknown-action trap | this plan | The exact JSON paths that `comments-read.js` depends on are written down with the fixture that proves them |

### JS program inventory (contract-level, no bodies in this plan)

| Script | In-page work | Returns |
|---|---|---|
| `probe-identity.js` | `fetch(BootstrapURL or "/")` → regex `INNERTUBE_API_KEY`, `INNERTUBE_CLIENT_VERSION`; read `document.cookie` names; detect the avatar/`data-external-id` identity | `{ok,apiKey,clientVersion,clientName,externalChannelID,pageOwnerID,login}` |
| `comments-read.js` | fetch watch HTML → last `"token"` before `comment-item-section` → `POST /youtubei/v1/next` → decode `commentThreadRenderer` + `mutations[].payload`; loop `continuation` until `ARGS.max` | `{ok,data:{comments:[…],nextToken,countText}}` |
| `replies-read.js` | `commentThreadRenderer.replies…continuationCommand.token` → `POST /youtubei/v1/next` | `{ok,data:{comments:[…]}}` |
| `comment-post.js` | SHA-1 `SAPISIDHASH` via `crypto.subtle` from `document.cookie` + `origin` + `/youtubei/v1/comments.post_comment`; resolve the form/engagement ids from the thread's `continuationItemRenderer`; `POST` | `{ok,data:{comment}}` — **must** echo the created comment id |
| `videos-list.js` / `channel-get.js` / `video-get.js` | `POST /youtubei/v1/browse` on the channel `videos` tab (`filter: scheduled` → the Studio `videos?filter=account_video_scheduled` browse id) | `{ok,data:{videos:[…],nextToken}}` |
| `video-edit.js` / `video-delete.js` | `POST /youtubei/v1/video-manager/edit-video` / `…delete-video` with `SAPISIDHASH` + `X-Goog-PageId` | `{ok,data:{video}}` / `{ok}` |
| `upload-progress.js` | Studio DOM: wizard step header, `aria-valuenow` of the progress bar, `ytcp-status-text`, presence of the Publish button | `{ok,data:{stage,percent,detail,ready}}` |

Auth-header rule (single place, shared by all write programs):
`Authorization: "SAPISIDHASH " + hex(SHA1(origin + " " + SAPISID + " " + path))`, with
`X-Goog-Authuser: <sessionId or 0>` and `X-Origin: https://www.youtube.com`. If `SAPISID`
is absent from `document.cookie`, the program returns `ErrNotLoggedIn` before any POST.

---

## Ordered, atomic build steps

Each step is self-contained: files to touch, what to write, how to know it is done.

1. **Error taxonomy** — create `internal/youtube/errors.go` (row 1). *Accept:* package builds.
2. **JS harness + identity probe** — create `internal/youtube/scripts/` + `session.go`
   (rows 2, 3) with `probe-identity.js`. Run it live through the bridge by hand
   (`zbrowser -a web_eval`) before wiring Go. *Depends:* 1. *Accept:* live `web_eval` of
   `probe-identity.js` in container `pachoigame` returns a non-empty `apiKey`,
   `clientVersion`, and `externalChannelID`; `session_test.go` passes against a stub.
3. **Config** — row 7 + row 10. *Depends:* none (parallel with 1–2). *Accept:* server
   auto-restart logs no config error; `Get().Youtube` populated.
4. **Reads** — `innertube.go` + `comments-read.js`, `replies-read.js`, `videos-list.js`,
   `video-get.js`, `channel-get.js` (rows 5, 2) and the tool face for `channel`, `videos`,
   `video`, `comments`, `replies` (rows 8, 9). *Depends:* 2, 3. *Accept:* unit tests green
   against the captured `next` fixture; live `zyoutube -a comments -v dQw4w9WgXcQ --max 40`
   returns ≥ 20 decoded comments and a `nextToken`; live `zyoutube -a channel` names the
   right channel; **zero uploads consumed**.
5. **Journal** — row 4. *Depends:* 1. *Accept:* unit test proves `UploadsOn` and `Active`.
6. **Upload pipeline** — `wizard.go` + `upload-progress.js` (row 6) and the `upload`,
   `upload_status`, `upload_publish` actions (row 8). *Depends:* 2, 4, 5. *Accept:* unit
   tests prove (a) `dry_run` issues no `upload` action, (b) quota policy blocks a second
   attach on the same day before any bridge call. Then **exactly one live upload**:
   `--dry_run=true` first, then `--visibility=schedule --publish_at=+7d`.
7. **Writes** — `comment-post.js` + `comment`, `reply` (row 8), with journal dedupe.
   *Depends:* 4, 5. *Accept:* posting the same text to the same `comment_id` twice returns
   `ErrDuplicate` on the second call; the first returns a comment id that
   `comments --max 5` then shows.
8. **Metadata + destructive** — `video-edit.js`, `video-delete.js`, actions `edit`,
   `delete` (`--confirm=true` mandatory). *Depends:* 4. *Accept:* `edit` changes the title
   of the scheduled test video and `video` reflects it; `delete` without `--confirm`
   returns `ErrNeedsConfirm` and touches nothing.
9. **CLI wrappers + docs** — `zexport_cli` (regenerate `cli/zyoutube`), row 13.
   *Depends:* 8. *Accept:* `zyoutube --help` lists all 12 actions; `zyoutube -a comments
   -v <id> --max 5` works from a shell.
10. **Skill** — row 12. *Depends:* 9 (patterns must be proven, not theorised). *Accept:*
    skill text reproduces the working sequence verbatim.

---

## Failure modes

| Failure | Where it bites | Guard |
|---|---|---|
| Container is logged out, or the wrong container is used | every action | `session.Open` → `probe-identity.js` asserts `login==true` and `externalChannelID` matches `Youtube.Container` expectation; otherwise `ErrNotLoggedIn`/`ErrContainerMismatch` with the `list_containers` hint. Never attempted silently as userContextId 0. |
| `web_eval` sandbox cannot `fetch` (seen once on a stale/discarded tab) | reads/writes | `Run` distinguishes `TypeError: NetworkError` and returns `ErrBridgeDown` with "reload the tab / re-run"; the tab is always closed on release so the next call starts clean. No retry loops that could double-POST. |
| Unknown/typo'd bridge action answers `success:true, data:null` | all | `session.Run` treats `data == null` as an error unconditionally (verified trap; the same rule is why `browser.go` never relies on it) |
| innertube response shape drifts | `comments-read.js` and siblings | every program has an explicit shape assertion and returns `{ok:false,error:"schema_drift",detail:<600-char response prefix>}`; Go turns it into `ErrSchemaDrift`. **No DOM fallback exists by decision** — drift is loud, fixable from the payload, and costs no upload. |
| 1 MB bridge reply cap truncates a response | any read | parsing happens in-page and only decoded rows cross; `Comments` caps `Max` at 100/page and the Go side rejects a `Run` result over 900 KB with `ErrSchemaDrift` as a backstop |
| Attach fails halfway (quota exhausted server-side, tab closed, wizard change) | `Attach` | `wizard.Attach` writes the journal row **before** issuing `upload`, then again on failure with `stage:"failed"`, screenshot path (`bridge screenshot`) and `get_interactive_map` dump in the error payload. Nothing retries automatically: one failed attach ends the run (policy, decided). |
| Double-posting a public reply | `Post` | `Journal.HasPost(sha256(videoID|parentID|text))` refuses with `ErrDuplicate` unless `--force=true` |
| Accidental public video / spam cadence | `Attach` | `Visibility` defaults from config (`private`); > `MaxUploadsPerDay` (1) attaches/day → `ErrQuotaPolicy` before any bridge call; `--force=true` is the only override |
| Publish button disabled / processing unfinished | `Publish` | `upload_publish` first runs `upload_status`; `ready==false` → returns the current `{stage,percent,detail}` and does nothing else (idempotent, safe to re-poll) |
| Bridge queue starvation while a wizard tab sits open | all bridge users | every step is tab-addressed, so it queues on its own tab (`address.go:CallBridge`), not the global Gate; only focus-bound actions (`screenshot`, `trusted_click`) take the global turn, and `wizard.Publish` uses the DOM-level `click` by default, `trusted_click` only on retry |
| Tab id dies (user closed it, server restart) | `Status`/`Publish` | addressed call returns bridge `tab_not_found`; mapped to `ErrTabLost` with recovery text: check `videos --filter=recent`, then re-run `upload` (which is quota-gated) |
| Scheduled publish field name differs from expectation | `Attach`/`Edit` | if the schedule field cannot be located, `Attach` returns `ErrSchemaDrift` **before** publishing, leaving the video as `private` rather than silently publishing it; the plan then moves scheduling to the post-publish `edit` path (only open item, see below) |
| Sandbox world cannot see `ytcfg`/`ytInitialData` | all programs | by design every program re-reads what it needs from `fetch`ed HTML; nothing depends on page globals (verified in this session) |
| `delete` used by mistake | `Delete` | mandatory `--confirm=true`; the confirmation error names the video id and title so the retry is deliberate |

---

## Key decisions and rejected alternatives

1. **Transport: in-page `youtubei/v1` over `web_eval`, single route, no DOM scraping fallback.**
   Rejected (a) `yt-dlp` for comments — the user's call: the bridge rides the real logged-in
   browser session and therefore does not trip the bot checks `yt-dlp` increasingly hits; (b)
   DOM scroll-scraping as primary — no stable `commentId` without clicking each thread,
   N scrolls × settle delays, unbounded payload; (c) DOM as *fallback* — a second route is
   untestable in the one place it would run and would mask drift; (d) the bridge `request`
   action — wrong cookie container by default and the 1 MB reply cap.
2. **Upload = Studio wizard DOM driven by bridge `upload`.** Rejected: replicating the media
   POST over innertube/`request` — the file bytes live on disk, so an in-page fetch can't
   read them, and a chrome-side binary XHR fights the response cap and container cookies;
   the bridge `upload` action already handles shadow-DOM inputs and background tabs.
3. **Short calls + on-disk journal instead of pooling `youtube`.** The user's constraint:
   `internal/pooling` exists to stop the harness killing long jobs and must not be
   destabilised; adding a `Progress` field to `Job`/`PollOutcome`/`statusPayload` would touch
   the shared hot path for one tool. Rejected: (a) `Wrap` progress plumbing — the pool id is
   minted *after* the handler starts (`internal/pooling/wrap.go:86`), so the handler cannot
   report against it without a new ctx sink; (b) one blocking `upload` with a 15-minute tool
   timeout. The poll pattern is moved into the skill instead.
4. **Per-call background tabs.** Rejected: a cached long-lived session tab (discardable by
   `AutoTabDiscard.uc.js`, stale logins, cross-call aliasing). The user explicitly waived
   the discard race — per-call tabs stay, it is simply not a concern.
5. **Policy: 1 upload/day, `dry_run` first, hard stop on attach failure, `schedule` as the
   normal mode, `private` as the default visibility.** Rejected: quota-sized limits
   (e.g. 3/day) — cadence here is a channel-health rule, not a YouTube-limit workaround.
6. **Domain package `internal/youtube/` with a thin `internal/tools/youtube.go`.** Rejected:
   one large tool file (`codegraph.go` precedent) — 900 lines mixing JS, DOM wizard, JSON
   decoding and MCP arg parsing in one file is not maintainable (user's stated preference).
7. **JS programs as `//go:embed`-ed files under `internal/youtube/scripts/`.** Rejected:
   JS as Go raw-string constants (edits recompile, diff noise, no `node --check`) and
   rejected runtime-loaded files outside the binary (missing-file failure at call time).
8. **One upload for live testing.** Rejected: one upload per visibility mode — `edit`
   mutates visibility on an existing video, so 1 upload exercises the whole matrix.

---

## Red-team critique summary

*(Step 4–5 of the workflow: to be run via the bridge `chat` action with `provider: claude`
on this file plus `internal/tools/types.go`, `internal/bridge/address.go`,
`internal/tools/tabflow.go`, `internal/pooling/wrap.go`; points triaged as
"folded in" / "resolved via follow-up interview" / "rejected: …".)*

---

## Open questions

1. **Scheduled-publish field.** The wizard's Schedule radio + date/time pickers are DOM
   (locatable by `byRole: "radio", name: "Schedule"`), but the innertube field for the
   publish time on the *edit* path (`video-manager/edit-video`) is not verified, and this
   session deliberately did not spend an upload to verify it. Build step 6 resolves it
   against the single scheduled test upload. If the edit path lacks the field, the rule is
   already fixed here: scheduling is **upload-time only**, and `edit --publish_at` returns
   `ErrSchemaDrift` telling the agent to use the wizard.
2. **`BootstrapURL = robots.txt` CSP.** Same-origin `fetch` to `/youtubei/` from a
   text/plain document is expected to be permitted (no CSP header on that response), but it
   is unverified. Step 2 verifies it and, if it fails, sets the single bootstrap to
   `https://www.youtube.com/` — one route either way, chosen at build time, not per call.
