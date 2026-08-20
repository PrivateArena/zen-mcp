# Procedural cosmetic filters
Source: https://github.com/gorhill/ublock/wiki/Procedural-cosmetic-filters

[Back to _"Static filter syntax"_ / _"Cosmetic filters"_](https://github.com/gorhill/ublock/wiki/Static-filter-syntax#cosmetic-filters)

* * *

The concept of procedural cosmetic filtering was introduced with uBlock Origin (uBO) [version 1.8.0](https://github.com/gorhill/uBlock/releases/tag/1.8.0).

The initial implementation was revised to allow chained/recursive use of the procedural operators with version [1.11.0](https://github.com/gorhill/uBlock/releases/tag/1.11.0)+. There is no limit on the number of operators chained, or the number of recursion level, aside common sense. Though, chaining to native CSS selector after procedural one was not supported before [1.17.5rc1](https://github.com/gorhill/uBlock/commit/8a88e9d93174badd6855c0e782737158c9ccd6f8).

Only use procedural cosmetic filters when plain CSS selectors won't work.

Regular cosmetic filters are _declarative_, i.e., they are used as a selector in a CSS rule and handled by browsers through `style` tag elements.

_Procedural_ means JavaScript code will find DOM elements that it must hide. A procedural cosmetic filter uses a filter _operator_ that will tell uBO how to find/filter DOM elements to find which DOM elements to target.

#### Important:

[](#important)

1.  Procedural filters must always be specific, i.e. prefixed with the hostname of the site(s) on which they are meant to apply\[[exception](https://github.com/gorhill/uBlock/wiki/Advanced-settings#allowgenericproceduralfilters)\]. If a procedural cosmetic filter is generic, i.e. meant to apply everywhere, it will be discarded by uBO. Examples: Good, because specific: `example.com##body > div:has-text(Sponsored)`. Bad, because generic: `##body > div:has-text(Sponsored)`. The element picker always prefixes the filter automatically with the hostname to ensure created cosmetic filters are specific.
    
2.  Efficient procedural cosmetic filters (or any cosmetic filters really) are the ones which result in the smallest set of nodes to visit. The element picker input field will display the number of elements matching the current filter. The element picker will only consider the entered text up to the first line break, while leaving the rest as is. You can use this feature to break up your filter to find out the size of the resultset of the first part(s) of your filter: the smallest resultset the most efficient is your cosmetic filter.
    
3.  Concatenating multiple procedural selectors in one filter is not supported. `example.com##p:has(img),div:has-text(advert)` will not work as expected ([#453](https://github.com/uBlockOrigin/uBlock-issues/issues/453)).
    
4.  [New cosmetic filter parser using CSSTree library](https://github.com/gorhill/uBlock/commit/a71b71e4c8a2037fc68970bc8912a76732edaade). The new parser no longer uses the browser DOM to validate that a cosmetic filter is valid or not, this is now done through a JS library, CSSTree. This means filter list authors will have to be more careful to ensure that a cosmetic filter is really valid, as there is no more guarantee that a cosmetic filter which works for a given browser/version will still work properly on another browser, or different version of the same browser. The new parser introduces breaking changes, there was no way to do otherwise. Some current procedural cosmetic filters will be shown as invalid with this change. Read the commit message for more details.
    
5.  You need to use quotes when an argument is causing issue in a procedural cosmetic operators. The way quoting is interpreted now is closer to how AdGuard documentation regarding the use of quotes, except that for uBO:
    
    -   Quotes are still optional if the argument doesn't trip the parser
    -   One can use either `"` or `'`
    -   When using quotes, instances of the same quote inside the argument must be escaped, i.e. `'this is a single quote: \''`, or `"this is a double quote: \""`, and instances of `\` must be escaped (as per AdGuard's documentation. A change from the [commit message](https://github.com/gorhill/uBlock/commit/a71b71e4c8a2037fc68970bc8912a76732edaade) is that escaping without quoting no longer does anything, so `##sidebar > div.side-content:has-text(Adoring Anya\'s twitter)` does not work anymore but `###sidebar > div.side-content:has-text(/Adoring Anya\'s twitter/)` would still work.

## Cosmetic filter operators

[](#cosmetic-filter-operators)

### List of available operators

[](#list-of-available-operators)

[:has(...)](#subjecthasarg), [:has-text(...)](#subjecthas-textneedle), [:matches-attr(...)](#subjectmatches-attrarg), [:matches-css(...)](#subjectmatches-cssarg), [:matches-css-before(...)](#subjectmatches-css-beforearg), [:matches-css-after(...)](#subjectmatches-css-afterarg), [:matches-media(...)](#subjectmatches-mediaarg), [:matches-path(...)](#subjectmatches-patharg), [:matches-prop(...)](#subjectmatches-proparg), [:min-text-length(n)](#subjectmin-text-lengthn), [:not(...)](#subjectnotarg), [:others(...)](#subjectothers), [:upward(...)](#subjectupwardarg), [:watch-attr(...)](#subjectwatch-attrarg), [:xpath(...)](#subjectxpatharg).

### `subject:has(arg)`

[](#subjecthasarg)

-   Description: Select element _subject_ if and only if evaluating _arg_ in the context of _subject_ returns one or more elements.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   _arg_: A valid plain CSS selector or procedural cosmetic filter, which is evaluated in the context of the _subject_ element.
-   Examples:
    -   `mobile.twitter.com##main [role="region"] > [role="grid"] > [role="rowgroup"] [role="row"]:has(div:last-of-type span:has-text(/^Promoted by/))`
    -   `strikeout.me##body > div:has(img[alt="AdBlock Alert"])`
    -   `yandex.ru##.serp-item:has(:scope .organic > .organic__subtitle > .label_color_yellow)` - `:scope` forces `.organic` to match inside `.serp-item` ([why `:scope` is needed](https://developer.mozilla.org/en-US/docs/Web/API/Document/querySelectorAll#javascript), [general `:scope` doc](https://developer.mozilla.org/en-US/docs/Web/CSS/:scope))
    -   `strdef.world##div[style]:has(> a[href="https://www.streamdefence.com/index.php"])` - `>` forces `a` to be direct descendant of `div[style]`

The `:has(arg)` operator is actually a planned pseudo-class in CSS4, but as of writing no browser supports it (was available in stable channel of Chromium 105+ based browsers after 30 August 2022). Instead of waiting for browser vendors to provide support, uBO provides support for `:has(arg)` as a procedural operator.

Updated: all major browsers now supports `:has()` natively, so native `:has()` should be more efficient than procedural `:has()` which needs Javascript code to function. When using `:has()` uBlock will automatically switch to native `:has()` if it's available in browser, if it's unavailable uBlock will fallback to a procedural `:has()`. Using `#?#` instead of `##` for a procedural cosmetic filter will prevent uBO from trying to convert the filter into a declarative (native) one.

See also:

-   [Procedural `:has` applies to multiple selectors; native `:has` only applies to the last one](https://github.com/uBlockOrigin/uBlock-issues/issues/2480)
-   [Nested procedural filtering into `:has()` did not work](https://github.com/uBlockOrigin/uBlock-issues/issues/2228)
-   [`:upward()` vs `:has()`](https://github.com/gorhill/ublock/wiki/Filter-Performance#upward-vs-has)

* * *

### `subject:has-text(needle)`

[](#subjecthas-textneedle)

-   Description: Select element _subject_ if the text _needle_ is found inside the element _subject_ or its children.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   _needle_: The literal text which must be found, or a literal regular expression. If using a literal regular expression, you can optionally use the `i` and/or `m` flags (version 1.15). It is possible to have `:has-text()` match the empty string by just quoting the empty string ([new in 1.45.3b10](https://github.com/gorhill/uBlock/commit/76d70102f069856bffac7cd27dc40500c3bb9563)): `##foo:has-text("")`
-   Examples:
    -   `example.com##body > div:last-of-type span:has-text(/^Promoted by/)`: starts with "Promoted by"
    -   `example.com##body > div:last-of-type span:has-text(/^Promoted by/i)`: starts with "Promoted by", ignore case
    -   `example.com##body > div:last-of-type span:has-text(Promoted by)`: contains "Promoted by" at any position

* * *

### `subject:matches-attr(arg)`

[](#subjectmatches-attrarg)

-   Description: Allows to select an element by its attributes, especially if they are randomized.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   _arg_: A declaration in the form `name="value"` or `"name"="value"`, where `name` is attribute name and `value` is attribute value (optional), `name` and `value` can be literal text or literal regular expression.

Introduced in uBO [1.45.3b10](https://github.com/gorhill/uBlock/commit/76d70102f069856bffac7cd27dc40500c3bb9563)

Solves [uBlockOrigin/uBlock-issues#2329 (comment)](https://github.com/uBlockOrigin/uBlock-issues/issues/2329#issue-1412857923) (randomly generated attributes).

The supported syntax is exactly as per AdGuard's documentation:

-   [https://kb.adguard.com/en/general/how-to-create-your-own-ad-filters#extended-css-matches-attr](https://kb.adguard.com/en/general/how-to-create-your-own-ad-filters#extended-css-matches-attr)

Though recommended, the quotes are not mandatory in uBO if the argument does not cause the parser to fail and if there are no ambiguities.

-   Examples:
    -   `k-rauta.fi##button:matches-attr(class="/[\w]{7}/")`
    -   `k-rauta.fi##button:matches-attr("class"="/[\w]{7}/")`

on `https://www.k-rauta.fi/tuote/valmistasoite-hole-in-1-250ml/6408070100905`

* * *

### `subject:matches-css(arg)`

[](#subjectmatches-cssarg)

-   Description: Select element _subject_ if and only if the result of evaluating _arg_ is one or more elements.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   _arg_: A declaration in the form `name: value`, where `name` is a valid CSS style property, and `value` is the expected value for that style property. `value` can be a literal text or literal regular expression:
    -   Literal text: the value will be matched _exactly_ against the property value as returned by the browser's [getComputedStyle](https://developer.mozilla.org/en-US/docs/Web/API/Window/getComputedStyle) method.
    -   Literal regular expression: you can optionally use the `i` and/or `m` flags (version 1.15).
-   Examples:
    -   `extratorrent.*##body > div[class]:matches-css(position: absolute)`
    -   `facet.wp.pl##div[class^="_"]:matches-css(background-image: /^url\("data:image/png;base64,/)`

* * *

### `subject:matches-css-before(arg)`

[](#subjectmatches-css-beforearg)

Same as `:matches-css(...)`, except that the style will be looked-up for the `:before` pseudo-class of the _subject_ element.

* * *

### `subject:matches-css-after(arg)`

[](#subjectmatches-css-afterarg)

Same as `:matches-css(...)` except that the style will be looked-up for `:after` pseudo-class of the _subject_ element.

* * *

### `subject:matches-media(arg)`

[](#subjectmatches-mediaarg)

-   Description: Allows limit cosmetic filter by media queries. If is possible will be converted to declarative stylesheet. Preferably should be used as last operator in a procedural cosmetic filter.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   _arg_: Any [supported](https://github.com/uBlockOrigin/uBlock-issues/issues/2185#issuecomment-1283956342) media type by CSSTree.
-   Examples:
    -   `example.com###target-1 > .target-2:matches-media((min-width: 800px))`
    -   `example.com###target-3 > .target-4:matches-media((min-width: 1920px) and (min-height: 930px)):style(color: red !important)`
    -   `github.com##pre:matches-media(print):style(white-space:pre-line !important;)` - disable print scrollbars in PDF

Introduced in uBO [1.43.1b8](https://github.com/gorhill/uBlock/commit/40c315a107257f5b1ac0c7fc92377934b23f6ed6), [feature request #2185](https://github.com/uBlockOrigin/uBlock-issues/issues/2185).

* * *

### `subject:matches-path(arg)`

[](#subjectmatches-patharg)

-   Description: Allows to further narrow the specificity according to the path and query of the current document location. Preferably should be used as first operator in a procedural cosmetic filter.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   _arg_: Plain text to be found at any position in the path + query, or a literal regex against which the path + query is tested.
-   Examples:
    -   `example.com##:matches-path(/shop) p`: Will hide all `p` elements when visiting `https://example.com/shop/stuff`, but not when visiting `https://example.com/` or any other page on `example.com` which has no instance of `/shop` in the path part of the URL. To only match the main page but not any of the subpages, use: `example.com##:matches-path(/^/＄/) p`

Introduced in uBO [1.37.3b13](https://github.com/gorhill/uBlock/commit/9dece3bd30bfa6ef35a6e58c37740adbd8482ab9), [feature request #1690](https://github.com/uBlockOrigin/uBlock-issues/issues/1690).

This is a all-or-nothing passthrough operator, which on/off behavior is dictated by whether the argument match the path of the current location. The argument can be either plain text to be found at any position in the path, or a literal Regular Expression against which the path is tested.

Whereas cosmetic filters can be made specific to whole domain, the new `:matches-path()` operator allows to further narrow the specificity according to the path of the current document location.

Typically this procedural operator is used as first operator in a procedural cosmetic filter, so as to ensure that no further matching work is performed should there be no match against the current path of the current document location.

* * *

### `subject:matches-prop(arg)`

[](#subjectmatches-proparg)

-   Description: Allows to select an element by a property name (or chain of properties), and optionally the property value.
-   Chainable: Yes
-   `subject`: Can be a plain CSS selector, or a procedural cosmetic filter.
-   `arg`: A declaration in the form `chain=value`, `chain="value"`, or `chain=/regex/`, where `chain` is a dot-separated string for the target property, and `value` is the optional property value to match. `value` can be literal text or literal regular expression. When no `value` is declared, the operator only tests for the presence of the target property.
-   Example:
    -   `example.org##div:matches-prop(imanad)`
    -   `example.org##img:matches-prop(naturalWidth=160)`

* * *

### `subject:min-text-length(n)`

[](#subjectmin-text-lengthn)

-   Description: DOM elements whose text length is greater than or equal to `n` will be selected.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   _n_: positive number, minimal text length of the subject DOM element.
-   Examples:
    -   Regular expression based filter: `quoka.de##^script:has-text(/[\w\W]{35000}/)` can be rewritten as: `quoka.de##^script:min-text-length(35000)`.

Introduced in uBO [1.20.1b2](https://github.com/gorhill/uBlock/commit/b428a25c3faf22630d3e9c542919c2d7ae10584f) As a result of internal discussion\[1\] with filter list maintainers. The original rationale for such procedural cosmetic operator is to be able to remove inline script elements according to a minimum text length using HTML filtering.

\[1\] [https://github.com/orgs/uBlockOrigin/teams/ublock-filters-volunteers/discussions/194?from\_comment=65](https://github.com/orgs/uBlockOrigin/teams/ublock-filters-volunteers/discussions/194?from_comment=65)

* * *

### `subject:not(arg)`

[](#subjectnotarg)

-   Description: Select element _subject_ if and only if the result of evaluating _arg_ is exactly zero elements.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   _arg_: A procedural cosmetic filter, which is evaluated in the context of the _subject_ element.

Introduced in uBO 1.17.5b9 to increase compatibility with AdGuard filter syntax.

Use to negate other procedural selectors. For example `:not(:has(.foo))` will match if there are no descendant matching `.foo`.

Note that if _arg_ is valid CSS selector, uBO will not consider the `:not` operator to be a procedural one, it will rather consider the operator as being part of a CSS selector. Thus this ensures compatibility with the existing [CSS `:not(...)` pseudo-class](https://developer.mozilla.org/en-US/docs/Web/CSS/:not).

* * *

### `subject:others()`

[](#subjectothers)

Experimental.

-   Description: Target all elements _outside_ than the currently selected set of elements.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   Examples:
    -   `twitter.com##:matches-path(/^/home/) [data-testid="primaryColumn"]:others()`
    -   `nature.com##:matches-path(/^/articles//) :is(.c-breadcrumbs,.c-article-main-column):others()`

Introduced in uBO [1.41.1b2](https://github.com/gorhill/uBlock/commit/152120bd9ec7a2ccea907e015fb195484ef7bc8e)

For any element feeding into `others()`, the resultset of the `others()` operator will include everything else except:

-   the descendants of a subject element
-   the ancestors of a subject element

The resultset will contains the siblings of a subject element _except_ when those siblings are either a descendant or ancestor of another subject element.

Though this operator is unlikely to be used in default lists, it opens the door to create specialized filter lists which purpose is some sort of "reader mode", where everything _else_ than a selected set of elements are hidden from view.

Related discussion:

-   [https://www.reddit.com/r/uBlockOrigin/comments/slyjzp/](https://www.reddit.com/r/uBlockOrigin/comments/slyjzp/)

* * *

### `subject:upward(arg)`

[](#subjectupwardarg)

-   Description: lookup the ancestor relative to the currently selected node.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   _arg_:
    -   Positive number >= 1 and < 256, distance from the currently selected node.
    -   A valid plain CSS selector.
-   Examples:
    -   Existing filter: `fastbay.org##.detLink:has-text(VPN):xpath(../../..)` can be rewritten as `fastbay.org##.detLink:has-text(VPN):upward(3)`
    -   `gorhill.github.io###pcf #a19 b:upward(2)`
    -   `gorhill.github.io###pcf #a20 b:upward(.fail)`

Introduced in uBO [1.25.3b0](https://github.com/gorhill/uBlock/commit/72bb70056843024b1a31fe1ab9c90bd4e8260ba2). Evolution of [`:nth-ancestor(n)`](#subjectnth-ancestorn) selector.

See also: [`:upward()` vs `:has()`](https://github.com/gorhill/ublock/wiki/Filter-Performance#upward-vs-has)

* * *

### `subject:watch-attr(arg)`

[](#subjectwatch-attrarg)

Experimental.

-   Description: Pass-through filter used to modify behavior of the procedural cosmetic filter engine by forcing re-evaluation when one or more attribute changes on the matching elements.
-   Chainable: Yes.
-   _subject_: Can be a plain CSS selector, or a procedural cosmetic filter.
-   _arg_: comma-separated list of attribute names. No argument means watch changes of any one attribute.

Example:

-   [Test case](https://ameshkov.github.io/web/watchattr.html) to detect `id` changes using filter `ameshkov.github.io###testdiv:watch-attr(id):has(p)`
-   `example.com##.j-mini-player[class]:watch-attr(class):remove-attr(class)`

Introduced in uBO [1.17.5rc3](https://github.com/gorhill/uBlock/commit/8a88e9d93174badd6855c0e782737158c9ccd6f8)

Solves [uBlockOrigin/uBlock-issues#341 (comment)](https://github.com/uBlockOrigin/uBlock-issues/issues/341#issuecomment-449764612) (overlay dialog used for two purposes, differ only by class name in child node).

By default hiding by procedural filters is reevaluated only when nodes in sub-tree are added or removed - uBO does not watch for attribute changes for performance reasons. This filter instructs uBO procedural filtering engine to watch for changes in specific attributes.

* * *

### `subject:xpath(arg)`

[](#subjectxpatharg)

-   Description: Create a new set of elements by evaluating an [XPath expression](https://developer.mozilla.org/en-US/docs/Web/XPath) using _subject_ as the context node (optional) and _arg_ as the expression.
-   Chainable: Yes.
-   _subject_: Optional. Can be a plain CSS selector, or a procedural cosmetic filter.
-   _arg_: A valid XPath expression.
-   Examples:
    -   `facebook.com##:xpath(//div[@id="stream_pagelet"]//div[starts-with(@id,"hyperfeed_story_id_")][.//h6//span/text()="People You May Know"])`

The `:xpath(...)` operator is different than other operators. Whereas all other operators are used to filter down a resultset of elements, the `:xpath(...)` operator can be used both to create a new resultset or filter down an existing one. For this reason, _subject_ is optional. For example, an `:xpath(...)` operator could be used to create a new resultset consisting of all ancestors elements of a subject element, something not otherwise possible with either plain CSS selectors or other procedural operators.