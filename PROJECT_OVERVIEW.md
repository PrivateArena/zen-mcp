<!-- codegraph-file-count: 141 -->
# zen-mcp — MCP Server for Zen IDE (Go 1.24)

## Purpose

zen-mcp is a Go-native, high-throughput MCP (Model Context Protocol) server that serves a Zen Browser/IDE with ~24 domain tools plus a terminal REPL and a CLI wrapper generator. It replaces the original TypeScript implementation: all tool logic (shell/run sandboxing, Firefox bridge automation, codegraph indexing/query, project memory, sequential thinking, async job pooling, skills/prompts, whiteboard-backed shared memory) is implemented directly in Go with zero runtime dependency on the TS codebase. The server runs in **stateless mode** (no MCP session ID negotiation) over a Streamable HTTP transport backed by `mark3labs/mcp-go`, with per-workspace `mcpserver` instances cached and LRU-reaped.

## Architecture

```
HTTP JSON-RPC (12 stateless routes, X-Workspace / body workspace detect)
  → server.routes.postMCP → serverCache.getOrCreate(workspace) → mcpserver
    → tools.AllDefs[defX] → HandleX → toolresponse.Wrap{Success,Error}
  Core subsystems: codegraph (tree-sitter→SQLite+FTS5) · projectmemory (.zenmcp brain)
  · pooling (async job registry) · gatekeeper (path/command safety) · shell/exec+tokenoptimizer
  Sidecars: terminal REPL (commander.ExecuteTool) · Firefox bridge · whiteboard client
```

## File Tree

```
zen-mcp/
  main.go                          # entry point: config load, 2 HTTP servers, terminal commander
  internal/
    server/                        # stateless MCP HTTP layer, server cache, pooling wiring, tools/list rewriter
    tools/                         # tool definitions (defX) + handlers (HandleX), Deps/ToolDef, AllDefs
    toolregistry/ toolresponse/    # registry, result wrapping, telemetry hooks, orphan detection
    toolstate/ toolsuggestions/    # enable-state layering, action validation & mistake correction
    pooling/                       # async job registry (registry.go) + handler wrapper (wrap.go)
    gatekeeper/                    # path/command safety: ValidatePathSafety, interactive confirmations
    shell/exec processes tokenoptimizer   # process execution, abort-all, output compaction
    codegraph/                     # tree-sitter indexer, SQLite+FTS5 storage, 9 language plugins
    projectmemory/                 # brain timeline (v1→v3), FTS index, git signals, virtual context
    prompts/ skills/               # prompt templates (YAML+frontmatter), CLI-mode transform, skill detection
    terminal/  terminal/handlers/  # raw-mode REPL (commander.go) + 14 CLI command handlers
    bridge/ agentbridge/           # Firefox bridge POST client + web-agent delegation
    analysis/ whiteboard/          # output file-type detection, whiteboard REST client
    mcpcfg/ shared/ workspace/     # config (merge+watch), KV store, path resolution
    telemetry/ logfilter/          # SQLite tool-call telemetry, leveled logging filter
```

## Component Roles

Single-language Go project. Grouped by layer; LOC approximate.

### Transport / server core

| File / Module | Role | LOC | Key Exports (with signatures) |
|---|---|---|---|
| internal/server/routes.go | Registers the 12 stateless HTTP routes; dispatches tools/call to cached mcpserver instances | ~460 | `SetupRoutes(mux *http.ServeMux, deps RouteDeps)`; `(d RouteDeps) postMCP(w, r)`; `autoDetectWorkspace(r *http.Request, st *shared.Store) string`; `serverCache.getOrCreate(logicalID, factory, registry) *mcpserver.MCPServer` |
| internal/server/tools.go | Wires all registered tools, pool wrapping, tool catalog resource, disabled filtering | ~100 | `RegisterAllTools(ctx, srv *mcpserver.MCPServer, reg *toolregistry.ToolRegistry, deps tools.Deps, workspace string) error`; `wrapIfPooled(name string, handler toolregistry.Handler) toolregistry.Handler`; `FilterEnabled(reg) (ctx, tools) []mcp.Tool` |
| internal/server/pool.go | Global server-cache pool with idle reaping | ~66 | `StartIdleReaper() func()`; `snapshotServerCaches() []*serverCache` |
| internal/server/patch.go | Per-tool timeouts, aborted-request cause detection | ~129 | `WrapHandlerWithTimeout(name string, inner toolregistry.Handler, getTimeout func(string) time.Duration) toolregistry.Handler`; `abortReason(ctx) string` |
| internal/server/toolslist.go | Rewrites tools/list JSON to inject annotations | ~112 | `toolsListRewriter(w http.ResponseWriter) *bufferingWriter`; `rewriteToolsListJSON(body []byte) ([]byte, bool)` |
| internal/server/catalog.go | Publishes tools:catalog resource | ~81 | `buildToolCatalog(reg *toolregistry.ToolRegistry) string` |
| internal/server/shutdown.go | SIGINT/SIGTERM handling | ~33 | `SetupShutdownHandlers(mode string, logf func(format string, args ...any)) chan struct{}` |
| internal/pooling/registry.go | Async job registry backing the pool tool (new in f1354bb) | ~293 | `NewRegistry(ttl, grace time.Duration, max int) *Registry`; `Register(name string, job *Job) (string, error)`; `LongPoll(ctx, id, wait) PollOutcome`; `Complete(id, res *mcp.CallToolResult) bool`; `Cancel(id) bool`; `Global() *Registry` |
| internal/pooling/wrap.go | Detaches long-running tool calls into a pooled job | ~134 | `Wrap(name string, reg *Registry, inner toolregistry.Handler) toolregistry.Handler` |
| internal/toolregistry/registry.go | Registry of tool name → registration (handler, schema, enabled) | ~91 | `Create() *ToolRegistry`; `Track(reg ToolRegistration)`; `GetTool(name) (*ToolRegistration, bool)`; `SetToolEnabled(name string, enabled bool) bool` |
| internal/toolresponse/response.go | Uniform CallToolResult wrapping, telemetry, timeouts, orphan suppression | ~460 | `WrapSuccess(ctx, tool string, data any, start time.Time) *mcp.CallToolResult`; `WrapError(tool string, err error, start time.Time) *mcp.CallToolResult`; `RenderOutput(format string, data any) string`; `SetTimeoutObserver(fn func(tool, kind string, elapsedMs int64))` |
| internal/toolstate/toolstate.go | Workspace + global enable-state layering | ~142 | `ResolveToolState(name, workspaceRoot string, workspaceCfg map[string]bool, reg *toolregistry.ToolRegistry) ToolStateLayers`; `ApplyToolStates(workspaceRoot string, reg) ApplyResult` |
| internal/toolsuggestions/suggestions.go | Validates tool calls, formats corrective suggestions | ~405 | `ValidateToolCall(toolName, action string, suppliedParams map[string]any, schema any) ValidationResult`; `FormatSuggestion(toolName, errorMessage, action string, schema any) string`; `FindMistakeCorrection(toolName, errorMessage, action string, schema any) *MistakeCorrection` |
| internal/shared/state.go | Concurrent string KV store, per-key change callbacks | ~79 | `NewStore() *Store`; `Set(key, value string)`; `Get(key) (string, bool)`; `OnChange(key string, fn func(string)) func()` |
| main.go | Entry: config init, newMcpServer, runHTTPServers (two ports) | ~245 | `newMcpServer(id string, reg *toolregistry.ToolRegistry, deps tools.Deps) *mcpserver.MCPServer`; `runHTTPServers(startTime, cfg, store, shutdownCh) error` |

### MCP tools (definition + dispatch)

| File / Module | Role | LOC | Key Exports (with signatures) |
|---|---|---|---|
| internal/tools/types.go | Shared Deps/ToolDef types; the single registration axis for all tools | ~74 | `AllDefs(workspace string, deps Deps) []ToolDef`; `jsonSchema(properties map[string]any, required []string) map[string]any` |
| internal/tools/codegraph.go | Largest tool: 19 codegraph actions over layered root/sub-graph DBs | ~1320 | `HandleCodegraphAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `discoverGraphRoots(workspaceRoot string) []string`; `expandQueryPaths(query string, session *layeredGraphSession) []string`; `actionIndex/actionMap/actionSkeletons/actionRelated/actionImpact(...)` |
| internal/tools/think.go | Sequential thinking + plan/task manager (plan.md persisted) | ~549 | `HandleThinkAction(ctx, workspace string, req mcp.CallToolRequest) *mcp.CallToolResult`; `(s *sequentialThinkingServer) processThought(input thoughtData) map[string]any`; `(p *planManager) createPlan(projectName, objective string, taskTitles []string) string` |
| internal/tools/browser.go | Firefox bridge browser control (navigate/read/chat/request/eval) | ~538 | `HandleBrowserAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `wrapBridgeOutput(ctx, action string, bridgeParams map[string]any, deps Deps, workspace string, start time.Time) any` |
| internal/tools/shell.go | Shell execution with gatekeeping + token-profile optimization | ~180 | `HandleShellAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `tokenOptConfig(cfg mcpcfg.ZenConfig) tokenoptimizer.Config`; `toBlacklist(entries) []tokenoptimizer.BlacklistEntry` |
| internal/tools/run.go | Sandboxed run tool with sandbox language config | ~199 | `HandleRunAction(ctx, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `randHex() string` |
| internal/tools/capture.go | Screen/UI capture + collaboration registry (resolve-once) | ~299 | `HandleCaptureAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `HandleCollaborateCapture(ctx, apiAddr, targetPath string, start time.Time, deps Deps)`; `NewCollaborationRegistry() *CollaborationRegistry` |
| internal/tools/uivision.go | Launches a GUI binary for capturable UI, process-group isolation | ~115 | `HandleUiVisionAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `setPgidSysProcAttr() *unix.SysProcAttr` |
| internal/tools/memory.go | Save/load project memory state, recent commands, dependencies | ~268 | `HandleMemoryAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `actionSave(dataDir, memoryName, dbPath, workspace, sessionTitle, objective, sessionNotes string) map[string]any`; `actionScope(workspace, scope string) map[string]any` |
| internal/tools/context.go | Retrieves indexed project context | ~105 | `HandleContextAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `actionRetrieveContext(dbPath, query string) map[string]any` |
| internal/tools/memoryisolate.go | Whiteboard-backed isolated memory cards (load/save/scope) | ~215 | `HandleIsolateLoad/HandleIsolateSave/HandleIsolateScope(ctx, client *whiteboard.Client, ws string, args map[string]any, start time.Time) *mcp.CallToolResult` |
| internal/tools/memoryshared.go | Shared/multi-project memory via whiteboard REST | ~263 | `HandleSharedLoad(ctx, client *whiteboard.Client, ws string, args map[string]any, start time.Time) *mcp.CallToolResult`; `loadRelatedProjects() map[string][]string` |
| internal/tools/pool.go | Pool tool: poll/cancel/status over pooling.Registry. Intro-Commit f1354bbfea2fc72fbb2068b047639d5e603478a4 | ~121 | `HandlePoolAction(ctx, reg *pooling.Registry, req mcp.CallToolRequest) *mcp.CallToolResult`; `statusPayload(state, id string, reg) map[string]any` |
| internal/tools/skills.go | Skill list/get with bundled resource resolution | ~98 | `HandleSkillsAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest)`; `handleSkillsList(ctx, start)`; `handleSkillsGet(ctx, id string, start)` |
| internal/tools/workspace.go | Set active workspace (resolved via shared store) | ~93 | `HandleWorkspaceAction(ctx, path, workspace string, deps Deps) *mcp.CallToolResult` |
| internal/tools/colab.go | Collaboration card exchange with gateway | ~90 | `HandleColabAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult` |

### codegraph engine

| File / Module | Role | LOC | Key Exports (with signatures) |
|---|---|---|---|
| internal/codegraph/engine.go | Top-level engine: index, search, map, skeleton, related, deadcode, impact, cycles | ~1132 | `NewCodeGraph(rootDir string) (*CodeGraph, error)`; `Index() (*IndexResult, error)`; `Search(query string, limit int) ([]NodeSearchResult, error)`; `GetSkeleton(relPath string) (string, error)`; `RelatedFiles(filePath string, limit int) ([]RelatedRecord, error)`; `FindDeadCode(query string, limit int) (*DeadcodeResult, error)` |
| internal/codegraph/storage.go | SQLite (modernc) persistence: schema, prepared statements, FTS5, relations | ~1349 | `NewStorage(dbPath string) (*Storage, error)`; `UpsertFile(fr FileRecord) (int64, error)`; `InsertNodes(nodes []NodeRecord) (int64, error)`; `GetRelatedForFile(filePath string, limit int) ([]RelatedRecord, error)`; `RunInTransaction(fn func(tx *sql.Tx) error) error` |
| internal/codegraph/scanner.go | Disk walk, incremental pro/profiling, ignore + exclusions, TS path aliases | ~380 | `NewScanner(storage *Storage, rootDir string) *Scanner`; `GetFilesToProcess() ([]FileRecord, error)`; `GetFileDetails(relPath) (content string, hash string, mtime int64, language string, isTest bool, err error)`; `ResolveAlias(specifier string) string` |
| internal/codegraph/parser.go | Singleton tree-sitter parser + shared query helpers | ~247 | `GetParser() *Parser`; `Parse(ext string, src []byte) ([]ParsedNode, []ParsedRelation, error)`; `ExtractQueryMatches(lang, node, src, queryStr, symbolType, nodes)` |
| internal/codegraph/types.go | Node/relation/result records | ~68 | `ParsedNode`, `ParsedRelation`, `RelatedRecord`, `DeadcodeResult`, `ShortestPathResult`, `CycleRecord` |
| internal/codegraph/languages_*.go (9 files) | Per-language tree-sitter grammar plugins (C, C++, Go, Java, Lua, Python, Ruby, Rust, TS) | ~300 ea | each `func (p *<lang>Plugin) Parse(filePath string, src []byte) ([]ParsedNode, []ParsedRelation, error)`; base surface in `languages_base.go`: `type LanguagePlugin interface{...}` |

### project memory & prompts

| File / Module | Role | LOC | Key Exports (with signatures) |
|---|---|---|---|
| internal/projectmemory/ftsindex.go | Memory FTS index, virtual context, recent commands | ~499 | `GetDatabase(dbPath string) *sql.DB`; `IndexActiveMemory(dbPath string, items []MemoryIndexItem)`; `SearchIndexedMemory(dbPath, query string, limit int) []SearchResult`; `IndexVirtualContext(dbPath string, data VirtualContextData) string`; `RecentCommands(dbPath string) []RecentCommand` |
| internal/projectmemory/timeline.go | Brain event schema v1→v3 migration, JSONL append/reconstruct | ~384 | `MigrateToV3(raw map[string]any) BrainEvent`; `AppendEvent(dataDir, memoryName string, event BrainEvent) error`; `ReconstructState(dataDir, memoryName string) ReconstructedState`; `LatestEvent(dataDir, memoryName string) (BrainEvent, bool)` |
| internal/projectmemory/registermap.go | Ordered JSON registry map of registered projects | ~150 | `RegisterProjectInMap(projectPath string, dependencies []string)` |
| internal/projectmemory/gitsignals.go | Git signal extraction since last visit | ~88 | `GetGitSignals(workspaceRoot, lastVisitedIso string) GitSignals` |
| internal/projectmemory/markdown.go | Key-ordered JSON → markdown serialization | ~130 | `JSONToMarkdown(raw string) string` |
| internal/prompts/loader.go | Loads prompt defs from YAML dirs, modular prompts, and skill-generated prompts | ~229 | `LoadPromptDefinitions() ([]PromptDefinition, error)`; `GetPromptDefinition(name string) (PromptDefinition, bool)` |
| internal/prompts/registration.go | Registers prompts/resources on the MCP server | ~83 | `RegisterPrompts(srv *mcpserver.MCPServer, workspace string)`; `RegisterResources(srv *mcpserver.MCPServer)` |
| internal/prompts/resolver.go | Resolves prompt templates with args + string substitution | ~114 | `ResolvePrompt(p PromptDefinition, args map[string]string, workspace string) (string, error)`; `WriteDebugLog(msg string)` |
| internal/prompts/skilldetector.go | Detects skills from trigger phrases/names in args | ~185 | `DetectSkills(argText string, enableTrigger, enableName bool, alreadyInjected map[string]bool) ([]DetectedSkill, error)`; `LoadSkillContent(skillID string) (string, error)` |
| internal/prompts/climode.go | Rewrites MCP RPC text into CLI wrapper syntax (idempotent) | ~243 | `TransformMCPToCLI(text string) string`; `CLITool(mcpName string) string` |
| internal/prompts/parser.go + substitutions.go | Placeholder {{ARG}} parsing and template substitution | ~95 | `substitutePlaceholders(template string, args map[string]string, known map[string]bool) string`; `SubstituteTemplate(template string, args map[string]string, argDefs []PromptArgument) string` |
| internal/skills/reference_resolver.go | Loads skills, resolves file references and command hints | ~317 | `LoadSkills() ([]SkillRegistryEntry, error)`; `ResolveSkillContent(content, skillBaseDir, skillID string) ResolvedSkillContent`; `ScanKnowledgeBase(skillBaseDir, skillID string) ([]ResolvedReference, error)` |

### execution safety & shell

| File / Module | Role | LOC | Key Exports (with signatures) |
|---|---|---|---|
| internal/gatekeeper/gatekeeper.go | Path allow-lists, dangerous roots, interactive confirmations, command validation | ~552 | `New(store *shared.Store) *Gatekeeper`; `ValidatePathSafety(path, operationName string) error`; `ValidatePathSafetySync(path, operationName string) error`; `ValidateCommandPayload(command, execDir string) error`; `RequestUserConfirmation(description, targetPath string) Decision`; `IsPathAllowed(path string) bool` |
| internal/shell/exec/exec.go | Command + sandbox execution with double timeouts and PGID | ~311 | `Run(command, cwd string, timeoutMs, activityTimeoutMs int) Result`; `RunSandbox(name string, args []string, cwd, stdin string, activityMs, hardMs int) SandboxResult` |
| internal/shell/processes/processes.go | Global child-process registry + AbortAll | ~59 | `Register(cmd *exec.Cmd)`; `Unregister(cmd *exec.Cmd)`; `AbortAll()` |
| internal/shell/tokenoptimizer/tokenoptimizer.go | Output token savings engine: compactor per command, blacklist, profiles, redirect-to-file | ~1341 | `OptimizeOutput(command, output string, options Options, cfg Config) string`; `ApplyBlacklist(command, output string, blacklist []BlacklistEntry) *string`; `ApplyTokenProfiles(command, stdout, stderr string, options Options, cfg Config) ProfileResult`; `GetSavings(original, filtered string) int` |

### terminal & sidecars

| File / Module | Role | LOC | Key Exports (with signatures) |
|---|---|---|---|
| internal/terminal/commander.go | Raw-mode interactive REPL, command dispatch, tool execution | ~440 | `StartTerminalCommander(shutdownCh <-chan struct{})`; `ExecuteTool(name string, args map[string]any) string`; `MakeFakeRequest(args map[string]any) mcp.CallToolRequest`; `ParseCodegraphArgs(args []string) ParsedCodegraphArgs` |
| internal/terminal/exportcli.go | Generates `zen-*` shell wrapper scripts with short-flag aliases | ~514 | `ExportCLI(w io.Writer, cliPort, mcpPort int)`; `ExportCLIWithShort(w io.Writer, cliPort, mcpPort int, short bool)`; `ExportCliClean(w io.Writer)` |
| internal/terminal/handlers/*.go (14 files) | `init()`-registered CLI commands (browser, codegraph, gatekeeper, git, index, leech, memory, prompts, refactor, shell, skills, system, vision, workspace) | ~20–240 ea | init-only registration: `func init()` calling `commander.Register(name, handler)`; notable exports: `cd(args []string) error`, `runGitCommand(workdir string, args ...string) (string, error)`, `brainExtract(args []string) error` |
| internal/bridge/bridge.go | Firefox bridge JSON POST client + response sanitization | ~194 | `CallBridge(ctx, action string, params map[string]any) (map[string]any, error)`; `DecodeHTMLEntities(text string) string`; `FixMojibake(text string) string` |
| internal/agentbridge/agentbridge.go | Delegates chat to a web agent through the bridge | ~72 | `DelegateToWebAgent(ctx, params AgentChatParams) (string, error)` |
| internal/whiteboard/client.go | REST client for the whiteboard card service | ~256 | `NewClient(baseURL, slug, title, owner string) *Client`; `EnsureBoard(ctx) error`; `UpsertCard(ctx, slug, title, content, group string) error`; `LoadBoardState(ctx) (BoardState, error)`; `LinkCards(ctx, fromCard, toCard string) error` |
| internal/analysis/analysis.go + filetype.go | Analyzes command output (file-type detection, saved analysis) | ~344 | `AnalyzeOutput(text string) OutputAnalysis`; `DetectFileType(text string) FileTypeResult`; `StoreOutputAnalysis(dbPath, virtID string, analysis OutputAnalysis) error` |

### config & logging

| File / Module | Role | LOC | Key Exports (with signatures) |
|---|---|---|---|
| internal/mcpcfg/config.go | Merged YAML/JSON config with nested merge + fsnotify watch | ~515 | `Load() error`; `Get() *ZenConfig`; `GetToolConfig(toolName string) ToolConfig`; `WatchConfig(reload func()) func()`; `DaemonURL() string`; `FirefoxBridgeURL() string` |
| internal/mcpcfg/paths.go | Resolved config/wiki/map/prompt/skills/telemetry paths | ~58 | `ConfigFilePath() string`; `SkillsDir() string`; `TelemetryDir() string` |
| internal/workspace/resolver.go + resolve.go | Fuzzy workspace-path resolution + priority fallback chain | ~224 | `NewPathResolver(aliasMap map[string]string, cwd string) *PathResolver`; `Resolve(input string) (string, bool)`; `ResolveWorkspace(inputWorkspace, registrationWorkspace string, st *shared.Store) string` |
| internal/telemetry/telemetry.go | Tool-call telemetry to SQLite + CLI query | ~354 | `LogToolCall(tool, action string, success bool, errorMessage string, durationMs ...int64) error`; `QueryTelemetry(args []string) string`; `Close() error` |
| internal/logfilter/logfilter.go | Leveled logging with security bypass and stdio file redirect | ~169 | `Setup(level string)`; `SetStdioFile(path string) error`; `Info/Debug/Warn/Error(args ...any)`; `Infof/Debugf/Warnf/Errorf(format string, args ...any)` |

## Cross-References

Central files by fan-in/fan-out from codegraph `related`:

| File | Called by / calls | Why it's central |
|---|---|---|
| internal/server/routes.go | `main.go:runHTTPServers`; calls `shared.NewStore`, `workspace.Resolve`, `toolslist`, `patch.WrapHandlerWithTimeout`; tested by `routes_test.go` | Dispatches every incoming MCP call to the workspace-scoped mcpserver; 12 routes + workspace detection live here |
| internal/tools/types.go | All `defX` files (defBrowser, defShell, defThink, …), `server/tools.go:RegisterAllTools`, `terminal/exportcli.go:collectTools` | Single registration axis — every tool's schema flows through `AllDefs`/`Deps` |
| internal/toolresponse/response.go | Every `HandleX` wraps results via `WrapSuccess`/`WrapError`; `server/patch.go` timeout wrapper; `terminal/commander.go:ExecuteTool`; `tools/browser.go` … | Uniform result/error/telemetry/orphan handling for all tool output |
| internal/codegraph/engine.go | `tools/codegraph.go` (all 19 actions), `terminal/handlers/codegraph.go`; calls `storage.go`, `scanner.go`, `parser.go`, `languages_*.go` | Gateway to indexing + every graph query; layered session logic sits above it |
| internal/pooling/registry.go | `pooling/wrap.go`, `tools/pool.go:HandlePoolAction`, `server/tools.go:wrapIfPooled`; e2e via `server/pooling_e2e_test.go` | The 2025 async-pooling feature: job registration, long-poll, TTL/grace reaping |
| internal/logfilter/logfilter.go | `SetStdioFile`/`Setup`/`emit` referenced across all packages (810 refs) | Global leveled-logging glue; security-bypass filtering shared by every subsystem |

## Data Flow

```
MCP client ── HTTP JSON-RPC (POST /mcp, /tools/call, /initialize) ──▶ server.routes.SetupRoutes
   X-Workspace header or body.workspace ──▶ autoDetectWorkspace ─▶ serverCache.getOrCreate(logicalID)
      ──▶ mcpserver (mcp-go StreamableHTTPServer) ─▶ FilterEnabled ─▶ toolregistry handler
         ─▶ HandleX(ctx, req) ─▶ (gatekeeper|shell/exec|codegraph|projectmemory|pooling) ─▶ WrapSuccess/WrapError
Sidecars: Firefox bridge (POST) ◀─ tools/browser, bridge.CallBridge ; whiteboard REST ◀─ memoryisolate/shared
Terminal REPL ── commander.ExecuteTool ──▶ same tool handlers (MakeFakeRequest)
```

## Key Architectural Patterns

1. **Stateless per-workspace server cache**: `serverCache.getOrCreate(logicalID, factory, registry)` mints one `mcpserver` per workspace with LRU cap + `StartIdleReaper`; no MCP session IDs are ever stored (AGENTS.md routes.go:162).
2. **Tool = defX + HandleX pair**: each tool declares `defX(workspace, deps Deps) ToolDef` (JSON schema via `jsonSchema` helpers) plus a `HandleX(ctx, workspace, deps, req mcp.CallToolRequest) *mcp.CallToolResult` that dispatches on an `action` field; `tools.AllDefs` is the single registration list.
3. **Async job pooling behind a handler wrapper**: `pooling.Wrap(name, reg, inner)` runs the inner handler in a detached job and returns a reversible `pool_id`; `pooling.Registry` owns `Register/Complete/Cancel/LongPoll` with TTL+grace reaping and a live config toggle — the double-invocation hazard (a wrapped poll re-spawning a job) is explicitly guarded.
4. **Layered codegraph sessions**: `internal/tools/codegraph.go:discoverGraphRoots` finds the workspace root plus nested sub-graph DBs; `isolate` selects the graph layer, `sessionStale` triggers background re-index across root+sub-graphs in parallel.
5. **tree-sitter plugins + prepared-statement storage**: `LanguagePlugin` uniform interface over 9 grammar plugins feeding shared `ExtractQueryMatches`; `storage.go` pre-compiles all SQL statements once and persists nodes/edges/relations/FTS5 into modernc SQLite.
6. **Command-output token optimization**: `tokenoptimizer.OptimizeOutput` picks command-specific compactors (git status/diff/log, ls, grep, cat, test output), applies `ApplyBlacklist` and `ApplyTokenProfiles`, and `redirectToFile` spills huge output to disk instead of the context.
7. **Gatekeeper-guarded execution path**: `shell`/`run` handlers first pass through `gatekeeper.ValidatePathSafety` + `ValidateCommandPayload`; interactive operations use `RequestUserConfirmation` → `AcceptConfirmation`/`RejectConfirmation` round-trip.
8. **Terminal REPL reuses the MCP tool path**: `commander.ExecuteTool(name, args)` builds a `MakeFakeRequest` and calls the identical handlers the HTTP server would, so CLI state matches server state.

## Read Triggers

| If you need to… | Open these files |
|---|---|
| Add a new MCP tool | internal/tools/types.go (AllDefs + jsonSchema), a new defX/HandleX file, internal/server/tools.go (RegisterAllTools) |
| Change tool output wrapping/telemetry | internal/toolresponse/response.go (WrapSuccess/WrapError, SetTimeoutObserver, orphan flag) |
| Add or change a codegraph query action | internal/tools/codegraph.go (action* handlers), internal/codegraph/engine.go + storage.go (query impl) |
| Tune shell output compaction | internal/shell/tokenoptimizer/tokenoptimizer.go, internal/tools/shell.go (tokenOptConfig / toBlacklist) |
| Change pooling behavior (async jobs) | internal/pooling/registry.go + wrap.go, internal/server/tools.go (wrapIfPooled), internal/tools/pool.go, internal/mcpcfg config `pooling` block |
| Add/modify an HTTP route or workspace detection | internal/server/routes.go (SetupRoutes, detectWorkspace, autoDetectWorkspace), main.go (runHTTPServers) |
| Change project-memory brain schema | internal/projectmemory/timeline.go (MigrateToV3, AppendEvent), ftsindex.go (IndexActiveMemory) |
| Adjust safety/allow-list policy | internal/gatekeeper/gatekeeper.go (ValidatePathSafety, ValidateCommandPayload), internal/tools/shell.go |
| Edit prompt templates or CLI-mode rewriting | internal/prompts/loader.go, registration.go, resolver.go, climode.go (TransformMCPToCLI) |
| Add a terminal CLI command | internal/terminal/commander.go (Register/dispatch), new file under internal/terminal/handlers/ with `init()` |
| Touch the Firefox bridge protocol | internal/bridge/bridge.go (CallBridge), internal/tools/browser.go, mcpcfg config `firefoxBridge` |

## Dependencies

### MCP transport
| Package / Module | Role | Version |
|---|---|---|
| github.com/mark3labs/mcp-go | mcpserver, StreamableHTTPServer, mcp request/result types | v0.48.0 |

### Codegraph / parsing
| Package / Module | Role | Version |
|---|---|---|
| github.com/tree-sitter/go-tree-sitter | tree-sitter runtime binding | v0.25.0 |
| tree-sitter-grammars/tree-sitter-lua + 8 tree-sitter-* grammar packages | per-language grammars (C, C++, Go, Java, Lua, Python, Ruby, Rust, TS) | v0.23.x–v0.25.0 |
| github.com/sabhiram/go-gitignore | gitignore semantics for scanner exclusions | indirect |

### Storage & util
| Package / Module | Role | Version |
|---|---|---|
| modernc.org/sqlite | pure-Go SQLite driver (FTS5, codegraph + memory + telemetry DBs) | v1.34.5 |
| github.com/fsnotify/fsnotify | config-watch reload | v1.10.1 |
| gopkg.in/yaml.v3 | prompt/skill frontmatter + YAML loading | v3.0.1 |
| golang.org/x/sys | unix termios/raw terminal + process-group control | v0.30.0 |
| github.com/dustin/go-humanize | size/rate formatting (indirect) | indirect |

## Build & Run

| Command | Purpose |
|---|---|
| `go build .` | Build the zen-mcp server binary from main.go |
| `go test ./...` | Run full unit + e2e test suite |
| `go vet ./...` | Static analysis |
| `go run .` | Server run with env vars for config path / ports (see mcpcfg/paths.go) |