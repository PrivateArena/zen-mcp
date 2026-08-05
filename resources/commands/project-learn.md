---
description: Extract and preserve core domain knowledge, technical insights, and architectural logic from this session into the project brain before it is lost.
argument-hint: |-
  i: Target memory filename/topic (e.g., 'auth-flow', 'dsp-pipeline'). Defaults to 'brain'.
---
Analyze this entire session history. Your goal is NOT to summarize what happened, but to perform a LOSSLESS EXTRACTION of the technical knowledge, architectural logic, and discovery vectors established during the conversation. 

Prioritize information density, exact syntax, and edge cases over brevity. Do not use generalized prose where specific parameters, error strings, or architectural definitions exist.

Generate a markdown payload utilizing exactly these headers. Omit a section entirely only if this session provided zero relevant data for it:

```
## Core Concepts & Axioms
Foundational definitions, mental models, or architectural truths established. What must a developer accept as a "given" before working on this?

## System Architecture & Flow
The mechanics of how things interact. Use ascii diagrams or strict sequential lists to map out data flows, bridges, or execution pipelines discussed.

## Hard Constraints & Gotchas
Non-obvious pitfalls, quirks of the environment, or hidden dependencies. List explicit things that failed, why they failed, and the specific negative constraints discovered to prevent future regression.

## Exact Syntax & Schema
Preserve exact code snippets, API endpoints, configuration keys, regular expressions, or opcode structures verified during this session. Do not truncate or use placeholders like `// ... code here`.

## Rationale & Decision Log
The "Why". For every major engineering choice made: what alternatives were weighed, why were they rejected, and what context justifies the current path?

## Folksonomy & Context Anchors
- **Upstream Dependencies:** What foundational concepts or tools must be understood *before* reading this?
- **Downstream Implications:** What future features, modules, or files will this knowledge directly impact?
- **Search Keywords:** Highly specific, un-stemmed indexing terms (e.g., error codes, specific functions) a user might query to find this block later.
```

Ensure that a developer reading this document "cold" receives the exact technical fidelity of the entire session without having to parse the original, conversational noise.

Then call `memory({ action: 'save', session_notes: <the markdown above>, session_title, objective })` using only the fields containing verified data.

---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=frontend-design`