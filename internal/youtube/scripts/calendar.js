// Commits a date to the Studio schedule picker by clicking the real calendar.
//
// Why not type it: the picker's text field displays anything you write into it and then resets to the
// model's value the moment the dialog closes — verified live, where a written date displayed, the trigger
// kept the old date, and a written time displayed while the committed value stayed at midnight. A DOM click
// on the day cell is what actually commits, and it needs no OS focus, so the human's cursor stays put.
//
// args.month is the month header as Studio renders it ("NOV 2026"), args.day the day number ("9"),
// args.trigger the selector of the element whose text must change as proof.
const L = args;
const clean = (s) => (s === null || s === undefined ? "" : String(s).replace(/\s+/g, " ").trim());
const all = (sel, root) => Array.from((root || document).querySelectorAll(sel));

const months = all(".cal-month, [class*='cal-month']").filter((m) => {
  const label = m.querySelector(".cal-month-label, [class*='cal-month-label']");
  return label && clean(label.textContent).toUpperCase() === String(L.month).toUpperCase();
});
if (!months.length) return { clicked: false, reason: "month " + L.month + " is not rendered", months: [] };

const cells = [];
for (const m of months) {
  for (const c of all(".calendar-day, [class*='calendar-day']", m)) {
    cells.push({ el: c, cls: clean(c.className), text: clean(c.textContent) });
  }
}
const want = String(L.day);
const match = cells.filter((c) => c.text === want && !/disabled/.test(c.cls));
if (!match.length) {
  return { clicked: false, reason: "day " + want + " is not selectable in " + L.month, months: months.map(clean) };
}

const chosen = match[0];
const before = clean((document.querySelector(L.trigger) || {}).innerText);
chosen.el.click();
const after = clean((document.querySelector(L.trigger) || {}).innerText);

return {
  clicked: true,
  day: want,
  month: L.month,
  classes: chosen.cls,
  before,
  after,
  committed: after !== before && after.includes(want),
  rendered: cells.length,
};
