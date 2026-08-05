---
description: Build production-grade Go frontends. TUI with tview, GUI with gio. Both achieve 0% idle CPU via event-driven repaint, unlike bubbletea/fyne. Covers setup, patterns, widget reference, performance tips, and common pitfalls.
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: Frontend Go — TUI (tview) & GUI (gio)

---
name: frontend-go
description: Build production-grade Go frontends — TUI with tview, GUI with gio (gioui.org). Both achieve 0% idle CPU, unlike bubbletea/tcell (TUI) or fyne/andlabs (GUI) which burn continuous CPU in idle loops. Use this skill when asked to create terminal UI or native desktop applications in Go.
license: Complete terms in LICENSE.txt
---

This skill guides creation of Go frontends that are resource-efficient, responsive, and production-grade. The guiding principle: **0% idle CPU**. Every watt goes to actual work, not polling loops.

The user provides requirements for a Go-based frontend — terminal UI or native desktop GUI.

## Zero-Idle Architecture

Both `tview` (TUI) and `gio` (GUI) share a fundamental architectural advantage: they use **event-driven repaint** rather than frame-loop polling.

| Library | Domain | Idle CPU | Repaint Trigger |
|---------|--------|----------|-----------------|
| **tview** | TUI | 0% | On input/event only |
| bubbletea | TUI | 2-8% | Frame-tick loop |
| **gio** | GUI | 0% | On input/event/invalidate only |
| fyne | GUI | 3-15% | Continuous 60fps loop |

This means tview/gio applications use zero CPU when sitting idle — critical for background daemons, monitoring tools, and battery-sensitive environments.

## TUI: tview

[tview](https://github.com/rivo/tview) builds on tcell but adds high-level widgets (form, table, tree, list, textview, flex grid) and **only repaints on actual events**.

### Primitives

- `tview.Box` — base with borders, focus, input handling
- `tview.Flex` / `tview.Grid` — layout containers
- `tview.List` — interactive list with keyboard nav
- `tview.TableView` — scrollable spreadsheet-style grid
- `tview.TreeView` — collapsible tree with nodes
- `tview.TextView` — scrollable, optionally interactive text
- `tview.InputField` — single-line text input
- `tview.TextArea` — multi-line text input
- `tview.Form` — form with labeled fields
- `tview.Modal` — centered dialog overlay
- `tview.Pages` — stack-based screen navigation
- `tview.Table` — simple table (legacy; prefer TableView)

### Application Setup

```go
package main

import (
    "github.com/rivo/tview"
)

func main() {
    app := tview.NewApplication()
    flex := tview.NewFlex().
        AddItem(tview.NewTextView().SetText("left"), 20, 1, false).
        AddItem(tview.NewTextView().SetText("right"), 0, 1, true)

    flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
        if event.Key() == tcell.KeyCtrlC {
            app.Stop()
        }
        return event
    })

    if err := app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
        panic(err)
    }
}
```

### Patterns

- **No global state**: Pass app reference or use closure captures
- **Flex over Grid**: `Flex` is simpler; use `Grid` only for complex layouts
- **Primitive references**: Store references to dynamically-updated primitives (e.g., `TextView`) for live updates
- **Goroutine safety**: Use `app.QueueUpdateDraw(fn)` to mutate primitives from goroutines
- **Input capture**: Prefer `SetInputCapture` on the root for global hotkeys
- **Modal confirmations**: Use `tview.NewModal()` with buttons — it blocks input to underlying primitives automatically

### Performance Tips

- Avoid re-creating primitives; reuse via `SetText()`, `SetCells()`, etc.
- `TextView.SetDynamicColors(true)` enables inline color tags without overhead
- `TableView` paginates large datasets internally — no manual virtual scrolling needed
- For real-time streaming logs, use `TextView` with `SetMaxLines(1000)` to bound memory

## GUI: gio (gioui.org)

[gio](https://gioui.org) is a portable immediate-mode GUI library that renders via OpenGL (desktop) and OpenGL ES (mobile). Its **event-driven architecture** means `FrameEvent` fires only when input arrives or `invalidate` is called. At rest: 0% CPU.

### Core Concepts

- **Ops tree**: Each frame builds a declarative ops tree (draw ops, input ops) — no retained widget tree
- **Context (`layout.Context`)**: Carries theme, constraints, and ops queue through the frame
- **Widgets**: Pure functions that take `layout.Context` and return `layout.Context` + dimensions
- **Events**: Handled via `event.Querier` interface per registered key/focus area

### Minimal Application

```go
package main

import (
    "gioui.org/app"
    "gioui.org/io/system"
    "gioui.org/layout"
    "gioui.org/op"
    "gioui.org/widget/material"
)

func main() {
    go func() {
        w := app.NewWindow()
        th := material.NewTheme()
        var ops op.Ops

        for e := range w.Events() {
            if e, ok := e.(system.FrameEvent); ok {
                gt := layout.NewContext(&ops, e)
                material.H6(th, "Hello, gio").Layout(gt)
                e.Frame(gt.Ops)
            }
        }
    }()
    app.Main()
}
```

### Widget Design

Widgets are **stateless functions** operating on the ops tree:

```go
func customButton(gt layout.Context, th *material.Theme, text string) layout.Dimensions {
    return material.Button(th, &clickable, text).Layout(gt)
}
```

- State (clickable, editor, slider) lives outside the widget and is passed in
- To invalidate (request next frame): call `op.InvalidateOp{}.Add(gt.Ops)` — the only way to wake the render loop

### Layout Strategies

- `layout.Flex` — horizontal/vertical distribution (analogous to CSS flexbox)
- `layout.List` — virtual-scrolling list (only renders visible items)
- `layout.Stack` — overlapping layers (like CSS absolute positioning)
- `layout.Inset` — padding/margin
- `layout.Direction` — alignment (start, end, center)

### Data Binding & Refresh

Since gio is immediate-mode, there is no reactivity system. Refresh the frame when data changes:

```go
// In event loop:
case dataUpdate := <-dataCh:
    data = dataUpdate
    w.Invalidate() // request next frame
```

Use `w.Invalidate()` from any goroutine to wake the render loop (it is thread-safe).

### Performance Tips

- **Reuse ops**: Declare `op.Ops` once and reuse each frame — zero allocation per frame after warm-up
- **Clip bounds**: `clip.Rect` operations reduce GPU overdraw
- **List recycling**: `layout.List` recycles widgets; set `layout.ListElement.Size` for fixed-size items to skip measurement
- **Texture atlas**: Gio batches similar images into GPU atlases automatically
- **Profiling**: Use `gioui.org/app/debug` for frame timing

## Choice Guideline

| Criterion | tview | gio |
|-----------|-------|-----|
| Terminal-only (SSH, tmux) | ✅ | ❌ |
| Native desktop (cross-platform) | ❌ | ✅ |
| Mobile (Android/iOS) | ❌ | ✅ |
| Binary size target | ~5 MB | ~15 MB |
| Learning curve | Low | Medium-High |
| Widget richness | High | Medium |
| Custom drawing | Limited (tcell primitives) | Full GPU rendering |
| Accessibility | Terminal-native (screen readers OK) | Limited |

## Common Pitfalls

### tview
- **tview.Application is not goroutine-safe**. Always use `QueueUpdateDraw` for mutations from background goroutines.
- **Mouse must be explicitly enabled** via `.EnableMouse(true)` on the Application. Without it, click events are ignored.
- **InputField/TextArea onChange** fires on every keystroke — debounce if triggering I/O.
- **Flex item weights**: Sum of proportions should be 0 (fixed) or the total flex weight. Mixing fixed and flex items needs explicit `FlexItem` config.

### gio
- **Event loop blocks**: `window.Events()` blocks — must run in a goroutine
- **No retained tree**: Every frame rebuilds the entire widget tree from scratch. If a widget's state isn't stored externally, it resets each frame.
- **Invalidate discipline**: Only call `w.Invalidate()` when the visual state actually changes. Calling it every frame defeats the zero-idle architecture.
- **Gio is not thread-safe**: Widget updates/state reads must happen in the event loop goroutine. External data flows through channels.
- **Font registration**: No bundled fonts by default. Use `font.Register` with embedded TTF data or use `material.NewTheme()` which bundles the Go fonts.

## Integration With Existing Go Projects

Both libraries integrate cleanly without framework lock-in:

```go
// tview — embed in any Go binary
func StartTUI(dataSource DataProvider) error {
    app := tview.NewApplication()
    // wire up dataSource to primitives
    return app.Run()
}

// gio — embed in any Go binary
func StartGUI(dataSource DataProvider) {
    go func() {
        w := app.NewWindow()
        // wire up dataSource to event loop
        for e := range w.Events() { /* ... */ }
    }()
    app.Main()
}
```

No init hooks, no global registries, no reflection magic. Standard Go patterns throughout.

**Remember**: Zero idle CPU is not a feature — it's the baseline. Any polling loop, frame timer, or goroutine spin is a defect.


---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=frontend-go`
- `skill id=tview-scroll`
- `skill id=frontend-design`