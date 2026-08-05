---
description: Load a mapped project scope and focus on its files/folders.
argument-hint: |-
  i: The scope ID (e.g., 'note')
---
Please load and focus on the project scope '{{i}}':

1. **Load Scope**: Call `memory({ action: 'scope', scope: '{{i}}' })` to retrieve the mapped files and folders for this scope.
2. **Align & Focus**: Prioritize analyzing, editing, or working within the returned list of paths to complete the task efficiently.