const visit = (root, out, depth = 0) => {
  for (const el of root.querySelectorAll("button,a[href],input,textarea,[role],[contenteditable=true],[data-test-id]")) {
    const r = el.getBoundingClientRect();
    if (r.width && r.height) out.push({
      tag: el.tagName.toLowerCase(), type: el.getAttribute("type") || undefined,
      role: el.getAttribute("role") || undefined,
      label: el.getAttribute("aria-label") || undefined,
      text: (el.innerText || el.value || "").trim().slice(0, 60),
      id: el.id || undefined, testId: el.getAttribute("data-test-id") || undefined,
      path: cssPath(el)
    });
  }
  if (depth > 12) return out;
  for (const el of root.querySelectorAll("*")) if (el.shadowRoot) visit(el.shadowRoot, out, depth + 1);
  return out;
};
const cssPath = (el) => {
  const p = []; let n = el;
  while (n && n.nodeType === 1 && p.length < 8) {
    const host = n.getRootNode() && n.getRootNode().host;
    const seg = n.tagName.toLowerCase() + (n.id ? "#" + n.id : "") +
      (n.getAttribute && n.getAttribute("data-test-id") ? "[t=" + n.getAttribute("data-test-id") + "]" : "");
    p.unshift(seg); n = host || n.parentElement;
  }
  return p.join(" > ");
};
const files = [...document.querySelectorAll("input[type=file]")].map(el => {
  const r = el.getBoundingClientRect();
  return { accept: el.accept, id: el.id || undefined, cls: (el.className||"").toString().slice(0,60),
           multiple: el.multiple, rect: [Math.round(r.x), Math.round(r.y), Math.round(r.width), Math.round(r.height)],
           path: cssPath(el) };
});
const body = (document.body && document.body.innerText || "").slice(0, 600);
return { url: location.href, title: document.title, ready: document.readyState,
         fileInputs: files, lightButtons: visit(document, []).length, dialogText: body };
