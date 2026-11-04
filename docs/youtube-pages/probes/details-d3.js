var deep = function (sel, root, out) { // the two editable fields, the visibility card, and the overflow menu
  out = out || []; const r = root || document;
  try { for (const n of r.querySelectorAll(sel)) out.push(n); } catch (e) {}
  for (const host of r.querySelectorAll("*")) if (host.shadowRoot && host.tagName !== "YT-I18N-MODULE") deep(sel, host.shadowRoot, out);
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
const eds = deep("[contenteditable='true']").map(n => ({ path: path(n), aria: n.getAttribute("aria-label"), txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 44) }));
const labelled = deep("[aria-label]").map(n => ({ tag: n.tagName.toLowerCase(), aria: n.getAttribute("aria-label"), id: n.id || null, txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 30) })).filter(x => /visib|delete|more|save|thumbnail|playlist|title|description|audience|age|category|chapters|language/i.test(x.aria)).slice(0, 30);
const banner = deep("ytcp-details-banner, ytcp-video-title, #header-bar, ytcp-video-metadata-header").map(n => ({ path: path(n), txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 120) }));
const rail = deep("ytcp-video-visibility-submit, #visibility, ytcp-visibility-card, .visibility-tab, ytcp-video-preview").map(n => ({ path: path(n), txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 90) }));
const dots = deep("tp-yt-paper-icon-button, ytcp-icon-button, [aria-label*='ore options'], [aria-label*='ctions']").map(n => ({ tag: n.tagName.toLowerCase(), id: n.id || null, aria: n.getAttribute("aria-label"), icon: n.getAttribute("icon") || n.getAttribute("path"), txt: (n.innerText || "").trim().slice(0, 24), path: path(n) }));
return { editables: eds, labelled, banner, rail, dots, saveMsg: ((deep("#save-msg")[0] || {}).innerText || "") };
