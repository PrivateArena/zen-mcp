# Orchestrator Skill Template (Antigravity CLI)

The orchestrator is a high-level skill that coordinates the entire team. In Antigravity CLI, the main agent acts as the orchestrator, delegating tasks to each subagent by calling the `invoke_subagent` tool and controlling the overall workflow.

**Three Templates by Execution Mode:**
- **Template A: Subagent Mode (Default)** — Sequential delegation, file-based data delivery
- **Template B: Parallel Subagents Mode** — Simultaneous execution of independent tasks
- **Template C: Hybrid Mode** — Mixing different patterns per Phase

---

## Template A: Subagent Mode (Default · Sequential)

Used when two or more subagents collaborate sequentially. Each subagent runs in an independent isolated context, receiving and processing intermediate results through designated file paths.

```markdown
---
name: {domain}-orchestrator
description: "Orchestrator coordinating the {domain} subagent team. {Initial run keyword}. Follow-up tasks: Be sure to use this skill when requesting to modify {domain} results, partial re-runs, updates, supplementations, running again, or improving previous results."
---

# {Domain} Orchestrator

An integrated skill that coordinates the subagent team of {domain} to generate the {final artifact}.

## Execution Mode: Subagent (Sequential)

## Subagent Composition

| Agent TypeName | Role | Skill | Output |
|:---|:---|:---|:---|
| {agent-1} | {Role} | {skill} | `_workspace/{phase}_{agent}_{artifact}.md` |
| {agent-2} | {Role} | {skill} | `_workspace/{phase}_{agent}_{artifact}.md` |

## Workflow

### Phase 0: Context Check (Support for Follow-up Tasks)

Determine the execution mode by checking the existence of existing artifacts:

1. Check for the existence of the `_workspace/` directory
2. Determine execution mode:
   - **`_workspace/` does not exist** → Initial Run. Proceed to Phase 1.
   - **`_workspace/` exists + user requests partial modification** → Partial Re-run. Call only the relevant subagent via `invoke_subagent` and overwrite only the modification targets among existing artifacts.
   - **`_workspace/` exists + user provides new input** → New Run. Back up and move the existing `_workspace/` to `_workspace_{YYYYMMDD_HHMMSS}/`, then proceed to Phase 1.
3. For partial re-runs: Include the previous artifact path in the `invoke_subagent` prompt to instruct the subagent to read the existing result and reflect feedback.

### Phase 1: Preparation
1. Analyze user input — {what to grasp}
2. Create `_workspace/` in the working directory
   - **Initial Run**: Create a new `_workspace/`
   - **New Run**: Re-create a new `_workspace/` immediately after moving the existing `_workspace/` to backup
3. Save input data to `_workspace/00_input/`

### Phase 2: {Main Task — e.g., Analysis/Research}

1. Run {agent-1} by calling the `invoke_subagent` tool:
   - **TypeName**: "{agent-1}"
   - **Role**: "{agent-1-role}"
   - **Prompt**: "Read data from the specified input path `_workspace/00_input/` to perform {specific analysis content}, and write the results to `_workspace/02_{agent-1}_result.md`."
   
2. Run {agent-2} by calling the `invoke_subagent` tool:
   - **TypeName**: "{agent-2}"
   - **Role**: "{agent-2-role}"
   - **Prompt**: "Load {agent-1}'s analysis result file `_workspace/02_{agent-1}_result.md` to perform {specific processing content}, and finally write the results to `_workspace/02_{agent-2}_result.md`."

### Phase 3: {Follow-up Task — e.g., Generation/Implementation}

3. Run {agent-3} by calling the `invoke_subagent` tool:
   - **TypeName**: "{agent-3}"
   - **Role**: "{agent-3-role}"
   - **Prompt**: "Load `_workspace/02_{agent-2}_result.md` to perform {specific generation content}, and write the results to `_workspace/03_{agent-3}_result.md`."

### Phase 4: Integration
1. Read all subagents' artifact files (utilize the `view_file` tool)
2. {Apply integration/verification logic}
3. Generate final artifact: `{output-path}/{filename}`

### Phase 5: Cleanup
1. Preserve the `_workspace/` directory (do not delete intermediate artifacts — for post-verification and history backtracking)
2. Report results summary to the user

## Data Flow

```
[Orchestrator/Main]
    │
    ├── invoke_subagent("agent-1") ──→ _workspace/02_agent1_result.md
    │                                         │
    │                                         ↓ (Read)
    ├── invoke_subagent("agent-2") ──→ _workspace/02_agent2_result.md
    │                                         │
    │                                         ↓ (Read)
    ├── invoke_subagent("agent-3") ──→ _workspace/03_agent3_result.md
    │
    └── Integrate and generate final results → Final artifact
```

## Error Handling

| Scenario | Strategy |
|------|------|
| Single subagent failure | Retry invocation once. If it fails again, note the omission in the report and proceed to the next step. |
| Majority failure | Report error logs to the user immediately and confirm whether to proceed. |
| Timeout | Proceed with recovery utilizing partial results and backup files collected so far. |
| Data conflict between subagents | Do not overwrite data; preserve it by writing the sources side-by-side. |

## Test Scenarios

### Normal Flow
1. User provides {input}.
2. Phase 1 derives {analysis results}.
3. Phase 2 executes subagents sequentially.
4. Phase 3 integrates artifacts to generate final results.
5. Expected result: `{output-path}/{filename}` is created.

### Error Flow
1. Phase 2 aborts due to an error during {agent-2} execution.
2. The orchestrator performs a retry invocation once.
3. If the retry execution fails, mark that step as omitted.
4. Proceed with Phase 3 using the remaining results.
5. Explicitly state and preserve "Failed to collect {agent-2} task area" in the final report.
```

---

## Template B: Parallel Subagents Mode (Parallel)

Used when saving resources and increasing speed by running multiple independent tasks concurrently. Output paths and filenames must be clearly distinguished to avoid overlap.

```markdown
---
name: {domain}-orchestrator
description: "Orchestrator coordinating the {domain} subagent team in parallel. {Initial run keyword}. Includes follow-up task keywords."
---

## Execution Mode: Parallel Subagents

## Subagent Composition

| Agent TypeName | Role | Skill | Output |
|:---|:---|:---|:---|
| {agent-1} | {Role} | {skill} | `_workspace/02_{agent-1}.md` |
| {agent-2} | {Role} | {skill} | `_workspace/02_{agent-2}.md` |
| {agent-3} | {Role} | {skill} | `_workspace/02_{agent-3}.md` |

## Workflow

### Phase 0: Context Check
(Same as Template A — branching on the existence of `_workspace/`)

### Phase 1: Preparation
1. Analyze input
2. Create `_workspace/`

### Phase 2: Parallel Execution
Immediately call the `invoke_subagent` tool sequentially so that all subagents run in the background (specifying unique output paths for each):

1. Call `invoke_subagent` ({agent-1})
2. Call `invoke_subagent` ({agent-2})
3. Call `invoke_subagent` ({agent-3})
*Wait until all subagent completion notifications and responses are collected.*

### Phase 3: Integration
1. Verify completion reports from each subagent and collect results under `_workspace/` (Read)
2. Apply integration logic → Write final artifacts

### Phase 4: Cleanup
1. Preserve `_workspace/`
2. Report results summary

## Error Handling
- Single agent failure: Retry once. If it continues to fail, note the omission and proceed with the pipeline.
- Majority failure: Guide user operation and get confirmation.
- Output file path duplication prevention: Include TypeName in each agent's output filename to make it unique.
```

---

## Template C: Hybrid Mode

Design a workflow by mixing different execution patterns (sequential, parallel, direct execution) per Phase. Be sure to specify `**Execution Pattern:** {Sequential | Parallel | Direct}` at the top of each Phase.

```markdown
---
name: {domain}-orchestrator
description: "{domain} Orchestrator (Hybrid). {Keyword}. Includes follow-up task keywords."
---

## Execution Mode: Hybrid

| Phase | Pattern | Reason |
|:---|:---|:---|
| Phase 2 (Parallel collection) | Parallel Subagents | Optimize collection speed of multiple independent materials |
| Phase 3 (Consensus integration) | Direct Execution (Main integration) | Complex consistency and value judgment required |
| Phase 4 (Independent verification) | Sequential Subagents | Thorough post-analysis through specialized QA agent |

## Workflow

### Phase 2: Parallel Data Collection
**Execution Pattern:** Parallel Subagents

Run multiple subagents concurrently. Results are safely saved in split files at `_workspace/02_{agent}_raw.md`.

### Phase 3: Consensus-based Integration
**Execution Pattern:** Direct Execution

The main agent collects and reads all output files from Phase 2 to resolve conflicting details and reflect comprehensive consistency. Produces the final integrated version at `_workspace/03_integrated.md`.

### Phase 4: Independent Verification
**Execution Pattern:** Sequential Subagents

Run the QA subagent `invoke_subagent` to write an objective quality and consistency verification report on the `_workspace/03_integrated.md` received as input.
```

**Hybrid Transition Rules:**
- **Parallel → Sequential**: Wait fully for the task completion (message collection) of all background subagents currently running in parallel, then enter the sequential Phase.
- **Sequential → Parallel**: After completing the previous sequential step, share and deliver the saved final result path to the prompts of all parallel subagents to start concurrent tasks.
- **Subagent → Direct Execution**: The main orchestrator directly reads and processes the artifacts completed by the subagents using tools.

---

## Writing Principles

1. **Specify Execution Pattern**: Be sure to write which pattern ("Sequential Subagents", "Parallel Subagents", "Direct Execution", "Hybrid") is used at the top of the orchestrator skill.
2. **Delegate via `invoke_subagent` tool calls**: Define subagent invocation as executing the `invoke_subagent` tool of Antigravity CLI, not as text (e.g., @analyst).
3. **Specify Unique Absolute Paths**: Set unique filenames containing the agent name under `_workspace/` so that artifacts do not get tangled or overwritten between agents.
4. **Error Fallback Design**: Describe realistic execution failure and recovery measures in the error handling section.
5. **Mandatory Reflection of Follow-up Control Keywords in description**: Make sure to include keywords like "re-run, run again, modify, update, supplement" in the description so that the skill does not become a one-off artifact.
6. **Error handling must be realistic** — Do not assume "everything will succeed."
7. **Test Scenarios Mandatory** — Describe at least 1 normal flow + 1 error flow.

## Follow-up Task Keywords in description

Writing only initial run keywords in the orchestrator description is insufficient. Be sure to include the following follow-up task expressions:

- re-run/run again/update/modify/supplement
- "only {part} of {domain} again"
- "based on previous results", "improve results"
- Common daily requests related to the domain (e.g., if it is a launch strategy harness, "launch", "promotion", "trending", etc.)

Without follow-up keywords, the harness is virtually dead after the first run.

## Actual Orchestrator Reference

Basic structure of orchestrator in parallel subagents pattern:
Preparation → Phase 0 (Context Check) → Parallel invoke_subagent calls → Result collection → Integration → Cleanup.
Refer to the research team example in `rules/team-examples.md`.
