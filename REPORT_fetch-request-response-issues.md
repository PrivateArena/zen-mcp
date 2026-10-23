# REPORT: fetch / request response-path issues (Firefox bridge → zen-mcp)

Scope: what the `fetch` and `request` tools get back from the userChrome.js MCP Bridge, where the bytes are altered or lost on the way in, and whether the fix belongs in the **bridge** (userChrome.js) or in the **MCP response path** (`internal/bridge` + the two tools).

Measured on: `zen-mcp` @ `a7b0293` + the `request` tool, Firefox/Zen bridge `127.0.0.1:9999`, 2026-10-23.
Fixture: `.scratch/bridge-probe/server.py` (gitignored — keep a copy of it; its routes and payload sizes are tabulated in [Appendix A](#appendix-a--fixture-server)). It serves a 263-byte entity-rich page, a JSON body with an opaque signature, a JSON body with a 64-bit integer, a 3,145,728-byte text file and a 2,200,087-byte HTML page.

## TL;DR — verdict and ownership

| # | Finding | Where the damage happens | Severity for agent use |
|---|---|---|---|
| 1 | Every response string is HTML-entity-decoded (`&amp;` → `&`, `&quot;` → `"`, `&#39;` → `'` …) | **Level 1 — zen-mcp** (`internal/bridge/bridge.go` → `sanitizeBridgeResponse`) | **High** for `request` (raw source / API payloads), low for `fetch` markdown |
| 2 | 1 MiB silent read cap makes a large body arrive as unparseable JSON | **Level 1 — zen-mcp** (`bridge.go:71`) | **High** for `fetch` (false "no readable content") and `request` |
| 3 | JSON response bodies are parsed by the bridge: big integers destroyed, `1e3`→`1000`, whitespace gone | **Level 2 — bridge** (already wrong in the raw JS response) | **High** for API work, medium for `fetch` |
| 4 | `request` is a *document navigation*, not an HTTP client: extension scripts land in the body, `Sec-Fetch-*` says `navigate` | **Level 2 — bridge** | **High** for `request` on HTML (19 KB of junk for a 263-byte page) |
| 5 | Sanitizer costs ~17 ms and 8.7 MB of garbage per 0.96 MB of response, with two regexes compiled per call | **Level 1 — zen-mcp** | Medium (throughput, GC pressure) |
| 6 | `FixMojibake` / `decodeLatin1ToUTF8` is an identity function — the repair it advertises never happens | **Level 1 — zen-mcp** | Low (latent, but every caller trusts the name) |
| 7 | Any `fetch`/`request` output over 24 KB is replaced by a `zcontext` handle — and the default `max_chars` is 30 KB, so this is the *normal* path for real pages | **Level 1 — zen-mcp** (`toolresponse` + `tokenoptimizer` + the tools' own defaults) | **Highest**: the caller gets a preview envelope instead of the page |

**Answer to the open question:** your instinct is right, but only for most of it. Issues **1, 2, 5, 6, 7 must be fixed at level 1** — they are zen-mcp behaviour and the bridge cannot help. Issues **3 and 4 cannot be fixed from the MCP side at all**: the bytes are already wrong when the bridge answers, so those are level-2 work. Sequencing: **7 first** (it is why the tools look broken on ordinary pages), then 1 and 2, then the bridge change for `request` semantics.

---

## Finding 1 — HTML entity decoding is applied to raw response bodies

### Evidence `[VERIFIED]`

Fixture page (`/entities.html`, 263 bytes on the wire) served `AT:&amp; copy:&copy; quote:&#39; nbsp:&nbsp; ellipsis:&hellip;`.

Raw bridge response (POST to `127.0.0.1:9999` directly, no Go involved) — **entities intact**:

```
'&amp;' present: True   '&copy;' present: True   '&#39;' present: True
'&nbsp;' present: True  '&hellip;' present: True '&quot;' present: True
```

Same request through `zrequest`:

```
<p>AT:& copy:© quote:' nbsp:  ellipsis:…</p>
<a href="/search?a=1&b=2">entity-encoded query link</a>
<pre>printf "hello %%s\n" world</pre>
```

Six entities rewritten inside a byte-exact payload. JSON API responses are hit the same way — `api.json` carries an opaque signature and it arrives mutated:

```
origin : {"signature": "MEUCIQD&amp;x1r+Rg==", "numeric": "&#39;quoted&#39;", ...}
tool   : {"signature":"MEUCIQD&x1r+Rg==","numeric":"'quoted'", ...}
```

A signature, ETag, CSRF token, or base64 fragment that contains `&amp;` / `&#39;` is silently corrupted: verification fails and the caller has no way to know why. `href="/s?a=1&amp;b=2"` (valid, escaped HTML) is rewritten into `?a=1&b=2`, which is a *different document* in the general case.

### Cause

`internal/bridge/bridge.go` — `CallBridge` runs `sanitizeBridgeResponse` over **every string in the response tree**, decoding 32 named entities plus `&#NNN;` / `&#xNNN;`. It was correct for `read`/`get_text`/`chat`, whose payloads are page text where decoding is what you want; it is wrong for `request`, whose entire contract is byte fidelity. The same decoder also corrupts `<pre>` blocks in `fetch` markdown (visible above: `&quot;` → `"`).

### Fix — level 1 (recommended)

Make decoding a **call-site decision**, not a transport default:

```go
// internal/bridge/bridge.go — transport stays verbatim
func CallBridge(ctx, action, params) (map[string]any, error)          // no string rewriting
func CallBridgeText(ctx, action, params) (map[string]any, error)      // decodes: read/get_text/chat
```

`fetch`/`search`/`browser` keep the decoded variant (behaviour unchanged, their tests stay green); `request` uses the verbatim one and gets real source. Cost: one thin wrapper plus 4–5 call-site edits.

Interim level-1 mitigation, if the split is not wanted: keep `CallBridge` but re-encode nothing — decode only `data.content`/`data.markdown`/`response` fields (the extraction points) instead of walking the whole tree.

---

## Finding 2 — the 1 MiB read cap turns big pages into a false failure

### Evidence `[VERIFIED]`

Same 3,145,728-byte resource (`/big.txt`), four independent measurements:

| Path | Bytes delivered | Envelope valid? |
|---|---|---|
| `curl` direct to origin | 3,145,728 | n/a |
| POST direct to bridge (`data` = full body) | 3,145,947 (2×), 1,867,649 (1×), 786,305 (1×) | **unterminated string** in the truncated runs |
| `zrequest` (MCP) | 1,048,576 (= `1<<20`) — twice; 786,305 once | unparseable |
| `zfetch` (MCP) | — | reports `has no readable content` |

Two separate truncation hazards:

1. **Level 1**: `internal/bridge/bridge.go:71` — `io.ReadAll(io.LimitReader(resp.Body, 1<<20))`. The cut is silent, so `json.Unmarshal` fails and `CallBridge` returns `{"raw": "<half a JSON envelope>"}`. `fetch` then reads no content and blames the page:
   `zfetch http://127.0.0.1:8099/big.txt` → `Error: ... has no readable content` (the page is 3 MB of perfectly readable text).
2. **Level 2**: the bridge truncates large `data` payloads *without any marker* (1,867,649 and 786,305 bytes observed for the same resource, direct from JS), producing the same unterminated JSON. That is nondeterministic, which is worse than a hard limit.

Consequence today: `fetch` cannot read any page whose extracted markdown exceeds ~1 MB (in practice the JS layer cuts earlier and unpredictably), and neither tool can tell a big page from a broken one. The JSON envelope plus escaping also means the usable body is a little under 1 MiB before the cap bites. `request` currently reports "larger than the bridge read limit" via `requestUncapturedBody`, which is honest but still a failure.

### Fix — level 1 first (real improvement), then level 2

* Level 1: read the full body up to a configurable ceiling (8–16 MiB is normal for a JSON tool response) and **propagate truncation**: `ErrResponseTooLarge{n int}` instead of a corrupted `{"raw": …}`. Then `fetch`/`request` can say "body ≥ N bytes, re-run with a `selector`, a smaller `max_chars`, or a Range request".
* Level 2 (needs the bridge): return an explicit `truncated: true` + `body_bytes` for the `request`/`read` actions, and honour a `max_bytes`/`Range` parameter so the JS side never hands over a mid-stream-cut body. Also worth asking for: `finalUrl` on `request` — redirects are currently invisible (the action returns only `success/status/statusText/headers/data`).

---

## Finding 3 — the bridge parses JSON bodies, so numbers are already wrong

### Evidence `[VERIFIED]`

Origin: `{"id_big":12345678901234567890,"id_str":"12345678901234567890","amount":1000.0,"sci":1e3,"nested":{"b":1,"a":2}}`

Direct bridge response (JS only — before any Go code sees it):

```
{"success":true, ..., "data":{"id_big":12345678901234567000,"id_str":"12345678901234567890",
 "amount":1000,"sci":1000,"tiny":0.000001,"seq":[3,1,2],"nested":{"b":1,"a":2}}}
```

Then `zrequest` re-marshals the parsed object, which also sorts keys:

```
{"amount":1000,"id_big":12345678901234567000,"id_str":"...","nested":{"a":2,"b":1},...}
```

`12345678901234567890` → `12345678901234567000`: a 64-bit id (Discord/Snowflake, nanosecond timestamps, `uint64` hashes) is silently destroyed by `JSON.parse` → double. `1000.0` → `1000` and `1e3` → `1000` break any float-vs-int or precision expectation. Key order survives the bridge but is lost in the Go re-marshal.

The tool already flags it (`| re-encoded: json (bridge parsed the body, original whitespace lost)`), but that only warns — the digits are gone.

### Fix — level 2 only (for fidelity), level 1 for the ordering/compactness

* **Level 2 (the real fix)**: the `request` action must return the body **as text**, exactly as received, and let the caller parse it (`data` = string; add `content_type`). Optionally keep a `data_json` companion. This one change also removes the `re-encoded:` caveat entirely.
* **Level 1 (cosmetic, optional)**: decode the envelope with `json.Decoder` + `UseNumber()` so Go does not re-sort keys or re-format numbers that survived the bridge.

---

## Finding 4 — `request` navigates a document; extensions rewrite the body

### Evidence `[VERIFIED]`

Same 263-byte fixture page:

| | Bytes | Content |
|---|---|---|
| Origin (`curl`) | **263** | 6 entities, 1 link, 1 `<pre>` |
| `zrequest` | **19,669** | same page **+ 19,441 bytes of injected `<script nonce="…">extendedCSS(…)</script>`** in `<head>` |

The injected script is a content-script payload (uBlock-style extended-CSS engine) that only ever lands in a *rendered document*, and it is already present in the direct JS bridge response, i.e. it is not added by zen-mcp. Corroborating header evidence from an origin echo of a `request` call:

```
"Sec-Fetch-Dest": "document", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-User": "?1",
"Upgrade-Insecure-Requests": "1", "Accept": "text/html,application/xhtml+xml,…", "User-Agent": "…Firefox/150.0"
```

A real `fetch()` from a chrome context sends none of those. Practical impact: `zrequest --max_chars 2000` on that page returns **zero bytes of the actual page** — the whole budget is consumed by the injected script. For HTML, `request` is therefore "browser-viewed page", not "curl".

This is also the tool's advantage — the fingerprint and session are what makes it unblockable — but the response side should not be post-processed by the page pipeline.

### Fix — level 2

Use a parent-process channel instead of a document load: `Services.newChannel`/`NetUtil.newChannel({uri, loadUsingSystemPrincipal: true})` (or `fetch(url, {credentials:"include"})` from the browser console scope) with `NetUtil.readInputStreamToString` + `OS.File`/`CTP` for bytes. That keeps cookies, real TLS stack and container identity while returning wire-exact bytes, and lets the action support binary bodies (today the bridge substitutes `[Binary Data: 5430 bytes]`, so images/ZIP/PDF are unreadable through either tool).

Level 1 can only *detect* this (e.g. flag `Sec-Fetch-Dest: document`-shaped answers or known injected-script signatures) — it cannot undo it.

---

## Finding 5 — the sanitizer is expensive and recompiles its regexes per call

### Evidence `[VERIFIED]` (benchmark, verbatim copy of `DecodeHTMLEntities`, 960 KB HTML fixture, AMD Ryzen 7 9700X, `go test -bench -benchmem -benchtime 20x`)

| Variant | ns/op | MB/s | B/op | allocs/op |
|---|---|---|---|---|
| current (32 × `strings.ReplaceAll` + 2 × `regexp.MustCompile`) | 16,511,186 | **58.1** | 8,701,556 | 24,119 |
| prebuilt `strings.Replacer` + package-level regexes | 6,516,850 | **147.3** | 7,069,421 | 24,062 |

Roughly **17 ms and ~9 MB of garbage per ~0.95 MB of response**, because the 32 replacements each copy the whole string, both `regexp.MustCompile` calls compile on every invocation, and the 32-entry entity map is rebuilt per call. `sanitizeBridgeResponse` then walks the whole response tree doing this per string. On the 3 MB body of Finding 2 the sanitizer alone costs ~50 ms and ~26 MB of short-lived allocations, on the way to a response that is then thrown away as unparseable.

### Fix — level 1 (cheap, safe)

Hoist the two regexes to package `var`, and build one `strings.NewReplacer` from `entityMap` at init. ~2.5× faster, fewer allocations, identical output for the entity set currently supported. Order-dependence caveat: `&amp;` must still be replaced last (or handled in a single pass) so `&amp;copy;` does not become `©`; `strings.Replacer` is leftmost-longest match, so the same table order must be asserted by a test.

---

## Finding 6 — `FixMojibake` does nothing

### Evidence `[VERIFIED]` (code read, `internal/bridge/bridge.go`)

```go
func FixMojibake(text string) string {
	if strings.Contains(text, "\u00e2") {
		result, err := decodeLatin1ToUTF8(text)   // ← identity: copies bytes, never decodes
		if err == nil { return result }
	}
	return text
}

func decodeLatin1ToUTF8(s string) string {
	decoded := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ { decoded = append(decoded, s[i]) }
	return string(decoded)   // == s
}
```

The copy is byte-for-byte equal to the input, so the advertised latin1→UTF-8 repair never happens (mojibake such as `â€™` survives in `read` output), and the guard allocates a full copy of the string whenever an `â` is present. Either implement the real repair (`latin1` byte → rune, then UTF-8 encode) or drop the function so callers stop believing it is handled.

---

## Finding 7 — the virtualization gate replaces the payload on the *default* path

### Evidence `[VERIFIED]`

`zrequest https://en.wikipedia.org/wiki/Ampersand --max_chars 100000` returns, instead of the page:

```
{
  "action_required": "Use Shell: zcontext -q virt_1792765850879_3613fff9",
  "analysis": { "confidence": 0.9, "file_type": "text", "volume_kb": "97.81", ... },
  "index_handle": "virt_1792765850879_3613fff9",
  "summary": "Successfully virtualized large output from tool 'request'.",
  "volume_kb": "97.81"
}
```

Sweeping the clamp on one URL locates the gate exactly (same page, only `max_chars` differs):

| `max_chars` | output | result |
|---|---|---|
| 4000 | 4.2 KB | ✅ provenance line + source inline |
| 20000 | 20 KB | ✅ provenance line + source inline |
| 25000 | 24.4 KB | ❌ `{ "index_handle": "virt_…" }` |
| 30000 (**default**) | 29 KB | ❌ virtualized handle |
| 100000 | 98 KB | ❌ virtualized handle |

`internal/toolresponse/response.go:305-312` virtualizes any tool text over `virtualizeLimit = 24 * 1024` (`internal/shell/tokenoptimizer/virtualize.go:16`) unless the tool is listed in `bypassTools`, whose default is `[session memory workspace think run]` (`internal/mcpcfg/config.go:173`). `fetchDefaultMaxChars` and `requestDefaultMaxChars` are both **30000** — the tools' own ceilings sit deliberately *above* that gate — so "page with more than 24 KB of content", the ordinary case for documentation, Wikipedia and news, never reaches the caller as content.

What the exchange costs:

* the provenance line (`url | title | status | truncated`) disappears, so the handle does not even record which URL it belongs to;
* `analysis.DetectFileType` mis-classifies the payload — HTML-derived markdown came back as `file_type: "yaml"` with `reading_advice: "Use 'yq keys' …"`, which is worse than no advice;
* reading the content costs a second round-trip (`zcontext -q virt_…`), and that path cannot re-narrow with `max_chars`/`selector`;
* the markdown document contract of the bridge content tools (provenance line + verbatim body) is silently replaced by a JSON envelope, which is exactly what `docOf` in the tests exists to catch.

### Fix — level 1, and the cheapest win in this report

Pick one, in order of preference:

1. Add `fetch`, `search`, `request` to `bypassTools`: they already clamp themselves, and `max_chars` is a far better-targeted budget than a global byte gate (their 100 KB ceiling is still small next to a context window).
2. Or keep the gate but virtualize only the **body**: emit the provenance line inline with the handle named in it (`body: virt_…`), so the caller knows what was fetched and can decide whether the round-trip is worth it.
3. Or lower the tools' defaults under the gate (e.g. `fetchDefaultMaxChars = 20000`) — this only moves the cliff, and makes the default answer shallow.

---

## Recommended order of work

1. **Level 1, day one** — no bridge update required, immediately measurable:
   1. Un-gate the content tools from virtualization (Finding 7) — one `bypassTools` entry, and it is why `zfetch`/`zrequest` look broken on ordinary pages.
   2. Split verbatim vs decoded transport (Finding 1) — `request`/`zrequest` becomes byte-exact.
   3. Configurable read ceiling + typed truncation error (Finding 2) — kills the misleading "has no readable content".
   4. `strings.Replacer` + hoisted regexes (Finding 5), plus a test pinning that `&amp;copy;` no longer collapses to `©` (the current per-call `map` iteration order makes that double-decode outcome nondeterministic).
   5. Implement or delete `decodeLatin1ToUTF8` (Finding 6).
   6. Optional: `UseNumber()` decoding so Go does not re-sort keys or re-format numbers that survived the bridge (Finding 3, cosmetic half).
2. **Level 2, bridge PR** (userChrome.js `request` action) — needs a bridge change and is the only way to get true fidelity:
   1. Return the body as text (no JSON parse) → removes the numeric/whitespace loss (Finding 3).
   2. Parent-process channel instead of document navigation → no extension injection, no `Sec-Fetch-Dest: document` (Finding 4); add binary-safe body and `finalUrl`, `truncated`, `body_bytes`, and a `max_bytes`/`Range` input (Finding 2).
   3. A level-1 companion worth having regardless: an `out` parameter on `request` that writes the body to a file and returns a summary + handle, so oversized and binary resources stop being a transport problem at all (see Finding 7 for why that gate already interferes).
3. When either lands: revisit `requestUncapturedBody` / `requestReEncodedTerm` in `internal/tools/request.go` — both are deliberate level-1 stopgaps and should be relaxed once the bridge reports truncation and text bodies honestly.

## Current mitigations already in the tree

* `requestUncapturedBody` (`internal/tools/request.go`) — turns a cap-truncated envelope into an explicit "could not be captured" error instead of an empty body.
* `re-encoded: json (…)` provenance term — the caller is told when a body was re-serialized, so the digits may already be wrong.
* `body: empty`, `status`, `content-type` provenance terms — distinguish a bodyless 204/HEAD from a lost payload.
* `binary placeholder` pass-through (`[Binary Data: 5430 bytes]`) — visible rather than silent, but no binary fetch is possible until Finding 4 is fixed.

---

## Appendix A — fixture server

`.scratch/bridge-probe/server.py` (gitignored), `python3 server.py 8099`:

| Route | Bytes | Purpose |
|---|---|---|
| `/entities.html` | 263 | `&amp; &copy; &#39; &nbsp; &hellip; &quot;`, entity-encoded query in `href` |
| `/api.json` | ~200 | opaque `&amp;`-bearing signature, `&#39;quoted&#39;`, literal NBSP |
| `/nums.json` | 142 | `12345678901234567890`, `1000.0`, `1e3`, key order |
| `/big.txt` | 3,145,728 | exceeds both read caps |
| `/big.html` | 2,200,087 | exceeds the caps through the `read`/Readability path |

Reproduce the level split (JS vs Go) with:

```bash
# JS only — proves Findings 3 and 4 come from the bridge, and 1 and 2 do not
curl -s -X POST http://127.0.0.1:9999/ -H 'Content-Type: application/json' \
  -d '{"action":"request","url":"http://127.0.0.1:8099/entities.html","method":"GET"}' | wc -c

# MCP path — Findings 1, 2 (and the re-encode of 3) show up here
zrequest --url http://127.0.0.1:8099/entities.html
zfetch   --url http://127.0.0.1:8099/big.html
```

## Appendix B — files touched by these findings

| File | Role in the findings |
|---|---|
| `internal/toolresponse/response.go`, `internal/shell/tokenoptimizer/virtualize.go` | the 24 KB virtualization gate that swallows content-tool output (Finding 7) |
| `internal/mcpcfg/config.go` | `bypassTools` default list (Finding 7) |
| `internal/bridge/bridge.go` | `sanitizeBridgeResponse`, `DecodeHTMLEntities`, `FixMojibake`, `1<<20` read cap (Findings 1, 2, 5, 6) |
| `internal/tools/request.go` | `requestBodyText` (re-encode), `requestUncapturedBody`, provenance terms (Findings 2, 3, 4 mitigations) |
| `internal/tools/fetch.go` | `fetchPageContent` + "has no readable content" wording (Finding 2 symptom) |
| `internal/tools/search.go` | shares the same decoded-payload assumption (Finding 1: intentional there) |
| userChrome.js MCP Bridge | `request` action implementation (Findings 3, 4) |
