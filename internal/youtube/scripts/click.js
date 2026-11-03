// DOM-level click: the fallback when a synthetic MouseEvent is swallowed, and the only channel that
// reaches an element with no stable selector and no OS focus available — a cell of a virtual scrolling
// calendar (see docs/youtube-pages/upload-wizard.md, Step 8). Resolves args.l the same way lookup.js
// does, so a Finding's recorded css path is directly clickable.
const L = args.l || {};
const q = (sel, ctx) => { try { return Array.from((ctx || document).querySelectorAll(sel)); } catch (e) { return []; } };
function deep(sel) {
  const hits = [];
  const walk = (root) => {
    for (const n of q(sel, root)) hits.push(n);
    for (const h of q("*", root)) if (h.shadowRoot) walk(h.shadowRoot);
  };
  walk(document);
  return hits;
}

const scope = L.root ? deep(L.root)[0] || document : document;
let nodes = L.css ? (L.root ? q(L.css, scope) : deep(L.css)) : [];
if (!nodes.length && L.text) {
  nodes = (L.root ? deepIn("*", scope) : deep("*")).filter((n) => {
    const own = (n.innerText || n.textContent || "").trim().replace(/\s+/g, " ");
    if (n.children.length && own.length > 90) return false;
    return L.exact ? own === L.text : own.toLowerCase().includes(String(L.text).toLowerCase());
  });
}
if (L.nth) nodes = nodes.slice(L.nth, L.nth + 1);

const node = nodes[0];
if (!node) return { clicked: false, count: 0, text: "" };
node.click();
return {
  clicked: true,
  count: nodes.length,
  text: (node.innerText || node.textContent || "").trim().replace(/\s+/g, " ").slice(0, 80),
};
