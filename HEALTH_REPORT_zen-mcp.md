# HEALTH REPORT — zen-mcp

## Executive Summary

zen-mcp is a well-structured Go project with ~100 files, strong architectural layering, and good test coverage across its heavy components. No file simultaneously meets the hotspot + heavy + complex + untested threshold that would warrant a P0. The highest-leverage risk is a cluster of P1 cross-file magic strings used as implicit contracts (e.g. `"workspace-root"` store key, `"stdio"` transport mode, `"node_modules"` exclusion) where a typo or unilateral rename would fail silently or behave inconsistently. Dead-code volume is moderate (~6 confirmed-dead symbols after reconciling false positives), but the deadcode indexer has a high false-positive rate for exported methods and register/unregister pairs. The single highest-leverage next action is to extract the duplicated string literals into a shared `internal/constants` package and add a `Store` key-type wrapper to eliminate the bare-string store-key risk.

## Metrics Snapshot

| File | References | Lines | Symbols | Category (Hotspot/Heavy/Complex/None) |
|---|---|---|---|---|
| internal/terminal/commander.go | 381 | 548 | 31 | Hotspot |
| internal/logfilter/logfilter.go | 232 | 169 | 17 | Hotspot |
| internal/tools/capture.go | 213 | 299 | 16 | Hotspot |
| internal/codegraph/engine.go | 207 | 1314 | 59 | Hotspot/Heavy/Complex |
| internal/toolresponse/response.go | 166 | 460 | 20 | Hotspot |
| internal/pooling/registry.go | 147 | 293 | 16 | Hotspot |
| internal/codegraph/storage.go | 130 | 1485 | 74 | Hotspot/Heavy/Complex |
| internal/shell/tokenoptimizer/tokenoptimizer.go | 112 | 1341 | 56 | Hotspot/Heavy/Complex |
| internal/shared/state.go | 88 | 79 | 6 | Hotspot |
| internal/shell/processes/processes.go | 76 | 59 | 5 | Hotspot |
| internal/terminal/exportcli.go | — | 721 | 32 | Heavy/Complex |
| internal/tools/codegraph.go | — | 1472 | 50 | Heavy/Complex |
| internal/tools/think.go | — | 549 | 37 | Heavy/Complex |
| internal/gatekeeper/gatekeeper.go | — | 540 | 31 | Heavy/Complex |
| internal/terminal/commander.go | — | 548 | 31 | Heavy/Complex |
| internal/server/livegraph.go | — | — | 31 | Complex |
| internal/mcpcfg/config.go | — | 515 | 54 | Heavy/Complex |

## Dead Code

| Symbol/File | Location (file:line) | Status (Confirmed-Dead/Ambiguous) | Rationale | What Would Confirm It |
|---|---|---|---|---|
| internal/gatekeeper/gatekeeper.go: ClearAllowedPathsCache | gatekeeper.go:119 | Confirmed-Dead | Zero external references; mirrors internal cache-nil pattern | Verify no terminal/CLI reset hook calls it |
| internal/prompts/resolver.go: WriteDebugLog | resolver.go:106 | Confirmed-Dead | Zero external references; sibling DebugLog is a no-op stub | Verify no test or plugin references it |
| internal/shared/state.go: eqFunc | state.go:54 | Confirmed-Dead | Unexported; no caller in Store methods or elsewhere | Verify no Subscribe/Off pattern exists in another file |
| internal/whiteboard/client.go: strconvAtoi | client.go:253 | Confirmed-Dead | Trivial wrapper with identical signature to stdlib; zero callers | Verify no external consumer uses it |
| internal/tools/browser.go: postJSON | browser.go:437 | Confirmed-Dead | Zero external references; bridge.go does its own inline POST | Verify no string-based invocation or reflection |
| internal/terminal/commander.go: restoreTerminal | commander.go:449 | Confirmed-Dead | Zero external references; setRawMode caller uses defer inline | Verify no test or main.go path calls it |
| internal/codegraph/scanner.go: GetFileDetails | scanner.go:292 | Ambiguous | Exported method on Scanner; zero internal callers, but public API surface | Verify no external package imports Scanner for this method |
| internal/codegraph/scanner.go: ResolveAlias | scanner.go:372 | Ambiguous | Exported method; paired with LoadTsConfigAliases which is called from NewScanner | Verify no external TS-alias consumer calls it |
| internal/projectmemory/ftsindex.go: ClearAllDatabaseCache | ftsindex.go:68 | Ambiguous | Exported; likely test/ops helper given dbCache reset pattern | Grep all `_test.go` files for explicit calls |
| internal/projectmemory/ftsindex.go: ClearDatabase | ftsindex.go:77 | Ambiguous | Exported; paired with ClearAllDatabaseCache | Grep all `_test.go` files for explicit calls |
| internal/server/pool.go: unregisterServerCache | pool.go:31 | Ambiguous | Part of register/unregister pair; comment notes legacy pool removal in progress | Verify serverCache.Close/shutdown path in server/cache.go or equivalent |
| internal/skills/reference_resolver.go: ScanBundledResources | reference_resolver.go:181 | Ambiguous | Exported; sibling ScanKnowledgeBase is wired into ResolveSkillContent | Verify no terminal handler or tool wraps this in a closure |
| internal/toolsuggestions/suggestions.go: SemanticPlaceholder | suggestions.go:286 | Ambiguous | Exported helper; may be newer unwired-in replacement for example generation | Check git blame/recency to decide delete vs. finish-integrating |
| internal/toolsuggestions/suggestions.go: MustJSON | suggestions.go:333 | Ambiguous | Exported helper; same caveat as SemanticPlaceholder | Check git blame/recency |

## Coupling / Spaghetti Findings

| File | Fan-in | Fan-out | Evidence (cycle/dense-cross-ref) | Why It Matters |
|---|---|---|---|---|
| internal/codegraph/engine.go | 207 refs | ~20 callees | 207 references; 20+ callers (tools/codegraph.go, watcher.go, commander.go, exportcli.go, 9 language plugins); no cycles detected | God-object: every codegraph action funnels through here; any signature change ripples across 20+ callers |
| internal/toolresponse/response.go | 166 refs | — | 166 references; central WrapSuccess/WrapError used by every HandleX function | Single point of failure for all tool output; changes affect every tool |
| internal/logfilter/logfilter.go | 232 refs | — | 232 references; global leveled-logging accessed from every package | Global-state coupling; security-bypass filter changes affect all subsystems |
| internal/mcpcfg/config.go | — | — | Config singleton accessed imperatively from gatekeeper.go (4+ sites), wrap.go, resolver.go | Global-config coupling with no interface boundary; hard to test or swap |

## Boilerplate / Duplication Findings

| Pattern | Locations | Skeleton Evidence | Suggested Collapse |
|---|---|---|---|
| init() + terminal.Register() registration | 12/14 terminal/handlers/*.go | Every handler file exports exactly 1 `init()` that calls `terminal.Register(name, handler)` | Extract `RegisterHandler(name, handler)` macro or use a declarative registry map |
| defX() + HandleXAction() tool pair | 16/20 internal/tools/*.go | Every tool file exports exactly 1 `defX(workspace, deps) ToolDef` and 1 `HandleXAction(ctx, workspace, deps, req)` | Consider a `ToolTemplate` or codegen to reduce 40-line boilerplate per tool |
| Duplicate containsStr helper | pooling/wrap.go:34, gatekeeper/gatekeeper.go:533 | Identical private `containsStr(list []string, s string) bool` bodies | Replace with `slices.Contains` (Go 1.21+) or a shared internal helper |
| Duplicate frontmatter parser | prompts/loader.go:195-234, skills/reference_resolver.go:19-53 | Near-identical hand-rolled YAML-frontmatter scanner with `"---"` delimiter + `currentKey == "triggers"` branching | Consolidate into one shared parser (skills/reference_resolver.go version is more complete) |
| Action-name vocabulary duplication | tools/browser.go (dispatch switch), toolsuggestions/suggestions.go (regex rules + help text), bridge.go (wire field) | Same action strings ("navigate", "click", "chat", "brainstorm", "web_eval", etc.) hardcoded 3 times | Extract into a shared `browser/actions` constants package |

## Magic Strings / Repeated Literals

| Literal | Occurrences | Locations (file:line) | Used In Branching? (Y/N) | Suggested Constant Name | Risk (typo-mismatch/inconsistent-update) |
|---|---|---|---|---|---|
| triggers | 4+ | prompts/loader.go:225,228; skills/reference_resolver.go:41,44,45,47,115-122,131 | Y | FrontmatterKeyTriggers | P1 — undeclared shared contract between two independent parsers |
| stdio | 2 | server/shutdown.go:18; gatekeeper/gatekeeper.go:288 | Y | TransportStdio | P1 — implicit cross-package contract: env var must equal caller-passed mode string |
| pool | 2 | server/tools.go:107; pooling/wrap.go:34 | Y | ToolNamePool | P1 — rename risk: if tool name changes, both guards must update or pool self-wraps |
| node_modules | 2 | tools/codegraph.go:48; codegraph/engine.go:1270 | Y | DirNameNodeModules | P1 — directory exclusion duplicated; rename risk in one file only |
| --force | 2 | terminal/handlers/codegraph.go:16; terminal/handlers/workspace.go:31 | Y | FlagForce | P1 — CLI flag duplicated; also tolerates typo `---force` in codegraph.go |
| workspace-root | 12 | main.go:92; tools/run.go:82; tools/workspace.go:43,53; server/routes.go:369,383; server/livegraph.go:47; workspace/resolve.go:25; terminal/handlers/workspace.go:60,69; terminal/commander.go:109; gatekeeper/gatekeeper.go:67,101 | Y | StoreKeyWorkspaceRoot | P1 — bare-string store key with no compile-time safety; typo fails silently |
| .zenmcp | 2+ | tools/codegraph.go:57; tools/watcher.go:189 | Y | DirNameZenMcp | P1 — hardcoded path segment in branching logic |
| --- | 4 | prompts/loader.go:203,208; skills/reference_resolver.go:27,32 | N | FrontmatterDelimiter | P2 — frontmatter delimiter; merged with "triggers" finding as same root cause |
| action | 2+ | bridge/bridge.go:41,96; tools/browser.go:40,64,71,102,115,124, etc. | Y | BrowserActionKey | P1 — wire-protocol field name duplicated across bridge, browser dispatch, and suggestions validation |

## Prioritized Risk List

| Priority (P0/P1/P2) | File/Pattern | Score Reason (metric intersection) | Test Coverage (Covered/Untested/Partial) | Suggested Route (architect/maintainer/human-judgment) |
|---|---|---|---|---|
| P1 | Magic string: "triggers" (loader.go + reference_resolver.go) | Cross-file literal duplication in branching logic (4+ occurrences, 2 files) | Covered | maintainer — extract constant + consolidate parsers |
| P1 | Magic string: "stdio" (shutdown.go + gatekeeper.go) | Cross-file literal in branching logic (2 files, implicit contract) | Covered | architect — introduce TransportMode enum or shared constant |
| P1 | Magic string: "pool" (tools.go + wrap.go) | Cross-file literal in branching logic (2 files, correctness risk) | Covered | maintainer — extract ToolNamePool constant |
| P1 | Magic string: "node_modules" (tools/codegraph.go + engine.go) | Cross-file literal in branching logic (2 files) | Covered | maintainer — extract DirNameNodeModules constant |
| P1 | Magic string: "--force" (handlers/codegraph.go + handlers/workspace.go) | Cross-file literal in branching logic (2 files) | Covered | maintainer — extract FlagForce constant; investigate `---force` typo |
| P1 | Magic string: "workspace-root" (8+ files) | Cross-file bare-string store key in branching logic | Covered | architect — wrap Store keys in typed constants or dedicated accessors |
| P1 | Magic string: ".zenmcp" (tools/codegraph.go + tools/watcher.go) | Cross-file literal in branching logic (2 files) | Covered | maintainer — extract DirNameZenMcp constant |
| P1 | Magic string: "action" (bridge.go + browser.go + suggestions.go) | Cross-file protocol field in branching logic; action vocabulary duplicated 3x | Covered | architect — centralize browser action names in shared package |
| P2 | Dead code: 6 confirmed-dead symbols | Dead code with no callers; moderate maintenance burden | Partial/Untested | human-judgment — delete or archive after verifying no external consumers |
| P2 | Boilerplate: terminal handler init+Register pattern | 12/14 files repeat identical registration boilerplate | Covered | maintainer — extract declarative registry |
| P2 | Boilerplate: tool defX+HandleXAction pattern | 16/20 files repeat identical tool pair pattern | Covered | maintainer — consider codegen or template |
| P2 | Coupling: engine.go god-object | Hotspot (207 refs) + Heavy (1314 lines) + Complex (59 symbols) | Covered | architect — split into per-action sub-handlers |
| P2 | Coupling: mcpcfg.Get() global singleton | Config accessed imperatively from 4+ packages | Covered | architect — inject config via deps struct |

## Second Opinion Reconciliation

| Finding | Source (mine/sub-agent) | Disposition (Kept/Refined/Added/Rejected) | Note |
|---|---|---|---|
| Confirmed-Dead: scanner.go GetFileDetails | mine | Rejected | Sub-agent: exported public API on Scanner; could be external consumer or reflection target |
| Confirmed-Dead: scanner.go ResolveAlias | mine | Rejected | Sub-agent: exported method paired with LoadTsConfigAliases; should be flagged together |
| Confirmed-Dead: gatekeeper.go ClearAllowedPathsCache | mine | Kept | Sub-agent: kept but notes likely test/ops hook |
| Confirmed-Dead: ftsindex.go ClearAllDatabaseCache, ClearDatabase | mine | Rejected | Sub-agent: likely test helpers referenced only from `_test.go` files |
| Confirmed-Dead: prompts/resolver.go WriteDebugLog | mine | Kept | Sub-agent: agree dead; added paired finding that sibling DebugLog is a no-op stub |
| Confirmed-Dead: server/pool.go unregisterServerCache | mine | Rejected | Sub-agent: part of register/unregister pair; likely called from shutdown path in another file |
| Confirmed-Dead: shared/state.go eqFunc | mine | Kept | Sub-agent: agree dead |
| Confirmed-Dead: skills/reference_resolver.go ScanBundledResources | mine | Rejected | Sub-agent: ambiguous; could be wired through terminal.Register closure |
| Confirmed-Dead: tools/browser.go postJSON | mine | Kept | Direct verification: zero callers; bridge.go does inline POST |
| Confirmed-Dead: toolsuggestions/suggestions.go SemanticPlaceholder, MustJSON | mine | Rejected | Sub-agent: likely unwired-in replacement path, not legacy dead code |
| Confirmed-Dead: whiteboard/client.go strconvAtoi | mine | Kept | Sub-agent: agree dead; trivial stdlib wrapper |
| Confirmed-Dead: terminal/commander.go restoreTerminal | mine | Kept | Direct verification: zero callers; setRawMode uses defer inline |
| P1: "triggers" magic string | mine | Refined | Sub-agent: confirmed P1; recommend consolidating duplicate frontmatter parsers rather than just constant-izing |
| P1: "stdio" magic string | mine | Refined | Sub-agent: recharacterize as "undeclared shared contract" (env var vs. caller param) rather than simple duplication |
| P1: "pool" magic string | mine | Refined | Sub-agent: confirmed P1; correctness risk if tool is renamed |
| P1: "node_modules" magic string | mine | Kept | Direct verification confirmed cross-file branching usage |
| P1: "--force" magic string | mine | Kept | Sub-agent: agree P1; bonus finding: `---force` typo-tolerance in codegraph.go |
| P1: "action" magic string | mine | Added | Sub-agent: action-name vocabulary duplicated across bridge.go, suggestions.go, browser.go as unsynchronized sources of truth |
| Added: "workspace-root" store key | sub-agent | Added | Cross-file bare-string store key in branching logic (12 occurrences, 8+ files) |
| Added: ".zenmcp" directory name | sub-agent | Added | Cross-file hardcoded path segment in branching logic |
| Added: Duplicate containsStr helper | sub-agent | Added | Identical private helper in wrap.go and gatekeeper.go |
| Added: Duplicate frontmatter parser | sub-agent | Added | Near-identical hand-rolled YAML-frontmatter scanner in loader.go and reference_resolver.go |
| Added: mcpcfg.Get() global coupling | sub-agent | Added | Config singleton accessed imperatively from 4+ packages |

## Open Questions

- Are `scanner.go: GetFileDetails` and `ResolveAlias` truly dead, or are they consumed by external tooling outside the indexed graph? The deadcode indexer reports zero callers, but they are exported methods on an internal package type. **Confidence: <70%** — needs grep across all importers or a runtime usage trace.
- Is `skills/reference_resolver.go: ScanBundledResources` wired through a `terminal.Register` closure that the static indexer cannot follow? The codebase uses init-time registration extensively, making static dead-code analysis inherently incomplete for this pattern. **Confidence: <70%**.
- Is `toolsuggestions/suggestions.go: SemanticPlaceholder` an abandoned prototype or a feature mid-integration? Git blame/recency would resolve whether to delete or finish wiring it in. **Confidence: <70%**.
- The `---force` typo-tolerance in `terminal/handlers/codegraph.go:16` appears unintentional (accepts both `--force` and `---force`). Is this dead code or a deliberate backward-compat shim? **Confidence: <85%**.
- Does any external consumer (outside this repo) call `mcpcfg/config.go: DaemonURL`, `ProxyURL`, or `LoadWikiConfig`? These are exported but have zero internal production callers. **Confidence: <70%**.
