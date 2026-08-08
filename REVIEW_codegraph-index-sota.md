# Code Review Plan: `internal/codegraph` Index() vs SOTA (codebase-memory-mcp / Graphify)

> Status: READ-ONLY review + architecture plan. No files modified. Confidence labels: `[VERIFIED]` = confirmed against local source; `[HYPOTHESIS]` = inferred from code paths.

## 1. Summary

This review targets the **Index() pipeline** of the Zen-MCP Go tree-sitter codegraph engine — `internal/codegraph/engine.go:62-182` plus its three collaborators: `scanner.go` (change detection), `parser.go` + 10 `languages_*.go` plugins (AST extraction), and `storage.go` (SQLite/FTS5 persistence). It was benchmarked against the two current open-source SOTA systems: **DeusData/codebase-memory-mcp** (C, tree-sitter + "Hybrid LSP" second pass, RAM-first SQLite pipeline, published Linux-kernel 3-min index benchmark) and **Graphify-Labs/graphify** (Python, tree-sitter AST + per-edge `EXTRACTED/INFERRED/AMBIGUOUS` confidence tags, git-mergeable graph artifact).

**Headline verdict:** the engine is structurally sound (tree-sitter is the right core, SQLite/FTS5 is the right store, schema is clean) but the Index() implementation is not optimized and has three correctness defects that will silently corrupt or hollow out the graph over time:

1. **Edge resolution ignores file scope** → wrong cross-file edges + Cartesian-product edge explosion for common symbol names (C1).
2. **`FindShortestPath` is permanently broken** — queries a non-existent `nodes.path` column and swallows the error (C2).
3. **Incremental reindex cascade-deletes incoming edges from unchanged files and never rebuilds them** → the call graph progressively loses edges (C3).
4. **Incremental indexing is O(total repo size) on every run** — every file is read + SHA-256 hashed + DB-queried on every call (M1/M2/M3), defeating the purpose of incremental mode.

Compared with CBM, the biggest architectural gaps are: no resolved-import-aware edge disambiguation, no confidence-graded edge resolution, no in-memory-first batched pipeline, no git-diff-driven change detection, and no index-integrity verification.

---

## 2. Top Issues (ordered by severity)

### CRITICAL

#### C1. Edge resolution ignores `rel.SourceFile` → wrong cross-file edges + Cartesian explosion `[VERIFIED]`
- **Location:** `engine.go:150-151` (Phase 3), duplicated at `engine.go:237-238` (`parseManifests`).
- **Mechanism:** `rel.SourceFile` is populated (`engine.go:132`) then never read again. `FindNodesByName` (`storage.go:503-522`) matches `WHERE n.name = ? OR n.qualified_name = ?` across **every node in every file**. A call site `Process()` in `pkg/a/handler.go` fans out an edge to **every** function named `Process` in the repo. Source names resolve the same way, so common-name × common-name is a Cartesian product — `O(matches_src × matches_tgt)` edges per relation.
- **Downstream impact:** corrupts `GetNeighbors`, `Impact`, `RelatedFiles`, dead-code, shortest-path; balloons edge table size.
- **Root cause is deeper than the missing filter:** `extractGoRelations` (`languages_go.go:84-89`) strips selector calls to the bare field name (`w.Header().Set(...)` → target `"Set"`), so even file-scoped lookup collides across types.
- **Mitigation plan:**
  1. Minimal: scope target lookup to source file's package/dir first; fall back to global only on zero matches; tag fallback edges `Confidence: "INFERRED"`.
  2. Cap fan-out: if `len(src)×len(tgt) > N`, skip/choose closest candidate instead of materializing the product.
  3. Target: resolve imports per file (already parsed via `import_spec`) and use the import table to disambiguate — this is exactly CBM's "Hybrid LSP" idea at the resolution layer.

#### C2. `FindShortestPath` always fails silently — queries nonexistent `nodes.path` column `[VERIFIED]`
- **Location:** `storage.go:950-971` (`getNodesByName`) called at `storage.go:829-836`; unused twin prepared statement `getNodesByName` at `storage.go:163-167`.
- **Mechanism:** `SELECT id, file_id, type, name, language, path, ... FROM nodes` — the `nodes` table (schema `storage.go:194-206`) has **no `path` column**; only `files` does. Every call errors `no such column: path`, and `FindShortestPath` swallows it (`return &ShortestPathResult{Found:false}, nil` at `storage.go:831,835`). **ShortestPath/ShortestPathResult is 100% non-functional with zero diagnostics.**
- **Mitigation:** join to `files` (like every other query in the file) or reuse the correct `FindNodesByName`. After fixing C2, apply M7 (BFS N+1) before relying on it at scale.

#### C3. Incremental reindex silently drops edges from unchanged files that reference a changed file `[VERIFIED]`
- **Location:** `engine.go:137-144` + FK cascade (`storage.go:196,209-210`, `PRAGMA foreign_keys = ON` at `storage.go:29`).
- **Mechanism:** `DeleteNodesForFile(fileID)` cascades `ON DELETE` on `edges.source_id`/`target_id` → deletes **every** edge touching the changed file's nodes from either side, including `unchanged B.caller --calls--> A.Process`. `DeleteEdgesForFile` (same file) then deletes edges matching source **or** target — identical directional mistake. Phase 3 (`engine.go:147-175`) rebuilds edges **only for files in this run's `parseResults`**. File B is unchanged → never re-parsed → its edge into A is gone forever until B itself is touched.
- **Impact:** compounding correctness bug — the graph hollows out over the life of a long-running index, undetectable without a full-reindex diff.
- **Mitigation:** (a) don't destructively delete nodes — upsert by qualified name and only delete edges whose **source** file is the changed file; or (b) add changed-file's "affected neighbors" (files whose relations reference its symbols) to the incremental set; or (c) track and re-run relation resolution for cross-file referrers.

#### C4. Deleted files are never cleaned up; `IndexResult.Deleted` is dead `[VERIFIED]`
- **Location:** `engine.go:56-59` (`IndexResult`), `scanner.go:51-102`; `storage.DeleteFile` exists at `storage.go:356-361` but is never called; `actionIndex` discards the whole result at `tools/codegraph.go:294`.
- **Mechanism:** `GetFilesToProcess` iterates only **disk** files; it never diffs stored paths against disk. A renamed/deleted file leaves its `FileRecord`, nodes, and edges in the DB forever → dead-code/search/map surface ghost symbols. `IndexResult.Deleted` is never assigned. The MCP caller surfaces only `"%d total files"`, so `Indexed/Total/Deleted` are invisible to the agent.
- **Mitigation:** after the disk walk, diff against `storage.GetAllFiles()`; `storage.DeleteFile(id)` for missing paths; set `result.Deleted`. Surface the full `IndexResult` in `actionIndex`.

### MAJOR

#### M1. `GetFilesToProcess` reads + SHA-256-hashes every file on every call regardless of mtime `[VERIFIED]`
- **Location:** `scanner.go:72-90`.
- **Mechanism:** `os.Stat` is cheap, but the full `os.ReadFile` + `sha256.Sum256` (lines 80-84) runs unconditionally **before** the mtime/hash comparison (line 89-90). Incremental index I/O == full index I/O even with zero changes.
- **Mitigation:** compare `ModTime()`/size against cached record first; only hash when mtime differs (hash stays as a lie-detector for clock skew/`touch`); optionally reconcile via periodic full-hash pass instead of every run.

#### M2. N+1: one `GetFileByPath` DB query per file per `Index()` `[VERIFIED]`
- **Location:** `scanner.go:89`; 50k-file repo → 50k sequential locked queries before parsing starts.
- **Mitigation:** one `SELECT path, hash, mtime FROM files` into an in-memory map, diff in memory.

#### M3. Double file read + TOCTOU between hash and parse `[VERIFIED]`
- **Location:** `scanner.go:80` (read-for-hash, bytes discarded) + `engine.go:82` (re-read for parse).
- **Mechanism:** 2× disk I/O per changed file; worse, if the file mutates between the two reads, the stored hash no longer matches the content actually parsed → future runs may skip reindexing on a hash/mtime match even though parsed content diverged from disk.
- **Mitigation:** thread `Content []byte` through `FileRecord` (or sibling struct) from scanner to `Index()` — read once per changed file.

#### M4. Per-file mini-transactions under a global mutex; batching primitive exists but is unused `[VERIFIED]`
- **Location:** `engine.go:137-144`; `Storage.RunInTransaction` (`storage.go:645-657`) — built, never called.
- **Mechanism:** per changed file: `DeleteNodesForFile` + `DeleteEdgesForFile` + `DeleteImportsForFile` + `InsertNodes` = up to 4 WAL commits + 4 mutex acquisitions. ~5k-file change → ~20k tiny transactions. Also: `DeleteEdgesForFile` at `engine.go:139` is **dead work** — cascade delete already removed the edges, so its query matches zero rows.
- **Mitigation:** wrap all of Phase 2 (ideally Phases 2+3) in one `RunInTransaction`; drop the redundant `DeleteEdgesForFile`.

#### M5. Global `storage.mu` + no index-level lock blocks parallelism and allows interleaved concurrent Index() runs `[VERIFIED]`
- **Location:** `storage.go` throughout; phase 1 parse loop at `engine.go:81` is fully sequential.
- **Mechanism:** every storage method takes `s.mu` (reads inconsistently — `FindNodesByName`/`ListFiles`/`SearchFTS` do not). Two concurrent `actionIndex` calls on the same cached per-workspace graph (`tools/codegraph.go getSessionByWorkspace`) can interleave Phase-2 deletes with Phase-3 edge resolution → FK violations silently dropped as orphan edges. tree-sitter `Parser` is not thread-safe and each language plugin holds one shared instance (`basePlugin`, `languages_base.go:18-34`) whose `mu` guards get/set only, not `Parse` — so naive goroutine-per-file would race.
- **Mitigation:** (a) add an index-level lock in `CodeGraph.Index()`; (b) standardize Storage locking (SQLite/WAL already serializes writers; drop the Go mutex or make it consistent); (c) parallelize Phase 1 with a bounded worker pool using **per-worker parser instances** (one per goroutine), then batch-write.

#### M6. `FindShortestPath` BFS does a DB round-trip per edge per hop `[VERIFIED]`
- **Location:** `storage.go:903` (nested `QueryRow SELECT id FROM nodes WHERE name = ? AND file_id = ...` inside the BFS loop).
- **Mechanism:** worst case `O(b^limit)` individual queries; currently masked by C2 (function returns before the loop ever runs).
- **Mitigation:** resolve target IDs via the already-joined edge query (add `e.target_id` to the SELECT at `storage.go:867-873`) — no second lookup needed at all.

#### M7. Manifest parsing is unconditional, un-hashed, and silently failing `[VERIFIED]`
- **Location:** `engine.go:179` + `186-257`.
- **Mechanism:** `parseManifests()` runs on **every** `Index()` even incremental, re-deletes/re-inserts the full manifest node set, re-resolves edges with the C1 Cartesian hazard, and swallows every error (`continue`). Malformed manifest → external-dependency nodes silently vanish.
- **Mitigation:** route manifests through the same hash/mtime change detection; return parse failures via `IndexResult`.

### MINOR / NIT

| ID | Severity | Location | Issue |
|----|----------|----------|-------|
| N1 | Minor | `scanner.go:47-48,76-78` | Hard caps `maxFiles=10000`, `maxFileSize=500KB` silently drop files/repos with no signal; should be configurable and reported in `IndexResult`. |
| N2 | Minor | `scanner.go:305-355` | `ResolveAlias` builds paths from `tsconfig.json` without `filepath.Clean`/root-boundary check (potential traversal). **Downgraded from Major:** `grep` confirms `ResolveAlias` has **zero callers** in the codebase — latent/defense-in-depth today. |
| N3 | Minor | `parser.go` / `scanner.go` | Duplicated hand-maintained extension lists (`plugins` map, `isSupported`, `detectLanguage`) with no single source of truth; extension drift silently skips files. Derive scanner lists from `Parser.GetSupportedExtensions()`. |
| N4 | Minor | `storage.go:636-642` | `sanitizeFtsQuery` only quotes whitespace queries; single-token `-foo`, `content:bar`, `foo*` pass through as raw FTS5 syntax → confusing empty results/errors. Always quote. |
| N5 | Minor | `engine.go:161,246` | `Confidence` hardcoded `"EXTRACTED"` even for ambiguous global-name matches (the least certain edges get the highest confidence tag). Tie into C1 fix; schema already supports arbitrary values. |
| N6 | Nit | `engine.go:102,987-992` | `truncate` is byte-based → can split a multi-byte UTF-8 rune mid-sequence, storing invalid UTF-8 in `content`. Use rune-safe truncation. |
| N7 | Nit | `engine.go:292-361` | Manifest parsers are shallow: go.mod misses `require (...)` blocks, `// indirect`, multi-module lines; Cargo.toml misses inline `{ version = ... }` TOML tables; pom.xml regex is greedy across blocks. |
| N8 | Nit | `storage.go:364-423,645-657,163-167` | Dead/inert code: `InsertNode` (singular), `ClearNodesForFile`, unused prepared `getNodesByName` (same broken-column bug as C2), `RunInTransaction` never wired. |

---

## 3. SOTA Comparison — where Zen-MCP lags / wins

### From codebase-memory-mcp (benchmarked SOTA)
| Capability | CBM | Zen-MCP | Gap |
|---|---|---|---|
| Edge resolution | "Hybrid LSP" 2nd pass + `CALLS`/`CALL_REFERENCE`/`USAGE` split | bare-name global match, hardcoded `EXTRACTED` | **C1** — biggest correctness gap |
| Change detection | git-poll + adaptive intervals, `detect_changes` blast radius | full read+hash every run | **M1/M2** |
| Pipeline | RAM-first in-memory SQLite, single dump at end, dump-verify ratio (`status:degraded`) | per-file transactions, no integrity check | **M4** + missing integrity net |
| Shareable index | zstd-compressed `.graph.db.zst` (fast/best tiers) | `.zenmcp/codegraph.db` local only | team cold-start reindex cost |
| Query | openCypher subset, `semantic_query` (bundled nomic embed + 11-signal scorer), `get_architecture`, `trace_path` | name/FTS + hand-rolled shortest path | feature depth; no semantic layer |
| Scale (published) | Linux kernel 28M LOC / 75K files in 3 min | **unbenchmarked at that scale** (bench fixtures are 1-50 tiny files) | benchmark gap |

### From Graphify
| Idea | Graphify | Worth borrowing for Zen-MCP? |
|---|---|---|
| Per-edge `EXTRACTED/INFERRED/AMBIGUOUS` confidence as first-class data | yes | **Yes** — schema already has the column; ties directly into C1/N5 |
| Git-mergeable graph artifact (`graph.json` + merge driver) | yes | Partially — a compact two-tier shareable artifact (like CBM's) beats raw JSON; JSON is size-capped at 512MiB and won't scale |
| PR-aware impact tooling, lessons-learned loop | yes | Out of scope for the engine; feature-level, not Index() |
| Multi-modal graph (docs/PDF/SQL/Infra) | yes | Different product scope; Zen-MCP's tree-sitter core is deliberately code-only |

### Zen-MCP already does well
- Pure-Go single binary, no external LSP processes, stateless-friendly design.
- tree-sitter via `go-tree-sitter` with cached per-language parsers — correct foundational choice.
- Clean SQLite/FTS5 schema with sensible indexes (`idx_edges_source_relation`, `idx_edges_target_relation`) and FTS triggers.
- 10 language plugins with per-language query coverage — more curated than graphify's breadth, less than CBM's 158 grammars but reasonable for a v2.x tool.

---

## 4. Recommended implementation order

1. **C2** (one-line fix, currently-shipping silently-broken feature) → then **M6** while in the file.
2. **C3** (graph integrity) + **C4** (stale data) — the two "silent divergence" bugs.
3. **C1** (edge correctness) — scoped lookup + confidence tagging + fan-out cap; this is the highest-impact correctness change and aligns with the SOTA pattern.
4. **M4 + M1/M2/M3** (single `RunInTransaction`; mtime-first change detection; single file read threaded through) — the "make incremental actually incremental" bundle.
5. **M5** (index-level lock + per-worker parsers + parallel Phase 1) — scale step, gated on the transaction/locking work above.
6. **M7/N1-N8** — manifest gating, error surfacing via `IndexResult`, extension-list unification, sanitizer, rune-safe truncate, dead-code cleanup.
7. **Later / stretch:** CBM-style shareable compressed artifact + dump-verify integrity check; optional git-signal change detection.

---

## 5. Red-team critique summary (browser.chat / Claude, independent review)

Independent review was run against `engine.go`, `parser.go`, `scanner.go`, `storage.go`, `languages_go.go`, `languages_base.go`, `types.go` (absolute paths uploaded via browser.chat). Points from that pass, disposition for each:

| Red-team point | Severity (theirs) | Disposition |
|---|---|---|
| C1 edge resolution ignores file scope + Cartesian explosion (`engine.go:150-151,237-238`) | Critical | **Folded in** — independently confirmed; expanded with selector-field root cause from `languages_go.go:84-89` |
| C2 `getNodesByName` selects nonexistent `nodes.path` → `ShortestPath` always `Found:false` | Critical | **Folded in** — **independently verified** against `storage.go:950-971` + schema `storage.go:194-206`; this was their find, not in my first pass |
| C3 cascade-delete drops edges from unchanged referrers on incremental index | Critical | **Folded in** — **independently verified** via FK cascade (`storage.go:29,196,209-210`) + Phase-3 rebuild scope (`engine.go:137-175`); strongest new finding |
| C4 deleted files never cleaned / `Deleted` dead | Critical | **Folded in** — matches my first-pass finding #5; added that `actionIndex` discards the result (`tools/codegraph.go:294`) |
| M1 every-file hash on every run | Major | **Folded in** — matches my finding; kept severity Major (I had it as the top perf issue) |
| M2 per-file `GetFileByPath` N+1 | Major | **Folded in** — confirmed; merged with my general N+1 note |
| M3 double file read + TOCTOU | Major | **Folded in** — matches my finding; they sharpened the TOCTOU consequence, kept |
| M4 per-file transactions / unused `RunInTransaction` | Major | **Folded in** — matches; added their `DeleteEdgesForFile`-is-dead-work observation |
| M5 global mutex + no parallelism | Major | **Folded in** — merged with my parallelism finding; added the per-worker-parser requirement (tree-sitter not thread-safe) and the concurrent-Index race |
| M6 tsconfig alias path traversal (`ResolveAlias`) | Major (unconfirmed) | **Downgraded to Minor** — I `grep`-verified `ResolveAlias` has **zero callers** in the repo; latent defense-in-depth, not exploitable today |
| M7 BFS per-edge DB round-trip | Major | **Folded in** — verified `storage.go:903`; noted it's masked by C2 until fixed |
| N1 manifests unconditional + swallowed errors | Minor | **Folded in** — matches my finding |
| N2 FTS sanitize misses single-token operators | Minor | **Folded in** — accepted |
| N3 duplicated language/extension lists | Minor | **Folded in** — accepted |
| N4 confidence hardcoded `EXTRACTED` | Minor | **Folded in** — accepted |
| N5 dead code (`RunInTransaction`, `InsertNode`, `ClearNodesForFile`, unused stmt) | Nit | **Folded in** — accepted; `RunInTransaction` repurposed as the M4 fix vehicle |

**Points raised by red team that were NOT in my first pass and were kept:** C2, C3, M2 (as its own item), M4's dead-work detail, N2, N3, N4. Their priority ordering ("fix C2 and C3 first") matches the recommended order in §4.

**Nothing was rejected outright**; the only adjustment was the M6 downgrade, based on direct evidence (`ResolveAlias` uncalled) that the reviewing agent could not see (files it lacked).

---

## 6. Constraints honored
- READ-ONLY: no source file modified; only this report written to the project root.
- 85% confidence protocol: all findings marked `[VERIFIED]` were confirmed by reading the cited source; no speculative intent-guessing.
- Two independent review passes (local first pass + browser.chat Claude second pass) reconciled with explicit keep/drop disposition.
