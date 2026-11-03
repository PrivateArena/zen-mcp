// Drive the page down so a lazily built section comes into existence. This is the only lever that works
// *before* the section exists: the bridge's `scroll` action needs an element to aim at, and a flow must not
// probe the structure of a section the page has not built yet (docs/youtube-pages/watch.md trap 6,
// watch-run.md §7 — the section is absent entirely for about a second after navigate, and the rows under it
// stay absent until it has been scrolled into view).
//
// It moves the viewport and nothing else: no element is pressed, no page state is written.
//
// args.selector names the element the flow is waiting for, exactly as the flow names it (the flow's own
// frozen locator, never a selector invented here). When the page has not built that element yet, the only
// thing left to drive is the bottom of the document — so both are done, and the reply says what happened.
//
// The bottom is taken from the documentElement with the body as a fallback: on the watch page
// document.body.scrollHeight measures 0 (probed live on watch?app=desktop&v=YSNFGgh0aAs), so a scroll written
// against the body alone would be a silent no-op.
const sel = args.selector || "";
const before = window.scrollY;
let section = null;
if (sel) {
  try { section = document.querySelector(sel); } catch (e) { section = null; }
}
const bottom = Math.max(
  (document.documentElement && document.documentElement.scrollHeight) || 0,
  (document.body && document.body.scrollHeight) || 0
);
window.scrollTo(0, bottom);
if (section) section.scrollIntoView();
return { before, after: window.scrollY, bottom, hasSection: !!section };
