package pages_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zen-mcp/internal/youtube"
	"zen-mcp/internal/youtube/pages"
)

// The fixture of the live probe (docs/youtube-pages/watch-run.md): the account's own Unlisted video
// YSNFGgh0aAs, @JangGaming's comment, and the reply under it whose id is <parent>.<suffix> (§5). The shapes
// below are the rows comments.js reports for that page, copied from the run's replies — including the fact
// that a reply's row is the one nested under #replies.
const (
	watchVideo  = "YSNFGgh0aAs"
	seedID      = "UgwEFzl5ZWYy-wHKInF4AaABAg"
	seedReplyID = "UgwEFzl5ZWYy-wHKInF4AaABAg.AcjrtNE1RFuAckORbsxpzd"
)

func seedRows() []map[string]any {
	return []map[string]any{
		{
			"id": seedID, "author": "@JangGaming",
			"text": "What is the tools that you used to create this video ? Looks like hyperframes ?",
			"when": "4 hours ago", "absolute": "Nov 3, 2026, 3:41 AM",
			"nested": false, "isReply": false,
			"path": "ytd-comments#comments > ytd-item-section-renderer#sections > ytd-comment-thread-renderer:nth-of-type(1) > div#comment-container > ytd-comment-view-model#comment",
		},
		{
			"id": seedReplyID, "author": "@PhuongAnh-l1s1z", "text": "Replied by the zen-mcp youtube page flow",
			"when": "0 seconds ago", "absolute": "Nov 3, 2026, 8:12 AM",
			"nested": true, "isReply": true,
			"path": "ytd-comments#comments > ytd-item-section-renderer#sections > ytd-comment-thread-renderer:nth-of-type(1) > div#comment-container > ytd-comment-view-model#comment > div#body > div#replies > ytd-comment-view-model#comment",
		},
	}
}

// watchScript is a scripted watch page: it answers comments.js, lookup.js, fill.js and body_text.js the way the
// live page did, and it moves on the same cues — the submit button only leaves disabled after the text lands,
// and a posted row only appears after the press that posts it. So a flow that passes here cannot be trusting a
// click's success flag, and one that presses submit twice is caught.
type watchScript struct {
	t *testing.T

	events        []string // find:/press:/type:/scroll: in the order the page met them
	clicks        int
	submitPresses int
	scrolls       int
	pageScrolls   int // window scrolls driven by scroll_page.js, the reveal of the lazy section
	shots         int

	rows         []map[string]any
	headerCount  int
	bodyText     string
	missingRow   bool // no comment section at all, however many times the page is driven down
	lazySection  bool // the section does not exist until the page has been driven down (watch.md trap 6)
	noThreads    bool
	hiddenTab    bool // a backgrounded document, which is the state YouTube will not build a section in // the section rendered with no comment rows
	noEditor     bool
	failType     bool
	postedRow    map[string]any // what the page shows after the submit press
	hiddenSubmit bool           // the disabled flag never flips (watch-run.md §4)
	// submitLookups counts reads of the submit button, and flipAt is the read at which the page stops
	// reporting it disabled — the asynchronous acceptance a flow must wait for rather than assume.
	submitLookups int
	flipAt        int
	pressedAt     int // submitLookups at the moment of the press, -1 until pressed
}

func (s *watchScript) Call(ctx context.Context, action string, params map[string]any) (map[string]any, error) {
	switch action {
	case "navigate":
		return ok(map[string]any{"id": "t_watch", "containerId": 20, "status": 200,
			"url": "https://www.youtube.com/watch?app=desktop&v=" + watchVideo}), nil
	case "scroll":
		s.scrolls++
		return ok(nil), nil
	case "click":
		s.clicks++
		label := describe(params)
		s.events = append(s.events, "press:"+label)
		if strings.Contains(label, "#submit-button") {
			s.submitPresses++
			s.pressedAt = s.submitLookups
			if s.postedRow != nil {
				s.rows = append([]map[string]any{s.postedRow}, s.rows...)
			}
		}
		return ok(map[string]any{"el": map[string]any{"tag": "button", "text": label}}), nil
	case "get_text":
		return ok(map[string]any{"el": map[string]any{"tag": "button", "visible": true}, "text": "Comment"}), nil
	case "screenshot":
		s.shots++
		return ok(nil), nil
	case "close_tab":
		return ok(nil), nil
	case "web_eval":
		return s.eval(params)
	}
	return ok(nil), nil
}

func (s *watchScript) eval(params map[string]any) (map[string]any, error) {
	code, _ := params["code"].(string)
	switch {
	case strings.Contains(code, "cheap verification channel"): // body_text.js
		text := s.bodyText
		if text == "" {
			text = "whiteboard - YouTube " + fmt.Sprint(len(s.rows)) + " Comments Sort by"
		}
		return ok(map[string]any{"result": map[string]any{
			"url": "https://www.youtube.com/watch?app=desktop&v=" + watchVideo, "text": text, "len": len(text),
		}}), nil

	case strings.Contains(code, "The watch page comment read"): // comments.js
		rows := s.rows
		count := len(rows)
		if s.noThreads {
			rows, count = nil, 0
		}
		return ok(map[string]any{"result": map[string]any{
			"video": watchVideo, "requested": watchVideo, "count": count,
			"headerCount": s.headerCount, "headerText": fmt.Sprint(len(rows), " Comments Sort by"), "rows": rows,
		}}), nil

	case strings.Contains(code, "deep-query probe"): // lookup.js
		return ok(map[string]any{"result": s.lookup(code)}), nil

	case strings.Contains(code, "visibilityState"): // hidden.js
		chars := 2400
		if s.hiddenTab {
			chars = 23
		}
		return ok(map[string]any{"result": map[string]any{
			"hidden": s.hiddenTab, "state": map[bool]string{true: "hidden", false: "visible"}[!s.hiddenTab], "chars": chars,
		}}), nil

	case strings.Contains(code, "Drive the page down"): // scroll_page.js
		s.pageScrolls++
		s.events = append(s.events, "scroll:window")
		return ok(map[string]any{"result": map[string]any{
			"before": 0, "after": 842, "bottom": 3026, "hasSection": !s.missingRow && !s.lazySection,
		}}), nil

	case strings.Contains(code, "Text entry with proof"): // fill.js
		var args struct {
			L struct {
				CSS  string `json:"css"`
				Root string `json:"root"`
			} `json:"l"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(payload(code), &args); err != nil {
			s.t.Fatalf("fill payload: %v", err)
		}
		s.events = append(s.events, "type:"+args.L.Root+" >> "+args.L.CSS+"="+args.Text)
		if s.failType || s.noEditor {
			return ok(map[string]any{"result": map[string]any{"ok": false, "method": "insertText", "text": "", "err": "read-back differs"}}), nil
		}
		return ok(map[string]any{"result": map[string]any{"ok": true, "method": "insertText", "text": args.Text}}), nil
	}
	s.t.Fatalf("unrecognised web_eval payload:\n%s", code)
	return nil, nil
}

func (s *watchScript) lookup(code string) map[string]any {
	var args struct {
		L struct {
			CSS  string `json:"css"`
			Root string `json:"root"`
		} `json:"l"`
	}
	if err := json.Unmarshal(payload(code), &args); err != nil {
		s.t.Fatalf("lookup payload: %v", err)
	}
	l := args.L
	s.events = append(s.events, "find:"+l.Root+" >> "+l.CSS)
	found := func(extra ...map[string]any) map[string]any {
		base := map[string]any{
			"ok": true, "count": 1, "css": l.CSS, "tag": "div", "text": l.CSS,
			"state": map[string]any{"exists": true, "visible": true, "disabled": false},
		}
		for _, e := range extra {
			for k, v := range e {
				base[k] = v
			}
		}
		return base
	}
	missing := func(reason string) map[string]any {
		return map[string]any{"ok": false, "count": 0, "reason": reason,
			"state": map[string]any{"exists": false, "visible": false, "disabled": false}}
	}
	switch {
	case s.missingRow && strings.Contains(l.CSS, "#comments"):
		return missing("no element for comments")
	case s.lazySection && strings.Contains(l.CSS, "#comments") && s.pageScrolls == 0:
		// The live page for ~1 s after navigate: no ytd-comments#comments anywhere until the page is driven down.
		return missing("the section is lazy: the page has not built it yet")
	case strings.Contains(l.CSS, "ytd-comment-thread-renderer"):
		if s.noThreads || len(s.rows) == 0 {
			return missing("no comment thread rendered")
		}
		return found(map[string]any{"count": len(s.rows)})
	case strings.Contains(l.CSS, "#submit-button"):
		s.submitLookups++
		disabled := s.hiddenSubmit || (s.flipAt > 0 && s.submitLookups < s.flipAt)
		name := "Comment"
		if strings.Contains(l.Root, "#replybox") {
			name = "Reply"
		}
		return found(map[string]any{"name": name, "text": name, "disabled": disabled,
			"state": map[string]any{"exists": true, "visible": true, "disabled": disabled}})
	case strings.Contains(l.CSS, "#contenteditable-root"):
		if s.noEditor {
			return missing("the editor does not exist until the box is open")
		}
		return found(map[string]any{"name": "Add a comment..."})
	}
	return found()
}

// page is the flow's handle on this scripted page. A watch URL carries no channel id, so the channel half of
// the identity assertion is empty here — which is exactly what the service does, whose channelHint is unset
// until a stage freezes a way to read it.
func (s *watchScript) page() *youtube.Page {
	diag, err := youtube.NewDiag("pachoigame", 8192, s.t.TempDir())
	if err != nil {
		s.t.Fatal(err)
	}
	p := youtube.NewPage(s, youtube.Tab{ID: "t_watch", ContainerID: 20}, diag)
	p.RequireIdentity(20, "")
	return p
}

func (s *watchScript) countEvents(prefix string) int {
	n := 0
	for _, e := range s.events {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

func (s *watchScript) hasEvent(substr string) bool {
	for _, e := range s.events {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func (s *watchScript) hasBoth(a, b string) bool {
	for _, e := range s.events {
		if strings.Contains(e, a) && strings.Contains(e, b) {
			return true
		}
	}
	return false
}

func watchJournal(t *testing.T) *youtube.Journal {
	t.Helper()
	j, err := youtube.OpenJournal(filepath.Join(t.TempDir(), "journal.json"), 3, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func slotStatus(t *testing.T, j *youtube.Journal, i int) youtube.JournalSlot {
	t.Helper()
	slots := j.All()
	if len(slots) <= i {
		t.Fatalf("journal holds %d slot(s), wanted at least %d", len(slots), i+1)
	}
	return slots[i]
}

func watchTimings() youtube.Timings {
	return youtube.Timings{Action: 250 * time.Millisecond, Wizard: 2 * time.Second, MaxScrolls: 4}
}

func TestWatchReadReturnsTheRowsThePageShows(t *testing.T) {
	s := &watchScript{t: t, rows: seedRows(), headerCount: 1}
	out, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{VideoID: watchVideo, Action: "read"}, watchTimings())
	if err != nil {
		t.Fatalf("read: %v (steps %v)", err, out.Steps)
	}
	var got struct {
		Video string `json:"video"`
		Count int    `json:"count"`
		Rows  []struct {
			ID       string `json:"id"`
			Author   string `json:"author"`
			Text     string `json:"text"`
			When     string `json:"when"`
			Absolute string `json:"absolute"`
			Nested   bool   `json:"nested"`
			IsReply  bool   `json:"isReply"`
			Path     string `json:"path"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(out.Data, &got); err != nil {
		t.Fatalf("outcome data is not the page's reply: %v (%s)", err, out.Data)
	}
	if got.Count != 2 || len(got.Rows) != 2 {
		t.Fatalf("read %d of %d rows, want both: %s", len(got.Rows), got.Count, out.Data)
	}
	if got.Rows[0].ID != seedID || got.Rows[0].Nested || got.Rows[0].IsReply {
		t.Errorf("row 0 = %+v, want the top-level comment by its lc id", got.Rows[0])
	}
	if got.Rows[0].Author != "@JangGaming" || got.Rows[0].When != "4 hours ago" || got.Rows[0].Absolute == "" {
		t.Errorf("row 0 = %+v, want author, relative and absolute time as the page gave them", got.Rows[0])
	}
	// §5: a reply's id is <parent>.<suffix>, and its row lives under #replies.
	if got.Rows[1].ID != seedReplyID || !got.Rows[1].Nested || !got.Rows[1].IsReply {
		t.Errorf("row 1 = %+v, want the reply, nested and addressed by its dotted id", got.Rows[1])
	}
	if len(out.Findings) == 0 {
		t.Error("a read recorded no locator findings, so --explain has nothing to show")
	}
	if s.countEvents("press:") != 0 {
		t.Errorf("a read pressed something: %v", s.events)
	}
}

// Trap 6 is the lazy section; an empty thread list is not an error, and a read must say so with zero rows
// rather than with a failure that looks like the page broke.
func TestWatchReadOnAVideoWithNoCommentsIsNotAnError(t *testing.T) {
	s := &watchScript{t: t, rows: nil, noThreads: true, headerCount: 0,
		bodyText: "whiteboard - YouTube 0 Comments Sort by"}
	out, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{VideoID: watchVideo, Action: "read"}, watchTimings())
	if err != nil {
		t.Fatalf("a video with no comments must read as empty, not as failed: %v", err)
	}
	var got struct {
		Count int               `json:"count"`
		Rows  []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(out.Data, &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, out.Data)
	}
	if got.Count != 0 || len(got.Rows) != 0 {
		t.Fatalf("read %d rows, want none: %s", got.Count, out.Data)
	}
	if s.scrolls == 0 {
		t.Error("the section was never scrolled into view, so nothing would have rendered (watch.md trap 6)")
	}
}

// The bug this guards against was met live: navigate returns while the page is still a 23-character
// document, and the section is lazy (watch.md trap 6), so the page has to be driven down *before* anything
// probes its structure. A flow that probes first reports a slow render as "the page carries no comments".
// A backgrounded tab is not a slow tab: the section is never built, so the flow must say that in one round
// trip and name the switch that changes it, rather than spend the scroll budget on a throttled document.
func TestWatchRefusesAHiddenTabWithoutScrolling(t *testing.T) {
	s := &watchScript{t: t, rows: seedRows(), headerCount: 1, hiddenTab: true}
	_, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{VideoID: watchVideo, Action: "read"}, watchTimings())
	if err == nil {
		t.Fatal("a hidden tab read as success")
	}
	if !strings.Contains(err.Error(), "allowFocusBoundActions") {
		t.Fatalf("error = %v, want it to name the switch that yields a visible tab", err)
	}
	if !strings.Contains(err.Error(), "backgrounded") {
		t.Fatalf("error = %v, want it to say the tab is backgrounded", err)
	}
	if s.pageScrolls != 0 {
		t.Fatalf("%d scrolls on a tab that can never render, want none", s.pageScrolls)
	}
}

func TestWatchDrivesThePageDownBeforeItProbesTheSection(t *testing.T) {
	s := &watchScript{t: t, rows: seedRows(), headerCount: 1, lazySection: true}
	out, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{VideoID: watchVideo, Action: "read"}, watchTimings())
	if err != nil {
		t.Fatalf("read: %v (steps %v)", err, out.Steps)
	}
	if s.pageScrolls == 0 {
		t.Fatalf("the page was never driven down, so the lazy section could not render: %v", s.events)
	}
	for _, e := range s.events {
		if strings.HasPrefix(e, "scroll:") {
			break
		}
		if strings.HasPrefix(e, "find:") {
			t.Fatalf("the page was probed before it was scrolled, which is the bug: %v", s.events)
		}
	}
	// And the read still happened: revealing the section is not a substitute for reading it.
	if !strings.Contains(string(out.Data), seedID) {
		t.Fatalf("a lazy section read as nothing: %s", out.Data)
	}
}

// §4 + §6: the write is proven by the submit button leaving disabled, the press happens once, and the posted
// row is read back with the id the page handed out.
func TestWatchCommentWritesWaitsForTheFlipAndSubmitsOnce(t *testing.T) {
	j := watchJournal(t)
	s := &watchScript{t: t, rows: seedRows(), headerCount: 2, flipAt: 2,
		postedRow: map[string]any{
			"id": "UgwfNOQ-X0OK0WZnVdl4AaABAg", "author": "@PhuongAnh-l1s1z",
			"text": "Comment posted by the zen-mcp youtube page flow (Sta…", "when": "0 seconds ago",
			"absolute": "Nov 3, 2026, 8:20 AM", "nested": false, "isReply": false, "path": "ytd-comments#comments",
		}}
	text := "Comment posted by the zen-mcp youtube page flow (Stage 2 test)"
	out, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{
		VideoID: watchVideo, Action: "comment", Text: text, Journal: j,
	}, watchTimings())
	if err != nil {
		t.Fatalf("comment: %v (steps %v)", err, out.Steps)
	}
	if s.submitPresses != 1 {
		t.Fatalf("submit presses = %d, want exactly 1 (a second press is the duplicate)", s.submitPresses)
	}
	// The press must come after the flip, not merely after the write: flipAt is the third read of the button.
	if s.pressedAt < s.flipAt {
		t.Fatalf("submit was pressed after %d reads of the button, but the page only accepted the text at %d (watch-run.md §4)", s.pressedAt, s.flipAt)
	}
	if !s.hasEvent("#simplebox-placeholder") {
		t.Fatalf("the placeholder was never pressed to open the box: %v", s.events)
	}
	if !s.hasEvent("type:ytd-comment-dialog-renderer >> #contenteditable-root=" + text) {
		t.Fatalf("the text did not go into the editor under the dialog root: %v", s.events)
	}
	if out.CommentID != "UgwfNOQ-X0OK0WZnVdl4AaABAg" {
		t.Fatalf("commentId = %q, want the id read back off the page", out.CommentID)
	}
	slot := slotStatus(t, j, 0)
	if slot.Kind != "comment" || slot.Status != "completed" || slot.CommentID != out.CommentID || slot.VideoID != "" {
		t.Fatalf("journal slot = %+v, want a completed comment slot bound to the posted id", slot)
	}
}

// A submit button that stays disabled means the page never took the text. Nothing may be pressed then: the
// write would be cosmetic, and the press would be the one thing that cannot be undone.
func TestWatchCommentRefusesWhileSubmitStaysDisabled(t *testing.T) {
	j := watchJournal(t)
	s := &watchScript{t: t, rows: seedRows(), headerCount: 2, hiddenSubmit: true}
	_, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{
		VideoID: watchVideo, Action: "comment", Text: "Probe text the page never accepted", Journal: j,
	}, watchTimings())
	if !errors.Is(err, youtube.ErrElement) {
		t.Fatalf("err = %v, want the element sentinel for a page that never accepted the write", err)
	}
	if s.submitPresses != 0 {
		t.Fatalf("submit presses = %d, want none while the button is disabled", s.submitPresses)
	}
	if slot := slotStatus(t, j, 0); slot.Status != "abandoned" {
		t.Fatalf("journal slot = %+v, want the claimed slot abandoned", slot)
	}
}

// §5.5: the press happened and the read-back cannot find it, which is a state no flow may resolve by pressing
// again. One press, an abandoned slot, and an error that tells the caller to look.
func TestWatchCommentPressesOnceAndReportsTransientWhenTheReadBackMisses(t *testing.T) {
	j := watchJournal(t)
	s := &watchScript{t: t, rows: seedRows(), headerCount: 2, flipAt: 1 /* postedRow nil: the row never appears */}
	_, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{
		VideoID: watchVideo, Action: "comment", Text: "A comment the page will not show again", Journal: j,
	}, watchTimings())
	if !errors.Is(err, youtube.ErrTransient) {
		t.Fatalf("err = %v, want the transient sentinel", err)
	}
	if s.submitPresses != 1 {
		t.Fatalf("submit presses = %d, want the single press the flow is allowed", s.submitPresses)
	}
	if slot := slotStatus(t, j, 0); slot.Status != "abandoned" {
		t.Fatalf("journal slot = %+v, want an abandoned slot for a post that may exist", slot)
	}
}

// A comment already on the page with that exact text is the re-run the plan refuses: a duplicate public post
// is not a retry, and nothing was pressed to find that out.
func TestWatchCommentRefusesToPostWhatIsAlreadyThere(t *testing.T) {
	j := watchJournal(t)
	rows := seedRows()
	rows[0]["text"] = "Probe from the zen-mcp youtube tool"
	s := &watchScript{t: t, rows: rows, headerCount: 2}
	_, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{
		VideoID: watchVideo, Action: "comment", Text: "Probe from the zen-mcp youtube tool", Journal: j,
	}, watchTimings())
	if !errors.Is(err, youtube.ErrUsage) {
		t.Fatalf("err = %v, want the usage refusal for a duplicate", err)
	}
	if s.clicks != 0 {
		t.Fatalf("presses = %d, want the refusal to come before any press: %v", s.clicks, s.events)
	}
	if len(j.All()) != 0 {
		t.Error("a refused comment still spent a journal slot")
	}
}

// §3: both boxes can be open at once, so a reply is addressed under its own row's #replybox and never through
// "the first #commentbox", which can post into the wrong box.
func TestWatchReplyScopesEveryLookupToTheRowsReplyBox(t *testing.T) {
	j := watchJournal(t)
	replyID := seedID + ".Abcdef1234567890Abcdef1234567890"
	s := &watchScript{t: t, rows: seedRows(), headerCount: 2, flipAt: 1,
		postedRow: map[string]any{
			"id": replyID, "author": "@PhuongAnh-l1s1z", "text": "Replied by the zen-mcp youtube page flow",
			"when": "0 seconds ago", "absolute": "Nov 3, 2026, 8:25 AM", "nested": true, "isReply": true,
			"path": "ytd-comments#comments",
		}}
	out, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{
		VideoID: watchVideo, Action: "reply", CommentID: seedID, Text: "Replied by the zen-mcp youtube page flow", Journal: j,
	}, watchTimings())
	if err != nil {
		t.Fatalf("reply: %v (steps %v)", err, out.Steps)
	}
	if !s.hasEvent(">> ytd-button-renderer#reply-button-end button") {
		t.Fatalf("the row's own Reply button was never resolved: %v", s.events)
	}
	if !s.hasBoth("nth-of-type(1)", "ytd-button-renderer#reply-button-end button") {
		t.Fatalf("the Reply button was not resolved under the addressed row: %v", s.events)
	}
	// Every part of the box has to live under that row's #replybox (§3), or the write can land in whichever
	// box happens to be first in the document.
	for _, e := range s.events {
		if !strings.Contains(e, "#contenteditable-root") && !strings.Contains(e, "#submit-button") {
			continue
		}
		if !strings.Contains(e, "#replybox") {
			t.Errorf("%q is not scoped under %s", e, "ytd-comment-reply-dialog-renderer#replybox")
		}
	}
	if s.submitPresses != 1 {
		t.Fatalf("submit presses = %d, want exactly 1", s.submitPresses)
	}
	if out.CommentID != replyID {
		t.Fatalf("commentId = %q, want the nested id the page reported (§5)", out.CommentID)
	}
	// The id lands in the field the slot's kind owns: a comment slot carries an lc id, never a video id, so a
	// later lookup for a video cannot be answered with it.
	if slot := slotStatus(t, j, 0); slot.Status != "completed" || slot.CommentID != replyID || slot.VideoID != "" {
		t.Fatalf("journal slot = %+v, want it bound to the reply's id in commentId", slot)
	}
}

// Replying to a row this page does not show is the one mistake a comment flow must never make, so it is a
// refusal before any press.
func TestWatchReplyRefusesACommentIDThatIsNotOnThePage(t *testing.T) {
	j := watchJournal(t)
	s := &watchScript{t: t, rows: seedRows(), headerCount: 2}
	_, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{
		VideoID: watchVideo, Action: "reply", CommentID: "UgnothingLikeThis0000000000000", Text: "Wrong target", Journal: j,
	}, watchTimings())
	if !errors.Is(err, youtube.ErrUsage) {
		t.Fatalf("err = %v, want the usage refusal naming the row that is not there", err)
	}
	if s.clicks != 0 {
		t.Fatalf("presses = %d, want nothing pressed for a row this page does not show: %v", s.clicks, s.events)
	}
	if len(j.All()) != 0 {
		t.Error("a refused reply spent a journal slot")
	}
}

// A dry run reads and locates, and presses nothing — not even the trigger that opens the box, because the
// probe is what the box's existence is (watch.md trap 4). It is run both with and without a journal, because
// the service hands a dry run none: nothing may be claimed for a rehearsal, and the absence of a ledger must
// not turn the rehearsal into a usage error that never reaches the page.
func TestWatchDryRunOpensNothingAndWritesNothing(t *testing.T) {
	for _, action := range []string{"comment", "reply"} {
		for _, withJournal := range []bool{true, false} {
			j := watchJournal(t)
			if !withJournal {
				j = nil
			}
			s := &watchScript{t: t, rows: seedRows(), headerCount: 2}
			in := youtube.WatchInput{VideoID: watchVideo, Action: action, Text: "Never sent", CommentID: seedID, DryRun: true, Journal: j}
			out, err := pages.Watch(context.Background(), s.page(), in, watchTimings())
			if err != nil {
				t.Fatalf("%s dry run (journal %v): %v", action, withJournal, err)
			}
			if s.clicks != 0 || s.submitPresses != 0 {
				t.Fatalf("%s: presses = %d, submit = %d, a dry run touches nothing: %v", action, s.clicks, s.submitPresses, s.events)
			}
			if s.countEvents("type:") != 0 {
				t.Fatalf("%s: a dry run wrote into an editor: %v", action, s.events)
			}
			if j != nil && len(j.All()) != 0 {
				t.Errorf("%s: a dry run claimed a journal slot", action)
			}
			if len(out.Steps) == 0 || !strings.Contains(strings.Join(out.Steps, " | "), "dry run") {
				t.Errorf("%s: the steps never say what the dry run stopped short of: %v", action, out.Steps)
			}
		}
	}
}

// A watch page with no comment section is not a comment flow, and a flow that pressed anyway would be aiming
// at nothing. The scroll guard bounds the attempt: the page is driven down a bounded number of times and then
// the section is named as never having rendered — which is a different claim from an empty comment section,
// and the wording must keep them apart.
func TestWatchRefusesAPageWithoutACommentSection(t *testing.T) {
	s := &watchScript{t: t, rows: seedRows(), missingRow: true}
	_, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{VideoID: watchVideo, Action: "comment",
		Text: "Anywhere?", Journal: watchJournal(t)}, watchTimings())
	if !errors.Is(err, youtube.ErrElement) {
		t.Fatalf("err = %v, want the element sentinel for a page with no %s", err, "ytd-comments#comments")
	}
	if s.pageScrolls == 0 {
		t.Errorf("the page was never driven down, so a lazy section would have been missed: %v", s.events)
	}
	if s.pageScrolls > 4 {
		t.Errorf("the page was driven down %d times, past the scroll guard of 4", s.pageScrolls)
	}
	if !strings.Contains(err.Error(), "never rendered a comment section") {
		t.Errorf("err = %v, want it to name the section as never rendered rather than as empty", err)
	}
	if s.clicks != 0 {
		t.Fatalf("presses = %d, want none", s.clicks)
	}
}

// The flow must not run without somewhere to record the post: an unrecorded public comment is exactly what
// the ledger exists to prevent (§4.6).
func TestWatchWriteWithoutAJournalIsRefused(t *testing.T) {
	s := &watchScript{t: t, rows: seedRows(), headerCount: 2}
	_, err := pages.Watch(context.Background(), s.page(), youtube.WatchInput{VideoID: watchVideo, Action: "comment", Text: "Unrecorded"}, watchTimings())
	if !errors.Is(err, youtube.ErrUsage) {
		t.Fatalf("err = %v, want a refusal to post with no ledger", err)
	}
	if s.clicks != 0 {
		t.Fatalf("presses = %d, want none", s.clicks)
	}
}

func filtered(events []string, prefix string) []string {
	var out []string
	for _, e := range events {
		if strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}
