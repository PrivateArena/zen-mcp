---
name: provider-hunt
description: "Discover and integrate new AI providers into the Bridge by reverse-engineering their web chat UI"
---

## 🎯 Mission

Add a new web AI provider to `JS/bridge/ai/providers.yaml` by navigating to the chat page, understanding the DOM, and creating a working profile.

## 🧠 Mindset

**SIMPLE IS THE BEST. KEEP IT SIMPLE, STUPID.**

- Use the most basic Bridge actions: `click` > `trusted_click` > `human_click`, `type` > `trusted_type` > `native_type`. The simplest actions don't steal focus and are more reliable.
- No rabit-hole loops when debugging. If something doesn't work after 2 tries, try a different approach.
- Avoid `native_` and `trusted_` variants unless absolutely necessary.

## 📋 Procedure

### 1. Navigate & Observe


`browser({ action:navigate, url:<chat_url>})`
`browser({ action:read })`
`browser({ action:screenshot })`


### 2. Find the Chat Input

`browser({ action: web_eval, code: "return <code>" })`

Key selectors to look for:
- `textarea` with placeholder text
- `div[contenteditable="true"]`
- `div[role="textbox"]`

### 3. Focus & Type

Always try simplest first, then escalate:

`browser({ action:click, selector:"<selector>" })`
`browser({ action:type, selector:"<selector>", text:"<text>" })`

### 4. Submit

Try **Enter first** (no focus steal):

`browser({ action:key_combo, combo:"Return" })`

If Enter doesn't work, find and click the submit button:

`browser({ action: web_eval, code: "return <code>" })`


Then click it:

`browser({ action:click, byText:"Send" })`

### 5. Verify Submit Worked

Check that the input cleared and a response appeared:

`browser({ action: web_eval, code: "return <code>" })`

If empty, Enter worked.

### 6. Wait & Extract Response

Wait for response to appear and extract text:

`browser({ action: wait_for_selector, selector: "<selector>", timeout: <value> })`
`browser({ action: read })`

### 7. Identify Response Selectors

Find stable selectors for the response container:

`browser({ action: web_eval, code: "return <code>" })`

Look for `div.prose`, `[class*="message"]`, or stable class patterns.

### 8. Match URL Pattern

`browser({ action: active_tab })`

The `match` field in providers.yaml should be a substring of the domain. Check if chat URLs follow a pattern like `/c/...` to avoid matching non-chat pages.

### 9. Verify Submit Disabled State

While generating, the submit button often gets disabled. Check using:

`browser({ action: web_eval, code: "return <code>" })`

Look for `disabled`, `pointer-events-none`, `opacity-50` classes.

### 10. Differentiate User vs AI Response

To get ONLY AI responses (not the user's own message), examine parent class patterns:

`browser({ action: web_eval, code: "return <code>" })`

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

`browser({ action: chat, provider: "<provider_name>", message: "<text>" })`

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