---
description: Recover full session context from the project brain and prepare to continue.
argument-hint: |-
  i: Memory filename (e.g. 'router', 'dsp'). Defaults to 'brain'.
---
Resume the session from project memory.

**Step 1 — Set workspace root.**
Call `workspace({ path: '/path/to/project' })`.

**Step 2 — Load the brain.**
Resolve the filename: '{{i}}' if provided, else 'brain'. Call `memory({ action: 'load', name: <resolved filename> })`. This also returns `git_signals` (files changed / commits since last visit) and `dependency_context` (linked sibling projects) — read both, they're often more current than the stored fields.

**Step 3 — Report what you loaded.**

> **Session:** `session_title` (Source: `<resolved filename>`)
> **Objective:** `objective`
> **What is done:** `what_is_done`
> **Facts & decisions:** list each `semantic.facts` entry — key decisions, environment notes, test commands; rationale for each.
> **Events:** summarize `episodic` entries with outcomes; don't re-litigate.
> **Procedures:** list `procedural` workflows by name and steps.
> **Git since last visit:** summarize `git_signals` (modified files, recent commits)

If `git_signals` shows changes that contradict the stored `what_is_done`/`episodic`, say so explicitly before proceeding — the brain may be stale relative to the actual working tree.

**Step 4 — Verify state.**
Run any test commands documented in `semantic.facts` (look for env/test entries). Report any regression plainly.

**Step 5 — Announce readiness.**
State the inferred next step from `what_is_done`/`episodic` and ask whether to proceed, in case priorities shifted since the last session.

If the brain has grown large (many episodic/semantic entries) and feels slow to read, mention that `memory({ action: 'compact' })` is available to fold the timeline log down — it's a fast, local operation, safe to run anytime.