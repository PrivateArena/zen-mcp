---
name: chat-research
description: Delegates codebase research, planning, code review, and design questions to context-isolated `subagent` calls given exact file paths, with a mandatory constrained-ask quality gate.
framework: "zen-mcp"
trigger: subagent
---

# 🤖 Subagent Delegation Skill (`chat-research`)

Leverages the `subagent` tool to offload research, planning, and review reasoning into a fresh, isolated context after identifying the exact files needed via `codebase-research`.

Used by three different callers, each with a different "why":
- **`delegate`** — open-ended understanding of a file/project (exploratory).
- **`review`** — second opinion on a first-pass findings draft (validating).
- **`architect`** — red-team on a draft plan (adversarial).

The Message-Quality Gate below is mandatory for all three. It's the one place this discipline lives — callers reference it instead of redefining "be specific" in their own words.

## 🔗 Chaining with `codebase-research`

Run `codebase-research` first to identify the minimal set of target files. Then switch to this skill to delegate the heavy reasoning.

## 🚦 Message-Quality Gate (mandatory before every `subagent` call)

A vague task wastes the call and comes back with a vague, unusable answer. Before calling `subagent({ role, task })`, fill in these four fields concretely. If any field is blank, the task isn't scoped yet — go back to `codebase-research` (or your own draft findings) rather than sending a placeholder.

1. **Context** (1–2 sentences) — what this project/module/draft does, in your own words, not the subagent's job to infer from scratch.
2. **Specific question(s)** — numbered, each answerable with a fact, decision, or list.
   - Passes: "Which module owns retry logic for the upload path?" / "Does this change break the existing `parseConfig` contract at config.ts:42?"
   - Fails: "Explain the architecture." / "Review this." / "Any thoughts?"
3. **Files or draft named, and why** — one clause per path on why it's relevant to the question above. If you can't state why a file matters, drop it. If a draft exists (findings table, architecture plan, proposed diff), put a concise version of *that* in the `task` text (or write it to a scratch file, e.g. `/tmp/<name>.md`, and pass that path) instead of naming raw files alone — critiquing a concrete claim ("I believe X causes a race condition because Y") produces a sharper response than critiquing an open prompt against the same files.
4. **Expected output shape** — bullet list / table / prose paragraph / yes-no-plus-rationale. Tell the subagent the shape you need back so the answer is usable without a follow-up round.

**Self-test before sending:** would this exact task be equally valid pasted against a *different* file or a *different* review? If yes, it's still generic — add the specific claim, file detail, or open question that makes it belong to this task and no other.

## 🚀 The "Off-Context Delegation" Workflow

### 1. Identify Targets
From `codebase-research` output, or your own draft, extract the precise file paths or claims under investigation.

### 2. Prepare the Task
Gather the minimal set:
- **Always include**: `PROJECT_OVERVIEW.md` (if it exists in the project root) — first path listed.
- **Include**: files directly relevant to the question, plus any draft (findings table, plan) being validated.
- **Avoid**: an unbounded path list — if you can't state why each path matters, the question isn't scoped yet, and that's a scoping problem, not a big task. The subagent reads the files itself, but its context is finite too.

### 3. Delegate to the subagent
Use `subagent({ role: '<review|explore>', task: <constrained question, per the quality gate> })`, and put the paths it must read inside the task text: "Read these files: `<abs paths>`".

One question per call is the default — make the calls sequentially. If the questions are tightly coupled, number them inside a single `task` instead of splitting them.


### 4. Synthesize Result
The subagent returns analysis, design opinions, or review findings. Reconcile it against your own pass explicitly (kept / refined / added / rejected) rather than accepting it uncritically — the reconcile/triage mechanics belong to the calling skill (`review`, `architect`), not here.

## 🧠 Operational Directives

- **Context Economy**: Name the minimal set of paths per call; split into multiple subagent calls if the scope is large.
- **Role Selection**: Prefer `role: 'review'` for critique/second-opinion passes and `role: 'explore'` for comprehension, planning, and debugging analysis. Never `general-purpose` — it may write.
- **Scope Guard**: Point it only at the files it needs — a subagent inherits your filesystem access and will read whatever path you name. If a target path is ambiguous, confirm it before delegating.
- **Deterministic Integration**: Always verify the subagent's output against local code before acting on recommendations — it is a hypothesis generator, not ground truth.