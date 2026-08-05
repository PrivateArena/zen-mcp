---
description: Synchronize the MCP workspace root with the CLI client's current directory.
---
Synchronize the MCP workspace root by identifying the current working directory from our session context and calling `workspace({ path: '/path/to/project' })` immediately. Confirm the new root path back once it's set.