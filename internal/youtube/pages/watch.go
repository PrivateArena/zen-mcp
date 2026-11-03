package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"zen-mcp/internal/youtube"
)

// Frozen locators for the watch page. Every string here was copied out of docs/youtube-pages/watch.md §1–§3
// or the tree in watch-run.md §1, from the live run of 2026-11-03 on watch?app=desktop&v=YSNFGgh0aAs — the
// desktop render those notes describe. Nothing is inferred: the sort menu, the "Show more replies" control and
// the comments-disabled wording are absent because they were never recorded, and the flows refuse rather than
// guess at them.
const (
	// WatchURL carries app=desktop for the same reason the probe opened that URL: every selector below was
	// recorded on the desktop render (watch.md header), and the plain /watch?v= form is what §5.1 prescribes.
	WatchURL = "https://www.youtube.com/watch?app=desktop&v="

	locComments       = "ytd-comments#comments"                     // §1 Section
	locSections       = "ytd-item-section-renderer#sections"        // §1 Section
	locThread         = "ytd-comment-thread-renderer"               // §1 Thread
	locCommentsHeader = "ytd-comments-header-renderer"              // §1 Header
	locSimpleBox      = "ytd-comment-simplebox-renderer#simple-box" // §2 Closed trigger
	locReplyDialog    = "ytd-comment-reply-dialog-renderer#replybox"

	// The two boxes share these three parts, so they are only ever addressed under one of the box roots
	// above: §3 shows both boxes open at once, and "the first #commentbox in the document" can post into the
	// wrong one.
	locPlaceholder   = "yt-formatted-string#simplebox-placeholder" // §2 Closed trigger
	locCommentDialog = "ytd-comment-dialog-renderer"               // §2 Opened box
	locEditor        = "#contenteditable-root"                     // §2 Editor
	locSubmit        = "ytd-button-renderer#submit-button button"  // §2 Submit
	// §7: pressing cancel through either channel left the box open — YouTube listens for its own host-level
	// handler — so it is recorded here as the control the probe found, and used by no flow step.
	locCancel         = "ytd-button-renderer#cancel-button button"    // §2 Cancel
	locRowReplyButton = "ytd-button-renderer#reply-button-end button" // §1 Reply button
)

// Page text recorded by the probe, used only for waits and for checking a control is the one the probe met.
const (
	txtSortBy   = "Sort by"  // §1 Header
	txtComments = "Comments" // §1 Header, plural form
	txtComment  = "Comment"  // §1 Header, §2 submit aria-label
	txtReply    = "Reply"    // §1 Reply button, §3 reply submit aria-label
	locScrollY  = 800        // the wheel offset that clears the sticky header (§7)
)

// commentRow is one row as the page renders it, and Path is its page-proven address — derived at runtime by
// comments.js from the row's real ancestors, the way lookup.js reports the path of a finding. A reply must be
// scoped to one row, and #replybox is not unique across rows (§3), so the address is what makes the press
// unambiguous. An empty Path means the chain is not unique, and the flow refuses rather than aim blindly.
type commentRow struct {
	ID       string `json:"id"`
	Author   string `json:"author"`
	Text     string `json:"text"`
	When     string `json:"when"`
	Absolute string `json:"absolute"`
	Nested   bool   `json:"nested"`
	IsReply  bool   `json:"isReply"`
	Path     string `json:"path"`
}

type commentRead struct {
	Video       string       `json:"video"`
	Requested   string       `json:"requested"`
	Count       int          `json:"count"`
	HeaderCount int          `json:"headerCount"`
	HeaderText  string       `json:"headerText"`
	Rows        []commentRow `json:"rows"`
}

// box is one of the two comment editors, with the ancestor that makes it addressable (§3), the aria-label its
// submit button carries (§2/§3) and the words the step text calls it by.
type box struct {
	root     string // the ancestor every lookup inside this box is scoped by (§3)
	label    string // the submit button's probed aria-label: "Comment" (§2) or "Reply" (§3)
	describe string // what the step text calls it
}

type watch struct {
	p    *youtube.Page
	t    youtube.Timings
	in   youtube.WatchInput
	out  youtube.Outcome
	step int
}

func (w *watch) note(format string, args ...any) {
	w.out.Steps = append(w.out.Steps, fmt.Sprintf("%d. %s", w.step, fmt.Sprintf(format, args...)))
}

func (w *watch) warn(format string, args ...any) {
	w.out.Warnings = append(w.out.Warnings, fmt.Sprintf(format, args...))
}

func (w *watch) shot(ctx context.Context, name string) {
	if w.in.Evidence == "" {
		return
	}
	if err := w.p.Screenshot(ctx, filepath.Join(w.in.Evidence, name+".png")); err != nil {
		w.warn("screenshot %s: %v", name, err)
	}
}

// Watch runs one watch-page action — read the rendered comments, post a top-level comment, or answer a row —
// against the video in in.VideoID.
//
// Comment and reply each press submit exactly once. The probe is unambiguous about why (watch-run.md §5): the
// box stayed open 20 s after a reply that had in fact posted, so a press cannot be judged by what the box did,
// and re-pressing is how a duplicate appears. Confirmation is a fresh read of the thread instead, and a
// confirmation that does not find the row is reported as transient, never retried.
func Watch(ctx context.Context, p *youtube.Page, in youtube.WatchInput, tim youtube.Timings) (youtube.Outcome, error) {
	if tim.Action <= 0 {
		tim.Action = 45 * time.Second
	}
	w := &watch{p: p, t: tim, in: in}
	w.out.VideoID = in.VideoID
	w.out.URL = WatchURL + in.VideoID

	switch in.Action {
	case "read":
	case "comment", "reply":
		// A public act that cannot be unsent must be recorded, which is what the ledger is for (§4.6). A dry run
		// is deliberately handed no journal by the service — it records nothing, because it presses nothing — so
		// the requirement is on the write, not on the rehearsal that refuses to perform it.
		if in.Journal == nil && !in.DryRun {
			return w.out, fmt.Errorf("%w: a %s has nowhere to be recorded — the caller must supply the journal", youtube.ErrUsage, in.Action)
		}
	default:
		return w.out, fmt.Errorf(`%w: watch action %q is not read, comment or reply`, youtube.ErrUsage, in.Action)
	}

	if err := w.open(ctx); err != nil {
		return w.out, err
	}
	var err error
	switch in.Action {
	case "comment":
		err = w.commentAction(ctx)
	case "reply":
		err = w.replyAction(ctx)
	default:
		err = w.readAction(ctx)
	}
	return w.out, err
}

// open navigates the watch page. The identity assertion is the container the navigate reply reports; a watch
// URL carries no channel id, so the channel half of R1 cannot be checked here — and the page-side proof the
// probe noted (an own video renders Analytics and Edit video, watch-run.md preamble) only holds for videos of
// this channel, so it is not a general gate.
// UNPROBED: a general page-side identity check on a watch page.
func (w *watch) open(ctx context.Context) error {
	w.step++
	if err := w.p.Open(ctx, w.out.URL); err != nil {
		return err
	}
	w.note("opened watch?v=%s in the configured identity", w.in.VideoID)
	return nil
}

// revealPause is the gap between two attempts to drive the page down while waiting for the section to exist.
// The live page needed about a second after navigate before it had a comment section at all, so a tight loop
// would only spin; every other wait in this package polls on a comparable cadence.
const revealPause = time.Second

// defaultMaxScrolls bounds the reveal loop when the caller supplied no guard, so a zero Timings can never turn
// a bounded wait into an endless one.
const defaultMaxScrolls = 6

// sectionReady pulls the comment section into the viewport and waits for it to render. Two levels of lazy
// rendering are live facts, not theory: ytd-comments#comments itself is absent for about a second after the
// navigate returns (a failed run measured a 23-character document with no section anywhere in it), and the
// rows under it stay absent until the section has been scrolled into view (watch.md trap 6, watch-run.md §7).
// So the page is driven down FIRST and the first structural probe comes after that — probing the section
// before anything exists to bring it up is what turned a slow render into "the page carries no comments".
func (w *watch) sectionReady(ctx context.Context) error {
	w.step++
	scrolls, f, err := w.revealSection(ctx)
	w.out.Record("comments section", f)
	if err != nil {
		return err
	}
	w.note("drove the page down %s and %s is on the page", plural(scrolls, "page scroll"), locComments)

	w.step++
	if err := w.scrollSection(ctx); err != nil {
		return err
	}
	// A row first, because it is one cheap probe: rows only exist once the section has rendered, so finding
	// one proves the wait is over without spending the header's budget on it.
	threads, err := w.p.Find(ctx, youtube.Locator{CSS: locThread})
	w.out.Record("comment thread", threads)
	if err != nil {
		return err
	}
	if threads.OK {
		w.note("comment section rendered with %s", plural(threads.Count, "thread"))
		return nil
	}
	// No rows: wait on the header's own words instead, which are present whether the video has one comment
	// or none (watch.md §1 records the section text as `1 Comment Sort by …`). Waiting for a thread here would
	// turn "no comments yet" into an error.
	got, werr := w.p.WaitAny(ctx, []string{txtSortBy, txtComments, txtComment}, w.t.Action)
	if werr == nil {
		w.note("comment section rendered with no comment threads; header shows %q", got)
		return nil
	}
	// The rows may still have arrived while the header text was being polled.
	if threads, err = w.p.Find(ctx, youtube.Locator{CSS: locThread}); err != nil {
		return err
	}
	w.out.Record("comment thread", threads)
	if !threads.OK {
		// The section is there — revealSection proved that — so this is not the empty case: a video that renders
		// its section with no comments is answered above, as a section with no threads, and is not an error.
		return fmt.Errorf("%w: %s is on the page but scrolling it into view produced neither a %s nor a %s under %s, so the section never filled in — this is not an empty comment section, which answers with the header's own words (%v)",
			youtube.ErrWait, locComments, locThread, locCommentsHeader, locSections, werr)
	}
	return nil
}

// revealSection brings the comment section into existence and returns how many times the page had to be
// driven down to get there. The section is what is being waited for, so the scroll comes first and the probe
// second — the old order probed a structure the page had not built yet and reported that as the page having
// no comments at all.
//
// Bounded twice over, with no unbounded wait: Timings.MaxScrolls scrolls, inside the one-page action budget.
func (w *watch) revealSection(ctx context.Context) (int, youtube.Finding, error) {
	rounds := w.t.MaxScrolls
	if rounds <= 0 {
		rounds = defaultMaxScrolls
	}
	deadline := time.Now().Add(w.t.Action)
	section := youtube.Locator{CSS: locComments}
	// Asked once, before any scrolling, because no scroll can fix it: a hidden document is throttled, and
	// YouTube defers the whole comment section of one (measured 2026-11-03 — the same URL gave 23 characters
	// and no ytd-comments hidden, and the section visible). Failing here says so in one round trip instead of
	// spending the scroll budget on a page that was never going to build.
	if hidden, chars, herr := w.p.Hidden(ctx); herr == nil && hidden {
		return 0, youtube.Finding{}, fmt.Errorf("%w: %s builds only in a visible tab and this run holds a backgrounded one (%d characters of body, hidden throughout). YouTube defers the section while the document is hidden, so scrolling cannot produce it — set youtube.allowFocusBoundActions to true to let this action open a foreground tab (watch.md trap 6)",
			youtube.ErrElement, locComments, chars)
	}
	var last youtube.Finding
	for attempt := 1; ; attempt++ {
		if err := w.scrollPage(ctx); err != nil {
			return attempt, last, err
		}
		f, err := w.p.Find(ctx, section)
		last = f
		if err != nil {
			return attempt, last, err
		}
		if f.OK {
			return attempt, f, nil
		}
		// Still no section. Either guard ends the loop; neither is a feature limit.
		stopped := ""
		switch {
		case attempt >= rounds:
			stopped = fmt.Sprintf("the %s guard ran out", plural(rounds, "scroll"))
		case !time.Now().After(deadline):
			select {
			case <-ctx.Done():
				return attempt, last, youtube.ErrAbandoned{Detail: ctx.Err().Error()}
			case <-time.After(revealPause):
			}
			continue
		default:
			stopped = fmt.Sprintf("the %s page budget ran out", w.t.Action)
		}
		// Absent is not the same as empty: a section that renders with zero comments is read as an empty list,
		// and never reaches this error. What a page with comments switched off answers is UNPROBED, so naming
		// that state here would be a guess — the narrow claim is that the section never appeared.
		return attempt, last, fmt.Errorf("%w: the page carried no %s at all — it was driven down %s and %s, so the watch page never rendered a comment section. This is a section that never appeared, not a video with no comments: a rendered-but-empty section reads as an empty list (watch.md trap 6, watch-run.md §7)",
			youtube.ErrElement, locComments, plural(attempt, "scroll"), stopped)
	}
}

// scrollPage drives the window down through the page's own scrolling. It cannot be a bridge scroll: that one
// needs an element to aim at and deliberately takes a numeric offset rather than a script, and before the
// section exists there is nothing to aim at. The program is the embedded scroll_page.js, run through Page.Run.
func (w *watch) scrollPage(ctx context.Context) error {
	var out struct {
		Before     float64 `json:"before"`
		After      float64 `json:"after"`
		HasSection bool    `json:"hasSection"`
	}
	if err := w.p.Run(ctx, "scroll_page.js", map[string]any{"selector": locComments}, &out); err != nil {
		return fmt.Errorf("drive the page down: %w", err)
	}
	return nil
}

// scrollSection is the read side of trap 6, and it is needed again after a post: the page scrolled itself
// back to the player afterwards (watch-run.md §7), and the sticky header covers the top of the viewport, so
// the section is aligned at start with an offset rather than merely brought "into view".
func (w *watch) scrollSection(ctx context.Context) error {
	if err := w.p.Scroll(ctx, youtube.Locator{CSS: locComments}, locScrollY, "start"); err != nil {
		return fmt.Errorf("scroll %s into view: %w", locComments, err)
	}
	return nil
}

// readRows runs the comment probe. It hands back the raw reply as well as the decoded rows: the outcome
// reports what the page answered, verbatim, so a flow cannot paraphrase its way into claiming more.
func (w *watch) readRows(ctx context.Context) (json.RawMessage, *commentRead, error) {
	raw, err := w.p.Eval(ctx, youtube.Script("comments.js"), map[string]any{"video": w.in.VideoID})
	if err != nil {
		return nil, nil, err
	}
	got := &commentRead{HeaderCount: -1}
	if err := json.Unmarshal(raw, got); err != nil {
		return raw, got, fmt.Errorf("decode the comment read: %w", err)
	}
	// The id in the URL, not the id we asked for, means a redirect — to another video, or to a wall. Reading
	// on regardless would return the wrong thread and call it the right one.
	if got.Video != "" && got.Video != w.in.VideoID {
		return raw, got, fmt.Errorf("%w: the page is showing video %s, not %s — nothing was read from it",
			youtube.ErrIdentity, got.Video, w.in.VideoID)
	}
	return raw, got, nil
}

func (w *watch) readAction(ctx context.Context) error {
	// UNPROBED: the control that loads the next page of a thread. The plan's read loop scrolls "until the
	// thread count stops growing", but comments-per-scroll was never measured (the probe video has one thread),
	// so this covers what the section rendered after one scroll into view and no continuation is ever pressed.
	if err := w.sectionReady(ctx); err != nil {
		return err
	}
	w.step++
	raw, got, err := w.readRows(ctx)
	if err != nil {
		return err
	}
	w.out.Data = raw
	// §6 trap 1: the header count goes stale, still reading "1 Comment" after a second post. The rows are the
	// read set; the header is reported as the page's own opinion when the two disagree.
	if got.HeaderCount >= 0 && got.HeaderCount != got.Count {
		w.warn("the header says %d (%s) but the page renders %d rows; the rows are what is reported (watch-run.md §6)",
			got.HeaderCount, firstNonEmpty(got.HeaderText, "no header text"), got.Count)
	}
	w.note("read %s from the page", plural(got.Count, "comment row"))
	w.shot(ctx, "comments")
	return nil
}

func (w *watch) commentAction(ctx context.Context) error {
	if strings.TrimSpace(w.in.Text) == "" {
		return fmt.Errorf("%w: a comment needs text", youtube.ErrUsage)
	}
	if err := w.sectionReady(ctx); err != nil {
		return err
	}
	w.step++
	_, got, err := w.readRows(ctx)
	if err != nil {
		return err
	}
	// The rows before the press are both the duplicate check the plan asks for and the "already there" set
	// that makes the read-back after the press mean something.
	if dup := findText(got.Rows, w.in.Text); dup != nil {
		return fmt.Errorf("%w: comment %s on this video already carries exactly that text — posting it again would be a duplicate",
			youtube.ErrUsage, firstNonEmpty(dup.ID, "(no id)"))
	}
	if w.in.DryRun {
		trigger := youtube.Locator{Root: locSimpleBox, CSS: locPlaceholder}
		f, ferr := w.p.Find(ctx, trigger)
		w.out.Record("comment box trigger", f)
		if ferr != nil {
			return ferr
		}
		if !f.OK {
			return fmt.Errorf("%w: %s offers no %s — the comment box is not reachable on this video",
				youtube.ErrElement, locSimpleBox, locPlaceholder)
		}
		// §2: the editor does not exist in the DOM until the placeholder is pressed, so a dry run that never
		// presses cannot report the editor or the submit button — only that the trigger is there to press.
		w.note("dry run: %d rows read and the comment trigger located; the editor cannot be resolved without opening the box, and nothing was pressed", got.Count)
		return nil
	}

	w.step++
	slot, err := w.claim()
	if err != nil {
		return err
	}
	// The placeholder is a yt-formatted-string with role=textbox, not a button (§2 trap 4), and the editor only
	// comes into existence after this press.
	trigger := youtube.Locator{Root: locSimpleBox, CSS: locPlaceholder}
	f, err := w.p.Click(ctx, trigger, "", 0)
	w.out.Record("comment box trigger", f)
	if err != nil {
		return w.fail(slot, fmt.Errorf("press %s: %w", locPlaceholder, err))
	}
	return w.submit(ctx, box{root: locCommentDialog, label: txtComment, describe: "comment box"}, slot,
		func(c context.Context) (youtube.Finding, error) {
			return w.p.WaitState(c, youtube.Locator{CSS: locCommentDialog}, w.t.Action, func(g youtube.Finding) bool { return g.OK })
		},
		func(c context.Context) (commentRow, error) {
			return w.confirm(c, idsOf(got.Rows), "the posted comment", func(r commentRow) bool {
				return !r.IsReply && r.ID != "" && sameCommentText(r.Text, w.in.Text)
			})
		})
}

// replyAction answers one row. The row is found in the read set before anything is pressed, and every lookup
// afterwards is scoped under that row: replying to the wrong comment is a public answer to the wrong person,
// so a row this page does not show is a refusal rather than an attempt.
func (w *watch) replyAction(ctx context.Context) error {
	if strings.TrimSpace(w.in.CommentID) == "" {
		return fmt.Errorf("%w: a reply needs comment_id, the lc id a read returned", youtube.ErrUsage)
	}
	if strings.TrimSpace(w.in.Text) == "" {
		return fmt.Errorf("%w: a reply needs text", youtube.ErrUsage)
	}
	if err := w.sectionReady(ctx); err != nil {
		return err
	}
	w.step++
	_, got, err := w.readRows(ctx)
	if err != nil {
		return err
	}
	row := findID(got.Rows, w.in.CommentID)
	if row == nil {
		return fmt.Errorf("%w: comment_id %q is not among the %s this page renders — read the comments first and address a row that read returned",
			youtube.ErrUsage, w.in.CommentID, plural(got.Count, "comment row"))
	}
	if row.Path == "" {
		return fmt.Errorf("%w: the row for comment_id %q has no address that is unique on this page, so its Reply button cannot be named without guessing",
			youtube.ErrElement, w.in.CommentID)
	}
	// Both boxes can be open at once (§3), and #replybox is not unique across rows, so the box is addressed
	// as a descendant chain of this row — a chain written without commas, which is what trap 2 warns about.
	boxRoot := row.Path + " " + locReplyDialog
	reply := youtube.Locator{Root: row.Path, CSS: locRowReplyButton}

	if w.in.DryRun {
		f, ferr := w.p.Find(ctx, reply)
		w.out.Record("reply button", f)
		if ferr != nil {
			return ferr
		}
		if !f.OK {
			return fmt.Errorf("%w: %s carries no %s — the row cannot be answered", youtube.ErrElement, row.ID, locRowReplyButton)
		}
		w.note("dry run: %d rows read and the reply affordance for %s located; the reply editor cannot be resolved before the box is opened, and nothing was pressed", got.Count, row.ID)
		return nil
	}

	w.step++
	slot, err := w.claim()
	if err != nil {
		return err
	}
	// The row is addressed by its own lc id (watch.md §1), which is why the wrong comment cannot be answered.
	f, err := w.p.Click(ctx, reply, "", 0)
	w.out.Record("reply button", f)
	if err != nil {
		return w.fail(slot, fmt.Errorf("press %s: %w", locRowReplyButton, err))
	}
	return w.submit(ctx, box{root: boxRoot, label: txtReply, describe: "reply box"}, slot,
		func(c context.Context) (youtube.Finding, error) {
			return w.p.WaitState(c, youtube.Locator{CSS: boxRoot}, w.t.Action, func(g youtube.Finding) bool { return g.OK })
		},
		func(c context.Context) (commentRow, error) {
			// §5: a reply's id is <parent>.<suffix>, and nested rows live under #replies — so the id alone
			// proves which thread the page answered, without trusting the box's own behaviour (§5, §7).
			return w.confirm(c, idsOf(got.Rows), "the reply", func(r commentRow) bool {
				return strings.HasPrefix(r.ID, w.in.CommentID+".") && r.Nested
			})
		})
}

// claim takes the journal slot for one comment write. It happens here, after the page is open and the target
// is known but before any press: §4.6's order is claim-then-mutate, and a comment cannot be unsent, so the
// slot must already be on the ledger when the press is issued.
func (w *watch) claim() (youtube.JournalSlot, error) {
	return w.in.Journal.Claim("comment")
}

// fail marks a claimed slot abandoned and hands the reason back, so a run that wrote nothing still leaves a
// trace of what it tried.
func (w *watch) fail(slot youtube.JournalSlot, err error) error {
	if w.in.Journal != nil && slot.SlotID != "" {
		if jerr := w.in.Journal.Abandon(slot.SlotID, "", err.Error()); jerr != nil {
			w.warn("journal could not abandon slot %s: %v", slot.SlotID, jerr)
		}
	}
	return err
}

// submit is the half the two write flows share: open box → write → the disabled flip → one press → re-read.
// open presses whatever brings the box up, confirm decides whether the page really took the post.
func (w *watch) submit(ctx context.Context, b box, slot youtube.JournalSlot,
	open func(context.Context) (youtube.Finding, error),
	confirm func(context.Context) (commentRow, error)) error {

	w.step++
	dlg, err := open(ctx)
	w.out.Record(b.describe, dlg)
	if err != nil || !dlg.OK {
		// Nothing was written, so nothing can have posted: the slot is abandoned and the press is not made.
		return w.fail(slot, fmt.Errorf("%w: pressing %s never brought up the %s under %s (%v)",
			youtube.ErrElement, b.describe, locEditor, b.root, err))
	}

	w.step++
	editor := youtube.Locator{Root: b.root, CSS: locEditor}
	ef, read, err := w.p.Type(ctx, editor, w.in.Text)
	w.out.Record("comment editor", ef)
	if err != nil {
		return w.fail(slot, fmt.Errorf("write the %s: %w", b.describe, err))
	}
	w.note("%s holds %d characters", b.describe, len([]rune(read)))
	w.shot(ctx, "box-filled")

	w.step++
	submit := youtube.Locator{Root: b.root, CSS: locSubmit}
	// watch-run.md §4: the submit button flipping disabled true → false is the page's own acknowledgement that
	// insertText reached its model. A press without that flip cannot tell a committed write from a cosmetic
	// one, and a press is unrecoverable, so the flow stops here rather than discover the difference publicly.
	btn, err := w.p.WaitState(ctx, submit, w.t.Action, func(g youtube.Finding) bool {
		return g.OK && !g.Disabled && !g.State.Disabled
	})
	w.out.Record("submit", btn)
	if err != nil || btn.Disabled || btn.State.Disabled {
		if btn.OK {
			return w.fail(slot, fmt.Errorf("%w: %s stayed disabled after writing into the editor, so the page never accepted the text (watch-run.md §4) — nothing was posted",
				youtube.ErrElement, locSubmit))
		}
		return w.fail(slot, fmt.Errorf("%w: the %s never offered %s (%v)", youtube.ErrElement, b.describe, locSubmit, err))
	}
	if btn.Name != "" && btn.Name != b.label {
		w.warn("%s is labelled %q, not the probed %q (watch.md §2/§3)", locSubmit, btn.Name, b.label)
	}
	w.shot(ctx, "box-ready")

	w.step++
	// Exactly one press, and never another: the probe met a box that stayed open 20 s after a reply that had
	// posted (watch-run.md §5), so the box's own behaviour is not evidence and a second press is the duplicate.
	pressed, err := w.p.Click(ctx, submit, "", 0)
	w.out.Record("submit pressed", pressed)
	if err != nil {
		return w.fail(slot, fmt.Errorf("%w: pressing %s reported a failure (%v) and the post cannot be told from a press that landed, so it will not be repeated",
			youtube.ErrTransient, locSubmit, err))
	}
	w.note("pressed %s once", locSubmit)

	w.step++
	row, err := confirm(ctx)
	if err != nil {
		return w.fail(slot, err)
	}
	w.out.CommentID = row.ID
	// The id bound here is the row's lc id, not a video id: it is the identifier the page handed back for
	// what this run posted, and the one a later reply addresses.
	if jerr := w.in.Journal.Complete(slot.SlotID, row.ID); jerr != nil {
		w.warn("journal could not record the posted comment %s: %v", row.ID, jerr)
	}
	w.note("%s posted: %s by %s, %s", b.describe, row.ID, firstNonEmpty(row.Author, "(no author read)"), firstNonEmpty(row.When, "just now"))
	w.shot(ctx, "posted")
	return nil
}

// confirm re-reads the thread until the row this run posted shows up, and reports which it found. seen is
// the id set read before the press, so a row that was already there can never be mistaken for the new one.
func (w *watch) confirm(ctx context.Context, seen map[string]bool, what string, match func(commentRow) bool) (commentRow, error) {
	// §7: the page scrolls itself back to the player after a post, and the rows are lazy, so the section has
	// to be pulled into view again before a re-read means anything.
	if err := w.scrollSection(ctx); err != nil {
		return commentRow{}, fmt.Errorf("%w: the post was pressed and the section could not be brought back into view to confirm it (%v)", youtube.ErrTransient, err)
	}
	var (
		found commentRow
		last  commentRead
	)
	last.HeaderCount = -1
	err := w.p.Poll(ctx, 900*time.Millisecond, w.t.Action, func(c context.Context) (bool, error) {
		_, got, err := w.readRows(c)
		if err != nil {
			return false, err
		}
		last = *got
		for _, r := range got.Rows {
			if match(r) && !seen[r.ID] {
				found = r
				return true, nil
			}
		}
		return false, nil
	})
	if err == nil {
		return found, nil
	}
	if !errors.Is(err, youtube.ErrWait) {
		// Not a missed row but a page that stopped answering (a redirect, a lost tab): that is the truth, and
		// dressing it as an unverified post would send the caller to look for something that was never posted.
		return commentRow{}, err
	}
	return commentRow{}, fmt.Errorf("%w: %s was pressed once and the %d rows re-read from the page do not contain it. The post either landed or it did not, and a second press is forbidden — check the video before retrying (watch.md §5.5)",
		youtube.ErrTransient, what, last.Count)
}

// idsOf is the pre-press address book: rows already on the page at the moment of the press.
func idsOf(rows []commentRow) map[string]bool {
	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.ID != "" {
			seen[r.ID] = true
		}
	}
	return seen
}

func findID(rows []commentRow, id string) *commentRow {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i]
		}
	}
	return nil
}

func findText(rows []commentRow, text string) *commentRow {
	for i := range rows {
		if rows[i].IsReply {
			continue
		}
		if sameCommentText(rows[i].Text, text) {
			return &rows[i]
		}
	}
	return nil
}

// sameCommentText compares a rendered row with what the flow posted. A collapsed comment is truncated on the
// page itself — the probe read back "Comment posted by the zen-mcp youtube page flow (Sta…" (watch-run.md §6) —
// so a row that is a prefix of the intended text counts as the same comment, and so does one that holds it in
// full.
func sameCommentText(row, posted string) bool {
	r, p := normText(row), normText(posted)
	if r == "" || p == "" {
		return false
	}
	for _, cut := range []string{"...", "…"} {
		if strings.HasSuffix(r, cut) {
			r = strings.TrimRight(strings.TrimSuffix(r, cut), " ")
		}
	}
	return strings.HasPrefix(p, r) || strings.HasPrefix(r, p)
}

func normText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
