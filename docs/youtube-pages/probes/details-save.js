var deep = function (sel, root, out) { // the Save control, its state, and what the page says after it is pressed
  out = out || []; const r = root || document;
  try { for (const n of r.querySelectorAll(sel)) out.push(n); } catch (e) {}
  for (const h of r.querySelectorAll("*")) if (h.shadowRoot && h.tagName !== "YT-I18N-MODULE") deep(sel, h.shadowRoot, out);
  return out;
};
var path = function (n) {
  const p = []; let c = n;
  while (c && c.nodeType === 1 && p.length < 6) {
    let s = c.tagName.toLowerCase() + (c.id ? "#" + c.id : "");
    p.unshift(s); c = c.parentElement || (c.getRootNode() && c.getRootNode().host) || null;
  }
  return p.join(" > ");
};
const saveBtns = () => deep("button, tp-yt-paper-button").filter(b => /^save$/i.test((b.innerText || "").trim()) || /^save$/i.test(b.getAttribute("aria-label") || ""));
const b0 = saveBtns()[0];
const before = { count: saveBtns().length, disabled: b0 && b0.disabled, path: b0 && path(b0), aria: b0 && b0.getAttribute("aria-label") };
if (!b0) return { ok: false, before, note: "no Save button" };
b0.focus && b0.focus(); b0.click();
let dialog = null, toast = null;
for (let i = 0; i < 24; i++) {
  await new Promise(r => setTimeout(r, 1000));
  const d = deep("tp-yt-paper-dialog, [role='dialog']").filter(x => x.offsetWidth > 0);
  if (d.length) { dialog = (d[0].innerText || "").replace(/\s+/g, " ").slice(0, 260); break; }
  const t = deep("[class*='snackbar'], [class*='toast'], ytcp-notification").filter(x => x.offsetWidth > 0);
  if (t.length) { toast = (t[0].innerText || "").replace(/\s+/g, " ").slice(0, 120); break; }
}
await new Promise(r => setTimeout(r, 1200));
const b1 = saveBtns()[0];
return { ok: true, before, dialog, toast, after: { disabled: b1 && b1.disabled, text: b1 && (b1.innerText || "").trim() }, header: ((deep("ytcp-video-metadata-header")[0] || {}).innerText || "").replace(/\s+/g, " ").slice(0, 100) };
