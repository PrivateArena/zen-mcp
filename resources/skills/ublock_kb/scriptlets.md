# Resources Library
Source: https://github.com/gorhill/ublock/wiki/Resources-Library

-   Current RAW version of Resources can be found in files inside [src/js/resources/ directory ↪](https://github.com/gorhill/uBlock/tree/master/src/js/resources) and [src/web\_accessible\_resources directory ↪](https://github.com/gorhill/uBlock/tree/master/src/web_accessible_resources)
-   [General purpose scriptlets](#general-purpose-scriptlets)
-   [Defuser scriptlets](#defuser-scriptlets)
-   [Empty redirect resources](#empty-redirect-resources)
-   [URL-specific sanitized redirect resources](#url-specific-sanitized-redirect-resources-surrogates)
-   [Other (deprecated/outdated)](#other)
-   [Scriptlets and Redirects compatibility tables between uBlock Origin / AdGuard / Adblock Plus](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/compatibility-table.md)
-   [Glossary](#glossary)

* * *

## Available general purpose scriptlets

[](#available-general-purpose-scriptlets)

-   [abort-current-script](#abort-current-scriptjs-) _(acs / abort-current-inline-script / acis)_
-   [abort-on-property-read](#abort-on-property-readjs-) _(aopr)_
-   [abort-on-property-write](#abort-on-property-writejs-) _(aopw)_
-   [abort-on-stack-trace](#abort-on-stack-tracejs-) _(aost)_
-   [addEventListener-defuser](#addeventlistener-defuserjs-) _(aeld / prevent-addEventListener)_
-   [addEventListener-logger](#addeventlistener-loggerjs-) _(aell)_
-   [trusted-click-element](#trusted-click-elementjs-) \[Trusted\]
-   [set-constant](#set-constantjs-) _(set)_
-   [trusted-set-constant](#trusted-set-constantjs-) _(trusted-set)_ \[Trusted\]
-   [trusted-suppress-native-method](#trusted-suppress-native-methodjs-) \[Trusted\]
-   [trusted-override-element-method](#trusted-override-element-methodjs-) \[Trusted\]
-   [trusted-prevent-dom-bypass](#trusted-prevent-dom-bypassjs-) \[Trusted\]
-   [set-cookie](#set-cookiejs-)
-   [trusted-set-cookie](#trusted-set-cookiejs-) \[Trusted\]
-   [remove-cookie](#remove-cookiejs-) _(cookie-remover)_
-   [set-local-storage-item](#set-local-storage-itemjs-) / set-session-storage-item
-   [trusted-set-local-storage-item](#trusted-set-local-storage-itemjs-) / trusted-set-session-storage-item \[Trusted\]
-   [remove-cache-storage-item](#remove-cache-storage-itemjs-)
-   [call-nothrow](#call-nothrowjs-)
-   [prevent-setInterval](#prevent-setIntervaljs-) _(no-setInterval-if / nosiif / setInterval-defuser)_
-   [prevent-setTimeout](#prevent-setTimeoutjs-) _(no-setTimeout-if / nostif / setTimeout-defuser)_
-   [adjust-setInterval](#adjust-setIntervaljs-) _(nano-setInterval-booster / nano-sib)_
-   [adjust-setTimeout](#adjust-setTimeoutjs-) _(nano-setTimeout-booster / nano-stb)_
-   [trusted-create-HTML](#trusted-create-htmljs-) \[Trusted\]
-   [prevent-innerHTML](#prevent-innerHTMLjs-)
-   [prevent-xhr](#prevent-xhrjs-) _(no-xhr-if)_
-   [trusted-prevent-xhr](#trusted-prevent-xhrjs-) \[Trusted\]
-   [prevent-fetch](#prevent-fetchjs-) _(no-fetch-if)_
-   [trusted-prevent-fetch](#trusted-prevent-fetchjs-) \[Trusted\]
-   [trusted-replace-xhr-response](#trusted-replace-xhr-responsejs-) \[Trusted\]
-   [trusted-replace-fetch-response](#trusted-replace-fetch-responsejs-) _(trusted-rpfr)_ \[Trusted\]
-   [trusted-replace-argument](#trusted-replace-argumentjs-) \[Trusted\]
-   [set-attr](#set-attrjs-)
-   [trusted-set-attr](#trusted-set-attrjs-) \[Trusted\]
-   [remove-attr](#remove-attrjs-) _(ra)_
-   [remove-class](#remove-classjs-) _(rc)_
-   [remove-node-text](#remove-node-textjs-) _(rmnt)_
-   [trusted-replace-node-text](#trusted-replace-node-textjs-) _(trusted-rpnt / replace-node-text / rpnt)_ \[Trusted\]
-   [trusted-replace-outbound-text](#trusted-replace-outbound-textjs-) _(trusted-rpot)_ \[Trusted\]
-   [spoof-css](#spoof-cssjs-)
-   [prevent-canvas](#prevent-canvasjs-)
-   [href-sanitizer](#href-sanitizerjs-)
-   [disable-newtab-links](#disable-newtab-linksjs-)
-   [close-window](#close-windowjs-) _(window-close-if)_
-   [prevent-window-open](#prevent-window-openjs-) _(nowoif / no-window-open-if / window.open-defuser)_
-   [object-prune](#object-prunejs----fn)
-   [trusted-prune-inbound-object](#trusted-prune-inbound-objectjs-) \[Trusted\]
-   [trusted-prune-outbound-object](#trusted-prune-outbound-objectjs-) \[Trusted\]
-   [json-prune](#json-prunejs-)
-   [json-prune-fetch-response](#json-prune-fetch-responsejs-)
-   [json-prune-xhr-response](#json-prune-xhr-responsejs-)
-   [evaldata-prune](#evaldata-prunejs-)
-   [xml-prune](#xml-prunejs-)
-   [m3u-prune](#m3u-prunejs-)
-   [noeval](#noevaljs-)
-   [noeval-silent](#noeval-silentjs-)
-   [noeval-if](#noeval-ifjs-) _(prevent-eval-if)_
-   [no-floc](#no-flocjs-)
-   [prevent-requestAnimationFrame](#prevent-requestAnimationFramejs-) _(no-requestAnimationFrame-if / norafif)_
-   [nowebrtc](#nowebrtcjs-)
-   [webrtc-if](#webrtc-ifjs-)
-   [window.name-defuser](#windowname-defuser-)
-   [prevent-refresh](#prevent-refreshjs-) _(refresh-defuser)_
-   [overlay-buster](#overlay-busterjs-)
-   [alert-buster](#alert-busterjs-)

* * *

## Available defuser scriptlets

[](#available-defuser-scriptlets)

-   [ampproject\_v0](#ampproject_v0js-)
-   [fingerprint2](#fingerprint2js-)
-   [fingerprint3](#fingerprint3js-)
-   [nobab](#nobabjs-) _(BlockAdblock)_
-   [nobab2](#nobab2js-) _(BlockAdblock 4.2b)_
-   [nofab](#nofabjs-) _(FuckAdblock)_
-   [popads-dummy](#popads-dummyjs-)
-   [popads](#popadsjs-)
-   [prebid-ads](#prebid-adsjs-)

* * *

## Available empty redirect resources

[](#available-empty-redirect-resources)

-   `1x1.gif`
-   `2x2.png`
-   `3x2.png`
-   `32x32.png`
-   `noop.css`
-   `noop.html` (`noopframe`)
-   `noop.js`
-   `noop.json`[1.53.5rc11](https://github.com/gorhill/uBlock/commit/bd8a91ed3ad1fd6cdb4ee25ab468c631dc637e13)
-   `noop.txt`
-   `noop-0.1s.mp3`
-   `noop-0.5s.mp3`
-   `noop-1s.mp4`
-   `noop-vast2.xml` [1.59.1b6](https://github.com/gorhill/uBlock/commit/59a9a43a83)
-   `noop-vast3.xml` [1.59.1b6](https://github.com/gorhill/uBlock/commit/59a9a43a83)
-   `noop-vast4.xml` [1.59.1b6](https://github.com/gorhill/uBlock/commit/59a9a43a83)
-   `noop-vmap1.xml` [1.33](https://github.com/gorhill/uBlock/commit/1669d122df581146a1e67e912ed365a5983bb405)
-   `none`
-   `click2load.html`

* * *

## Available URL-specific sanitized redirect resources (surrogates)

[](#available-url-specific-sanitized-redirect-resources-surrogates)

-   [amazon\_ads.js](#amazon_adsjs-)
-   [amazon\_apstag.js](#amazon_apstagjs-)
-   [doubleclick\_instream\_ad\_status](#doubleclick_instream_ad_statusjs-)
-   [google-analytics\_ga.js](#google-analytics_gajs-)
-   [google-analytics\_analytics.js](#google-analytics_analyticsjs-)
-   [google-analytics\_inpage\_linkid.js](#google-analytics_inpage_linkidjs-)
-   [google-analytics\_cx\_api.js](#google-analytics_cx_apijs-)
-   [google-ima.js](#google-imajs-)
-   [googletagservices\_gpt.js](#googletagservices_gptjs-)
-   [googletagmanager\_gtm.js](#googletagmanager_gtmjs-)
-   [googlesyndication\_adsbygoogle.js](#googlesyndication_adsbygooglejs-)
-   [scorecardresearch\_beacon.js](#scorecardresearch_beaconjs-)
-   [outbrain-widget.js](#outbrain-widgetjs-)
-   [hd-main.js](#hd-mainjs-)

* * *

## General purpose scriptlets

[](#general-purpose-scriptlets)

-   Most scriptlet relies on `Object` _properties_ ([_methods_](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Object#Methods_of_the_Object_constructor)), altering them may not be the best idea (you should know what you are doing).
-   Some properties related more to browser APIs rather than JS language built-ins can behave in unexpected way. For example browser can override them without scriptlet noticing this. Keep this in mind when using them in scriptlet injection filers.
-   "Optional" for "string/_regular expression_" parameter defaults to "catch all" (`/.?/`) if not specified.
-   "String" parameter means plain character(s)/word(s), quotes will be taken literally, commas [must be escaped](https://github.com/uBlockOrigin/uAssets/commit/2bec415a9bc4f81b29be3bf083ef1a20552f39db#commitcomment-29327114) in regex literals: `/foo\x2cbar\u002cbaz/`, after [1.22.0](https://github.com/gorhill/uBlock/commit/d67340f14db6ce5b446ef0ff4586b5e4d31f1086#diff-b03ba512faa0934947e57d28dc99b43bL242) commas can be escaped by backslash character (`foo\,bar`).
-   "Regular expression" parameter means JavaScript [regular expression literal](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/Regular_Expressions#Writing_a_regular_expression_pattern).
-   Mime type is `application/javascript` if not present.
-   You can use the short alias form when available for scriptlet name.
-   You should omit the `.js` from the scriptlet name in scriptlet injection filters (eventually in some future this will be the official way to do this).
    -   Do **not** skip `.js` when the scriptlet is used with `redirect=`, only when used in `+js(...)`.
-   Crossed out resources are deprecated/removed.
-   Starting with [1.46.1b17](https://github.com/gorhill/uBlock/commit/81498474d6d440b032681aa9952d593749b39efb) support for regex-based values as target domain has been added. Use sparingly, when no other solution is practical from a maintenance point of view -- keeping in mind that uBO has to iterate through all the regex-based values, unlike plain hostname or entity-based values which are mere lookups. Related discussion: [uBlockOrigin/uBlock-issues#2234](https://github.com/uBlockOrigin/uBlock-issues/discussions/2234). Example: `/img[a-z]{3,5}\.buzz/##+js(nowoif)`.
-   The usage of named arguments is optional, positional arguments are still supported as documented. Named arguments is required to use "log" and/or "debug" arguments.
-   The logging/debugging capabilities work only in the **dev build** of uBO or if the advanced setting `filterAuthorMode` is set to `true`.
-   Starting with [1.57.3b1](https://github.com/gorhill/uBlock/commit/3b4f02db21), the old method of displaying scriptlets's logs in browser's dev console and via `(log, 1)` tokens is deprecated:
    -   now all scriptlets's logging output is automatically displayed in [the logger](https://github.com/gorhill/ublock/wiki/The-logger) & the logger's console pane.
    -   in the logger, toggling labels `info` and `error` in [filter input field](https://github.com/gorhill/ublock/wiki/The-logger#filtering-the-logger-output) can be used to reduce information displayed in the logger to informations and errors, including scriptlets, and with additional `not` operator selected, the opposite effect can be achieved.
    -   in the logger's console pane, toggling additionally [verbose mode button](https://github.com/gorhill/ublock/wiki/The-logger#verbose-mode), will cause console pane to print more logs from scriptlets.
    -   by default, scriptlets will print out only calls and informations related to their orders and targets,  
        to log _all_ calls without defusing any of them, a scriptlet without parameters can be used, example: `wikipedia.org##+js(aeld)`
    -   related discussions: [uBlockOrigin/uAssets/discussions/17907](https://github.com/uBlockOrigin/uAssets/discussions/17907) and [uBlockOrigin/uBlock-issues/discussions/3096](https://github.com/uBlockOrigin/uBlock-issues/discussions/3096)
-   The only filter lists deemed from a "trusted source" are uBO-specific filter lists (i.e. "uBlock filters -- ..."). The user's own filters from "My filters" are trusted only in the **dev build** of uBO. This behaviour can be overridden with the advanced setting `trustedListPrefixes`.
-   Tokens are parameters which modify the behavior of a scriptlet, they must be used after the required and optional parameters.

* * *

### abort-current-script.js [↪](https://github.com/gorhill/uBlock/blob/784ebb09050cb6617bd857f7c6a4311ac9649ce9/assets/resources/scriptlets.js#L35)

[](#abort-current-scriptjs-)

### acs.js /

[](#acsjs-)

### abort-current-inline-script.js /

[](#abort-current-inline-scriptjs-)

### acis.js

[](#acisjs)

Aborts execution of inline script (_throws_ `ReferenceError`) when attempts to access specified _property_ when text content or `src` attribute value (new in [1.37.0](https://github.com/gorhill/uBlock/commit/ebc42ae21e7900fafeaf1041038b94488b1d50e5)) of `<script>` _element_ matches specified text or _regular expression_.

Note that `acis.js` and `abort-current-inline-script.js` aliases are deprecated and can be removed in the future.

Parameters:

-   required, _property_ (chain of properties joined by `.`) attached to window object accessed inside `<script>` tag we want to break
-   optional, string/_regular expression_ matching in `<script>` _element_ content
-   optional, new in [1.37.0](https://github.com/gorhill/uBlock/commit/ebc42ae21e7900fafeaf1041038b94488b1d50e5), string/_regular expression_ matching in the decoded value of the `src` attribute of the `<script>` tag, when the attribute content is not a remote network address, but the actual inline script URL-encoded or base64-endcoded as a `data:` URI

Examples:

-   `weristdeinfreund.de##+js(acis, Number.isNaN)`
-   `tichyseinblick.de##+js(acis, Math, /\}\s*\(.*?\b(self|this|window)\b.*?\)/)`

Starting with [1.48.5b4](https://github.com/gorhill/uBlock/commit/edbe96a4010cff59a37c2aa2ec0fb89c48c913f9), you can use the logging abilites. (deprecated)  
(read about [the new method](#general-purpose-scriptlets) of logging scriptlets)

Tokens:

-   `log, 1`: output useful information at the dev console. (deprecated)
-   `debug, 1`: break at key locations in the scriptlet.

Also see:

-   [AdGuard `abort-current-inline-script`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#abort-current-inline-script)

* * *

### abort-on-property-read.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L96)

[](#abort-on-property-readjs-)

### aopr.js

[](#aoprjs)

Aborts execution of script (_throws_ `ReferenceError`) when attempts to read specified _property_. Writes are ignored.

Parameters:

-   required, _property_ (chain of properties joined by `.`) attached to window object

Examples:

-   `tagesspiegel.de##+js(aopr, Notification)`

Also see:

-   [AdGuard `abort-on-property-read`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#abort-on-property-read)

* * *

### abort-on-property-write.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L150)

[](#abort-on-property-writejs-)

### aopw.js

[](#aopwjs)

Aborts execution of script (_throws_ `ReferenceError`) when attempts to write specified _property_.

Parameters:

-   required, _property_ (chain of properties joined by `.`) attached to window object that will be overwritten

Examples:

-   `yggtorrent.*##+js(aopw, Fingerprint2)`

Also see:

-   [AdGuard `abort-on-property-write`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#abort-on-property-write)

* * *

### abort-on-stack-trace.js [↪](https://github.com/gorhill/uBlock/blob/793e2c78963ba86c8d36b950807ce952f7199c1f/assets/resources/scriptlets.js#L194)

[](#abort-on-stack-tracejs-)

### aost.js

[](#aostjs)

#### _Experimental, under development_

[](#experimental-under-development)

New in [1.29.3rc9](https://github.com/gorhill/uBlock/commit/b735ac6b6abab7d5f45e15bbba3b4ba6cbf43935)

Aborts execution of script (_throws_ `ReferenceError`) when attempts to access specified _property_ when _stack trace_ matches specified text or _regular expression_. [Internal discussion](https://github.com/orgs/uBlockOrigin/teams/ublock-issues-volunteers/discussions/237?from_comment=59)

Parameters:

-   required, _property_ (chain of properties joined by `.`) to trap in order to launch the stack trace matching code, ex. `Math.random`
-   optional, string/_regular expression_, the needle to match against the stack trace. If the empty string, only log. Prepend pattern with `!` to test for unmatched patterns in stack trace (New in [1.51.1b6](https://github.com/gorhill/uBlock/commit/84cc69aa10bc9c8045ed94b528ddc158c06ae7ab))

The scriptlet logs when there is a match. If verbose mode is enabled, it will log both matches/no matches.

Stack trace is normalized, but there still can be differences (Chromium vs Firefox) because of different format of stack trace.

There is a special string which can be used to match inline script context - `inlineScript`.

Though the stack trace is rendered in the console using new line to separate the stack trace lines, internally `\t` is used. The reason is to be more easily be able to create regex-based needle when using regex `.` character class.

The stack trace is prepended with `stackDepth:...` in order to allow to filter on stack depth, however higher depth values can likely differ between Chromium and Firefox.

Firefox often reports `injectedScript`, attempt has been made to convert entries in Chromium which seems to correspond to this, so that both browser families will report `injectedScript`.

The column value is normalized to 1, however there is too much discrepancy between browser families for that value to be of any use.

Filtering according to reported line numbers (`...:1234:1`), will not be reliable for inline scripts, since the line at which those inline scripts are located will vary from one page to another. It should be reliable for when the stack trace entry is for code in a JS file.

Also see:

-   [AdGuard `abort-on-stack-trace`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#abort-on-stack-trace)

* * *

### addEventListener-defuser.js [↪](https://github.com/gorhill/uBlock/blob/07d3c96261656e44f674550fbde50da8f6a15acc/assets/resources/scriptlets.js#L308)

[](#addeventlistener-defuserjs-)

### aeld.js /

[](#aeldjs-)

### prevent-addEventListener.js

[](#prevent-addeventlistenerjs)

Prevents attaching event listeners.

Parameters (when using positional arguments):

-   optional, string/_regular expression_, name of the event listener to defuse
-   optional, string/_regular expression_ matching in stringified handler function, narrows down defusing to specific handler

Examples:

-   `vev.io##+js(aeld, adb.updated)`
-   `newser.com##+js(aeld, load, Object)`
-   `vivo.sx##+js(aeld, , preventDefault)`
-   `vidto.me##+js(aeld, /^(?:click|mousedown|mousemove|touchstart|touchend|touchmove)$/, system.popunder)`

Tokens:

-   "runAt": when this parameter is present, uBO will take it into account to possibly defer defusing the event listener ([New in 1.49.3b4](https://github.com/gorhill/uBlock/commit/3c12173dfe4eea7c4b6758c556ed2dd5fcdbdd99)):
    -   end: execute scriptlet at `DOMContentLoaded` event ("interactive")
    -   idle: execute scriptlet at `load` event ("complete")
-   "elements": if present, will be used to apply the scriptlet to only elements matching the selector ([New in 1.55.1b1](https://github.com/gorhill/uBlock/commit/060f9d68fc641d08642a42a3903ccc89634735dc))
    -   must be a valid CSS selector (also `window` and `document` elements can be used as a selector ([New in 1.58.1b5](https://github.com/gorhill/uBlock/commit/91ee5bdeae6981cc20038b6610ab5c16b58623cf)))
-   "debug": an integer value telling when to break into the debugger, useful to inspect the debugger's call stack.
    -   1: break into the debugger when both type and pattern match, so effectively when defusing is taking place.
    -   2: break into the debugger when either type or pattern matches.

● "log": (deprecated)

▬▬► SPOILER START ◄▬▬

-   "log": an integer value telling when to log (see [uBlockOrigin/uAssets#17907](https://github.com/uBlockOrigin/uAssets/discussions/17907)):
    -   1: log only when both type and pattern matches, i.e. when a call to `addEventListener()` is defused
    -   2: log when either the type or pattern matches
    -   3: log all calls to `addEventListener()`

Examples:

-   `wikipedia.org##+js(aeld, /mouse/, /.^/, log, 2)`
-   `wikipedia.org##+js(aeld, , /.^/, log, 2)`
-   `wikipedia.org##+js(aeld, , , log, 1)`
-   `wikipedia.org##+js(aeld, _, , log, 3)`

The first filter will log calls to `addEventListener()` which have the pattern "mouse" in the event type (so "mouseover", "mouseout", etc.) **without defusing any of them** (because pattern can't match _anything_).

The second filter will log all calls **without defusing any of them** (because type can't match _anything_).

The third filter will log and defuse _all_ calls to `addEventListener()`

The fourth filter will log _all_ calls to `addEventListener()` **without defusing any of them**

▬▬► SPOILER END ◄▬▬

(read about [the new method](#general-purpose-scriptlets) of logging scriptlets)

Examples:

-   `jpvhub.com##+js(aeld, click, popMagic, runAt, idle)`
-   `subkade.ir##+js(aeld, click, return"undefined", elements, a.indirect)`
-   `wikipedia.org##+js(aeld)`

The third filter will log _all_ calls to `addEventListener()` **without defusing any of them**

Also see:

-   [AdGuard `prevent-addEventListener`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#prevent-addEventListener)

* * *

### aell.js /

[](#aelljs-)

### addEventListener-logger.js [↪](https://github.com/gorhill/uBlock/blob/07d3c96261656e44f674550fbde50da8f6a15acc/assets/resources/scriptlets.js#L352)

[](#addeventlistener-loggerjs-)

Removed in [1.48.1b3](https://github.com/gorhill/uBlock/commit/439951824af608bd445ec458f837fa39f366d75f).

Logs to the console event listeners created on page.

The logging or debugging of `addEventListener()` calls can now be done with the [`addEventListenerDefuser`](#addeventlistener-defuserjs-) scriptlet, which now supports named arguments.

Also see:

-   [AdGuard `log-addEventListener`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#log-addEventListener)

* * *

### remove-cookie.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L942)

[](#remove-cookiejs-)

### cookie-remover.js

[](#cookie-removerjs)

Removes current page cookies specified by name. For current domain, wildcard (dot) subdomain(s), after [1.28.0](https://github.com/gorhill/uBlock/commit/c4d39d37632fbee4d513116641a282ed2a48c89d) also for domain one level above `www`, current and `/` path, script accessible (HttpOnly=false), on load and before unload.

Caveats: cookies set for higher level domain will not be removed. For example, if current page domain is `page.example.com`, cookies set for `example.com` will not be removed. One exception is `www` subdomain, which will work after [1.28.0](https://github.com/gorhill/uBlock/commit/c4d39d37632fbee4d513116641a282ed2a48c89d).

Parameters:

-   optional, string/_regular expression_, matching in the name of the cookie

Tokens:

-   "when": when this parameter is present, it adds ability to trigger cookie removal on specific events (with delay 500ms):
    -   when "when" is not present: executes on page load and before unload
    -   `scroll`: executes when scrolling page (since [1.53.5rc3](https://github.com/gorhill/uBlock/commit/ef311ddbec9e50e032cfce062bd6c69aed5ae977))
    -   `keydown`: executes when key pressed (since [1.53.5rc4](https://github.com/gorhill/uBlock/commit/3db46c1728a2a93fd103c258a7e73a60d84c95fd))

Examples:

-   `subdivx.com##+js(cookie-remover, ref_cookie)`
-   `discovermagazine.com##+js(remove-cookie, AWSALBTG, when, scroll)`  
    (solves: ['remove-cookie' scriptlet sometimes does not work](https://github.com/uBlockOrigin/uBlock-issues/issues/3293))

Also see:

-   [AdGuard `remove-cookie`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#remove-cookie)

* * *

### csp.js [↪](https://github.com/uBlockOrigin/uAssets/blob/2c68a4f5456e4677cec76f2784d2c1d7abc36efb/filters/resources.txt#L1849)

[](#cspjs-)

Removed. Deprecated by `$csp` network filter option.  
Applies content security policy by inserting `<meta http-equiv=Content-Security-Policy content="*directive*">` tag to html `<head>` _element_. Read more at [https://www.w3.org/TR/CSP2/#delivery-html-meta-element](https://www.w3.org/TR/CSP2/#delivery-html-meta-element)  
[Content Security Policy Quick Reference Guide](https://content-security-policy.com/)

Parameters:

-   required, valid Content Security Policy directive

* * *

### call-nothrow.js [↪](https://github.com/gorhill/uBlock/blob/e93117cbb607472a830e1c0653dfbddde4c965fc/assets/resources/scriptlets.js#L1984)

[](#call-nothrowjs-)

New in [1.48.1b0](https://github.com/gorhill/uBlock/commit/e93117cbb607472a830e1c0653dfbddde4c965fc).

Prevents a call to an existing function from throwing an exception. It encloses existing functions in this block and ignores the exception:

```
try {
 \[existing function\]
}
catch() {
 \[ignore when throws\]
}
```

It will return `undefined` because returning variable is never set.

The exception will be caught by the scriptlet and neutralized. The first argument must be a reference to a function call. At the moment, the function call must exist at the time the scriptlet is called.

Parameters:

-   required, _a reference to a function call_

Examples:

-   `example.com##+js(call-nothrow, Object.defineProperty)`

Also see:

-   [AdGuard `call-nothrow`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#call-nothrow)

* * *

### disable-newtab-links.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L869)

[](#disable-newtab-linksjs-)

Prevents creating new tabs/windows by deactivating links with `target` attribute.

Parameters:

-   none

Examples:

-   `there.to##+js(disable-newtab-links)`

To prevent new tabs/windows by specifying the location URL, see: [`window-close-if.js`](https://github.com/gorhill/ublock/wiki/Resources-Library#window-close-ifjs)

Also see:

-   [AdGuard `disable-newtab-links`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#disable-newtab-links)

* * *

### evaldata-prune.js [↪](https://github.com/gorhill/uBlock/blob/f3b720d532c7a42a6ad5167e3b6f860004b4c2b6/assets/resources/scriptlets.js#L1039)

[](#evaldata-prunejs-)

New in [1.49.3rc15](https://github.com/gorhill/uBlock/commit/c8de9041917b61035171e454df886706f27fc4f3)

Intercepts calls to `eval()` and will work only if what is passed to `eval` can be parsed as JSON.

For parameters, see: [json-prune](#json-prunejs-)

When no "prune paths" argument (first parameter) is provided, the scriptlet is used for logging purpose and the "needle paths" argument (second parameter) is used to filter logging output.

Examples:

-   `m.nivod4.tv##+js(evaldata-prune, entity.commercial)`

Also see:

-   [AdGuard `evaldata-prune`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#evaldata-prune)

* * *

### object-prune.js [↪](https://github.com/gorhill/uBlock/blob/66e3a1ad47162a89709e99072e5eaecdd83cb633/assets/resources/scriptlets.js#L786) \* (fn)

[](#object-prunejs----fn)

New in [1.51.1b14](https://github.com/gorhill/uBlock/commit/cfac880cf51d8c652ce34ba7fee5e1a45c12ca1c) \* (fn)

\[Documentation to be completed\]

* * *

### trusted-prune-inbound-object.js [↪](https://github.com/gorhill/uBlock/blob/66e3a1ad47162a89709e99072e5eaecdd83cb633/assets/resources/scriptlets.js#L4633)

[](#trusted-prune-inbound-objectjs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet)

New in [1.52.3rc0](https://github.com/gorhill/uBlock/commit/1c9da227d714250c0b6319e2f635998f9869a70d)

To perform object pruning for any given call which has an object as argument (hence "inbound").

Parameters:

-   required, the name of the property to trap. Must be a function, and must exist when the scriptlet tries to install the trap.
-   required, the position of the object to prune in the argument list when the trapped function is called. The position is 1-based and must be an integer greater than 0.
-   required, the properties to prune (as with [`json-prune`](#json-prunejs-))
-   optional, the properties which must all be present for pruning to occur (as with [`json-prune`](#json-prunejs-))

Varargs:

-   `dontOverwrite, 1`: do not modify the target inbound object.
-   `stackToMatch`: string or regular expression that must match the current function call stack trace. Prepend pattern with `!` to test for unmatched patterns in stack trace.

Examples:

1.  Remove `title` and `name` properties before passing the object to `JSON.stringify` call:
    
    ```
    example.org##+js(trusted-prune-inbound-object, JSON.stringify, 1, title name)
    ```
    
2.  Remove `status` property before passing the object to `Object.keys` call but do not modify caller's instance of the object:
    
    ```
    example.org##+js(trusted-prune-inbound-object, Object.keys, 1, status, , dontOverwrite, 1)
    ```
    

Also see:

-   [AdGuard `trusted-prune-inbound-object`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-prune-inbound-object)

* * *

### trusted-prune-outbound-object.js [↪](https://github.com/gorhill/uBlock/blob/66e3a1ad47162a89709e99072e5eaecdd83cb633/assets/resources/scriptlets.js#L4713)

[](#trusted-prune-outbound-objectjs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-1)

New in [1.52.3rc3](https://github.com/gorhill/uBlock/commit/86f0d6dd978b79a34ffa257825539fb4446759a4)

Essentially a complement of [`trusted-prune-inbound-object`](#trusted-prune-inbound-objectjs-) added in [1.52.3rc0](https://github.com/gorhill/uBlock/commit/1c9da227d714250c0b6319e2f635998f9869a70d)

To perform object pruning on any object returned synchronously by any given call.

Parameters:

-   required, the name of the property to trap. Must be a function, and must exist when the scriptlet tries to install the trap.
-   required, the properties to prune (as with [`json-prune`](#json-prunejs-))
-   optional, the properties which must all be present for pruning to occur (as with [`json-prune`](#json-prunejs-))

The scriptlets `json-prune` and `evaldata-prune` essentially perform the same function, and will eventually be rewritten to internally delegate to generic `trusted-prune-outbound-object`.

* * *

### json-prune.js [↪](https://github.com/gorhill/uBlock/blob/d338e4c4b6caf339873a60c9d48fde58e9a495ce/assets/resources/scriptlets.js#L365)

[](#json-prunejs-)

New in [1.23.0](https://github.com/gorhill/uBlock/commit/2fd86a66fcc2665e5672cc5862e24b3782ee7504)

Intercepts calls to `JSON.parse()` and `Response.json()`[New in 1.31.0](https://github.com/gorhill/uBlock/commit/13f92756befaa9a8d3ba1615bd7abc7075758c67). If the result of the parsing is an Object, remove specified properties from the result before returning to the caller.

Parameters:

-   optional, string, a list of space-separated properties to remove
-   optional,
    -   string, a list of space-separated properties which must be all present for the pruning to occur; OR
    -   string/_regular expression_, for logging purposes, matching in stringified JSON payloads (New in [1.27.0](https://github.com/gorhill/uBlock/commit/578594bbd7c545b62f18267d640a605f8e07a53a))
-   optional, string or regular expression that must match the current function call stack trace (New in [1.51.1b5](https://github.com/gorhill/uBlock/commit/4649ae4d78)). Prepend pattern with `!` to test for unmatched patterns in stack trace (New in [1.51.1b6](https://github.com/gorhill/uBlock/commit/84cc69aa10bc9c8045ed94b528ddc158c06ae7ab))

A property in a list of properties can be a chain of properties, example: `adpath.url.first`.

After [1.28.0](https://github.com/gorhill/uBlock/commit/f433932d8602230539d3408e9946d4d70b40306c), two special _"wildcard tokens"_ have been added:

-   `[]`: iterate in all elements in an array. To deal with cases where the property to remove is an element in an array. To remove `adserver` object properties from array in following JSON payload:
    
    ```
      {"playlist": [{"adserver": "first"},{"adserver": "second"}]}
    
    ```
    
    Use:
    
    ```
      +js(json-prune, playlist.[].adserver)
    
    ```
    
-   `*`: iterate through all own properties of an object. For example, to deal with hard to predict random-named properties. To remove `adserver` object properties from inside _randomly named_ objects in following JSON payload:
    
    ```
      {"playlist": {"random1": {"adserver": "first"}, "randomB": {"adserver": "second"}}}
    
    ```
    
    Use:
    
    ```
     +js(json-prune, playlist.*.adserver)
    
    ```
    

Version [1.57.0](https://github.com/gorhill/uBlock/commit/e7a0f8c7810c002bc7cec6a1160f8b091f8ea52e) extend syntax with two special token properties:

-   `[-]`: remove an array entry if part right of `[-]` matches the inspected item.
-   `{-}`: remove a property if part right of `{-}` matches the inspected item.

When used without parameters, will log current hostname + json payload to the logger.  
New in [1.27.0](https://github.com/gorhill/uBlock/commit/578594bbd7c545b62f18267d640a605f8e07a53a) - second parameter can be used to limit logging to JSON payloads which stringified content match specified string or _regular expression_.

Tokens:

-   `log, ...`: This extends logging capabilities of `json-prune` scriptlet as follows: `...##+js(json-prune, a, b, stackNeedle, log, [logneedle], logstack, 1)`. Whereas before, the only way to log `json-prune` usage was to skip providing the property chain: `...##+js(json-prune, , b)` Where `b` was the expression to filter out logging output. With the extended logging capabilities, the logging output can be filtered out with `logneedle`, which can be a regex literal. Additionally, to log the stack trace the `stackNeedle` argument must be set to non-empty string. You can use `/.^/` to log the stack trace without matching it (New in [1.51.1b5](https://github.com/gorhill/uBlock/commit/81b2fcee5d)).

Examples:

-   `youthhealthmag.com##+js(json-prune, unit_list)`
-   `winfuture.de##+js(json-prune, adtagparameter, enabled)`
-   `imgsen.com##+js(json-prune, *, showTrkURL)` - will remove everything when needle matches, new in [1.35](https://github.com/gorhill/uBlock/commit/d338e4c4b6caf339873a60c9d48fde58e9a495ce)

If the site uses `eval` in lieu of `JSON.parse`, see: [evaldata-prune](#evaldata-prunejs-)

Also see:

-   [AdGuard `json-prune`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#json-prune)

* * *

### json-prune-fetch-response.js [↪](https://github.com/gorhill/uBlock/blob/66e3a1ad47162a89709e99072e5eaecdd83cb633/assets/resources/scriptlets.js#L1229)

[](#json-prune-fetch-responsejs-)

New in [1.51.1rc1](https://github.com/gorhill/uBlock/commit/749cec0f095f659d6c0b90eb89b729e9deb07c87)

Removes specified properties from the JSON response of a fetch call.

Syntax:

```
...##+js(json-prune-fetch-response, prune paths [, needle paths [, ...varargs ]])

```

For parameters, see: [json-prune](#json-prunejs-)

Except that the `stack` parameter is treated as a vararg.

Tokens:

-   "propsToMatch": string of space-separated properties to match; possible props:
    -   string or regular expression for matching the URL passed to fetch call;
    -   colon-separated pairs `name:value` where
        -   `name` is [`init` option name](https://developer.mozilla.org/en-US/docs/Web/API/WindowOrWorkerGlobalScope/fetch#parameters)
        -   `value` is string or regular expression for matching the value of the option passed to fetch call.
-   "stackToMatch": string or regular expression that must match the current function call stack trace. Prepend pattern with `!` to test for unmatched patterns in stack trace.

Examples:

```
tumblr.com##+js(json-prune-fetch-response, response.timeline.elements.\[-\].advertiserId, , propsToMatch, url:/api/v2/tabs/for\_you)
nbc.com##+js(json-prune-fetch-response, avails, , propsToMatch, amazonaws.com)
```

Also see:

-   [AdGuard `json-prune-fetch-response`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#json-prune-fetch-response)

* * *

### json-prune-xhr-response.js [↪](https://github.com/gorhill/uBlock/blob/66e3a1ad47162a89709e99072e5eaecdd83cb633/assets/resources/scriptlets.js#L1795)

[](#json-prune-xhr-responsejs-)

New in [1.51.1rc3](https://github.com/gorhill/uBlock/commit/3152896d428c54c76cfd66c3da110bd4d6506cbc)

Removes specified properties from the JSON response of a `XMLHttpRequest` call.

For usage, see: [json-prune-fetch-response](#json-prune-fetch-responsejs-)

Example:

```
www.facebook.com##+js(json-prune-xhr-response, data.viewer.instream\_video\_ads data.scrubber, , propsToMatch, /api/graphql)
```

Also see:

-   [AdGuard `json-prune-xhr-response`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#json-prune-xhr-response)

* * *

### xml-prune.js [↪](https://github.com/gorhill/uBlock/blob/bf690145c493acd86e578d7a860da238f0af72d4/assets/resources/scriptlets.js#L1672)

[](#xml-prunejs-)

Removes an element from the specified XML retrieved using fetch, xhr support was added in [1.49.3rc6](https://github.com/gorhill/uBlock/commit/d3fae27017a1ce65fcba35ef7ca59b6a22ad2bde).

New in [1.44.5b3](https://github.com/gorhill/uBlock/commit/bf690145c493acd86e578d7a860da238f0af72d4)

After [1.49.3rc16](https://github.com/gorhill/uBlock/commit/f8c4b8e52d9e93e0419eb8b0891084e59be0616b), it can also remove attributes.

Parameters:

-   required, a selector of elements or xpath of elements/attributes (New in [1.49.3rc16](https://github.com/gorhill/uBlock/commit/f8c4b8e52d9e93e0419eb8b0891084e59be0616b)) which are to be removed.
-   optional, a selector or xpath (New in [1.49.3rc15](https://github.com/gorhill/uBlock/commit/8ed78cfb234d3b9c615ee1deebea0ff0439ea7f3) that must have a match in the document for the pruning to occur. No selector means the pruning can be performed regardless.
-   optional, a URL which must be a match for the pruning to occur. If left blank, the pruning can be performed regardless.

Tokens:

-   `log, 1` (deprecated, read about [the new method](#general-purpose-scriptlets) of logging scriptlets)
-   `logdoc, 1`: Dumps the whole document being worked on to the logger

Examples:

-   `cbs.com##+js(xml-prune, Period[id*="-roll-"][id*="-ad-"], , pubads.g.doubleclick.net/ondemand)`
-   `play.max.com##+js(xml-prune, xpath(//*[name()="Period"][not(.//*[name()="SegmentTimeline"])]), , .mpd)`
-   `example.com##+js(xml-prune, xpath(//*[name()="MPD"]/@mediaPresentationDuration), , .mpd, log, 1)`

Also see:

-   [AdGuard `xml-prune`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#xml-prune)

* * *

### m3u-prune.js [↪](https://github.com/gorhill/uBlock/blob/115f7bb68704c4fede763cbc2d07f1caf041274f/assets/resources/scriptlets.js#L1743)

[](#m3u-prunejs-)

New in [1.44.5b6](https://github.com/gorhill/uBlock/commit/115f7bb68704c4fede763cbc2d07f1caf041274f#diff-30b28769623e5478a0f68519eda037164484cfb444cb5a8e48518fa7bb32e658)

Sometimes sites serve real video content and video ads all in one place inside `.m3u8` files. You can use `m3u-prune` to remove those ad segments.

Examples:

-   `player.theplatform.com##+js(m3u-prune, tvessaiprod.nbcuni.com, /theplatform\.com\/.*?\.m3u8/)`
-   `mephimtv.cc##+js(m3u-prune, /#EXT-X-DISCONTINUITY(.|\n){1\,100}#EXT-X-DISCONTINUITY/gm, mixed.m3u8)`

If the first argument is a regex with multine flag set, the scriptlet will execute the regex against the whole text, and remove matching text from the whole text (New in [1.47.5b10](https://github.com/gorhill/uBlock/commit/b3821e6869d45b5697c44922ee7242e35e11a29d)).

If the matching text does not contain whole lines, the text won't be removed, i.e. it is not allowed to remove only part of a line.

Also see:

-   [AdGuard `m3u-prune`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#m3u-prune)

* * *

### noeval.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/noeval.js)

[](#noevaljs-)

Prevent web pages from using _`eval()`_, and report attempts to the logger. This should not be used as a generic filter due to the fact that it breaks many websites, including those using Cloudflare's DDoS protection.

Examples:

-   `solowarez.org##+js(noeval)`

Also see:

-   [AdGuard `noeval`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#noeval)

* * *

### noeval-silent.js /

[](#noeval-silentjs-)

### silent-noeval.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/noeval-silent.js)

[](#silent-noevaljs-)

Prevent web pages from using _`eval()`_.

* * *

### noeval-if.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L340)

[](#noeval-ifjs-)

### prevent-eval-if.js

[](#prevent-eval-ifjs)

Prevent web pages from using _`eval()`_ on specific matching payloads.

Parameters:

-   optional, string/_regular expression_, matching in payload string.

When used without parameters, will log calls to `eval()` to the logger.

Examples:

-   `orgyxxxhub.com##+js(noeval-if, replace)`

* * *

### no-floc.js [↪](https://github.com/gorhill/uBlock/blob/bfdc81e9e400f7b78b2abc97576c3d7bf3a11a0b/assets/resources/scriptlets.js#L668)

[](#no-flocjs-)

Obsolete: [FLoC ended its experiment in July of 2021](https://github.com/uBlockOrigin/uBlock-issues/issues/1553#issuecomment-1021680848).

New in [1.35.0](https://github.com/gorhill/uBlock/commit/bfdc81e9e400f7b78b2abc97576c3d7bf3a11a0b).

Defuses Google FLoC ("Federated Learning of Cohorts") tracking. Read more on [https://amifloced.org/](https://amifloced.org/)

uBlock Origin (uBO) ensures FLoC is opt-in. The generic filter `*##+js(no-floc)` in "uBlock filters -- Privacy" ensures the feature is disabled when using default settings/lists.

Users can opt-in to FLoC by adding a generic exception filter to their custom filters, `#@#+js(no-floc)`; or they can opt-in only for a specific set of websites through a more specific exception filter:

```
example.com,shopping.example#@#+js(no-floc)

```

Solves [uBlockOrigin/uBlock-issues#1553](https://github.com/uBlockOrigin/uBlock-issues/issues/1553).

* * *

### prevent-fetch.js [↪](https://github.com/gorhill/uBlock/blob/b6ed83bc5c840431ed03cddaed1daeb395db3b0e/assets/resources/scriptlets.js#L586)

[](#prevent-fetchjs-)

### no-fetch-if.js

[](#no-fetch-ifjs)

New in [1.31.3b9](https://github.com/gorhill/uBlock/commit/ba11a700139bbc648e4ae5b2bc7af90ef03db5df).

Defuses calls to `fetch()` by returning a promise which always resolve to an empty response.

Parameters:

-   optional, space-separated list of conditions which must be ALL fulfilled in order for the defusing to take place:
    -   string/_regular expression_, prefixed by `!` for negation (new in [1.59.1b13](https://github.com/gorhill/uBlock/commit/e8202af11d)), matching in URL passed to `fetch()` call
    -   colon-separated `name:value` pairs of [`init` option name](https://developer.mozilla.org/en-US/docs/Web/API/WindowOrWorkerGlobalScope/fetch#Parameters) and string/_regular expression_ matching in value of that option passed to `fetch()` call.
-   optional, directive; arguments to define the response body:
    -   `true` to randomize the response, random 10-character string [1.51.1b15](https://github.com/gorhill/uBlock/commit/418087d)
    -   `emptyObj` to return the response value as empty object [1.54.1rc2](https://github.com/gorhill/uBlock/commit/e1ae17e)
    -   `emptyArr` to return the response value as empty array [1.54.1rc2](https://github.com/gorhill/uBlock/commit/e1ae17e)
    -   `emptyStr` to return the response value as empty string [1.54.1rc2](https://github.com/gorhill/uBlock/commit/e1ae17e)
    -   `length:min[-max]` to fulfill the response with randomized text with length specified, limited to 500000 characters [1.54.1b1](https://github.com/gorhill/uBlock/commit/6aeab2a)
    -   `war:[name of resource]` to fulfill the response with the content of a [web\_accessible\_resources](https://github.com/gorhill/uBlock/tree/master/src/web_accessible_resources) resource [1.54.1b1](https://github.com/gorhill/uBlock/commit/6aeab2a)
-   optional, (new in [1.58.1rc4](https://github.com/gorhill/uBlock/commit/e785b99338)), string for defining response type / JSON string with properties to set on the returned response instance, original response type is used if not specified:
    -   `ok`, supported values: `false`, `true`
    -   `type`, supported values: `"basic"`, `"cors"`, `"default"`, `"error"`, `"opaque"`
    -   `statusText`, supported values: `""`, `"Not Found"`. defaults to `"OK"` when not overridden. [1.58.1rc5](https://github.com/gorhill/uBlock/commit/9ce30563613495f5b609211c017aed580847a9cf)

When used without parameters, the parameters passed to `no-fetch-if` will be logged to the logger.

Examples:

```
example.com##+js(no-fetch-if, method:HEAD)
example.com##+js(no-fetch-if, adsbygoogle.js)
example.com##+js(no-fetch-if, !negatedValue)
example.com##+js(no-fetch-if, adsbygoogle.js method:HEAD)
example.com##+js(no-fetch-if, /adsbygoogle.js$/ method:/HEAD|POST/)
example.com##+js(no-fetch-if, adsbygoogle.js, length:11000)
example.com##+js(no-fetch-if, adsbygoogle.js, war:googlesyndication\_adsbygoogle.js)
example.com##+js(no-fetch-if, doubleclick, , '{"type": "opaque"}')
```

See also:

-   [AdGuard `prevent-fetch`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#prevent-fetch)

* * *

### trusted-prevent-fetch.js [↪](https://github.com/gorhill/uBlock/blob/f106a810528825ae3e3683bf7aab2f247d91d311/src/js/resources/prevent-fetch.js#L197)

[](#trusted-prevent-fetchjs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-2)

New in [1.63.3b8](https://github.com/gorhill/uBlock/commit/4ce26b63ff5d0560ec3f529e275a8430a0db7327)

Essentially the same as [`prevent-fetch`](#prevent-fetchjs) except that if the `directive` argument is not a known token, it will be used as is as the response text of the fetch request, whereas `prevent-fetch` returns an empty string when the directive is unknown.

* * *

### trusted-replace-xhr-response.js [↪](https://github.com/gorhill/uBlock/blob/66e3a1ad47162a89709e99072e5eaecdd83cb633/assets/resources/scriptlets.js#L4362)

[](#trusted-replace-xhr-responsejs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-3)

New in [1.51.1rc3](https://github.com/gorhill/uBlock/commit/3152896d428c54c76cfd66c3da110bd4d6506cbc)

Replaces response text content of `xhr` requests if all given parameters match.

Parameters:

-   required, argument for matching contents of `responseText` that should be replaced. Possible values: If set, `replacement` is required. Possible values:
    -   `*`: match all text content
    -   non-empty string
    -   regular expression
-   optional, should be set if first parameter is set. String to replace the response text content matched by pattern. Empty string to remove content. Defaults to empty string.
-   optional, string of space-separated properties to match; possible props:
    -   string or regular expression for matching the URL passed to `XMLHttpRequest.open()` call; empty string, wildcard `*` will match all fetch calls
    -   colon-separated pairs `name:value` where:
        -   `name` — string or regular expression for matching XMLHttpRequest property name
        -   `value` — string or regular expression for matching the value of the option passed to `XMLHttpRequest.open()` call

Tokens:

-   `includes, pattern`: replace only if `pattern` match in the text response. Can be a plain string or a regex. [1.58.1b6](https://github.com/gorhill/uBlock/commit/9072772f6134d80b4cfa4b90a10c624810a781fc)

Scriptlet does nothing if response body can't be converted to text.

Example:

-   `in-jpn.com##+js(trusted-replace-xhr-response, /.*/, , pagead2.googlesyndication.com/pagead/js/adsbygoogle.js?)`

Also see:

-   [AdGuard `trusted-replace-xhr-response`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-replace-xhr-response)

* * *

### trusted-replace-fetch-response.js [↪](https://github.com/gorhill/uBlock/blob/2282215e1cb0f9fb0974fba5d16ca5fb9c5bcc57/assets/resources/scriptlets.js#L3527)

[](#trusted-replace-fetch-responsejs-)

### trusted-rpfr.js

[](#trusted-rpfrjs)

#### _Trusted scriptlet_

[](#trusted-scriptlet-4)

New in [1.51.1b8](https://github.com/gorhill/uBlock/commit/82a7d11f78).

Replaces response text content of `fetch` requests if all given parameters match.

Parameters:

-   required, argument for matching contents of `responseText` that should be replaced. Possible values:
    -   `*`: match all text content
    -   non-empty string
    -   regular expression
-   optional, should be set if first parameter is set. String to replace the response text content matched by pattern. Empty string to remove content. Defaults to empty string.
-   optional, string of space-separated properties to match; possible props:
    -   string or regular expression for matching the URL passed to fetch call; empty string, wildcard `*` will match all fetch calls
    -   colon-separated pairs `name:value` where:
        -   `name` is [init option name](https://developer.mozilla.org/en-US/docs/Web/API/WindowOrWorkerGlobalScope/fetch#parameters)
        -   `value` is string or regular expression for matching the value of the option passed to fetch call

Tokens:

-   `includes, pattern`: replace only if `pattern` match in the text response. Can be a plain string or a regex. [1.58.1b6](https://github.com/gorhill/uBlock/commit/9072772f6134d80b4cfa4b90a10c624810a781fc)

Scriptlet does nothing if response body can't be converted to text.

Solves [uBlockOrigin/uBlock-issues#2742](https://github.com/uBlockOrigin/uBlock-issues/issues/2742).

Examples:

-   `example.com##+js(trusted-replace-fetch-response, adb_detect:true, adb_detect:false, example.com)`
-   `example.com##+js(trusted-replace-fetch-response, /#EXT-X-VMAP-AD-BREAK[\s\S]*?/, #EXT-X-ENDLIST, example.com)`
-   `example.com##+js(trusted-replace-fetch-response, *, , 'method:GET')`
-   `example.com##+js(trusted-replace-fetch-response, /#EXT-X-VMAP-AD-BREAK[\s\S]*?/, #EXT-X-ENDLIST, '/\.m3u8/ method:/GET|HEAD/')`
-   `example.com##+js(trusted-replace-fetch-response, *, , another_domain.com)`

Also see:

-   [AdGuard `trusted-replace-fetch-response`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-replace-fetch-response)

* * *

### trusted-replace-argument.js [↪](https://github.com/gorhill/uBlock/blob/66e3a1ad47162a89709e99072e5eaecdd83cb633/assets/resources/scriptlets.js#L4747)

[](#trusted-replace-argumentjs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-5)

New in [1.55.1b18](https://github.com/gorhill/uBlock/commit/34da372d7af0ca576b7063053d7966fdb04bdf9b)

Replace an argument passed to a method.

Syntax:

-   `[...]##+js(trusted-replace-argument, propChain, argposRaw, argraw [,condition, pattern])`

Parameters:

-   required, `propChain`: The property chain to the function which argument must be replaced when called.
-   required, `argposRaw`: The zero-based position of the argument in the argument list. Use a negative number for a position relative to the last argument. Use literal `this` to replace the value used in `prototype`\-based methods.
-   required, `argraw`: The replacement value, validated using the same heuristic as with the `set-constant.js` scriptlet.
    -   If the replacement value matches `json:...`, the value will be the json-parsed string after `json:`
    -   If the replacement value matches `repl:/.../.../`, the target argument will be replaced according the regex-replacement directive following `repl:` [1.61.3b1](https://github.com/gorhill/uBlock/commit/adced29b5bd26165ca89c4562a048c7e73a667f6)

If `argposRaw` parameter is not declared, the scriptlet will log all calls to `propChain` along with the arguments passed and will not replace the argument passed to the trapped method.

Tokens:

-   `condition, pattern`: The replacement will occur only when pattern matches the target argument.

Examples of usage:

-   `alliptvlinks.com##+js(trusted-replace-argument, MutationObserver, 0, noopFunc)`
-   `scimagojr.com##+js(trusted-replace-argument, document.querySelector, 0, json:"body", condition, .ad-zone)`
-   `example.com##+js(trusted-replace-argument, String.prototype.split, this, repl:/&ads=1/&ads=0/, condition, ads_enabled=true)`
    -   For illustration, this filter will intercept `String.prototype.split` method and regex-replace the string before the string is split, see example below.
        -   ```
            var config = "ads_enabled=true&ads=1";
            config.split("&");
            // &ads=1 will be replaced with &ads=0
            
            ```
            

* * *

### prevent-requestAnimationFrame.js [↪](https://github.com/gorhill/uBlock/blob/1de0e820b87fdd3717b9f2653baaa7a934075055/assets/resources/scriptlets.js#L522)

[](#prevent-requestanimationframejs-)

### no-requestAnimationFrame-if.js /

[](#no-requestanimationframe-ifjs-)

### norafif.js

[](#norafifjs)

New in [1.27.0](https://github.com/gorhill/uBlock/commit/1de0e820b87fdd3717b9f2653baaa7a934075055).

**Defuses** calls to _`requestAnimationFrame()`_ function when parameter:

-   **is not prefixed** with `!` and **matches** the stringified _callback_ argument to _`requestAnimationFrame()`_; OR
-   **is prefixed** with `!` and **does not match** the stringified _callback_ argument to _`requestAnimationFrame()`_.

Parameters:

-   optional, string/_regular expression_, matching in the stringified _callback_ argument passed to requestAnimationFrame.

Use with `/^/` parameter to defuse all calls unconditionally.

When used without parameters, will log calls to _`requestAnimationFrame()`_ to the logger.

Examples:

-   `bloomberg.com##+js(norafif, paywall-inline-tout)`

Also see:

-   [AdGuard `prevent-requestAnimationFrame`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#prevent-requestAnimationFrame)

* * *

### prevent-setInterval.js [↪](https://github.com/gorhill/uBlock/blob/8f3d8cde7a9de45695d2706087701df59553c01b/assets/resources/scriptlets.js#L701)

[](#prevent-setintervaljs-)

### no-setInterval-if.js /

[](#no-setinterval-ifjs-)

### nosiif.js /

[](#nosiifjs-)

### setInterval-defuser.js

[](#setinterval-defuserjs)

New in [1.23.0](https://github.com/gorhill/uBlock/commit/9367a6015b8cbb6b49347b00a105aab8f24df861)

**Defuses** calls to _`setInterval()`_ function when parameters:

-   **are not prefixed** with `!` and **match** the _`setInterval()`_ argument; OR
-   **are prefixed** with `!` and **do not match** the _`setInterval()`_ argument.

Parameters:

-   optional, string/_regular expression_, matching in stringified callback function
-   optional, a value to match against the delay. Can be a single value for exact match, or a range:
    -   `min-max`: matches if delay >= min and delay <= max
    -   `min-`: matches if delay >= min
    -   `-max`: matches if delay <= max

Use with `/^/` parameter to defuse all calls unconditionally.

When used without parameters, will log calls to _`setInterval()`_ to the logger.

Examples:

-   `jpidols.tv##+js(nosiif)`
-   `finanzen.*##+js(nosiif, nrWrapper)`
-   `yachtrevue.at##+js(nosiif, text/css, 10)`

Also see:

-   [AdGuard `prevent-setInterval`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#prevent-setInterval)

* * *

### prevent-setTimeout.js [↪](https://github.com/gorhill/uBlock/blob/8f3d8cde7a9de45695d2706087701df59553c01b/assets/resources/scriptlets.js#L776)

[](#prevent-settimeoutjs-)

### no-setTimeout-if.js /

[](#no-settimeout-ifjs-)

### nostif.js /

[](#nostifjs-)

### setTimeout-defuser.js

[](#settimeout-defuserjs)

New in [1.23.0](https://github.com/gorhill/uBlock/commit/9367a6015b8cbb6b49347b00a105aab8f24df861)

**Defuses** calls to _`setTimeout()`_ function when parameters:

-   **are not prefixed** with `!` and **match** the _`setTimeout()`_ argument; OR
-   **are prefixed** with `!` and **do not match** the _`setTimeout()`_ argument.

Parameters:

-   optional, string/_regular expression_, matching in stringified callback function
-   optional, a value to match against the delay. Can be a single value for exact match, or a range:
    -   `min-max`: matches if delay >= min and delay <= max
    -   `min-`: matches if delay >= min
    -   `-max`: matches if delay <= max

Use with `/^/` parameter to defuse all calls unconditionally.

When used without parameters, will log calls to _`setTimeout()`_ to the logger.

Test page: [https://gorhill.github.io/uBlock/tests/scriptlet-injection-filters-1.html](https://gorhill.github.io/uBlock/tests/scriptlet-injection-filters-1.html)

Examples:

-   `computerbild.de##+js(nostif, ())return)`
-   `lablue.*##+js(nostif, push, 500)`

In [1.31.3b11](https://github.com/gorhill/uBlock/commit/ba11a700139bbc648e4ae5b2bc7af90ef03db5df) aliased as `setTimeout-defuser.js` for backward compatibility.

Also see:

-   [AdGuard `prevent-setTimeout`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#prevent-setTimeout)

* * *

### nowebrtc.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L721)

[](#nowebrtcjs-)

Disables WebRTC by preventing web pages from using [_`RTCPeerConnection()`_](https://developer.mozilla.org/en-US/docs/Web/API/RTCPeerConnection). Report attempts in console.

Examples:

-   `x1337x.*##+js(nowebrtc)`

Also see:

-   [AdGuard `nowebrtc`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#nowebrtc)

* * *

### prevent-innerHTML.js [↪](https://github.com/gorhill/uBlock/blob/935ce74d3a7b300be12a952f2e7dfa9f0f595db9/src/js/resources/prevent-innerHTML.js#L42)

[](#prevent-innerhtmljs-)

New in [1.63.1b0](https://github.com/gorhill/uBlock/commit/fe744816f1df077fae764e15e5bb5bd0f1c1edc1).

Conditionally prevent assignment to `innerHTML` property.

Parameters:

-   Optional, `selector`: The element must matches `selector` for the prevention to take place.
-   Optional, `pattern`: A pattern to match against the assigned value. The pattern can be a plain string, or a regex. Prepend with `!` to reverse the match condition.

When verbose mode is enabled, the scriptlet will log the assigned value of the element.

Example:

```
example.com##+js(prevent-innerHTML, body, adblock)
```

* * *

### trusted-create-HTML.js [↪](https://github.com/gorhill/uBlock/blob/f106a810528825ae3e3683bf7aab2f247d91d311/src/js/resources/create-html.js#L49)

[](#trusted-create-htmljs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-6)

New in [1.63.3b15](https://github.com/gorhill/uBlock/commit/20dd6065049d0a80b23db83011ae0c10ce34cbd2)

Element(s) from a parsed HTML string are added as child element(s) to a specific parent element in the DOM.

Parameters:

-   required, `parent`: A CSS selector identifying the element to which created element(s) will be added.
-   required, `html`: An HTML string to be parsed using DOMParser, and which resulting elements are to be added as child element(s).
-   optional, `duration`: If specified, the time in ms after which the added elements will be removed. No removal will occur if not specified.

Example:

```
example.org##+js(trusted-create-html, html, <iframe src=""></iframe>)
```

* * *

### prevent-xhr.js [↪](https://github.com/gorhill/uBlock/blob/745fbd1c02b7179052ba97f51c54f7cb000636f0/assets/resources/scriptlets.js#L1171)

[](#prevent-xhrjs-)

### no-xhr-if.js

[](#no-xhr-ifjs)

New in [1.38.0](https://github.com/gorhill/uBlock/commit/745fbd1c02b7179052ba97f51c54f7cb000636f0).

Defuses [XMLHttpRequest](https://developer.mozilla.org/en-US/docs/Web/API/XMLHttpRequest/Using_XMLHttpRequest) network requests by returning empty response. Based on [`no-fetch-if.js`](#no-fetch-ifjs).

Parameters:

-   optional, space-separated list of conditions which must be ALL fulfilled in order for the defusing to take place:
    -   string/_regular expression_, prefixed by `!` for negation, matching in URL passed to XMLHttpRequest `open()` call
    -   colon-separated `name:value` pairs of [XMLHttpRequest method `open()`](https://developer.mozilla.org/en-US/docs/Web/API/XMLHttpRequest/open#parameters) parameter names (only `method` and `url` currently supported) and string/_regular expression_ matching in value of passed argument.
-   optional, directive; arguments to define the response body:
    -   `true` to randomize the response, random 10-character string [1.51.1b15](https://github.com/gorhill/uBlock/commit/418087d)
    -   `emptyObj` to return the response value as empty object [1.54.1rc2](https://github.com/gorhill/uBlock/commit/e1ae17e)
    -   `emptyArr` to return the response value as empty array [1.54.1rc2](https://github.com/gorhill/uBlock/commit/e1ae17e)
    -   `emptyStr` to return the response value as empty string [1.54.1rc2](https://github.com/gorhill/uBlock/commit/e1ae17e)
    -   `length:min[-max]` to fulfill the response with randomized text with length specified, limited to 500000 characters [1.54.1b1](https://github.com/gorhill/uBlock/commit/6aeab2a)
    -   `war:[name of resource]` to fulfill the response with the content of a [web\_accessible\_resources](https://github.com/gorhill/uBlock/tree/master/src/web_accessible_resources) resource [1.51.1b16](https://github.com/gorhill/uBlock/commit/bf591d9)

When used without parameters, the parameters passed to `no-xhr-if` will be logged to the logger.

Examples:

```
example.com##+js(no-xhr-if, method:HEAD)
example.com##+js(no-xhr-if, adsbygoogle.js)
example.com##+js(no-xhr-if, adsbygoogle.js method:HEAD)
example.com##+js(no-xhr-if, /adsbygoogle.js$/ method:/HEAD|POST/)
example.com##+js(no-xhr-if, adsbygoogle.js, length:11000)
example.com##+js(no-xhr-if, doubleclick.net/instream/ad\_status.js, war:doubleclick\_instream\_ad\_status.js)
```

Also see:

-   [AdGuard `prevent-xhr`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#prevent-xhr)

* * *

### trusted-prevent-xhr.js [↪](https://github.com/gorhill/uBlock/blob/947602d4fe46ec832d3f95ee3a65633fc4113f44/assets/resources/scriptlets.js#L4173)

[](#trusted-prevent-xhrjs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-7)

New in [1.60.1b3](https://github.com/gorhill/uBlock/commit/fe49ced2ac937a8556a19cb61f4c2cb05ab3c54c)

Essentially the same as [`no-xhr-if`](#no-xhr-ifjs) except that if the `directive` argument is not a known token, it will be used as is as the response text of the xhr request, whereas `prevent-xhr` returns an empty string when the directive is unknown.

Examples:

```
anitube.vip##+js(trusted-prevent-xhr, outbrain.com, outbrain)
koramaup.com##+js(trusted-prevent-xhr, googlesyndication, 'a.getAttribute("data-ad-client")||""')
```

* * *

### set-attr.js [↪](https://github.com/gorhill/uBlock/blob/786d9b2212e9a2105f516a2ffe5d031da2bdd4b5/assets/resources/scriptlets.js#L2957)

[](#set-attrjs-)

New in [1.50.1b16](https://github.com/gorhill/uBlock/commit/786d9b2212e9a2105f516a2ffe5d031da2bdd4b5).

Sets the specified attribute on the specified elements. This scriptlet runs once when the page loads then afterward on DOM mutations.

Parameters:

-   required, CSS selector of DOM elements for which the attribute `attr` must be modified.
-   required, the name of the attribute to modify
-   required, the value to assign to the target attribute. Possible values:
    -   `''`: empty string (default)
    -   `true`
    -   `false`
    -   positive decimal integer: `0 <= value < 32768`
    -   `[other]`: copy the value from attribute `other` on the same element. This allows to copy the value of one attribute to another attribute on the same element.

Examples:

```
example.com##+js(set-attr, div.class > a.class, test-attribute, 0)
example.com##+js(set-attr, a > img, src, \[data-src\])
```

Solves [uBlockOrigin/uBlock-issues#2347](https://github.com/uBlockOrigin/uBlock-issues/issues/2347).

Also see:

-   [AdGuard `set-attr`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#set-attr)

* * *

### trusted-set-attr.js [↪](https://github.com/gorhill/uBlock/blob/947602d4fe46ec832d3f95ee3a65633fc4113f44/assets/resources/attribute.js#L183)

[](#trusted-set-attrjs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-8)

New in [1.60.1b17](https://github.com/gorhill/uBlock/commit/11ca4a39239478e35605ec072fca140ac4c70d3b)

Sets the specified attribute on the specified elements. This scriptlet runs once when the page loads then afterward on DOM mutations.

Parameters:

-   required, selector: A CSS selector for the elements to target.
-   required, attr: The name of the attribute to be added.
-   optional, value: The new value of the attribute. Since the scriptlet requires a trusted source, the value can be anything.

Examples:

1.  Adds a `class` attribute to the body element with the value `some-class`.
    
    ```
    example.com##+js(trusted-set-attr, body, class, some-class)
    ```
    
2.  Adds a `test-attribute` attribute to the body element with the value `[true, true]`.
    
    ```
    example.com##+js(trusted-set-attr, body, test-attribute, '\[true, true\]')
    ```
    

Also see:

-   [AdGuard `trusted-set-attr`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#-%EF%B8%8F-trusted-set-attr)

* * *

### remove-attr.js [↪](https://github.com/gorhill/uBlock/blob/0f330c7359567587df6c35e9108b75c339533a56/assets/resources/scriptlets.js#L658)

[](#remove-attrjs-)

### ra.js

[](#rajs)

Deprecated by [`:remove-attr()`](https://github.com/gorhill/ublock/wiki/Static-filter-syntax#subjectremove-attrarg-subjectremove-classarg)

**Filter authors must use the new operator instead of the `+js()` counterpart.**

Removes attribute(s) from DOM tree node(s). By default will run only once when the initial HTML document has been completely loaded and parsed but sub-resources such as scripts, images, stylesheets and frames are still loading.

Parameters:

-   required, attribute or list of attributes joined by `|`
-   optional, _CSS selector_, specifies nodes from which attributes will be removed
-   optional, new in [1.33](https://github.com/gorhill/uBlock/commit/0f330c7359567587df6c35e9108b75c339533a56), one or more space-separated tokens dictating the behavior of the scriptlet
    -   `asap`: added in [1.36.1b2](https://github.com/gorhill/uBlock/commit/35d7406214e39fa5ad5c73cfab3eecb0eb7c8b7f), execute as soon as possible, do not wait for DOM to become available.
    -   `stay`: This tells the scriplet to stay active and act on document changes.
    -   `complete`: This tells the scriplet to start acting only when the document is complete, i.e. once all secondary resources have been loaded.

Examples:

-   `userscloud.com##+js(ra, onclick, .btn-icon-stacked)`
-   `magesy.*,majesy.*##+js(ra, oncontextmenu)`
-   `zerodot1.gitlab.io##+js(ra, oncontextmenu|onselectstart|ondragstart)`
-   `example.com##+js(remove-attr, class, .j-mini-player, stay)`

Also see:

-   [AdGuard `remove-attr`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#remove-attr)

* * *

### remove-class.js [↪](https://github.com/gorhill/uBlock/blob/3160bc8ccdab2b7dbc906ea213b29a4c04120be1/assets/resources/scriptlets.js#L757)

[](#remove-classjs-)

### rc.js

[](#rcjs)

Deprecated by [`:remove-class()`](https://github.com/gorhill/ublock/wiki/Static-filter-syntax#subjectremove-attrarg-subjectremove-classarg)

**Filter authors must use the new operator instead of the `+js()` counterpart.**

New in [1.26.0](https://github.com/gorhill/uBlock/commit/49d9929191461cc8534ebf5707d94a5970945bde).

Removes classes from DOM tree node(s). By default will run only once after page load. Syntax based on [`remove-attr.js`](#remove-attrjs-)

Parameters:

-   required, class name or list of class names joined by `|`
-   optional, _CSS selector_, specifies nodes from which classes should be removed
-   optional, new in [1.36](https://github.com/gorhill/uBlock/commit/2de24a11843df653173e50b9e952052361c64147), one or more space-separated tokens dictating the behavior of the scriptlet
    -   `stay`: This tells the scriplet to stay and act on DOM changes, while the default behavior is to act only once when the document becomes interactive.
    -   `complete`: This tells the scriplet to start acting only when the document is complete, i.e. once all secondary resources have been loaded, while the default is to start acting when the document is interactive - which is earlier than when the document is complete.

Examples:

-   `danskebank.fi##+js(rc, cookie-consent-banner-open, html)` [Picture of the element](https://images2.imgbox.com/68/2b/tdWI9hBG_o.png)

Also see:

-   [AdGuard `remove-class`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#remove-class)

* * *

### trusted-replace-node-text.js [↪](https://github.com/gorhill/uBlock/blob/f3b720d532c7a42a6ad5167e3b6f860004b4c2b6/assets/resources/scriptlets.js#L2570)

[](#trusted-replace-node-textjs-)

### trusted-rpnt.js /

[](#trusted-rpntjs-)

### replace-node-text.js /

[](#replace-node-textjs-)

### rpnt.js

[](#rpntjs)

#### _Trusted scriptlet_

[](#trusted-scriptlet-9)

New in [1.49.3b16](https://github.com/gorhill/uBlock/commit/41876336db48292de06707adfa5e97dab74297d2)

Replace text instance(s) with another text instance inside specific DOM nodes.

By default, the scriptlet will bail out when the document itself has been fully loaded, i.e. when `DOMContentLoaded` event is fired.

The mutation observer of this scriptlet can be a significant overhead for pages with dynamically updated DOM, and in most cases the scriptlet is useful only for DOM changes occurring before the `DOMContentLoaded` event, so the default is to quit out when that event is received ("quit out" means discarding the mutation observer and having the scriptlet garbage-collected by the JS engine).

Parameters:

-   required, the name of the node for which the text content must be substituted. Valid node names can be found at: [https://developer.mozilla.org/en-US/docs/Web/API/Node/nodeName](https://developer.mozilla.org/en-US/docs/Web/API/Node/nodeName)
-   required, a string or regex to find in the text content of the node as the target of substitution
-   optional, the replacement text. Can be omitted if the goal is to delete the text which matches the pattern. Cannot be omitted if extra pairs of parameters have to be used (see below)

Tokens:

-   `includes, pattern`: A string or regex which must be found in the text content of the node in order for the substitution to occur
-   `excludes, pattern`: A string or regex which must **not** be found in the text content of the node in order for the substitution to occur
-   `sedCount, n`: This will cause the scriptlet to stop after `n` instances of substitution. Since a mutation oberver is used by the scriptlet, it's advised to stop it whenever it becomes pointless. Default to zero, which means the scriptlet never stops
-   `stay, 1`: Force the scriptlet to stay at work forever
-   `quitAfter, ms`: This tells the scriptlet to quit `ms` milliseconds after the page has been loaded, i.e. after the `DOMContentLoaded` event has been fired
-   `log, 1`: This will cause the scriptlet to output information at the console, useful as a debugging tool for filter authors  
    (deprecated, read about [the new method](#general-purpose-scriptlets) of logging scriptlets)

Examples:

-   `example.com##+js(rpnt, #text, /^Advertisement$/)`
-   `example.com##+js(rpnt, #text, Example Domain, Changed, condition, Example, stay, 1)`
-   `example.com##+js(rpnt, script, /devtoolsDetector\.launch\(\)\;/, , sedCount, 1)`

Related discussions:

-   [New rpnt (prev. sed) scriptlet](https://github.com/uBlockOrigin/uAssets/discussions/18185)
-   [Using trusted-replace-node-text](https://github.com/uBlockOrigin/uAssets/discussions/23769)

Also see:

-   [remove-node-text](#remove-node-textjs-)
-   [AdGuard `trusted-replace-node-text`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-replace-node-text)

* * *

### remove-node-text.js [↪](https://github.com/gorhill/uBlock/blob/f3b720d532c7a42a6ad5167e3b6f860004b4c2b6/assets/resources/scriptlets.js#L2531)

[](#remove-node-textjs-)

### rmnt.js

[](#rmntjs)

New in [1.49.3rc15](https://github.com/gorhill/uBlock/commit/2bb446797a12086f2eebc0c8635b671b8b90c477)

Remove the _whole_ text of a DOM node.

By default, the scriptlet will bail out when the document itself has been fully loaded, i.e. when `DOMContentLoaded` event is fired.

The mutation observer of this scriptlet can be a significant overhead for pages with dynamically updated DOM, and in most cases the scriptlet is useful only for DOM changes occurring before the `DOMContentLoaded` event, so the default is to quit out when that event is received ("quit out" means discarding the mutation observer and having the scriptlet garbage-collected by the JS engine).

Parameters:

-   required, the name of the node for which the text content must be removed. Valid node names can be found at: [https://developer.mozilla.org/en-US/docs/Web/API/Node/nodeName](https://developer.mozilla.org/en-US/docs/Web/API/Node/nodeName)
-   required, A string or regex to find in the text content of the node as the target of removing

Optionally, extra pairs of parameters (tokens) can be used to modify the behavior of the scriptlet.

Tokens:

-   `includes, pattern`: A string or regex which must be found in the text content of the node in order for the removing to occur
-   `excludes, pattern`: A string or regex which must **not** be found in the text content of the node in order for the removing to occur
-   `sedCount, n`: This will cause the scriptlet to stop after `n` instances of removing. Since a mutation oberver is used by the scriptlet, it's advised to stop it whenever it becomes pointless. Default to zero, which means the scriptlet never stops
-   `stay, 1`: Force the scriptlet to stay at work forever
-   `quitAfter, ms`: This tells the scriptlet to quit `ms` milliseconds after the page has been loaded, i.e. after the `DOMContentLoaded` event has been fired
-   `log, 1`: This will cause the scriptlet to output information at the console, useful as a debugging tool for filter authors  
    (deprecated, read about [the new method](#general-purpose-scriptlets) of logging scriptlets)

Examples:

-   `example.com##+js(rmnt, #text, Example)`
-   `example.com##+js(rmnt, #text, Example, condition, Exa)`
-   `example.com##+js(rmnt, script, timeLeft)`

Also see:

-   [trusted-replace-node-text](#trusted-replace-node-textjs-)
-   [AdGuard `remove-node-text`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#remove-node-text)

* * *

### trusted-replace-outbound-text.js [↪](https://github.com/gorhill/uBlock/blob/21e1ee30ee36c1b9a7a3c9f43ac97e52d8e79661/assets/resources/scriptlets.js#L4708)

[](#trusted-replace-outbound-textjs-)

### trusted-rpot.js

[](#trusted-rpotjs)

#### _Trusted scriptlet_

[](#trusted-scriptlet-10)

New in [1.57.1b1](https://github.com/gorhill/uBlock/commit/21e1ee30ee)

(currently alias `trusted-rpot` is unsupported yet)

Replace the text in the outbound function call.

Parameters:

-   required, a name of method for which the text content must be substituted
-   required, a string or regex to match in the outbound text as the target of substitution,  
    if not provided or empty, the scriptlet will only log the outbound text without modifying it.
-   optional, the replacement string for the matched part,  
    can be omitted if the goal is to delete the text which matches the pattern.

Tokens:

-   `condition, pattern`: is a vararg which tells the scriptlet to act only if `pattern` is found in the outbound text.
-   `encoding, base64`: Decode and encode outbound base64-encoded text before and after replacement. [1.58.1b3](https://github.com/gorhill/uBlock/commit/fa6740a059)

Examples:

-   `www.reddit.com##+js(trusted-replace-outbound-text, JSON.stringify)` (logging only)
-   `www.reddit.com##+js(trusted-replace-outbound-text, JSON.stringify, '/"csrf_token":"[^"]+",/')`  
    matches and removes: `"csrf_token":"ed5ce21c7d7c95c8662c7844c0f4a4dc",` from the JSON payload
-   `www.reddit.com##+js(trusted-replace-outbound-text, JSON.stringify, '/(?<="csrf_token":")[^"]+?(?=")/')` or  
    `www.reddit.com##+js(trusted-replace-outbound-text, JSON.stringify, '/("csrf_token":")(?:[^"]+?)(")/', /$1$2/)`  
    matches `ed5ce21c7d7c95c8662c7844c0f4a4dc` or any similiar value in `"csrf_token":"..."` and removes it resulting in empty value: `"csrf_token":""`
-   `www.reddit.com##+js(trusted-replace-outbound-text, JSON.stringify, '/(?<="adblock":{"enabled":)true/', false)`  
    matches `true` in `"adblock":{"enabled":true` and replaces it with `false` resulting in a new spoofed value: `"adblock":{"enabled":false`
-   `example.com##+js(trusted-replace-outbound-text, eval, ads:true, ads:false, encoding, base64)` For instance, `eval('"YWRzOnRydWU="')` which decoded value is 'ads:true', will return 'YWRzOmZhbHNl' which is 'ads:false' after decoding.

When the replacement starts with `json:`, it will be first decoded using JSON.parse() (since [1.59.1b8](https://github.com/gorhill/uBlock/commit/0dcb985601)), example:

-   `example.com##+js(trusted-replace-outbound-text, somefn, json:"ok")`

The doublequotes are required since this is what JSON.parse() expects as a valid JSON string.

Solves related issues / discussions:

-   [Add a scriptlet to intercept atob call and prune its text output](https://github.com/uBlockOrigin/uBlock-issues/issues/3157)
-   [Add new scriptlet — 'trusted-replace-outbound-text'](https://github.com/AdguardTeam/Scriptlets/issues/410)
-   [reddit.com: privacy](https://github.com/uBlockOrigin/uAssets/issues/18938)
-   [trusted-replace-outbound-text](https://github.com/uBlockOrigin/uBlock-issues/discussions/3292)

Also see:

-   [AdGuard `trusted-replace-outbound-text`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-replace-outbound-text)

* * *

### href-sanitizer.js [↪](https://github.com/gorhill/uBlock/blob/d7b7dea7faaf17486d5c54454852c4a117f50fd1/assets/resources/scriptlets.js#L1845)

[](#href-sanitizerjs-)

#### _Experimental_

[](#experimental)

New in [1.47.5b4](https://github.com/gorhill/uBlock/commit/e123256eaf64be19f81eba123970db07b45eb0ae)

Set the `href` attribute to a value found in the DOM at, or below the targeted `a` element, and optionally with transformation steps.

Parameters:

-   required, A plain CSS selector for elements which `href` property must be sanitized, if ALL the following conditions are met:
    -   The element is a link (`<a>`) element
    -   The link element has an existing `href` attribute
    -   The text content of the element is a valid `https`\-based URL
-   optional, one or more tokens to lookup the source of the `href` property, and optionally the transformation steps to perform, otherwise the text content of the element will be used.
    -   `[name]`: Use the value of the attribute `name` as the URL
    -   `?name`: Use the value of the `href` query parameter `name` [1.49.3rc15](https://github.com/gorhill/uBlock/commit/56e1d92dbd65e6168620053b1fec4c21c03d664e). Supports unwrapping query parameters recursively [1.58.1rc11](https://github.com/gorhill/uBlock/commit/84be9cde6d)
    -   Transformation steps: see [`urlskip`](https://github.com/gorhill/ublock/wiki/Static-filter-syntax#urlskip) documentation [1.61.3b10](https://github.com/gorhill/uBlock/commit/9bf8d53ebe7cab20ac8491741c5b99cf9d35d643)

Examples:

-   `vk.com##+js(href-sanitizer, a[href^="/go?to="][title], [title])`
-   `vk.com##+js(href-sanitizer, a[href^="/away.php?to="][title], ?to)`
-   `<a href="https://app.adjust.com/2uo1qc?redirect=https%3A%2F%2Fplay.google.com%2Fstore%2Fapps%2Fdetails%3Fid%3Dorg.mozilla.firefox&amp;campaign=www.mozilla.org&amp;adgroup=mobile-android-page">Text</a>`: `mozilla.org##+js(href-sanitizer, a[href^="https://app.adjust.com/"][href*="?redirect="], ?redirect)`
-   `example.com##+js(href-sanitizer, a.clickTracker, ?r?u`
-   `example.org##+js(href-sanitizer, a[href*="/redirect"], ?url ?url -base64)`

Solves [Anti-redirect scriptlet for vk.com (href-sanitizer)](https://github.com/uBlockOrigin/uBlock-issues/issues/2531).

Also see:

-   [AdGuard `href-sanitizer`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#href-sanitizer)

* * *

### prevent-refresh.js [↪](https://github.com/gorhill/uBlock/blob/c0a43b0d32e38aa3858644db20fc69a7b0c85e82/assets/resources/scriptlets.js#L726)

[](#prevent-refreshjs-)

### refresh-defuser.js

[](#refresh-defuserjs)

New in [1.38.7b3](https://github.com/gorhill/uBlock/commit/c0a43b0d32e38aa3858644db20fc69a7b0c85e82)

Attempts to defuse reloading of a document through a [meta "refresh" tag](https://developer.mozilla.org/en-US/docs/Web/HTML/Element/meta#attr-http-equiv). Will stop navigation (call [`window.stop()`](https://developer.mozilla.org/en-US/docs/Web/API/Window/stop)) at specified delay.

Parameters:

-   optional, number (float), number of seconds until the page will be reloaded / when defuser should run. Will be derived from source tag when not specified.

Also see:

-   [AdGuard `prevent-refresh`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#prevent-refresh)

* * *

### trusted-click-element.js [↪](https://github.com/gorhill/uBlock/blob/9123563895f0499849b4d85c4f95e1ed6ace2231/assets/resources/scriptlets.js#L4009)

[](#trusted-click-elementjs-)

New in [1.53.5rc13](https://github.com/gorhill/uBlock/commit/7af88b025deaf4fe23f101984cb2e181315400b1)

#### _Trusted scriptlet_

[](#trusted-scriptlet-11)

Clicks elements on webpages. Can target single or multiple elements in a sequence. The scriptlet will abort after 10s timeout or after all the elements have been clicked. Also hidden elements can be targeted.

Parameters:

-   required, a list of css selector(s). Multiple selectors must be quoted. Use `>>>` to select elements inside shadow root. (New in [1.54.1b3](https://github.com/gorhill/uBlock/commit/941077a25c))
-   optional, Extramatch: conditions for the clicking to occur, cookies and localStorage are matched using the format `name:key[=value]` where the value is optional. Multiple conditions can be separated by comma. Prefix with `!` to only click if the cookie/localStorage item does not exist. Supported names: [1.55.1b12](https://github.com/gorhill/uBlock/commit/45e62c939f)
    -   `cookie`: test string or regex against cookies
    -   `localStorage`: test string or regex against localStorage
-   optional, delay in ms how long the scriptlet will postphone the execution

Tokens:

`log, 1`: This will cause the scriptlet to output information at the console, useful as a debugging tool for filter authors  
(deprecated, read about [the new method](#general-purpose-scriptlets) of logging scriptlets)

Examples:

-   `example.com##+js(trusted-click-element, button.reject-all)`
-   `example.com##+js(trusted-click-element, '[for="CookiePurposes_1_"], [for="CookiePurposes_2_"], button.js-save')`
-   `example.com##+js(trusted-click-element, #cmpwrapper >>> .cmpboxbtnyes)`
-   `example.com##+js(trusted-click-element, #cmp-btn-accept, , 500)`
-   `example.com##+js(trusted-click-element, #cmp-btn-accept, !cookie:/^gpt_ppid[^=]+=/, 5000)`
-   `example.com##+js(trusted-click-element, button[name="agree"], 'cookie:cmpconsent, localStorage:promo=yes')`

Also see:

-   [AdGuard `trusted-click-element`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-click-element)

* * *

### set-constant.js [↪](https://github.com/gorhill/uBlock/blob/a9e6f9c72c920d68a6e5b01b844ad39e6f2e02b0/assets/resources/scriptlets.js#L878)

[](#set-constantjs-)

### set.js

[](#setjs)

Creates _property_ and initializes it with a value from a predefined set.

Scriptlet will succeed only when:

-   original _property_ is `undefined` (scriptlet is called early enough) or `null`[1.25.0](https://github.com/gorhill/uBlock/commit/c7dc65fe33ed58ff2bad10ce4a8848b97c8591ce) **OR**
-   new _property_ written by `set.js` is `undefined` or `null`[1.25.0](https://github.com/gorhill/uBlock/commit/c7dc65fe33ed58ff2bad10ce4a8848b97c8591ce) **OR**
-   type of original _property_ is equal to type of new _property_

Value set by scriptlet can be overwritten by page script when:

-   current _property_ was not set to `undefined` or `null`[1.25.0](https://github.com/gorhill/uBlock/commit/c7dc65fe33ed58ff2bad10ce4a8848b97c8591ce) **AND**
-   new _property_ is not `undefined` or `null`[1.25.0](https://github.com/gorhill/uBlock/commit/c7dc65fe33ed58ff2bad10ce4a8848b97c8591ce) **AND**
-   type of original _property_ is different than type of new _property_

Parameters:

-   required, _property_ (chain of properties joined by `.`) attached to window object
-   required, possible values:
    -   positive decimal integer, no sign, with maximum value of 0x7FFF (32767)
    -   one value from set of predefined constants:
        -   `undefined`
        -   `false`
        -   `true`
        -   `null`[2018-11-24](https://github.com/uBlockOrigin/uAssets/commit/8fd3f7e3a344e5fc29344f1ba914e82457eb1d13#diff-8809d5783978a0b5b88f93d7dab99de0)
        -   `noopFunc` - function with empty body
        -   `trueFunc` - function returning true
        -   `falseFunc` - function returning false
        -   `throwFunc` - function throwing an error[1.59.1rc5](https://github.com/gorhill/uBlock/commit/4ccc0d0fda461b49cfa04cd246d9a39c38c97d51)
        -   `''` - empty string[2019-01-06](https://github.com/uBlockOrigin/uAssets/commit/5051610f0e2374955a03c54be42bbbe9115f05c7#diff-8809d5783978a0b5b88f93d7dab99de0R2132)
        -   `[]` - empty array[1.36](https://github.com/gorhill/uBlock/commit/ce801b952b5777775385efc00479405af54edbc9)
        -   `{}` - empty object[1.36](https://github.com/gorhill/uBlock/commit/ce801b952b5777775385efc00479405af54edbc9)

● optional (deprecated in favor of "runAt" token)

▬▬► SPOILER START ◄▬▬

-   optional, to defer execution of `set-constant`, possible values:
    -   _not present_: execute immediately
    -   1: execute immediately
    -   `interactive`, `end`, `2`: set the constant when the event `DOMContentInteractive` is fired
    -   `complete`, `idle`, `3`: set the constant when the event `load` is fired

▬▬► SPOILER END ◄▬▬

Tokens:

-   "runAt": when this parameter is present, uBO will take it into account to possibly defer execution of the scriptlet  
    new in [1.49.3b4](https://github.com/gorhill/uBlock/commit/e1500ee88d2524da0c93e85b8855d0671a3c6cdb), updated in [1.51.1rc0](https://github.com/gorhill/uBlock/commit/f407c28a00338032ff08a03d9f19a2487af814ff), solves [uBlockOrigin/uAssets#7320](https://github.com/uBlockOrigin/uAssets/issues/7320):
    -   "runAt" not present: execute immediately
    -   `loading` or `1` : execute immediately
    -   `interactive` or `end` or `2`: set the constant when the event `DOMContentLoaded` is fired
    -   `complete` or `idle` or `3`: set the constant when the event `load` is fired
-   "as": when this parameter is present, uBO will take it into account how the values will be returned  
    new in [1.49.3b13](https://github.com/gorhill/uBlock/releases/tag/1.49.3b13), updated in [1.51.1rc0](https://github.com/gorhill/uBlock/commit/f407c28a00338032ff08a03d9f19a2487af814ff), solves [uBlockOrigin/uBlock-issues#2615](https://github.com/uBlockOrigin/uBlock-issues/issues/2615):
    -   `function`: the constant will be a function returning the specified value (`asFunction` is deprecated)
    -   `callback`: the constant will be a function returning a function returning the specified value (`asCallback` is deprecated)
    -   `resolved`: the constant will be a promise resolving to the specified value (`asResolved` is deprecated)
    -   `rejected`: the constant will be a promise failing with the specified value (`asRejected` is deprecated)
-   "stack": considered to be implemented in the future, ideally, AdGuard would support its `stack` parameter as a vararg, to be discussed,  
    issue to be solved: [https://github.com/uBlockOrigin/uBlock-issues/issues/2783](https://github.com/uBlockOrigin/uBlock-issues/issues/2783)
    -   string or regular expression that must match the current function call stack trace, defaults to matching every call; if regular expression is invalid, it will be skipped

Examples:

-   `kompetent.de##+js(set, Object.keys, trueFunc)`
-   `t-online.de##+js(set, abp, false)`
-   `identi.li##+js(set, t_spoiler, 0)`
-   `joysound.com##+js(set, document.body.oncopy, null, 3)` (deprecated)
-   `joysound.com##+js(set, document.body.oncopy, null, runAt, 3)`

Also see:

-   [trusted-set-constant](#trusted-set-constantjs-)
-   [AdGuard `set-constant`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#set-constant)

* * *

### trusted-set-constant.js [↪](https://github.com/gorhill/uBlock/blob/f3b720d532c7a42a6ad5167e3b6f860004b4c2b6/assets/resources/scriptlets.js#L2605)

[](#trusted-set-constantjs-)

### trusted-set.js

[](#trusted-setjs)

#### _Trusted scriptlet_

[](#trusted-scriptlet-12)

Behaves exactly like [set-constant](#set-constantjs-), except that any arbitrary JSON-compatible value can be set.

By default the value is treated as a string, which can be anything.

If the value starts with `{` and ends with `}`, the value will be JSON-parsed, and the `value` property of the resulting object will be used. Since [1.59.1b9](https://github.com/gorhill/uBlock/commit/ae5dc6299e513cb75c601fcebadff3a7235de6a4), `{ "value": ... }` can be replaced with `json:`.

Solves: [https://github.com/uBlockOrigin/uAssets/discussions/18185#discussioncomment-5977456](https://github.com/uBlockOrigin/uAssets/discussions/18185#discussioncomment-5977456)

Examples:

-   `example.com##+js(trusted-set, prop, { "value": 100000 })`
-   `example.com##+js(trusted-set, prop, '{ "value": [ "one", "two", 3 ]}')`
-   `example.com##+js(trusted-set, prop, { "value": { "url": "about:blank" }})`

Also see:

-   [set-constant](#set-constantjs-)
-   [AdGuard `trusted-set-constant`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-set-constant)

* * *

### trusted-suppress-native-method.js [↪](https://github.com/gorhill/uBlock/blob/97d11c03c20bdc15877c342c404f179ca5c63ff6/assets/resources/scriptlets.js#L4838)

[](#trusted-suppress-native-methodjs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-13)

New in [1.58.1rc0](https://github.com/gorhill/uBlock/commit/97d11c03c2)

This is a first draft version with current limitations:

-   Does not support matching arguments which are object or array

Syntax:

-   `example.org##+js(trusted-suppress-native-method, methodPath, signatureStr[, how[, stack]])`

Parameters:

-   methodPath – required, string path to a native method (joined with `.` if needed). The property must be attached to `window`.
-   signatureStr – required, string of `|`\-separated argument matchers. Supported value types with corresponding matchers:
    -   string – exact string, part of the string or regexp pattern. Empty string `""` to match an empty string. Regexp patterns inside object matchers are not supported.
    -   number, boolean, null, undefined – exact value,
    -   object – partial of the object with the values as mentioned above, i.e by another object, that includes property names and values to be matched,
    -   array – partial of the array with the values to be included in the incoming array, without considering the order of values.

To ignore specific argument, explicitly use whitespace as a matcher, e.g `' | |{"prop":"val"}'` to skip matching first and second arguments.

If `signatureStr` parameter is not declared, the scriptlet will log all calls to `methodPath` along with the arguments passed and will not prevent the trapped method.

Tokens:

-   "how": optional, string, one of the following:
    -   `abort` – default, aborts the call by throwing an error,
    -   `prevent` – replaces the method call with the call of an empty function.
    -   `debug` – triggers a `debugger` statement and the target method is not suppressed. Useful to find out how the method is being called by the page's code.
-   "stack": optional, string or regular expression that must match the current function call stack trace. Prepend pattern with `!` to test for unmatched patterns in stack trace.

The scriptlet will log stack trace in uBO's logger when verbose mode is enabled.

Examples:

```
example.org##+js(trusted-suppress-native-method, localStorage.setItem, '/key/|"value"', prevent)
example.org##+js(trusted-suppress-native-method, Object.prototype.hasOwnProperty, '"test"')
example.org##+js(trusted-suppress-native-method, Node.prototype.appendChild, '{"id":"str"}', prevent)
example.org##+js(trusted-suppress-native-method, Document.prototype.querySelectorAll, '"div"')
example.org##+js(trusted-suppress-native-method, Array.prototype.concat, '\[1, "str", true\]')
example.org##+js(trusted-suppress-native-method, sessionStorage.setItem, ' |"item-value"', abort, someFuncName)
```

1.  Prevent `localStorage.setItem('test-key', 'test-value')` call matching first argument by regexp pattern and the second one by substring.
2.  Abort `obj.hasOwnProperty('test')` call matching the first argument.
3.  Prevent `Node.prototype.appendChild` call on element with the id `test-id` by object matcher.
4.  Abort all `document.querySelectorAll` calls with `div` as the first argument.
5.  Abort `Array.prototype.concat([1, 'str', true, null])` calls by matching array argument contents.
6.  Use stack argument to match by the call, while also matching the second argument.

Also see:

-   [AdGuard `trusted-suppress-native-method`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-suppress-native-method)

* * *

### trusted-override-element-method.js [↪](https://github.com/gorhill/uBlock/blob/947602d4fe46ec832d3f95ee3a65633fc4113f44/assets/resources/scriptlets.js#L4267)

[](#trusted-override-element-methodjs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-14)

New in [1.60.1b5](https://github.com/gorhill/uBlock/commit/95b0ce5e3a64d966924dd701d1e336402d586b25)

Override the behavior of a method on matching elements.

Parameters:

-   required, methodPath: The method which calls must be intercepted.
-   optional, selector: A CSS selector which the target element must match. If not specified, the override will occur for all elements.
-   optional, disposition: How the override should be handled. If not specified, the overridden call will be equivalent to an empty function.
    -   `throw`: an exception will be thrown (throws `ReferenceError`).
    -   `debug`: triggers a `debugger` statement if uBO's logger is opened. Useful for debugging purposes.
    -   Any other value will be validated and returned as a supported safe constant.

Example:

```
streamsilk.com##+js(trusted-override-element-method, HTMLAnchorElement.prototype.click, a\[target="\_blank"\]\[style\])
```

* * *

### trusted-prevent-dom-bypass.js [↪](https://github.com/gorhill/uBlock/blob/36db7f83274ae219d68e24d4aa477c007050e951/src/js/resources/scriptlets.js#L3568)

[](#trusted-prevent-dom-bypassjs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-15)

New in [1.60.1b4](https://github.com/gorhill/uBlock/commit/1abc8647425752960f6778fc451bd2a07cb3d3a6)

Prevent the bypassing of uBO scriptlets through anonymous embedded context.

Ensure that a target method in the embedded context is using the corresponding parent context's method (which is assumed to be properly patched), or to replace the embedded context with that of the parent context.

Root issue: [https://issues.chromium.org/issues/40202434](https://issues.chromium.org/issues/40202434)

Parameters:

-   required, methodPath: The method which calls must be intercepted. The arguments of the intercepted calls are assumed to be HTMLElement, anything else will be ignored.
-   optional, targetProp: The method in the embedded context which should be delegated to the parent context. If no method is specified, the embedded context becomes the parent one, i.e. all properties of the embedded context will be that of the parent context.

Examples:

-   `##+js(trusted-prevent-dom-bypass, Element.prototype.append, open)`
-   `##+js(trusted-prevent-dom-bypass, Element.prototype.appendChild, XMLHttpRequest)`

* * *

### set-cookie.js [↪](https://github.com/gorhill/uBlock/blob/4649ae4d78fa7d46e80d71d39d377d1b65309020/assets/resources/scriptlets.js#L2908)

[](#set-cookiejs-)

New in [1.50.1b0](https://github.com/gorhill/uBlock/commit/27a54c084556f657522b06484d2e28b21e1fac5a).

Sets a cookie with the specified name, value, and path. Double quotes surrouding the cookie value are allowed.

Parameters:

-   required, cookie name to be set
-   required, cookie value; possible values:
    -   decimal integer: `>= -32767 && <= 32767` (`<= 15` up to [1.56.1b18](https://github.com/gorhill/uBlock/commit/9146134874050ee50a11e25f31034a2416cf5988), `>= 0` up to [1.61.1b0](https://github.com/gorhill/uBlock/commit/e613282698167feda708e62a44b65d44dd528f06))
    -   one of the predefined constants (in any case variation):
        -   `accept` / `reject`,
        -   `accepted`2 / `rejected`2 / `notaccepted`2,
        -   `allow` / `disallow`2 / `deny`,
        -   `allowed`2 / `denied`,
        -   `approved`5 / `disapproved`5,
        -   `checked`8 / `unchecked`8,
        -   `dismiss` / `dismissed`,
        -   `enable`2 / `disable`2,
        -   `enabled`2 / `disabled`2,
        -   `essential`7 / `nonessential`7,
        -   `forbidden`9 / `forever`9,
        -   `hide`6 / `hidden`6,
        -   `necessary`4 / `required`4,
        -   `ok`,
        -   `on`1 / `off`1,
        -   `true` / `t`3 / `false` / `f`3,
        -   `yes` / `y` / `no` / `n`,
        -   `all`10 / `none`10 / `functional`10,
        -   `granted`11 / `done`11,
        -   `decline`12 / `declined`12,
        -   `closed`13 / `next`13 / `mandatory`13,
        -   `disagree`13 / `agree`13
-   optional, cookie path, defaults to /; possible values:
    -   `/`: root path
    -   `none`: to set no path at all

1\. [1.52.3b17](https://github.com/gorhill/uBlock/commit/eb66129fbfcdd3e1335cc5058d3afb1a887bad76)  
2\. [1.52.3rc12](https://github.com/gorhill/uBlock/commit/d51b393fdbdc7dec0e21086795b9ce394cf21adf)  
3\. [1.53.1b8](https://github.com/gorhill/uBlock/commit/4ab1c36ac9bfa07508b479f13334e6dd4e49cf0f)  
4\. [1.53.5rc5](https://github.com/gorhill/uBlock/commit/32fb93c5256047d8ddbc3599ecfd1568b578e7fe)  
5\. [1.57.3rc1](https://github.com/gorhill/uBlock/commit/7f0b2f4a5ef0013d4b5256adc343994bcce0f90e)  
6\. [1.58.1b7](https://github.com/gorhill/uBlock/commit/bdb99862ce93d61e9202ee1bad656276e4a9ee5a)  
7\. [1.58.1rc0](https://github.com/gorhill/uBlock/commit/37d31a82d8)  
8\. [1.59.1b5](https://github.com/gorhill/uBlock/commit/3e2171f550)  
9\. [1.60.1b12](https://github.com/gorhill/uBlock/commit/4d982d9972be4eb0a23e289c6f3ec3ae8cca36f2) 10\. [1.61.3b10](https://github.com/gorhill/uBlock/commit/791a2b08e1a15c169c0a86130de769822628c81d) 11\. [1.62](https://github.com/gorhill/uBlock/commit/62178de75682a9e7e8fc7d1bedda2c7f476b5b66) 12\. [1.62.1b1](https://github.com/gorhill/uBlock/commit/4b12247da106249a4d1e1143fd1c30d97656de73) 13\. [1.62.1b2](https://github.com/gorhill/uBlock/commit/35a47d674b47b14fed18691ce5ede6a7adddb4a1)

The scriptlet encodes cookie names and values, e.g value "{ test: 'value'}" becomes `%7B%20test%3A%20'value'%7D`.

Tokens:

-   `reload, 1`: the scriplet will force a reload of the webpage if the cookie being set was not already set.
-   `dontOverwrite, 1`: the scriptlet will not modify the cookie if it already exists.

`; Secure` will be automatically used when cookie names starts with `__Secure-` or `__Host-` (since [1.58.1b8](https://github.com/gorhill/uBlock/commit/b4d8750f445cfa06bebd184a6f3cdb4d73148e72)).

Examples:

-   `example.com##+js(set-cookie, CookieConsent, 1)`
-   `example.com##+js(set-cookie, gdpr-settings-cookie, true)`
-   `example.com##+js(set-cookie, cookie_consent, ok, none)`

Also see:

-   [trusted-set-cookie](#trusted-set-cookiejs-)
-   [AdGuard `set-cookie`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#set-cookie)
-   [AdGuard `set-cookie-reload`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#set-cookie-reload)

* * *

### trusted-set-cookie.js [↪](https://github.com/gorhill/uBlock/blob/4649ae4d78fa7d46e80d71d39d377d1b65309020/assets/resources/scriptlets.js#L3191)

[](#trusted-set-cookiejs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-16)

Sets a cookie with arbitrary name and value, and with optional ability to offset cookie attribute 'expires' and set path.

New in [1.50.1b1](https://github.com/gorhill/uBlock/commit/eaea26b5e97e147560cefa9118134b2e40060cf6).

Parameters:

-   required, cookie name to be set
-   required, cookie value. Possible values:
    -   arbitrary value
    -   empty string for no value
    -   `$now$` keyword for setting current time in ms, e.g 1667915146503. Can be used inside a string since [1.58.1b0](https://github.com/gorhill/uBlock/commit/0e1e4b82c5ea464f1e7837c5c8f400e5ffac4b3c)
    -   `$currentDate$` keyword for setting current time as string, e.g 'Tue Nov 08 2022 13:53:19 GMT+0300'
    -   `$currentISODate$` keyword for setting current time in ISO format as string, e.g '2024-07-07T12:10:00.641Z' (since [1.58.1rc0](https://github.com/gorhill/uBlock/commit/a3576ea6519dc08e5244dafc296dc8ac31b07655))
-   optional, offset from current time in seconds, after which cookie should expire; defaults to no offset. Possible values:
    -   positive integer in seconds
    -   `1year` keyword for setting expiration date to one year
    -   `1day` keyword for setting expiration date to one day
-   optional, argument for setting cookie path, defaults to `/`; possible values:
    -   `/`: root path
    -   `none`: to set no path at all

Variadic parameters (must appears after required and optional parameters):

-   `reload, 1`: the scriplet will force a reload of the webpage if the cookie being set was not already set.
-   `domain, [value]`: the scriptlet will set the domain of the cookie to _value_. Regex is supported ([1.64.1b5](https://github.com/gorhill/uBlock/commit/3a2bb625190488b18c1486e51a1dc8fa465c14b4)), if used it will be matched against `document.location.hostname`.
-   `dontOverwrite, 1`: the scriptlet will not modify the cookie if it already exists.

`; Secure` will be automatically used when cookie names starts with `__Secure-` or `__Host-`.

Examples:

-   `example.com##+js(trusted-set-cookie, cmpconsent, 1-accept_1)`
-   `example.com##+js(trusted-set-cookie, cmpconsent, $now$)`
-   `example.com##+js(trusted-set-cookie, cmpconsent, '{"accepted":true,"expire":$now$}')`
-   `example.com##+js(trusted-set-cookie, cmpconsent, accept, 259200)`
-   `example.com##+js(trusted-set-cookie, cmpconsent, accept, 1year)`
-   `example.com##+js(trusted-set-cookie, cmpconsent, decline, , none)`
-   `www.google.*##+js(trusted-set-cookie, SOCS, CAESHAgBEhJnd3NfMjAyNTA2MDQtMF9SQzEaAnJvIAEaBgiAgo7CBg, 1year, , domain, /\bgoogle\..+$/, dontOverwrite, 1)`

The second and third filters will set a cookie with `new Date().getTime()` value.

The forth and fifth filters will set a cookie which will expire in 3 days or 1 year.

The sixth filter sets a cookie with no path.

The seventh filter sets a cookie where the domain value is the result of the regex matched against `document.location.hostname`.

Also see:

-   [set-cookie](#set-cookiejs-)
-   [AdGuard `trusted-set-cookie`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-set-cookie)
-   [AdGuard `trusted-set-cookie-reload`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-set-cookie-reload)

* * *

### set-local-storage-item.js [↪](https://github.com/gorhill/uBlock/blob/4649ae4d78fa7d46e80d71d39d377d1b65309020/assets/resources/scriptlets.js#L2959)

[](#set-local-storage-itemjs-)

### set-session-storage-item.js [↪](https://github.com/gorhill/uBlock/blob/4649ae4d78fa7d46e80d71d39d377d1b65309020/assets/resources/scriptlets.js#L2959)

[](#set-session-storage-itemjs-)

New in [1.50.1b4](https://github.com/gorhill/uBlock/commit/9443ba80a06c8748fc5164bb0ad6da752a8b3bf3) and [1.50.1b12](https://github.com/gorhill/uBlock/commit/083a318090e38fb81c2b94ea326521d5a76f7c57).

Set a local/session storage entry to a specific, allowed value. Scriptlet won't set item if storage is full. Double quotes surrouding the key value are allowed.

-   required, key name to be set.
-   required, key value; possible values:
    -   decimal integer `>= -32767 && <= 32767` (only `<= 32767` up to [1.61.1b1](https://github.com/gorhill/uBlock/commit/e5a088738dd7aeb7b84df6766ae036de76f82000))
    -   one of the predefined constants:
        -   `''`: empty string
        -   `undefined`, `null`
        -   `false`, `true`
        -   `on`, `off`
        -   `yes`, `no`
        -   `accept`, `reject` [1.57.3b9](https://github.com/gorhill/uBlock/commit/363ad6795c6fdbc42bf9aae468f1cb81bd990ab0)
        -   `accepted`, `rejected` [1.57.3b9](https://github.com/gorhill/uBlock/commit/363ad6795c6fdbc42bf9aae468f1cb81bd990ab0)
        -   `allowed`, `denied` [1.59.1b5](https://github.com/gorhill/uBlock/commit/41c2258f91)
        -   any from [`set-cookie`](#set-cookiejs-) [1.59.1b5](https://github.com/gorhill/uBlock/commit/79e10323ad)
        -   `{}`: empty object
        -   `[]`: empty array
        -   `""`
        -   `$remove$`: remove specific item from localStorage

Examples:

-   `example.com##+js(set-local-storage-item, player.live.current.mute, false)`
-   `example.com##+js(set-local-storage-item, exit-intent-marketing, 1)`
-   `example.com##+js(set-local-storage-item, foo, $remove$)`
-   `example.com##+js(set-session-storage-item, player.live.current.mute, false)`
-   `example.com##+js(set-session-storage-item, exit-intent-marketing, 1)`
-   `example.com##+js(set-session-storage-item, foo, $remove$)`

Solves [uBlockOrigin/uBlock-issues#2697](https://github.com/uBlockOrigin/uBlock-issues/discussions/2697).

Also see:

-   [trusted-set-local-storage-item](#trusted-set-local-storage-itemjs-)
-   [AdGuard `set-local-storage-item`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#set-local-storage-item)

* * *

### trusted-set-local-storage-item.js [↪](https://github.com/gorhill/uBlock/blob/c7490e8493d449f1f4ddb6f415a4d7506f0a1802/assets/resources/scriptlets.js#L3853)

[](#trusted-set-local-storage-itemjs-)

### trusted-set-session-storage-item.js [↪](https://github.com/gorhill/uBlock/blob/c7490e8493d449f1f4ddb6f415a4d7506f0a1802/assets/resources/scriptlets.js#L3853)

[](#trusted-set-session-storage-itemjs-)

#### _Trusted scriptlet_

[](#trusted-scriptlet-17)

New in [1.50.1b4](https://github.com/gorhill/uBlock/commit/9443ba80a06c8748fc5164bb0ad6da752a8b3bf3) and [1.53.5rc7](https://github.com/gorhill/uBlock/commit/f3d6a21e7abfd0e5ad584cc9e6c15bf24d2e05a6).

Parameters:

-   required, key name to be set.
-   required, key value; possible values:
    -   arbitrary value
    -   `$now$`: keyword for setting current time in ms,  
        corresponds to `Date.now()` and `(new Date).getTime()` calls, can be used inside a string since [1.57.3b8](https://github.com/gorhill/uBlock/commit/2ccc3135c1)
    -   `$currentDate$`: keyword for setting string representation of the current date and time,  
        corresponds to `Date()` and `(new Date).toString()` calls, can be used inside a string since [1.57.3b8](https://github.com/gorhill/uBlock/commit/2ccc3135c1)
    -   `$currentISODate$`: keyword for setting string representation of the current date and time in ISO format,  
        corresponds to `Date()` and `(new Date).toISOString()` calls, can be used inside a string since [1.57.3b8](https://github.com/gorhill/uBlock/commit/2ccc3135c1)

Examples:

-   `example.com##+js(trusted-set-local-storage-item, COOKIE_CONSENTS, {"preferences":3\,"flag":false})`
-   `example.com##+js(trusted-set-local-storage-item, providers, [16364\,88364])`
-   `example.com##+js(trusted-set-local-storage-item, player.live.current.play, $currentDate$)`
-   `example.com##+js(trusted-set-local-storage-item, ppu_main_none, '')`
-   `example.com##+js(trusted-set-session-storage-item, opd, 1.4)`

Also see:

-   [set-local-storage-item](#set-local-storage-itemjs-)
-   [AdGuard `trusted-set-local-storage-item`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-trusted-scriptlets.md#trusted-set-local-storage-item)

* * *

### remove-cache-storage-item.js [↪](https://github.com/gorhill/uBlock/blob/66e3a1ad47162a89709e99072e5eaecdd83cb633/assets/resources/scriptlets.js#L4076)

[](#remove-cache-storage-itemjs-)

New in [1.55.1b23](https://github.com/gorhill/uBlock/commit/6c54731a728b06f89a66ddc851ebae3e19e45448)

Removes cache storage item.

Syntax: `...##+js(remove-cache-storage-item, cacheNamePattern[, urlPattern])`

Parameters:

-   required, `cacheNamePattern`: the name of the cache to target. Plain string or _regex_.
-   optional, `urlPattern`: the URL of the resource to remove. Plain string or _regex_.  
    If no pattern is provided, the whole cache is removed.

Reference: [https://developer.mozilla.org/en-US/docs/Web/API/CacheStorage](https://developer.mozilla.org/en-US/docs/Web/API/CacheStorage)

* * *

### adjust-setInterval.js [↪](https://github.com/gorhill/uBlock/blob/001f5a650084ffa4842f9361bc975ca724bd69ba/assets/resources/scriptlets.js#L463)

[](#adjust-setintervaljs-)

### nano-setInterval-booster.js /

[](#nano-setinterval-boosterjs-)

### nano-sib.js

[](#nano-sibjs)

Adjusts interval for specified _`setInterval()`_ callbacks.

Parameters:

-   optional, string/_regular expression_, matching in stringified callback function
-   optional - defaults to 1000, decimal integer, matching interval. New in [1.33.0](https://github.com/gorhill/uBlock/commit/001f5a650084ffa4842f9361bc975ca724bd69ba): `*` will match any interval.
-   optional - default to 0.05 (20x faster), float, capped at 50 times for up and down, interval multiplier

Examples:

-   `identi.li##+js(nano-sib, , ,0.02)`
-   `platinmods.*##+js(nano-sib)`
-   `1ink.cc##+js(nano-sib, mSec, 1050)`

Also see:

-   [AdGuard `adjust-setinterval`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#adjust-setinterval)

* * *

### spoof-css.js [↪](https://github.com/gorhill/uBlock/blob/f3b720d532c7a42a6ad5167e3b6f860004b4c2b6/assets/resources/scriptlets.js#L2443)

[](#spoof-cssjs-)

Spoof the CSS property value when `getComputedStyle()` or `getBoundingClientRect()` are used.

Parameters:

-   required, a valid CSS selector which matches the elements for which the spoofing must apply
-   required, a CSS property name (can be dashed- or camel-cased)
-   required, the value to return regardless of the currently computed value

Examples:

-   ```
    example.com##+js(spoof-css, .ad, clip-path, none)
    example.com##+js(spoof-css, .ad, \_debug, 1)
    ```
    

There can be any number of selectors, grouped inside single quotes:

-   ```
    example.com##+js(spoof-css, 'a\[href="x.com"\], .ads, .bottom', clip-path, none)
    ```
    

There can be any number of property-name/property-value pairs, all separated by commas:

-   ```
    example.com##+js(spoof-css, .ad, clip-path, none, display, block)
    ```
    

Special properties: [1.61.1b4](https://github.com/gorhill/uBlock/commit/5f5e3d730fabe0e98e2f9931ad3c36113b7b9893)

-   `_debug, 1`: Force the browser to break when `getComputedStyle()` or `getBoundingClientRect()` is called, useful to help pinpoint usage of those calls in the page's source code.
-   `_rectx, value`: Spoof the [`DOMRect.x`](https://developer.mozilla.org/en-US/docs/Web/API/DOMRect/x) returned by [`getBoundingClientRect()`](https://developer.mozilla.org/en-US/docs/Web/API/DOMRect/DOMRect).
-   `_recty, value`: Spoof the [`DOMRect.y`](https://developer.mozilla.org/en-US/docs/Web/API/DOMRect/y) returned by [`getBoundingClientRect()`](https://developer.mozilla.org/en-US/docs/Web/API/DOMRect/DOMRect).
-   `_rectw, value`: Spoof the [`DOMRect.width`](https://developer.mozilla.org/en-US/docs/Web/API/DOMRect/width) returned by [`getBoundingClientRect()`](https://developer.mozilla.org/en-US/docs/Web/API/DOMRect/DOMRect).
-   `_recth, value`: Spoof the [`DOMRect.height`](https://developer.mozilla.org/en-US/docs/Web/API/DOMRect/height) returned by [`getBoundingClientRect()`](https://developer.mozilla.org/en-US/docs/Web/API/DOMRect/DOMRect).

If both `_rectw` and `width` are present in the list of properties to spoof, `_rectw` will have precedence over `width` in the spoofing of `getBoundingClientRect()` output. Same for `_recth`.

Solves [uBlockOrigin/uBlock-issues#2618](https://github.com/uBlockOrigin/uBlock-issues/issues/2618).

Also see:

-   [AdGuard `spoof-css`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#spoof-css)

* * *

### prevent-canvas.js [↪](https://github.com/gorhill/uBlock/blob/66e3a1ad47162a89709e99072e5eaecdd83cb633/assets/resources/scriptlets.js#L4030)

[](#prevent-canvasjs-)

New in [1.52.3b2](https://github.com/gorhill/uBlock/commit/1ff3878a4920e31791547238e3e879a32357914f)

Prevent usage of specific or all (default) canvas APIs.

Syntax:

-   `example.com##+js(prevent-canvas [, contextType])`

Parameters:

-   optional, `contextType`: A specific type of canvas API to prevent (default to all APIs). Can be a string or regex which will be matched against the type used in getContext() call. Prepend with `!` to test for no-match.

Examples:

-   `example.com##+js(prevent-canvas)` - prevents `example.com` from accessing all canvas APIs
-   `*##+js(prevent-canvas, /webgl/)` - prevents access to any flavor of WebGL API, everywhere
-   `example.com##+js(prevent-canvas, !2d)` - prevents `example.com` from accessing any flavor of canvas API except `2d`

References: [https://developer.mozilla.org/en-US/docs/Web/API/HTMLCanvasElement/getContext](https://developer.mozilla.org/en-US/docs/Web/API/HTMLCanvasElement/getContext)

* * *

### adjust-setTimeout.js [↪](https://github.com/gorhill/uBlock/blob/001f5a650084ffa4842f9361bc975ca724bd69ba/assets/resources/scriptlets.js#L513)

[](#adjust-settimeoutjs-)

### nano-setTimeout-booster.js /

[](#nano-settimeout-boosterjs-)

### nano-stb.js

[](#nano-stbjs)

Adjusts delay for specified _`setTimeout()`_ callbacks.

Parameters:

-   optional, string/_regular expression_, matching in stringified callback function
-   optional - defaults to 1000, decimal integer, matching delay. New in [1.33.0](https://github.com/gorhill/uBlock/commit/001f5a650084ffa4842f9361bc975ca724bd69ba): `*` will match any delay.
-   optional - default to 0.05 (20x faster), float, capped at 50 times for up and down, delay multiplier

Examples:

-   `bdupload.*##+js(nano-stb)`
-   `imgrock.*##+js(nano-stb, /.?/, 4000)`

Also see:

-   [AdGuard `adjust-settimeout`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#adjust-settimeout)

* * *

### sharedWorker-defuser.js [↪](https://github.com/uBlockOrigin/uAssets/blob/2c68a4f5456e4677cec76f2784d2c1d7abc36efb/filters/resources.txt#L1822)

[](#sharedworker-defuserjs-)

Removed. Deprecated by `$csp` filter option.  
Defuses sharedWorker by passing empty worker file (Blob URL) for specified worker URLs

Parameters:

-   optional, string/_regular expression_, matching in worker URL

* * *

### webrtc-if.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L556)

[](#webrtc-ifjs-)

Allows opening RTC connections to matching [RTCIceServer](https://developer.mozilla.org/en-US/docs/Web/API/RTCIceServer) only.

Parameters:

-   required, string/_regular expression_, matching in `RTCIceServer` `urls`, `username` or `credential`.

* * *

### close-window.js [↪](https://github.com/gorhill/uBlock/blob/8d7469afcfae7dabefe819eb513a0ba026ed9dd7/assets/resources/scriptlets.js#L1273)

[](#close-windowjs-)

### window-close-if.js

[](#window-close-ifjs)

New in [1.39.3b10](https://github.com/gorhill/uBlock/commit/c198b9a748265c0e1ce7f5bad4528d5bf6ce8161).

Closes fresh browser tabs of the specified page. Can also be used to close tabs which have been opened from other applications. Can be narrowed down to specific path by parameter. Whole browser window will be closed if it's the last/only tab (depends on browser configuration).

Improvements:

-   [1.44.3b11](https://github.com/gorhill/uBlock/commit/65a056107210796426033ebe2eebb89a98c93a23), If the argument to the `window-close-if` scriptlet is a regex, the match will be against _the whole location URL_, otherwise the match will be against the part+query part of the location URL.

Parameters:

-   optional, string, matching in the path and query part of the web page address or _regular expression_, matching in the whole location URL.

Examples:

-   `acestream.com##+js(window-close-if, /plan/select?popup=noads)`
-   `example.com##+js(window-close-if, /^/)` - will close all new tabs going to `example.com` on _any_ site.
-   `hostdl.com##+js(window-close-if, /^https?://(www\.)?hostdl\.com/)` - will close all new tabs matching either only base domain or www one (i.e. **not** `login.hostdl.com`).

Also see:

-   [AdGuard `close-window`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#close-window)

* * *

### prevent-window-open.js [↪](https://github.com/gorhill/uBlock/blob/4649ae4d78fa7d46e80d71d39d377d1b65309020/assets/resources/scriptlets.js#L1879)

[](#prevent-window-openjs-)

### nowoif.js /

[](#nowoifjs-)

### no-window-open-if.js /

[](#no-window-open-ifjs-)

### window.open-defuser.js

[](#windowopen-defuserjs)

Prevent opening new windows by [_`window.open()`_](https://developer.mozilla.org/en-US/docs/Web/API/Window/open) when URL positively or negatively matches to specific string.

Improvements:

-   [1.29.2](https://github.com/gorhill/uBlock/commit/d544543ab580930733c4def8817fbff251ad10ce), third parameter can now configure behavior of the scriptlet.
-   [1.27.0](https://github.com/gorhill/uBlock/commit/6259f88598b2d3e044679d6fe0fdb6eb16f6c479), `nowoif.js` alias is now available.
-   1.26.0 ([one](https://github.com/gorhill/uBlock/commit/b27848a060eee961e2403192097448467b3bc7b5), [two](https://github.com/gorhill/uBlock/commit/0f33f2386d147e4930b402a07418da670524e43f)),  
    if second argument is present and a valid integer value, the defuser will return a valid window object even though no popup window is opened. The returned window object will cease to be valid after the specified number of seconds. If not present, no window will be opened and the scriptlet will return `null`.

If the first argument is set to the special value `debug` _and_ the logger is opened, the scriptlet will trigger a `debugger` statement and the prevention will not occur.

When used without parameters, will log calls to _`window.open()`_ to the logger.

Parameters:

-   optional, string/_regular expression_, prefixed by `!` for negation, matching in all parameters passed to _`window.open()`_ (all the arguments are joined as a single space-spearated string, and the result is used as the target for matching the pattern, new in [1.50.1b2](https://github.com/gorhill/uBlock/commit/0bd4b600cf78a988b8ed677780ec258518822663)), if not provided, all calls to `window.open()` are prevented.
-   optional, positive decimal integer, number of seconds after returned `window` object will be invalidated.
-   optional, possible values:
    -   `obj`: Use an `object` element instead of `iframe` element (default) as a decoy to be used in place of a popup window, when the page requires a valid `window` instance to be returned.
    -   `blank`: Replace the `url` parameter with `about:blank`. [1.59.1b9](https://github.com/gorhill/uBlock/commit/7f11d6216eb866fdbec4c13d9766b76432908f4f)

Tokens:

-   `log, 1`: Cause the scriptlet to log information regarding how `window.open()` is used by the page on which the scriptlet is used.  
    (deprecated, read about [the new method](#general-purpose-scriptlets) of logging scriptlets)

Examples:

-   `file-up.org##+js(nowoif)`
-   `vidstream.*##+js(nowoif, !api?call=, 10, obj)`
-   `example.com##+js(nowoif, download-link, , , log, 1)`

Also see:

-   [AdGuard `prevent-window-open`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-scriptlets.md#prevent-window-open)

* * *

### window.name-defuser [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L624)

[](#windowname-defuser-)

Clears `window.name` _property_ which can be misused for tracking purposes.

Parameters:

-   none

* * *

### overlay-buster.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L633)

[](#overlay-busterjs-)

Experimental, gets rid of overlay dialogs, works for ~30s after page load. Preferred way to handle overlays is to use standard cosmetic filters and optionally [style injection](https://github.com/gorhill/ublock/wiki/Static-filter-syntax#style).

* * *

### alert-buster.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L691)

[](#alert-busterjs-)

Disables [_`alert()`_](https://developer.mozilla.org/en-US/docs/Web/API/Window/alert) dialog boxes by redirecting messages to console.

* * *

## Defuser scriptlets

[](#defuser-scriptlets)

### ampproject\_v0.js /

[](#ampproject_v0js-)

### ampproject.org/v0.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/ampproject_v0.js)

[](#ampprojectorgv0js-)

Removes animation (artificial 8s delay) added to desktop pages supporting AMP, when ampproject.org scripts are blocked.

### fingerprint2.js [↪](https://github.com/gorhill/uBlock/blob/master/src/web_accessible_resources/fingerprint2.js)

[](#fingerprint2js-)

Fingerprintjs2 shim.

Also see: [AdGuard `fingerprintjs2`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-redirects.md#fingerprintjs2)

### fingerprint3.js [↪](https://github.com/gorhill/uBlock/blob/master/src/web_accessible_resources/fingerprint3.js)

[](#fingerprint3js-)

FingerprintJS v3 shim.

Also see: [AdGuard `fingerprintjs3`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-redirects.md#fingerprintjs3)

### nobab.js /

[](#nobabjs-)

### bab-defuser.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/nobab.js)

[](#bab-defuserjs-)

Defuses BlockAdblock. Prevents executing of _`eval()`_ on sets of predefined payloads.

Also see: [AdGuard `prevent-bab`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-redirects.md#prevent-bab)

### nobab2.js [↪](https://github.com/gorhill/uBlock/blob/d17d634b7c95261c376b42c0fb0a65fc9eff32ae/src/web_accessible_resources/nobab2.js)

[](#nobab2js-)

Redirect resource. Defuses BAB 4.2b.

Also see: [AdGuard `prevent-bab2`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-redirects.md#prevent-bab2)

### nofab.js /

[](#nofabjs-)

### fuckadblock.js-3.2.0 [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/nofab.js)

[](#fuckadblockjs-320-)

Convenience, Sanitize `FuckAdBlock`, `BlockAdBlock`, `SniffAdBlock`, `fuckAdBlock`, `blockAdBlock`, `sniffAdBlock` properties. Often used as redirect in network filters. TODO: copy to redirect?

Also see: [AdGuard `prevent-fab-3.2.0`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-redirects.md#prevent-fab-3.2.0)

### popads-dummy.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/popads-dummy.js)

[](#popads-dummyjs-)

Convenience, sets static properties (`PopAds`, `popns`)

Also see: [AdGuard `set-popads-dummy`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-redirects.md#set-popads-dummy)

### popads.js /

[](#popadsjs-)

### popads.net.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/popads.js)

[](#popadsnetjs-)

Convenience, abort-on-property-write.js (`PopAds`, `popns`), _throws_ "`magic`"

Also see: [AdGuard `prevent-popads-net`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-redirects.md#prevent-popads-net)

### prebid-ads.js [↪](https://github.com/gorhill/uBlock/blob/master/src/web_accessible_resources/prebid-ads.js)

[](#prebid-adsjs-)

New in 1.41.0

Prebid-ads shim. `canRunAds`/`isAdBlockActive`?

Also see: [AdGuard `prebid-ads`](https://github.com/AdguardTeam/Scriptlets/blob/master/wiki/about-redirects.md#prebid-ads)

## Empty redirect resources

[](#empty-redirect-resources)

These are smallest/shortest/fastest to execute files. Should be used in network filters as a parameter to `$redirect` option. They purpose is to mislead page to think that real files have been served.

### Available resources

[](#available-resources)

-   Images
    -   `1x1.gif` [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/1x1.gif)
    -   `2x2.png` [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/2x2.png)
    -   `3x2.png` [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/3x2.png)
    -   `32x32.png` [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/32x32.png)
-   Source code
    -   `noop.css` [↪](https://github.com/gorhill/uBlock/blob/4564e3a9b8d3cb2afb70ebd2161271f6b9b969bc/src/web_accessible_resources/noop.css)
    -   `noop.html` [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/noop.html)
    -   `noop.js` [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/noop.js)
    -   `noop.json` [↪](https://github.com/gorhill/uBlock/blob/ff57f01026f1b98ca9d9a8bb36386ae9dfd3eae3/src/web_accessible_resources/noop.json)
    -   `noop.txt` [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/noop.txt)
    -   `noop-vast2.xml` [↪](https://github.com/gorhill/uBlock/blob/ff57f01026f1b98ca9d9a8bb36386ae9dfd3eae3/src/web_accessible_resources/noop-vast2.xml)
    -   `noop-vast3.xml` [↪](https://github.com/gorhill/uBlock/blob/ff57f01026f1b98ca9d9a8bb36386ae9dfd3eae3/src/web_accessible_resources/noop-vast3.xml)
    -   `noop-vast4.xml` [↪](https://github.com/gorhill/uBlock/blob/ff57f01026f1b98ca9d9a8bb36386ae9dfd3eae3/src/web_accessible_resources/noop-vast4.xml)
    -   `noop-vmap1.xml` [↪](https://github.com/gorhill/uBlock/blob/ff57f01026f1b98ca9d9a8bb36386ae9dfd3eae3/src/web_accessible_resources/noop-vmap1.xml)
-   Media files
    -   `noop-0.1s.mp3` [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/noop-0.1s.mp3)
    -   `noop-0.5s.mp3` [↪](https://github.com/gorhill/uBlock/blob/c521479ef9d9676e08fcd6751fde7330dce189e7/src/web_accessible_resources/noop-0.5s.mp3)
    -   `noop-1s.mp4` [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/noop-1s.mp4)
-   Special purpose
    -   `none`
        
        reserved token, can be used to disable specific redirect filters. Starting with [1.31.0](https://github.com/gorhill/uBlock/commit/157cef6034a8ec926c1e59c7e77f0a1fcbef473c), classic exception filters and `badfilter` option can be used.
        
    -   `click2load.html`
        
        for embedded `<iframe>` elements. New in [1.31.0](https://github.com/gorhill/uBlock/commit/59169209850c54c31d94990f0c956281fe43eb03) (also [2e5d32e9](https://github.com/gorhill/uBlock/commit/2e5d32e96798dd55f3fae66d7091645ff7ad3784), [46d7f8a7](https://github.com/gorhill/uBlock/commit/46d7f8a70c937441545db9c53df2647081ee9d12)). Frames redirected to this resource will not be collapsed, instead, widget with clickable and selectable frame source link will be displayed. Clicking the icon next to source link will open frame content in new tab. Clicking the widget content will unblock and load original frame content.
        

Example rules:

-   `||ad.server.com/$script,redirect=noop.js,domain=www.google.com`
-   `||dailymotion.com$3p,frame,redirect=click2load.html,domain=lemonde.fr` for example video link:  
    `https://www.lemonde.fr/international/video/2024/08/23/en-images-le-discours-de-kamala-harris-pour-clore-la-convention-democrate_6291494_3210.html`
-   `||vimeo.com$3p,frame,redirect=click2load.html,domain=theregister.com` for example video link:  
    `https://www.theregister.com/2024/08/01/a_fresh_approach_for_container/`

* * *

## URL-specific sanitized redirect resources (surrogates)

[](#url-specific-sanitized-redirect-resources-surrogates)

### amazon\_ads.js /

[](#amazon_adsjs-)

### amazon-adsystem.com/aax2/amzn\_ads.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/amazon_ads.js)

[](#amazon-adsystemcomaax2amzn_adsjs-)

### amazon\_apstag.js [↪](https://github.com/gorhill/uBlock/blob/f842ab6d3c1cf0394f95d27092bf59627262da40/src/web_accessible_resources/amazon_apstag.js)

[](#amazon_apstagjs-)

New in [1.27.0](https://github.com/gorhill/uBlock/commit/f842ab6d3c1cf0394f95d27092bf59627262da40).

### doubleclick\_instream\_ad\_status.js /

[](#doubleclick_instream_ad_statusjs-)

### doubleclick.net/instream/ad\_status.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/doubleclick_instream_ad_status.js)

[](#doubleclicknetinstreamad_statusjs-)

### google-analytics\_ga.js /

[](#google-analytics_gajs-)

### google-analytics.com/ga.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/google-analytics_ga.js)

[](#google-analyticscomgajs-)

### google-analytics\_analytics.js /

[](#google-analytics_analyticsjs-)

### google-analytics.com/analytics.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/google-analytics_analytics.js)

[](#google-analyticscomanalyticsjs-)

### google-analytics\_inpage\_linkid.js /

[](#google-analytics_inpage_linkidjs-)

### google-analytics.com/inpage\_linkid.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/google-analytics_inpage_linkid.js)

[](#google-analyticscominpage_linkidjs-)

### google-analytics\_cx\_api.js /

[](#google-analytics_cx_apijs-)

### google-analytics.com/cx/api.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/google-analytics_cx_api.js)

[](#google-analyticscomcxapijs-)

### google-ima.js [↪](https://github.com/gorhill/uBlock/blob/93e5133783301c0329b3ce8f9e6079badd06d62d/src/web_accessible_resources/google-ima.js)

[](#google-imajs-)

### googletagservices\_gpt.js /

[](#googletagservices_gptjs-)

### googletagservices.com/gpt.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/googletagservices_gpt.js)

[](#googletagservicescomgptjs-)

### googletagmanager\_gtm.js /

[](#googletagmanager_gtmjs-)

### googletagmanager.com/gtm.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/googletagmanager_gtm.js)

[](#googletagmanagercomgtmjs-)

### googlesyndication\_adsbygoogle.js /

[](#googlesyndication_adsbygooglejs-)

### googlesyndication.com/adsbygoogle.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/googlesyndication_adsbygoogle.js)

[](#googlesyndicationcomadsbygooglejs-)

### scorecardresearch\_beacon.js /

[](#scorecardresearch_beaconjs-)

### scorecardresearch.com/beacon.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/scorecardresearch_beacon.js)

[](#scorecardresearchcombeaconjs-)

### outbrain-widget.js /

[](#outbrain-widgetjs-)

### widgets.outbrain.com/outbrain.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/outbrain-widget.js)

[](#widgetsoutbraincomoutbrainjs-)

### hd-main.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/hd-main.js)

[](#hd-mainjs-)

### disqus\_forums\_embed.js AND disqus\_embed.js /

[](#disqus_forums_embedjs-and-disqus_embedjs-)

### disqus.com/forums/\*/embed.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/disqus_forums_embed.js) AND disqus.com/embed.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/disqus_embed.js)

[](#disquscomforumsembedjs--and-disquscomembedjs-)

Removed in [1.29.0](https://github.com/gorhill/uBlock/commit/7c22a312945a2bff41a2b5696a7e54f1c4c01cf2).

* * *

## Other

[](#other)

Deprecated by general purpose scriptlets / outdated (please move to proper section if still used).

* * *

### golem.de.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/assets/resources/scriptlets.js#L756)

[](#golemdejs-)

Deprecated, addEventListener-defuser

* * *

### chartbeat.js /

[](#chartbeatjs-)

### static.chartbeat.com/chartbeat.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/chartbeat.js)

[](#staticchartbeatcomchartbeatjs-)

Deprecated, sets static properties (`pSUPERFLY.activity`, `pSUPERFLY.virtualPage`)

### ligatus\_angular-tag.js /

[](#ligatus_angular-tagjs-)

### ligatus.com/\*/angular-tag.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/ligatus_angular-tag.js)

[](#ligatuscomangular-tagjs-)

Deprecated, sets static properties (`adProtect`, `uabpdl`, `uabDetect`)

### addthis\_widget.js /

[](#addthis_widgetjs-)

### addthis.com/addthis\_widget.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/addthis_widget.js)

[](#addthiscomaddthis_widgetjs-)

### monkeybroker.js /

[](#monkeybrokerjs-)

### d3pkae9owd2lcf.cloudfront.net/mb105.js [↪](https://github.com/gorhill/uBlock/blob/a94df7f3b27080ae2dcb3b914ace39c0c294d2f6/src/web_accessible_resources/monkeybroker.js)

[](#d3pkae9owd2lcfcloudfrontnetmb105js-)

* * *

## Glossary

[](#glossary)

-   `throw`: [https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/throw](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/throw)
-   `eval()`: [https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global\_Objects/eval](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/eval)
-   `setInterval()` [https://developer.mozilla.org/en-US/docs/Web/API/WindowOrWorkerGlobalScope/setInterval](https://developer.mozilla.org/en-US/docs/Web/API/WindowOrWorkerGlobalScope/setInterval)
-   `setTimeout()` [https://developer.mozilla.org/en-US/docs/Web/API/WindowOrWorkerGlobalScope/setTimeout](https://developer.mozilla.org/en-US/docs/Web/API/WindowOrWorkerGlobalScope/setTimeout))
-   regular expression: [https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/Regular\_Expressions#Writing\_a\_regular\_expression\_pattern](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/Regular_Expressions#Writing_a_regular_expression_pattern)
-   element: [https://developer.mozilla.org/en-US/docs/Web/HTML/Element](https://developer.mozilla.org/en-US/docs/Web/HTML/Element)
-   property: [https://developer.mozilla.org/en-US/docs/Glossary/property/JavaScript](https://developer.mozilla.org/en-US/docs/Glossary/property/JavaScript)
-   method: [https://developer.mozilla.org/en-US/docs/Glossary/Method](https://developer.mozilla.org/en-US/docs/Glossary/Method)
-   CSS selector: [https://developer.mozilla.org/en-US/docs/Web/CSS/CSS\_Selectors](https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_Selectors)