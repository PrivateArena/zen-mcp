var deep = function (sel, root, out) { // read the delete gate without touching it
  out = out || []; const r = root || document;
  try { for (const n of r.querySelectorAll(sel)) out.push(n); } catch (e) {}
  for (const h of r.querySelectorAll("*")) if (h.shadowRoot && h.tagName !== "YT-I18N-MODULE") deep(sel, h.shadowRoot, out);
  return out;
};
const cb = deep("#confirm-checkbox")[0], inp = deep("#confirm-checkbox input")[0];
const btn = deep("tp-yt-paper-button, button").filter(b => (b.innerText || "").trim() === "Delete forever")[0];
return { ariaChecked: cb && cb.getAttribute("aria-checked"), inputChecked: inp && inp.checked, nativeChecked: cb && cb.checked, deleteForeverDisabled: btn && btn.disabled };
