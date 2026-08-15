---
name: docstring-generation
description: Hybrid deterministic+LLM docstring generation — AST pass first, codegraph-guided finishing second.
framework: "zen-mcp"
trigger: docstring generation / docsless cleanup
---

# ✍️ Hybrid Docstring Generation Skill

Two-phase workflow: a pure-algorithm AST pass does the mechanical majority of
documentation with zero LLM tokens, then a codegraph-guided LLM pass finishes
the ambiguous remainder. Ordering matters — the LLM phase must never redo work
the deterministic phase already did.

## Phase 1: Deterministic AST Pass (no LLM, no codegraph)

Run the standalone tool:
```bash
docgen -write ./...
```

What it does, so you can reason about its output rather than re-deriving it:

- **Naming decomposition**: `IsX/HasX` → "reports whether X", `GetX` →
  "returns X", `NewX` → "constructs a new X", `ParseX` → "parses X", etc.
- **Signature analysis**: `(T, error)` returns, pointer vs value receivers,
  variadic/`context.Context` params.
- **Body scan (syntactic only, no semantic understanding)**: loops →
  "iterates over", `go func` → "launches concurrent work", `sync.Mutex` →
  "safe for concurrent use", error string literals → lifted verbatim into
  "returns an error if ...".
- **Confidence scoring**: a doc comment is only written when confidence
  clears the threshold; everything else is left untouched for Phase 2.
- Idempotent and safe to re-run: skips symbols with an existing `Doc`
  comment and files with a `// Code generated` header automatically.

Read the tool's own report for the skipped list, but treat it as advisory
only — Step 2 below re-derives ground truth from codegraph, because the
tool's self-report can drift (partial writes, parse errors).

## Phase 2: Codegraph-Guided Finishing (LLM, batched)

> **Conditional optimization**, same principle as codebase-research: never
> re-list or re-map what's already known. Phase 1 already touched every
> file; only pull context for symbols still missing docs.

### 1. Refresh the index (Phase 1 changed line numbers)
```bash
zcodegraph -a index
```

### 2. Enumerate ground truth
```bash
zcodegraph -a docsless
```
Authoritative — re-parses post-Phase-1 source, so it reflects exactly
what's left, not what Phase 1 thinks it left.

### 3. Batch by file/package
Group remaining symbols by file, not by symbol. One context-gathering pass
per file batch, never per function.

### 4. Gather context — minimum necessary
```bash
codegraph({ action: 'skeletons', query: <files in this batch> })
codegraph({ action: 'related', query: <file> })   # only if name+signature give no signal
```
`related` is reserved for symbols where the name has no usable verb-phrase
and the body has no error-string or control-flow signal — i.e. exactly the
cases Phase 1 explicitly declined. Don't call `related` for every symbol in
a batch; if the batch is fully name-resolvable, skip the call entirely.

### 5. Write in Phase 1's voice
Match whatever doc-comment grammar is already established in the file
(symbol-name-first, period-terminated — standard Go doc convention) so the
two passes are indistinguishable in the diff.

### 6. Re-validate
```bash
zcodegraph -a docsless
```
Must trend toward 0 (excluding intentionally-skipped generated files). If
it doesn't, keep batching — don't stop silently on a partial pass.

## 🧠 Operational Directives

- **Never re-decide what Phase 1 resolved.** An existing doc comment —
  Phase 1's or a human's — is untouched unless the invocation explicitly
  asks for a "fix incorrect docs" pass, which is a different mode.
- **No fabrication.** If signature, body, and graph all give nothing, write
  `// TODO(docgen): purpose unclear` — never a confident-sounding guess.
- **Trust `docsless` over self-reports.** It's ground truth; a tool log is
  not.
- **Struct fields**: exported fields only by default; infer purpose from
  how the graph shows the field being read/written, not just name+tag.
- **Batch, don't per-symbol.** One skeletons/related call per file batch.
