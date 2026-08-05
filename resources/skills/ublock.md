---
name: ublock
description: Expert-level skills for uBlock Origin filter syntax, resources, and procedural filtering.
version: 0.5
framework: "ublock"
trigger: ublock
---

# uBlock Origin Filter Engineering Skill

Comprehensive guide for uBlock Origin (uBO) filter development. uBO extends standard EasyList syntax with powerful network modifiers, scriptlet injections, and procedural cosmetic operators.

## 🛡️ Core Principles

1.  **Specificity**: Prefer `example.com##...` over `##...`.
2.  **Efficiency**: Minimize DOM traversal. Use the Logger to profile filters.
3.  **Action over Hiding**: Use `:remove()` to prune DOM or `$redirect` to neuter requests.

---

## 🛰️ Static Network Filtering

### Syntax: `[pattern]$[modifiers]`
Network filters act on the request URL. `pattern` can be literal, regex, or a HOSTS-like hostname.

#### 🏁 Targeting Modifiers
| Modifier | Alias | Description |
| :--- | :--- | :--- |
| `1p` / `3p` | `first-party` / `third-party` | Weak partyness (matches base domain). |
| `strict1p` / `strict3p` | - | [MV2] Strict partyness (matches exact hostname). |
| `domain=...` | `from=` | Restrict filter origin. Supports `~`, Entities (`google.*`), and regex. |
| `to=...` | - | [DNR] Restrict request destination. Supports Entities and regex. |
| `denyallow=...` | - | Default-deny pattern with specific allowed exceptions. |
| `method=...` | - | Filter by HTTP method (e.g., `method=get\|post`). |
| `header=n:v` | - | Filter response based on header presence or value (supports regex). |
| `ipaddress=...` | - | [MV2] Filter by IP (e.g., `192.168.*`, `lan`, `loopback`). |
| `match-case` | - | Force case-sensitivity for regex patterns. |

#### 🛠️ Resource Types & Scope
| Type | Description |
| :--- | :--- |
| `all` | Blocks network + popups + doc + inline fonts/scripts. |
| `doc` | `document`. Blocks the main frame (Strict Blocking). |
| `frame` | `subdocument`. Blocks iframes. |
| `xhr` | `xmlhttprequest`. Blocks fetch/XHR. |
| `ping` | Blocks `audit` pings and `sendBeacon`. |
| `popup` | Prevents opening new windows/tabs (Popunder supported). |
| `inline-script` / `inline-font` | Disables via dynamic CSP. |

#### ⚡ Action & Transformation (Modifiers)
| Modifier | Alias | Description |
| :--- | :--- | :--- |
| `important` | - | Bypasses all exception (`@@`) filters. Use for 100% block. |
| `badfilter` | - | Discards an existing filter from a different list. |
| `redirect=tok` | - | Block and redirect to local neutered surrogate (e.g., `noopjs`). |
| `redirect-rule=tok`| - | Redirect ONLY if the request is already blocked by other means. |
| `removeparam=id` | `queryprune` | Strips query params. asign `_` to resolve regex ambiguity. |
| `csp=directive` | - | Inject or tighten Content Security Policy (Document only). |
| `permissions=pol` | - | Inject Permissions-Policy (e.g., `camera=()`). |
| `urlskip=steps` | - | [Trusted] Unpack tracker URLs (e.g., `?url -base64 +https`). |
| `replace=/a/b/` | - | [Trusted] Regex-replace response body (Firefox MV2). |
| `uritransform` | - | [Trusted] Transform path/query/hash. |

#### 🏷️ Entities & Hostname Regex (Extended)
- **Entity**: `google.*` matches `google.com`, `google.co.jp`, but NOT `google.evil.biz`.
- **Hostname Regex**: `/^nitter\.[^.]+\.[^.]+$/##...` targets complex multi-level subdomains.
- **Strict Hostname**: `/^example\.org$/##...` prevents matching subdomains like `www`.


---

## 🎨 Cosmetic & HTML Filtering

### Procedural Operators (DOM-based)
Used when plain CSS selectors are insufficient. Triggered by `##`.
**Mandatory Requirement**: Must be **specific** (prefixed with hostname).

| Operator | Syntax | Description |
| :--- | :--- | :--- |
| `:has(sel)` | `subj:has(sel)` | Selects `subj` if child matches `sel`. uBO uses **native `:has()`** if available, else procedural. |
| `:has-text(rx)`| `subj:has-text(rx)`| Selects `subj` if text content matches string or `/regex/`. Case-insensitive with `/.../i`. |
| `:upward(n/sel)`| `subj:upward(2)` | Moves up `n` levels or to the first ancestor matching `sel` CSS. |
| `:matches-css(p:v)`| `subj:matches-css(col: red)`| Matches computed style via `getComputedStyle`. Supports regex values. |
| `:matches-attr(a=v)`| `subj:matches-attr(id=/ad.*/)`| Matches attributes. Required for randomized attribute names/values. |
| `:matches-prop(p=v)`| `subj:matches-prop(naturalWidth=160)`| Matches JS properties of the DOM element (e.g., `naturalWidth`). |
| `:matches-path(p)`| `subj:matches-path(/shop)`| Passthrough if URL path matches `p`. Best used as **first** operator for performance. |
| `:matches-media(m)`| `subj:matches-media((min-width: 800px))`| Narrows filter based on media queries (responsive design logic). |
| `:min-text-length(n)`| `subj:min-text-length(100)`| Selects if `textContent.length >= n`. Often used in HTML filtering (`##^`). |
| `:not(procedural)`| `subj:not(:has(.ad))`| Negates procedural operators. For CSS-only negation, uses standard `:not()`. |
| `:xpath(expr)` | `subj:xpath(//div)` | Selects/filters via XPath. Can create entirely new result sets (subject is optional). |
| `:watch-attr(attr)`| `subj:watch-attr(class)`| Forces re-evaluation on attribute change. uBO normally ignores attribute mutations. |
| `:others()` | `subj:others()` | Selects everything *except* the descendants and ancestors of `subj`. (Reader Mode). |

#### ⚡ Performance & Optimization
- **Order Matters**: Put `:matches-path()` first to short-circuit evaluation.
- **Narrow the Subject**: `##.ad:has-text(Ad)` is faster than `##div:has-text(Ad)`.
- **Native vs Procedural**: Modern browsers support native `:has()`. uBO automatically converts `##.a:has(.b)` to native CSS for speed unless procedural-only features are used.


### Action Operators
Action operators perform a surgical modification on the selected element rather than just hiding it.

| Operator | Syntax | Description |
| :--- | :--- | :--- |
| `:remove()` | `subj:remove()` | Physically deletes the element from the DOM tree. |
| `:style(css)` | `subj:style(opacity: 0)`| Injects/overrides CSS styles directly on the element (`!important` allowed). |
| `:remove-attr(at)`| `subj:remove-attr(id\|class)`| Deletes specific attributes. Use `\|` to separate multiple. (Replaces `+js(ra)`). |
| `:remove-class(c)`| `subj:remove-class(hidden)`| Removes specific class(es) from the element. (Replaces `+js(rc)`). |

#### 🔥 Procedural Chaining Examples
- `example.com##.post:has(.sponsored):upward(1):remove()`: Find a sponsored post and remove its parent container.
- `example.com##:matches-path(/article) .paywall:style(display: none !important)`: Hide paywall only on article pages using direct style injection.
- `example.com##div:has-text(Advertisement):watch-attr(class):remove-class(hidden)`: If an "Advertisement" div is dynamically unbidden via class change, strip that class.


### HTML Filtering (Response-stream)
Prefix: `##^`. Acts before the browser parses HTML.
- `example.com##^script:has-text(ad_token)`: Removes script from stream.

---

## 💉 Scriptlet Injection (`##+js(name, args)`)

Injects pre-defined JavaScript "scriptlets" to modify page behavior. uBO scriptlets are often more efficient than general-purpose user scripts and require a trusted source for "trusted" variants.

### 🔌 Fundamental Lifecycle & Logging
- **Syntax**: `hostname##+js(scriptlet-name, arg1, arg2, ...)`
- **Escaping**: Commas in arguments must be escaped as `\,`. Backslashes as `\\`.
- **Aliases**: Many scriptlets have short aliases (e.g., `acs` for `abort-current-script`).
- **Targeting**: Can use `/regex/` for hostnames: `/img[a-z]{3}\.com/##+js(nowoif)`.
- **Tokens**: Special parameters like `runAt` (`loading`, `interactive`, `idle`, `end`, `complete`), `log`, or `debug` that modify behavior.
- **Logging**: Scriptlet execution is visible in the uBO Logger. Log output can be filtered using `log, [needle]`.

---

### 🛡️ Execution Control & Abort
Prevent scripts from running by trapping property access or stack traces.

| Scriptlet | Alias | Description | Parameters |
| :--- | :--- | :--- | :--- |
| `abort-current-script` | `acs` | Aborts an inline script accessing a prop. | `prop`, `pattern` (content), `srcPattern` |
| `abort-on-property-read`| `aopr` | Aborts if a property is read. | `prop` |
| `abort-on-property-write`| `aopw` | Aborts if a property is written. | `prop` |
| `abort-on-stack-trace` | `aost` | Aborts if stack trace matches pattern. | `prop`, `stackPattern` |
| `call-nothrow` | - | Wraps a function in try/catch to ignore errors. | `functionReference` |
| `noeval` | - | Globally disables `eval()`. | - |
| `noeval-if` | - | Disables `eval()` if payload matches pattern. | `pattern` |
| `noeval-silent` | - | Disables `eval()` without logging. | - |
| `no-floc` | - | [Obsolete] Dsiabled Google FLoC tracking. | - |

### 🛠️ Property & Value Manipulation
Modify global constants, JSON payloads, or XML responses.

| Scriptlet | Alias | Description | Parameters |
| :--- | :--- | :--- | :--- |
| `set-constant` | `set` | Sets a property to a constant value. | `prop`, `value` (e.g., `true`, `noopFunc`, `{}`, `[]`) |
| `trusted-set-constant` | `trusted-set`| [Trusted] Set arbitrary JSON values. | `prop`, `value` (JSON or `json:`) |
| `json-prune` | - | Prunes properties from `JSON.parse` output. | `prunePaths`, `needlePaths`, `stack` |
| `json-prune-fetch-response` | - | Prunes JSON from `fetch()` responses. | `prunePaths`, `needlePaths`, `varargs` |
| `json-prune-xhr-response` | - | Prunes JSON from `XHR` responses. | `prunePaths`, `needlePaths`, `varargs` |
| `evaldata-prune` | - | Prunes JSON data passed to `eval()`. | `prunePaths`, `needlePaths` |
| `xml-prune` | - | Removes elements/attrs from XML responses. | `selector`, `matchCondition`, `urlPattern` |
| `m3u-prune` | - | Prunes segments from `.m3u8` playlists. | `pattern`, `urlPattern` |
| `object-prune` | - | Prunes properties from JS objects. | `prunePaths`, `needlePaths` |
| `trusted-prune-inbound-object` | - | [Trusted] Prune object passed as argument. | `prop`, `argPos`, `prunePaths` |
| `trusted-prune-outbound-object` | - | [Trusted] Prune object returned by function. | `prop`, `prunePaths` |

### 🌐 Network & Storage Defusing
Neuter network requests or modify browser storage.

| Scriptlet | Alias | Description | Parameters |
| :--- | :--- | :--- | :--- |
| `prevent-fetch` | `no-fetch-if`| Neuter `fetch()` based on URL/options. | `conditions`, `responseDirective` |
| `trusted-prevent-fetch` | - | [Trusted] Same as `prevent-fetch` with custom body. | `conditions`, `responseBody` |
| `prevent-xhr` | `no-xhr-if` | Neuter `XHR` based on URL/method. | `conditions`, `responseDirective` |
| `trusted-prevent-xhr` | - | [Trusted] Same as `prevent-xhr` based on body. | `conditions`, `responseBody` |
| `trusted-replace-fetch-response` | `trusted-rpfr` | [Trusted] Regex replace `fetch` response. | `match`, `replace`, `propsToMatch` |
| `trusted-replace-xhr-response` | - | [Trusted] Regex replace `XHR` response. | `match`, `replace`, `propsToMatch` |
| `set-cookie` | - | Sets a restricted set of cookie values. | `name`, `value`, `path` |
| `trusted-set-cookie` | - | [Trusted] Set arbitrary cookies/expiry. | `name`, `value`, `expiry`, `path` |
| `remove-cookie` | `cookie-remover`| Removes cookies by name/pattern on events. | `namePattern`, `when` (event: `scroll`, `keydown`) |
| `set-local-storage-item` | - | Sets LocalStorage entries. | `key`, `value` (e.g., `$remove$`) |
| `trusted-set-local-storage-item` | - | [Trusted] Sets arbitrary Local/Session storage. | `key`, `value` (JSON or keywords like `$now$`) |
| `remove-cache-storage-item` | - | Removes Cache API items. | `namePattern`, `urlPattern` |
| `nowebrtc` | - | Disables WebRTC `RTCPeerConnection`. | - |
| `webrtc-if` | - | Restrict WebRTC to specific servers. | `matchPattern` |

### 🖱️ DOM & Event Interaction
Prevent detection or automate interactions via the DOM.

| Scriptlet | Alias | Description | Parameters |
| :--- | :--- | :--- | :--- |
| `addEventListener-defuser`| `aeld` | Prevents attaching event listeners. | `type`, `handlerPattern`, `runAt` |
| `trusted-click-element` | - | [Trusted] Automate clicks on elements. | `selectors`, `extraMatch`, `delay` |
| `set-attr` | - | Sets an attribute on matching elements. | `selector`, `attr`, `value` |
| `trusted-set-attr` | - | [Trusted] Sets arbitrary attribute values. | `selector`, `attr`, `value` |
| `remove-attr` | `ra` | Removes attributes (Deprecated for `:remove-attr`). | `attr`, `selector`, `tokens` |
| `remove-class` | `rc` | Removes classes (Deprecated for `:remove-class`). | `class`, `selector`, `tokens` |
| `remove-node-text` | `rmnt` | Deletes whole text of a DOM node. | `nodeName`, `matchPattern` |
| `trusted-replace-node-text` | `trusted-rpnt` | [Trusted] Regex-replace text in node. | `nodeName`, `match`, `replace` |
| `trusted-replace-outbound-text`| `trusted-rpot`| [Trusted] Replace text in outbound function call. | `method`, `match`, `replace` |
| `trusted-create-HTML` | - | [Trusted] Inject HTML into the DOM. | `parentSelector`, `htmlString`, `duration` |
| `prevent-innerHTML` | - | Blocks setting `innerHTML` on element. | `selector`, `matchPattern` |
| `trusted-override-element-method` | - | [Trusted] Override element prototype methods. | `methodPath`, `selector`, `disposition` |
| `trusted-prevent-dom-bypass` | - | [Trusted] Prevent scriptlet bypass via iframes. | `methodPath`, `targetProp` |

### 🧪 Browser & Timing Spoofing
Defuse timers, spoof variables, and handle popups.

| Scriptlet | Alias | Description | Parameters |
| :--- | :--- | :--- | :--- |
| `adjust-setInterval` | `nano-sib` | Speed up/slow down `setInterval`. | `callbackPattern`, `matchInterval`, `mult` |
| `adjust-setTimeout` | `nano-stb` | Speed up/slow down `setTimeout`. | `callbackPattern`, `matchDelay`, `mult` |
| `prevent-setInterval` | `nosiif` | Stop `setInterval` if it matches pattern. | `callbackPattern`, `delayMatch` |
| `prevent-setTimeout` | `nostif` | Stop `setTimeout` if it matches pattern. | `callbackPattern`, `delayMatch` |
| `prevent-requestAnimationFrame` | `norafif`| Defuse `requestAnimationFrame`. | `callbackPattern` |
| `spoof-css` | - | Spoof `getComputedStyle` results. | `selector`, `prop`, `value` |
| `prevent-canvas` | - | Disables Canvas APIs (fingerprinting). | `contextType` |
| `href-sanitizer` | - | Cleans tracking params from links. | `selector`, `tokens` |
| `disable-newtab-links` | - | Prevents links from opening new tabs. | - |
| `close-window` | `window-close-if`| Closes current tab if URL matches. | `pattern` |
| `prevent-window-open` | `nowoif` | Defuse `window.open()`. | `pattern`, `delay`, `decoyType` |
| `window.name-defuser` | - | Clears `window.name` tracking property. | - |
| `prevent-refresh` | `refresh-defuser`| Stop `<meta refresh>` reloads. | `delaySeconds` |
| `alert-buster` | - | Disables `alert()` dialogs. | - |
| `overlay-buster` | - | Removes common overlay dialogs. | - |

---

### 🏥 Specialized Redirects (Defusers)
Pre-defined surrogates for common tracking libraries.
- `ampproject_v0`: Removes AMP animation delays.
- `fingerprint2` / `fingerprint3`: Shims for FingerprintJS.
- `nobab` / `nobab2`: Defuses BlockAdblock.
- `nofab`: Defuses FuckAdblock.
- `popads`: Defuses PopAds.
- `prebid-ads`: Shim for Prebid.js.

### 🖼️ Empty Redirect Resources
Use as `$redirect=resource-name`.
- **Images**: `1x1.gif`, `2x2.png`, `3x2.png`, `32x32.png`.
- **Code**: `noop.js`, `noop.css`, `noop.html`, `noop.json`, `noop.txt`.
- **XML**: `noop-vast2.xml`, `noop-vast3.xml`, `noop-vast4.xml`, `noop-vmap1.xml`.
- **Media**: `noop-0.1s.mp3`, `noop-0.5s.mp3`, `noop-1s.mp4`.
- **Special**: `click2load.html` (replaces frame with a manual load widget).

### 🏷️ Surrogate Shims (URL-Specific)
Aliases for blocking specific common tracking scripts:
`amazon_ads.js`, `amazon_apstag.js`, `doubleclick_instream_ad_status.js`, `google-analytics_analytics.js`, `google-ima.js`, `googletagmanager_gtm.js`, `googlesyndication_adsbygoogle.js`, `scorecardresearch_beacon.js`, `outbrain-widget.js`, `hd-main.js`.

---

## 🧩 Advanced Directives

- `!#include [file]`: Modularize lists.
- `!#if env_firefox`: Environment-specific logic.
- `!#if cap_html_filtering`: Capability checks.
- `hostname regex`: `/^nitter\..*$/##.ad-unit`.

## 🛠️ Best Practices
- **Escaping**: Commas in args must be `\,`. Backslashes must be `\\`.
- **Regex**: Use `/.../` for regex literals. Anchor (`^` / `$`) for performance.
- **Trust**: User filters are only "trusted" in Dev mode or if configured via `trustedListPrefixes`.
