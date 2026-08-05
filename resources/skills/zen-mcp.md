---
name: zen-mcp
description: "Operational framework for Zen MCP interaction. USE THIS SKILL at the start of every session to ensure workspace synchronization and efficient tool usage (Shell, Git, Codegraph, Archive)."
framework: "zen-mcp"
trigger: zen mcp
---

# Zen MCP Operational Excellence

This skill codifies the mandatory patterns for interacting with the Zen MCP environment. It prioritizes resource efficiency, state consistency, and technical accuracy.

## 1. Session Initialization (CRITICAL)

The very first action in any new session MUST be to synchronize the workspace context. Failure to do so leads to path errors and command failures.

*   **Primary Tool**: `workspace`
*   **Action**: Use the provided project absolute path to align the agent's working directory.
*   **Verification**: Run `shell.run(command="pwd")` immediately after to confirm alignment.

```typescript
// Example Initialization Flow
await mcp.workspace({ path: "/media/jang/home/Deve/web-reader-mcp-master" });
const { output } = await mcp.shell.run({ command: "pwd" });
// Check output matches the path
```

## 2. Tool Selection Hierarchy

To minimize token usage and maximize precision, follow this selection logic:

| Task | Preferred Tool | Why? |
| :--- | :--- | :--- |
| **Code Search** | `codegraph.search` | AI-powered symbol context, fewer tokens than `grep`. |
| **Git Operations** | `git.*` (status, diff, log) | Native git performance and formatting. |
| **File Reading** | `file.read` (or `shell.range`) | Supports range-based reading for large files (>50KB). |
| **Archives/Docs** | `archive.zip` / `archive.doc_read` | Specialized parsers for non-text formats. |
| **Browser/Web** | `web.chat` / `browser.*` | Persistent sessions for complex web flows. |

## 3. Quota & Efficiency Guards

*   **Token Optimization**: Always use the MCP `shell` (which is token-optimized) over built-in terminal tools.
*   **File Limits**: Never read more than 50KB at once. If a file is larger, use `shell.run` with `head`, `tail`, or `grep` to extract specific fragments.
*   **Batching**: Use `file.write_batch` when modifying 2+ files simultaneously.

## 4. Common Workflows

### A. Code Refactoring (The "Safety First" Pattern)
1.  **Context**: Use `workspace.context` to snapshot the current project state.
2.  **Logic**: Use `think` to plan the changes.
3.  **Search**: Use `codegraph.usage` to identify all affected call sites.
4.  **Edit**: Use `file.write_batch` or `replace_file_content` for surgical edits.
5.  **Verify**: Re-run build/test commands via `shell.run`.

### B. Binary & Archive Inspection
*   Use `archive.zip` without `fileName` to list contents.
*   Use `archive.zip` with `fileName` and `encoding="base64"` for binary assets.
*   Use `archive.xml_nodes` to understand the schema of XML/MXL/MSCZ files before full parsing.

## 5. Anti-Patterns (DO NOT DO)

*   **Recursion**: Do not run recursive `ls` or `grep` on large directories (e.g., `node_modules`).
*   **Placeholders**: Do not use placeholders in `shell` commands. Always use absolute or validated relative paths.
*   **Loops**: If a command fails 3 times, stop and report the error instead of retrying with slight variations.
