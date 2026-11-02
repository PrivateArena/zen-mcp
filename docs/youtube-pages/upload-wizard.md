# Page notes — Studio upload wizard

Origin `studio.youtube.com`. Consumed by Stage 1 (`upload`, `upload_status`, `upload_abort`).
**Status: FROZEN — the whole flow was executed live on 2026-11-02 with one upload, no retries.**
Narrative evidence, quoted probe responses and screenshots: [`upload-wizard-run.md`](upload-wizard-run.md).

Resulting video: `BeCRc-x5B6E` (`https://youtu.be/BeCRc-x5B6E`), title `output (upload probe)`,
scheduled public **Nov 9, 2026 12:00 AM** (channel zone), uploaded Nov 2, 2026.

## Entry

| Fact | Value | Probe |
|---|---|---|
| Requested URL | `https://www.youtube.com/upload` (add `hl=en` for labels) | run §Step 0 |
| Final URL | `https://studio.youtube.com/channel/UCZZei3idefNE6mAUXs2Eh8A/videos/upload?d=ud&filter=%5B%5D&sort=…` | navigate reply, `status:200` |
| Identity | `containerId: 20` = `pachoigame`; sidebar chip `Your channel / Phuong Anh` | navigate reply + `01-*.png` |
| Modal | `Upload videos` dialog, `tp-yt-paper-dialog#dialog`, text `Drag and drop video files to upload` | `01-select-files-dialog.png` |

## Frozen selectors

| Step | Locator (bridge params) | Verified by |
|---|---|---|
| attach | `selector: ytcp-uploads-file-picker#ytcp-uploads-dialog-file-picker input[type=file]` (light DOM, hidden `0×0`, `multiple`, `accept:""`, **no id**) | `{filename:"output.mp4",size:301582,success:true}` + dialog gone afterwards |
| (visible alternative) | `#select-files-button button` = `button[aria-label="Select files"]` | `01-*.png` |
| transfer | read `.progress-label` / `p.ytcp-video-upload-progress` text | `Upload complete ... Processing will begin shortly` |
| panel changed | `[role="tab"]` with `aria-selected="true"` ∈ `Details`/`Video elements`/`Checks`/`Visibility` | tab array after every Next |
| made for kids (**mandatory**) | `ytkc-made-for-kids-select tp-yt-paper-radio-button` — `by_text: "No, it's not made for kids"` | `aria-checked:"true"`, inline error `You need to answer this question` gone |
| title | `selector: #title-textarea div#textbox` — `div[role=textbox]` **contenteditable** | read-back + `All changes saved.` |
| description | `selector: #description-textarea div#textbox` — **contenteditable** | read-back + `saved` |
| Next | `selector: #next-button button` (`aria-label="Next"`) | tab `aria-selected` moved |
| visibility now | `#privacy-radios` group: `#private-radio-button`, `Unlisted`, `Public`, `Set as instant Premiere` — byText clicks fine | panel text |
| schedule mode | radio `Schedule` (`p#visibility-title` carries the label) | panel text `Schedule as public` |
| date | `ytcp-text-dropdown-trigger#datepicker-trigger` → modal `ytcp-date-picker … tp-yt-iron-list#calendar-main`, cells `span.calendar-day` | trigger text `Nov 3, 2026` → `Nov 9, 2026` |
| time | `ytcp-datetime-picker input.style-scope.tp-yt-paper-input` — **writing it does not commit** (see traps) | typed `10:00 AM`, model kept `12:00 AM` |
| commit | `#done-button button` — label is `Publish`/`Save`/`Schedule` by mode | share dialog text |
| result | `.ytcp-video-share-dialog` text: `Video link https://youtu.be/<id>` | `… https://youtu.be/BeCRc-x5B6E` |

## Traps (each one observed, not theorised)

1. **The video exists the moment the attach lands** — Studio autosaves it as `Saved as private` and
   mints the id. Claim the journal slot *before* the `upload` call; a failed run leaves a draft.
2. **COPPA blocks Next.** `Is this video made for kids? (required)` must be answered or the panel never
   advances — with no error dialog, only inline text. It is a parameter, not an edge case.
3. **Contenteditable fields ignore `type`.** It focuses and writes nothing (`trigger_native_type` in the
   reply is the tell). Use `selectNodeContents` + `document.execCommand("insertText")`; `selectAllChildren`
   + `collapseToEnd` *appends* instead of replacing.
4. **The date/time text inputs are display-only.** `.value`, native setter + events, Enter, and
   `form.requestSubmit()` all revert when the picker closes. Commit by clicking: the calendar
   (`span.calendar-day`, skip `.disabled`, disambiguate by month group — 112 cells are rendered across
   Nov 2026/Dec 2026/Jan 2027) and, for time, the picker's option list. `key_combo` cannot help: it is
   OS-level and the tab is in the background.
5. **No `data-test-id` attributes** exist in this dialog; ids and `aria-label`s are the stable handles.
6. **Never `wait_for_network_idle`** here — the SPA beacons forever. Wait on the progress text.
7. `triggerDrop` omitted on `upload`, or the file attaches twice.
8. Time strings use **U+202F** between minutes and `AM`/`PM` (`12:00 AM`, code 8239).
9. `screenshot` with a **relative** `path` answers `success:true, data:null` and writes nothing — always
   absolute.
10. Closing the wizard without committing is *not* a no-op once the attach happened; treat it as a state
    change and report it.
