// Read-only state probe for the Studio details panel's overflow menu and dialogs
// (docs/youtube-pages/studio-details.md §3, §5.5). It answers one question about every address it is handed:
// is the page actually SHOWING this node? §4 trap 4 is the reason the question is asked here rather than in
// Go — after a cancel, a heading reading "Permanently delete this video?" was still reachable by a text
// search, and a delete flow that believes a closed dialog is open is worse than the reverse. The doc's own
// rule (§5.5, "read dialogs and menus with offsetWidth > 0") is what `shown` implements, literally.
//
// Every address arrives as args, frozen in pages/details.go from the probe record: this program holds none of
// its own, and it presses nothing. `path` is the same shape lookup.js reports as Finding.CSS, so a flow can
// aim a later press at exactly the node this read proved — which is how the menu item and the dialog's
// buttons get addressed at all, since §3 records them by their exact text alone.
const A = args || {};
const esc = (s) => (window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/[^a-zA-Z0-9_-]/g, (c) => "\\" + c));
const q = (sel, ctx) => { try { return Array.from((ctx || document).querySelectorAll(sel)); } catch (e) { return []; } };

function deep(sel, root, hits) {
  hits = hits || [];
  const r = root || document;
  for (const n of q(sel, r)) hits.push(n);
  for (const h of q("*", r)) if (h.shadowRoot) deep(sel, h.shadowRoot, hits);
  return hits;
}

// under is the same walk started from a node the page already proved, so a control recorded by text alone can
// be found under its proven container without inventing an address for it.
function under(root, sel, hits) {
  hits = hits || [];
  if (!root) return hits;
  for (const n of q(sel, root)) hits.push(n);
  for (const h of q("*", root)) if (h.shadowRoot) under(h.shadowRoot, sel, hits);
  return hits;
}

const shown = (n) => !!(n && (n.offsetWidth > 0 || n.offsetHeight > 0));

function path(node) {
  const seg = [];
  for (let n = node; n && n.nodeType === 1 && seg.length < 8; ) {
    let s = n.tagName.toLowerCase();
    if (n.id) { seg.unshift(s + "#" + esc(n.id)); break; }
    const cls = (typeof n.className === "string" ? n.className : "").trim().split(/\s+/).filter(Boolean).slice(0, 1).map(esc).join(".");
    const sib = n.parentElement ? Array.from(n.parentElement.children).filter((c) => c.tagName === n.tagName) : [];
    s += (cls ? "." + cls : "") + (sib.length > 1 ? ":nth-of-type(" + (sib.indexOf(n) + 1) + ")" : "");
    seg.unshift(s);
    n = n.parentElement || (n.getRootNode && n.getRootNode().host) || null;
  }
  return seg.join(" > ");
}

// report is the flow's whole vocabulary about one node: its own words, whether the page is showing it, the
// state a press is gated on, and the address it was found at.
function report(n, cap) {
  return {
    tag: n.tagName.toLowerCase(),
    text: (n.innerText || n.textContent || "").toString().trim().replace(/\s+/g, " ").slice(0, cap || 60),
    path: path(n),
    shown: shown(n),
    disabled: !!n.disabled || (n.getAttribute && n.getAttribute("aria-disabled") === "true"),
    checked: !!(n.getAttribute && n.getAttribute("aria-checked") === "true") || !!n.checked,
  };
}

// The interactive shapes a control can have. The two custom tags are the ones this page's own recorded
// addresses are built out of (ytcp-button in §2, the tp-yt-/ytcp- family throughout §3); the rest is the
// generic set docs/youtube-pages/README.md §R4 probes with. No selector unique to a control that was never
// probed appears here.
const CONTROLS = "button,a[href],[role],ytcp-button";

const out = { url: location.href, title: document.title, dialogs: [], items: [], buttons: [], notices: [], checkbox: null };

const dialogs = A.dialog ? deep(A.dialog) : [];
for (const n of dialogs.slice(0, 8)) out.dialogs.push(report(n, 400));

// Only an open dialog's controls are listed: §3 records the dialog's buttons by text, and a closed dialog
// keeps them in the DOM, so the set read from a hidden dialog is worth nothing to a flow.
for (const dlg of dialogs.filter(shown)) {
  for (const n of under(dlg, CONTROLS)) {
    const r = report(n, 60);
    if (r.text) out.buttons.push(r);
    if (out.buttons.length >= 24) break;
  }
}

if (A.menuItem) for (const n of deep(A.menuItem).slice(0, 40)) out.items.push(report(n, 40));
if (A.checkbox) { const cb = deep(A.checkbox)[0]; if (cb) out.checkbox = report(cb, 120); }
for (const sel of A.notices || []) {
  for (const n of deep(sel).slice(0, 8)) out.notices.push(report(n, 120));
}
return out;
