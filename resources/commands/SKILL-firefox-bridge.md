---
description: How to use MCP Bridge API for Firefox automation.
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: Firefox Bridge API

---
name: firefox-bridge
description: "Firefox MCP Bridge API"
framework: "ucjs"
---

# 🦊 Firefox MCP Bridge API (userChrome)

**Version**: `6.7.0` (`v95`) | **Endpoint**: `POST http://127.0.0.1:9999` | **Content-Type**: `application/json`

```bash
curl -X POST http://127.0.0.1:9999 -d  '<JSONBODY>'
```

Control Firefox directly from the parent process with zero detection. Supports containers, persistent sessions, and full tab interaction.

---

## 🛠 CORE ACTIONS

| Action | Description |
|--------|-------------|
| `ping` | Verify connection and get version |
| `reload` / `restart` | Hot-reload the Bridge script from disk |
| `activate` | Focus the Firefox window |
| `chrome_logs` | Last 300 internal bridge log entries |
| `web_logs` | Console logs from the active tab (returned in `data.logs`) |
| `active_tab` | Get current active tab info |
| `list_tabs` | List all tabs; optional `query` filters by title/URL |
| `list_containers` | List all Firefox contextual identities |
| `switch_tab` | Switch to tab by `index` |

```json
{"action": "ping"}
{"action": "list_tabs", "query": "youtube"}
{"action": "switch_tab", "index": 0}
```

### `web_eval`
Execute arbitrary JavaScript in the active tab. Must use `return` to get a result. Returns `{ result, logs[] }`.
```json
{"action": "web_eval", "code": "return document.title"}
```

---

## 🧭 NAVIGATION

### `navigate`
Navigate current or new tab to a URL (with smart URL fixing).

| Param | Description |
|-------|-------------|
| `url` | Target URL or search query |
| `new_tab` | Open in new foreground tab |
| `back_tab` | Open in new background tab |
| `container` | Container name or ID |
| `timeout` | ms to wait for page load (default 30000) |

```json
{"action": "navigate", "url": "https://google.com", "back_tab": true}
```

### `new_tab`
Open a new tab. Supports `back_tab` for background creation.
```json
{"action": "new_tab", "url": "https://google.com", "back_tab": true}
```

### `refresh`
Reload a tab. Hard reload (cache-bypass) by default.
- `index` (optional): Tab to reload (default: active).
- `hard` / `bypass_cache` (optional): Set `false` for normal reload (default `true`).
```json
{"action": "refresh"}
{"action": "refresh", "hard": false}
{"action": "refresh", "index": 2}
```

---

## 🤖 AI CHAT & VISUAL ANALYSIS ✨ FLAGSHIP

High-level orchestration for interacting with AI providers directly in their web interfaces. Bypasses API quotas and supports **Visual Reasoning** via automatic screenshot capture.

### `chat`
Full flow: Navigate → Capture → Upload → Prompt → Poll → Extract. Supports single-prompt single-provider queries, as well as multi-provider and multi-prompt execution in parallel.

| Param | Description |
|-------|-------------|
| `provider` | `"gemini"`, `"claude"`, `"duckduckgo"`, `"chatgpt"`, `"perplexity"`, `"nvidia"`, `"google"`, `"github"`. Can also be an **array of strings** to query multiple services in parallel. |
| `message` | Text prompt, or **array of text prompts** for multi-query execution. |
| `screenshot` | `true`/`"viewport"` (active tab), `"full"` (full-page), `".selector"` (element), `{ "byRole": "..." }` (semantic) |
| `path` | Local file path or array of paths to upload instead of screenshot |
| `container` | Firefox container (default: `"Personal"`) |

> [!NOTE]
> If `provider` and `message` arrays are of unequal length, the providers will rotate/wrap-around automatically (e.g., `messages = ["q1", "q2", "q3"]` and `providers = ["gemini", "claude"]` maps `q1 -> gemini`, `q2 -> claude`, `q3 -> gemini`).
> When running multiple queries, screenshot capture is executed once on the original active tab to prevent duplicate rendering and concurrency conflicts.

```json
# Basic text chat (backward compatible)
{"action":"chat","provider":"duckduckgo","message":"Write a funny poem about Firefox"}

# Parallel multi-chat (multi-question & multi-provider)
{"action":"chat","provider":["gemini","claude"],"message":["Explain closures in JS","Explain async/await in JS"]}

# Unequal sizes: provider rotates (q1 -> gemini, q2 -> claude, q3 -> gemini)
{"action":"chat","provider":["gemini","claude"],"message":["q1","q2","q3"]}

# Visual reasoning: analyze current tab
{"action":"chat","provider":"gemini","message":"Analyze this layout. Is the contrast high enough?","screenshot":true}

# Full-page audit
{"action":"chat","provider":"claude","message":"Audit this page for SEO.","screenshot":"full"}

# Targeted element analysis
{"action":"chat","provider":"gemini","message":"What game is in this thumbnail?","screenshot":".video-thumbnail-container"}
```

### `brainstorm`
Sequentially queries multiple web AI agents, caches responses under `JS/brainstorm_cache/[id]/`, then synthesizes a final answer. Resume-on-failure via `id`.
- `id`: Unique session ID (required; used to resume/retry).
- `prompt`: Question to ask agents (required to initialize).
- `min_cache_count` (optional): Minimum successful responses before orchestration.
- `container` (optional): Container name for web tabs.
```json
{"action":"brainstorm","id":"kb_go","prompt":"How to design Kontakt word builder in Go?","min_cache_count":3}
```

### `brainstorm_status`
Inspect state and files of an active/completed session.
```json
{"action":"brainstorm_status","id":"kb_go"}
```

---

## 🕷 CRAWL ENGINE

YAML-profile-driven web crawler. Supports `request` mode (direct fetch) and `navigate` mode (browser automation).

### `crawl`
Execute a crawl pipeline from a profile (`.yaml` in `JS/bridge/crawl/profiles/`).
- `profile`: Profile name.
- `vars` (optional): Template variables (e.g. `query`, `page`).
- `max_pages` (optional): Page ceiling (default from profile or 10).
- `zip` (optional): Set `true` to group parallel arrays into objects. Example: `{"name":["a"],"price":["1"]}` → `{"items":[{"name":"a","price":"1"}]}`. Also settable in profile under `extract.zip`.
- `markdown` / `md` (optional): Set `true` to include the full Markdown report inline (`report_path` is always available when cache is saved).
**Default:** Markdown report is written to `JS/crawl_cache/{id}/results.md` and referenced via `report_path` in the response. Inline text is omitted to save tokens.
```json
{"action":"crawl","profile":"shopee_search","vars":{"query":"split keyboard"},"max_pages":2,"zip":true}
```

**Default response** (no `markdown` flag):
```json
{
  "success": true,
  "id": "crawl_shopee_search_...",
  "results": [{"url": "...", "page": 1, "data": {"items": [...]}, "done": false}],
  "report_path": "/home/user/.mozilla/firefox/.../chrome/JS/crawl_cache/crawl_shopee_search_.../results.md"
}
```

`report_path` points to the cached Markdown file. Add `"markdown": true` to get the full text inline.

### `crawl_list`
List all available crawl profiles.
```json
{"action":"crawl_list"}
```

### Crawl Profile Schema
```yaml
mode: navigate                        # "navigate" (tab) or "request" (fetch)
url_template: "https://example.com/search?q={query}&p={page}"
container: Personal
wait_for: ".search-results"           # (navigate) CSS selector to wait for
wait_ms: 1000                         # (navigate) Stabilization pause in ms

extract:
  name: ".item-card .title|all|text|trim"
  price: ".item-card .price|all|text|trim|digits"
  link: ".item-card a.link|all|attr:href|prefix:https://example.com|clean_url"
  zip: true                     # Optional: zip parallel arrays → [{name,price,link},...]

pagination:
  mode: click                         # "click" | "url_increment" | "scroll" | "none"
  next_selector: "a.next-btn"
  done_selector: "a.next-btn.disabled"
  max_pages: 5
```

### Pipe Expression DSL
`selector | transform1 | transform2 || fallback_selector`

| Transform | Description |
|-----------|-------------|
| `all` | Return array of all matches (must be first) |
| `text` / `html` / `markdown` | Extract content |
| `trim` | Strip whitespace |
| `clean_newlines` | Normalize newlines, collapse 3+ to 2 |
| `digits` | Keep only digits, periods, commas |
| `attr:NAME` | Read HTML attribute |
| `prefix:STR` / `suffix:STR` | Prepend/append string |
| `lower` / `upper` | Change case |
| `int` / `float` | Cast to number |
| `url_decode` | Decode URI components |
| `clean_url` | Strip tracking params (`utm_*`, `fbclid`, `gclid`, `extraParams`, `sp_atk`, `xptdk`, `mc_*`) |
| `truncate:N` | Truncate string to N chars (default 100, appends `…`) |
| `\|\|` (OR) | Fallback if left is null/empty |
| `&&` (AND) | Evaluate both; return array |

---

## 🔄 WORKFLOW ENGINE

YAML-driven web automation for daily chores: login maintenance, reward collection, and routine page interaction. Built on top of the existing bridge actions (`navigate`, `wait_for_network_idle`, `trusted_click`, `type`, `get_text`, etc.).

### `workflow`
Execute a workflow by profile name or inline step list.

| Param | Description |
|-------|-------------|
| `profile` | Profile name (`.yaml` in `JS/bridge/workflow/profiles/`). |
| `steps` | Inline step array (alternative to `profile`). |
| `vars` | Template variables (e.g. `username`, `password`, captured values). |
| `maxSteps` | Hard step ceiling (default 100). |
| `maxRuntimeMs` | Wall-clock timeout in ms (default 300000 = 5 min). |
| `keepTabs` | Keep workflow-owned tabs open after completion (default `false`). |
| `dryRun` | Validate and resolve without executing (default `false`). |
| `saveCache` | Write `workflow_cache/{id}/` with `results.json`, `step_log.md`, `session.json` (default `true`). |

```json
{"action":"workflow","profile":"login_keepalive","vars":{"username":"me","password":"secret"}}
```

**Response:**
```json
{
  "success": true,
  "run_id": "wf_login_keepalive_1700000000000_123",
  "profile": "login_keepalive",
  "detail": "Workflow completed successfully",
  "steps_executed": 8,
  "report_path": "/home/user/.mozilla/.../chrome/JS/workflow_cache/wf_login_keepalive_.../step_log.md",
  "variables": { "welcome_msg": "Welcome back", ... },
  "steps": [ { "step": 1, "action": "navigate", "status": "success", ... }, ... ]
}
```

### `workflow_list`
List all available workflow profiles.
```json
{"action":"workflow_list"}
```

### Workflow Profile Schema
```yaml
name: login_keepalive
description: Navigate, fill credentials, verify login

container: Personal            # Optional: default container (Personal)
timeout: 30000                 # Default per-step timeout ms
maxSteps: 100                  # Hard step ceiling
maxRuntimeMs: 300000           # 5 minute wall-clock limit
keepTabs: false                # Keep tabs open after run

steps:
  # ── Simple action step ───────────────────────────────────────────────
  - action: navigate
    url: "https://example.com/login"
    back_tab: true             # true=background, false=foreground
    name: open_login           # Optional: named step for goto

  - action: wait_for_selector
    selector: "#username"
    timeout: 15000

  # ── Variable interpolation: {vars.field} ─────────────────────────────
  # Use {{ and }} for literal braces in typed text.
  - action: type
    selector: "#username"
    text: "{vars.username}"

  # ── Capture: write result into ctx.vars for later steps ──────────────
  - action: capture
    capture_action: get_text   # Bridge action to run (default: get_text)
    selector: ".welcome"
    save_as: welcome_msg       # Variable name to store result

  # ── Conditional branching ────────────────────────────────────────────
  - if:
      op: contains             # eq | neq | contains | not_contains | exists |
      left: "{vars.welcome_msg}"   not_exists | gt | gte | lt | lte | is_empty
      right: "Welcome"         # | is_not_empty
    then:
      - action: get_text
        selector: ".dashboard-summary"
        save_as: dashboard_status

  # ── Loop ─────────────────────────────────────────────────────────────
  - loop:
      count: 3                 # Max iterations (required)
      var: reward_index        # Optional: loop counter variable name
      while:                   # Optional: condition-based exit
        op: exists
        selector: ".next-page"
    steps:
      - action: click
        selector: ".next-page"
      - action: wait_for_network_idle
        idle_ms: 500

  # ── Goto ─────────────────────────────────────────────────────────────
  - action: scroll
    y: 500
  - goto: scroll_bottom        # Jump to named step

  # ── Error policy per step ────────────────────────────────────────────
  - action: trusted_click
    byText: "Claim"
    on_error: continue         # abort (default) | continue | retry_n
    retry_n: 2                 # Max retries (native actions: 0, capped at 3)
```

### Step Actions
Every step uses the existing bridge action names unchanged. Supported step `action` values are any valid bridge action: `navigate`, `wait_for_selector`, `wait_for_network_idle`, `wait_for_function`, `click`, `trusted_click`, `human_click`, `type`, `trusted_paste`, `trusted_type`, `native_type`, `native_paste`, `native_click`, `trusted_hover`, `hover`, `drag_and_drop`, `trusted_drag_and_drop`, `key_combo`, `trusted_key_combo`, `scroll`, `select_option`, `clear`, `focus`, `upload`, `get_text`, `get_attribute`, `is_visible`, `is_enabled`, `screenshot`, `web_eval`, `request`, `solve_challenge`, `get_cookie`, `get_storage`, `read`, `get_interactive_map`.

### Control Flow Steps

| Step type | Fields | Description |
|-----------|--------|-------------|
| `if` | `if` / `condition`, `then` (step list) | Execute `then` steps if condition is truthy. |
| `loop` | `loop.count`, `loop.var` (optional), `loop.while` (optional), `loop.steps` | Repeat `steps` up to `count` times, or while `while` condition holds. |
| `goto` | `goto` (step name string) | Jump to named step. |

### Variable Interpolation
Substitute `{vars.field}` in `url`, `selector`, `text`, `byText`, `byLabel`, `byName`, `byRole`, `byAltText`, `code`, `combo`, and `path` fields. Nested paths work: `{vars.user.name}`. Missing variables resolve to `""` in text/selector context; throw in expression context. Use `{{` and `}}` for literal braces.

### Capture Step
Write a bridge action's result into `ctx.vars` for use by later steps:
```yaml
- action: capture
  capture_action: get_text    # Which bridge action to run
  selector: ".price"
  save_as: current_price
```
If the capture action fails, `ctx.vars.current_price` is set to `null` and the step's `on_error` policy applies before any downstream `if`/`loop` is reached.

### Per-Step Error Policy

| Policy | Behavior |
|--------|----------|
| `abort` (default) | Stop the workflow immediately. |
| `continue` | Skip this step and proceed to the next. |
| `retry_n` | Retry up to `n` times (max 3) with exponential backoff + jitter. Only applies to transient infrastructure errors (`frame_timeout`, `network_idle_timeout`). Native/OS-level actions (`human_click`, `native_click`, `native_type`, `native_paste`, `drag_and_drop`, `hover`, `key_combo`) are `unsafe_to_retry: true` by default and never retry. |

### Tab Mode
- **Background by default** for DOM-level and trusted actions (`click`, `trusted_click`, `type`, `trusted_paste`, `trusted_key_combo`, `wait_for_*`, `get_text`, `screenshot`, `navigate`, `web_eval`, etc.).
- **Foreground by default** for OS-level/screen-coordinate actions (`human_click`, `native_click`, `native_type`, `native_paste`, `drag_and_drop`, `hover`, `key_combo`) because they call `OS.focusFirefox()` and compute screen coordinates. A loud log entry is emitted when focus is stolen.
- Override explicitly with `back_tab: true` or `back_tab: false` on any step.

### Safety Limits
- `max_steps` (default 100): hard ceiling on executed action steps. Control steps (`if`, `loop`, `goto`) do not count.
- `max_runtime_ms` (default 300000): wall-clock ceiling checked before each step.
- On breach: workflow aborts with `"max_steps exceeded"` or `"max_runtime_ms exceeded"`. Cache is still written.

### Cache Layout
```
JS/workflow_cache/
  wf_<profile>_<timestamp>_<rand>/
    results.json      # Step results array
    step_log.md       # Human-readable step-by-step log
    session.json      # Run metadata
    error_step<N>.png # Screenshot on step failure (if any)
```

---

## ⏳ WAITING & SYNCHRONIZATION

### `wait_for_selector` (aliases: `wait_for_node`, `wait_for_element`)
Block until an element appears. Supports all standard locators.
- `deep` (optional): Wait for element to **stop mutating** for `idle_ms` — essential for streaming/React content.
- `idle_ms` (optional): Stability duration in ms (default 500).
- `timeout` (optional): Max wait ms (default 45000).
```json
{"action":"wait_for_node","byText":"Processing","deep":true,"idle_ms":1000}
```

### `wait_for_function`
Poll arbitrary JS until it returns truthy. Replaces `time.sleep` guards.
- `code`: JS body (must `return` a value).
- `polling` (optional): Poll interval ms (default 500).
- `timeout` (optional): Total timeout ms (default 30000).
```json
{"action":"wait_for_function","code":"return !document.querySelector(\".progress-bar-container\")","polling":1000,"timeout":300000}
```

### `wait_for_network_idle` ✨
Wait until no HTTP requests are in-flight for `idle_ms` ms.
- `idle_ms` (optional): Quiet period ms (default 500).
- `deep` (optional): Wait for full stream completion (robust against HTTP/2, long-polling).
- `timeout` (optional): Max wait ms (default 20000).
```json
{"action":"wait_for_network_idle","idle_ms":1000,"deep":true}
```

---

## 🖱 INTERACTION

### Universal Locator Parameters
All interaction actions share these locator options:

| Param | Description |
|-------|-------------|
| `selector` | CSS selector (auto-scrolls into view) |
| `byText` | Partial case-insensitive text match |
| `byRole` | ARIA role; add `"name"` to narrow |
| `byName` | HTML `name` attribute |
| `byLabel` | Associated `<label>` text |
| `byAltText` | Image `alt` attribute |
| `relX`, `relY` | Relative click position 0.0–1.0 (default center) |
| `button` | `"left"` (default), `"right"`, `"middle"` |

### Click Actions

| Action | Description |
|--------|-------------|
| `human_click` | Native OS click via Bezier path — undetectable |
| `trusted_click` | Click via Firefox internal APIs — no focus stealing. Supports direct `x`/`y` coordinates |
| `click` | DOM-level click (no native OS path) |

```json
{"action":"human_click","byRole":"button","name":"Publish"}
{"action":"trusted_click","x":1280,"y":720}
{"action":"human_click","byText":"Context Item","button":"right"}
```

### Hover Actions

| Action | Description |
|--------|-------------|
| `hover` | Move mouse via `xdotool` (legacy) |
| `trusted_hover` | Move via Firefox internal APIs — no focus stealing |

```json
{"action":"trusted_hover","selector":"nav > a"}
```

### Drag & Drop

| Action | Description |
|--------|-------------|
| `drag_and_drop` | OS-level drag from `selector`/`source` to `target` |
| `trusted_drag_and_drop` | Via Firefox internal APIs — undetectable |

```json
{"action":"trusted_drag_and_drop","byText":"main tutorial","target":{"byName":"htmlcode"}}
```

### Text Input

| Action | Description |
|--------|-------------|
| `type` | DOM-level text input |
| `native_type` | OS-level via `xdotool` |
| `native_paste` | OS-level via `xclip` (faster for long text) |
| `trusted_type` | Via Firefox internal APIs (currently redirects to `trusted_paste`) |
| `trusted_paste` | Injects to internal Firefox clipboard + synthesizes `Ctrl+V` — bypasses OS clipboard managers |

```json
{"action":"trusted_paste","selector":"#input","text":"Super Secret Token"}
{"action":"native_paste","text":"Long description text..."}
```

### Keyboard

| Action | Description |
|--------|-------------|
| `key_combo` | Fire shortcuts via `xdotool` |
| `trusted_key_combo` | Synthesize native key combos via Firefox internal APIs |

```json
{"action":"trusted_key_combo","combo":"ctrl+a","selector":"textarea"}
{"action":"key_combo","combo":"Return"}
```

### Other Interaction

| Action | Description |
|--------|-------------|
| `focus` | DOM-level focus (low detection) |
| `select` | Select all text in an input/textarea |
| `clear` | Reset input to empty; fires `input`/`change` events |
| `select_option` | Pick `<select>` option by `value` or `label` |
| `scroll` | Scroll element into view or page by `x`/`y` pixels; `block`: `"start"`, `"center"` (default), `"end"` |
| `upload` | Attach a local file to a file input (supports video MIME types) |
| `solve_challenge` | Bypass Cloudflare/Turnstile via "Chaos Mouse Move" + automated element hunting |

```json
{"action":"scroll","selector":"#comments-section"}
{"action":"scroll","y":500}
{"action":"upload","selector":"input[type=file]","path":"/home/user/video.mp4"}
{"action":"select_option","selector":"#visibility-select","label":"Public"}
```

---

## 🔍 PAGE INTROSPECTION

| Action | Description |
|--------|-------------|
| `is_visible` | Returns `{ data: true/false }` — element visible (not hidden/zero-size/display:none) |
| `is_enabled` | Returns `{ data: true/false }` — element not disabled (checks `disabled`, `aria-disabled`) |
| `get_attribute` | Read any DOM attribute; use `"_all"` to get all attributes + `innerText` |
| `get_text` / `get_content` | Extract text or HTML from elements |
| `read` | Page content as compacted markdown (token-efficient) |
| `get_interactive_map` | Page skeleton of interactive/ARIA elements with IDs, tags, roles |

```json
{"action":"is_visible","byRole":"dialog"}
{"action":"get_attribute","selector":"#submit-btn","attribute":"_all"}
{"action":"read"}
{"action":"get_interactive_map"}
```

### `screenshot` ✨
Capture tab screenshot. Saves to `/tmp/firefox_bridge_ss_DATETIME.png` by default (returns path). Pass `base64: true` for inline data.

```json
{"action":"screenshot"}
{"action":"screenshot","path":"/home/user/my_screenshot.png"}
{"action":"screenshot","base64":true}
```

**Default response:** `{ "success": true, "data": "/tmp/firefox_bridge_ss_....png", "rect": { ... } }`  
**Base64 response:** `{ "success": true, "data": "data:image/png;base64,...", "path": null, "rect": { ... } }`

---

## 💾 DATA & STORAGE

### `get_cookie`
Extract cookies from Firefox's cookie database (supports multi-container).
- `domain` / `url` (optional): Target domain/URL (default: active tab's domain).
- `container` (optional): Container name or ID (default: active tab's container).
```json
{"action":"get_cookie","domain":"google.com","container":"Personal"}
```

### `get_storage`
Extract `localStorage` and `sessionStorage` from the active page.
```json
{"action":"get_storage"}
```

---

## 🧪 SCRIPTING

### `web_eval`
Execute JS in the active tab. `mcp.log()` entries appear in `data.logs`.
```json
{ "action": "web_eval", "code": "mcp.log('Working...'); return { title: document.title, url: location.href };" }
```
**Response:** `{ "success": true, "data": { "result": {...}, "logs": ["Working..."] } }`

### `request` ✨
Chrome-level `fetch` that **bypasses CORS**. Supports custom headers.
- `url`, `method` (default `"GET"`), `headers`, `body`, `timeout` (default 30000ms).
```json
{
  "action": "request",
  "url": "https://shopee.vn/api/v4/voucher/get_vouchers_by_shop?shop_id=123",
  "headers": { "Referer": "https://shopee.vn/" }
}
```

---

## 📓 GOOGLE COLAB (NATIVE BRIDGE)

Non-disruptive native API for Colab notebooks. No OS clipboard or focus required.

| Action | Params | Description |
|--------|--------|-------------|
| `colab_get_status` | — | Kernel status, RAM, Disk usage |
| `colab_get_cells` | — | All cells: index, id, type, text, outputs, executing |
| `colab_add_cell` | `type` (`"code"`/`"text"`), `index` | Add cell at index (default: end) |
| `colab_update_cell` | `index`, `text` | Update cell content |
| `colab_run_cell` | `index` | Execute cell |
| `colab_delete_cell` | `index` | Delete cell |
| `colab_clear_outputs` | `index` | Clear cell outputs |
| `colab_restart` | — | Restart runtime (handles confirmation dialog) |

```json
{"action":"colab_get_cells"}
{"action":"colab_update_cell","index":0,"text":"print(\"Hello!\")"}
{"action":"colab_run_cell","index":0}
```

---

## 🔑 Selector Priority Guide

Use in order of resilience when CSS classes are brittle:

| Method | Example | Resilience |
|--------|---------|------------|
| `byRole` + `name` | `"byRole": "button", "name": "Publish"` | ⭐⭐⭐⭐ Best |
| `byLabel` | `"byLabel": "Select Video"` | ⭐⭐⭐ Great |
| `byText` | `"byText": "PUBLISH"` | ⭐⭐⭐ Great |
| `byName` | `"byName": "video_id"` | ⭐⭐ Good |
| `byAltText` | `"byAltText": "Channel Logo"` | ⭐⭐ Good |
| Semantic CSS | `"selector": "button[aria-label='Publish']"` | ⭐⭐ Good |
| ID selector | `"selector": "#publish-btn"` | ⭐⭐ Good |
| Class selector | `"selector": ".ytcp-button-renderer"` | ⭐ Fragile |


---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=firefox-bridge`