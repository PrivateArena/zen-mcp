// The cheap verification channel: the tab's URL plus a bounded slice of its visible text. Every flow
// confirms an action by a phrase appearing here ("All changes saved", "No issues found", "is scheduled"),
// never by the action's own success flag. args.max bounds the payload.
const max = (args && args.max) || 4000;
const text = (document.body && document.body.innerText) || "";
return { url: location.href, title: document.title || "", len: text.length, text: text.replace(/\s+/g, " ").slice(0, max) };
