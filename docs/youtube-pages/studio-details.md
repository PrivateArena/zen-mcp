# Page notes — Studio details panel

Origin `studio.youtube.com`. Consumed by Stage 4 (`edit`, `delete`).
**Status: skeleton — probe before coding. Entry point must be *discovered*, not assumed: do not
presume `/videos/editing?videoId=`.**

Stage 4's destructive tests (`delete`, title round-trip) run against a **throwaway video named by the
user**, never against the `whiteboard` fixture (locked decision 1).

## Entry

| Fact | Value | Probe |
|---|---|---|
| entry point (row → details) | TODO | TODO |
| direct URL, if any | TODO | TODO |

## Selector table

| Step | Locator (bridge params) | Probe that produced it | Verified by (text/state) |
|---|---|---|---|
| title field | TODO | TODO | read-back equals the patch |
| description field | TODO | TODO | read-back |
| visibility / schedule controls | TODO — if the panel has no schedule field, report `locator_unresolved` with the §4.4 diagnosis bundle instead of faking it | TODO | — |
| `Save` | TODO | TODO | the `Saved` affordance appears |
| `… More actions → Delete video` | TODO | TODO | confirmation dialog text quoted below |
| delete confirmation | TODO | TODO | row absent from the Content list |

## Dialog text (verbatim, from the probe)

_TODO_
