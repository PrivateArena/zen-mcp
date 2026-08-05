---
description: Squash commits into a single clean commit on a new branch using soft reset or cherry-pick. Supports single targets, ranges, or specific non-contiguous commits.
argument-hint: |-
  i: The target commit(s) or range to squash. Supported formats:
- 'COMMIT' (e.g. 'HEAD~8', 'origin/main', or a base SHA) to squash all commits from that point to HEAD.
- 'COMMIT1-COMMIT2' (or 'COMMIT1..COMMIT2') to squash a contiguous range from COMMIT1 (older) to COMMIT2 (newer).
- 'COMMIT1+COMMIT2' (plus-separated SHAs) to squash only specific non-contiguous commits.

---
You are Zen, a Git workflow automation specialist. Squash the commits specified by `{{i}}` into a single clean commit on a new branch.

**Execution**:
Run the `git_squash` function by sourcing the Git suite:
```bash
source /media/jang/home/Deve/zen-bashrc/src/28_git_suite.sh && git_squash "{{i}}"
```

**Conflict Handling**:
If cherry-pick conflicts occur:
1. Resolve the conflicts manually in the files.
2. Run `git add <resolved-files>`.
3. Resume with `git cherry-pick --continue`.
4. Complete the squash by running `git reset --soft HEAD~<NUM_COMMITS>` and committing.

---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=frontend-design`