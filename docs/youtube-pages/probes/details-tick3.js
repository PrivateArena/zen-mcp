var deep = function (sel, root, out) { // tick the understanding checkbox, then read the gate it controls
  out = out || []; const r = root || document;
  try { for (const n of r.querySelectorAll(sel)) out.push(n); } catch (e) {}
  for (const h of r.querySelectorAll("*")) if (h.shadowRoot && h.tagName !== "YT-I18N-MODULE") deep(sel, h.shadowRoot, out);
  return out;
};
const btns = () => deep("tp-yt-paper-dialog#dialog tp-yt-paper-button, tp-yt-paper-dialog#dialog button")
  .map(b => (b.innerText || "").trim()).filter(Boolean).map(t => t + "=" + (deep("tp-yt-paper-dialog#dialog tp-yt-paper-button, tp-yt-paper-dialog#dialog button").find(x => (x.innerText || "").trim() === t) || {}).disabled);
const state = {};
const rootBox = deep("#confirm-checkbox")[0];
const input = deep("#confirm-checkbox input")[0] || deep("#checkbox-container input")[0];
const target = rootBox || input;
if (!target) return { ok: false, note: "no confirm checkbox" };
state.before = btns().join(",");
target.focus && target.focus();
for (const t of ["pointerdown", "mousedown", "pointerup", "mouseup", "click"]) target.dispatchEvent(new MouseEvent(t, { bubbles: true, cancelable: true, view: window }));
if (input && input.click) input.click();
await new Promise(r => setTimeout(r, 900));
state.after = btns().join(",");
state.checked = (deep("#confirm-checkbox")[0] || {}).getAttribute ? deep("#confirm-checkbox")[0].getAttribute("aria-checked") : null;
state.inputChecked = !!input && input.checked;
state.label = ((deep("#dialog-content-confirm-checkboxes")[0] || {}).innerText || "").replace(/\s+/g, " ").slice(0, 110);
return { ok: true, state };
