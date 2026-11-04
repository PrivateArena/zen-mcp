var args = { css: "#overflow-menu-button" }; // press one control by its frozen address, then report what appeared
function deep(sel, root, out) {
  out = out || []; const r = root || document;
  try { for (const n of r.querySelectorAll(sel)) out.push(n); } catch (e) {}
  for (const h of r.querySelectorAll("*")) if (h.shadowRoot && h.tagName !== "YT-I18N-MODULE") deep(sel, h.shadowRoot, out);
  return out;
}
const el = deep(args.css)[0];
if (!el) return { clicked: false, note: "no element for " + args.css };
el.focus && el.focus();
for (const t of ["pointerdown", "mousedown", "pointerup", "mouseup", "click"]) {
  el.dispatchEvent(new MouseEvent(t, { bubbles: true, cancelable: true, view: window }));
}
await new Promise(r => setTimeout(r, 1200));
const items = deep("tp-yt-paper-item, [role='menuitem'], ytcp-menu-item, paper-item").map(n => ({ tag: n.tagName.toLowerCase(), txt: (n.innerText || "").replace(/\s+/g, " ").trim().slice(0, 32), aria: n.getAttribute("aria-label"), disabled: n.disabled === true }));
return { clicked: true, aria: el.getAttribute("aria-label"), items: items.filter(i => i.txt).slice(0, 14) };
