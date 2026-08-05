---
description: Efficiently map and understand complex project using MCP browser.chat. Delegating jobs to sub-agents to maximize token saving.
argument-hint: |-
  i: Context or specific instructions for this skill
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
- `skill id=chat-research`
- `skill id=codebase-research`