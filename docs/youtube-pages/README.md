# YouTube page notes

The observation log behind `internal/youtube/pages/*.go`. It is the evidence base the plan
(`ARCHITECTURE_PLAN_youtube-channel-tool-v2.md` §3.2) requires: **a selector may not enter Go code
before it appears in a probe response recorded here.** No selector without a probe.

| Page notes | Page | Origin | Consumed by |
|---|---|---|---|
| [upload-wizard.md](upload-wizard.md) | Studio upload wizard — **frozen** | `studio.youtube.com` | Stage 1 · `upload`, `upload_status`, `upload_abort` |
| [upload-wizard-run.md](upload-wizard-run.md) | the live Stage 1 run: commands, quoted replies, screenshots | `studio.youtube.com` | evidence for the frozen table |

Probe programs actually executed (terminal, `web_eval -c @file`) live in [`probes/`](probes/).
| [watch.md](watch.md) | Watch page comments | `www.youtube.com` | Stage 2 · `comments`, `comment`, `reply` |
| [studio-content.md](studio-content.md) | Studio Content list | `studio.youtube.com` | Stage 3 · `videos` |
| [studio-details.md](studio-details.md) | Studio details panel | `studio.youtube.com` | Stage 4 · `edit`, `delete` |

Code-side counterpart of the container fact below: `YoutubeConfig` (`internal/mcpcfg/config.go`,
`config.json → youtube`). It ships **no default identity** — with no `youtube.container` configured
the tool must refuse rather than act as whoever the browser happens to be logged in as.

## Standing facts

| Fact | Value | Evidence |
|---|---|---|
| Identity container | `pachoigame` = `containerId 20` | `list_containers`, 2026-11-02 — the numeric id in `config.json` is this value, asserted from every `navigate` reply, never guessed |
| Channel | `UCZZei3idefNE6mAUXs2Eh8A` (from the Studio redirect URL) | [upload-wizard.md](upload-wizard.md) E1 |
| `youtube.com/upload` | Redirects into the Studio wizard and hands over the channel id in the URL | E1 |
| Test fixture | `https://www.youtube.com/watch?v=YSNFGgh0aAs` (`whiteboard`, unlisted, 0 comments) | E8–E11 |
| Studio renders in shadow roots | Light-DOM `querySelectorAll("button")` finds nothing | E4 |
| `get_interactive_map` | Unusable — replies `data:null`; not used by any flow | bridge bug, see plan E-table |

## Conventions (identical in every note)

1. **Open** with an explicit `container` and `new_tab`/`back_tab` — a container is only applied when a
   tab is *created*, so re-using a tab cannot select the identity. Record the returned `id` (`t_…`)
   and pass it as `index` on **every** later call; a stale id is a hard `tab_not_found`, never a
   fallback to the selected tab.
2. **`hl=en`** on every URL — the locators are English labels.
3. **Settle with element conditions** (`wait_for_selector` / `wait_for_function`), never
   `wait_for_network_idle` (the Studio SPA streams beacons forever).
4. **Probe DOM through shadow roots** — `deepQuery(sel)` is injected in the `web_eval` sandbox,
   `deepQueryAll` is not, so list probes carry their own walker (§R4 template below).
5. **A `web_eval` program's first token must be `return|var|let|const`** or the bridge wraps it in
   `return (…)` and it dies with `SyntaxError: missing ) in parenthetical`.
6. **Verify by observation** after every state-changing action — re-read the thing that should have
   changed. Unverified writes are reported `unverified`.
7. **Close the tab** on the way out (`close_tab` with the recorded `index`).

## §R4 — shadow-DOM locator probe

The standing `web_eval` program for "what can I click here". Reports visible interactive nodes with a
shadow-crossing CSS path; capped so the reply stays under the bridge's 1 MiB inline limit.

```js
const visit = (root, out, depth = 0) => {
  for (const el of root.querySelectorAll("button,a[href],input,textarea,[role],[contenteditable=true],[data-test-id]")) {
    const r = el.getBoundingClientRect();
    if (r.width && r.height) out.push({
      tag: el.tagName.toLowerCase(), role: el.getAttribute("role"),
      label: el.getAttribute("aria-label"), text: (el.innerText || el.value || "").trim().slice(0, 60),
      id: el.id, testId: el.getAttribute("data-test-id"), cls: (el.className || "").toString().slice(0, 60),
      path: cssPath(el),
    });
  }
  if (depth > 12) return out;
  for (const el of root.querySelectorAll("*")) if (el.shadowRoot) visit(el.shadowRoot, out, depth + 1);
  return out;
};
const cssPath = (el) => { const p = []; let n = el; while (n && n.nodeType === 1 && p.length < 8) {
  const host = n.getRootNode().host; const seg = n.tagName.toLowerCase() + (n.id ? "#" + n.id : "");
  p.unshift(seg); n = host || n.parentElement; } return p.join(" > "); };
return visit(document, []).slice(0, 40);
```

Run it as a file, never inline (`--json` quoting mangles JS):

```bash
cat > /tmp/probe.json <<'EOF'
{"action":"web_eval","index":"t_…","code":"@/tmp/probe.js"}
EOF
zbrowser --json-file /tmp/probe.json
```

## Stage 0 log (2026-11-02)

| Probe | Result |
|---|---|
| `list_containers` | `pachoigame` → `id: 20` (22 containers) — written into `config.json youtube.containerId` |
| `{navigate, url: youtube.com/, container: pachoigame, new_tab: true}` | `t_mvzdqtnt_6g1`, `containerId: 20`, `statusText` reported, `title: YouTube`; `active_tab` right after → **same id** (a foreground `new_tab` takes the selection, which is what a probe wants) |
| `navigate --url youtube.com/feed/library --container pachoigame --back_tab true` (flag form, through the `browser` tool) | `t_mvzdr6vu_6g2`, `containerId: 20` — **the explicit container beat `CONTAINER_MAP`'s `youtube.com → Personal`** live, as unit-tested by `TestBrowserExplicitContainerBeatsTheDomainDefault` |
| `wait_for_selector -i t_… --selector "#avatar-btn" --timeout 20000` | `true` — locator flags now reach the bridge through the MCP schema; `--json-file` is no longer required for locator params (Stage 0.1) |
| `get_text -i t_… --selector "#avatar-btn"` | `{content:"", el:{id:"avatar-btn", focused:false, visible:true, rect:[1204,11,54,34]}}` — frame reply carries its own provenance |
| `navigate --url "youtube.com/watch?v=YSNFGgh0aAs&hl=en" --container pachoigame --back_tab true --timeout 45000` | `t_mvzdzitu_6g3`, `containerId: 20`, `title: whiteboard - YouTube` — the whole identity chain works through the **MCP tool in flag form**, no `--json` needed |
| `close_tab -i <each id>` | `{id,success:true}` ×3; `active_tab` afterwards still reports the user's own pre-existing Studio tab — the probes never moved the selection back |

**Stage 0 accepted.** `containerId 20` asserted from `navigate`, the locator/container flags are live,
`zyoutube` does not exist yet (Stage 1 writes it), the server restarted clean on every code change.

## Locator table format

Each entry records the *probe* that produced the selector, so a future redesign is a diff against
evidence rather than against a guess:

| Step | Locator (bridge params) | Probe that produced it | Verified by (text/state) |
|---|---|---|---|

## Which channel a run acts as

Every action takes `container` — a Firefox contextual-identity name, the same idea as `zbrowser -a navigate
--container`. Omit it and `youtube.container` is used. Only names declared in `youtube.containers`
(`name -> containerId` in `config.json`) are accepted; anything else is refused before a tab exists, because an
id discovered from the live browser is how a typo becomes another channel. Each channel keeps its own ledger
(`journal.json` for the default, `journal-<name>.json` for the rest), so the daily upload cap is per channel.
The probe videos in these pages live in `pachoigame`, containerId 20.
