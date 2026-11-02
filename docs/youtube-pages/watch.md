# Page notes — Watch page

Origin `www.youtube.com`. Consumed by Stage 2 (`comments`, `comment`, `reply`).
**Test fixture: `https://www.youtube.com/watch?v=YSNFGgh0aAs` — `whiteboard`, unlisted, owned by
`pachoigame`, comments enabled, 0 comments at the time of probing (so Stage 2's own posts are its
test data, and the unlisted URL keeps them off the public feed).**

## Entry & state

| Fact | Value | Probe |
|---|---|---|
| navigate | `container: pachoigame` → `containerId: 20`, logged in (`ytd-masthead #avatar-btn` present) | E8 |
| `canonical` | the fixture URL — provenance assertion for every write | E8 |
| Title | `whiteboard - YouTube` | E9 |
| `#comments` | present after `readyState:complete` | E9 |
| Comment count header | `0 Comments\nSort by` | E9 |
| `ytd-comment-thread-renderer` | **0** rendered | E9 |
| Composer | `#simplebox-placeholder` / `#contenteditable-root` present | E9 |
| Lazy load | `#comments` renders its threads only after `scrollIntoView()` + ~4 s; works on a **background** tab | E11 |

Reproduce:

```bash
cat > /tmp/nav.json <<'EOF'
{"action":"navigate","url":"https://www.youtube.com/watch?v=YSNFGgh0aAs&hl=en","container":"pachoigame","back_tab":true}
EOF
zbrowser --json-file /tmp/nav.json | jq -c '{success,id,url,containerId,title,status,error}'
```

## Selector table

| Step | Locator (bridge params) | Probe that produced it | Verified by (text/state) |
|---|---|---|---|
| settle | `wait_for_selector selector: #comments` | E9 | `#comments` present |
| lazy-load trigger | `scroll selector: #comments block: start` (or `web_eval` `scrollIntoView`) | E11 | thread count > 0 **after** the scroll, never before |
| comment count header | TODO | TODO | text matches `^[\d.,]+\s*Comments` |
| thread | `selector: ytd-comment-thread-renderer` | E9 (count query) | row count grows per scroll |
| author | TODO | TODO | non-empty text |
| comment body | TODO | TODO | non-empty text |
| comment id | TODO — read from the thread's `continuation`/`comment-id` attribute, not from the DOM order | TODO | stable across two reads |
| top composer | TODO (`#simplebox-placeholder` opens it) | E9 | `#contenteditable-root` visible |
| reply button (per thread) | TODO | TODO | the thread's own body text still visible beside the form |
| `Cancel` / `Comment` buttons | TODO | TODO | form present/absent |
| Show more replies | TODO | TODO | thread count grows |

## Text entry (R5 decision — open)

`type` writes `.value` without focusing (E16: reply `{"el":{"focused":false,…}}`), which a
Polymer/`contenteditable` surface may not notice. The probe must decide **once** between:

1. `type` + read-back,
2. `web_eval` text assignment + `input`/`change` event dispatch,
3. `trusted_paste` — only if a human sets `youtube.allowFocusBoundActions: true`.

Whatever wins is used unconditionally in code, and every typed value is verified by read-back before
the send button is clicked. Nothing is ever escalated at runtime.

**Decision:** _TODO — record the method, the exact probe, and the observed reply shape._

## Posts made from this page

Stage 2's acceptance posts stay on the fixture (they are its data). List them here:

| videoId | commentId | text | date |
|---|---|---|---|
