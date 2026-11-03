// The watch page comment read (docs/youtube-pages/watch.md §1, tree in watch-run.md §1). One call answers:
// how many rows the page has rendered, what its own header claims, and every row — author, text, relative
// and absolute time, the `lc` comment id, whether it is nested under #replies, and the row's page-proven
// address.
//
// Read-only. The id is *in the page*: `#published-time-text a`'s href carries `&lc=<id>`, which survives a
// reload, so a later comment_id can address the same row with no network call (watch.md §1).
const video = args.video || "";
const clean = (s) => (s === null || s === undefined ? "" : String(s).replace(/\s+/g, " ").trim());
const q = (sel, ctx) => { try { return Array.from((ctx || document).querySelectorAll(sel)); } catch (e) { return []; } };

// The rows are light DOM today (watch.md §1: "no shadow walking needed to read a row"), but the section is a
// polymer tree that can move a child into a shadow root without warning, so the walk pierces shadow roots the
// way lookup.js's deep() does rather than assuming the shape holds.
function deep(sel, root) {
  const hits = [];
  const walk = (r) => {
    for (const n of q(sel, r)) hits.push(n);
    for (const h of q("*", r)) if (h.shadowRoot) walk(h.shadowRoot);
  };
  walk(root || document);
  return hits;
}
const one = (sel, ctx) => { const h = deep(sel, ctx); return h.length ? h[0] : null; };

// The anchor is asked for on its own, never through "#published-time-text a, #published-time-text": that
// returns the span in document order and the lc id with it (watch.md trap 1).
function stamp(node) {
  const a = one("#published-time-text a", node) || one('a[href*="lc="]', node);
  if (!a) return { id: "", when: "", absolute: "" };
  let id = "";
  try {
    const m = /[?&]lc=([^&]+)/.exec(a.getAttribute("href") || "");
    if (m) id = decodeURIComponent(m[1]);
  } catch (e) {
    id = ""; // a malformed href is a row with no addressable id, not a failed read
  }
  return { id, when: clean(a.innerText || a.textContent), absolute: clean(a.getAttribute("title")) };
}

function ancestorID(node, id) {
  for (let n = node; n; n = n.parentElement || (n.getRootNode && n.getRootNode().host) || null) {
    if (n.nodeType === 1 && n.id === id) return true;
    if (n === document.body) return false;
  }
  return false;
}

// pathOf derives a row's address from its real ancestors, the way lookup.js reports the page-proven path of
// a finding — it is never a guessed selector. The reply flow needs it because both boxes can be open at once
// (watch.md §3), so "the first #commentbox" is ambiguous and every lookup has to be scoped to this row.
// An empty result means the chain is not unique in the document, and the flow refuses rather than aim at a
// row it cannot name.
function pathOf(node) {
  const seg = [];
  for (let n = node; n && n.nodeType === 1 && seg.length < 12; n = n.parentElement) {
    let s = n.tagName.toLowerCase();
    if (n.id) s += "#" + (window.CSS && CSS.escape ? CSS.escape(n.id) : n.id);
    const kids = n.parentElement ? Array.from(n.parentElement.children).filter((c) => c.tagName === n.tagName) : [];
    if (kids.length > 1) s += ":nth-of-type(" + (kids.indexOf(n) + 1) + ")";
    seg.unshift(s);
    if (n.id === "comments" || n === document.body) break;
  }
  const path = seg.join(" > ");
  let hits = 0;
  try { hits = document.querySelectorAll(path).length; } catch (e) { return ""; }
  return hits === 1 ? path : "";
}

// headerCount is reported, never trusted: watch.md §6 trap 1 caught it still saying "1 Comment" after a
// second post, so a flow counts rows and treats this as the page's own (stale) opinion.
const header = one("ytd-comments-header-renderer");
const headerText = header ? clean(header.innerText || header.textContent) : "";
let headerCount = -1;
const hm = /(\d[\d,]*)\s+Comments?\b/.exec(headerText);
if (hm) {
  const n = Number(hm[1].replace(/,/g, ""));
  if (!isNaN(n)) headerCount = n;
}

const rows = deep("ytd-comment-view-model").map((node) => {
  const t = stamp(node);
  const author = one("#author-text", node);
  const text = one("#content-text", node);
  return {
    id: t.id,
    author: author ? clean(author.innerText || author.textContent) : "",
    text: text ? clean(text.innerText || text.textContent) : "",
    when: t.when,
    absolute: t.absolute,
    nested: ancestorID(node, "replies"),
    // A reply's id is <parent>.<suffix> (watch-run.md §5); the tool parses it as opaque.
    isReply: t.id.indexOf(".") >= 0,
    path: pathOf(node),
  };
});

// The id the page is really showing, so a flow can catch a redirect to another video (or to a login wall)
// instead of reading the wrong thread.
const shown = (/[?&]v=([A-Za-z0-9_-]{6,})/.exec(location.search) || [])[1] || "";

return { video: shown, requested: video, count: rows.length, headerCount, headerText, rows };
