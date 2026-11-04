var deep = function (sel, root, out) { // the details page as a section map
  out = out || []; const r = root || document;
  try { for (const n of r.querySelectorAll(sel)) out.push(n); } catch (e) {}
  for (const host of r.querySelectorAll("*")) if (host.shadowRoot && host.tagName !== "YT-I18N-MODULE") deep(sel, host.shadowRoot, out);
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
const near = (n) => {
  let c = n;
  for (let i = 0; i < 8 && c; i++) {
    if (c.id) return c.id;
    c = c.parentElement || (c.getRootNode() && c.getRootNode().host) || null;
  }
  return null;
};
const editables = deep("[contenteditable='true']").map(n => ({ id: n.id || null, owner: near(n), txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 50), path: path(n) }));
const radios = deep("tp-yt-paper-radio-group").map(n => ({ id: n.id || null, aria: n.getAttribute("aria-label"), txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 60), path: path(n) }));
const sections = deep("ytcp-form-container, ytcp-set, ytcp-video-details-form, ytcp-form-renderer").map(n => ({ tag: n.tagName.toLowerCase(), id: n.id || null, txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 70) })).filter(s => s.txt);
const tabs = deep("tp-yt-paper-tab, [role='tab']").map(n => ({ sel: n.getAttribute("aria-selected"), txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 24) }));
const btns = deep("button, tp-yt-paper-button").map(n => ({ id: n.id || null, aria: n.getAttribute("aria-label"), txt: (n.innerText || "").replace(/\s+/g, " ").slice(0, 28) })).filter(b => b.aria || b.txt).slice(0, 40);
return { editables, radios, sections: sections.slice(0, 12), tabs, buttons: btns, body: (document.body.innerText || "").replace(/\s+/g, " ").slice(0, 150) };
