# Agent Team Design Patterns (Antigravity CLI)

## Execution Mode: Subagents

Antigravity CLI implements multi-agent workflows through the `invoke_subagent` tool. The main agent acts as the orchestrator, and each subagent performs tasks with an independent context and toolset.

### Subagents — Default Mode

The main agent calls the `invoke_subagent` tool to invoke a subagent (specifying TypeName). Each subagent runs in a separate isolated context window, and returns the final state and message to the orchestrator upon task completion.

```
[Main/Orchestrator]
    ├── invoke_subagent(TypeName: "agent-a") → Perform task → Return results
    ├── invoke_subagent(TypeName: "agent-b") → Perform task → Return results
    ├── invoke_subagent(TypeName: "agent-c") → Perform task → Return results
```

**Key Features:**

- Subagents are defined in JSON format in the file `.agents/plugins/{plugin-name}/agents/{name}/agent.json`.
- Perform explicit delegation calls using the `invoke_subagent` tool call.
- Each subagent has an independent context, tools, and MCP servers.
- Additional information or feedback can be transmitted between the running subagent and the orchestrator using the `send_message` tool.
- Results are returned to the main context in the form of a summary and completion report.

**Constraints:**

- Direct communication between subagents is not possible (orchestration only through the main agent).
- The main agent is responsible for all workflow control.
- Avoid concurrent write conflicts on the same file (during parallel execution).

### Parallel Subagents

Useful when you want to process multiple independent tasks concurrently. Call the `invoke_subagent` tool multiple times sequentially to run them as background tasks, and wait for and collect the task results of all subagents.

```
[Main]
    ├── invoke_subagent(TypeName: "agent-a") (Parallel Background)
    ├── invoke_subagent(TypeName: "agent-b") (Parallel Background)
    └── invoke_subagent(TypeName: "agent-c") (Parallel Background)
          └── Integrate results after all agents complete
```

**Suitable Case:** When independence between tasks is completely guaranteed, and you only need to merge the final results.
**Caution:** To prevent concurrent write conflicts on the same file, each subagent must write to a distinct, unique output path (e.g., under `_workspace/`).

### Direct Execution

For simple tasks, the main agent directly processes the task using tools (`view_file`, `write_to_file`, etc.) without creating a separate subagent plugin.

```
[Main]
    └── Direct execution (Read/Write/Shell, etc.)
```

**Suitable Case:** Simple exploration, small-scale text editing, etc., when the overhead associated with agent separation and switching is larger.

### Mode Selection Decision Tree

```
Are there 2 or more specialized areas?
├── Yes → Can tasks be executed independently?
│         ├── Yes → Parallel Subagents
│         │         Run independently for each area, integrate results only.
│         │
│         └── No → Sequential Subagents (Pipeline)
│                  Output of the previous agent is needed as the next input.
│
└── No (1) → Direct Execution
              Single tasks do not require subagent separation.
```

> **Core Principle:** `invoke_subagent` is the default. When choosing direct execution, ask yourself, "Does this task really not require an independent agent persona or isolated context?"

---

## Team Architecture Types

### 1. Pipeline

Sequential workflow. The output of the previous subagent is passed as the input of the next subagent.

```
[analyst] ─(Output file)─→ [designer] ─(Output file)─→ [builder] ─→ [verifier]
```

**Suitable Case:** When each step strongly depends on the output of the previous step.
**Example:** Document/content creation — Information research and analysis → Layout design → Draft writing → Quality verification.
**Caution:** Since a bottleneck or failure of a preceding agent delays the entire pipeline, the retry mechanism upon error must be clarified.
**Implementation:** File-based data delivery — Each subagent saves the result file to `_workspace/`, and the orchestrator instructs the next subagent to read the absolute path of that file in the invocation prompt.

### 2. Fan-out/Fan-in

Perform independent tasks concurrently and then integrate the results.

```
                    ┌→ [expert-a] ─┐
[Main/Orchestrator] ─┼→ [expert-b] ─┼→ [Integrate results]
                    └→ [expert-c] ─┘
```

**Suitable Case:** When concurrent analysis from different expert perspectives or processing of large split data is required for the same input.
**Example:** Comprehensive research — Security review, performance modeling, and legal compliance reviews are conducted in parallel by respective agents, and then collected into a single report.
**Caution:** Map each subagent's output path to a unique filename (`02_expert_a_result.md`, etc.) to prevent concurrent write conflicts.

### 3. Expert Pool

Select and invoke the appropriate agent based on situations or conditions.

```
[Main/Router] ──(Classification/Condition check)──→ { analyst-agent | builder-agent | qa-agent }
```

**Suitable Case:** When different instructions and toolsets are required depending on the nature or technical field of the input request.
**Example:** Code review — Depending on the extension or code properties of the modified file, selectively call only the necessary experts among security experts, performance optimization experts, or database experts.

### 4. Producer-Reviewer

The generation subagent and verification subagent collaborate as a pair.

```
[producer-agent] ──(Draft)──→ [reviewer-agent] ──(Feedback/Verification)
        ▲                             │
        └─────── (Re-run on error) ───┘
```

**Suitable Case:** When output accuracy or code quality is strictly required, and mechanical or qualitative verification criteria are clear.
**Caution:** To prevent falling into an infinite loop, the maximum number of retries and loops (maximum 2~3 times) must be limited at the orchestrator level.

### 5. Supervisor

The main agent manages the overall status and task distribution list, and dynamically delegates tasks to subagents.

```
                  ┌→ [worker-a]
[supervisor-agent] ┼→ [worker-b]  (Dynamic distribution based on progress/state)
                  └→ [worker-c]
```

**Suitable Case:** When the list of target files or tasks changes dynamically at runtime, or when the total volume of processing is massive so that incremental split processing is advantageous.

### 6. Hierarchical Delegation

An upper subagent performs recursive delegation to lower subagents using the `invoke_subagent` tool call to hierarchically handle complex overall problems.

```
[main] → [team-lead-agent] ──→ [worker-agent-1]
                           └──→ [worker-agent-2]
```

**Caution:** If the delegation depth exceeds 3 levels, intermediate context is omitted, leading to exponential increases in latency and context loss. Delegation within a maximum of 2 levels is strongly recommended.

---

## Subagent Definition Format (Antigravity CLI)

In Antigravity CLI, a subagent is defined in JSON format in the file `.agents/plugins/{plugin-name}/agents/{agent-name}/agent.json`.

> [!NOTE]
> **Multilingual and English Writing Rules**
> If the user has requested the harness configuration in English or if the project's default language is English, all instruction texts (core roles, task principles, input/output protocols, etc.) within `description` and `systemPromptSections` must be translated and written in natural **Technical English**. Do not leave Korean comments or texts.

### `agent.json` Definition Structure Example

```json
{
  "name": "agent-name",
  "description": "Summary of the subagent's role and explanation of its intended use.",
  "hidden": false,
  "config": {
    "customAgent": {
      "systemPromptSections": [
        {
          "title": "Agent System Instructions",
          "content": "You are the specialized subagent '[TypeName]' for [Domain].\n\n## Core Roles\n1. [Core Role 1]\n2. [Core Role 2]\n\n## Work Principles\n- [Work Principle 1]\n- [Work Principle 2]\n\n## Input/Output Protocol\n- Input: [File path and context inherited from the orchestrator]\n- Output: [Path and structure to write the artifacts upon task completion]\n\n## Error Handling\n- If a tool failure or omission is detected, try to recover once on your own. If it is unresolvable, print the cause message and stop execution.\n\n## Collaboration Protocol\n- If the result file of the previous step exists under `_workspace/`, load and utilize it. If additional feedback is provided, prioritize reflecting it to update the result file."
        }
      ],
      "toolNames": [
        "send_message",
        "find_by_name",
        "grep_search",
        "view_file",
        "list_dir",
        "read_url_content",
        "search_web",
        "schedule",
        "multi_replace_file_content",
        "replace_file_content",
        "write_to_file",
        "run_command",
        "manage_task",
        "define_subagent",
        "invoke_subagent",
        "manage_subagents",
        "call_mcp_tool"
      ],
      "systemPromptConfig": {
        "includeSections": [
          "user_information",
          "mcp_servers",
          "skills",
          "subagent_reminder",
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

## Agent Separation Criteria

| Criteria | Separate | Integrate |
| ------------ | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| **Expertise** | When domain task areas (e.g., planning vs building vs QA) are completely different | When task knowledge and prompts are similar or closely related |
| **Parallelism** | When multiple independent files need to be processed in parallel | When tasks have close sequential dependencies where subsequent steps can run only after preceding ones finish |
| **Context** | When saving context is required because the file size to load for a specific analysis is massive | When contexts between agents must be dynamically and highly coupled and shared |
| **Reusability** | When there is a possibility of being called independently in various scenarios or other plugin workflows | When limited strictly to a special supporting role of a specific orchestrator |

---

## Subagent Reuse Design

Before creating a new subagent, check for duplication with existing subagents. Repeatedly building harnesses can easily lead to accumulating subagents with overlapping roles under different names.

| Situation | Action |
|------|------|
| Existing subagent completely covers the new role | Do not create new — reuse existing subagent |
| Existing subagent partially covers the role and is generalizable | Generalize and expand the existing subagent |
| Domain specialization is intended for partial coverage | Proceed with new creation — maintain as separate subagent |
| Role scope is completely different | Proceed with new creation |

**Principle:** The more a single subagent focuses on a single role, the higher its reusability and the less duplication. If a role has two or more aspects, check first if it can be separated.

**When generalizing existing subagents:** The behavior of orchestrators and plugin configurations that depend on that subagent may change. Check dependencies in `project/.agents/plugins/*/agents/` before expanding, and confirm that existing operations are maintained with a dry run after generalization.

---

## Skill vs Agent Distinction

| Category | Skill | Subagent (Agent) |
| ---------- | ------------------------------------------------------------- | ------------------------------------------------------------------ |
| **Definition** | Procedural knowledge and specific action workflow guidelines | Expert entity with work principles, persona, and specific tool execution capabilities |
| **Location** | `.agents/plugins/{plugin-name}/skills/{skill-name}/SKILL.md` | `.agents/plugins/{plugin-name}/agents/{agent-name}/agent.json` |
| **Trigger** | Automatic matching based on description analysis and explicit skill activation | Explicit delegation execution via `invoke_subagent` tool |
| **Use Case** | **"How"** to perform tasks efficiently | **"Who"** will perform the specialized task in an independent context |
