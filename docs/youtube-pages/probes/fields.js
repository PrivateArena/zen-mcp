const seg = (n) => n.tagName.toLowerCase() + (n.id ? "#" + n.id : "") +
  (n.getAttribute && n.getAttribute("data-test-id") ? "[t=" + n.getAttribute("data-test-id") + "]" : "");
const pathOf = (el) => { const p = []; let n = el;
  while (n && n.nodeType === 1 && p.length < 9) { const h = n.getRootNode() && n.getRootNode().host; p.unshift(seg(n)); n = h || n.parentElement; } return p.join(" > "); };
const box = document.querySelector('[role="dialog"]') || document;
const q = (sel) => [...box.querySelectorAll(sel)].map(el => ({
  tag: el.tagName.toLowerCase(), id: el.id || undefined, role: el.getAttribute("role") || undefined,
  label: (el.getAttribute("aria-label") || "").slice(0, 60), text: (el.innerText || el.value || "").trim().slice(0, 50),
  disabled: el.disabled === true || el.getAttribute("aria-disabled") === "true" || el.getAttribute("aria-checked"),
  path: pathOf(el) }));
const share = (document.body.innerText.match(/https?:\/\/(?:www\.)?(?:youtu\.be|youtube\.com)\/[^\s"')]+/g) || []).slice(0, 5);
return {
  title: q('div[role="textbox"][aria-label*="title" i], input#content'),
  description: q('div[role="textbox"][aria-label*="viewers" i]'),
  next: q('button[aria-label="Next"], ytcp-button[aria-label="Next"]'),
  radios: q('tp-yt-paper-radio-button'),
  videoLinkCard: (document.body.innerText.match(/Video link[\s\S]{0,120}/) || [""])[0].replace(/\s+/g," ").slice(0,160),
  shareUrls: share,
  saveState: (document.body.innerText.match(/Saved as \w+|Save as \w+/g) || []).slice(0,3),
  stepHeader: (document.querySelector("h1,paper-tabs") || {}).innerText,
};
