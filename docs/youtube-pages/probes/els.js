const seg = (n) => n.tagName.toLowerCase() + (n.id ? "#" + n.id : "") +
  (n.getAttribute && n.getAttribute("data-test-id") ? "[t=" + n.getAttribute("data-test-id") + "]" : "");
const cssPath = (el) => { const p = []; let n = el;
  while (n && n.nodeType === 1 && p.length < 8) { const host = n.getRootNode() && n.getRootNode().host; p.unshift(seg(n)); n = host || n.parentElement; } return p.join(" > "); };
const out = [];
for (const el of document.querySelectorAll("button,a[href],input,textarea,[role],[contenteditable=true],[data-test-id]")) {
  const r = el.getBoundingClientRect();
  if (!r.width || !r.height) continue;
  out.push({ tag: el.tagName.toLowerCase(), type: el.getAttribute("type") || undefined,
    role: el.getAttribute("role") || undefined, label: el.getAttribute("aria-label") || undefined,
    text: (el.innerText || el.value || "").trim().replace(/\s+/g," ").slice(0, 50),
    id: el.id || undefined, testId: el.getAttribute("data-test-id") || undefined, path: cssPath(el) });
}
return { count: out.length, insideDialog: out.filter(e => (e.path||"").includes("ytcp-uploads-dialog")), outside: out.filter(e => !(e.path||"").includes("ytcp-uploads-dialog")).length };
