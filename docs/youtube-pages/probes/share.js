const box = document.querySelector(".ytcp-video-share-dialog, [class*='video-share-dialog']") || document;
const text = (box.innerText || "").replace(/\s+/g, " ").trim();
const urls = (text.match(/https?:\/\/[^\s"]+/g) || []);
const b = (document.body.innerText || "").replace(/\s+/g, " ");
return {
  shareText: text.slice(0, 220),
  urls: urls.slice(0, 4),
  scheduled: b.match(/is scheduled[^.]{0,70}/gi) || [],
  buttons: [...document.querySelectorAll("button, ytcp-button")].filter(e => { const r = e.getBoundingClientRect(); return r.width && r.height; })
    .map(e => (e.getAttribute("aria-label") || e.innerText || "").trim().replace(/\s+/g, " ").slice(0, 26)).filter(Boolean).slice(-10)
};
