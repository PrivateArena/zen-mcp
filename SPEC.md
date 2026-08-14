# zen-mcp Technical Specification

## 1. System Overview & Scope

### Problem Statement & Purpose

zen-mcp is a Go-native, high-throughput MCP (Model Context Protocol) server that replaces the original TypeScript implementation for the Zen IDE/Browser. It exposes ~24 domain tools plus a terminal REPL and a CLI wrapper generator. All tool logic (shell/run sandboxing, Firefox bridge automation, codegraph indexing/query, project memory, sequential thinking, async job pooling, skills/prompts, whiteboard-backed shared memory) lives directly in Go with zero runtime dependency on the TS codebase. The server runs in **stateless mode** (no MCP session ID negotiation) over a Streamable HTTP transport backed by `mark3labs/mcp-go`, with per-workspace `mcpserver` instances cached and LRU-reaped.

### System Architecture

```
HTTP JSON-RPC (12 stateless routes, X-Workspace / body workspace detect)
  → server.routes.postMCP → serverCache.getOrCreate(workspace) → mcpserver
    → tools.AllDefs[defX] → HandleX → toolresponse.Wrap{Success,Error}
  Core subsystems: codegraph (tree-sitter→SQLite+FTS5) · projectmemory (.zenmcp brain)
  · pooling (async job registry) · gatekeeper (path/command safety) · shell/exec+tokenoptimizer
  Sidecars: terminal REPL (commander.ExecuteTool) · Firefox bridge · whiteboard client
```

## 2. Directory & Module Topology

### Folder Structure

```
zen-mcp/
  main.go                          # entry point: config load, 2 HTTP servers, terminal commander
  go.mod                           # Go 1.24 module manifest
  PROJECT_OVERVIEW.md              # project architecture overview (primary doc)
  internal/
    server/                        # stateless MCP HTTP layer, server cache, pooling wiring
    tools/                         # tool definitions (defX) + handlers (HandleX), Deps/ToolDef, AllDefs
    toolregistry/                  # registry of tool name → registration (handler, schema, enabled)
    toolresponse/                  # uniform CallToolResult wrapping, telemetry, timeouts
    toolstate/                     # workspace + global enable-state layering
    toolsuggestions/               # action validation & mistake correction
    pooling/                       # async job registry + handler wrapper
    gatekeeper/                    # path/command safety: ValidatePathSafety, interactive confirmations
    shell/
      exec/                        # command + sandbox execution with double timeouts
      processes/                   # global child-process registry + AbortAll
      tokenoptimizer/              # output token savings: compactor, blacklist, profiles
    codegraph/                     # tree-sitter indexer, SQLite+FTS5 storage, 9 language plugins
    projectmemory/                 # brain timeline (v1→v3), FTS index, git signals, virtual context
    prompts/                       # prompt templates (YAML+frontmatter), CLI-mode transform, skill detection
    skills/                        # skill registry, file reference resolver
    terminal/
      commander.go                 # raw-mode REPL, command dispatch, tool execution
      exportcli.go                 # generates zen-* shell wrapper scripts
      handlers/                    # 14 CLI command handlers (init()-registered)
    bridge/                        # Firefox bridge JSON POST client
    agentbridge/                   # web-agent delegation through bridge
    analysis/                      # output file-type detection, saved analysis
    whiteboard/                    # whiteboard REST client (card service)
    mcpcfg/                        # config (merge+watch), path resolution
    shared/                        # concurrent string KV store, per-key change callbacks
    telemetry/                     # tool-call telemetry to SQLite + CLI query
    logfilter/                     # leveled logging filter, stdio file redirect
    workspace/                     # fuzzy workspace-path resolution
```

### Component Breakdown

| File / Module | Role | LOC (approx.) |
|---|---|---|
| `main.go` | Entry point: config init, newMcpServer, runHTTPServers, SIGTERM | ~245 |
| `internal/server/routes.go` | 12 stateless HTTP routes, workspace detection, server cache dispatch | ~460 |
| `internal/tools/codegraph.go` | 16+ codegraph actions over layered root/sub-graph DBs | ~1320 |
| `internal/codegraph/storage.go` | SQLite (modernc) persistence: schema, prepared statements, FTS5, relations | ~1349 |
| `internal/shell/tokenoptimizer/tokenoptimizer.go` | Output token savings: compactor, blacklist, profiles, redirect-to-file | ~1341 |
| `internal/tools/think.go` | Sequential thinking + plan/task manager | ~549 |
| `internal/tools/browser.go` | Firefox bridge browser control | ~538 |
| `internal/terminal/exportcli.go` | Generates `zen-*` shell wrapper scripts | ~568 |
| `internal/gatekeeper/gatekeeper.go` | Path/command safety, interactive confirmations | ~552 |
| `internal/projectmemory/ftsindex.go` | Memory FTS index, virtual context, recent commands | ~499 |
| `internal/toolresponse/response.go` | Uniform CallToolResult wrapping, telemetry, orphan suppression | ~460 |

## 3. Component Design & Core Contracts

### Interfaces & Type Definitions

#### Tool Registration Axis (`internal/tools/types.go`)

```go
// Deps is the single dependency bundle injected into every tool handler.
type Deps struct { /* fields: CodeGraph, Whiteboard, Bridge, Config, Store, Pooling, ... */ }

// ToolDef declares a tool's schema and handler factory.
type ToolDef struct {
    Name        string
    Description string
    InputSchema map[string]any  // JSON Schema
    Annotations *mcp.ToolAnnotation
    // defX(workspace string, deps Deps) ToolDef
}

// AllDefs returns the full registered tool set in TS registration order.
func AllDefs(workspace string, deps Deps) []ToolDef

// jsonSchema is the shared helper for building input schemas.
func jsonSchema(properties map[string]any, required []string) map[string]any
```

#### Handler Signature (`internal/toolregistry/registry.go`)

```go
// Handler is the universal tool handler contract.
type Handler func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

// ToolRegistration binds a handler to its schema and enabled state.
type ToolRegistration struct {
    Name        string
    Schema      map[string]any
    Handler     Handler
    Enabled     bool
}

type ToolRegistry struct { /* name → ToolRegistration map + mu */ }

func Create() *ToolRegistry
func (r *ToolRegistry) Track(reg ToolRegistration)
func (r *ToolRegistry) GetTool(name string) (*ToolRegistration, bool)
func (r *ToolRegistry) SetToolEnabled(name string, enabled bool) bool
func (r *ToolRegistry) ListTools() []*ToolRegistration
```

#### Server Dispatch & Cache (`internal/server/routes.go`)

```go
// RouteDeps bundles dependencies for the HTTP layer.
type RouteDeps struct {
    Config     *mcpcfg.ZenConfig
    Store      *shared.Store
    ToolDeps   tools.Deps
    Registry   *toolregistry.ToolRegistry
    AbortObs   func(method string, elapsedMs int64, reason string)
}

// serverCache is an LRU cache of per-workspace *mcpserver.MCPServer.
type serverCache struct { mu sync.Mutex; cap int; entries map[string]*entry; ... }

func newServerCache() *serverCache
func (c *serverCache) getOrCreate(logicalID string, factory func(string) *mcpserver.MCPServer, registry *toolregistry.ToolRegistry) *mcpserver.MCPServer

// SetupRoutes registers the 12 stateless HTTP routes.
func SetupRoutes(mux *http.ServeMux, deps RouteDeps)

// detectWorkspace resolves workspace from body or X-Workspace header.
func detectWorkspace(msg rpcMessage, r *http.Request, st *shared.Store) string
func autoDetectWorkspace(r *http.Request, st *shared.Store) string
```

#### Result Wrapping (`internal/toolresponse/response.go`)

```go
type ToolContext struct { Tool string; Action string }

func WrapSuccess(ctx context.Context, tool string, data any, start time.Time) *mcp.CallToolResult
func WrapError(tool string, err error, start time.Time) *mcp.CallToolResult
func WrapErrorWithContext(ctx context.Context, tool string, err error, start time.Time) *mcp.CallToolResult
func RenderOutput(format string, data any) string
func SetTimeoutObserver(fn func(tool, kind string, elapsedMs int64))
func SetToolSchema(name string, schema map[string]any)
func GetToolSchema(name string) map[string]any
func WithToolContext(ctx context.Context, tc ToolContext) context.Context
func MarkWithOrphanFlag(ctx context.Context, flag *atomic.Bool) context.Context
```

#### Async Pooling (`internal/pooling/registry.go`)

```go
type Job struct {
    ID        string
    Name      string
    State     string          // pending | running | done | cancelled
    Result    *mcp.CallToolResult
    CreatedAt time.Time
    TTL       time.Duration
    Grace     time.Duration
}

type PollOutcome struct { State string; Result *mcp.CallToolResult }

type Registry struct { mu sync.Mutex; jobs map[string]*Job; ... }

func NewRegistry(ttl, grace time.Duration, max int) *Registry
func (r *Registry) Register(name string, job *Job) (string, error)
func (r *Registry) Complete(id string, res *mcp.CallToolResult) bool
func (r *Registry) Cancel(id string) bool
func (r *Registry) LongPoll(ctx context.Context, id string, wait time.Duration) PollOutcome
func (r *Registry) State(id string) string
func (r *Registry) List() []JobInfo
func (r *Registry) EvictExpired(now time.Time) int
func Global() *Registry
```

#### Gatekeeper (`internal/gatekeeper/gatekeeper.go`)

```go
type Decision struct { Approved bool; PendingID string }

type PendingInfo struct { ID, Description, TargetPath string }

type Gatekeeper struct { store *shared.Store; ... }

func New(store *shared.Store) *Gatekeeper
func (g *Gatekeeper) ValidatePathSafety(path, operationName string) error
func (g *Gatekeeper) ValidatePathSafetySync(path, operationName string) error
func (g *Gatekeeper) ValidateCommandPayload(command, execDir string) error
func (g *Gatekeeper) RequestUserConfirmation(description, targetPath string) Decision
func (g *Gatekeeper) AcceptConfirmation(id string) bool
func (g *Gatekeeper) RejectConfirmation(id string, suggestion string) bool
func (g *Gatekeeper) IsPathAllowed(path string) bool
func (g *Gatekeeper) GetDangerousRoots() []string
```

#### Codegraph Engine (`internal/codegraph/engine.go`)

```go
type CodeGraph struct { rootDir string; storage *Storage; scanner *Scanner; parser *Parser }

func NewCodeGraph(rootDir string) (*CodeGraph, error)
func (cg *CodeGraph) Close() error
func (cg *CodeGraph) Index() (*IndexResult, error)
func (cg *CodeGraph) Search(query string, limit int) ([]NodeSearchResult, error)
func (cg *CodeGraph) GetSkeleton(relPath string) (string, error)
func (cg *CodeGraph) RelatedFiles(filePath string, limit int) ([]RelatedRecord, error)
func (cg *CodeGraph) FindDeadCode(query string, limit int) (*DeadcodeResult, error)
func (cg *CodeGraph) FindShortestPath(from, to string, limit int) (*ShortestPathResult, error)
func (cg *CodeGraph) FindCycles() ([]CycleRecord, error)
func (cg *CodeGraph) Impact(symbolName string) (string, error)
func (cg *CodeGraph) Explain(symbolName string) (string, error)
func (cg *CodeGraph) Map() (string, error)
func (cg *CodeGraph) GenerateMermaid(query string, limit int) (string, error)
func (cg *CodeGraph) Status() (map[string]any, error)
```

#### Codegraph Storage (`internal/codegraph/storage.go`)

```go
type Storage struct { db *sql.DB; stmts map[string]*sql.Stmt; mu sync.RWMutex }

func NewStorage(dbPath string) (*Storage, error)
func (s *Storage) Close() error
func (s *Storage) UpsertFile(fr FileRecord) (int64, error)
func (s *Storage) InsertNodes(nodes []NodeRecord) (int64, error)
func (s *Storage) InsertEdges(edges []EdgeRecord) error
func (s *Storage) SearchFTS(query string) ([]NodeSearchResult, error)
func (s *Storage) FindNodesByName(name string) ([]NodeRecord, error)
func (s *Storage) GetNeighbors(nodeID int64, limit int) (callers []NodeRecord, callees []NodeRecord, err error)
func (s *Storage) GetRelatedForFile(filePath string, limit int) ([]RelatedRecord, error)
func (s *Storage) FindDeadCode(query string, limit int) *DeadcodeResult
func (s *Storage) FindShortestPath(fromName, toName string, limit int) (*ShortestPathResult, error)
func (s *Storage) FindCycles() ([]CycleRecord, error)
func (s *Storage) GetAllFiles() []FileRecord
func (s *Storage) GetAllEdges(pathFilter string, limit int) []EdgeRecord
func (s *Storage) RunInTransaction(fn func(tx *sql.Tx) error) error
func (s *Storage) SetMetadata(key, value string) error
func (s *Storage) GetMetadata(key string) string
func (s *Storage) GetStats() map[string]int
```

```go
type NodeRecord struct {
    ID, FileID       int64
    Name, Kind       string
    QualifiedName    string
    Line, Col        int
    Signature        string
    Language         string
}
type FileRecord struct { ID int64; Path, Language, Hash string; MTime int64 }
type EdgeRecord struct { SourceID, TargetID int64; Relation, Metadata string }
type NodeSearchResult struct { Node NodeRecord; Score float64 }
```

#### Project Memory Timeline (`internal/projectmemory/timeline.go`)

```go
type BrainEvent struct {
    SchemaVersion int
    Type          string
    Title         string
    Content       string
    Timestamp     time.Time
    // v3 fields...
}

type ReconstructedState struct { Events []BrainEvent; /* ... */ }

func MigrateToV3(raw map[string]any) BrainEvent
func AppendEvent(dataDir, memoryName string, event BrainEvent) error
func ReconstructState(dataDir, memoryName string) ReconstructedState
func LatestEvent(dataDir, memoryName string) (BrainEvent, bool)
```

#### Project Memory FTS Index (`internal/projectmemory/ftsindex.go`)

```go
type MemoryIndexItem struct { Type, Title, Content string; Metadata map[string]any }
type VirtualContextData struct { ID, Title, Content string; Metadata map[string]any }
type SearchResult struct { Title, Content string; Score float64 }
type RecentCommand struct { Title, Command string; Timestamp time.Time }

func GetDatabase(dbPath string) *sql.DB
func ClearAllDatabaseCache()
func IndexActiveMemory(dbPath string, items []MemoryIndexItem)
func SearchIndexedMemory(dbPath, query string, limit int) []SearchResult
func IndexVirtualContext(dbPath string, data VirtualContextData) string
func RetrieveVirtualContext(dbPath, id, filterQuery string) string
func RecentCommands(dbPath string) []RecentCommand
```

#### Shared Concurrent Store (`internal/shared/state.go`)

```go
type Store struct { mu sync.RWMutex; data map[string]string; watchers map[string][]func(string) }

func NewStore() *Store
func (s *Store) Set(key, value string)
func (s *Store) Get(key string) (string, bool)
func (s *Store) GetAll() map[string]string
func (s *Store) OnChange(key string, fn func(string)) func()
func (s *Store) Clear()
```

#### Config (`internal/mcpcfg/config.go`)

```go
type ZenConfig struct {
    MCPPort, CLIPort          int
    DataDir                   string
    WorkspaceDir              string
    ConfigPaths               []string
    DaemonURL, ProxyURL       string
    FirefoxBridgeURL          string
    Pooling                   PoolingConfig
    TokenOptimization         TokenOptimizationConfig
    Sandbox                   SandboxConfig
    PromptFeature             PromptFeatureConfig
    Wiki                      WikiConfig
    ToolConfig                map[string]json.RawMessage
    Blacklist                 []BlacklistEntry
}

type PoolingConfig struct { Enabled bool; TTL, Grace time.Duration; MaxJobs int }
type TokenOptimizationConfig struct { Enabled bool; Profiles map[string]json.RawMessage }
type SandboxConfig struct { Languages []SandboxLanguage }
type BlacklistEntry struct { Pattern string; Tools []string }
```

```go
func Load() error
func Get() *ZenConfig
func GetToolConfig(toolName string) ToolConfig
func WatchConfig(reload func()) func()
func DaemonURL() string
func FirefoxBridgeURL() string
```

#### Terminal Commander (`internal/terminal/commander.go`)

```go
type Handler func(args []string) error

func Register(name string, h Handler)
func Get(name string) (Handler, bool)
func List() []string
func StartTerminalCommander(shutdownCh <-chan struct{})
func WaitTerminalCommander()
func MakeFakeRequest(args map[string]any) mcp.CallToolRequest
func ExecuteTool(name string, args map[string]any) string
func SetDeps(d tools.Deps)
func GetDeps() tools.Deps
```

### Subsystem Dispatch & IPC

#### HTTP JSON-RPC Routes (`internal/server/routes.go`)

The 12 stateless routes registered via `SetupRoutes`:
- `POST /mcp` — primary MCP JSON-RPC endpoint (handles `initialize`, `tools/list`, `tools/call`, `notifications/...`)
- All routes resolve workspace from `X-Workspace` header or `body.workspace` field via `detectWorkspace`/`autoDetectWorkspace`.
- Each request is routed to `serverCache.getOrCreate(logicalID)` which returns a `*mcpserver.MCPServer` (mark3labs/mcp-go StreamableHTTPServer).
- Tool calls flow: `mcpserver → FilterEnabled → toolregistry handler → HandleX → WrapSuccess/WrapError`.

#### Tool Definition + Handler Pattern

Each tool follows the `defX` / `HandleX` pair pattern:

```go
// Registration: AllDefs returns []ToolDef containing defX outputs
func defCodegraph(workspace string, deps Deps) ToolDef

// Handler: dispatched by the MCP framework when the tool is called
func HandleCodegraphAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult
```

The `action` field in `req.Params.Arguments` selects the sub-action (e.g., `actionIndex`, `actionMap`, `actionNeighbors`, `actionUsage`, `actionSkeletons`, `actionRelated`, `actionSearch`, `actionExplain`, `actionImpact`, `actionShortestPath`, `actionFindCycles`, `actionDeadcode`, `actionMermaid`, `actionMarkdown`, `actionStatus`, `actionFiles`).

#### Tool Registration Wiring (`internal/server/tools.go`)

```go
func RegisterAllTools(ctx context.Context, srv *mcpserver.MCPServer, reg *toolregistry.ToolRegistry, deps tools.Deps, workspace string) error
func wrapIfPooled(name string, handler toolregistry.Handler) toolregistry.Handler
func FilterEnabled(reg *toolregistry.ToolRegistry) func(ctx context.Context, tools []mcp.Tool) []mcp.Tool
```

## 4. State Management & Data Pipeline

### State Architecture

- **Shared KV Store** (`internal/shared/state.go`): concurrent string→string map with per-key `OnChange` callbacks. Used for active workspace tracking and global state across subsystems.
- **Per-Workspace `mcpserver` Cache** (`internal/server/routes.go:serverCache`): LRU-cached `*mcpserver.MCPServer` per logical workspace ID. Idle entries reaped by `StartIdleReaper()`. No MCP session IDs are stored (stateless mode).
- **Tool Enable-State Layering** (`internal/toolstate/toolstate.go`): per-workspace + global enable/disable state resolved via `ResolveToolState(name, workspaceRoot, workspaceCfg, reg)`.
- **Async Job Registry** (`internal/pooling/registry.go`): in-memory map of pool_id → Job with TTL + grace eviction. `Global()` singleton.

### Storage Mechanics

| Subsystem | Persistence | Engine |
|---|---|---|
| codegraph | `modernc.org/sqlite` | SQLite + FTS5. Single `Storage` struct with pre-compiled prepared statements. Schema: files, nodes, edges, FTS5 virtual table, metadata key/value. |
| projectmemory | `modernc.org/sqlite` | SQLite per-memory DB. Schema: active_memory, virtual_context, project_events, recent_commands. |
| telemetry | `modernc.org/sqlite` | SQLite. Tool-call log table with tool, action, success, error, duration_ms. |
| config | YAML/JSON files | `fsnotify`-watched merged config. Nested merge with user overrides. |
| projectmemory timeline | JSONL | Append-only line-delimited JSON. Schema v1→v3 migration on read. |
| whiteboard | HTTP REST | Remote whiteboard card service. |

### Data Flow Diagram

```
MCP client
  │
  ▼
HTTP POST /mcp (JSON-RPC)
  │
  ▼
server.routes.postMCP
  │   detectWorkspace (X-Workspace header OR body.workspace)
  │   autoDetectWorkspace (body.workspace → header → shared.Store)
  │
  ▼
serverCache.getOrCreate(logicalID)  ← LRU eviction + idle reaper
  │
  ▼
mcpserver (mark3labs/mcp-go StreamableHTTPServer)
  │   FilterEnabled → skip disabled tools
  │
  ▼
toolregistry handler
  │   (possibly wrapped by pooling.Wrap for async jobs)
  │
  ▼
HandleX(ctx, workspace, deps, req)
  │
  ├─▶ gatekeeper.ValidatePathSafety / ValidateCommandPayload  (shell/run tools)
  ├─▶ shell.exec.Run / RunSandbox
  ├─▶ codegraph engine (Index / Search / Related / ...)
  │     └─▶ codegraph.storage (SQLite + FTS5 prepared statements)
  ├─▶ projectmemory (timeline AppendEvent / ftsindex SearchIndexedMemory)
  ├─▶ pooling.Registry (async job register/complete/long-poll)
  ├─▶ bridge.CallBridge (Firefox browser automation)
  ├─▶ whiteboard.Client (card service REST)
  └─▶ tokenoptimizer.OptimizeOutput (shell output compaction)
  │
  ▼
toolresponse.WrapSuccess / WrapError
  │   RenderOutput (command result formatting)
  │   SetTimeoutObserver (telemetry hook)
  │   MarkWithOrphanFlag (abandoned request suppression)
  │
  ▼
HTTP JSON response → MCP client
```

## 5. Execution, Security & Performance Boundaries

### Gatekeeping & Safety

- **Path Safety**: `gatekeeper.ValidatePathSafety(path, operationName)` checks path against allow-lists, dangerous roots, and recursively restricted roots. Returns error if path is outside allowed scope.
- **Command Safety**: `gatekeeper.ValidateCommandPayload(command, execDir)` inspects shell commands before execution.
- **Interactive Confirmations**: `gatekeeper.RequestUserConfirmation(description, targetPath)` returns a `Decision` with `PendingID`. Caller must `AcceptConfirmation(id)` or `RejectConfirmation(id, suggestion)`. Pending confirmations exposed via `GetPendingConfirmations()`.
- **Allowed Paths Management**: `LoadAllowedPaths`, `SaveAllowedPaths`, `AddAllowedPath`, `ClearAllowedPathsCache`.

### Token & Resource Optimization

- **Output Compaction** (`internal/shell/tokenoptimizer/tokenoptimizer.go`):
  - `OptimizeOutput(command, output, options, cfg)`: command-specific compactors for `git status`, `git diff`, `git log`, `ls`, `grep`, `cat`, test output.
  - `ApplyBlacklist(command, output, blacklist)`: regex-based sensitive-pattern suppression.
  - `ApplyTokenProfiles(command, stdout, stderr, options, cfg)`: per-command token budget profiles.
  - `redirectToFile`: spills huge output to disk instead of context.
- **Background Process Registry** (`internal/shell/processes/processes.go`):
  - `Register(cmd *exec.Cmd)` / `Unregister(cmd *exec.Cmd)` / `AbortAll()` — global child-process tracking for SIGTERM cleanup.
- **Async Job Pooling** (`internal/pooling/registry.go`):
  - `Register`, `Complete`, `Cancel`, `LongPoll` with TTL + grace eviction.
  - `Global()` singleton with live config toggle (enabled/elapsedMs knobs re-read per call).
  - Double-invocation hazard guarded: a wrapped poll will not spawn a second job for the same pool_id.

## 6. Architectural Decisions & Constraints

### Key Decisions (ADR style)

1. **Stateless HTTP transport (no MCP session IDs)**
   - Rationale: Eliminates session-state overhead and simplifies server-cache logic. Each request resolves workspace independently via headers/body, then dispatches to a cached `mcpserver`.
   - Trade-off: No persistent MCP session; per-workspace cache must handle idle eviction.

2. **Single-language Go (zero TS runtime dependency)**
   - Rationale: Eliminates Node.js runtime, simplifies deployment, improves throughput for I/O-bound tool execution.
   - Trade-off: All TS-side logic must be reimplemented in Go (already complete).

3. **tree-sitter + SQLite+FTS5 for codegraph**
   - Rationale: tree-sitter provides accurate incremental parsing across 9 languages; SQLite+FTS5 gives fast symbol search without an external search server.
   - Trade-off: Pre-compiled prepared statements add startup cost; incremental re-indexing complexity.

4. **Async job pooling behind handler wrapper**
   - Rationale: Long-running tool calls (e.g., codegraph indexing) return immediately with a `pool_id`; client long-polls for completion. Keeps HTTP request/response cycle short.
   - Trade-off: Added complexity in `pooling.Registry` and `pooling/wrap.go`; must guard against double-invocation on poll re-entry.

5. **Layered codegraph sessions (root + sub-graph DBs)**
   - Rationale: Monorepos with nested `.zenmcp` directories get separate SQLite sub-graphs. `discoverGraphRoots` finds root + sub-graphs; `isolate` selects layer.
   - Trade-off: `sessionStale` triggers parallel re-index across all layers on demand.

6. **Command-output token optimization as first-class concern**
   - Rationale: LLM context windows are limited. `tokenoptimizer` command-specific compactors + blacklist + redirect-to-file keeps tool output within budget.
   - Trade-off: Requires per-command maintenance in `tokenoptimizer.go`; adds latency for output transformation.

7. **Gatekeeper as centralized safety layer**
   - Rationale: All path-sensitive and command-sensitive tools (`shell`, `run`) pass through `gatekeeper.ValidatePathSafety` and `ValidateCommandPayload` before execution. Interactive ops use `RequestUserConfirmation` round-trip.
   - Trade-off: Extra latency for safety checks; interactive flows require a client capable of presenting confirmations.

8. **Terminal REPL reuses MCP tool handlers**
   - Rationale: `commander.ExecuteTool(name, args)` builds a `MakeFakeRequest` and calls the same `HandleX` functions the HTTP server uses. CLI and server state are guaranteed identical.
   - Trade-off: REPL is tightly coupled to MCP tool internals.

### Non-Functional Requirements & Exclusions

- **Performance targets**: 12 HTTP routes; per-workspace LRU cache with configurable TTL; token-optimizer profiles reduce output by command category.
- **Stateless guarantee**: No MCP session IDs stored. All state is in-memory or on-disk SQLite/JSONL. Server restart loses in-flight jobs (pooling registry is ephemeral).
- **Concurrency**: `sync.RWMutex` on shared store, server cache, and codegraph storage. Prepared statements pre-compiled once per `Storage` instance.
- **Exclusions**:
  - No persistent MCP session state (by design).
  - No cross-workspace tool isolation (tools operate within the resolved workspace boundary).
  - No built-in authentication or TLS (delegated to reverse proxy or environment).
  - No TS runtime; original TS implementation is fully replaced.
