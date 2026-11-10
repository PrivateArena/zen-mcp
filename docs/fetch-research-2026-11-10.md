# zfetch / WebFetch research — 2026-11-10

Goal: make `fetch` (`internal/tools/fetch.go` + Firefox bridge) behave like a harness
WebFetch — "fetch the URL, get the content that answers the problem".

Part 1 = measurements on this machine. Part 2 = bridge code map. Part 3 = external
research (asked Claude via `zchat`, prompt preserved at `/tmp/webfetch_prompt.md`,
raw answer at `/tmp/claude_webfetch.log`).

---

## PART 1 — Measurements [VERIFIED]

### Fixture: https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API

Default `zfetch -u <url>` → 13,758 B / 298 lines.

| Region | Bytes | Share | Content |
|---|---|---|---|
| L1–152 | 5,823 | 42% | skip links, top nav, full HTML/CSS/JS/WebAPI sidebar reference dumps, Blog |
| L153–235 | 4,659 | 34% | the article |
| L236–298 | 3,276 | 24% | "Help improve MDN", GitHub links, Mozilla footer |

- 113 / 298 lines are navigation links.
- The browser-compatibility tables are absent.
- `--selector main` → 5,893 B / 92 lines, clean article, still no tables.
- `--selector '#browser_compatibility'` → 202 B (heading only; on MDN that id is the `<h2>`).
- `--selector article` → hard error, MDN has no `<article>` (content root is `main.layout__content`).
- `--idle_ms 6000` does not recover the tables.

### Shadow DOM is the whole story for the missing tables

`web_eval` in the loaded tab, after network idle and after `scrollIntoView()`:

```
document.querySelectorAll('table')            → 1   (Specifications only)
document.querySelectorAll('[class*=bc-]')     → 0
performance resource entries                  → bcd.developer.mozilla.org/bcd/api/v0/current/api.fetch.json
                                                (the compat DATA arrived)
document.visibilityState                      → "hidden"  (background tab)
custom elements on page                       → MDN-COMPAT-TABLE-LAZY ×2
mdn-compat-table-lazy.shadowRoot             → 405 B  → contains <mdn-compat-table query="api.fetch">
mdn-compat-table.shadowRoot                  → 279,738 B of HTML with the real tables
deep table count (piercing shadow roots)      → 3
light-DOM table count                         → 1
```

So: two shadow boundaries deep, fully rendered, 100% invisible to the light-DOM
serializer. Nothing to do with timing, waiting, or focus.

`zfetch --selector mdn-compat-table` → `"... has no readable content"` — the selector
resolves (frame.js `deepQueryAll` pierces), the serializer then finds nothing.

### The default path DOES have article detection; it fails silently

`zfetch -u https://overreacted.io/before-you-memo/` → payload starts at the article,
no nav, no footer. Readability succeeded. On MDN the same default returned the whole
document, i.e. Readability returned nothing and the code fell back to `doc.body`
with no flag in the reply. Conclusion: "junk nav" is not "no extraction logic", it
is "extraction logic failed and nobody was told".

### Other observed defects

- Relative links survive into the payload (`[Headers](/en-US/docs/Web/API/Headers)`),
  so the caller cannot chain another fetch.
- Heading self-anchors cost ~40% of every heading line: `## [Concepts and usage](#concepts_and_usage)`.
- Ordered lists emit a literal `1.` for every item.
- `clampBridgeContent` is a blind byte cut from the top, mid-sentence.

---

## PART 2 — Bridge code map

Root `B=/media/jang/home/PortableApp/firefox/profile/chrome/JS/bridge`,
vendored frame libs `U=/media/jang/home/PortableApp/firefox/profile/chrome/JS/utils`.

| Location | What |
|---|---|
| `B/server.mjs:249` | `handleHttpRequest` — dispatch; `read` is NOT in `ACTION_HANDLERS` |
| `B/server.mjs:265` | ⇒ implicit frame passthrough `runFrameAction(payload)` |
| `B/server.mjs:9` | `FRAME_UTILS = ["SimpleMarkdown.js","CrawlExtract.js","InteractiveMap.js"]`, injected `:173`, `frame.js` injected `:189` (permanent `loadFrameScript`, chrome frame script ⇒ `openOrClosedShadowRoot` available) |
| `B/frame.js:426` | `get_content` / `get_text` / `read` branch; `prop` chosen `:427` |
| `B/frame.js:430` | **`if (!selector && !hasSemantic)`** — the switch: no selector ⇒ Readability; any selector ⇒ raw subtree, Readability skipped |
| `B/frame.js:432` | `target = doc.body` (the silent fallback) |
| `B/frame.js:437-448` | load Readability (`moz-src:///toolkit/.../Readability.js`, fallback `U/Readability.js`) |
| `B/frame.js:452` | `new win.Readability(doc.cloneNode(true)).parse()` — **cloneNode drops every shadow root** |
| `B/frame.js:453-457` | keeps only `article.content`, re-parses into a div |
| `B/frame.js:458-460` | `catch {}` — swallows every Readability failure, no flag |
| `B/frame.js:461` | `SimpleMarkdown.fromElement(target)` |
| `B/frame.js:468-483` | scoped read (`multi` ⇒ `deepQueryAll`) |
| `B/frame.js:10-66` | `shadowOf` (uses `openOrClosedShadowRoot` ⇒ closed roots too), `PIERCE=">>>"`, `pierceQueryAll`, `deepQuery`, `deepQueryAll` |
| `B/frame.js:997` | `if (win.top !== win) return;` — only `detect_challenge` has this; `read` can lose the reply race to a subframe |
| `U/SimpleMarkdown.js:21` | `fromElement(element)` — the markdown serializer |
| `U/SimpleMarkdown.js:25` | `process(node, options)` |
| `U/SimpleMarkdown.js:44-52` | **`getChildren` iterates `node.childNodes` only — no `shadowRoot`, no `<slot>`**. This is THE gap. |
| `U/SimpleMarkdown.js:59-186` | `switch (tag)`: no cases for `details`/`summary`/`h5`/`h6`/`figure`/`dl`/`main`/`article`/`section`/`nav`/`header`/`footer`/`aside` (all → children only, no marker) |
| `U/SimpleMarkdown.js:71` | `href = node.getAttribute("href") \|\| node.href` — attribute wins ⇒ relative survives |
| `U/SimpleMarkdown.js:82` | same for `img src` |
| `U/SimpleMarkdown.js:175-177,222` | `handleTable` via `table.rows`/`row.cells` (light DOM only) |
| `U/SimpleMarkdown.js:12-13` | only existing `shadowRoot` use in this file (activeElement chain) |
| `U/Readability.js:142` | the ONLY `shadow` hit in vendored Readability — it never pierces |
| `U/Readability.js:456` | `_fixRelativeUris` using `doc.baseURI` (works, but only on the successful-Readability path) |
| `B/action_handlers.mjs:605` | `wait_for_network_idle` — `http-on-*` observers, `idle_ms` default 500, tab-scoped via `B/scope.mjs:15` |
| `B/frame.js:819` | `wait_for_selector` — MutationObserver, `deep:true` stability gate, but observes `doc.body` only ⇒ blind to shadow mutations |
| `B/server.mjs:13-18,274-286` | `REPLY_GUARDED_ACTIONS` byte guard — `read` is deliberately NOT guarded |
| no cap | no `max_chars`/chunking in the bridge at all; the only cap is Go-side `fetch.go:34-37` (30,000 default) |
| `B/crawl/` | linear template scraper (profiles + pipe DSL), no link discovery/frontier; `crawl_extract` is a second extraction entry point via `U/CrawlExtract.js:8-36`, which has its own duplicate `deepQuery` copy |
| `chrome/bridge_test.sh` | the only existing test touching extraction |
| `mcpReload()` (`chrome/JS/bridge.uc.js:5-21`) | hot-reloads the bridge; frame re-injection tagged per INSTANCE_ID ⇒ **edits to `frame.js` take effect without restarting Firefox** |

---

## PART 3 — External research (Claude, web-researched; sources inline)

See `/tmp/claude_webfetch.log` for the full text with links. Headlines:

**Reference implementations**
- Claude Code WebFetch: static HTTP fetch (no JS render), Turndown → markdown, cut at
  100k chars, then **Haiku summarizes against the caller's prompt**; trusted hosts with
  `text/markdown` < 100k pass through; cross-host redirects are returned to the model
  instead of followed; 15-min / 50 MB LRU cache keyed by URL; untrusted hosts get a
  stricter prompt with a 125-char quote cap.
- Anthropic API `web_fetch`: `max_content_tokens`, provenance block (url, `retrieved_at`),
  text + PDF only.
- OpenAI Responses `web_search`: exposes `open_page` / **`find_in_page`** + token budget —
  the model pages through content rather than receiving a dump.
- Gemini url-context: index-first, 20 URLs / 34 MB, per-URL `url_retrieval_status`.
- Cursor: sends `Accept: text/markdown` first; 85 KB HTML → 4.8 KB.
- MCP reference fetch server: Readability (readabilipy) → markdownify, `max_length` 5000
  default with **`start_index` paging**.
- pi-web-fetch (closest architecture): headless Chrome + networkidle2 + trafilatura +
  optional sub-agent pass.

**Reader services**
- Crawl4AI is the only one with documented shadow-DOM support: `flatten_shadow_dom`
  (monkeypatches `attachShadow`, resolves `<slot>`), needs hydration delay
  (example uses 3 s), reported ~1 KB → ~33 KB payload change. Plus `PruningContentFilter`
  and query-aware `BM25ContentFilter` producing `fit_markdown`.
- Jina Reader: `X-With-Shadow-Dom`, `X-With-Iframe`, `X-Wait-For-Selector`,
  `X-Target-Selector`, `X-Remove-Selector`, `X-Base` (resolve relative URLs against the
  **final** URL), `X-Token-Budget`.
- Firecrawl: `onlyMainContent` deterministic filter; `@firecrawl/html-extractor` is a
  usable Apache-2.0 library — trafilatura-style 5-stage pipeline, page-type classification
  (incl. documentation), 7-feature scoring, fallback chain, `extractionQuality` 0-1,
  `usedFallback`, `errorReason`.
- Tavily Extract: `query` → reranked ~800-char chunks. Linkup `/fetch`: markdown for
  HTML+PDF, `renderJs`, `includeRawHtml` for tables markdown loses.
- Readability + Turndown: neither pierces shadow roots.

**Benchmarks**
- WCXB (arXiv 2605.21097) — first benchmark with a **documentation** page type:
  rs-trafilatura F1 0.931, Trafilatura 0.888, Resiliparse 0.883, MinerU-HTML 0.838,
  ReaderLM-v2 0.776, **Readability 0.674 (worst of the mainstream heuristics)**.
  Documented documentation failure mode = TOC/sidebar navigation leaking in.
  Median documentation main content ≈ 16k chars.
- Bevendorff (SIGIR 2023): heuristics beat neural on complex pages.
- ReaderLM-v2 ≈ 10.4 s/page on an A100 ⇒ not viable per fetch.

**Shadow DOM dedup rule** (spec semantics; matches Crawl4AI): serialize the *flattened
tree the browser renders* — if a node has a shadow root, walk that root and skip the
host's light children; for `<slot>`, use `assignedNodes({flatten:true})` and fall back to
the slot's own children only when nothing is assigned. Firefox gives us
`openOrClosedShadowRoot`, so closed roots need no monkeypatching.

**Readiness** (beyond network idle 500 ms): MutationObserver DOM-quiet (must be attached
per shadow root), no `:not(:defined)` custom elements left, placeholder detection
(`aria-busy`, `[class*=skeleton]`, empty shadow root), text length stable across two
samples 250 ms apart, hard cap 5-8 s, and report `settled:false` if it fires.
Caveat: page-defined properties (`el.updateComplete`) are likely hidden by Xray wrappers
from a frame script — would need `wrappedJSObject`; untested here.

**Token economics**: markdown vs HTML ≈ 80% savings (Cloudflare: 16,180 → 3,150 tokens
for one post; `x-markdown-tokens` header). Truncate at heading boundaries, expose an
outline + `start_index`/`section`/`goal`. Absolutize against `document.baseURI`. Drop
skip-links, self-anchors, fragment-only links.

**Safety**: our path drives the user's real Firefox (cookies!) — every headless product
above fetches anonymously. Needs a container/private-context decision, redirect-chain
validation against private IP + `file:`/`about:`/`moz-extension:`, cache key including
selector/mode, and never sharing cache across auth contexts.

---

## PART 4 — Conclusion for the build

Claude's corrections to our diagnosis:
1. Cause 1 (shadow DOM) confirmed — it is a visibility bug, not timing. Still add a
   hydration check because the render follows the JSON fetch.
2. Cause 2 was mislabeled: our default does have Readability; it fails silently. Fix is
   "landmark first, scoring fallback, **and a `method`/`confidence` flag in the reply**".
3. Bytes are the wrong metric — build a fixture suite with must-include / must-exclude
   snippets (MDN compat tables would have been caught immediately).
4. Do not lean on Readability as the answer (F1 0.674 on documentation).

Priority (payload gain / cost): landmark scoping + confidence flag → link/heading
hygiene → shadow-piercing serializer + slot dedup → composite readiness → provenance
header → visibility/chrome pruning → outline/`section`/`goal` paging → two-stage
(trafilatura-class) extraction → SSRF/isolation → `Accept: text/markdown` pre-flight.
Skip neural extractors and a default LLM summary pass.
