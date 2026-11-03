// R4 deep-query probe: resolve args.l (a Locator) through every shadow root and report every channel
// for the hit — CSS path, state, geometry — in one response. Read-only: this is the probe, not the action.
const L = args.l || {};
const maxText = args.maxText || 120;
const esc = (s) => (window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/[^a-zA-Z0-9_-]/g, (c) => "\\" + c));
const q = (sel, ctx) => { try { return Array.from((ctx || document).querySelectorAll(sel)); } catch (e) { return null; } };

function deep(sel) {
  const hits = [];
  let blocked = false;
  const walk = (root) => {
    if (hits.length > 300) return;
    const found = q(sel, root);
    if (found === null) { blocked = true; return; }
    for (const n of found) hits.push(n);
    const hosts = q("*", root) || [];
    for (const h of hosts) if (h.shadowRoot) walk(h.shadowRoot);
  };
  walk(document);
  return { hits, blocked };
}

// A root that hosts a shadow tree is queried inside it: Studio's own controls are frequently the host's
// shadow children, which no light-DOM query can reach.
function enter(node) {
  if (!node) return document;
  return node.shadowRoot || (node.assignedSlot && node.assignedRootNode ? node.assignedRootNode : node);
}
function deepIn(sel, root) {
  const hits = [];
  const walk = (r) => { for (const n of q(sel, r) || []) hits.push(n); for (const h of q("*", r) || []) if (h.shadowRoot) walk(h.shadowRoot); };
  walk(root);
  return hits;
}

const scope = L.root ? enter(deep(L.root).hits[0]) : document;
let nodes = [];
if (L.css) nodes = L.root ? (q(L.css, scope) || []) : deep(L.css).hits;

// A text/role lookup honours args.l.root, which is how a duplicated label ("9", "Schedule") or a control
// inside one widget is disambiguated.
const CANDIDATES = "button,a,[role],[aria-label],label,span,p,div,ytcp-button,tp-yt-paper-radio-button";
if (!nodes.length && (L.text || L.role || L.name)) {
  nodes = (L.root ? deepIn(CANDIDATES, scope) : deep(CANDIDATES).hits).filter((n) => {
    const own = (n.innerText || n.textContent || "").trim().replace(/\s+/g, " ");
    if (n.children.length && own.length > 90) return false;
    if (L.text) {
      const got = own.toLowerCase(), want = String(L.text).toLowerCase();
      if (!(L.exact ? got === want : got.includes(want))) return false;
    }
    if (L.role) {
      const role = n.getAttribute("role") || n.tagName.toLowerCase();
      if (role !== L.role) return false;
    }
    if (L.name && (n.getAttribute("aria-label") || "") !== L.name) return false;
    return true;
  });
}

if (!nodes.length && (L.x || L.y)) nodes = [document.elementFromPoint(L.x, L.y)].filter(Boolean);
if (L.nth) nodes = nodes.slice(L.nth, L.nth + 1);

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

function state(node) {
  const r = node.getBoundingClientRect(), cs = getComputedStyle(node);
  const st = { exists: true, visible: r.width > 0 && r.height > 0 && cs.visibility !== "hidden" && cs.opacity !== "0", disabled: false };
  if ("disabled" in node) st.disabled = !!node.disabled;
  if (node.getAttribute && node.getAttribute("aria-disabled") === "true") st.disabled = true;
  if (node.getAttribute && node.getAttribute("aria-checked")) st.checked = node.getAttribute("aria-checked") === "true";
  else if ("checked" in node && (node.type === "radio" || node.type === "checkbox")) st.checked = !!node.checked;
  if (node.getAttribute && node.getAttribute("aria-selected")) st.selected = node.getAttribute("aria-selected") === "true";
  const values = [];
  if (node.value) values.push(String(node.value).slice(0, 80));
  const lab = node.labels && node.labels[0];
  if (lab) values.push((lab.innerText || lab.textContent || "").trim().replace(/\s+/g, " ").slice(0, 80));
  if (values.length) st.values = values;
  return st;
}

const node = nodes[0];
if (!node) return { ok: false, reason: "no element for " + JSON.stringify(L), count: 0 };

const rect = node.getBoundingClientRect();
return {
  ok: true,
  count: nodes.length,
  tag: node.tagName.toLowerCase(),
  id: node.id || undefined,
  role: node.getAttribute("role") || undefined,
  name: node.getAttribute("aria-label") || undefined,
  css: node.id ? "#" + esc(node.id) : path(node),
  text: (node.innerText || node.textContent || "").trim().replace(/\s+/g, " ").slice(0, maxText),
  rect: [Math.round(rect.x), Math.round(rect.y), Math.round(rect.width), Math.round(rect.height)],
  // Reported beside the label so a flow gating on "the page accepted my write" (watch-run.md §4: the submit
  // button flipping disabled true → false) reads the same fact the record shows under --explain.
  disabled: !!node.disabled || (node.getAttribute && node.getAttribute("aria-disabled") === "true"),
  state: state(node),
  inShadow: !!(node.getRootNode && node.getRootNode().host),
};
