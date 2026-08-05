---
description: Configures a harness. A meta-skill that creates professional subagent definitions (agent.json) and skills (SKILL.md) in the form of a plugin. Used when: (1) requested to 'configure harness' or 'build harness', (2) requested to 'design harness' or 'engineer harness', (3) building a harness-based automation system for a new domain/project, (4) reconfiguring or expanding an existing harness configuration, (5) operations/maintenance requests for existing harnesses such as 'check harness', 'audit harness', 'harness status', 'agent/skill sync', etc.
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: Harness - Agent Team & Skill Architect

---
name: harness
description: "Configures a harness. A meta-skill that creates professional subagent definitions (agent.json) and skills (SKILL.md) in the form of a plugin. Used when: (1) requested to 'configure harness' or 'build harness', (2) requested to 'design harness' or 'engineer harness', (3) building a harness-based automation system for a new domain/project, (4) reconfiguring or expanding an existing harness configuration, (5) operations/maintenance requests for existing harnesses such as 'check harness', 'audit harness', 'harness status', 'agent/skill sync', etc."
---

# Harness — Agent Team & Skill Architect for Antigravity CLI

A meta-skill that configures a harness suitable for the domain/project, defines the role of each subagent, and creates skills that the agents will use.

**Core Principles:**
1. **Package in a plugin structure.** — Package the subagent definition (`.agents/plugins/{domain}-plugin/agents/{name}/agent.json`) and the skill (`.agents/plugins/{domain}-plugin/skills/{name}/SKILL.md`) within a single plugin folder.
2. **Use the `invoke_subagent` tool as the default execution mode.** — Utilize the subagent delegation mechanism of Antigravity CLI (invoking `invoke_subagent` and collaborating via `send_message`).
3. **Register the harness pointer in AGENTS.md.** — Record only the minimal pointer (trigger rules + change history) so that the orchestrator skill triggers in new sessions.
4. **The harness is an evolving system, not a static fixture.** — Reflect feedback after each run and continuously update agents, skills, and AGENTS.md.
5. **Determine the writing language based on the user's requested language and context.** — If the user makes requests in English, or if the project's global collaboration environment, codebase comments, and documentation standards are in English, write all texts for newly created subagents (such as `description` and `systemPromptSections` in `agent.json`), skills (such as `SKILL.md` body and YAML frontmatter `description`), and the orchestrator in English.

## Workflow

### Phase 0: Status Audit

When the harness skill is triggered, first check the status of the existing harness.

1. Read `project/.agents/plugins/` and `project/AGENTS.md`.
2. Branch the execution mode based on the current status:
   - **New Build**: The plugin directory does not exist or is empty → Run all phases starting from Phase 1.
   - **Existing Expansion**: The existing harness exists and there is a request to add a new agent/skill → Run only the required phases according to the Phase Selection Matrix below.
   - **Operations/Maintenance**: Request to audit, modify, or sync the existing harness → Proceed to the Phase 7-5 Operations/Maintenance workflow.

   **Phase Selection Matrix for Existing Expansion:**
   | Change Type | Phase 1 | Phase 2 | Phase 3 | Phase 4 | Phase 5 | Phase 6 |
   |----------|---------|---------|---------|---------|---------|---------|
   | Add Agent | Skip (Use Phase 0 results) | Determine placement only | Required (includes 3-0) | If dedicated skill is needed (includes 4-0) | Modify Orchestrator | Required |
   | Add/Modify Skill | Skip | Skip | Skip | Required (includes 4-0) | If connection changes | Required |
   | Architecture Change | Skip | Required | Affected agents only (includes 3-0) | Affected skills only (includes 4-0) | Required | Required |
3. Cross-reference the existing agent/skill list with the AGENTS.md records to detect drift.
4. Report the audit summary to the user and confirm the execution plan.

### Phase 1: Domain Analysis
1. Identify the domain/project from the user request.
2. Identify core task types (generation, verification, editing, analysis, etc.).
3. Analyze conflicts/duplications with existing agents/skills based on the Phase 0 audit results.
4. Explore the project codebase — understand the tech stack, data models, and key modules.
5. **Detect user proficiency** — Understand the technical level from conversation context clues (terminology used, level of questions) and adjust the subsequent communication tone. Do not use terms like "assertion" or "JSON schema" without explanation for users with little coding experience.
6. **Identify the writing language** — Detect if the user is requesting the harness configuration in English, or if the codebase's (README, existing skills/code) default language is English. If it is an English-based project or the user requests in English, determine the language of the harness artifacts (skills, agent definitions, orchestrator) to be English.

### Phase 2: Team Architecture Design

#### 2-1. Select Execution Mode

**Subagents are the default execution mode.** When two or more agents collaborate, delegate via Antigravity CLI subagents. The main agent plays the role of the orchestrator, and each subagent performs tasks in an independent context created by calling the `invoke_subagent` tool and returns the results.

| Mode | When to Use | Characteristics |
|------|----------|------|
| **Subagent** (Default) | Two or more collaborating, each agent working in an independent context | `invoke_subagent` delegation, file-based result sharing |
| **Direct Execution** (Alternative) | Simple tasks, when separating agents is overhead | The main agent handles it directly without a separate agent |
| **Parallel Subagents** | Handling independent tasks concurrently | Run multiple subagents concurrently via `invoke_subagent` to collect results |

**Decision Ordering:**
1. First, identify task areas that can be separated into subagents — default if there are 2 or more specialized areas.
2. Consider parallel subagents if they are independent tasks that can be executed in parallel.
3. Use direct execution for simple, one-off tasks.

> For a detailed comparison table and pattern-specific decision trees, see "Execution Mode" in `rules/agent-design-patterns.md`.

#### 2-2. Select Architecture Pattern

1. Decompose the task into specialized areas.
2. Determine the subagent team structure (see `rules/agent-design-patterns.md` for architecture patterns):
   - **Pipeline**: Sequential dependent tasks (subagent chain)
   - **Fan-out/Fan-in**: Parallel independent tasks (parallel subagents)
   - **Expert Pool**: Conditional select invocation (conditional subagents)
   - **Producer-Reviewer**: Quality check after generation (subagent loop)
   - **Supervisor**: Central agent manages state and dynamically distributes tasks (main → sub)
   - **Hierarchical Delegation**: Upper agent recursively delegates to lower agents (subagent chain)

#### 2-3. Agent Separation Criteria

Determine based on four axes: expertise, parallelism, context, and reusability. For a detailed criteria table, see "Agent Separation Criteria" in `rules/agent-design-patterns.md`. Reviewing duplication/reusability with existing subagents is covered in Phase 3-0.

### Phase 3: Subagent Definition Creation

#### 3-0. Review Existing Subagents for Duplication

Before creating a new subagent, check for duplication with existing `agent.json` files in `project/.agents/plugins/*/agents/`. Repeatedly building harnesses can easily lead to accumulating subagents with overlapping roles under different names.

> For duplication classification criteria and reuse design, see "Subagent Reuse Design" in `rules/agent-design-patterns.md`.

**All subagents must be defined as a `project/.agents/plugins/{domain}-plugin/agents/{name}/agent.json` file.**
Defining subagents as files allows them to be reused in the next session, and explicitly defining roles and task principles ensures collaboration quality.

Required components of `agent.json` for each subagent:
- `name`: Agent name (maps to TypeName)
- `description`: A summary for the user to understand the agent's identity at a glance
- `config.customAgent.systemPromptSections`: Specify detailed instructions for the agent by section (including core role, task principles, input/output protocols)
- `config.customAgent.toolNames`: Recommended toolset for the agent to utilize
- `config.customAgent.systemPromptConfig.includeSections`: List of system prompt sections to be provided by Antigravity CLI
- **Language Consistency**: Consistent with the writing language determined in Phase 1 (Korean or English), write the `description` and the system prompt inside `systemPromptSections`. For English projects or English requests, write all prompts and descriptions in natural Technical English.

> For definition structures and actual file examples, see "Subagent Definition Structure" in `rules/agent-design-patterns.md` + `rules/team-examples.md`.

**Mandatory Requirements When Including QA Agents:**
- The QA agent requires full tool access, so do not apply read-only restrictions.
- The core of QA is not "existence verification" but **"boundary interface cross-comparison"** — reading both the API response and the front hook simultaneously to compare shapes.
- QA should not be run once at the end of completion, but **incrementally run immediately after each module is completed** (incremental QA).
- Detailed guide: See `rules/qa-agent-guide.md`.

### Phase 4: Create Skills

Create the skills to be used by each subagent in `project/.agents/plugins/{domain}-plugin/skills/{name}/SKILL.md`. For a detailed writing guide, see `rules/skill-writing-guide.md`.

#### 4-0. Review Existing Skills for Duplication

Before creating a new skill, check for duplication with existing skills in `project/.agents/plugins/*/skills/`. Repeatedly building harnesses can easily lead to accumulating skills with overlapping functions under different names.

> For duplication classification criteria and generalization patterns, see "Skill Reuse Design" in `rules/skill-writing-guide.md`.

#### 4-1. Skill Structure

```
{domain}-plugin/
└── skills/
    └── {skill-name}/
        ├── SKILL.md (Required)
        │   ├── YAML frontmatter (name, description required)
        │   └── Markdown Body
        └── Bundled Resources (Optional)
            ├── scripts/    - Execution code for repetitive/deterministic tasks
            ├── rules/      - Reference documents to load conditionally
            └── assets/     - Files used in outputs (templates, images, etc.)
```

#### 4-2. Description Writing — Active Trigger Indication

The description is the skill's only trigger mechanism. Antigravity CLI decides whether to use a skill based only on the name + description in the `available_skills` list, so write descriptions in a **pushy** manner.

**Bad Example:** `"A skill that processes PDF documents"`
**Good Example:** `"Performs all PDF operations including reading PDF files, extracting text/tables, merging, splitting, rotating, watermarking, encrypting, decrypting, and OCR. Be sure to use this skill when mentioning .pdf files or requesting PDF artifacts."`

Key: Describe both what the skill does + specific trigger situations, and distinguish it from similar cases that should not trigger it.

#### 4-3. Body Writing Principles

| Principle | Description |
|------|------|
| **Explain the Why** | Instead of authoritative commands like "ALWAYS/NEVER", convey the reason why it should be done that way. LLMs make correct judgments even in edge cases when they understand the reason. |
| **Keep it Lean** | The context window is a public good. Target the SKILL.md body to be within 500 lines, and delete or move content that does not justify its token weight to `rules/`. |
| **Generalize** | Rather than narrow rules fitting only specific examples, explain the principles to respond to various inputs. Do not overfit. |
| **Bundle Repetitive Code** | If agents are found to be writing common scripts in test runs, pre-bundle them in `scripts/`. |
| **Write in Imperative** | Use imperative/instructional tones like "Do X", "Create Y". |
| **Language Consistency** | Use the detected language (Korean/English) consistently. For English requests/global projects, write the YAML frontmatter description and the entire skill body in perfect English, ensuring the explanations sound like natural technical documentation rather than machine-translated. |

#### 4-4. Progressive Disclosure

Skills manage context through a 3-stage loading system:

| Stage | Loading Time | Size Target |
|------|----------|----------|
| **Metadata** (name + description) | Always present in context | ~100 words |
| **SKILL.md Body** | When skill triggers | <500 lines |
| **rules/** | Only when needed | Unlimited (scripts can run without loading) |

**Size Management Rules:**
- When SKILL.md approaches 500 lines, separate details into `rules/` and leave pointers in the body indicating "when to read this file."
- For rules files over 300 lines, include a **Table of Contents (ToC)** at the top.
- If there are domain/framework-specific variations, separate them into domain-specific subdirectories under `rules/` to load only the relevant files.

```
cloud-deploy-plugin/
└── skills/
    └── cloud-deploy/
        ├── SKILL.md (Workflow + Selection Guide)
        └── rules/
            ├── aws.md    ← Loaded only when AWS is selected
            ├── gcp.md
            └── azure.md
```

#### 4-5. Skill-Agent Connection Principles

- 1 Subagent ↔ 1~N Skills (1:1 or 1:many)
- A skill shared by multiple subagents is also possible
- Skills contain "How to do", while agents contain "Who does it".

> For detailed writing patterns, examples, and data schema standards, see `rules/skill-writing-guide.md`.

### Phase 5: Integration and Orchestration

The orchestrator is a special form of skill that orchestrates the entire team by weaving individual subagents and skills into a single workflow. If the individual skills created in Phase 4 define "what and how each agent does", the orchestrator defines "who collaborates when and in what order". For a specific template, see `rules/orchestrator-template.md`.

**Modifying the Orchestrator for Existing Expansion:** For an existing expansion rather than a new build, do not create a new orchestrator; instead, modify the existing orchestrator. Reflect the new agent in the team composition, task allocation, and data flow when adding a subagent, and add trigger keywords related to the new agent to the description.

The orchestrator pattern varies depending on the execution mode selected in Phase 2-1:

#### 5-0. Orchestrator Patterns (by Mode)

**Subagent Pattern (Default):**
The main agent (orchestrator) runs subagents by calling the `invoke_subagent` tool. Each subagent performs tasks in an independent context and returns results to the main agent. Share intermediate artifacts based on files.

```
[Orchestrator/Main]
    ├── invoke_subagent(TypeName: "analyst") ──→ _workspace/02_analysis.md
    ├── invoke_subagent(TypeName: "builder") ──→ _workspace/03_build.md
    ├── invoke_subagent(TypeName: "qa") ──────→ _workspace/04_qa_report.md
    └── Collect and integrate results
```

**Parallel Subagents Pattern:**
Run independent tasks concurrently. Call the `invoke_subagent` tool sequentially to send multiple agents to the background, wait until all results are collected, and integrate them after collection.

```
[Orchestrator/Main]
    ├── invoke_subagent(TypeName: "researcher-a") ──→ _workspace/02_research_a.md
    ├── invoke_subagent(TypeName: "researcher-b") ──→ _workspace/02_research_b.md
    ├── invoke_subagent(TypeName: "researcher-c") ──→ _workspace/02_research_c.md
    └── Integrate all results → Final artifact
```

**Direct Execution Pattern (Alternative):**
For simple tasks, the main agent handles it directly without a separate subagent plugin. Used when the subagent creation overhead is greater than the task itself.

```
[Main]
    └── Direct execution (Tool calls)
```

#### 5-1. Data Delivery Protocol

Specify the data delivery method between subagents in the orchestrator:

| Strategy | Method | Suitable Case |
|------|------|-----------|
| **File-based** | Write and read files at agreed paths | Large data, structured artifacts, audit trail needed |
| **Return-value-based** | Utilize the subagent execution result message | Lightweight results, simple state delivery |
| **AGENTS.md Sharing** | Record context in project AGENTS.md | Configuration information that needs long-term preservation |

**Recommended Combination:** File-based (main artifact) + Return-value-based (state delivery)

Rules for file-based delivery:
- Create a `_workspace/` folder under the working directory to store intermediate artifacts
- Filename convention: `{phase}_{agent}_{artifact}.{ext}` (e.g., `01_analyst_requirements.md`)
- Output only the final artifact to the user-specified path, and preserve intermediate files (`_workspace/`) (for post-verification and audit trails)

#### 5-2. Error Handling

Include error handling policies in the orchestrator. Core principles: retry once; if it fails again, proceed without that result (note the omission in the report); preserve conflicting data by attributing sources instead of deleting.

> For the strategy table by error type and implementation details, see "Error Handling" in `rules/orchestrator-template.md`.

#### 5-3. Team Size Guidelines

| Task Scale | Recommended Subagents | Tasks per Agent |
|----------|-------------------|--------------|
| Small (5~10 tasks) | 2~3 | 3~5 |
| Medium (10~20 tasks) | 3~5 | 4~6 |
| Large (20+ tasks) | 5~7 | 4~5 |

> The more subagents there are, the greater the orchestration burden on the main agent. A focused team of 3 is better than a distracted team of 5.

#### 5-4. Register Harness Pointer in AGENTS.md

After completing the harness configuration, register a minimal pointer in the project's `AGENTS.md`. Since AGENTS.md is loaded in every new session, recording the harness existence and trigger rules allows the orchestrator skill to handle the rest.

**AGENTS.md Template:**

```markdown
## Harness: {Domain Name}

**Goal:** {One-line core goal of the harness}

**Trigger:** Use the `{orchestrator-skill-name}` skill when requesting tasks related to {Domain}. Simple questions can be answered directly.

**Change History:**
| Date | Change | Target | Reason |
|------|----------|------|------|
| {YYYY-MM-DD} | Initial Configuration | All | - |
```

**What NOT to put in AGENTS.md:** Subagent list, skill list, directory structure, detailed execution rules. Reason: The agent/skill list is managed inside the orchestrator skill and the plugin directory, so it is redundant. The directory structure can be checked directly in the file system. AGENTS.md contains only the **pointer (trigger rules) + change history**.

#### 5-5. Support for Follow-up Tasks

The orchestrator must handle not only the initial run but also follow-up tasks. Ensure the following three:

**1. Include follow-up keywords in the orchestrator description:**
Triggering with initial creation keywords only is insufficient for follow-up requests. Mandatory follow-up expressions in the description:
- "re-run", "run again", "update", "modify", "supplement"
- "only {subtask} of {domain} again"
- "based on previous results", "improve results"

**2. Add a context check step in orchestrator Phase 1:**
At the start of the workflow, check if existing artifacts exist to determine the execution mode:
- `_workspace/` exists + user requests partial modification → **Partial Re-run** (Re-invoke only the relevant subagent and overwrite only the modified targets among existing artifacts)
- `_workspace/` exists + user provides new input → **New Run** (Move the existing _workspace to `_workspace_prev/` and proceed to Phase 1)
- `_workspace/` does not exist → **Initial Run**

**3. Include re-invocation instructions in subagent definitions:**
Specify "behavior when previous artifacts exist" in the systemPromptSections inside each subagent's `agent.json`:
- Read and reflect improvements if previous result files exist
- Update result files by prioritizing user feedback if provided

> See "Phase 0: Context Check" section in the orchestrator template: `rules/orchestrator-template.md`

### Phase 6: Verification and Testing

Verify the generated harness. For a detailed testing methodology, see `rules/skill-testing-guide.md`.

#### 6-1. Structure Verification

- Verify that all subagent `agent.json` files are in the correct locations
- Validate the skill's frontmatter (name, description)
- Verify reference consistency between subagents

#### 6-2. Verification by Execution Mode

- **Subagent**: Verify input/output connections and file-based data flows for each agent
- **Parallel Subagents**: Ensure independence (prevent concurrent writing to the same file) and verify result collection logic
- **Direct Execution**: Verify that the orchestrator did not perform unnecessary agent separation

#### 6-3. Skill Execution Test

Perform actual execution tests for each generated skill:

1. **Write Test Prompts** — Write 2~3 realistic test prompts for each skill. Write them in specific, natural sentences that actual users would enter.
2. **With-skill vs Without-skill Comparison Run** — If possible, compare execution with and without the skill to check the value added by the skill.
3. **Result Evaluation** — Evaluate the quality of artifacts qualitatively (user review) + quantitatively (assertion-based). If the artifacts are objectively verifiable (file creation, data extraction, etc.), define assertions; if subjective (style, design), rely on user feedback.
4. **Iterative Improvement Loop** — If problems are found in test results:
   - **Generalize** the feedback to modify the skill (avoid narrow modifications fitting only specific examples)
   - Re-test after modification
   - Repeat until the user is satisfied or there is no further meaningful improvement.
5. **Bundle Repetitive Patterns** — If common code written by agents is discovered during test execution, pre-bundle that code in `scripts/`.

#### 6-4. Trigger Verification

Validate that the description of each skill triggers correctly:

1. **Should-trigger Queries** (8~10 queries) — Various expressions with the intent to trigger the skill (formal/casual, explicit/implicit)
2. **Should-NOT-trigger Queries** (8~10 queries) — "Near-miss" queries with similar keywords but for which other tools/skills are suitable

**Key to near-miss writing:** Obvious unrelated queries like "write a Fibonacci function" have no test value. **Ambiguous boundary queries** like "extract charts from this Excel file to PNG" (xlsx skill vs image conversion) are good test cases.

Verify trigger conflicts with existing skills at this stage as well.

#### 6-5. Dry Run Test

- Review whether the Phase order of the orchestrator skill is logical
- Verify that there are no empty segments (dead links) in the data delivery path
- Verify that the input of all subagents matches the output of the previous Phase
- Verify that the fallback path for each error scenario is executable

#### 6-6. Write Test Scenarios

- Add a `## Test Scenarios` section to the orchestrator skill
- Describe at least 1 normal flow + 1 error flow

### Phase 7: Harness Evolution

The harness is not a static artifact created once and done. It is a system that continuously evolves based on user feedback.

#### 7-1. Collect Feedback After Run

Request feedback from the user after completing each harness run:
- "Is there anything to improve in the results?"
- "Is there anything you want to change in the subagent configuration or workflow?"

If there is no feedback, move on. Do not force it, but always provide the opportunity.

#### 7-2. Feedback Reflection Path

The modification target varies depending on the type of feedback:

| Feedback Type | Modification Target | Example |
|-----------|----------|------|
| Artifact Quality | Skill of the relevant agent | "Analysis is too superficial" → Add depth criteria to the skill |
| Agent Role | Subagent definition `agent.json` | "Security review is also needed" → Add a new agent |
| Workflow Order | Orchestrator skill | "Verification should come first" → Change Phase order |
| Team Composition | Orchestrator + Agent | "These two can be merged" → Merge agents |
| Trigger Omission | Skill description | "Does not work with this expression" → Expand description |

#### 7-3. Change History

Record all changes in the **Change History** table of AGENTS.md (same table as the "Change History" section of the Phase 5-4 template):

```markdown
**Change History:**
| Date | Change | Target | Reason |
|------|----------|------|------|
| 2026-04-05 | Initial Configuration | All | - |
| 2026-04-07 | Added QA Agent | agents/qa/agent.json | Feedback on lack of artifact quality verification |
| 2026-04-10 | Added Tone Guide | skills/content-creator | Feedback on "too stiff" |
```

Track the direction of harness evolution through this history to prevent regression.

#### 7-4. Evolution Triggers

Suggest evolution not only when the user explicitly requests "modify the harness", but also in the following situations:
- When the same type of feedback is repeated 2 or more times
- When a pattern of subagents repeatedly failing is discovered
- When it is observed that the user bypasses the orchestrator to perform tasks manually

#### 7-5. Operations/Maintenance Workflow

Systematically perform auditing, modifying, and syncing of the existing harness. Follow this workflow when entering the "operations/maintenance" branch in Phase 0.

**Step 1: Status Audit**
- Compare the list of files in the plugin directory (`agents/`) with the agent configuration of the orchestrator skill → Generate a drift list.
- Compare the list of files in the plugin directory (`skills/`) with the skill configuration of the orchestrator skill → Generate a drift list.
- Report the audit results to the user.

**Step 2: Incremental Add/Modify**
- Add/modify/delete subagents and skills based on the user's request.
- Make changes one at a time, and run Step 3 (sync) immediately after each change.

**Step 3: Update AGENTS.md Change History**
- Record the date, change content, target, and reason in the change history table.

**Step 4: Verify Changes**
- Verify the structure of the modified agents/skills (based on Phase 6-1 criteria)
- Verify triggers if the modifications affect triggers (based on Phase 6-4 criteria)
- Perform up to Phase 6-3 (execution test) and 6-5 (dry run) for large-scale changes (architecture changes, adding/deleting 3 or more agents)
- Final verification of alignment between AGENTS.md and actual files

## Artifact Checklist

Verify after completion:

- [ ] `.agents/plugins/{domain}-plugin/plugin.json` — **Mandatory creation of plugin definition file**
- [ ] `.agents/plugins/{domain}-plugin/agents/{name}/agent.json` — **Mandatory creation of subagent definition file**
- [ ] `.agents/plugins/{domain}-plugin/skills/` — Skill files (SKILL.md + rules/)
- [ ] 1 Orchestrator skill (includes data flow + error handling + test scenarios)
- [ ] Explicit execution mode (choose among subagent-based / parallel subagents / direct execution)
- [ ] No conflict with existing agents/skills
- [ ] Completed duplication review for existing subagents before creating new ones (Phase 3-0)
- [ ] Completed duplication review for existing skills before creating new ones (Phase 4-0)
- [ ] **Observe consistency and multilingual rules for writing language (Korean/English)** (If English request, write all agent/skill definitions and orchestrator text in English)
- [ ] Skill description written in a pushy manner — **includes follow-up task keywords**
- [ ] SKILL.md body within 500 lines, separate `rules/` if exceeded
- [ ] Verified execution with 2~3 test prompts
- [ ] Completed trigger verification (should-trigger + should-NOT-trigger)
- [ ] **Register harness pointer in AGENTS.md** (trigger rules + change history)
- [ ] **Record adding/deleting/modifying agents/skills in AGENTS.md change history**
- [ ] **Context check step in orchestrator Phase 1** (distinguish initial/follow-up/partial re-run)

## References

- Harness Patterns: `rules/agent-design-patterns.md`
- Existing Harness Examples (includes full file text): `rules/team-examples.md`
- Orchestrator Template: `rules/orchestrator-template.md`
- **Skill Writing Guide**: `rules/skill-writing-guide.md` — Writing patterns, examples, data schema standards
- **Skill Testing Guide**: `rules/skill-testing-guide.md` — Testing/evaluation/iterative improvement methodologies
- **QA Agent Guide**: `rules/qa-agent-guide.md` — Reference when including QA agents in the build harness.


## Referenced Files
The following Bundled skill files are part of this skill. Read them if needed:
- [rules/skill-writing-guide.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/harness/rules/skill-writing-guide.md)
- [rules/agent-design-patterns.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/harness/rules/agent-design-patterns.md)
- [rules/orchestrator-template.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/harness/rules/orchestrator-template.md)
- [rules/team-examples.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/harness/rules/team-examples.md)
- [rules/skill-testing-guide.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/harness/rules/skill-testing-guide.md)
- [rules/qa-agent-guide.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/harness/rules/qa-agent-guide.md)

---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=harness`