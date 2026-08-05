# Skill Writing Guide (Antigravity CLI)

Detailed writing guide to improve the quality of skills created in the harness. Supplementary reference for SKILL.md Phase 4.

---

## Table of Contents

1. [Description Writing Patterns](#1-description-writing-patterns)
2. [Body Writing Style](#2-body-writing-style)
3. [Output Format Definition Patterns](#3-output-format-definition-patterns)
4. [Example Writing Patterns](#4-example-writing-patterns)
5. [Progressive Disclosure Patterns](#5-progressive-disclosure-patterns)
6. [Criteria for Script Bundling](#6-criteria-for-script-bundling)
7. [Data Schema Standards](#7-data-schema-standards)
8. [What NOT to Include in Skills](#8-what-not-to-include-in-skills)
9. [Skill Reuse Design](#9-skill-reuse-design)

---

## 1. Description Writing Patterns

The description is the skill's only trigger mechanism. Antigravity CLI decides whether to use a skill based only on the name + description in the `available_skills` list.

### Understanding the Trigger Mechanism

Antigravity CLI tends not to invoke skills easily for simple tasks (e.g., simple file reading) that can be easily handled with basic tools. The more complex, multi-step, and demanding of expertise a task is, the higher the probability of triggering a skill.

### Writing Principles

1. Describe both **what the skill does** + **specific trigger situations**.
2. Explicitly specify boundary conditions to distinguish similar cases that should NOT trigger it.
3. Write in a slightly "pushy" manner — resolving the tendency to judge trigger decisions conservatively.

### Good Examples

```yaml
description: "Performs all PDF operations including reading PDF files, extracting text/tables,
  merging, splitting, rotating, watermarking, encrypting, decrypting, and OCR. Be sure to use
  this skill when mentioning .pdf files or requesting PDF artifacts. Especially useful when
  conversion/editing/analysis is needed rather than simply 'reading' the PDF."
```

```yaml
description: "All spreadsheet operations including adding columns, formula calculations,
  formatting, charts, and data cleansing for Excel/CSV/TSV files. Use this skill whenever the
  user mentions a spreadsheet file — even when casually referred to (e.g., 'xlsx in Downloads folder')."
```

### Bad Examples

- `"A skill that processes data"` — Too vague, unclear about the files or tasks.
- `"PDF-related tasks"` — Lacks list of concrete operations, does not describe trigger situations.

---

## 2. Body Writing Style

### Why-First Principle

LLMs make correct judgments even in edge cases when they understand the reason. Conveying context is more effective than authoritative rules.

**Bad Example:**
```markdown
ALWAYS use pdfplumber for table extraction. NEVER use PyPDF2 for tables.
```

**Good Example:**
```markdown
Use pdfplumber for table extraction. This is because PyPDF2 is specialized for text extraction and
cannot preserve the row/column structure of tables. pdfplumber recognizes cell boundaries and returns
structured data.
```

### Generalization Principle

If problems are found in feedback or test results, **generalize at the principle level** instead of narrow modifications fitting only specific examples.

**Overfitted Modification:**
```markdown
If there is a "Q4 Sales" column, convert that column to a number.
```

**Generalized Modification:**
```markdown
If a column name contains keywords implying numeric values (e.g., "sales", "amount", "quantity"),
convert that column to a numeric type. Maintain the original value if the conversion fails.
```

### Imperative Tone

Use imperative forms like "Do X", "Create Y" rather than polite styles. A skill is an instruction sheet.

### Multilingual and English Writing Principles

If the user has requested the harness configuration in English or if the project's global collaboration environment/codebase standards are in English, the skill (`SKILL.md`) must be written entirely in English.

1. **YAML Frontmatter Description**: Write in a "pushy" manner using specific and clear English technical terms so that Antigravity CLI can easily understand it.
2. **Natural Technical English Tone**:
   - Write using the verb base form (Imperative Mood) for instructions (e.g., "Create", "Extract", "Validate", etc. Active imperative is recommended over passive structures like "is used to" or "should be").
   - Use third-person singular/present tense or concise sentence structures in explanatory sentences.
3. **Terminology Unification**: Consistently translate terms such as "서브에이전트", "산출물", and "오케스트레이터" in Korean skills into "subagent", "artifact", and "orchestrator" in English skills, maintaining unified definitions.

### Context Preservation

The context window is a public good. Ask yourself if every sentence justifies its token cost:
- "Is this something Antigravity CLI already knows?" → Delete.
- "Will the agent make mistakes without this explanation?" → Keep.
- "Is a single concrete example more effective than a long explanation?" → Replace with an example.

---

## 3. Output Format Definition Patterns

Used in skills where the output format is important:

```markdown
## Report Structure
Follow this template exactly:

# [Title]
## Summary
## Key Findings
## Recommendations
```

Defining format structures briefly, with actual examples, is more effective.

---

## 4. Example Writing Patterns

Examples are more effective than long explanations:

```markdown
## Commit Message Format

**Example 1:**
Input: Add user authentication based on JWT token
Output: feat(auth): implement JWT-based authentication

**Example 2:**
Input: Fix bug where the show password button does not work on the login page
Output: fix(login): fix toggle show password button behavior
```

---

## 5. Progressive Disclosure Patterns

### Pattern 1: Separation by Domain

```
bigquery-plugin/
└── skills/
    └── bigquery-skill/
        ├── SKILL.md (Overview + Domain Selection Guide)
        └── rules/
            ├── finance.md (Revenue, billing metrics)
            ├── sales.md (Opportunities, pipeline)
            └── product.md (API usage, features)
```

If the user asks about revenue, load only `finance.md` to maximize context efficiency.

### Pattern 2: Conditional Details

```markdown
# DOCX Processing

## Document Creation
Create a new document with docx-js. → Refer to [DOCX-JS.md](rules/docx-js.md).

## Document Editing
Modify XML directly for simple edits.
**If track changes is required**: Refer to [REDLINING.md](rules/redlining.md).
```

### Pattern 3: Structure of Large Reference Files

Include a Table of Contents at the top for reference files over 300 lines:

```markdown
# API Reference

## Table of Contents
1. [Authentication](#authentication)
2. [Endpoint List](#endpoint-list)
3. [Error Codes](#error-codes)
4. [Rate Limits](#rate-limits)

---

## Authentication
...
```

---

## 6. Criteria for Script Bundling

Observe the execution histories of agents in test runs. If the following patterns are observed, bundle them:

| Signal | Action |
|:---|:---|
| Same helper script created in 3 out of 3 tests | Bundle in `scripts/` |
| Same pip install/npm install executed every time | Specify dependency installation steps in the skill |
| Same multi-step approach repeated | Describe as standard procedure in the skill body |
| Similar error followed by the same workaround every time | Describe known issues and workarounds in the skill |

Bundled scripts must undergo execution testing.

---

## 7. Data Schema Standards

Use standard schemas for data exchange consistency between skills. They can be used for testing/evaluation of skills created in the harness.

### eval_metadata.json

Metadata for each test case:

```json
{
  "eval_id": 0,
  "eval_name": "descriptive-name-here",
  "prompt": "User's task prompt",
  "assertions": [
    "Artifact contains X",
    "File created in Y format"
  ]
}
```

### grading.json

Scoring results based on assertions:

```json
{
  "expectations": [
    {
      "text": "Artifact contains 'Seoul'",
      "passed": true,
      "evidence": "Confirmed 'Seoul region data extraction' at step 3"
    }
  ],
  "summary": {
    "passed": 2,
    "failed": 1,
    "total": 3,
    "pass_rate": 0.67
  }
}
```

**Caution on field names:** Use `text`, `passed`, and `evidence` exactly (avoid variations like `name`, `met`, `details`, etc.).

### timing.json

Execution time/token measurements:

```json
{
  "total_tokens": 84852,
  "duration_ms": 23332,
  "total_duration_seconds": 23.3
}
```

---

## 8. What NOT to Include in Skills

- Supplementary documents such as README.md, CHANGELOG.md, INSTALLATION_GUIDE.md, etc.
- Meta-information from the skill creation process (test results, iteration history).
- User-facing manuals (a skill is an instruction sheet for AI agents).
- General knowledge that Antigravity CLI already knows.

---

## 9. Skill Reuse Design

Before creating a new skill, check for duplication with existing skills. Repeatedly building harnesses can easily lead to accumulating skills with overlapping functions under different names.

| Situation | Action |
|------|------|
| Existing skill completely covers the new features | Do not create new — connect the existing skill to the subagent |
| Existing skill partially covers the features and is generalizable | Generalize and expand the existing skill |
| Domain specialization is intended for partial coverage | Proceed with new creation — maintain as separate skill |
| Feature scope is completely different | Proceed with new creation |

**Principle:** The more a single skill focuses on a single role, the higher its reusability and the less duplication. If a role has two or more aspects, check first if it can be separated.

### How far to generalize

Generalization is infinitely possible, so stop at the **intended scope of responsibility**. Maintain intended domain specialization and remove only accidental dependencies.

Example: "fintech risk assessment PDF" skill

| Stage | Result |
|------|------|
| Remove fintech dependency | "Assessment result PDF" — Stop here if the scope is the assessment report |
| Remove assessment dependency | "PDF formatting" — Reuse if it already exists; do not create a separate skill |

If the scope of responsibility is intended specialization for "fintech risk assessment", do not generalize it and maintain it as a separate skill.

The behavior of subagents relying on that skill may change. Check dependencies before expanding, and reflect the expanded use scope in the description.
