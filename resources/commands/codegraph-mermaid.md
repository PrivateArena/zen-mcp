---
description: Deep code analysis: indexing, architecture (Mermaid), and symbol usage.
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
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=codebase-research`
- `skill id=frontend-design`