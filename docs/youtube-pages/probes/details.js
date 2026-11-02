const seg = (n) => n.tagName.toLowerCase() + (n.id ? "#" + n.id : "") +
  (n.getAttribute && n.getAttribute("data-test-id") ? "[t=" + n.getAttribute("data-test-id") + "]" : "");
const cssPath = (el) => { const p = []; let n = el;
  while (n && n.nodeType === 1 && p.length < 10) { const host = n.getRootNode() && n.getRootNode().host; p.unshift(seg(n)); n = host || n.parentElement; } return p.join(" > "); };
const out = [];
const sel = 'button,input,textarea,[role="button"],[role="radio"],[role="checkbox"],[role="combobox"],[contenteditable=true],[data-test-id],tp-yt-paper-radio-group,ytcp-select';
for (const el of document.querySelectorAll(sel)) {
  const r = el.getBoundingClientRect();
  if (!r.width || !r.height) continue;
  const path = cssPath(el);
  if (!path.includes("ytcp-video-metadata-editors") && !path.includes("ytcp-stepper") && !path.includes("ytcp-video-editor-main-body") && !path.includes("ytcp-button-renderer")) {
    if (!/ytcp|stepper|dialog/.test(path)) continue;
  }
  out.push({ tag: el.tagName.toLowerCase(), type: el.getAttribute("type") || undefined,
    role: el.getAttribute("role") || undefined, label: el.getAttribute("aria-label") || undefined,
    text: (el.innerText || el.value || "").trim().replace(/\s+/g," ").slice(0, 44),
    id: el.id || undefined, testId: el.getAttribute("data-test-id") || undefined,
    name: el.getAttribute("name") || undefined, ph: el.getAttribute("placeholder") || undefined,
    y: Math.round(r.y), path: path.slice(-150) });
}
const headers = [...document.querySelectorAll("h1,h2,[role=\"heading\"]")].map(h => (h.innerText||"").trim()).filter(t => t && t.length < 40);
return { url: location.href, count: out.length, headers: [...new Set(headers)].slice(0,10), elements: out.slice(0, 40) };
