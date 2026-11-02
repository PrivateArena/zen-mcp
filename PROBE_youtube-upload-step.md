Test upload file: /media/jang/home/Deve/zen-hyperframes/demo/my-video/output.mp4

- Go to https://www.youtube.com/upload in pachoigame container using `zbrowser -a navigate --container pachoigame
- Probe to find the upload element using selector
- Using `zbrowser -a upload` to upload
- After this, do not retry again, confirm that upload is success using -a screenshot, view the screenshot
- Now we have the Youtube video information page to set video title, description... Probe it to get selectors of necessary elements
- After changing video information, you need to Next a few times, just click the Next button using -a click
- There will be a screen for setting Visibility, probe it to get the select element, then Next and Finish

That's how we upload a Youtube video, ASK ME TO ASSIST IF NEEDED TO SPEED UP THE PROGRESS. DO NOT TRY TO GUESS.
---

## RESULT — executed 2026-11-02, one upload, no retries

Full step-by-step record with commands, quoted probe replies and 9 screenshots:
**[`docs/youtube-pages/upload-wizard-run.md`](docs/youtube-pages/upload-wizard-run.md)**
Frozen selector table: **[`docs/youtube-pages/upload-wizard.md`](docs/youtube-pages/upload-wizard.md)**

Outcome: video **`BeCRc-x5B6E`** — https://youtu.be/BeCRc-x5B6E — title `output (upload probe)`,
scheduled **public on Nov 9, 2026 12:00 AM** ("Video scheduled" confirmed on the done screen).

| Step | How it actually resolves |
|---|---|
| open | `zbrowser -a navigate --url https://www.youtube.com/upload --container pachoigame --new_tab true` → `containerId:20`, redirects into Studio, channel id in the URL |
| attach | `zbrowser -a upload --selector "ytcp-uploads-file-picker#ytcp-uploads-dialog-file-picker input[type=file]" --path <abs>` → `{filename,size,success:true}` (no `triggerDrop`) |
| confirm | `.progress-label` = `Upload complete ... Processing will begin shortly`; app bar `Saved as private`; `Video link https://youtu.be/BeCRc-x5B6E` |
| title/description | `#title-textarea div#textbox`, `#description-textarea div#textbox` — contenteditable: **`-a type` writes nothing**; `selectNodeContents` + `document.execCommand("insertText")` lands (verified by `All changes saved.`) |
| Next | `#next-button button`, panel change verified by `[role=tab][aria-selected=true]` |
| gate | COPPA `Is this video made for kids? (required)` **blocks Next** until answered (`by_text "No, it's not made for kids"`) |
| visibility | byText `Private`/`Unlisted`/`Public`/`Schedule`; schedule date = click `#datepicker-trigger` then the `span.calendar-day` for the day (**writing the date/time text input never commits**) |
| finish | `#done-button button`, labelled `Schedule` in schedule mode |

Two gaps left unprobed by instruction: the share dialog's `Close`, and the time picker's option list —
so `publish_at` can set the date reliably, but the **time must not be written to its input** (typed
`10:00 AM` displayed and was ignored; the model kept `12:00 AM`).
