# Agent Team Examples (Antigravity CLI)

---

## Example 1: Research Team (Parallel Subagents Mode)

### Team Architecture: Fan-out/Fan-in
### Execution Mode: Parallel Subagents

```
[Orchestrator/Main]
    ├── invoke_subagent(TypeName: "official-researcher")   → research_official.md
    ├── invoke_subagent(TypeName: "media-researcher")      → research_media.md
    ├── invoke_subagent(TypeName: "community-researcher")  → research_community.md
    ├── invoke_subagent(TypeName: "background-researcher") → research_background.md
    └── Integration → ConsolidatedReport.md
```

### Subagent Composition

| Agent TypeName | Role | Output |
|:---|:---|:---|
| `official-researcher` | Official docs/blogs | `_workspace/02_official.md` |
| `media-researcher` | Media/Investment | `_workspace/02_media.md` |
| `community-researcher` | Community/SNS | `_workspace/02_community.md` |
| `background-researcher` | Background/Competition/Academics | `_workspace/02_background.md` |
| (Orchestrator/Main) | Consolidated Report | `ConsolidatedReport.md` |

> Research subagents are each defined in `.agents/plugins/research-plugin/agents/{name}/agent.json`. The files specify roles, search scopes, and output formats to ensure reusability and consistency in results.

### Orchestrator Workflow (Parallel Subagents)

```
Phase 1: Preparation
  - Analyze user input (grasp topic, research mode)
  - Create _workspace/
  - Save input data to _workspace/00_input/

Phase 2: Parallel Investigation
  - Run 4 subagents concurrently in the background (invoke_subagent):
    TypeName: "official-researcher"   → _workspace/02_official.md
    TypeName: "media-researcher"      → _workspace/02_media.md
    TypeName: "community-researcher"  → _workspace/02_community.md
    TypeName: "background-researcher" → _workspace/02_background.md
  - Wait for all agent completion notifications

Phase 3: Integration
  - Main reads 4 outputs
  - Generate consolidated report
  - Attended with sources for conflicting information

Phase 4: Cleanup
  - Preserve _workspace/ (for post-verification and audit trails)
```

---

## Example 2: Sci-Fi Novel Writing Team (Sequential + Parallel Subagents)

### Team Architecture: Pipeline + Fan-out
### Execution Mode: Subagents (Sequential + Parallel Mixed)

```
Phase 1 (Parallel): invoke_subagent(worldbuilder) + invoke_subagent(character-designer) + invoke_subagent(plot-architect)
  → Independently generate world setting/characters/plot
Phase 2 (Sequential): invoke_subagent(prose-stylist) (writing) — Input all Phase 1 results
Phase 3 (Parallel): invoke_subagent(science-consultant) + invoke_subagent(continuity-manager) (reviews)
Phase 4 (Sequential): invoke_subagent(prose-stylist) (apply review comments & modify)
```

### Subagent Composition

| Agent TypeName | Role | Skill |
|:---|:---|:---|
| `worldbuilder` | Construct world settings | world-setting |
| `character-designer` | Design characters | character-profile |
| `plot-architect` | Outline plot structure | outline |
| `prose-stylist` | Style editing + writing | write-scene, review-chapter |
| `science-consultant` | Science verification | science-check |
| `continuity-manager` | Consistency verification | consistency-check |

### Subagent File Example: `worldbuilder/agent.json`

```json
{
  "name": "worldbuilder",
  "description": "An expert constructing the world settings of sci-fi novels. Designs physical laws, social structures, technology levels, and history.",
  "hidden": false,
  "config": {
    "customAgent": {
      "systemPromptSections": [
        {
          "title": "Agent System Instructions",
          "content": "You are 'worldbuilder', an expert in designing sci-fi novel world settings. Based on scientific facts but extending imagination, you build the physical, social, and technological foundations of the world where stories unfold.\n\n## Core Roles\n1. Define physical laws and tech levels of the world\n2. Design social structures, political frameworks, and economic systems\n3. Establish historical contexts and current conflict structures\n4. Describe the environment and atmosphere of each location\n\n## Work Principles\n- Internal consistency is highest priority — no contradictions between settings\n- Infer ripple effects on the world through chain questions like \"What if this tech exists?\"\n- A world setting in service of the story — avoid excessive details that hinder the plot\n\n## Input/Output Protocol\n- Input: User's world concept, genre requirements\n- Output: `_workspace/01_worldbuilder_setting.md`\n- Format: Markdown. Categorized by sections (physical/social/technological/historical/locational)\n\n## Error Handling\n- If the concept is ambiguous, propose 3 options and ask to choose\n- If scientific errors are found, propose alternatives alongside\n\n## Collaboration Protocol\n- Prioritize consistency since character-designer and plot-architect will refer to your outputs.\n- Modify settings according to science-consultant's feedback when collected."
        }
      ],
      "toolNames": [
        "view_file",
        "write_to_file",
        "replace_file_content"
      ],
      "systemPromptConfig": {
        "includeSections": [
          "user_information",
          "skills",
          "messaging",
          "artifacts"
        ]
      }
    }
  }
}
```

### Workflow Details

```
Phase 1: Call @worldbuilder, @character-designer, and @plot-architect in parallel
         → Save to _workspace/01_world.md, _workspace/01_characters.md, and _workspace/01_plot.md, respectively
Phase 2: Call @prose-stylist (providing 3 outputs of Phase 1 as input)
         → Save to _workspace/02_prose_draft.md
Phase 3: Call @science-consultant and @continuity-manager in parallel
         → Save to _workspace/03_science_review.md and _workspace/03_continuity_review.md, respectively
Phase 4: Re-call @prose-stylist (reflecting review results)
         → Save final manuscript
```

---

## Example 3: Webtoon Production Team (Sequential Subagents — Producer-Reviewer Loop)

### Team Architecture: Producer-Reviewer
### Execution Mode: Sequential Subagents

```
Loop (Max 2 times):
Phase 1: invoke_subagent(webtoon-artist) → Generate panel images
Phase 2: invoke_subagent(webtoon-reviewer) → Quality review
Phase 3: (If REDO occurs) Re-call invoke_subagent(webtoon-artist) → Regenerate problematic panels
```

### Subagent Composition

| Agent TypeName | Role | Skill |
|:---|:---|:---|
| `webtoon-artist` | Generate panel images | generate-webtoon |
| `webtoon-reviewer` | Quality review | review-webtoon |

### Subagent File Example: `webtoon-reviewer/agent.json`

```json
{
  "name": "webtoon-reviewer",
  "description": "An expert reviewing the quality of webtoon panels. Evaluates composition, character consistency, text legibility, and direction.",
  "hidden": false,
  "config": {
    "customAgent": {
      "systemPromptSections": [
        {
          "title": "Agent System Instructions",
          "content": "You are 'webtoon-reviewer', an expert in reviewing the quality of webtoon panels. You evaluate panels based on visual quality, story delivery, and character consistency.\n\n## Core Roles\n1. Evaluate composition and visual quality of each panel\n2. Verify consistency of character appearances across panels\n3. Evaluate legibility and placement of speech bubble texts\n4. Review direction flow and pacing of the overall episode\n\n## Work Principles\n- Clearly determine outcome in 3 stages: PASS / FIX / REDO\n- FIX is when it is resolvable with partial modifications, REDO requires complete regeneration\n- Judge based on objective criteria (consistency, legibility, composition) rather than subjective taste\n\n## Input/Output Protocol\n- Input: Panel files in the directory `_workspace/panels/`\n- Output: `_workspace/review_report.md`\n- Format:\n  ```\n  ## Panel {N}\n  - Verdict: PASS | FIX | REDO\n  - Reason: [Specific Reason]\n  - Modification Directives: [Specific modification directions in case of FIX/REDO]\n  ```\n\n## Error Handling\n- If file load fails, judge that panel as REDO\n- Panels that are still REDO after 2 regenerations are treated as PASS with warnings\n\n## Collaboration Protocol\n- Deliver modification directives to webtoon-artist (based on result files)\n- Re-review regenerated panels (max 2 loops)"
        }
      ],
      "toolNames": [
        "view_file",
        "write_to_file",
        "replace_file_content",
        "list_dir"
      ],
      "systemPromptConfig": {
        "includeSections": [
          "user_information",
          "skills",
          "messaging",
          "artifacts"
        ]
      }
    }
  }
}
```

### Error Handling

```
Retry Policy:
- REDO panel → Request webtoon-artist to regenerate (including specific modification directives)
- Forced PASS after max 2 loops
- If more than 50% of the entire panels are REDO, propose prompt modifications to the user
```

---

## Example 4: Code Review Team (Parallel Subagents Mode)

### Team Architecture: Fan-out/Fan-in
### Execution Mode: Parallel Subagents

```
[Main] → invoke_subagent(security-reviewer): Check security vulnerabilities
       → invoke_subagent(performance-reviewer): Analyze performance impact
       → invoke_subagent(test-reviewer): Verify test coverage
       → Main integrates all results
```

### Subagent Composition

| Agent TypeName | Role | Output |
|:---|:---|:---|
| `security-reviewer` | Check security vulnerabilities | `_workspace/02_security.md` |
| `performance-reviewer` | Analyze performance impact | `_workspace/02_performance.md` |
| `test-reviewer` | Verify test coverage | `_workspace/02_test.md` |
| (Main) | Synthesize results | `Review_ConsolidatedReport.md` |

Each subagent independently performs analysis and saves the results in files. The main agent reads all results to generate a consolidated review report.

---

## Example 5: Supervisor Pattern — Code Migration Team (Mixed Mode)

### Team Architecture: Supervisor
### Execution Mode: Sequential + Parallel Subagents Mixed

```
[Main/Supervisor]
    1. Analyze file list (Direct Execution — Grep/Glob)
    2. Allocate batch (Direct Execution — Main splits)
    3. invoke_subagent(migrator-1) (batch A) Parallel
    4. invoke_subagent(migrator-2) (batch B) Parallel
    5. invoke_subagent(migrator-3) (batch C) Parallel
    6. Collect and integrate results (Direct Execution)
```

### Subagent Composition

| Step | Execution Subject | Role |
|:---|:---|:---|
| Analysis | Main (Direct) | Collect file list, estimate complexity |
| Distribution | Main (Direct) | Split and allocate batches |
| Migration | migrator-1~3 (Parallel) | Migrate allocated file batches |
| Integration | Main (Direct) | Integration testing and report outcomes |

### Dynamic Distribution Logic of Supervisor

```
1. Main collects all target file lists (Glob/Grep tools)
2. Estimate complexity (file size, import counts, dependencies)
3. Split batches evenly
4. Delegate each batch to migrators in parallel (invoke_subagent calls)
5. Wait for all subagent completion reports
   - Success → Next step
   - Failure → Retry once; omit the batch if it fails again
6. All tasks complete → Main runs integration tests
```

---

## Artifact Patterns Summary

### Plugin Definition File
Location: `.agents/plugins/{domain}-plugin/plugin.json`

### Subagent Definition File
Location: `.agents/plugins/{domain}-plugin/agents/{agent-name}/agent.json`

### Skill File Structure
Location: `.agents/plugins/{domain}-plugin/skills/{skill-name}/SKILL.md`

### Integrated Skill (Orchestrator)
High-level skill that coordinates the entire team. Defines subagent composition and workflows by scenarios.
Template: Refer to `rules/orchestrator-template.md`.
**Must specify execution pattern** — Choose among Sequential Subagents (default), Parallel Subagents, Direct Execution, or Hybrid.
