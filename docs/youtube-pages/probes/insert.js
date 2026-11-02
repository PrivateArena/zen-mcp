const el = document.querySelector("ytcp-social-suggestions-textbox#description-textarea div#textbox");
if (!el) return { ok: false, why: "element not found" };
el.focus();
const sel = window.getSelection();
sel.selectAllChildren(el);
sel.collapseToEnd();
const text = args.text;
let method = "execCommand:insertText";
let ok = document.execCommand("insertText", false, text);
if (!el.textContent.trim()) {
  method = "textContent+input";
  el.textContent = text;
  el.dispatchEvent(new InputEvent("input", { bubbles: true, cancelable: true, data: text, inputType: "insertText" }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
  ok = true;
}
return { ok, method, contenteditable: el.getAttribute("contenteditable"), text: el.textContent.slice(0, 80), html: (el.innerHTML||"").slice(0,80) };
