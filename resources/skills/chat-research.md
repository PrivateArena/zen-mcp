---
name: chat-research
description: Delegates codebase research, planning, code review, and design questions to browser.chat sub-agents with surgical file uploads and a mandatory constrained-ask quality gate.
framework: "zen-mcp"
trigger: browser.chat
---

# 🤖 Chat-Research Skill

Leverages `browser.chat` to offload research, planning, and review reasoning to external sub-agents after identifying the exact files needed via `codebase-research`.

Used by three different callers, each with a different "why":
- **`delegate`** — open-ended understanding of a file/project (exploratory).
- **`review`** — second opinion on a first-pass findings draft (validating).
- **`architect`** — red-team on a draft plan (adversarial).

The Message-Quality Gate below is mandatory for all three. It's the one place this discipline lives — callers reference it instead of redefining "be specific" in their own words.

## 🔗 Chaining with `codebase-research`

Run `codebase-research` first to identify the minimal set of target files. Then switch to this skill to delegate the heavy reasoning.

## 🚦 Message-Quality Gate (mandatory before every browser.chat call)

A vague message wastes the call and comes back with a vague, unusable answer. Before calling `browser.chat`, fill in these four fields concretely. If any field is blank, the task isn't scoped yet — go back to `codebase-research` (or your own draft findings) rather than sending a placeholder.

1. **Context** (1–2 sentences) — what this project/module/draft does, in your own words, not the sub-agent's job to infer from scratch.
2. **Specific question(s)** — numbered, each answerable with a fact, decision, or list.
   - Passes: "Which module owns retry logic for the upload path?" / "Does this change break the existing `parseConfig` contract at config.ts:42?"
   - Fails: "Explain the architecture." / "Review this." / "Any thoughts?"
3. **Files or draft attached, and why** — one clause per upload on why it's relevant to the question above. If you can't state why a file matters, drop it. If a draft exists (findings table, architecture plan, proposed diff), attach a concise version of *that* instead of raw files alone — critiquing a concrete claim ("I believe X causes a race condition because Y") produces a sharper response than critiquing an open prompt against the same files.
4. **Expected output shape** — bullet list / table / prose paragraph / yes-no-plus-rationale. Tell the sub-agent the shape you need back so the answer is usable without a follow-up round.

**Self-test before sending:** would this exact message be equally valid pasted against a *different* file or a *different* review? If yes, it's still generic — add the specific claim, file detail, or open question that makes it belong to this task and no other.

## 🚀 The "Off-Context Delegation" Workflow

### 1. Identify Targets
From `codebase-research` output, or your own draft, extract the precise file paths or claims under investigation.

### 2. Prepare Payload
Gather the minimal set:
- **Always include**: `PROJECT_OVERVIEW.md` (if it exists in the project root).
- **Include**: files directly relevant to the question, plus any draft (findings table, plan) being validated.
- **Avoid**: bulk uploads — needing more than ~9 files is a sign the question isn't scoped yet, not a sign the task is big.

### 3. Delegate to Sub-Agent
Use `browser.chat` with `upload_files` and the message built via the quality gate above.
```bash
# Multiple messages can be sent to the sub-agent in a single call if needed.
browser.chat(
  provider=["claude"],
  message=["<constrained question 1, per the quality gate>", "<constrained question 2>"],
  upload_files=["src/index.ts", "src/daemon/server.ts", "PROJECT_OVERVIEW.md"]
)
```

### 4. Synthesize Result
The external agent returns analysis, design opinions, or review findings. Reconcile it against your own pass explicitly (kept / refined / added / rejected) rather than accepting it uncritically — the reconcile/triage mechanics belong to the calling skill (`review`, `architect`), not here.

## 🧠 Operational Directives

- **Token Efficiency**: Upload fewer than 10 files per call. Split into multiple calls if the scope is large.
- **Context Preservation**: The web agent has no local memory. Attach everything — files and any draft — needed to understand the question in a single call.
- **Provider Selection**: Prefer `provider="claude"` for deep code review and architecture analysis.
- **Scope Guard**: Never upload secrets, keys, or environment-specific config. If a target file is ambiguous, validate before uploading.
- **Deterministic Integration**: Always verify browser.chat output against local code before acting on recommendations.