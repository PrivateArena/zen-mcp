# Skill Testing & Iterative Improvement Guide (Antigravity CLI)

Methodology for verifying and iteratively improving the quality of skills created in the harness. Supplementary reference for SKILL.md Phase 6.

---

## Table of Contents

1. [Test Framework Overview](#1-test-framework-overview)
2. [How to Write Test Prompts](#2-how-to-write-test-prompts)
3. [Execution Testing: With-skill vs Baseline](#3-execution-testing-with-skill-vs-baseline)
4. [Quantitative Evaluation: Assertion-based Scoring](#4-quantitative-evaluation-assertion-based-scoring)
5. [Leveraging Specialized Agents](#5-leveraging-specialized-agents)
6. [Iterative Improvement Loop](#6-iterative-improvement-loop)
7. [Description Trigger Verification](#7-description-trigger-verification)
8. [Workspace Structure](#8-workspace-structure)

---

## 1. Test Framework Overview

Skill quality verification is a combination of **qualitative evaluation** and **quantitative evaluation**.

| Evaluation Type | Method | Suitable Skills |
|:---|:---|:---|
| **Qualitative** | User directly reviews the outputs | Subjective quality like writing style, design, or creative works |
| **Quantitative** | Automated scoring based on assertions | Objectively verifiable tasks like file creation, data extraction, or code generation |

Core loop: **Write → Test Run → Evaluate → Improve → Re-test**

---

## 2. How to Write Test Prompts

### Principles

Test prompts should be **specific, natural sentences that actual users would enter**. Abstract or artificial prompts have low test value.

### Bad Examples

```
"Process the PDF"
"Extract data"
"Generate a chart"
```

### Good Examples

```
"In 'Q4_Sales_Final_v2.xlsx' located in the Downloads folder, add a profit margin (%) column
using column C (sales) and column D (costs). Then, sort in descending order based on the profit margin."
```

```
"Extract the table on page 3 from this PDF and convert it to CSV. Since the table header has 2 lines,
the first line is the category and the second line is the actual column name."
```

### Prompt Diversity

- Mix **formal / casual** tones.
- Mix **explicit / implicit** intent (explicitly stating file formats vs inferring from context).
- Mix **simple / complex** tasks.
- Include abbreviations, typos, and casual expressions in some prompts.

### Coverage

Start with 2~3 prompts, but design them to cover:
- 1 Core usecase
- 1 Edge case
- (Optional) 1 Complex task

---

## 3. Execution Testing: With-skill vs Baseline

### 3-1. Comparison Run Structure

For each test prompt, compare execution with and without the skill:

**With-skill Run:**
```
Prompt: "{Test Prompt}"
Skill Path: {Skill Path}
Output Path: _workspace/iteration-N/eval-{id}/with_skill/outputs/
```

**Baseline Run:**
```
Prompt: "{Test Prompt}" (Identical)
Skill: None
Output Path: _workspace/iteration-N/eval-{id}/without_skill/outputs/
```

### 3-2. Baseline Selection

| Situation | Baseline |
|:---|:---|
| New Skill Creation | Run the same prompt without the skill |
| Existing Skill Improvement | Pre-modification skill version (preserve snapshot) |

---

## 4. Quantitative Evaluation: Assertion-based Scoring

### 4-1. Writing Assertions

If outputs are objectively verifiable, define assertions for automated scoring.

**Good Assertions:**
- Can objectively determine True/False.
- Clear from the descriptive name alone what is being inspected.
- Verifies the core value of the skill.

**Bad Assertions:**
- Always passes regardless of the skill existence (e.g., "Output exists").
- Requires subjective judgment (e.g., "Well written").

### 4-2. Programmable Verification

If assertions can be verified through code, write scripts. It is faster, more reliable than manual checks, and reusable for each iteration.

### 4-3. Watch out for Non-discriminating Assertions

Assertions that "pass 100% in both configurations" do not measure the discriminative value of the skill. If you find such assertions, remove them or replace them with more challenging ones.

### 4-4. Grading Result Schema

```json
{
  "expectations": [
    {
      "text": "Profit margin column added",
      "passed": true,
      "evidence": "Verified 'profit_margin_pct' column in column E"
    },
    {
      "text": "Sorted in descending order based on profit margin",
      "passed": false,
      "evidence": "Original order maintained without sorting"
    }
  ],
  "summary": {
    "passed": 1,
    "failed": 1,
    "total": 2,
    "pass_rate": 0.50
  }
}
```

---

## 5. Leveraging Specialized Agents

Utilizing subagents with specialized roles during the testing/evaluation process improves quality.

### 5-1. Grader

Performs assertion-based scoring and extracts verifiable claims from artifacts to cross-verify.

**Role:**
- Determines Pass/Fail per assertion + provides evidence.
- Extracts factual claims from the outputs and verifies them.
- Provides feedback on the quality of the evaluation itself (e.g., proposing changes if assertions are too easy or ambiguous).

### 5-2. Comparator (Blind Evaluator)

Anonymizes the two outputs as A/B, determining quality without knowing which result used the skill.

**When to use:** When you want to rigorously check "Is the new version really better?" Can be omitted in general iterative improvements.

**Judgment Criteria:**
- Content: Accuracy, completeness
- Structure: Organization, formatting, usability
- Overall score

### 5-3. Analyzer

Analyzes statistical patterns in benchmark data:
- Non-discriminating assertions (passes both configurations → no discrimination).
- High-variance evaluations (results vary significantly per run → unstable).
- Time/Token trade-offs (when the skill improves quality but increases cost).

---

## 6. Iterative Improvement Loop

### 6-1. Feedback Collection

Show the outputs to the user and receive feedback. Empty feedback is interpreted as "no issues."

### 6-2. Principles of Improvement

1. **Generalize feedback** — Modifying narrowly to fit only test examples is overfitting. Correct at the principle level.
2. **Remove weight that does not earn its keep** — Read execution histories; if the skill is dictating unproductive work, delete that part.
3. **Explain the Why** — Even if user feedback is concise, understand why it is important and reflect that understanding in the skill.
4. **Bundle repetitive tasks** — If identical helper scripts are generated in all test runs, pre-include them in `scripts/`.

### 6-3. Iteration Procedure

```
1. Modify the skill
2. Re-run all test cases in the new iteration-N+1/ directory
3. Present results to the user (comparing with previous iteration)
4. Collect feedback
5. Modify again → Repeat
```

**Termination Conditions:**
- User is satisfied.
- Feedback is empty (no issues in all outputs).
- There is no further meaningful improvement.

### 6-4. Draft → Review Pattern

When modifying a skill, **re-read with a fresh perspective** after writing the draft and improve it. Don't try to write it perfectly at once; go through a draft-review cycle.

---

## 7. Description Trigger Verification

### 7-1. Write Trigger Eval Queries

Write 20 evaluation queries — 10 should-trigger + 10 should-NOT-trigger.

**Query Quality Criteria:**
- Specific, natural sentences that actual users would enter.
- Include specific details such as file paths, personal context, column names, company names, etc.
- Mix length, tone, and format.
- Focus on **edge cases** rather than obvious answers.

**Should-trigger Queries (8~10 queries):**
- Various expressions for the same intent (formal/casual).
- Cases where the skill/file type is not explicitly mentioned but is clearly needed.
- Long-tail usecases.
- Cases that compete with other skills but where this skill must win.

**Should-NOT-trigger Queries (8~10 queries):**
- **Near-miss is key** — Queries with similar keywords but for which other tools/skills are suitable.
- Obvious unrelated queries have no test value.
- Adjacent domains, ambiguous expressions, and cases with overlapping keywords but different contexts.

### 7-2. Verify Conflicts with Existing Skills

Check if the description of the new skill overlaps with the trigger area of existing skills:

1. Collect descriptions of existing skills.
2. Verify if the should-trigger queries of the new skill incorrectly trigger existing skills.
3. If conflicts are found, describe the boundary conditions of the description more clearly.

---

## 8. Workspace Structure

Directory structure to systematically manage testing/evaluation results:

```
{skill-name}-workspace/
├── iteration-1/
│   ├── eval-descriptive-name-1/
│   │   ├── eval_metadata.json
│   │   ├── with_skill/
│   │   │   ├── outputs/
│   │   │   ├── timing.json
│   │   │   └── grading.json
│   │   └── without_skill/
│   │       ├── outputs/
│   │       ├── timing.json
│   │       └── grading.json
│   ├── eval-descriptive-name-2/
│   │   └── ...
│   └── benchmark.json
├── iteration-2/
│   └── ...
└── evals/
    └── evals.json
```

**Rules:**
- Use **descriptive names** for eval directories instead of numbers (e.g., `eval-multi-page-table-extraction`).
- Preserve each iteration in an independent directory (do not overwrite previous iterations).
- Do not delete `_workspace/` (for post-verification and history backtracking).
