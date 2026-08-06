<!-- codegraph-file-count: 90 -->

# Zen MCP Server v2.4.1 — Go (stateless MCP over HTTP)

## Purpose
A stateless MCP (Model Context Protocol) server that exposes a codebase-research toolset to an MCP client (opencode). It is a Go port of the TypeScript `web-reader-mcp-master` project, mirroring its file layout and registration order. Primary output is a single `zen-mcp` binary (pure Go, SQLite via modernc) serving two HTTP listeners: a filtered MCP port and an unfiltered CLI port. It provides 16 tools: codegraph (tree-sitter code indexing), shell/run (with token optimization and sandboxing), browser (via a Firefox bridge), project memory, skills, prompts, and a terminal REPL plus exported CLI wrapper scripts.

## Architecture
```
HTTP (stateless MCP: POST /mcp) → server/routes.go (SetupRoutes, 12 routes)
  → serverCache → pool.AcquireServer → mcpserver.MCPServer (mark3labs/mcp-go)
      → RegisterAllTools → toolregistry → tools.AllDefs (16 tool defs)
          → subsystems: codegraph engine · shell exec · bridge · whiteboard · projectmemory
```

## File Tree
```
zen-mcp/
├── cmd/zen/main.go               # entry point: dual HTTP listeners, pool wiring
├── internal/
│   ├── server/                   # 12 HTTP routes, tool/prompt/resource registration, server pool
│   ├── tools/ + toolregistry/    # 16 MCP tool defs+handlers; registry + enable/disable
│   ├── codegraph/                # tree-sitter graph engine (SQLite/FTS5) + 10 language plugins
│   ├── shell/                    # exec + sandbox, processes, token optimizer + virtualizer
│   ├── terminal/ (+handlers/)    # REPL commander, CLI wrapper export, 13 command handlers
│   ├── mcpcfg/ + shared/         # config load/watch, path resolvers; shared Store, workspace
│   ├── gatekeeper/ + toolstate/  # path/command safety; per-workspace tool states
│   ├── toolresponse/ + toolsuggestions/  # result wrapping, virtualizer; call validation
│   ├── prompts/ + skills/        # prompt defs (YAML+skills), registration, skill detection
│   ├── projectmemory/ + analysis/  # timeline, FTS index, git signals; output/filetype analysis
│   ├── bridge/ + agentbridge/ + whiteboard/  # Firefox bridge, web-agent delegation, cards
│   └── telemetry/ + logfilter/   # SQLite telemetry; leveled logging
└── config.json · go.mod · .air.toml  # runtime config, module deps, auto-restart watch
```

## Tools
| Tool | File | Purpose |
|---|---|---|
| codegraph | internal/tools/codegraph.go | 20 graph actions (index, search, map, skeletons, related, impact…) over the SQLite index |
| shell | internal/tools/shell.go | Run shell commands with token-optimized, virtualized output |
| run | internal/tools/run.go | Sandboxed language execution (per config sandbox.languages) |
| browser | internal/tools/browser.go | Firefox control via bridge (navigate, eval, chat, screenshot) |
| memory | internal/tools/memory.go | Project memory load/save/scope |
| memory_isolate | internal/tools/memoryisolate.go | Whiteboard-backed isolated memory cards |
| memory_shared | internal/tools/memoryshared.go | Whiteboard-backed shared memory across projects |
| context | internal/tools/context.go | Retrieve indexed project context |
| workspace | internal/tools/workspace.go | Resolve/verify workspace roots |
| think | internal/tools/think.go | Sequential thinking + plan manager with task board |
| skills | internal/tools/skills.go | List/get skills with resolved references |
| ui_vision | internal/tools/uivision.go | Launch GUI, capture window via zen-cap, Gemini description |
| capture | internal/tools/capture.go | Screen capture (standard + collaborative) via zen-cap |
| colab | internal/tools/colab.go | Collaborative capture session |
| shell (repl) | internal/terminal/commander.go | Stdin REPL + ExecuteTool; exported CLI wrappers per tool |

## Component Roles
| File / Module | Role | LOC | Key Exports (with signatures) |
|---|---|---|---|
| cmd/zen/main.go | Entry point: stdio/HTTP modes, dual filtered+unfiltered listeners, idle reaper, REPL start | 224 | `newMcpServer(id string, reg *toolregistry.ToolRegistry, deps tools.Deps) *mcpserver.MCPServer`; `main()`; `runHTTPServers(startTime time.Time, cfg *mcpcfg.ZenConfig, store *shared.Store)` |
| internal/server/routes.go | 12 stateless HTTP routes incl. `POST /mcp`, shared-key store, collaborate, health | 307 | `SetupRoutes(mux *http.ServeMux, deps RouteDeps)`; `autoDetectWorkspace(r *http.Request, st *shared.Store) string`; `writeJSON(w, code, v)` |
| internal/server/tools.go | Register all tools on an MCPServer, apply filter, publish catalog | 82 | `RegisterAllTools(ctx, srv *mcpserver.MCPServer, reg *toolregistry.ToolRegistry, deps tools.Deps, workspace string) error`; `FilterEnabled(reg *toolregistry.ToolRegistry) func(ctx, []mcp.Tool) []mcp.Tool` |
| internal/server/catalog.go | `tools:catalog` resource content | 82 | `registerToolCatalogResource(srv *mcpserver.MCPServer, reg *toolregistry.ToolRegistry, deps tools.Deps)`; `buildToolCatalog(reg *toolregistry.ToolRegistry) string` |
| internal/server/pool.go | Cached MCPServer pool: acquire/release, inflight tracking, idle reaper, swap | 460 | `AcquireServer(cacheTag, logicalID string, factory Factory, registry *toolregistry.ToolRegistry, acquireTimeout ...time.Duration) (*mcpserver.MCPServer, error)`; `ReleaseServer(...)`; `SwapServer(...)`; `StartIdleReaper() func()`; `PoolServerFrom(ctx) *mcpserver.MCPServer` |
| internal/server/patch.go | Registration-time handler patches (timeout wrap, param summary) | 96 | `WrapHandlerWithTimeout(name string, inner toolregistry.Handler, getTimeout func(string) time.Duration) toolregistry.Handler`; `SummarizeParams(args map[string]any) string` |
| internal/server/shutdown.go | SIGINT/SIGTERM handling, idempotent | 13 | `SetupShutdownHandlers(mode string, logf func(format string, args ...any))` |
| internal/server/toolslist.go | Rewrite `tools/list` responses to annotate tools | 112 | `rewriteToolsListJSON(body []byte) ([]byte, bool)`; `toolsListRewriter(w http.ResponseWriter) *bufferingWriter` |
| internal/toolregistry/registry.go | Tool registry with enable/disable state | 91 | `Create() *ToolRegistry`; `(r *ToolRegistry) Track(reg ToolRegistration)`; `ListTools() []*ToolRegistration`; `SetToolEnabled(name string, enabled bool) bool` |
| internal/tools/types.go | ToolDef/Deps types; AllDefs registration order | 73 | `AllDefs(workspace string, deps Deps) []ToolDef`; `jsonSchema(properties map[string]any, required []string) map[string]any` |
| internal/tools/codegraph.go | codegraph tool: 20 actions over layered graph sessions | 1231 | `HandleCodegraphAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `getSessionByWorkspace(workspace string) (*layeredGraphSession, error)` |
| internal/tools/browser.go | Firefox bridge tool | 569 | `HandleBrowserAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `postJSON(ctx, url string, body map[string]any) (map[string]any, error)` |
| internal/tools/think.go | Sequential thinking + plan/task manager | 549 | `HandleThinkAction(ctx context.Context, workspace string, req mcp.CallToolRequest) *mcp.CallToolResult`; `logSessionEvent(workspace, typ, title, content string)` |
| internal/tools/memory.go | Project memory load/save/scope | 267 | `HandleMemoryAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `actionSave(dataDir, memoryName, dbPath, workspace, sessionTitle, objective, sessionNotes string) map[string]any` |
| internal/tools/memoryshared.go | Shared (cross-project) whiteboard memory | 262 | `HandleMemorySharedAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult` |
| internal/tools/memoryisolate.go | Isolated per-project whiteboard memory | 214 | `HandleMemoryIsolateAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult` |
| internal/tools/run.go | Sandboxed language execution | 199 | `HandleRunAction(ctx context.Context, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult` |
| internal/tools/shell.go | Shell command tool, wires token optimizer config | 180 | `HandleShellAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `tokenOptConfig(cfg mcpcfg.ZenConfig) tokenoptimizer.Config` |
| internal/tools/capture.go | zen-cap screen capture | 177 | `HandleCaptureAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `HandleStandardCapture(ctx, apiAddr, targetPath, mode string, args map[string]any, start time.Time) *mcp.CallToolResult` |
| internal/tools/workspaceresolver.go | Alias/path resolution for workspace inputs | 148 | `NewPathResolver(aliasMap map[string]string, cwd string) *PathResolver`; `(p *PathResolver) Resolve(input string) (string, bool)` |
| internal/tools/uivision.go | Launch GUI + zen-cap capture + Gemini describe | 114 | `HandleUiVisionAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `setPgidSysProcAttr() *unix.SysProcAttr` |
| internal/tools/context.go | Retrieve indexed context | 105 | `HandleContextAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult`; `actionRetrieveContext(dbPath, query string) map[string]any` |
| internal/tools/skills.go | List/get skills | 97 | `HandleSkillsAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult` |
| internal/tools/workspace.go | Workspace root verify/resolve tool | 92 | `HandleWorkspaceAction(ctx, path, workspace string, deps Deps) *mcp.CallToolResult` |
| internal/tools/colab.go | Collaborative capture session | 89 | `HandleColabAction(ctx, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult` |
| internal/codegraph/engine.go | CodeGraph engine facade over storage: index, query, mermaid, deadcode, impact | 931 | `NewCodeGraph(rootDir string) (*CodeGraph, error)`; `(cg *CodeGraph) Index() (*IndexResult, error)`; `GetRepositoryMap(maxItems int) (string, error)`; `RelatedFiles(filePath string, limit int) ([]RelatedRecord, error)`; `Impact(symbolName string) (string, error)`; `Status() (map[string]any, error)` |
| internal/codegraph/storage.go | SQLite (FTS5) storage: schema, node/edge CRUD, queries, dead code, cycles, shortest path | 1227 | `NewStorage(dbPath string) (*Storage, error)`; `SearchFTS(query string) ([]NodeSearchResult, error)`; `GetRelatedForFile(filePath string, limit int) ([]RelatedRecord, error)`; `FindDeadCode(query string, limit int) *DeadcodeResult`; `RunInTransaction(fn func(tx *sql.Tx) error) error` |
| internal/codegraph/scanner.go | Disk scan, ignore patterns, tsconfig alias loading, change detection | 355 | `NewScanner(storage *Storage, rootDir string) *Scanner`; `(s *Scanner) GetFilesToProcess() ([]FileRecord, error)`; `LoadTsConfigAliases()`; `ResolveAlias(specifier string) string` |
| internal/codegraph/parser.go | Singleton tree-sitter parser dispatch per extension | 247 | `GetParser() *Parser`; `(p *Parser) Parse(ext string, src []byte) ([]ParsedNode, []ParsedRelation, error)`; `ExtractQueryMatches(...) ` |
| internal/codegraph/types.go | Shared node/edge/relation/result types | 66 | structs: `ParsedNode`, `ParsedRelation`, `RelatedRecord`, `DeadcodeResult`, `ShortestPathResult`, `CycleRecord` |
| internal/codegraph/languages_base.go | LanguagePlugin interface + basePlugin (parser/language set) | 35 | interface `LanguagePlugin { setParser(...); getParser() (*tree_sitter.Parser, *tree_sitter.Language) }` |
| internal/codegraph/languages_*.go (10 files) | Per-language tree-sitter query plugins (c, cpp, go, java, lua, python, ruby, rust, typescript) | 55–176 | no exports (unexported plugin structs implementing `Parse`) |
| internal/shell/exec/exec.go | Command execution + sandbox runner | 311 | `Run(command, cwd string, timeoutMs, activityTimeoutMs int) Result`; `RunSandbox(name string, args []string, cwd, stdin string, activityMs, hardMs int) SandboxResult` |
| internal/shell/tokenoptimizer/tokenoptimizer.go | Command-specific output compaction, blacklist, token profiles, redirect virtualization | 1369 | `OptimizeOutput(command, output string, options Options, cfg Config) string`; `ApplyBlacklist(command, output string, blacklist []BlacklistEntry) *string`; `ApplyTokenProfiles(command, stdout, stderr string, options Options, cfg Config) ProfileResult`; `CountTokens(text string) int` |
| internal/shell/tokenoptimizer/virtualize.go | Virtualize oversized tool output to file with summary | 153 | `CheckAndVirtualizeOutput(toolName, text, workspaceRoot string) string`; `extractDistinctVocabulary(text string) []string` |
| internal/shell/processes/processes.go | Registered child-process tracking, AbortAll (SIGKILL) | 59 | `Register(cmd *exec.Cmd)`; `Unregister(cmd *exec.Cmd)`; `AbortAll()` |
| internal/terminal/commander.go | Stdin REPL command dispatch + ExecuteTool bridge | 249 | `Register(name string, h Handler)`; `ExecuteTool(name string, args map[string]any) string`; `StartTerminalCommander()`; `SetDeps(d tools.Deps)`; `FallbackPort(cliPort, mcpPort int, cliAvailable bool) int` |
| internal/terminal/exportcli.go | Generate/remove CLI wrapper scripts per tool | 87 | `ExportCLI(w io.Writer, cliPort, mcpPort int)`; `ExportCliClean(w io.Writer)` |
| internal/terminal/handlers/git.go | git review/archive commands | 214 | no exports (`init` registers handlers); `runGitCommand(workdir string, args ...string) (string, error)`; `buildReviewPrompt(...)` |
| internal/terminal/handlers/codegraph.go | codegraph CLI command | 229 | no exports (`init`) |
| internal/terminal/handlers/gatekeeper.go | allowed-paths CLI management | 188 | no exports (`init`) |
| internal/terminal/handlers/memory.go | brain extract command | 233 | no exports (`init`); `brainExtract(args []string) error` |
| internal/terminal/handlers/browser.go | browser CLI | 168 | no exports (`init`); `handleBrowserRequest(args []string) error` |
| internal/terminal/handlers/system.go | system/tool-cost info | 150 | no exports (`init`); `buildToolCost() string` |
| internal/terminal/handlers/prompts.go | prompt generation CLI | 142 | no exports (`init`); `generateCommands() error` |
| internal/terminal/handlers/refactor.go | refactor command | 103 | no exports (`init`) |
| internal/terminal/handlers/workspace.go | `cd` workspace command | 76 | no exports (`init`); `cd(args []string) error` |
| internal/terminal/handlers/index.go | RegisterAll entry for handlers package | 6 | `RegisterAll()` |
| internal/terminal/handlers/leech.go | leech command stub | 12 | no exports |
| internal/terminal/handlers/shell.go | shell command stub | 18 | no exports |
| internal/terminal/handlers/skills.go | skills CLI | 21 | no exports |
| internal/terminal/handlers/vision.go | vision CLI | 25 | no exports |
| internal/mcpcfg/config.go | ZenConfig struct, defaults, JSON merge, live reload watcher, URL helpers | 472 | `Get() *ZenConfig`; `Load() error`; `GetToolConfig(toolName string) ToolConfig`; `DaemonURL() string`; `WatchConfig(reload func()) func()` |
| internal/mcpcfg/paths.go | Resolved project paths (config, prompts, skills, telemetry) | 58 | `ConfigFilePath() string`; `PromptDir() string`; `SkillsDir() string`; `TelemetryDir() string` |
| internal/gatekeeper/gatekeeper.go | Path safety, allowed-paths persistence, command payload validation, confirmations | 552 | `New(store *shared.Store) *Gatekeeper`; `(g *Gatekeeper) ValidatePathSafety(path, operationName string) error`; `ValidateCommandPayload(command, execDir string) error`; `RequestUserConfirmation(description, targetPath string) Decision`; `AcceptConfirmation(id string) bool`; `IsPathAllowed(path string) bool` |
| internal/toolresponse/response.go | Unified result wrapping, output rendering, tool context, virtualizer hook, schema store | 395 | `WrapSuccess(ctx context.Context, tool string, data any, start time.Time) *mcp.CallToolResult`; `WrapError(tool string, err error, start time.Time) *mcp.CallToolResult`; `RenderOutput(format string, data any) string`; `SetVirtualizer(fn func(tool, text string) (string, error))`; `SetToolSchema(name string, schema map[string]any)` |
| internal/toolstate/toolstate.go | Per-workspace tool enable/disable resolution and application | 142 | `ResolveToolState(name string, workspaceRoot string, workspaceCfg map[string]bool, reg *toolregistry.ToolRegistry) ToolStateLayers`; `ApplyToolStates(workspaceRoot string, reg *toolregistry.ToolRegistry) ApplyResult` |
| internal/toolsuggestions/suggestions.go | Tool-call validation, mistake correction, schema introspection, semantic placeholders | 405 | `GetToolSuggestion(toolName string) *ToolSuggestion`; `ValidateToolCall(toolName, action string, suppliedParams map[string]any, schema any) ValidationResult`; `FormatSuggestion(...)`; `FindMistakeCorrection(...)` |
| internal/prompts/loader.go | Load prompt definitions from YAML + skills, frontmatter parsing | 228 | `LoadPromptDefinitions() ([]PromptDefinition, error)`; `GetPromptDefinition(name string) (PromptDefinition, bool)`; `generateSkillPrompts() ([]PromptDefinition, error)` |
| internal/prompts/skilldetector.go | Detect skills from prompt args/triggers | 179 | `DetectSkills(argText string, enableTrigger, enableName bool, alreadyInjected map[string]bool) ([]DetectedSkill, error)`; `LoadSkills() ([]Skill, error)`; `LoadSkillContent(skillID string) (string, error)` |
| internal/prompts/registration.go | Register prompts + tools/catalog resource on server | 82 | `RegisterPrompts(srv *mcpserver.MCPServer, workspace string)`; `RegisterResources(srv *mcpserver.MCPServer)` |
| internal/prompts/resolver.go | Resolve prompt template with args; debug log | 99 | `ResolvePrompt(p PromptDefinition, args map[string]string, workspace string) (string, error)`; `WriteDebugLog(msg string)` |
| internal/prompts/substitutions.go | `{{arg}}` template substitution | 14 | `SubstituteTemplate(template string, args map[string]string, argDefs []PromptArgument) string`; `WarnMissingArgs(...)` |
| internal/prompts/types.go | PromptDefinition/PromptArgument/Skill types | 74 | structs only |
| internal/projectmemory/timeline.go | JSONL timeline events with v1→v3 migration, state reconstruction | 314 | `AppendEvent(dataDir, memoryName string, event BrainEvent) error`; `ReconstructState(dataDir, memoryName string) ReconstructedState`; `MigrateToV3(raw map[string]any) BrainEvent`; `NormalizeKey(s string) string` |
| internal/projectmemory/ftsindex.go | SQLite FTS5 memory index, virtual context, recent commands, event log | 499 | `IndexActiveMemory(dbPath string, items []MemoryIndexItem)`; `SearchIndexedMemory(dbPath, query string, limit int) []SearchResult`; `IndexVirtualContext(dbPath string, data VirtualContextData) string`; `RecentCommands(dbPath string) []RecentCommand` |
| internal/projectmemory/gitsignals.go | Git commit/branch signals since last visit | 88 | `GetGitSignals(workspaceRoot, lastVisitedIso string) GitSignals` |
| internal/projectmemory/registermap.go | map.json registration + lastVisited stamp | 62 | `RegisterProjectInMap(projectPath string, dependencies []string)` |
| internal/projectmemory/markdown.go | Memory markdown rendering | 130 | no exports (internal helpers) |
| internal/analysis/analysis.go | Output analysis + persistence | 81 | `AnalyzeOutput(text string) OutputAnalysis`; `StoreOutputAnalysis(dbPath, virtID string, analysis OutputAnalysis) error`; `GetStoredAnalysis(dbPath, virtID string) *OutputAnalysis` |
| internal/analysis/filetype.go | Heuristic file-type detection (json/html/yaml/log/csv/diff…) | 263 | `DetectFileType(text string) FileTypeResult`; `isBinary(sample string) bool`; `guessDelimiter(rows []string) string` |
| internal/analysis/readingadvice.go | Suggest reading tool from file type | 79 | `SuggestReadingTool(ft FileTypeResult) ReadingAdvice` |
| internal/skills/reference_resolver.go | Skill registry, frontmatter, reference/knowledge-base scanning | 317 | `LoadSkills() ([]SkillRegistryEntry, error)`; `FindSkillByID(id string) (SkillRegistryEntry, error)`; `ResolveSkillContent(content, skillBaseDir, skillID string) ResolvedSkillContent` |
| internal/skills/types.go | Skill registry types | 31 | structs only |
| internal/shared/state.go | Shared key/value store with change callbacks | 79 | `NewStore() *Store`; `(s *Store) Set(key, value string)`; `Get(key string) (string, bool)`; `OnChange(key string, fn func(string)) func()` |
| internal/shared/workspace.go | Workspace resolution cascade | 33 | `ResolveWorkspace(inputWorkspace, registrationWorkspace string, st *Store) string` |
| internal/bridge/bridge.go | Firefox bridge HTTP client with response sanitization | 199 | `CallBridge(ctx context.Context, action string, params map[string]any) (map[string]any, error)`; `DecodeHTMLEntities(text string) string`; `FixMojibake(text string) string` |
| internal/agentbridge/agentbridge.go | Delegate chat to web agent via Firefox bridge | 72 | `DelegateToWebAgent(ctx context.Context, params AgentChatParams) (string, error)` |
| internal/whiteboard/client.go | Whiteboard board/card client (REST) | 256 | `NewClient(baseURL, slug, title, owner string) *Client`; `EnsureBoard(ctx) error`; `UpsertCard(ctx, slug, title, content, group string) error`; `LoadBoardState(ctx) (BoardState, error)`; `LinkCards(ctx, fromCard, toCard string) error` |
| internal/whiteboard/slug.go | Derive whiteboard slug from workspace/git | 141 | `ResolveProjectSlug(workspace string) SlugInfo` |
| internal/telemetry/telemetry.go | SQLite tool-call telemetry + CLI query | 299 | `LogToolCall(tool string, action string, success bool, errorMessage string) error`; `QueryTelemetry(args []string) string`; `Close() error` |
| internal/logfilter/logfilter.go | Leveled logging with stdio redirect and severity filtering | 169 | `Setup(level string)`; `Debug/Info/Warn/Error(...any)`; `Debugf/Infof/Warnf/Errorf(format string, args ...any)`; `SetStdioFile(path string) error` |

## Cross-References
| File | Called by / calls | Why it's central |
|---|---|---|
| internal/terminal/commander.go | ← all 13 handler `init`s register commands; → `ExecuteTool`, `Logf`, `Register` | REPL dispatch + tool invocation hub; every terminal command routes through it |
| internal/codegraph/engine.go | ← tools/codegraph.go `getSessionByWorkspace`; → `Storage` (Search/GetRepositoryMap), 10 language plugins `Parse` | Single engine facade over the SQLite graph; all codegraph tool actions land here |
| internal/gatekeeper/gatekeeper.go | ← shell, run, workspace, codegraph, capture tools; → `ValidatePathSafety`, `ValidateCommandPayload`, confirmations | Choke point for every path/command safety decision across tools and CLI |
| internal/toolresponse/response.go | ← every tool handler; → `WrapSuccess`, `WrapError`, `RenderOutput`, virtualizer | Universal result normalization; all tool outputs/errors pass through it |
| internal/server/routes.go | ← cmd/zen/main.go (both listeners); → `serverCache.getOrCreate`, `postMCP`, `writeJSON` | Front door of the HTTP surface; owns stateless-session semantics |
| internal/tools/types.go | → all 16 `defX` factories via `AllDefs` | Defines the tool set and its TS-mirrored registration order |

## Data Flow
```
MCP client ──POST /mcp (JSON-RPC, stateless)──► SetupRoutes (server/routes.go)
   │  initialize · tools/list · tools/call · prompts/list
   ▼
serverCache.getOrCreate → pool.AcquireServer → newMcpServer → RegisterAllTools
   │                                                        (toolregistry → tools.AllDefs)
   ▼
tool handler (defX + HandleXAction)
   ├─► codegraph engine (SQLite/FTS5)        └─► gatekeeper.ValidatePathSafety / ValidateCommandPayload
   ├─► shell exec (sandbox) + tokenoptimizer        (confirmations → Accept/Reject)
   ├─► bridge → Firefox / agentbridge / whiteboard
   └─► toolresponse.WrapSuccess/WrapError ──► JSON-RPC result
CLI port (unfiltered registry): same routes; terminal REPL (stdin) + exported CLI wrappers → ExecuteTool
```

## Key Architectural Patterns
1. Stateless MCP transport: every `POST /mcp` is a fresh session (SSE and DELETE `/mcp` explicitly rejected); a pool of cached `MCPServer` instances keyed by workspace (`AcquireServer`/`ReleaseServer`, inflight-aware `SwapServer`, idle reaper) is reused across calls.
2. Dual-server, filtered vs unfiltered registries: identical `SetupRoutes` served on `mcpPort` (filtered) and `cliPort` (unfiltered); `FilterEnabled` closure + per-workspace `ApplyToolStates` gate the MCP surface while CLI wrappers hit the unfiltered one.
3. `defX` + `HandleXAction` tool convention: each tool is a `ToolDef` (JSON schema) plus a handler; `tools.AllDefs` fixes TS registration order; `toolresponse.WrapSuccess/WrapError` normalizes every result/error.
4. Registration-time patching: `server/patch.go` `WrapHandlerWithTimeout` + `SummarizeParams` mirror the TS `patch-mcp.ts`, applied when tools are registered.
5. Tree-sitter language plugins: `LanguagePlugin` interface + `basePlugin` + one unexported plugin per language feeding the `Parser` singleton → `Scanner` → `Storage` (SQLite FTS5) → query API.
6. Gatekeeper as single safety choke point: `ValidatePathSafety` / `ValidateCommandPayload` plus queued user confirmations guard every path/command entry.
7. Token-optimization pipeline: shell output → `tokenoptimizer.OptimizeOutput` (per-command compactors, blacklist, token profiles) → `CheckAndVirtualizeOutput` redirect to file with a summary vocab, wired via `toolresponse.SetVirtualizer`.
8. JSONL timeline with schema migration: `projectmemory/timeline.go` appends `BrainEvent` lines and reconstructs state, migrating v1→v2→v3 shapes (`MigrateToV3`).

## Read Triggers
| If you need to... | Open these files |
|---|---|
| Add a new MCP tool | internal/tools/types.go (AllDefs), copy a `defX`+`HandleXAction` file, internal/server/tools.go (RegisterAllTools), internal/toolsuggestions/suggestions.go |
| Add a codegraph action | internal/tools/codegraph.go (action* funcs), internal/codegraph/engine.go (query method), internal/codegraph/storage.go |
| Change shell execution or sandbox | internal/shell/exec/exec.go, internal/tools/shell.go, internal/tools/run.go, internal/shell/processes/processes.go |
| Compact output of a new command | internal/shell/tokenoptimizer/tokenoptimizer.go (compact* funcs), internal/tools/shell.go (tokenOptConfig), tokenoptimizer/virtualize.go |
| Add or adjust an HTTP endpoint | internal/server/routes.go (SetupRoutes), cmd/zen/main.go (runHTTPServers) |
| Change tool enable/disable resolution | internal/toolstate/toolstate.go, internal/server/tools.go (FilterEnabled), internal/server/pool.go |
| Change path or command safety rules | internal/gatekeeper/gatekeeper.go, internal/tools/workspaceresolver.go |
| Add a prompt or skill | internal/prompts/loader.go, internal/prompts/registration.go, internal/prompts/skilldetector.go, internal/skills/reference_resolver.go |
| Extend project memory events | internal/projectmemory/timeline.go, internal/projectmemory/ftsindex.go, internal/projectmemory/gitsignals.go |
| Modify browser/bridge integration | internal/bridge/bridge.go, internal/tools/browser.go, internal/agentbridge/agentbridge.go |
| Add a terminal command | internal/terminal/commander.go (Register), internal/terminal/handlers/<new>.go, internal/terminal/handlers/index.go |
| Tweak config loading/defaults | internal/mcpcfg/config.go, internal/mcpcfg/paths.go |

## Dependencies
### Runtime
| Package / Module | Role | Version |
|---|---|---|
| github.com/mark3labs/mcp-go | MCP server framework (tools, prompts, resources, capabilities) | v0.48.0 |
| modernc.org/sqlite | Pure-Go SQLite for codegraph, memory, telemetry (FTS5 via build tag) | v1.34.5 |
| tree-sitter/go-tree-sitter | Tree-sitter runtime for codegraph parsing | v0.25.0 |
| tree-sitter grammars (c, cpp, go, java, lua, python, ruby, rust, typescript) | Per-language parse grammars | 0.23–0.25 |
| github.com/fsnotify/fsnotify | config.json live-reload watcher | v1.10.1 |
| golang.org/x/sys | unix.SysProcAttr process-group handling for sandbox | v0.30.0 |
| gopkg.in/yaml.v3 | Prompt/skill frontmatter parsing | v3.0.1 |

### Indirect (top)
| Package / Module | Role |
|---|---|
| google/uuid, google/jsonschema-go | ID + schema generation (mcp-go) |
| github.com/sabhiram/go-gitignore, spf13/cast, dustin/go-humanize, yosida95/uritemplate/v3 | ignore matching, casting, formatting, URI templates |
| modernc.org/libc, mathutil, memory | sqlite runtime support |

## Build & Run
| Command | Purpose |
|---|---|
| `go build -tags fts5 -o zen-mcp ./cmd/zen` | Build the server binary (tags required for FTS5) |
| `air` | Auto-restart dev loop per .air.toml (rebuild + run) |
| `./run.sh` | Build, watch-rebuild, and run the binary |
| `./zen-mcp --stdio` | Run in stdio transport mode |
| `go test -tags fts5 ./...` | Run the full test suite |
