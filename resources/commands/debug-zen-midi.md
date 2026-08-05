---
description: Diagnose visual or process issues in zen-midi
---
I am seeing issues with zen-midi. Please investigate:

1. **Process Check**: Use `processManager` to check if 'zen-midi' is running and what its uptime is.
2. **Visual Capture**:
   - If running: Use `screenshotCapture` (mode: 'window' or 'selection') to capture the current state.
   - Or use `videoRecorder` for a 5-second clip if it's an animation issue.
3. **Log Analysis**: Use `logViewer` to tail the last 50 lines of the zen-midi log file.
4. **Report**: Summarize the findings.

---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=musictheory`
- `skill id=visual-analyze`