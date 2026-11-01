---
name: provider-hunt
description: "Discover and integrate new AI providers into the Bridge by reverse-engineering their web chat UI"
framework: "zen-mcp"
triggers: provider hunt, add ai provider, providers.yaml
---

## 🎯 Mission

Add a new web AI provider to `JS/bridge/ai/providers.yaml` by navigating to the chat page, understanding the DOM, and creating a working profile.

## 🧰 Transport

Every probe below is a `zbrowser` CLI call — the generated wrapper around the bridge (`zbrowser -a <action> --<param> <value>`). Full action reference: the `firefox-bridge` skill (`MCP_BRIDGE_API.md`).

```bash
zbrowser -a ping                    # bridge + zen-mcp alive before you start
zbrowser -a chrome_logs             # bridge-side log when an action fails
zbrowser -a web_logs -i "$ID"       # page console log
ZENMCP_TIMEOUT=30 zbrowser -a ...    # client ceiling, so a wedged wait cannot hang the hunt
zpool -a list                        # what is running / queued
zpool -a cancel -p browser_N         # unstick a call that outlived its use
```

`--timeout` is overwritten by zen-mcp with the `browser` tool timeout (`toolConfigs.browser.timeout`, 40 min in this config), so a condition that never fires blocks the bridge queue for minutes. Prefer short, repeatable probes (`web_eval`, `get_text`, `is_visible`) over one long wait.

Keep the tab handle from `navigate` in `$ID` and pass `-i "$ID"` everywhere — DOM actions then run in a background tab without stealing focus, and nothing races your other tabs.

> [!IMPORTANT]
> Flag values arrive as **strings**. Strings, and `--timeout`-style ints, are fine on flags; booleans that must be `false`, coordinates, and objects (`target`, `headers`, `vars`) need `zbrowser --json '{"action":…}'`. A prompt with a comma in `-mes` splits into two prompts — use `--json` for real prose.

## 🧠 Mindset

**SIMPLE IS THE BEST. KEEP IT SIMPLE, STUPID.**

- Use the most basic Bridge actions: `click` > `trusted_click` > `human_click`, `type` > `trusted_type` > `native_type`. The simplest actions don't steal focus and are more reliable.
- No rabit-hole loops when debugging. If something doesn't work after 2 tries, try a different approach.
- Avoid `native_` and `trusted_` variants unless absolutely necessary.

## 📋 Procedure

### 1. Navigate & Observe

```bash
ID=$(zbrowser -a navigate --url <chat_url> --back_tab true | jq -r .id)
zbrowser -a read -i "$ID"
zbrowser -a get_interactive_map            # NOTE: maps the ACTIVE tab, -i is ignored
zbrowser -a screenshot -i "$ID"            # visual check; needs real visibility
```

### 2. Find the Chat Input

```bash
zbrowser -a web_eval -i "$ID" -c 'return <code>'
zbrowser -a web_eval -i "$ID" -c @/tmp/probe.js   # long probe: write JS to a file
```

Key selectors to look for:
- `textarea` with placeholder text
- `div[contenteditable="true"]`
- `div[role="textbox"]`

### 3. Focus & Type

Always try simplest first, then escalate:

```bash
zbrowser -a click -i "$ID" --selector "<selector>"
zbrowser -a type  -i "$ID" --selector "<selector>" --text "<text>"
```

### 4. Submit

Try **Enter first** (no focus steal):

```bash
zbrowser -a key_combo --combo Return
```

If Enter doesn't work, find and click the submit button:

```bash
zbrowser -a web_eval -i "$ID" -c 'return <code>'
zbrowser -a click -i "$ID" --by_text "Send"
```

### 5. Verify Submit Worked

Check that the input cleared and a response appeared:

```bash
zbrowser -a web_eval -i "$ID" -c 'return <code>'
```

If empty, Enter worked.

### 6. Wait & Extract Response

Wait for response to appear and extract text:

```bash
zbrowser -a wait_for_selector -i "$ID" --selector "<selector>"   # bounded by the injected tool timeout, not by --timeout
zbrowser -a read -i "$ID"
```

For streaming responses do not wait on the container alone — it exists from the first token. Poll the finish condition (`submit` re-enabled, `stop` button gone) with repeated `web_eval`, or `wait_for_function` on that exact predicate.

### 7. Identify Response Selectors

Find stable selectors for the response container:

```bash
zbrowser -a web_eval -i "$ID" -c 'return <code>'
```

Look for `div.prose`, `[class*="message"]`, or stable class patterns.

### 8. Match URL Pattern

```bash
zbrowser -a active_tab -i "$ID"
```

The `match` field in providers.yaml should be a substring of the domain. Check if chat URLs follow a pattern like `/c/...` to avoid matching non-chat pages.

### 9. Verify Submit Disabled State

While generating, the submit button often gets disabled. Check using:

```bash
zbrowser -a web_eval -i "$ID" -c 'return <code>'
zbrowser -a get_attribute -i "$ID" --selector "<submit>" --attribute _all
```

Look for `disabled`, `pointer-events-none`, `opacity-50` classes.

### 10. Differentiate User vs AI Response

To get ONLY AI responses (not the user's own message), examine parent class patterns:

```bash
zbrowser -a web_eval -i "$ID" -c 'return <code>'
```

- **AI response** ➜ parent has generic `flex flex-col gap-3` → no `bg-surface-raised`
- **User message** ➜ parent has `bg-surface-raised` bubble

### 11. Add to providers.yaml

```yaml
provider_name:
  url:       'https://example.ai/'
  match:     example.ai
  container: Personal
  brainstorm: false

  selectors:
    input:    'textarea[placeholder="Ask anything…"]'
    submit:   'button[type="submit"]'
    upload:   null
    stop:     null
    response: 'div.prose|last|markdown|clean_newlines'

  flow:
    focus:  click      # simplest: DOM click
    type:   type       # simplest: DOM type
    submit: enter      # try enter first; fallback to click
```

### 12. Test

```bash
zbrowser -a chat -p <provider_name> -mes "Say OK"
# prose prompts (commas) or screenshot payloads → exact types:
zbrowser --json '{"action":"chat","provider":"<provider_name>","message":"Say OK in one word."}'
```

Then confirm the response landed on disk / in the returned text, and re-run the failing step with `zbrowser -a chrome_logs` if it does not.

## 📐 Pipe DSL Reference

The `response:` field in providers.yaml supports Unix-style pipe chaining:

```
CSS_SELECTOR|pipe1|pipe2 || FALLBACK_CSS|pipe3
```

### Element Selection (choose which matched element to use)

| Pipe | Description | Example |
|------|-------------|---------|
| `first` | First NEW element (relative to `initialCount`) | `div.prose|first|text` |
| `last` | Last NEW element (default) | `div.prose|last|markdown` |
| `index:N` | Nth element relative to first new (0-based) | `div.prose|index:1|text` |

### Extraction Mode

| Pipe | Description | Default |
|------|-------------|---------|
| `markdown` | `SimpleMarkdown.fromElement()` - rich text | ✅ default |
| `text` | Plain `innerText` | |
| `html` | Raw `innerHTML` | |
| `attr:NAME` | Attribute value (e.g. `attr:src`, `attr:href`) | |

### Text Transforms

| Pipe | Description |
|------|-------------|
| `trim` | Strip leading/trailing whitespace |
| `clean_newlines` | Collapse 3+ newlines to 2 |
| `digits` | Keep only `0-9`, `.`, `,` |
| `lower` | Lowercase |
| `upper` | Uppercase |
| `truncate:N` | Truncate to N chars, append `…` |
| `join:SEP` | Join all elements with separator |

### Fallback

Use `||` between branches to try alternatives:

```yaml
response: '.primary-response|first|text || .fallback-response|last|markdown'
```

## 🔁 Flow Decision Tree

```
submit action:
  try enter → input cleared? → yes → keep "enter"
  no → find submit button → use "click"

type action:
  try type → text appears? → yes → use "type"
  no → try trusted_paste → text appears? → yes → use "trusted_paste"
  no → try native_type

focus action:
  click works → use "click"
  trusted_click needed → use "trusted_click" (still no focus steal)
  native_click → last resort (steals focus)
```

## 🧩 Response Selector Strategy

In order of preference:
1. `div.prose` — Common in markdown-rendered responses
2. `[class*="message"]` — Generic message containers
3. `article` — Semantic HTML
4. Role-based: `[role="status"]`, `[role="log"]`
5. Carousel: `.\\@container\\/carousel [class*="min-w-0"] .prose` — Arena-style layout

## 🚫 Pitfalls

- **DO NOT** use `read` to check response text during generation — it returns incomplete content. Use `wait_for_selector` first.
- **DO NOT** use `native_` or `trusted_` actions unless `click`/`type` fails — they steal focus and break concurrent workflows.
- **DO NOT** assume Enter works — verify by checking `textarea.value` is empty after submission.
- **BEWARE** of consent banners and cookie modals — dismiss them before testing.
- **BEWARE** of streaming responses — the response element exists immediately but content fills gradually. Wait for the submit button to become re-enabled.
- **BEWARE** of flag typing — `--back_tab false` is truthy, `--x 1280` is text. Prefer `zbrowser --json` when a param is not a plain string.