# Page notes — Studio Content list

Origin `studio.youtube.com`. Consumed by Stage 3 (`videos`) and by Stage 4's "row is gone" check.
**Status: skeleton — probe before coding.**

## Entry

| Fact | Value | Probe |
|---|---|---|
| URL | `https://studio.youtube.com/channel/UCZZei3idefNE6mAUXs2Eh8A/videos/upload?hl=en&filter=%5B%5B%5D%5D` (the `filter` shape copied from E1's redirect target) | Stage 0 probe, 2026-11-02: `new_tab:true container:pachoigame` → `containerId:20` |
| Table renders in shadow roots | light-DOM queries find nothing | E4 (same renderer as the wizard) |

## Selector table

| Step | Locator (bridge params) | Probe that produced it | Verified by (text/state) |
|---|---|---|---|
| filter chips (`Published` / `Drafts` / `Scheduled` / `Live`) | TODO | TODO | chip selected-state + row set |
| row | TODO | TODO | row count > 0 |
| title / visibility chip / publish date / duration / views / comments | TODO | TODO | Stage 1 video under `Scheduled`, fixture with `Unlisted` |
| empty state text | TODO | TODO | quoted verbatim |

## Acceptance (Stage 3)

The Stage 1 upload appears under `filter=scheduled` with the right date, and the fixture
(`whiteboard`) appears with `visibility: unlisted` — which proves the visibility chip is read, not
assumed.
