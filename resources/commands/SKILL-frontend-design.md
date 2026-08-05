---
description: This skill guides creation of distinctive, production-grade frontend interfaces that avoid generic AI slop aesthetics. Implement real working code with exceptional attention to aesthetic details and creative choices.
argument-hint: |-
  i: Context or specific instructions for this skill
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
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=frontend-design`