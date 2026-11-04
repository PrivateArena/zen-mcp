var deep = function (sel, root, out) { // find the gate: any checkbox-shaped node in the delete dialog
  out = out || []; const r = root || document;
  try { for (const n of r.querySelectorAll(sel)) out.push(n); } catch (e) {}
  for (const h of r.querySelectorAll("*")) if (h.shadowRoot && h.tagName !== "YT-I18N-MODULE") deep(sel, h.shadowRoot, out);
  return out;
};
var path = function (n) {
  const p = []; let c = n;
  while (c && c.nodeType === 1 && p.length < 8) {
    let s = c.tagName.toLowerCase() + (c.id ? "#" + c.id : "");
    p.unshift(s); c = c.parentElement || (c.getRootNode() && c.getRootNode().host) || null;
  }
  return p.join(" > ");
};
const boxes = deep("[role='checkbox'], input[type=checkbox], tp-yt-paper-checkbox, paper-checkbox, ytcp-checkbox");
const out = boxes.map(b => ({ tag: b.tagName.toLowerCase(), id: b.id || null, role: b.getAttribute("role"), checked: b.getAttribute("aria-checked") || b.checked, txt: (b.innerText || "").replace(/\s+/g, " ").slice(0, 70), path: path(b) }));
return { count: boxes.length, boxes: out.slice(0, 6), dialogTags: deep("tp-yt-paper-dialog, [role='dialog']").map(d => d.tagName.toLowerCase() + "#" + (d.id || "-")) };
