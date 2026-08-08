# DEAD_CODE_REPORT.md

Generated: 2026-08-08
Workdir: /media/jang/home/Deve/zen-mcp

## Summary

| Remediation | Count |
|:---|:---|
| Remove (production) | 22 functions/methods |
| Remove (test-only) | 12 functions/methods |
| Ignore (false positive) | 8 functions/methods |
| Files deleted | 4 files (2 production, 2 test) |

## Actually Dead — Removed from Production

| File | Symbol | Rationale |
|:---|:---|:---|
| `internal/codegraph/storage.go` | `DeleteImportsForFile` | No callers |
| `internal/codegraph/storage.go` | `InsertNodes` | Only called in `engine_test.go` (test-only) |
| `internal/codegraph/storage.go` | `DeleteNodesForFile` | Only called in `engine_bench_test.go` (test-only) |
| `internal/codegraph/storage.go` | `DeleteEdgesForFile` | Only called in `engine_bench_test.go` (test-only) |
| `internal/codegraph/storage.go` | `SetMetadata` | No callers; metadata table unused |
| `internal/codegraph/storage.go` | `GetMetadata` | No callers; metadata table unused |
| `internal/codegraph/storage.go` | `FindNodeByName` | No callers; unused query path |
| `internal/codegraph/storage.go` | `FindNodeByTypeAndName` | No callers; unused query path |
| `internal/codegraph/storage.go` | `GetAllEdges` | No callers; unused query path |
| `internal/codegraph/storage.go` | `GetAllEdgeRecords` | No callers; unused query path |
| `internal/codegraph/storage.go` | `SearchSymbols` | No callers; superseded by FTS search |
| `internal/codegraph/scanner.go` | `GetFileDetails` | No callers; superseded by inline logic in `Scan()` |
| `internal/codegraph/scanner.go` | `ResolveAlias` | No callers; TS alias resolution incomplete |
| `internal/gatekeeper/gatekeeper.go` | `ClearAllowedPathsCache` | No callers; cache invalidation not needed |
| `internal/gatekeeper/gatekeeper.go` | `ValidatePathSafetySync` | Only in `gatekeeper_test.go` |
| `internal/mcpcfg/config.go` | `DaemonURL` | Only in `config_test.go`; daemon mode not implemented |
| `internal/mcpcfg/config.go` | `ProxyURL` | No callers; proxy mode not implemented |
| `internal/mcpcfg/config.go` | `LoadWikiConfig` | Only in `config_test.go`; wiki feature not implemented |
| `internal/projectmemory/ftsindex.go` | `ClearAllDatabaseCache` | No callers; cache clear not implemented |
| `internal/projectmemory/ftsindex.go` | `ClearDatabase` | No callers; database drop not implemented |
| `internal/prompts/resolver.go` | `DebugLog` | No callers; debug logging not implemented |
| `internal/prompts/resolver.go` | `WriteDebugLog` | No callers; debug logging not implemented |
| `internal/server/pool.go` | All exported functions | Only in `pool_test.go`; replaced by `serverCache` in `routes.go` |
| `internal/server/patch.go` | `WrapHandlerWithTimeout` | Only in `patch_test.go`; timeout wrapping not applied in production |
| `internal/server/patch.go` | `SummarizeParams` | Only used by `WrapHandlerWithTimeout` |
| `internal/shared/state.go` | `OnChange` | Only in `state_test.go`; pub/sub not used in production |
| `internal/skills/reference_resolver.go` | `ScanBundledResources` | No callers; feature not wired to skill loader |
| `internal/toolresponse/response.go` | `IsCommandResult` | Only in `response_test.go`; not used in production |
| `internal/toolsuggestions/suggestions.go` | `GetAllToolNames` | Only in `suggestions_test.go` |
| `internal/toolsuggestions/suggestions.go` | `FormatToolReference` | Only in `suggestions_test.go` |
| `internal/toolsuggestions/suggestions.go` | `FindMistakeCorrection` | Only in `suggestions_test.go` |
| `internal/toolsuggestions/suggestions.go` | `SemanticPlaceholder` | Only in `suggestions_test.go` |
| `internal/toolsuggestions/suggestions.go` | `MustJSON` | Only in `suggestions_test.go` |
| `internal/toolsuggestions/suggestions.go` | `MistakeCorrection` | Only used by removed `FindMistakeCorrection` |
| `internal/whiteboard/client.go` | `strconvAtoi` | No callers; redundant wrapper around `strconv.Atoi` |
| `internal/projectmemory/timeline.go` | `NormalizeKey` | Only in `projectmemory_test.go` |

## False Positives — Kept

| File | Symbol | Actual Status |
|:---|:---|:---|
| `internal/codegraph/storage.go` | `InsertEdges` | Called by `engine.go:210`, `engine.go:378` |
| `internal/codegraph/storage.go` | `GetAllFiles` | Called by `engine.go:270`, `scanner.go:65` |
| `internal/codegraph/storage.go` | `RunInTransaction` | Used by `ReindexFileData` |
| `internal/toolsuggestions/suggestions.go` | `regexRule` | Used in same-file `ActionRules` map |
| `internal/toolsuggestions/suggestions.go` | `withHelp` | Used in same-file `ActionRules` map |
| `internal/toolsuggestions/suggestions.go` | `withRequired` | Used in same-file `ActionRules` map |
| `internal/toolresponse/response.go` | `SetToolSchema` | Called by `server/tools.go:56` |
| `internal/toolresponse/response.go` | `SetVirtualizer` | Called by `main.go:91` |

## Orphan Files

| File | Verdict |
|:---|:---|
| `go.mod` | False positive — manifest file, not dead code |

## Files Deleted

| File | Reason |
|:---|:---|
| `internal/server/pool.go` | All exported functions dead; superseded by `serverCache` in `routes.go` |
| `internal/server/pool_test.go` | Tests exclusively for removed pool functions |
| `internal/server/patch.go` | `WrapHandlerWithTimeout` and `SummarizeParams` dead; timeout wrapping not applied |
| `internal/server/patch_test.go` | Tests exclusively for removed patch functions |

## Test Files Updated

| File | Change |
|:---|:---|
| `internal/mcpcfg/config_test.go` | Removed `TestLoadWikiConfig` and `DaemonURL` assertions |
| `internal/gatekeeper/gatekeeper_test.go` | Removed `TestValidatePathSafetySync*` and sync call from `TestGatekeeperDisabled` |
| `internal/codegraph/engine_bench_test.go` | Removed `BenchmarkIndexDBWrite` (used dead delete functions) |
| `internal/toolresponse/response_test.go` | Removed `TestIsCommandResult` |
| `internal/shared/state_test.go` | Removed `TestStoreOnChangeOtherKey` and `OnChange` usage from `TestStoreBasics` |
| `internal/toolsuggestions/suggestions_test.go` | Removed tests for 5 dead functions |
| `internal/projectmemory/projectmemory_test.go` | Removed `TestNormalizeKey` |

## Verification

- Build: `go build -tags fts5 ./...` — PASS
- Tests: `go test -tags fts5 ./...` — PASS (all packages)
