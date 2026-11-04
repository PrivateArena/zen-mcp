package pages_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"zen-mcp/internal/youtube"
	"zen-mcp/internal/youtube/pages"
)

// The fixture of the 2026-11-04 probe (docs/youtube-pages/studio-details.md): the Stage 1 upload, reached at
// studio.youtube.com/video/BeCRc-x5B6E/edit, whose panel read `output (edited via page flow)`.
const (
	detailsVideo = "BeCRc-x5B6E"
	detailsTitle = "output (edited via page flow)"
)

// The addresses studio-details.md §2/§3 recorded, spelled here exactly as details.go spells them: the test and
// the flow may not share a constant, because the point of the test is that the flow aims at the probed page.
const (
	detailsTitleCSS    = "div#title-textbox-container #textbox"
	detailsDescCSS     = "#description-container #textbox"
	detailsSaveCSS     = "ytcp-button#save button" // the measured form; the display path in studio-details.md 2 gives 0 hits
	detailsOverflowCSS = "#overflow-menu-button"
	detailsConsentCSS  = "#confirm-checkbox"

	detailsAriaTitle = "Add a title that describes your video (type @ to mention a channel)"
	detailsAriaDesc  = "Tell viewers about your video (type @ to mention a channel)"

	// §3 records the menu items and the dialog's buttons by their text alone, so their addresses are derived at
	// run time from the nodes details_state.js proved. These are the paths this fake derives, and the flow must
	// press exactly those — not a sibling that merely shares letters (trap 7).
	detailsMenuRoot      = "tp-yt-overlay-menu#overflow-menu > tp-yt-paper-listbox#menu"
	detailsDeleteItemCSS = detailsMenuRoot + " > tp-yt-paper-item:nth-of-type(9)"
	detailsDialogCSS     = "tp-yt-paper-dialog#dialog"
	detailsForeverCSS    = detailsDialogCSS + " > div#dialog-content > div#buttons > paper-button.delete-forever"
	detailsDownloadCSS   = detailsDialogCSS + " > div#dialog-content > div#buttons > paper-button.download-video"
	detailsHeading       = "Permanently delete this video?"
)

// detailsMenu is §3's list, in the order the probe read it, so `Delete` sits among eleven shown items and an
// item matched by substring rather than exactly would land on Details, Download or Promote.
var detailsMenu = []string{"Details", "Analytics", "Editor", "Comments", "Subtitles", "Claims", "Clips & Shorts", "Download", "Delete", "Promote", "Brainstorm video ideas"}

func menuItemCSS(i int) string {
	return fmt.Sprintf("%s > tp-yt-paper-item:nth-of-type(%d)", detailsMenuRoot, i+1)
}

// detailsScript is a scripted Studio details panel in the idiom of watchScript: it answers details_state.js,
// lookup.js, fill.js and click.js the way the live probe found them, and it only ever moves on the presses the
// flow really made. So a flow that presses Save twice is caught, one that reads a closed dialog as an open one is
// caught, and one that confirms a delete the page never tied to this video is caught.
type detailsScript struct {
	t *testing.T

	navigates int
	pressLog  []string // the address of every bridge press, in order
	types     []string // every fill.js write, as "css=text"
	events    []string

	// what the panel's editors hold — the target of fill.js and the text lookup.js reports
	title string
	desc  string

	// the page's own state, which only a press changes
	menuOpen   bool
	dialogOpen bool
	checked    bool
	gateOpen   bool

	// knobs, all zero-valued for the honest page
	failSave         bool // the Save press reports a failure, and looks exactly like one in flight (§5.2)
	mangleWrite      bool // the editor ends up holding something other than what was typed
	readCap          int  // the page reports at most this many characters of a field, as maxDiagBytes does
	gateNeverOpens   bool // the consent click lands and Delete forever stays disabled (§4 trap 1)
	dialogNamesOther bool // the dialog that comes up names some other video
	hiddenDialog     bool // only a closed dialog is in the DOM (§4 trap 4)
	menuStaysClosed  bool // the Options press left the menu closed; its items are still in the DOM, hidden
	hiddenNotice     bool // a closed snackbar is still in the DOM (§4 trap 4)
	noNotice         bool // the snackbar never arrives at all
	saveDisabled     bool // the form is already clean, so there is nothing to commit
	// snackAfter and cleanAfter are how many extra reads the page makes the flow wait for before each of the two
	// save confirmations appears (§2 requires both).
	snackAfter int
	cleanAfter int

	probeArgsChecked bool
	stateReads       int
	savePresses      int
	saveFinds        int
	findAtSavePress  int
	postSaveReads    int
}

func (s *detailsScript) url() string {
	return pages.DetailsURL + detailsVideo + "/edit"
}

func (s *detailsScript) Call(ctx context.Context, action string, params map[string]any) (map[string]any, error) {
	switch action {
	case "navigate":
		s.navigates++
		// §1: status 200, title `Video details - YouTube Studio`, containerId 20.
		return ok(map[string]any{"id": "t_details", "containerId": 20, "status": 200,
			"title": "Video details - YouTube Studio", "url": s.url()}), nil
	case "click":
		return s.click(params)
	case "get_text":
		// The second and third R4 channels, read-only; the flow only records them.
		return ok(map[string]any{"el": map[string]any{"tag": "div", "visible": true}, "text": "Save"}), nil
	case "screenshot":
		// Evidence is only written when the service set an evidence dir, which these runs have none of.
		return ok(nil), nil
	case "close_tab":
		return ok(nil), nil
	case "web_eval":
		return s.eval(params)
	}
	return ok(nil), nil
}

// click is the bridge channel — the only one that can tick the consent checkbox (§4 trap 1) — and every state
// change on this page is caused by a press, never by a read.
func (s *detailsScript) click(params map[string]any) (map[string]any, error) {
	sel := describe(params)
	s.pressLog = append(s.pressLog, "press:"+sel)
	switch {
	case strings.Contains(sel, "#save"):
		s.savePresses++
		s.findAtSavePress = s.saveFinds
		if s.failSave {
			// A transport failure, not a not-found: the press may still have landed, and that is why it is never
			// repeated (§5.2). A not-found here would fall back to the page channel, which pressed nothing.
			return fail("Timeout waiting for the bridge to answer"), nil
		}
	case strings.Contains(sel, detailsOverflowCSS):
		s.menuOpen = !s.menuStaysClosed
	case strings.Contains(sel, "tp-yt-paper-item"):
		if sel == detailsDeleteItemCSS {
			s.menuOpen = false
			s.dialogOpen = !s.hiddenDialog // a closed dialog stays in the DOM either way (trap 4)
		}
	case strings.Contains(sel, detailsConsentCSS):
		s.checked = true
		s.gateOpen = !s.gateNeverOpens
	case strings.Contains(sel, "paper-button"):
		if sel == detailsForeverCSS {
			s.dialogOpen = false // the page leaves the details state (§5.4)
		}
	}
	return ok(map[string]any{"el": map[string]any{"tag": "button", "text": sel}}), nil
}

func (s *detailsScript) eval(params map[string]any) (map[string]any, error) {
	code, _ := params["code"].(string)
	switch {
	case strings.Contains(code, "state probe for the Studio details panel"): // details_state.js
		s.checkProbeArgs(code)
		return ok(map[string]any{"result": s.state()}), nil
	case strings.Contains(code, "deep-query probe"): // lookup.js
		return ok(map[string]any{"result": s.lookup(code)}), nil
	case strings.Contains(code, "Text entry with proof"): // fill.js
		return ok(map[string]any{"result": s.fill(code)}), nil
	case strings.Contains(code, "cheap verification channel"): // body_text.js
		text := s.bodyText()
		return ok(map[string]any{"result": map[string]any{
			"url": s.url(), "title": "Video details - YouTube Studio", "text": text, "len": len(text),
		}}), nil
	case strings.Contains(code, "DOM-level click"): // click.js
		// Nothing here is reachable by the page channel that the bridge could not already see; a flow that
		// needed it would be pressing something the probe never recorded.
		s.events = append(s.events, "dom-click:"+describeFromPayload(code))
		return ok(map[string]any{"result": map[string]any{"clicked": false, "count": 0, "text": ""}}), nil
	}
	s.t.Fatalf("unrecognised web_eval payload:\n%s", code)
	return nil, nil
}

// state is details_state.js's reply: the menu, the dialogs, the open dialog's buttons, the notices and the
// consent checkbox — each with the `shown` flag that is the flow's whole defence against trap 4.
func (s *detailsScript) state() map[string]any {
	s.stateReads++
	if s.savePresses > 0 {
		s.postSaveReads++
	}
	out := map[string]any{"url": s.url(), "title": "Video details - YouTube Studio"}

	dialogs := []map[string]any{}
	switch {
	case s.dialogOpen:
		dialogs = append(dialogs, detailsNode("tp-yt-paper-dialog", s.dialogText(), detailsDialogCSS, true, false, false))
	case s.hiddenDialog:
		// §4 trap 4, verbatim: after a cancel the heading was still reachable by a text search. Present,
		// unreadable-by-geometry, and worth nothing to a flow that filters on shown.
		dialogs = append(dialogs, detailsNode("tp-yt-paper-dialog", s.dialogText(), detailsDialogCSS, false, false, false))
	}
	out["dialogs"] = dialogs

	buttons := []map[string]any{}
	if s.dialogOpen { // only an open dialog's controls are listed, as the probe does
		buttons = append(buttons,
			detailsNode("paper-button", "Retry", detailsDialogCSS+" > div#dialog-content > div#buttons > paper-button.retry", false, false, false),
			detailsNode("paper-button", "Download video", detailsDownloadCSS, true, false, false),
			detailsNode("paper-button", "Cancel", detailsDialogCSS+" > div#dialog-content > div#buttons > paper-button.cancel", true, false, false),
			detailsNode("paper-button", "Delete forever", detailsForeverCSS, true, !s.gateOpen, false))
	}
	out["buttons"] = buttons

	items := []map[string]any{}
	for i, name := range detailsMenu {
		if s.menuOpen {
			items = append(items, detailsNode("tp-yt-paper-item", name, menuItemCSS(i), true, false, false))
		} else if s.menuStaysClosed {
			// A closed paper menu keeps every item in the DOM, reachable by a query and worth nothing.
			items = append(items, detailsNode("tp-yt-paper-item", name, menuItemCSS(i), false, false, false))
		}
	}
	out["items"] = items

	notices := []map[string]any{}
	up := s.savePresses > 0 && !s.noNotice && s.postSaveReads > s.snackAfter
	if up {
		notices = append(notices, detailsNode("div", "Changes saved", "[class*='snackbar']", true, false, false))
	} else if s.hiddenNotice {
		notices = append(notices, detailsNode("div", "Changes saved", "[class*='snackbar']", false, false, false))
	}
	out["notices"] = notices

	checkbox := detailsNode("ytcp-checkbox-lit", "", "ytcp-checkbox-lit#confirm-checkbox", true, false, s.checked)
	out["checkbox"] = checkbox
	return out
}

// dialogText is §3's dialog copy, with the video's identity in the middle of it — which is the whole reason the
// flow can tell this video's dialog from another one's.
func (s *detailsScript) dialogText() string {
	who := detailsTitle
	if s.dialogNamesOther { // the page is showing a delete dialog, but not for this video
		who = "a different video (deleted by mistake)"
	}
	return detailsHeading + " 0:08 " + who + " Uploaded Nov 2, 2026 0 views I understand that deleting this " +
		"video is permanent and I will lose all access to audience and video analytics Download video Cancel Delete forever"
}

func detailsNode(tag, text, path string, shown, disabled, checked bool) map[string]any {
	return map[string]any{"tag": tag, "text": text, "path": path, "shown": shown, "disabled": disabled, "checked": checked}
}

// checkProbeArgs is the one place the flow says out loud which nodes it is asking the page about, and they
// have to be the ones §3 recorded: a dialog read aimed at anything else cannot be trusted to see a dialog.
func (s *detailsScript) checkProbeArgs(code string) {
	if s.probeArgsChecked {
		return
	}
	s.probeArgsChecked = true
	var a struct {
		Dialog   string   `json:"dialog"`
		Checkbox string   `json:"checkbox"`
		MenuItem string   `json:"menuItem"`
		Notices  []string `json:"notices"`
	}
	if err := json.Unmarshal(payload(code), &a); err != nil {
		s.t.Fatalf("details_state payload: %v", err)
	}
	if !strings.Contains(a.Dialog, "tp-yt-paper-dialog") || !strings.Contains(a.Checkbox, "#confirm-checkbox") ||
		!strings.Contains(a.MenuItem, "tp-yt-paper-item") || len(a.Notices) == 0 {
		s.t.Errorf("the dialog probe was aimed at %q / %q / %q / %v, not the addresses studio-details.md §2–§3 recorded",
			a.Dialog, a.Checkbox, a.MenuItem, a.Notices)
	}
}

func (s *detailsScript) bodyText() string {
	return "Video details Details Analytics Editor Comments Subtitles Claims Clips & Shorts " + s.title +
		" 0:08 Uploaded Nov 2, 2026 Visibility Scheduled Save"
}

// lookup answers lookup.js for the addresses this page has: the two editors, the Save button, the overflow
// button, the consent checkbox, and the menu item and dialog buttons whose paths the probe derived.
func (s *detailsScript) lookup(code string) map[string]any {
	var args struct {
		L struct {
			CSS  string `json:"css"`
			Root string `json:"root"`
		} `json:"l"`
	}
	if err := json.Unmarshal(payload(code), &args); err != nil {
		s.t.Fatalf("lookup payload: %v", err)
	}
	css := args.L.CSS
	if args.L.Root != "" {
		// A scoped address is one lookup to the page, so the fake resolves the pair the same way: root first,
		// leaf second, exactly as the deep query walks it.
		css = args.L.Root + " " + css
	}
	s.events = append(s.events, "find:"+css)
	found := func(text, name string, disabled bool, checked *bool) map[string]any {
		st := map[string]any{"exists": true, "visible": true, "disabled": disabled}
		if checked != nil {
			st["checked"] = *checked
		}
		return map[string]any{"ok": true, "count": 1, "css": css, "tag": "div", "text": s.cap(text),
			"name": name, "disabled": disabled, "state": st}
	}
	missing := func(reason string) map[string]any {
		return map[string]any{"ok": false, "count": 0, "reason": reason,
			"state": map[string]any{"exists": false, "visible": false, "disabled": false}}
	}
	switch {
	case strings.Contains(css, "#title-textbox-container"):
		return found(s.title, detailsAriaTitle, false, nil)
	case strings.Contains(css, "#description-container"):
		return found(s.desc, detailsAriaDesc, false, nil)
	case strings.Contains(css, "#save"):
		s.saveFinds++
		// §2: enabled while the form is dirty, disabled once the page has taken the change. After a press it
		// takes cleanAfter further reads to flip, so a flow that looks once and trusts it is caught.
		clean := s.saveDisabled || (s.savePresses > 0 && s.saveFinds-s.findAtSavePress > s.cleanAfter)
		return found("Save", "Save", clean, nil)
	case strings.Contains(css, detailsOverflowCSS):
		return found("", "Options", false, nil)
	case strings.Contains(css, detailsConsentCSS):
		return found("", "", false, &s.checked)
	case strings.Contains(css, "tp-yt-paper-item"):
		for i, name := range detailsMenu {
			if menuItemCSS(i) == css {
				return found(name, "", false, nil)
			}
		}
		return missing("no such menu item on this page")
	case strings.Contains(css, "paper-button"):
		switch css {
		case detailsForeverCSS:
			return found("Delete forever", "", !s.gateOpen, nil)
		case detailsDownloadCSS:
			return found("Download video", "", false, nil)
		}
		return missing("the dialog does not show that button")
	}
	return missing("nothing recorded at " + css)
}

// fill is the page-channel write: focus, selectNodeContents, insertText, input (§2). It reports what the editor
// holds afterwards, which is the only reason a flow can tell a write from a wish.
func (s *detailsScript) fill(code string) map[string]any {
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
	css := args.L.CSS
	s.types = append(s.types, css+"="+args.Text)
	got := args.Text
	if s.mangleWrite {
		got = "Studio kept the old text"
	}
	switch {
	case strings.Contains(css, "#title-textbox-container"):
		s.title = got
	case strings.Contains(css, "#description-container"):
		s.desc = got
	default:
		s.t.Fatalf("fill aimed at %s, which is not a details field", css)
	}
	return map[string]any{"ok": true, "method": "insertText", "text": got}
}

// cap imitates youtube.maxDiagBytes: the page hands back at most so many characters, which is a short read and
// not a lost write — and has to be reported as the difference.
func (s *detailsScript) cap(text string) string {
	if s.readCap <= 0 {
		return text
	}
	r := []rune(text)
	if len(r) <= s.readCap {
		return text
	}
	return string(r[:s.readCap])
}

func (s *detailsScript) page() *youtube.Page {
	diag, err := youtube.NewDiag("pachoigame", 8192, s.t.TempDir())
	if err != nil {
		s.t.Fatal(err)
	}
	// The Studio URL carries the channel identity only in the container, so the channel half of the assertion is
	// empty here — exactly what Service.Details hands the page.
	p := youtube.NewPage(s, youtube.Tab{ID: "t_details", ContainerID: 20}, diag)
	p.RequireIdentity(20, "")
	return p
}

func (s *detailsScript) pressCount(substr string) int {
	n := 0
	for _, e := range s.pressLog {
		if strings.Contains(e, substr) {
			n++
		}
	}
	return n
}

func (s *detailsScript) presses() string {
	return strings.Join(s.pressLog, ", ")
}

func detailsTimings(action time.Duration) youtube.Timings {
	return youtube.Timings{Action: action, Wizard: 5 * time.Second, MaxScrolls: 4}
}

func newDetailsScript(t *testing.T) *detailsScript {
	t.Helper()
	return &detailsScript{t: t, title: detailsTitle, desc: "Probe run for the zen-mcp youtube tool (Stage 4 up)"}
}

// §5.2 in one run: both fields are written, each is read back, Save is pressed exactly once, and the flow waits
// for BOTH signals §2 records — the `Changes saved` snackbar and the Save button going disabled. The page makes
// them arrive at different times, so a flow that stopped at the first one it saw would report here.
func TestDetailsEditWritesBothFieldsAndSavesOnce(t *testing.T) {
	j := watchJournal(t)
	newTitle := "output (edited via page flow) — Stage 4"
	newDesc := "Probe run for the zen-mcp youtube tool (Stage 4 edit).\nSecond line: newlines round-trip."
	s := newDetailsScript(t)
	s.snackAfter, s.cleanAfter = 1, 2 // the snackbar first, the clean form two reads later
	out, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "edit", Title: newTitle, Description: newDesc, Journal: j,
	}, detailsTimings(3*time.Second))
	if err != nil {
		t.Fatalf("edit: %v (steps %v)", err, out.Steps)
	}
	if s.navigates == 0 || s.pressCount("#save") != 1 {
		t.Fatalf("save presses = %d after %d navigate(s), want one press of the panel's own Save: %v",
			s.pressCount("#save"), s.navigates, s.presses())
	}
	if !s.hasType(detailsTitleCSS, newTitle) || !s.hasType(detailsDescCSS, newDesc) {
		t.Errorf("the writes did not go into the two editors: %v", s.types)
	}
	if s.title != newTitle || s.desc != newDesc {
		t.Errorf("the page ends up reading %q / %q", s.title, s.desc)
	}
	// Both signals, not the first one: the snackbar landed on the second state read, the disabled flip on the
	// third read of the button, and the run had to watch both.
	if s.postSaveReads < 2 || s.saveFinds-s.findAtSavePress < 3 {
		t.Errorf("the run stopped after %d state read(s) and %d Save read(s), so it cannot have seen both signals",
			s.postSaveReads, s.saveFinds-s.findAtSavePress)
	}
	if len(s.pressLog) != 1 {
		t.Errorf("the run pressed %d things (%s), want the one Save press", len(s.pressLog), s.presses())
	}
	if out.VideoID != detailsVideo || out.URL != s.url() {
		t.Errorf("outcome = %+v, want the video the URL named", out)
	}
	joined := strings.Join(out.Steps, " | ")
	if !strings.Contains(joined, "Changes saved") || !strings.Contains(joined, "disabled") {
		t.Errorf("the steps never report both confirmations: %v", out.Steps)
	}
	slot := slotStatus(t, j, 0)
	if slot.Kind != "edit" || slot.Status != "completed" || slot.VideoID != detailsVideo {
		t.Errorf("journal slot = %+v, want a completed edit slot bound to the video", slot)
	}
	if s.pressCount("tp-yt-paper-item") != 0 || s.pressCount(detailsOverflowCSS) != 0 {
		t.Errorf("an edit opened the overflow menu: %s", s.presses())
	}
}

// §4 trap 5: an edit the form never took is invisible everywhere except in the Save button's flag, so a field
// that reads back as something else is a refusal, and Save is never pressed.
func TestDetailsEditRefusesWhenTheReadBackDiffers(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	s.mangleWrite = true
	_, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "edit", Title: "A title Studio will not accept", Journal: j,
	}, detailsTimings(400*time.Millisecond))
	if !errors.Is(err, youtube.ErrElement) {
		t.Fatalf("err = %v, want the element sentinel for a write the page did not take", err)
	}
	if !strings.Contains(err.Error(), "was NOT pressed") {
		t.Errorf("err = %v, want it to say the button was not pressed", err)
	}
	if s.pressCount("#save") != 0 {
		t.Fatalf("save presses = %d, want none after a read-back mismatch: %s", s.pressCount("#save"), s.presses())
	}
	if s.title == "A title Studio will not accept" {
		t.Errorf("the page did take the write, so the case is not testing what it means to")
	}
	if len(j.All()) != 0 {
		t.Errorf("a run that pressed nothing spent a journal slot: %+v", j.All())
	}
}

// A capped read is the page's own truncation, not a lost write: the run says the tail is unverified rather than
// passing off a short read as confirmation, and it still commits what the page did take.
func TestDetailsEditReportsACappedReadAsUnverified(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	s.readCap = 12
	out, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "edit", Title: "output (edited via page flow) — Stage 4", Journal: j,
	}, detailsTimings(2*time.Second))
	if err != nil {
		t.Fatalf("a short read must not be reported as a lost write: %v", err)
	}
	if len(out.Warnings) == 0 || !strings.Contains(strings.Join(out.Warnings, " | "), "unverified") {
		t.Errorf("warnings = %v, want one saying the tail of the read is unverified", out.Warnings)
	}
	if s.pressCount("#save") != 1 {
		t.Errorf("save presses = %d, want the single press, on %s", s.pressCount("#save"), s.presses())
	}
}

// §5.3: with nothing to write, the enabled Save would be a press aimed at nothing. Refused before the page is
// opened, so the refusal cannot even be a slow panel.
func TestDetailsEditWithNoFieldsOpensNoPage(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	_, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "edit", Title: "   ", Description: "  ", Journal: j,
	}, detailsTimings(400*time.Millisecond))
	if !errors.Is(err, youtube.ErrUsage) {
		t.Fatalf("err = %v, want the usage refusal for an edit with nothing to write", err)
	}
	if s.navigates != 0 || len(s.pressLog) != 0 || len(s.types) != 0 {
		t.Fatalf("the page was touched: %d navigate(s), %d press(es), %d write(s)", s.navigates, len(s.pressLog), len(s.types))
	}
	if len(j.All()) != 0 {
		t.Error("a refused edit spent a journal slot")
	}
}

// §5.2: "a press that is in flight looks the same as one that failed". So the ambiguous Save is reported
// transient, is not repeated, and its slot is abandoned rather than completed.
func TestDetailsEditReportsAnAmbiguousSaveAsTransient(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	s.failSave = true
	_, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "edit", Title: "A title whose save may or may not have landed", Journal: j,
	}, detailsTimings(400*time.Millisecond))
	if !errors.Is(err, youtube.ErrTransient) {
		t.Fatalf("err = %v, want the transient sentinel for a press that may have landed", err)
	}
	if s.pressCount("#save") != 1 {
		t.Fatalf("save presses = %d, want exactly one whatever it reported: %s", s.pressCount("#save"), s.presses())
	}
	slot := slotStatus(t, j, 0)
	if slot.Status != "abandoned" || slot.VideoID != detailsVideo {
		t.Fatalf("journal slot = %+v, want the abandoned slot bound to the video it may have changed", slot)
	}
}

// The snackbar the flow waits for is the one the page is SHOWING: a closed `Changes saved` still in the DOM is
// not a confirmation, and the commit it would have reported is unconfirmed.
// The button going clean is the durable signal; the snackbar expires. Measured live 2026-11-04: the save had
// landed and the snackbar was already gone, so demanding both reported a real edit as an unconfirmed commit.
// A notice the page is not showing must not count as confirmation either, and is not needed when the flip is
// there.
func TestDetailsEditConfirmsOnTheFlipWhenTheSnackbarIsMissed(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	s.hiddenNotice = true
	s.noNotice = true
	s.cleanAfter = 0
	out, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "edit", Title: "A title the page confirmed by going clean", Journal: j,
	}, detailsTimings(300*time.Millisecond))
	if err != nil {
		t.Fatalf("a save the page reported clean failed: %v", err)
	}
	if s.pressCount("#save") != 1 {
		t.Fatalf("save presses = %d, want exactly 1", s.pressCount("#save"))
	}
	if slot := slotStatus(t, j, 0); slot.Status != "completed" {
		t.Fatalf("journal slot = %+v, want the confirmed save completed", slot)
	}
	if !strings.Contains(strings.Join(out.Warnings, " | "), "snackbar") {
		t.Fatalf("warnings = %v, want the missed snackbar said out loud", out.Warnings)
	}
}

// The reverse case keeps the safety property: the press happened and the page still holds the change.
func TestDetailsEditReportsASaveThePageNeverTook(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	s.cleanAfter = 1 << 20 // the button stays enabled for the whole budget: nothing was committed
	s.noNotice = true
	_, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "edit", Title: "A title the page refused to take", Journal: j,
	}, detailsTimings(300*time.Millisecond))
	if !errors.Is(err, youtube.ErrTransient) {
		t.Fatalf("err = %v, want an unconfirmed commit reported transient", err)
	}
	if s.pressCount("#save") != 1 {
		t.Fatalf("save presses = %d, want the one press it was allowed, never a second", s.pressCount("#save"))
	}
	if slot := slotStatus(t, j, 0); slot.Status != "abandoned" {
		t.Fatalf("journal slot = %+v, want the unconfirmed save abandoned", slot)
	}
}

// §5.3 the other way round: the fields already read as requested, so the page holds no pending change and Save
// is disabled. Pressing a disabled control is a guess about what it would do, so the run says so and stops —
// with nothing claimed, because nothing changed.
func TestDetailsEditMakesNoPressWhenTheFormIsAlreadyClean(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	s.saveDisabled = true
	out, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "edit", Title: detailsTitle, Journal: j,
	}, detailsTimings(400*time.Millisecond))
	if err != nil {
		t.Fatalf("an edit that was already in place is not a failure: %v", err)
	}
	if len(s.pressLog) != 0 {
		t.Fatalf("the run pressed a disabled button: %s", s.presses())
	}
	if !strings.Contains(strings.Join(out.Steps, " | "), "no pending change") {
		t.Errorf("the steps never say the page held nothing to commit: %v", out.Steps)
	}
	if len(j.All()) != 0 {
		t.Errorf("a run that changed nothing spent a journal slot: %+v", j.All())
	}
}

// §5.4: anything short of the confirmation parameter stops before the overflow menu — here, before the browser
// is touched at all.
func TestDetailsDeleteWithoutConfirmationPressesNothing(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	_, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "delete", Confirmed: false, Journal: j,
	}, detailsTimings(400*time.Millisecond))
	if !errors.Is(err, youtube.ErrUsage) {
		t.Fatalf("err = %v, want the usage refusal for an unconfirmed delete", err)
	}
	if !strings.Contains(err.Error(), "yes_i_understand") {
		t.Errorf("err = %v, want it to name the parameter the caller owes", err)
	}
	if s.navigates != 0 || len(s.pressLog) != 0 {
		t.Fatalf("the page was opened anyway: %d navigate(s), presses %q", s.navigates, s.presses())
	}
	if len(j.All()) != 0 {
		t.Error("a refused delete spent a journal slot")
	}
}

// The dialog is the only place the page says which video is about to be destroyed, and it has to name this
// one. Nothing is pressed after that check fails — no checkbox, no Delete forever.
func TestDetailsDeleteRefusesADialogThatDoesNotNameTheVideo(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	s.dialogNamesOther = true // the dialog's copy names some other video
	_, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "delete", Confirmed: true, Journal: j,
	}, detailsTimings(400*time.Millisecond))
	if !errors.Is(err, youtube.ErrIdentity) {
		t.Fatalf("err = %v, want the identity sentinel for a dialog naming another video", err)
	}
	if !strings.Contains(err.Error(), "nothing was deleted") {
		t.Errorf("err = %v, want it to say nothing was deleted", err)
	}
	if s.pressCount(detailsConsentCSS) != 0 || s.pressCount("paper-button") != 0 {
		t.Fatalf("presses after the refusal: %s", s.presses())
	}
	slot := slotStatus(t, j, 0)
	if slot.Kind != "delete" || slot.Status != "abandoned" || slot.VideoID != detailsVideo {
		t.Errorf("journal slot = %+v, want the abandoned delete slot bound to the video", slot)
	}
}

// §4 traps 1 and 2: only a trusted click ticks the checkbox, and Delete forever becoming enabled is the page's
// own proof that it did. While it stays disabled, nothing after the checkbox is pressed.
func TestDetailsDeleteRefusesWhileDeleteForeverStaysDisabled(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	s.gateNeverOpens = true
	_, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "delete", Confirmed: true, Journal: j,
	}, detailsTimings(300*time.Millisecond))
	if !errors.Is(err, youtube.ErrElement) {
		t.Fatalf("err = %v, want the element sentinel for a gate that never opened", err)
	}
	if !strings.Contains(err.Error(), "stays disabled=true") {
		t.Errorf("err = %v, want it to report the button it found and its state", err)
	}
	if s.pressCount(detailsConsentCSS) != 1 {
		t.Fatalf("checkbox presses = %d, want the one trusted press the gate is read after", s.pressCount(detailsConsentCSS))
	}
	if s.pressCount("paper-button") != 0 {
		t.Fatalf("a dialog button was pressed while the gate was closed: %s", s.presses())
	}
	if slot := slotStatus(t, j, 0); slot.Status != "abandoned" {
		t.Errorf("journal slot = %+v, want it abandoned", slot)
	}
}

// The happy delete: §3's chain walked, one press of Delete forever, and the slot bound to the video it removed.
// Trap 7 is asserted by the presses that did NOT happen: the menu item is `Delete` and nothing else, and the
// dialog's other button is `Download video`.
func TestDetailsDeletePressesDeleteForeverOnce(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	out, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "delete", Confirmed: true, Journal: j,
	}, detailsTimings(2*time.Second))
	if err != nil {
		t.Fatalf("delete: %v (steps %v)", err, out.Steps)
	}
	if got := s.pressCount(detailsForeverCSS); got != 1 {
		t.Fatalf("Delete forever presses = %d, want exactly 1: %s", got, s.presses())
	}
	if got := s.pressCount(detailsDeleteItemCSS); got != 1 {
		t.Fatalf("the menu item presses = %d, want one press of the item reading exactly %q: %s", got, "Delete", s.presses())
	}
	for _, wrong := range []string{detailsDownloadCSS, menuItemCSS(0), menuItemCSS(7), "Cancel"} {
		if s.pressCount(wrong) != 0 {
			t.Errorf("%s was pressed; §3's chain names Delete and Delete forever only", wrong)
		}
	}
	if s.pressCount(detailsOverflowCSS) != 1 || s.pressCount(detailsConsentCSS) != 1 {
		t.Fatalf("the chain was pressed the wrong number of times: %s", s.presses())
	}
	if s.dialogOpen {
		t.Error("the page still shows the dialog, so the delete is unconfirmed")
	}
	joined := strings.Join(out.Steps, " | ")
	for _, want := range []string{"Delete forever", "names this video", "dialog is gone"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the steps never say %q: %v", want, out.Steps)
		}
	}
	slot := slotStatus(t, j, 0)
	if slot.Kind != "delete" || slot.Status != "completed" || slot.VideoID != detailsVideo {
		t.Fatalf("journal slot = %+v, want a completed delete slot bound to the video", slot)
	}
}

// §4 trap 4, the dangerous direction: a `details_state.js` reply whose dialog node is present but not shown is
// not an open dialog. The flow must stop at the menu item rather than press a checkbox it believes it can see.
func TestDetailsDeleteIgnoresAStaleHiddenDialog(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	s.hiddenDialog = true
	_, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "delete", Confirmed: true, Journal: j,
	}, detailsTimings(300*time.Millisecond))
	if !errors.Is(err, youtube.ErrElement) {
		t.Fatalf("err = %v, want the element sentinel for a dialog the page is not showing", err)
	}
	if !strings.Contains(err.Error(), "never brought up a visible") {
		t.Errorf("err = %v, want it to say no visible dialog arrived", err)
	}
	if !strings.Contains(err.Error(), "0 dialog(s) shown of 1") {
		t.Errorf("err = %v, want the refusal to report the hidden dialog it ignored", err)
	}
	if s.pressCount(detailsConsentCSS) != 0 || s.pressCount("paper-button") != 0 {
		t.Fatalf("something was pressed against a closed dialog: %s", s.presses())
	}
	if slot := slotStatus(t, j, 0); slot.Status != "abandoned" {
		t.Errorf("journal slot = %+v, want it abandoned", slot)
	}
}

// §5.5's rule read the other way: a menu the page never opened still has its items in the DOM, and `Delete` is
// one of them. The exact-text match is not enough on its own — the shown flag is what says the page is offering
// that item — so a flow that searches the nodes alone would press into a closed menu and call it the delete.
func TestDetailsDeleteRefusesAMenuThatNeverOpened(t *testing.T) {
	j := watchJournal(t)
	s := newDetailsScript(t)
	s.menuStaysClosed = true
	_, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "delete", Confirmed: true, Journal: j,
	}, detailsTimings(300*time.Millisecond))
	if !errors.Is(err, youtube.ErrElement) {
		t.Fatalf("err = %v, want the element sentinel for a menu the page is not showing", err)
	}
	if !strings.Contains(err.Error(), `never showed a menu item reading exactly "Delete"`) {
		t.Errorf("err = %v, want it to name the item it never saw", err)
	}
	if s.pressCount("tp-yt-paper-item") != 0 || s.pressCount(detailsConsentCSS) != 0 {
		t.Fatalf("a closed menu was pressed anyway: %s", s.presses())
	}
	if slot := slotStatus(t, j, 0); slot.Status != "abandoned" {
		t.Errorf("journal slot = %+v, want it abandoned", slot)
	}
}

// §5.6: a dry run reads the panel and finds the Options button, and presses nothing — it does not open the
// delete dialog, does not press Save, and claims nothing, whether or not the caller handed it a ledger.
func TestDetailsDryRunPressesNothingAndClaimsNothing(t *testing.T) {
	for _, action := range []string{"edit", "delete"} {
		for _, withJournal := range []bool{true, false} {
			j := watchJournal(t)
			if !withJournal {
				j = nil
			}
			s := newDetailsScript(t)
			out, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
				VideoID: detailsVideo, Action: action, Title: "Never written", Description: "Never written either",
				DryRun: true, Confirmed: true, Journal: j,
			}, detailsTimings(400*time.Millisecond))
			if err != nil {
				t.Fatalf("%s dry run (journal %v): %v", action, withJournal, err)
			}
			if len(s.pressLog) != 0 {
				t.Fatalf("%s: a dry run pressed something: %s", action, s.presses())
			}
			if len(s.types) != 0 {
				t.Fatalf("%s: a dry run wrote into an editor: %v", action, s.types)
			}
			if s.menuOpen || s.dialogOpen {
				t.Fatalf("%s: a dry run opened the menu or the dialog", action)
			}
			if j != nil && len(j.All()) != 0 {
				t.Errorf("%s: a dry run claimed a journal slot", action)
			}
			joined := strings.Join(out.Steps, " | ")
			if !strings.Contains(joined, "dry run") || !strings.Contains(joined, "NOT pressed") && !strings.Contains(joined, "nothing was typed") {
				t.Errorf("%s: the steps never say what the dry run stopped short of: %v", action, out.Steps)
			}
		}
	}
}

// Both actions change the channel, so neither runs without somewhere to record the change (§4.6).
func TestDetailsWriteWithoutALedgerIsRefused(t *testing.T) {
	for _, c := range []youtube.DetailsInput{
		{VideoID: detailsVideo, Action: "edit", Title: "Unrecorded"},
		{VideoID: detailsVideo, Action: "delete", Confirmed: true},
	} {
		s := newDetailsScript(t)
		_, err := pages.Details(context.Background(), s.page(), c, detailsTimings(400*time.Millisecond))
		if !errors.Is(err, youtube.ErrUsage) {
			t.Errorf("%s without a journal: err = %v, want the refusal to run a mutation unrecorded", c.Action, err)
		}
		if s.navigates != 0 || len(s.pressLog) != 0 {
			t.Errorf("%s: the page was touched: %d navigate(s), %s", c.Action, s.navigates, s.presses())
		}
	}
}

// The page does not accept an action the service did not name: the flow's own vocabulary is closed too, so a
// mis-wired caller cannot reach the delete chain by spelling the edit action wrongly.
func TestDetailsRefusesAnUnknownAction(t *testing.T) {
	s := newDetailsScript(t)
	_, err := pages.Details(context.Background(), s.page(), youtube.DetailsInput{
		VideoID: detailsVideo, Action: "list_videos", Title: "Not a details action", Journal: watchJournal(t),
	}, detailsTimings(400*time.Millisecond))
	if !errors.Is(err, youtube.ErrUsage) {
		t.Fatalf("err = %v, want the usage refusal", err)
	}
	if s.navigates != 0 || len(s.pressLog) != 0 {
		t.Errorf("an unknown action touched the page: %s", s.presses())
	}
}

func (s *detailsScript) hasType(css, text string) bool {
	for _, e := range s.types {
		if e == css+"="+text {
			return true
		}
	}
	return false
}

// describeFromPayload is describe for a web_eval payload: which address a page-channel press was aimed at.
func describeFromPayload(code string) string {
	var args struct {
		L struct {
			CSS string `json:"css"`
		} `json:"l"`
	}
	if err := json.Unmarshal(payload(code), &args); err != nil {
		return "?"
	}
	return args.L.CSS
}
