// One number that decides whether a lazy page can render at all: a backgrounded document is throttled, and
// YouTube uses that to defer the whole comment section (verified 2026-11-03: hidden tab -> 23 characters of
// body and no ytd-comments, the same URL in a visible tab -> the section rendered).
return { hidden: document.hidden, state: document.visibilityState, chars: (document.body ? document.body.innerText.length : -1) };
