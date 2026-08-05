---
description: Implementation of accelerated mouse scroll logic and global focus capture for Go TUI applications using tview. MUST use this whenever creating or refactoring a tview-based dashboard to ensure professional UX.
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: TView Accelerated Scroll & Focus

# Tview Scroll & Focus Best Practices

To achieve "flawless" scroll and focus behavior in `tview` applications, follow these patterns.

## 1. Accelerated Mouse Scroll
Default `tview` scrolling is slow (1 line per event). Use **Key Injection** to move selection and scroll simultaneously for `List`, `TextView`, and `TextArea`.

```go
// ScrollAccelerate injects multiple Up/Down keys into the primitive's InputHandler.
func ScrollAccelerate(primitive tview.Primitive, speed int) func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
	return func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseScrollUp {
			for i := 0; i < speed; i++ {
				primitive.InputHandler()(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), nil)
			}
			return action, nil // Consume event
		}
		if action == tview.MouseScrollDown {
			for i := 0; i < speed; i++ {
				primitive.InputHandler()(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), nil)
			}
			return action, nil // Consume event
		}
		return action, event
	}
}
```

## 2. Table Specific Scroll
For `tview.Table`, direct offset manipulation is faster and prevents selection jitters.

```go
func ScrollAccelerateTable(table *tview.Table, speed int) func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
	return func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		row, col := table.GetOffset()
		if action == tview.MouseScrollUp {
			newRow := row - speed
			if newRow < 0 { newRow = 0 }
			table.SetOffset(newRow, col)
			return action, nil
		}
		if action == tview.MouseScrollDown {
			table.SetOffset(row+speed, col)
			return action, nil
		}
		return action, event
	}
}
```

## 3. Global Focus & Scroll Integration
The best user experience comes from capturing mouse events at the **Application** level. This allows for "Click to Focus" and "Scroll to Focus" across all panels.

```go
app.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	x, y := event.Position()
	panels := []tview.Primitive{list1, list2, form1}
	
	var hoveredPanel tview.Primitive
	for _, p := range panels {
		px, py, pw, ph := p.GetRect()
		if x >= px && x < px+pw && y >= py && y < py+ph {
			hoveredPanel = p
			break
		}
	}

	if hoveredPanel != nil {
		// 1. Click to Focus
		if action == tview.MouseLeftClick {
			app.SetFocus(hoveredPanel)
		}

		// 2. Accelerated Scroll with Focus
		if action == tview.MouseScrollUp || action == tview.MouseScrollDown {
			app.SetFocus(hoveredPanel) // Focus first to ensure selection follows scroll
			a, e := ScrollAccelerate(hoveredPanel, 5)(action, event)
			return e, a
		}
	}
	return event, action
})
```

## Key Rules
- **Return order**: `app.SetMouseCapture` returns `(*tcell.EventMouse, tview.MouseAction)`, whereas `primitive.SetMouseCapture` returns `(tview.MouseAction, *tcell.EventMouse)`.
- **Consumption**: Return `nil` for the event to stop `tview` from performing its own internal (slow) scroll.
- **Focus**: Always `app.SetFocus(hoveredPanel)` during scroll events so the user doesn't have to click before scrolling.


---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=tview-scroll`
- `skill id=frontend-go`