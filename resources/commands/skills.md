---
description: Activate a skill by ID — content is injected directly into this prompt with zero tool calls. Usage: /mcp:skills <id>
argument-hint: |-
  i: The skill ID (e.g., zen-mcp, firefox-bridge, tview-scroll)
---
Please use MCP Tool `skill id="{{i}}"` to retrieve it manually.