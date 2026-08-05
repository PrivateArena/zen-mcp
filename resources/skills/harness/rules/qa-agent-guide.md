# QA Subagent Design Guide (Antigravity CLI)

Reference guide when including QA agents in the build harness. Based on bug patterns found in a real project (SatangSlide) and their root cause analysis, it provides validation methodologies to systematically catch defects that QA is prone to missing.

> **Note:** This guide is compatible with the Antigravity CLI plugin specifications. Position subagent definition files at `.agents/plugins/{plugin-name}/agents/{name}/agent.json`.

---

## Table of Contents

1. [Patterns of Defects Missed by QA Subagents](#1-patterns-of-defects-missed-by-qa-subagents)
2. [Integration Consistency Verification](#2-integration-consistency-verification)
3. [QA Subagent Design Principles](#3-qa-subagent-design-principles)
4. [Verification Checklist Template](#4-verification-checklist-template)
5. [QA Subagent Definition Template (agent.json)](#5-qa-subagent-definition-template-agentjson)

---

## 1. Patterns of Defects Missed by QA Subagents

### 1-1. Interface Mismatch (Boundary Mismatch)

The most frequent defect. Although two components are implemented "correctly" individually, the contract breaks at the connection point.

| Interface | Mismatch Example | Reason for Missing |
|:---|:---|:---|
| API Response → Front hook | API returns `{ projects: [...] }`, hook expects `SlideProject[]` | Normal when verified individually; no cross-comparison done |
| API Response field name → Type definition | API is `thumbnailUrl` (camelCase), type is `thumbnail_url` (snake_case) | Compiler cannot catch it if cast as a TypeScript generic |
| File path → Link href | Page is at `/dashboard/create`, link points to `/create` | File structure and href are not cross-compared |
| State transition map → Actual status update | Map defines `generating_template → template_approved`, transition omitted in code | Checked only the map existence; did not track all update code |
| API Endpoint → Front hook | API exists but has no matching hook (never called) | Failed to map API list and hook list 1:1 |
| Immediate response → Asynchronous result | API immediately returns `{ status }`, front accesses `data.failedIndices` | Checked type only, without distinguishing sync/async responses |

### 1-2. Why Static Code Review Fails to Catch It

- **Limitation of TypeScript Generics**: `fetchJson<SlideProject[]>()` — compiles successfully even if the runtime response is `{ projects: [...] }`.
- **`npm run build` succeeds ≠ normal operation**: If type casting, `any`, or generics are used, the build succeeds but fails at runtime.
- **Existence Verification vs Connection Verification**: "Does the API exist?" and "Does the API's response match the caller's expectations?" are completely different verifications.

---

## 2. Integration Consistency Verification

Areas of **cross-comparison verification** that must be included in the QA subagent.

### 2-1. API Response ↔ Front Hook Type Cross-Verification

**Method**: Compare the `NextResponse.json()` caller inside each API route with the `fetchJson<T>` type parameter of the corresponding hook.

```
Verification Steps:
1. Extract the shape of the object passed to NextResponse.json() in the API route.
2. Verify the type T of fetchJson<T> in the corresponding hook.
3. Compare if the shape and T match.
4. Verify wrapping (e.g., if the API returns { data: [...] }, check if the hook extracts .data).
```

**Patterns to watch out for especially:**
- Pagination API: `{ items: [], total, page }` vs front expecting an array.
- Discrepancy between snake_case DB fields → camelCase API responses → front type definitions.
- Difference in shape between immediate responses (202 Accepted) and final results.

### 2-2. File Path ↔ Link/Router Path Mapping

**Method**: Extract URL paths of page files under `src/app/` and compare them with all `href`, `router.push()`, and `redirect()` values in the code.

```
Verification Steps:
1. Extract URL patterns from page.tsx file paths under src/app/.
   - (group) → Remove from URL
   - [param] → Dynamic segment
2. Collect all href=, router.push(, and redirect( values in the code.
   - Verify if each link maps to an actually existing page path.
3. Watch out for URL prefixes of pages inside route groups (e.g., under dashboard/).
```

### 2-3. State Transition Completeness Tracking

**Method**: Extract all `status:` updates in the code and compare them with the state transition map.

```
Verification Steps:
1. Extract the list of allowed transitions from the state transition map (STATE_TRANSITIONS).
2. Search for the .update({ status: "..." }) pattern in all API routes.
3. Verify if each transition is defined in the map.
4. Identify those defined in the map but not executed in code (dead transitions).
5. Especially check if the transition from intermediate states (e.g., generating_template) to the final state (template_approved) is missing.
```

### 2-4. API Endpoint ↔ Front Hook 1:1 Mapping

**Method**: List all API routes and front hooks to verify that they match.

```
Verification Steps:
1. Extract endpoint lists by HTTP method from route.ts under src/app/api/.
2. Extract fetch call URL lists from use*.ts under src/hooks/.
3. Identify API endpoints not called by hooks → Flag as "Unused".
4. Determine whether "Unused" is intentional (admin APIs, etc.) or not (call missing).
```

---

## 3. QA Subagent Design Principles

### 3-1. Do Not Restrict Tool Usage

The QA subagent should be able to search patterns with Grep, cross-check automatically by running scripts, and make corrections if necessary. Therefore, grant secure read/write tools by default.

### 3-2. Prioritize "Cross-Comparison" over "Existence Verification" in Checklists

| Weak Checklist | Strong Checklist |
|:---|:---|
| Does the API endpoint exist? | Does the API endpoint's response shape match the corresponding hook's type? |
| Is the state transition map defined? | Do all status update codes match the transitions in the map? |
| Does the page file exist? | Do all links in the code point to actually existing pages? |
| Is TypeScript strict mode enabled? | Are there any type safeties bypassed by generic casting? |

### 3-3. "Read Both Sides Simultaneously" Principle

To catch boundary interface bugs, QA must not read only one side. Be sure to:
- Read the API route **and** the corresponding hook **together**.
- Read the state transition map **and** the actual update code **together**.
- Read the file structure **and** the link paths **together**.

Explicitly write this principle into the subagent definition.

### 3-4. Execute QA Immediately After Each Module is Finished, Not After the Build

If QA is placed only in "Phase 4: After Overall Completion" in the orchestrator, bugs accumulate, raising debugging costs. Execute cross-verification of the relevant API + corresponding hook immediately after each backend API and feature completion, either in parallel or sequentially (Incremental QA).

---

## 4. Verification Checklist Template

An integration consistency checklist for web applications to include in the QA subagent definition.

```markdown
### Integration Consistency Verification (Web App)

#### API ↔ Frontend Connection
- [ ] Response shape of all API routes matches the generic type of corresponding hooks.
- [ ] Verified that wrapped responses ({ items: [...] }) are correctly unwrapped in the hooks.
- [ ] snake_case ↔ camelCase conversion is applied consistently.
- [ ] Verified that immediate responses (202) and final result shapes are distinguished in the front.
- [ ] Front hooks corresponding to all API endpoints exist and are actually called.

#### Routing Consistency
- [ ] All href/router.push values in the code match actual page file paths.
- [ ] Route path verification takes into account that route groups ((group)) are removed from the URL.
- [ ] Verified that dynamic segments ([id]) are filled with correct parameters.

#### State Machine Consistency
- [ ] All defined state transitions are executed in the code (no dead transitions).
- [ ] All status updates in the code are defined in the transition map (no unauthorized transitions).
- [ ] Transitions from intermediate states to final states are not omitted.
- [ ] State-based branching (if status === "X") in the front is actually reachable.

#### Data Flow Consistency
- [ ] Mapping between DB schema field names and API response field names is consistent.
- [ ] Front type definitions and API response field names match.
- [ ] Handling of null/undefined for optional fields is consistent on both sides.
```

---

## 5. QA Subagent Definition Template (agent.json)

An `agent.json` file template to utilize when creating a QA agent for the build harness.

```json
{
  "name": "qa-inspector",
  "description": "QA validation expert. Verifies specification compliance, integration consistency, and design quality.",
  "hidden": false,
  "config": {
    "customAgent": {
      "systemPromptSections": [
        {
          "title": "Agent System Instructions",
          "content": "You are the specialized subagent 'qa-inspector' for Antigravity CLI.\n\n## Core Roles\nFocus on verifying implementation quality against specifications and **integration consistency between modules**.\n\n## Verification Priorities\n1. **Integration Consistency** (Highest) — Interface mismatch is the main cause of runtime errors.\n2. **Functional Spec Compliance** — API/state machine/data model.\n3. **Design Quality** — Colors/typography/responsiveness.\n4. **Code Quality** — Unused code, naming conventions.\n\n## Verification Method: \"Read Both Sides Simultaneously\"\nFor boundary verifications, always **open both codes simultaneously** and cross-compare:\n- API response shape: `route.ts`'s NextResponse.json() **and** `hooks/`'s fetchJson<T>\n- Routing: `src/app/` page file paths **and** href/router.push values\n- State transition: STATE_TRANSITIONS map **and** `.update({ status })` code\n- DB → API → UI: Table column names **and** API response fields → Type definitions\n\n## Input/Output Protocol\n- Input: Codebase path to verify, specification documents.\n- Output: Verification report (`_workspace/qa_report.md`).\n  * List passed/failed/unverified items using icons.\n  * For failed items, include specific file:line info along with clear suggestions for modification code."
        }
      ],
      "toolNames": [
        "view_file",
        "write_to_file",
        "replace_file_content",
        "list_dir",
        "grep_search",
        "run_command"
      ],
      "systemPromptConfig": {
        "includeSections": [
          "user_information",
          "skills",
          "messaging",
          "artifacts",
          "user_rules"
        ]
      }
    }
  }
}
```

---

## Real-World Case: Bugs Discovered in SatangSlide

All content in this guide is derived from lessons extracted from the actual bugs below.

| Bug | Interface | Cause |
|:---|:---|:---|
| `projects?.filter is not a function` | API→Hook | API returned `{projects:[]}`, hook expected an array |
| 404 for all links on dashboard | File path→href | Missing `/dashboard/` prefix |
| Theme image not visible | API→Component | `thumbnailUrl` vs `thumbnail_url` |
| Theme selection not saved | API→Hook | select-theme API existed, hook was missing |
| Creation page waiting forever | State transition→Code | Transition code to `template_approved` was missing |
| `data.failedIndices` crash | Immediate response→Front | Accessed background result in immediate response |
| 404 viewing slide after completion | File path→href | `/projects/` → `/dashboard/projects/` |
