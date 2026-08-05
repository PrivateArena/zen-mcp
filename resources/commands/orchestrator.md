---
description: Coordinate tasks across multiple modes, using browser.brainstorm to surface decomposition options before delegating.
argument-hint: |-
  i: What to coordinate
---
# Skill: Codebase Research & Token Efficiency

---
name: codebase-research
description: High-precision codebase research using surgical Tool-Chaining and context listing.
---

# 🔍 Optimal Codebase Research Skill

An optimized workflow for mapping and analyzing codebases with minimum token waste and maximum signal.

## 🚀 The "Precision Tunnelling" Workflow (Strategy A)

This is the fastest path to understanding a specific feature or subsystem.

> **Conditional Optimization**: If `PROJECT_OVERVIEW.md` exists in the project root, **read it** and skip the file discovery and dependency mapping steps below — jump directly to **Skeleton Extraction** and **Cross-Link Discovery**.

### 1. File Discovery
Get the complete project file tree.
```bash
codegraph({ action: files })
```

### 2. Dependency Mapping
Understand how directories and entry points connect.
```bash
codegraph({ action: map })
# or for visual graph:
codegraph({ action: mermaid, query: "/path/to/entry" })
```

### 3. Skeleton Extraction
Extract exports, classes, function signatures, and interfaces WITHOUT body content.
```bash
codegraph({ action: skeletons, query: "src/index.ts,src/types.ts,src/lib/core/config.ts" })
```

### 4. Cross-Link Discovery
Find what links to the entry point and how the dependency graph flows.
```bash
codegraph({ action: related, query: "src/index.ts" })
```

## 🛠️ Surgical Deep Dive (Strategy B)

When the skeleton map is built, extract logic with surgical precision.

### 1. Range Extraction
Only read the specific function bodies identified in the skeleton.
```bash
file.read(path="...", offset=120, limit=30)
```

### 2. Cross-Reference Verification
If a function calls an external utility, use `codegraph.usage` or `codegraph.neighbors` on the target symbol rather than reading the file.

## 🧠 Operational Directives

- **Trust the Index**: If the project was recently indexed, skip `codegraph.index()`.
- **Kill the Status**: Skip `codegraph.status()` if you are already in a known workspace.
- **Limit Mermaid**: Never call `mermaid` with a query that returns >100 nodes; the token cost of the markdown diagram is high.
- **Limit Skeletons**: Query multiple files in a single skeletons call using comma-separated paths instead of reading files individually.
- **Internal Scratchpad**: Maintain a "Symbol Map" in your internal working memory to avoid repeating searches.
- **List vs Graph**: `files` is often higher signal than a generic `map` for understanding project structure.
- **API Alignment**: Always use the native codegraph actions (files, map, skeletons, related, usage, neighbors, search, explain) rather than wrapping file-system tools for structural discovery.


---
# Skill: Chat Research & Token Efficiency

---
name: chat-research
description: Delegates codebase research, planning, code review, and design questions to browser.chat sub-agents with surgical file uploads.
---

# 🤖 Chat-Research Skill

Leverages `browser.chat` to offload research, planning, and review tasks to external sub-agents after identifying the exact files needed via `codebase-research`.

## 🔗 Chaining with `codebase-research`

Run `codebase-research` first to identify the minimal set of target files. Then switch to this skill to delegate the heavy reasoning.

## 🚀 The "Off-Context Delegation" Workflow

### 1. Identify Targets
From the `codebase-research` output, extract the precise file paths that contain the symbols, logic, or architecture under investigation.

### 2. Prepare Payload
Gather the minimal file set:
- **Always include**: `PROJECT_OVERVIEW.md` (if it exists in the project root).
- **Include only**: files directly relevant to the question. Avoid bulk uploads.

### 3. Delegate to Sub-Agent
Use `browser.chat` with `upload_files` and a focused `message`.
```bash
# Multiple messages can be sent to the sub-agent in a single call if needed.
browser.chat(
  provider=["claude"],
  message=["Review this architecture flow and identify issues", "Review risks and mitigations", "Review performance bottlenecks and optimization suggestions"],
  upload_files=["src/index.ts", "src/daemon/server.ts", "PROJECT_OVERVIEW.md"]
)
```

### 4. Synthesize Result
The external agent returns analysis, design opinions, or review findings. Integrate the result back into the local workflow.

## 🧠 Operational Directives

- **Token Efficiency**: Upload fewer than 10 files per call. Split into multiple calls if the scope is large.
- **Context Preservation**: The web agent has no local memory. Attach all files needed to understand the question in a single call.
- **Provider Selection**: Prefer `provider="claude"` for deep code review and architecture analysis. Use other providers for fast lookups or multi-perspective brainstorming via `browser.brainstorm`.
- **Scope Guard**: Never upload secrets, keys, or environment-specific config. If a target file is ambiguous, validate before uploading.
- **Deterministic Integration**: Always verify browser.chat output against local code before acting on recommendations.


---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=codebase-research`
- `skill id=chat-research`
- `skill id=frontend-design`