// Text entry with proof (R5). args.l locates the field, args.text is what must end up in it, and the
// read-back is returned so the caller can trust or reject the write.
//
// Two field kinds exist on YouTube and they need different handling:
//   - a contenteditable rich field (Studio title/description, the comment box) ignores plain value writes
//     and the bridge's own type action; it takes a selection plus document.execCommand("insertText").
//     The selection is replaced with selectNodeContents — selectAllChildren + collapseToEnd APPENDS.
//   - a real <input>/<textarea> (search, the time field) takes the native value setter plus input/change.
// Writing a date or time input this way displays the text but does NOT commit Studio's model; a flow that
// needs a committed date clicks the calendar instead. See docs/youtube-pages/upload-wizard.md.
const L = args.l || {};
const want = args.text == null ? "" : String(args.text);
const preserve = !!args.preserve; // append instead of replacing the existing content

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
const node = nodes[L.nth || 0];
if (!node) return { ok: false, err: "element vanished before typing", text: "" };

node.focus();
let method = "";
if (node.isContentEditable) {
  const sel = window.getSelection();
  sel.removeAllRanges();
  const range = document.createRange();
  if (preserve) {
    range.selectNodeContents(node);
    range.collapse(false);
  } else {
    range.selectNodeContents(node);
  }
  sel.addRange(range);
  method = "insertText";
  const done = document.execCommand("insertText", false, want);
  if (!done) {
    node.textContent = (preserve ? node.textContent : "") + want;
    node.dispatchEvent(new InputEvent("input", { bubbles: true, data: want }));
    method = "textContent";
  }
} else {
  const proto = node.tagName === "TEXTAREA" ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value").set;
  if (preserve) {
    setter.call(node, String(node.value || "") + want);
  } else {
    node.setSelectionRange && node.setSelectionRange(0, String(node.value || "").length);
    setter.call(node, want);
  }
  node.dispatchEvent(new Event("input", { bubbles: true }));
  node.dispatchEvent(new Event("change", { bubbles: true }));
  method = "native-setter";
}

const readBack = node.isContentEditable
  ? (node.innerText || node.textContent || "").replace(/\s+/g, " ").trim()
  : String(node.value);
const norm = (s) => String(s).replace(/\s+/g, " ").trim();
return { ok: norm(readBack) === norm(want), method, text: readBack.slice(0, 400), err: norm(readBack) === norm(want) ? "" : "read-back differs from the intended text" };
