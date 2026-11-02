const texts = [];
const walk = (root) => {
  for (const el of root.querySelectorAll("*")) {
    const t = (el.innerText || "").trim();
    if (t && /%|Uploading|Processed|Finished|Complete|seconds|minutes|Estimated/gi.test(t) && t.length < 120 && el.children.length === 0) {
      texts.push({ tag: el.tagName.toLowerCase(), id: el.id || undefined, testId: el.getAttribute("data-test-id") || undefined, cls: (el.className||"").toString().slice(0,40), text: t.slice(0,80) });
    }
    if (el.shadowRoot) walk(el.shadowRoot);
  }
};
walk(document);
const seen = new Set(); const uniq = texts.filter(t => { const k = t.tag + "|" + t.text; if (seen.has(k)) return false; seen.add(k); return true; });
return { url: location.href, dialog: !!document.querySelector("ytcp-uploads-dialog"), n: uniq.length, texts: uniq.slice(0, 14) };
