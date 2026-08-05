---
description: Build Go frontends — TUI with tview, GUI with gio. Zero-idle CPU architecture. Use when the task involves terminal UI, native desktop, or cross-platform GUI in Go.
argument-hint: |-
  i: Frontend requirements — component, screen, layout, or interaction to build
---
# Skill: Frontend Design Skill

---
name: frontend-design
description: Create distinctive, production-grade frontend interfaces with high design quality. Use this skill when the user asks to build web components, pages, artifacts, posters, or applications (examples include websites, landing pages, dashboards, React components, HTML/CSS layouts, or when styling/beautifying any web UI). Generates creative, polished code and UI design that avoids generic AI aesthetics.
license: Complete terms in LICENSE.txt
---

This skill guides creation of distinctive, production-grade frontend interfaces that avoid generic "AI slop" aesthetics. Implement real working code with exceptional attention to aesthetic details and creative choices.

The user provides frontend requirements: a component, page, application, or interface to build. They may include context about the purpose, audience, or technical constraints.

## Design Thinking

Before coding, understand the context and commit to a BOLD aesthetic direction:
- **Purpose**: What problem does this interface solve? Who uses it?
- **Tone**: Pick an extreme: brutally minimal, maximalist chaos, retro-futuristic, organic/natural, luxury/refined, playful/toy-like, editorial/magazine, brutalist/raw, art deco/geometric, soft/pastel, industrial/utilitarian, etc. There are so many flavors to choose from. Use these for inspiration but design one that is true to the aesthetic direction.
- **Constraints**: Technical requirements (framework, performance, accessibility).
- **Differentiation**: What makes this UNFORGETTABLE? What's the one thing someone will remember?

**CRITICAL**: Choose a clear conceptual direction and execute it with precision. Bold maximalism and refined minimalism both work - the key is intentionality, not intensity.

Then implement working code (HTML/CSS/JS, React, Vue, etc.) that is:
- Production-grade and functional
- Visually striking and memorable
- Cohesive with a clear aesthetic point-of-view
- Meticulously refined in every detail
- Simple and performant (avoid complex animations, shadows, etc.)

## Frontend Aesthetics Guidelines

Focus on:
- **Typography**: The core of a performant UI. Choose fonts that are beautiful, unique, and interesting. Avoid generic fonts like Arial and Inter; opt instead for distinctive choices that elevate the frontend's aesthetics. Pair a characterful display font with a highly legible, refined body font.
- **Color & Theme**: Commit to a cohesive, high-contrast aesthetic. Use CSS variables for absolute consistency. Dominant, purposeful colors with sharp accents define the identity without needing resource-heavy effects.
- **Spatial Composition**: Rely on layout over effects. Use unexpected layouts, asymmetry, and generous negative space to create visual interest. Prioritize grid-based precision and clean hierarchies.
- **Zen Workspace Architecture**:
    - **Focus First**: Maximize the primary work area. The interface must be non-disruptive.
    - **Estate Management**: Sidebars, toolbars, and controls must not exceed **20%** of total screen estate. 
    - **Autohide & Edge Docking**: To prevent bloat, implement autohide mechanisms. Dock secondary tools to the edges (Top/Middle/Bottom 33% of left or right screen edges).
- **Performance-First Design**: Skip heavy animations, parallax, and complex shadows. Use subtle transitions (e.g., hover states) only when they clarify function. Avoid gradient meshes or grain overlays that impact render performance.
- **Native Efficiency**: Leverage semantic HTML and optimized CSS. Ensure the UI feels "instant" and lightweight on any hardware.

## Architectural SSOT (Single Source of Truth)

- **Backend Priority**: When frontend and backend share variables (configs, constants, state schemas), the **backend is the authoritative source**.
- **DRY Enforcement**: Never double-declare shared variables in the frontend. Retrieve them dynamically from the backend or generate types/constants from the backend source. 
- **Consistency**: Ensure the frontend UI state is a direct reflection of the backend's truth, minimizing client-side "shadow state" to avoid synchronization bugs.

NEVER use generic AI-generated aesthetics like overused font families (Inter, Roboto, Arial, system fonts), cliched color schemes, or "glassmorphism" that adds unnecessary GPU overhead.

Interpret creatively within the bounds of high-performance minimalism. No design should be the same, but all should be blazing fast.

**IMPORTANT**: Elegance comes from restraint and precision. A simple design requires even more attention to spacing, typography, and detail. Excellence is achieved when there is nothing left to take away.

Remember: You are capable of extraordinary creative work. Don't hold back, show what can truly be created when thinking outside the box and committing fully to a distinctive, performant vision.

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
- `skill id=frontend-design`
- `skill id=frontend-go`
- `skill id=tview-scroll`