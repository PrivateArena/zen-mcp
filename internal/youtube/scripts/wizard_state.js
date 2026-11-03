// The upload wizard's one verification probe (plan §3 R5: "did the panel change" lives on tab state, not
// on the step headers). One call answers: which step is active, which footer buttons exist and whether
// they are clickable, what the transfer says, whether Studio saved, what the blocking messages are, and
// what the final share dialog reports.
const clean = (s) => (s === null || s === undefined ? "" : String(s).replace(/\s+/g, " ").trim());
const vis = (n) => { const r = n.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
const all = (sel) => Array.from(document.querySelectorAll(sel));

const tabs = all("[role=tab]").map((t) => ({ text: clean(t.innerText || t.getAttribute("aria-label")), selected: t.getAttribute("aria-selected") === "true" }));
const buttons = all("ytcp-button, button").filter(vis).map((b) => {
  const inner = b.matches("button") ? b : b.querySelector("button");
  return {
    id: b.id || (inner && inner.id) || undefined,
    label: clean(b.getAttribute("aria-label")) || clean(b.innerText),
    disabled: !!(b.disabled || (inner && inner.disabled) || (b.hasAttribute("disabled") || (inner && inner.hasAttribute("disabled")))),
  };
}).filter((b) => b.label);

const dialog = all("ytcp-uploads-dialog").some(vis);
const shareBox = document.querySelector(".ytcp-video-share-dialog, [class*='video-share-dialog']");
const shareText = shareBox ? clean(shareBox.innerText) : "";
const body = clean(document.body && document.body.innerText);

const progress = all(".progress-label, [class*='upload-progress'] , [class*='video-creation-header']")
  .filter(vis).map((n) => clean(n.innerText)).filter(Boolean).slice(0, 6);

const alerts = all("[role=alert], [class*='error-text']").filter(vis).map((n) => clean(n.innerText)).filter(Boolean).slice(0, 6);

const links = ((shareText + " " + body).match(/https?:\/\/(?:www\.youtube\.com\/watch\?v=|youtu\.be\/)[A-Za-z0-9_-]{6,}/g) || []);
const videoId = (links[0] || "").indexOf("watch?v=") >= 0
  ? links[0].split("watch?v=")[1]
  : (links[0] || "").split("youtu.be/")[1] || "";

return {
  dialog,
  panels: tabs,
  panel: (tabs.find((t) => t.selected) || {}).text || "",
  buttons: buttons.slice(0, 12),
  progress,
  saved: /all changes saved|saved as private|changes saved/i.test(body),
  checksComplete: /checks complete|no issues found/i.test(body),
  copyright: clean((body.match(/Copyright[^\n]{0,60}/i) || [""])[0]).slice(0, 80),
  scheduled: clean((body.match(/video (?:upload )?is scheduled[^\n]{0,90}/i) || [""])[0]),
  alerts,
  share: shareText.slice(0, 260),
  videoId,
  links: links.slice(0, 3),
  url: location.href,
};
