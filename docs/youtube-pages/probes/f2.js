const seg = (n) => n.tagName.toLowerCase() + (n.id ? "#" + n.id : "") +
  (n.getAttribute && n.getAttribute("data-test-id") ? "[t=" + n.getAttribute("data-test-id") + "]" : "");
const pathOf = (el) => { const p = []; let n = el;
  while (n && n.nodeType === 1 && p.length < 9) { const h = n.getRootNode() && n.getRootNode().host; p.unshift(seg(n)); n = h || n.parentElement; } return p.join(" > "); };
const row = (el) => ({ tag: el.tagName.toLowerCase(), id: el.id || undefined, role: el.getAttribute("role") || undefined,
  label: (el.getAttribute("aria-label") || "").slice(0, 62), text: (el.innerText || el.value || "").trim().slice(0, 44),
  disabled: el.disabled === true || el.getAttribute("aria-disabled") === "true",
  checked: el.getAttribute("aria-checked") || undefined, testId: el.getAttribute("data-test-id") || undefined, path: pathOf(el) });
const box = document;
return {
  title: [...box.querySelectorAll('[role="textbox"]')].map(row).filter(r => /title|video name/i.test(r.label||r.text||"")),
  titleAll: [...box.querySelectorAll('[role="textbox"]')].map(row).map(r => ({id:r.id,label:r.label.slice(0,40)})),
  desc: [...box.querySelectorAll('[role="textbox"]')].map(row).filter(r => /viewers|description/i.test(r.label||"")),
  next: [...box.querySelectorAll('button, ytcp-button')].map(row).filter(r => /^next$/i.test((r.label||"") + "" ) || /^next$/i.test(r.text||"")),
  radios: [...box.querySelectorAll('tp-yt-paper-radio-button, [role="radio"]')].map(row),
  headers: [...box.querySelectorAll('h1,h2,[role="heading"],#stepper-app-bar-actions,h3')].map(e => (e.innerText||"").trim()).filter(t=>t&&t.length<50).slice(0,10),
  tabs: [...box.querySelectorAll('[role="tab"]')].map(row).map(r => ({t:r.text,a:r.checked,dis:r.disabled})),
};
