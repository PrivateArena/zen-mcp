var deep = function (sel, root, out) { // persisted state: the fields, the visibility card, the trailer
  out = out || []; const r = root || document;
  try { for (const n of r.querySelectorAll(sel)) out.push(n); } catch (e) {}
  for (const h of r.querySelectorAll("*")) if (h.shadowRoot && h.tagName !== "YT-I18N-MODULE") deep(sel, h.shadowRoot, out);
  return out;
};
const card = deep("ytcp-video-preview, [class*='visibility']").filter(n => /private|public|unlisted|schedul/i.test(n.innerText || "")).map(n => ({ cls: (typeof n.className === "string" ? n.className.slice(0, 40) : null), txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 110) }));
const banner = deep("ytcp-details-banner, [class*='details-banner'], h1, [role='heading']").map(n => (n.innerText || "").replace(/\s+/g, " ").trim()).filter(t => t && t.length < 80).slice(0, 5);
return {
  title: ((deep("div#title-textbox-container #textbox")[0] || {}).innerText || "").trim(),
  desc: ((deep("#description-container #textbox")[0] || {}).innerText || "").replace(/\s+/g, " ").trim().slice(0, 90),
  saveDisabled: ((deep("ytcp-button#save button")[0] || {}).disabled) ?? null,
  snackbar: (deep("[class*='snackbar'], ytcp-notification").filter(n => n.offsetWidth > 0).map(n => (n.innerText || "").replace(/\s+/g, " ").slice(0, 60))[0]) || null,
  visibility: card.slice(0, 3), banner,
};
