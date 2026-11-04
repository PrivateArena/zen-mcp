var deep = function (sel, root, out) { // freeze the Studio details panel: fields, sections, controls
  out = out || []; const r = root || document;
  try { for (const n of r.querySelectorAll(sel)) out.push(n); } catch (e) {}
  for (const host of r.querySelectorAll("*")) if (host.shadowRoot && host.tagName !== "YT-I18N-MODULE") deep(sel, host.shadowRoot, out);
  return out;
};
var path = function (n) {
  const p = []; let c = n;
  while (c && c.nodeType === 1 && p.length < 7) {
    let s = c.tagName.toLowerCase() + (c.id ? "#" + c.id : "");
    p.unshift(s); c = c.parentElement || (c.getRootNode() && c.getRootNode().host) || null;
  }
  return p.join(" > ");
};
const fields = deep("input, textarea, [contenteditable], tp-yt-paper-input, ytcp-text-edit-dialog, tp-yt-paper-radio-group, tp-yt-paper-toggle-button, tp-yt-paper-button, button");
const rows = fields.map(n => ({
  tag: n.tagName.toLowerCase(), id: n.id || null, type: n.getAttribute("type"),
  aria: n.getAttribute("aria-label"), label: n.getAttribute("aria-labelledby"),
  txt: (n.innerText || n.value || "").toString().replace(/\s+/g, " ").slice(0, 46),
  editable: n.getAttribute("contenteditable"), disabled: n.disabled === true, path: path(n),
}));
const cards = deep("ytcp-form-input-container, ytcp-formRenderer, section[role], ytcp-autosave-status, #save-msg").map(n => ({ path: path(n), txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 40) }));
return {
  url: location.href, title: document.title,
  sections: deep("ytcp-details-banner, ytcp-video-title, #save-msg").map(n => (n.innerText || "").replace(/\s+/g, " ").slice(0, 60)),
  cards: cards.slice(0, 6),
  fields: rows.filter(r => /title|desc|visib|thumbnail|playlist|kids|audience|save|edit|more|delete|tri/i.test((r.id || "") + " " + (r.aria || "") + " " + r.txt)).slice(0, 26),
  fieldCount: rows.length,
};
