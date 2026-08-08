# ARCHITECTURE PLAN — Dead Code Review & Cleanup Pipeline

## Summary

This plan defines a repeatable pipeline for auditing the zen-mcp codebase using the codegraph `deadcode` action, cross-referencing its output against actual call sites, categorizing each dead symbol as **Wire Up** (incomplete feature needing completion) or **Remove** (dead code with no future use), and producing a machine-readable report. The pipeline is designed to be runnable by a separate agent without memory of this session, with every component tied to a concrete file path, function signature, and acceptance criterion.

## System Boundaries

```
INPUT: codegraph deadcode output (text format)
  ↓
[DeadcodeHarness] — runs deadcode action, parses output
  ↓
[CallSiteVerifier] — cross-references each symbol against grep/codegraph search
  ↓
[Categorizer] — classifies each symbol as WIRE_UP, REMOVE, FALSE_POSITIVE, TEST_ONLY_DEAD
  ↓
[ReportWriter] — emits DEAD_CODE_REPORT.md with per-symbol verdicts and rationale
OUTPUT: DEAD_CODE_REPORT.md
```

## Component Breakdown

### 1. DeadcodeHarness

**File:** `scripts/deadcode_harness.go` (new)

Runs the codegraph `deadcode` action and emits structured JSON.

```go
// scripts/deadcode_harness.go
package main

import (
    "encoding/json"
    "os"
    "zen-mcp/scripts/codegraphclient"
)

type DeadcodeReport struct {
    TotalDeadSymbols int      `json:"totalDeadSymbols"`
    OrphanFiles      []string `json:"orphanFiles"`
    DeadSymbols      []DeadSymbol `json:"deadSymbols"`
}

type DeadSymbol struct {
    File    string `json:"file"`
    Name    string `json:"name"`
    Kind    string `json:"kind"` // "function" | "method"
    Line    int    `json:"line"`
    Package string `json:"package"`
}
```

```go
// Run() runs deadcode and returns *DeadcodeReport
func Run(workdir string) (*DeadcodeReport, error)
```

**Depends on:** codegraphclient (wrapper around zen-mcp codegraph tool)
**Acceptance criteria:** `go run scripts/deadcode_harness.go` outputs JSON with all 45 dead symbols and 11 orphan files from the current index.

---

### 2. CallSiteVerifier

**File:** `scripts/callsite_verifier.go` (new)

For each dead symbol, searches the entire codebase for callers, including test files, closures, interface methods, and side-effect imports.

```go
// scripts/callsite_verifier.go
package main

type Verdict string

const (
    VerdictActuallyDead     Verdict = "ACTUALLY_DEAD"      // no production or test callers
    VerdictTestOnlyDead     Verdict = "TEST_ONLY_DEAD"     // only in _test.go files
    VerdictFalsePositive    Verdict = "FALSE_POSITIVE"     // has production callers
    VerdictSideEffectImport Verdict = "SIDE_EFFECT_IMPORT"  // registered via init()
)

type VerifiedSymbol struct {
    File        string   `json:"file"`
    Name        string   `json:"name"`
    Kind        string   `json:"kind"`
    Line        int      `json:"line"`
    Verdict     Verdict  `json:"verdict"`
    Callers     []string `json:"callers"`     // file:line of each caller
    Rationale   string   `json:"rationale"`
}

// VerifyAll takes a DeadcodeReport and returns []VerifiedSymbol
func VerifyAll(report *DeadcodeReport, workdir string) ([]VerifiedSymbol, error)

// searchCallers finds all call sites for a given symbol in the codebase
func searchCallers(symbolName, workdir string) ([]string, error)

// isSideEffectImport checks if a file is loaded via _ "package" import
func isSideEffectImport(filePath, workdir string) (bool, error)
```

**Depends on:** DeadcodeHarness output
**Acceptance criteria:** Running `VerifyAll` against the current codebase correctly classifies:
- `InsertNodes` (storage.go:330) as `FALSE_POSITIVE` (called by engine.go:122)
- `brainExtract` (memory.go:22) as `SIDE_EFFECT_IMPORT` (registered in init())
- `AcquireServer` (pool.go:141) as `TEST_ONLY_DEAD` (only in pool_test.go)
- `SetMetadata` (storage.go:608) as `ACTUALLY_DEAD` (no callers)

---

### 3. Categorizer

**File:** `scripts/categorizer.go` (new)

Maps each verified symbol to a remediation action.

```go
// scripts/categorizer.go
package main

type Remediation string

const (
    RemediationRemove      Remediation = "REMOVE"      // delete the symbol
    RemediationWireUp      Remediation = "WIRE_UP"     // wire into existing system
    RemediationKeepTest    Remediation = "KEEP_TEST"   // test-only, keep for coverage
    RemediationIgnore       Remediation = "IGNORE"       // false positive, no action
)

type CategorizedSymbol struct {
    File          string        `json:"file"`
    Name          string        `json:"name"`
    Line          int           `json:"line"`
    Verdict       Verdict       `json:"verdict"`
    Remediation   Remediation   `json:"remediation"`
    Rationale     string        `json:"rationale"`
    WiredTo       []string      `json:"wiredTo,omitempty"`   // if WIRE_UP, target symbols
}
```

**Decision rules:**
1. `ACTUALLY_DEAD` → `REMOVE`
2. `TEST_ONLY_DEAD` → `REMOVE` (test-only symbols are not part of production binary)
3. `FALSE_POSITIVE` → `IGNORE`
4. `SIDE_EFFECT_IMPORT` → `IGNORE` (file is loaded, only unused internal functions within need review)
5. Within a `SIDE_EFFECT_IMPORT` file, if an exported function has no callers → `REMOVE`

**Depends on:** CallSiteVerifier output
**Acceptance criteria:** All 45 dead symbols and 11 orphan files are assigned exactly one remediation.

---

### 4. ReportWriter

**File:** `scripts/report_writer.go` (new)

Emits `DEAD_CODE_REPORT.md` from categorized symbols.

```go
// scripts/report_writer.go
package main

type Report struct {
    GeneratedAt string              `json:"generatedAt"`
    Workdir     string              `json:"workdir"`
    TotalSymbols int                `json:"totalSymbols"`
    Summary     RemediationSummary  `json:"summary"`
    Symbols     []CategorizedSymbol `json:"symbols"`
    OrphanFiles []OrphanFileVerdict `json:"orphanFiles"`
}

type RemediationSummary struct {
    Remove      int `json:"remove"`
    WireUp      int `json:"wireUp"`
    KeepTest    int `json:"keepTest"`
    Ignore      int `json:"ignore"`
}

type OrphanFileVerdict struct {
    File     string   `json:"file"`
    Verdict  string   `json:"verdict"`
    Evidence []string `json:"evidence"`
}

// WriteReport writes the markdown report to DEAD_CODE_REPORT.md
func WriteReport(report *Report, outputPath string) error
```

**Depends on:** Categorizer output
**Acceptance criteria:** `go run scripts/report_writer.go` generates `DEAD_CODE_REPORT.md` with:
- Per-symbol verdict table
- Orphan file analysis
- Summary counts
- Recommended actions

---

## Data Flow

```mermaid
flowchart TD
    A[codegraph deadcode action] --> B[DeadcodeHarness parses output]
    B --> C[CallSiteVerifier cross-references]
    C --> D{For each symbol}
    D --> E[grep for direct callers]
    D --> F[check init() registration]
    D --> G[check same-package usage]
    E --> H[Assign Verdict]
    F --> H
    G --> H
    H --> I[Categorizer assigns Remediation]
    I --> J[ReportWriter emits DEAD_CODE_REPORT.md]
```

## State Management

The pipeline is stateless between runs. Each run:
1. Reads the current codegraph index (does not re-index)
2. Produces a fresh `DEAD_CODE_REPORT.md`
3. Does not mutate source files

State is held in-memory as `DeadcodeReport` → `[]VerifiedSymbol` → `[]CategorizedSymbol` → `Report`.

## Implementation Blueprint

| Step | File | Action | Signature / Schema | Depends on | Done when |
|------|------|--------|-------------------|------------|-----------|
| 1 | `scripts/codegraphclient/codegraphclient.go` | Create | `func RunDeadcode(workdir string) (string, error)` — invokes `zen-mcp codegraph deadcode` via MCP | — | Returns raw deadcode text |
| 2 | `scripts/deadcode_harness.go` | Create | `func Run(workdir string) (*DeadcodeReport, error)` | Step 1 | Outputs parsed JSON with 45 dead symbols |
| 3 | `scripts/callsite_verifier.go` | Create | `func VerifyAll(report *DeadcodeReport, workdir string) ([]VerifiedSymbol, error)` | Step 2 | Classifies all symbols with correct verdicts |
| 4 | `scripts/categorizer.go` | Create | `func Categorize(verified []VerifiedSymbol) ([]CategorizedSymbol, error)` | Step 3 | Assigns REMOVE/IGNORE/KEEP_TEST to each symbol |
| 5 | `scripts/report_writer.go` | Create | `func WriteReport(report *Report, outputPath string) error` | Step 4 | Writes DEAD_CODE_REPORT.md with tables and summaries |
| 6 | `DEAD_CODE_REPORT.md` | Create | Markdown document with per-symbol verdicts, orphan file analysis, and recommended actions | Steps 1-5 | Document is generated and reviewed |

## Failure Modes

| Failure Mode | Guard | Location |
|--------------|-------|----------|
| codegraph index stale | Check `lastIndexed` timestamp; warn if >24h old | `scripts/deadcode_harness.go:Run()` |
| deadcode parser misses new symbol format | Fall back to regex extraction from raw text | `scripts/deadcode_harness.go` |
| grep misses callers in generated files | Exclude `vendor/`, `.git/`, `zen-mcp` binary dirs | `scripts/callsite_verifier.go:searchCallers()` |
| side-effect import not detected | Scan `main.go` and `cmd/` for `_ "package"` imports | `scripts/callsite_verifier.go:isSideEffectImport()` |
| false positive on interface implementation | Check `_test.go` files and interface satisfaction | `scripts/callsite_verifier.go` |
| report write fails due to missing dir | Create `scripts/` dir if not exists | `scripts/report_writer.go:WriteReport()` |

## Key Decisions

| Decision | Alternative Considered | Rejection Rationale |
|----------|----------------------|-------------------|
| **Stateless pipeline (no DB)** | Persist results to SQLite for historical tracking | Adds complexity without clear benefit; single-run audit is sufficient |
| **Separate `scripts/` package** | Embed analysis in existing `codegraph` engine | Keeps audit logic isolated from production code; no risk of shipping deadcode analysis in binary |
| **JSON intermediate format** | Pass structs directly between stages | JSON enables inspection, manual override, and future tooling integration |
| **Text-based deadcode parsing** | Request JSON output from codegraph | Current codegraph only emits text; parsing is brittle but avoids modifying the codegraph tool |
| **Test-only dead → REMOVE** | Keep test-only symbols for future use | Test-only symbols bloat test binary; if needed, they can be restored from git |

## Expected Categorization Results (Based on Current Index)

### Actually Dead (Remove)

| File | Symbol | Rationale |
|------|--------|-----------|
| `internal/codegraph/storage.go` | `RunInTransaction` (L593) | No callers; superseded by per-method transaction handling |
| `internal/codegraph/storage.go` | `SetMetadata` (L608) | No callers; metadata table unused |
| `internal/codegraph/storage.go` | `GetMetadata` (L616) | No callers; metadata table unused |
| `internal/codegraph/storage.go` | `GetAllFiles` (L628) | No callers; superseded by incremental scanner |
| `internal/codegraph/storage.go` | `FindNodeByName` (L922) | No callers; unused query path |
| `internal/codegraph/storage.go` | `FindNodeByTypeAndName` (L942) | No callers; unused query path |
| `internal/codegraph/storage.go` | `GetAllEdges` (L967) | No callers; unused query path |
| `internal/codegraph/storage.go` | `GetAllEdgeRecords` (L1004) | No callers; unused query path |
| `internal/codegraph/storage.go` | `SearchSymbols` (L1052) | No callers; superseded by FTS search |
| `internal/codegraph/scanner.go` | `GetFileDetails` (L267) | No callers; superseded by inline logic in `Scan()` |
| `internal/codegraph/scanner.go` | `ResolveAlias` (L347) | No callers; TS alias resolution incomplete |
| `internal/skills/reference_resolver.go` | `ScanBundledResources` (L181) | No callers; feature not wired to skill loader |
| `internal/mcpcfg/config.go` | `DaemonURL` (L283) | No callers; daemon mode not implemented |
| `internal/mcpcfg/config.go` | `ProxyURL` (L288) | No callers; proxy mode not implemented |
| `internal/mcpcfg/config.go` | `LoadWikiConfig` (L318) | Only in tests; wiki feature not implemented |
| `internal/gatekeeper/gatekeeper.go` | `ClearAllowedPathsCache` (L119) | No callers; cache invalidation not needed |
| `internal/projectmemory/ftsindex.go` | `ClearAllDatabaseCache` (L68) | No callers; cache clear not implemented |
| `internal/projectmemory/ftsindex.go` | `ClearDatabase` (L77) | No callers; database drop not implemented |
| `internal/whiteboard/client.go` | `strconvAtoi` (L253) | No callers; redundant wrapper around `strconv.Atoi` |
| `internal/prompts/resolver.go` | `WriteDebugLog` (L102) | No callers; debug logging not implemented |
| `internal/logfilter/logfilter.go` | `Infof` (L104) | No callers; only `Info()` is used |
| `internal/toolresponse/response.go` | `IsCommandResult` (L33) | Only in tests; not used in production |
| `internal/projectmemory/timeline.go` | `NormalizeKey` (L16) | Only in tests; normalization logic inlined elsewhere |
| `internal/server/patch.go` | `WrapHandlerWithTimeout` (L37) | Only in tests; timeout wrapping not applied in production |
| `internal/shared/state.go` | `OnChange` (L54) | Only in tests; pub/sub not used in production |
| `internal/server/pool.go` | All exported functions (L141-L396) | Only in pool_test.go; replaced by simpler `serverCache` in routes.go |

### Test-Only Dead (Remove from production code)

| File | Symbol | Rationale |
|------|--------|-----------|
| `internal/server/pool.go` | `AcquireServer` (L141) | Only in pool_test.go |
| `internal/server/pool.go` | `ReleaseServer` (L202) | Only in pool_test.go |
| `internal/server/pool.go` | `HasInflightCalls` (L247) | Only in pool_test.go |
| `internal/server/pool.go` | `HasAnyInflightCalls` (L257) | Only in pool_test.go |
| `internal/server/pool.go` | `SwapServer` (L271) | Only in pool_test.go |
| `internal/server/pool.go` | `GetCachedServer` (L363) | Only in pool_test.go |
| `internal/server/pool.go` | `ClearServerCache` (L376) | Only in pool_test.go |
| `internal/gatekeeper/gatekeeper.go` | `ValidatePathSafetySync` (L455) | Only in gatekeeper_test.go |
| `internal/mcpcfg/config.go` | `LoadWikiConfig` (L318) | Only in config_test.go |

### False Positives (Ignore)

| File | Symbol | Actual Status |
|------|--------|---------------|
| `internal/codegraph/storage.go` | `InsertNodes` (L330) | Called by engine.go:122 |
| `internal/codegraph/storage.go` | `InsertEdges` (L383) | Called by engine.go:158 |
| `internal/toolresponse/response.go` | `SetToolSchema` (L258) | Called by server/tools.go:56 |
| `internal/terminal/handlers/memory.go` | `brainExtract` (L22) | Registered via init() at L169-L170 |

### Orphan Files (Ignore — All False Positives)

| File | Reason |
|------|--------|
| `internal/codegraph/types.go` | Used by same-package files (storage.go, engine.go) |
| `internal/prompts/substitutions.go` | Used by resolver.go within same package |
| `internal/skills/types.go` | Used by reference_resolver.go within same package |
| `internal/terminal/handlers/*.go` (11 files) | Loaded via side-effect import `_ "zen-mcp/internal/terminal/handlers"` in main.go:22 |

## Red-Team Critique Summary

> *[This section will be populated after running browser.chat red-team review]*

Pending: Run `browser({ action: 'chat', provider: 'claude', message: 'Critique this architecture plan...' })`.

## Open Questions

1. **Should `pool.go` be deleted entirely or kept as a reference implementation?**
   - The pool feature was superseded by `serverCache` in `routes.go`. However, `pool_test.go` contains 350+ lines of test coverage for pool behavior that may represent intended future behavior.
   - **Recommended default:** Delete `pool.go` and `pool_test.go` unless the team intends to reintroduce pooling.

2. **Should `storage.go` dead query methods be removed or kept as public API?**
   - Methods like `GetAllFiles`, `FindNodeByName`, `SearchSymbols` are exported (`Storage` type methods). If external packages depend on them (none found), removal is a breaking change.
   - **Recommended default:** Remove since no callers exist within this repo.

3. **Should `WrapHandlerWithTimeout` be removed or kept as a stub for future timeout implementation?**
   - The `patch.go` file is only 96 lines; removing it simplifies the server package.
   - **Recommended default:** Remove; timeout logic can be reintroduced when needed.

## Appendix: Verification Commands

To independently verify any symbol:

```bash
# Search for direct callers
grep -rn "SymbolName" --include="*.go" .

# Search for method expressions
grep -rn "\.SymbolName(" --include="*.go" .

# Check side-effect imports
grep -rn '_ "package-path"' --include="*.go" .

# Run deadcode via codegraph
# (via MCP tool: zen-mcp_codegraph action=deadcode)
```
