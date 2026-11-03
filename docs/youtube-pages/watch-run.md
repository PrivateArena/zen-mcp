# Watch page probe run — Stage 2 (comment / reply / read)

Live session on 2026-11-03, container `pachoigame` (`containerId 20`), tab `t_mw0cz7t1_6hh`, video
`YSNFGgh0aAs` (`whiteboard`, **the account's own Unlisted video** — visible because the page renders
`Analytics` and `Edit video`; that is also the page-side proof of identity). Screenshots in `img/watch/`
(01–05, captured by hand because `screenshot` grabs the viewport, and the sticky header/player hide the
comment section once the page scrolls).

Sequence, with the bridge replies that mattered.

## 1. Open the page and read the section

| Command | Reply |
|---|---|
| `active_tab` | `{"containerId":20,"id":"t_mw0cz7t1_6hh","url":"…watch?app=desktop&v=YSNFGgh0aAs"}` |
| `web_eval` structural probe (deep query, shadow-piercing) | `{"commentsCount":1, …}` |

Row structure as the page exposes it (light DOM, stable ids — no shadow walk needed to *read*):

```
ytd-comment-thread-renderer
  #comment-container
    ytd-comment-view-model#comment
      #body
        #author-thumbnail
        #main
          #header
            #header-author
              h3 > a#author-text                     "@JangGaming"
              span#published-time-text > a           "4 hours ago", href "…&lc=UgwEFzl5ZWYy-wHKInF4AaABAg"
          ytd-expander#expander > #content
            yt-attributed-string#content-text        "What is the tools that you used to create this video ?…"
        ytd-comment-engagement-bar#action-buttons
          #toolbar > ytd-button-renderer#reply-button-end > yt-button-shape > button[aria-label="Reply"]
        #reply-dialog > ytd-comment-reply-dialog-renderer#replybox   (created by pressing Reply)
      #action-menu                                   (⋮)
```

**The comment id is on the page**: `#published-time-text a[href*="lc="]` → `…&lc=<id>`. It survives a page
reload and is what a later `comment_id` (moderate/delete) will address — no network call needed.

## 2. Comment box

`web_eval` press on `yt-formatted-string#simplebox-placeholder` (`role="textbox"`, text `Add a comment...`):

```json
{"clicked":true,"path":"div#header > ytd-comments-header-renderer > div#simple-box
  > ytd-comment-simplebox-renderer > div#placeholder-area > yt-formatted-string#simplebox-placeholder"}
```

After it, the box exists under `ytd-comment-dialog-renderer > ytd-commentbox#commentbox`, with the editor at
`… #creation-box > tp-yt-paper-input-container#input-container > div > div#labelAndInputContainer > div >
ytd-emoji-input#emoji > yt-user-mention-autosuggest-input > yt-formatted-string#contenteditable-textarea >
div#contenteditable-root` (`aria-label="Add a comment..."`), `#cancel-button`, and
`ytd-button-renderer#submit-button button[aria-label="Comment"]` — **`disabled: true` while the editor is empty**.

## 3. Reply box

Press the row's `Reply` (`ytd-button-renderer#reply-button-end button`, `aria-label="Reply"`):

```json
{"clicked":true,
 "boxes":[ "… #simple-box > #comment-dialog > ytd-comment-dialog-renderer > ytd-commentbox#commentbox",
           "… ytd-comment-engagement-bar#action-buttons > div#reply-dialog
              > ytd-comment-reply-dialog-renderer#replybox > ytd-commentbox#commentbox" ],
 "buttons":[ …{"aria":"Comment","disabled":true}… , …{"aria":"Reply","disabled":true}… ]}
```

**Both boxes can be open at once**, so a flow must scope every lookup by ancestor: the top-level box is under
`ytd-comment-dialog-renderer`, a reply box under `#replybox`. "The first `#commentbox` in the document" is
ambiguous and can post into the wrong box.

## 4. Writing into the editor, and the gate that proves it worked

```json
{"cmd":true,"readBack":"Probe from the zen-mcp youtube tool — replying to test the p",
 "submitAria":"Reply","submitDisabled":false}
```

`execCommand("insertText")` after `focus()` + selecting the node's contents (the Studio lesson) lands the text in
the model; the submit button flipping `disabled: true → false` is the page's own acknowledgement. A flow that
presses submit without checking that flip cannot tell a committed write from a cosmetic one.

## 5. Submit the reply

```json
{"ok":true,"state":{"aria":"Reply","cmd":true,"disabled":false,"readBack":"Replied by the zen-mcp youtube page flow"},
 "before":2,"after":4,"outcome":null,"boxesOpen":2}
```

`outcome: null` means the box-close watcher never fired within 20 s — **the box staying open is not a failure**;
the read below is the truth:

```json
{"author":"@PhuongAnh-l1s1z","id":"UgwEFzl5ZWYy-wHKInF4AaABAg.AcjrtNE1RFuAckORbsxpzd",
 "nested":true,"text":"Replied by the zen-mcp youtube page flow (Stage 2 probe)…","when":"0 seconds ago"}
```

**A reply's id is `<parent-id>.<suffix>`.** The tool should parse it as opaque, but may take the prefix apart to
tell a reply from a top-level comment, and `#replies` is where nested rows live.

## 6. Submit a top-level comment

Same shape (`aria "Comment"`, `disabled: false`, `cmd: true`), then:

```json
{"count":3,
 "rows":[{"author":"@PhuongAnh-l1s1z","id":"UgwfNOQ-X0OK0WZnVdl4AaABAg","nested":false,
          "text":"Comment posted by the zen-mcp youtube page flow (Sta…","when":"0 seconds ago"},
         {"author":"@JangGaming","id":"UgwEFzl5ZWYy-wHKInF4AaABAg","nested":false,…},
         {"author":"@PhuongAnh-l1s1z","id":"UgwEFzl5ZWYy-wHKInF4AaABAg.AcjrtNE1RFuAckORbsxpzd","nested":true,…}]}
```

Two traps this exposes:

1. **The header count goes stale.** It still read `1 Comment` after the second post. A flow must count rows, not
   trust the header.
2. **DOM order is not the sort order.** The new comment came back *first* under `Sort by: Top` — because the
   author's own fresh comment is shown to the author at the top. So "find the row I just posted" must search the
   whole set by text or id, never look at `rows[0]`.

## 7. Other findings

* The page scrolled itself back to the player after posting; screenshots and re-reads need `#comments`
  scrolled into view again. `screenshot` captures the viewport, and the sticky header covers the top of it —
  which is why the section shots here were taken by hand.
* Pressing `#cancel-button` did **not** close a box, through either the page channel or the bridge's locator
  click. Not needed by any flow step; cleanup is by closing the tab.
* `read` must scroll the section into view first: the rows are lazy-rendered.

## 8. What the Go flow must do

1. Open `watch?v=<id>` in the configured container; assert identity (container id + the video's channel).
2. `read`: scroll `#comments` into view, wait for a thread, walk rows (top-level and `#replies`), return
   author / text / relative + absolute time / `lc` id / `is_reply`, honouring `--max` and paging.
3. `comment`: press `#simplebox-placeholder`, wait for `#commentbox`, insert with `execCommand`, **wait for
   `#submit-button button.disabled == false`**, then press it once — after claiming the journal slot.
4. `reply`: press that row's `#reply-button-end button` (row found by `comment_id`'s `lc` link, so the wrong
   comment is never answered), fill `#replybox … #contenteditable-root`, same disabled-flip gate, press once.
5. Confirm by re-reading the section for the new id (or text). Not found → `ErrTransient` with the trail, and
   **never a second submit**: the press either landed or it did not, and re-pressing is how a duplicate appears.
6. `--dry_run` stops before the first press.
