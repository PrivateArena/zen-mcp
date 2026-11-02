const b = (document.body.innerText || "").replace(/\s+/g, " ");
return {
  url: location.href,
  scheduled: b.match(/is scheduled[^.]{0,60}/gi) || [],
  done: b.match(/All done|View on YouTube|Copy link|Edit video|Video URL/gi) || [],
  stillOnVisibility: !!document.querySelector("ytcp-video-visibility-select"),
  snippet: b.slice(0, 300)
};
