var args = { text: "Delete", depth: 1 }; // walk the delete confirmation chain one step and report the dialog it reveals
function deep(sel, root, out) {
  out = out || []; const r = root || document;
  try { for (const n of r.querySelectorAll(sel)) out.push(n); } catch (e) {}
  for (const h of r.querySelectorAll("*")) if (h.shadowRoot && h.tagName !== "YT-I18N-MODULE") deep(sel, h.shadowRoot, out);
  return out;
}
const press = (n) => { n.focus && n.focus(); for (const t of ["pointerdown", "mousedown", "pointerup", "mouseup", "click"]) n.dispatchEvent(new MouseEvent(t, { bubbles: true, cancelable: true, view: window })); };
const cands = deep("tp-yt-paper-item, paper-item, [role='menuitem'], tp-yt-paper-button, button").filter(n => (n.innerText || "").trim() === args.text);
if (!cands.length) return { clicked: false, note: "nothing labelled " + args.text };
const el = cands[cands.length - 1];
press(el);
await new Promise(r => setTimeout(r, 1500));
const dlg = deep("tp-yt-paper-dialog, [role='dialog']").filter(d => d.offsetWidth > 0);
const shot = dlg.map(d => ({
  id: d.id || null,
  text: (d.innerText || "").replace(/\s+/g, " ").slice(0, 300),
  buttons: Array.from(d.querySelectorAll("tp-yt-paper-button, button")).map(b => ({ txt: (b.innerText || "").trim().slice(0, 40), aria: b.getAttribute("aria-label"), id: b.id || null, disabled: b.disabled === true })),
  checks: Array.from(d.querySelectorAll("tp-yt-paper-checkbox, [role='checkbox']")).map(c => ({ aria: c.getAttribute("aria-label"), checked: c.getAttribute("aria-checked"), txt: (c.innerText || "").replace(/\s+/g, " ").slice(0, 120) })),
}));
return { clicked: true, label: el.innerText.trim(), dialogs: shot };
