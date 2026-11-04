package pages

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"zen-mcp/internal/youtube"
)

// Frozen locators for the Studio details panel. Every string here was copied out of
// docs/youtube-pages/studio-details.md — the live probe of 2026-11-04 on studio.youtube.com/video/BeCRc-x5B6E/edit
// (§1–§3) — and nothing else is used. Where the probe recorded a control by its text alone (the dialog's
// buttons and the menu's items, §3) the flow reads that text off a node the page is showing and presses the
// address the page derived for it, rather than inventing a selector; where a control was never recorded at all
// (the visibility card, the audience radios, the thumbnail, advanced settings) no flow step touches it.
const (
	// DetailsURL is §1's entry: the URL takes the video id, which is why this flow needs no content-list
	// scrape to reach a video. It is recorded in that exact form, with no hl=en, and the navigate reply came
	// back `status 200` with the title `Video details - YouTube Studio` — so the recorded URL is used unchanged.
	DetailsURL = "https://studio.youtube.com/video/"
	// detailsURLTail closes §1's entry URL around the video id.
	detailsURLTail = "/edit"

	locTitleEditor = "div#title-textbox-container #textbox" // §2 short form, "resolved 1 hit"
	locDescEditor  = "#description-container #textbox"      // §2 short form, "resolved 1 hit"
	// locSave is the commit control as an address the page answers. §2 records its display path as
	// ytcp-button#save > div > ytcp-button-shape > button, and that chain resolves NOTHING: measured 2026-11-04,
	// the child-combination forms gave 0 hits while host-plus-leaf gave 1, because #save's children are a dom-if
	// and a div, with the shape below the div. A probe path describes where a node lives; it is not a selector.
	// This is the shortest measured-resolving form, and it is the address the state probe is handed too — give
	// it anything else and the probe reports "no buttons", which once made a save that had landed look like one
	// that had not (§4 trap 8).
	locSave     = "ytcp-button#save button"            // §2, aria-label "Save"; measured 1 hit
	locSnackbar = "[class*='snackbar']"                // §2 save confirmation, first address
	locNotice   = "ytcp-notification"                  // §2 save confirmation, second address
	locOverflow = "#overflow-menu-button"              // §3, aria-label "Options", icon more-vert
	locMenuItem = "tp-yt-paper-item"                   // §3 menu items
	locDialog   = "tp-yt-paper-dialog#dialog"          // §3 confirmation dialog
	locConsent  = "ytcp-checkbox-lit#confirm-checkbox" // §3 consent checkbox
	// locConsentBridge is the same unique node by the selector §4 trap 1 records pressing it *through the
	// bridge* (`click --selector "#confirm-checkbox"`). The consent press uses that form and no other.
	locConsentBridge = "#confirm-checkbox"
)

// Page text recorded by the probe. Each is used for a wait, for an identity assertion, or for checking that
// the control reached is the one the probe met — and never as a substring test where an exact string is
// recorded, because §4 trap 7 is that this page's controls share letters.
const (
	txtBanner        = "Video details"                                                       // §1 banner heading, and the navigate reply title
	txtTitleAria     = "Add a title that describes your video (type @ to mention a channel)" // §2 title editor
	txtDescAria      = "Tell viewers about your video (type @ to mention a channel)"         // §2 description editor
	txtSave          = "Save"                                                                // §2, the Save button's aria-label
	txtOptions       = "Options"                                                             // §3, the overflow button's aria-label
	txtSaved         = "Changes saved"                                                       // §2, the snackbar after a real save
	txtDelete        = "Delete"                                                              // §3 menu item, EXACT
	txtDeleteHeading = "Permanently delete this video?"                                      // §3 dialog heading
	txtDeleteForever = "Delete forever"                                                      // §3 dialog button, EXACT
)

// statePause is the cadence of every menu/dialog/save wait in this flow, matching the other page flows.
const statePause = 700 * time.Millisecond

// pageNode is one element as details_state.js saw it. Shown is the trap-4 filter: a node the page is not
// displaying is not a node this flow acts on, however reachable its text is. Path is derived at run time from
// the element's real ancestors — the same shape lookup.js reports as Finding.CSS — because §3 records the menu
// item and the dialog's buttons by their text alone, so their addresses have to come from the page.
type pageNode struct {
	Tag      string `json:"tag"`
	Text     string `json:"text"`
	Path     string `json:"path"`
	Shown    bool   `json:"shown"`
	Disabled bool   `json:"disabled"`
	Checked  bool   `json:"checked"`
}

// finding is this read in the R4 shape, so out.Record carries it into --explain beside the probe reads the
// flow made through lookup.js.
func (n pageNode) finding() youtube.Finding {
	checked := n.Checked
	return youtube.Finding{
		OK: true, Tag: n.Tag, Text: n.Text, CSS: n.Path, Disabled: n.Disabled,
		State: youtube.ElementState{Exists: true, Visible: n.Shown, Disabled: n.Disabled, Checked: &checked},
	}
}

// detailsState is one read of the page's interactive surface: the dialogs and menu items that exist, which of
// them the page is showing, the open dialog's buttons, and the consent checkbox.
type detailsState struct {
	URL      string     `json:"url"`
	Title    string     `json:"title"`
	Dialogs  []pageNode `json:"dialogs"`
	Items    []pageNode `json:"items"`
	Buttons  []pageNode `json:"buttons"`
	Notices  []pageNode `json:"notices"`
	Checkbox *pageNode  `json:"checkbox"`
}

// shownDialog is the dialog the page is displaying, or nil. §3's abort proof is the dialog count moving 1 → 0,
// so the count of *shown* dialogs is the observable this flow watches.
func (s detailsState) shownDialog() *pageNode {
	for i := range s.Dialogs {
		if s.Dialogs[i].Shown {
			return &s.Dialogs[i]
		}
	}
	return nil
}

// shownItem finds a menu item whose text is exactly want and that the page is showing.
func (s detailsState) shownItem(want string) *pageNode { return matchExact(s.Items, want) }

// shownButton is the same question asked of an open dialog's buttons.
func (s detailsState) shownButton(want string) *pageNode { return matchExact(s.Buttons, want) }

func matchExact(nodes []pageNode, want string) *pageNode {
	for i := range nodes {
		if !nodes[i].Shown {
			continue
		}
		if normText(nodes[i].Text) == normText(want) {
			return &nodes[i]
		}
	}
	return nil
}

// shownNoticeHolds finds a toast the page is showing whose text carries phrase — §2's `Changes saved`.
func (s detailsState) shownNoticeHolds(phrase string) *pageNode {
	for i := range s.Notices {
		if !s.Notices[i].Shown {
			continue
		}
		if strings.Contains(normText(s.Notices[i].Text), normText(phrase)) {
			return &s.Notices[i]
		}
	}
	return nil
}

// summary is the one line that says what the page was showing, so a refusal names the state it refused instead
// of leaving the caller to work out which dialog it meant.
func (s detailsState) summary() string {
	shown := 0
	for _, n := range s.Dialogs {
		if n.Shown {
			shown++
		}
	}
	items := 0
	for _, n := range s.Items {
		if n.Shown {
			items++
		}
	}
	buttons := make([]string, 0, len(s.Buttons))
	for _, n := range s.Buttons {
		if n.Shown {
			buttons = append(buttons, fmt.Sprintf("%s(disabled=%v)", n.Text, n.Disabled))
		}
	}
	return fmt.Sprintf("%d dialog(s) shown of %d, %d menu item(s) shown, buttons: %s, page at %s",
		shown, len(s.Dialogs), items, firstNonEmpty(strings.Join(buttons, " | "), "none"), s.URL)
}

type details struct {
	p    *youtube.Page
	t    youtube.Timings
	in   youtube.DetailsInput
	out  youtube.Outcome
	step int

	// title is the title as the panel itself read it when it opened, before anything was written. It is what
	// the delete dialog must name (§5.4), so a dialog belonging to another video can never be confirmed.
	title string
}

func (d *details) note(format string, args ...any) {
	d.out.Steps = append(d.out.Steps, fmt.Sprintf("%d. %s", d.step, fmt.Sprintf(format, args...)))
}

func (d *details) warn(format string, args ...any) {
	d.out.Warnings = append(d.out.Warnings, fmt.Sprintf(format, args...))
}

func (d *details) shot(ctx context.Context, name string) {
	if d.in.Evidence == "" {
		return
	}
	if err := d.p.Screenshot(ctx, filepath.Join(d.in.Evidence, name+".png")); err != nil {
		d.warn("screenshot %s: %v", name, err)
	}
}

// Details runs one Studio details-panel action on the video in in.VideoID: `edit` writes the fields supplied and
// commits them with a single Save press, `delete` walks the overflow menu to the irreversible press.
//
// Both press exactly once. §4 trap 5 is that a Save is a real commit whose pending state is invisible anywhere
// else on the channel, and the delete press cannot be undone at all; §5.2 states the consequence — "a press
// that is in flight looks the same as one that failed" — so an ambiguous press is reported transient and is
// never repeated, and the journal slot is claimed immediately before the press and abandoned on every failure
// path.
func Details(ctx context.Context, p *youtube.Page, in youtube.DetailsInput, tim youtube.Timings) (youtube.Outcome, error) {
	if tim.Action <= 0 {
		tim.Action = 45 * time.Second
	}
	d := &details{p: p, t: tim, in: in}
	d.out.VideoID = in.VideoID
	d.out.URL = DetailsURL + in.VideoID + detailsURLTail

	switch in.Action {
	case "edit":
		if strings.TrimSpace(in.Title) == "" && strings.TrimSpace(in.Description) == "" {
			// §5.3: the enabled Save button is the last signal that anything changed, so a run with nothing to
			// write could only press a control that has nothing to commit. Refused before the page is opened.
			return d.out, fmt.Errorf("%w: an edit needs title and/or description — the page was never opened and nothing was written", youtube.ErrUsage)
		}
	case "delete":
		// §5.4: "anything short of the confirmation parameter stops before the overflow menu" — and this stops
		// before the browser is touched at all, so the refusal cannot even be a slow page load.
		if !in.Confirmed {
			return d.out, fmt.Errorf(`%w: deleting a video is permanent and cannot be undone, so it needs the caller's explicit confirmation (yes_i_understand=true). The overflow menu was never opened and nothing was pressed`, youtube.ErrUsage)
		}
	default:
		return d.out, fmt.Errorf(`%w: details action %q is not edit or delete`, youtube.ErrUsage, in.Action)
	}
	if in.Journal == nil && !in.DryRun {
		// Both actions change the channel, and a change with no record of it is what the ledger exists to
		// prevent (§4.6). A dry run is handed no journal by the service because it presses nothing.
		return d.out, fmt.Errorf("%w: a %s has nowhere to be recorded — the caller must supply the journal", youtube.ErrUsage, in.Action)
	}

	if err := d.open(ctx); err != nil {
		return d.out, err
	}
	if err := d.panelReady(ctx); err != nil {
		return d.out, err
	}
	var err error
	if in.Action == "delete" {
		err = d.deleteVideo(ctx)
	} else {
		err = d.editVideo(ctx)
	}
	return d.out, err
}

// open navigates the panel in the tab the caller owns, so the identity assertion (the configured container, and
// the channel hint the service sets) rides on the navigate reply as it does everywhere else in this package.
func (d *details) open(ctx context.Context) error {
	d.step++
	if err := d.p.Open(ctx, d.out.URL); err != nil {
		return err
	}
	// §1 records the navigate reply as `status 200` with the title `Video details - YouTube Studio`. A reply
	// whose title has not caught up is the page being slow, not the page being wrong — the panel's own banner
	// is asserted from the DOM below, and that is what the run relies on.
	if t := d.p.Tab().Title; t != "" && !strings.Contains(t, txtBanner) {
		d.warn("the navigate reply titled the tab %q rather than naming the %q panel (§1)", t, txtBanner)
	}
	d.note("opened the Studio details panel for %s", d.in.VideoID)
	return nil
}

// panelReady proves this is the details panel of the video asked for, before a field is written or a menu is
// opened: §2's title editor resolves, the tab is on a URL naming this video id, the page carries §1's banner
// heading, and the video card §1 records (`0:08 output (edited via page flow)`) names the video whose fields
// are about to be changed.
func (d *details) panelReady(ctx context.Context) error {
	d.step++
	title := youtube.Locator{CSS: locTitleEditor}
	f, err := d.p.WaitState(ctx, title, d.t.Action, func(g youtube.Finding) bool { return g.OK })
	d.out.Record("title editor", f)
	if err != nil || !f.OK {
		return fmt.Errorf("%w: %s never resolved, so this page is not the Studio details panel (%v)", youtube.ErrElement, locTitleEditor, err)
	}
	d.checkAria("title editor", f, txtTitleAria)
	d.title = normText(f.Text)

	desc, derr := d.p.Find(ctx, youtube.Locator{CSS: locDescEditor})
	d.out.Record("description editor", desc)
	if derr != nil {
		return derr
	}
	if !desc.OK && strings.TrimSpace(d.in.Description) != "" {
		return fmt.Errorf("%w: the details panel offers no %s, so the description cannot be written on this page", youtube.ErrElement, locDescEditor)
	} else if desc.OK {
		d.checkAria("description editor", desc, txtDescAria)
	}
	d.shot(ctx, "01-details")

	d.step++
	if u := d.p.Tab().URL; u != "" && !strings.Contains(u, d.in.VideoID) {
		return fmt.Errorf("%w: the tab is at %s, which does not name video %s — §1's entry URL carries the id, so this is not that video's panel", youtube.ErrIdentity, u, d.in.VideoID)
	}
	text, err := d.p.BodyText(ctx)
	if err != nil {
		return err
	}
	body := normText(text)
	if !strings.Contains(body, txtBanner) {
		return fmt.Errorf("%w: the page shows no %q banner (§1), so it is not the details panel; it reads %q",
			youtube.ErrElement, txtBanner, truncate(body, 140))
	}
	if d.title == "" {
		d.warn("the title field is empty, so the panel could not be cross-checked against the video card (§1); the banner heading is the only identity read")
	} else if !strings.Contains(body, d.title) {
		return fmt.Errorf("%w: the panel's own title field reads %q but nothing on the page names that video, so this is not the panel for it", youtube.ErrIdentity, truncate(d.title, 80))
	}
	d.note("details panel ready: %s, and the video card names %q", txtBanner, truncate(d.title, 60))
	return nil
}

// checkAria compares a control's accessible name with the one the probe recorded and says so when they differ.
// It is a warning rather than a refusal: the label is copy that Studio can reword, while the address that
// resolved is the hard fact.
func (d *details) checkAria(role string, f youtube.Finding, want string) {
	if f.Name != "" && f.Name != want {
		d.warn("%s carries aria-label %q, not the probed %q (studio-details.md §2/§3) — the page may have changed under this flow", role, f.Name, want)
	}
}

// checkShown reports a control the page holds but does not display. lookup.js answers with the geometry flag in
// state.visible; the flow's hard filter is applied to the menus and dialogs, where §4 trap 4 bites, and to the
// buttons it is about to press it is said out loud rather than silently trusted.
func (d *details) checkShown(role string, f youtube.Finding) {
	if f.OK && !f.State.Visible {
		d.warn("%s resolves but the page is not showing it (state.visible=false) — §4 trap 4: a node that exists is not a node that is open", role)
	}
}

// field is one writable details field: the address §2 records, the aria-label it was seen carrying, and the
// text the caller asked for.
type field struct {
	role string
	css  string
	aria string
	want string
}

// fields is the write set in page order. A field that was not asked for is not in it, and so is never touched
// (§5.2: "write only the fields given").
func (d *details) fields() []field {
	var out []field
	if strings.TrimSpace(d.in.Title) != "" {
		out = append(out, field{role: "title", css: locTitleEditor, aria: txtTitleAria, want: d.in.Title})
	}
	if strings.TrimSpace(d.in.Description) != "" {
		out = append(out, field{role: "description", css: locDescEditor, aria: txtDescAria, want: d.in.Description})
	}
	return out
}

func (d *details) editVideo(ctx context.Context) error {
	fields := d.fields()
	if d.in.DryRun {
		d.step++
		for _, fl := range fields {
			f, err := d.p.Find(ctx, youtube.Locator{CSS: fl.css})
			d.out.Record(fl.role+" (dry run)", f)
			if err != nil {
				return err
			}
			if !f.OK {
				return fmt.Errorf("%w: the details panel offers no %s, so the %s cannot be edited on this page", youtube.ErrElement, fl.css, fl.role)
			}
			d.checkAria(fl.role, f, fl.aria)
			d.note("dry run: the %s reads %q and would become %q; nothing was typed", fl.role, truncate(normText(f.Text), 60), truncate(normText(fl.want), 60))
		}
		d.note("dry run: nothing was typed and %s was not pressed, so the form holds no pending change (§4 trap 5)", locSave)
		return nil
	}

	for _, fl := range fields {
		if err := d.writeField(ctx, fl); err != nil {
			return err
		}
	}
	d.shot(ctx, "02-fields-written")
	return d.save(ctx)
}

// writeField enters one field through the page channel and reads it back twice. The write is the Stage 1 lesson
// unchanged — focus, selectNodeContents, document.execCommand("insertText"), input event (§2) — and §4 trap 5 is
// why a write that did not land is a refusal rather than a warning: an edit the form never took is invisible
// everywhere except in the Save button's disabled flag.
func (d *details) writeField(ctx context.Context, fl field) error {
	d.step++
	loc := youtube.Locator{CSS: fl.css}
	f, read, err := d.p.Type(ctx, loc, fl.want)
	d.out.Record(fl.role, f)
	if err != nil {
		return fmt.Errorf("%w: writing the %s did not reach the page (%v); the editor reads %q and %s was NOT pressed, so nothing was saved",
			youtube.ErrElement, fl.role, err, truncate(normText(read), 80), locSave)
	}
	d.note("wrote the %s (%d characters)", fl.role, len([]rune(fl.want)))

	d.step++
	back, err := d.p.Find(ctx, loc)
	d.out.Record(fl.role+" read back", back)
	if err != nil {
		return err
	}
	if !back.OK {
		return fmt.Errorf("%w: %s disappeared after the %s was written, so the write cannot be confirmed and %s was NOT pressed",
			youtube.ErrElement, fl.css, fl.role, locSave)
	}
	got := normText(back.Text)
	switch {
	case got == normText(fl.want):
		d.note("the %s now reads %q", fl.role, truncate(got, 80))
	case got != "" && strings.HasPrefix(normText(fl.want), got):
		// The read is capped by youtube.maxDiagBytes, so a long value comes back as its own prefix. That is a
		// short read, not a lost write, and it is reported as such rather than passed off as confirmation.
		d.warn("the %s read came back as the first %d characters of the %d written — the probe's read cap, so the tail is unverified",
			fl.role, len([]rune(got)), len([]rune(fl.want)))
	default:
		return fmt.Errorf("%w: the %s reads %q after the write, not %q, so the page did not take the text and %s was NOT pressed",
			youtube.ErrElement, fl.role, truncate(got, 80), truncate(normText(fl.want), 80), locSave)
	}
	return nil
}

// save commits the form with exactly one press, then waits for the two signals §2 records on this page. §4
// trap 3 is the whole design: this page has no #save-msg, so a flow that waits for the wizard's `All changes
// saved` waits forever — the signals are the `Changes saved` snackbar and the Save button going disabled.
func (d *details) save(ctx context.Context) error {
	d.step++
	save := youtube.Locator{CSS: locSave}
	f, err := d.p.Find(ctx, save)
	d.out.Record("save button", f)
	if err != nil {
		return err
	}
	if !f.OK {
		return fmt.Errorf("%w: the details panel offers no %s, so nothing could be committed", youtube.ErrElement, locSave)
	}
	d.checkShown("save button", f)
	if f.Name != "" && f.Name != txtSave {
		d.warn("the save button is labelled %q, not the probed %q (§2)", f.Name, txtSave)
	}
	if f.Disabled || f.State.Disabled {
		// §2: enabled while the form is dirty, disabled once it is clean. Disabled after a verified write means
		// the page already holds this value — there is nothing to commit, and pressing a disabled control is a
		// guess about what it would do.
		d.note("%s is disabled, so the page holds no pending change: the fields already read as requested, and no press was made", locSave)
		return nil
	}

	d.step++
	// The claim sits here, immediately before the press, because the press is the mutation (§4.6, §5.2).
	slot, err := d.claim("edit")
	if err != nil {
		return err
	}
	pressed, err := d.p.Click(ctx, save, "", 0)
	d.out.Record("save pressed", pressed)
	if err != nil {
		return d.fail(slot, fmt.Errorf("%w: pressing %s reported a failure (%v). A save in flight looks exactly like one that failed, so it is not repeated (§5.2) — look at the video before running this again",
			youtube.ErrTransient, locSave, err))
	}
	d.note("pressed %s once", locSave)
	d.shot(ctx, "03-saved")

	d.step++
	if err := d.awaitSaved(ctx); err != nil {
		return d.fail(slot, err)
	}
	if jerr := d.in.Journal.Complete(slot.SlotID, d.in.VideoID); jerr != nil {
		d.warn("journal could not record the saved video %s: %v", d.in.VideoID, jerr)
	}
	return nil
}

// awaitSaved polls until both confirmations have arrived and reports which of them did when they did not. The
// snackbar fades, so the two signals are collected in one loop rather than waited for in series.
func (d *details) awaitSaved(ctx context.Context) error {
	var snack, clean bool
	var last detailsState
	err := d.p.Poll(ctx, statePause, d.t.Action, func(c context.Context) (bool, error) {
		if !clean {
			if f, ferr := d.p.Find(c, youtube.Locator{CSS: locSave}); ferr == nil && f.OK && (f.Disabled || f.State.Disabled) {
				clean = true
				d.out.Record("save button after the press", f)
			}
		}
		if !snack {
			s, serr := d.state(c)
			if serr == nil {
				last = s
				if n := s.shownNoticeHolds(txtSaved); n != nil {
					snack = true
					d.out.Record("save confirmation", n.finding())
				}
			}
		}
		// The flip is the signal; the snackbar is decoration that expires. Requiring both made a save that had
		// really landed report an unconfirmed commit, because by the time the poll saw the button clean the
		// snackbar was already gone — and an "unconfirmed" edit tells the caller to go look, when the page has
		// already said the form is clean. Measured 2026-11-04: disabled=true, snackbar already absent.
		return clean, nil
	})
	// Say which signals were actually seen: claiming the snackbar when only the flip arrived would report more
	// verification than the page gave.
	if err == nil {
		if snack {
			d.note("%s confirmed: the snackbar showed %q and %s became disabled (§2)", txtSaved, txtSaved, locSave)
		} else {
			d.warn("the %q snackbar was not seen, but %s went disabled — the page reports the form clean, which is the durable signal (§2, §4 trap 3)", txtSaved, locSave)
		}
		return nil
	}
	if errors.Is(err, youtube.ErrAbandonedTab) {
		return err
	}
	// No flip means the page never took the change. The press happened, so it is never repeated.
	return fmt.Errorf("%w: %s was pressed once and the page confirmed it only partly within %s — snackbar %q seen: %v, %s disabled: %v; %s. Nothing was pressed again; check the video (§2, §4 trap 3)",
		youtube.ErrTransient, locSave, d.t.Action, txtSaved, snack, locSave, clean, last.summary())
}

// deleteVideo walks §3's chain: Options → the exact-text `Delete` item → the dialog, which must name this video
// → the consent checkbox through the bridge → the gate opening → one press. Everything before the last step is
// reversible by walking away, so the refusals are placed where the money is: nothing presses Delete forever
// unless the page itself has proved the consent.
func (d *details) deleteVideo(ctx context.Context) error {
	d.step++
	overflow := youtube.Locator{CSS: locOverflow}
	f, err := d.p.Find(ctx, overflow)
	d.out.Record("options button", f)
	if err != nil {
		return err
	}
	if !f.OK {
		return fmt.Errorf("%w: the details panel offers no %s (%s), so the delete control is not reachable on this page", youtube.ErrElement, locOverflow, txtOptions)
	}
	d.checkShown("options button", f)
	d.checkAria("options button", f, txtOptions)

	if d.in.DryRun {
		d.step++
		s, serr := d.state(ctx)
		if serr != nil {
			return serr
		}
		d.note("dry run: %s located and NOT pressed, so neither the menu nor the delete dialog was ever opened; the video is %q and %s",
			locOverflow, truncate(d.title, 60), s.summary())
		return nil
	}

	d.step++
	slot, err := d.claim("delete")
	if err != nil {
		return err
	}
	pressed, err := d.p.Click(ctx, overflow, "", 0)
	d.out.Record("options button pressed", pressed)
	if err != nil {
		return d.fail(slot, fmt.Errorf("press %s: %w — nothing was deleted", locOverflow, err))
	}
	d.note("pressed %s once", locOverflow)

	d.step++
	var item *pageNode
	st, err := d.waitState(ctx, func(s detailsState) (bool, error) {
		item = s.shownItem(txtDelete)
		return item != nil, nil
	})
	if err != nil || item == nil {
		// Trap 7 in the refusal's wording: the menu is searched for the exact string, and an item that merely
		// contains those letters is not it.
		return d.fail(slot, fmt.Errorf("%w: pressing %s never showed a menu item reading exactly %q (%v); %s — nothing was deleted",
			youtube.ErrElement, locOverflow, txtDelete, err, st.summary()))
	}
	d.out.Record("delete menu item", item.finding())
	d.note("the menu shows %s; the one pressed reads %q", plural(len(shownNodes(st.Items)), "item"), item.Text)

	d.step++
	if item.Path == "" {
		return d.fail(slot, fmt.Errorf("%w: the %q menu item has no address that is unique on this page, so it cannot be pressed without guessing — nothing was deleted",
			youtube.ErrElement, txtDelete))
	}
	itemPressed, err := d.p.Click(ctx, youtube.Locator{CSS: item.Path}, "", 0)
	d.out.Record("delete menu item pressed", itemPressed)
	if err != nil {
		return d.fail(slot, fmt.Errorf("press %s: %w — nothing was deleted", txtDelete, err))
	}
	if normText(itemPressed.Text) != normText(txtDelete) {
		d.warn("what was pressed reads %q, not the probed %q (§3) — the dialog below is what decides whether anything was deleted", itemPressed.Text, txtDelete)
	}
	d.note("pressed the menu item %q once", txtDelete)

	d.step++
	var dlg *pageNode
	st, err = d.waitState(ctx, func(s detailsState) (bool, error) {
		dlg = s.shownDialog()
		return dlg != nil, nil
	})
	if err != nil || dlg == nil {
		return d.fail(slot, fmt.Errorf("%w: pressing %q never brought up a visible %s (%v); %s — nothing was deleted",
			youtube.ErrElement, txtDelete, locDialog, err, st.summary()))
	}
	d.out.Record("delete dialog", dlg.finding())
	if st.Checkbox != nil {
		d.out.Record("consent checkbox", st.Checkbox.finding())
	}

	// Two things must be true of the words on that dialog before it counts as the confirmation for THIS video:
	// the heading §3 recorded, and the title read off the panel before anything was touched. §3 shows the
	// dialog prints the duration, the title, the upload date and the view count, so the title is the part this
	// flow can compare against what it already knows.
	words := normText(dlg.Text)
	if !strings.Contains(words, normText(txtDeleteHeading)) {
		return d.fail(slot, fmt.Errorf("%w: the dialog the page is showing does not read %q (§3); it reads %q — nothing was deleted",
			youtube.ErrElement, txtDeleteHeading, truncate(words, 140)))
	}
	if d.title == "" {
		return d.fail(slot, fmt.Errorf("%w: the panel's title field was empty, so the dialog could not be checked against this video and a delete cannot be confirmed blind — nothing was deleted",
			youtube.ErrUsage))
	}
	if !strings.Contains(words, d.title) {
		return d.fail(slot, fmt.Errorf("%w: the dialog names %q, not the video whose details panel reads %q — nothing was deleted",
			youtube.ErrIdentity, truncate(words, 140), truncate(d.title, 80)))
	}
	d.note("the dialog names this video: %q", truncate(words, 120))

	d.step++
	// §4 trap 1: a synthetic click from a page program left the Lit checkbox unchecked while reporting itself
	// done, so this press goes through the bridge channel, with the selector that probe used. Page.Click's
	// fallback only runs when the bridge could not see the control at all — a not-found press is a no-op, so it
	// cannot silently stand in for a trusted one — and the gate below settles the question either way.
	cb, err := d.p.Click(ctx, youtube.Locator{CSS: locConsentBridge}, "", 0)
	d.out.Record("consent checkbox pressed", cb)
	if err != nil {
		return d.fail(slot, fmt.Errorf("%w: pressing %s reported a failure (%v), so the consent cannot be relied on — nothing was pressed after the checkbox and nothing was deleted",
			youtube.ErrElement, locConsent, err))
	}
	d.note("pressed %s once through the bridge channel", locConsent)

	d.step++
	var gate *pageNode
	st, err = d.waitState(ctx, func(s detailsState) (bool, error) {
		gate = nil
		if b := s.shownButton(txtDeleteForever); b != nil && !b.Disabled {
			gate = b
		}
		return gate != nil, nil
	})
	if err != nil {
		held := "the button is not on the dialog at all"
		if b := st.shownButton(txtDeleteForever); b != nil {
			held = fmt.Sprintf("it reads %q and stays disabled=%v", b.Text, b.Disabled)
		}
		// §4 trap 2: the button becoming enabled is the page's own proof that the consent was registered.
		// Without it there is no way to tell a consent the page took from one it ignored, and what follows
		// cannot be undone — so nothing is pressed.
		return d.fail(slot, fmt.Errorf("%w: %s never became enabled after %s was pressed — %s (§4 trap 1: only a trusted click ticks this checkbox). Nothing was pressed after the checkbox and nothing was deleted",
			youtube.ErrElement, txtDeleteForever, locConsent, held))
	}
	d.out.Record("delete gate open", gate.finding())
	d.note("%s went enabled, which is the page's proof that the consent was registered (§4 trap 2)", txtDeleteForever)

	d.step++
	if gate.Path == "" {
		return d.fail(slot, fmt.Errorf("%w: %s has no address that is unique on this page, so it cannot be pressed without guessing — nothing was deleted",
			youtube.ErrElement, txtDeleteForever))
	}
	// One press, and never a second whatever it reports: a delete that times out may already have happened.
	forever, err := d.p.Click(ctx, youtube.Locator{CSS: gate.Path}, "", 0)
	d.out.Record("delete forever pressed", forever)
	if err != nil {
		return d.fail(slot, fmt.Errorf("%w: pressing %s reported a failure (%v). The video may already be deleted and pressing again is forbidden — check the channel content list (§5.4)",
			youtube.ErrTransient, txtDeleteForever, err))
	}
	d.note("pressed %q once", txtDeleteForever)

	d.step++
	if err := d.confirmDeleted(ctx, forever); err != nil {
		return d.fail(slot, err)
	}
	if jerr := d.in.Journal.Complete(slot.SlotID, d.in.VideoID); jerr != nil {
		d.warn("journal could not record the deleted video %s: %v", d.in.VideoID, jerr)
	}
	d.note("video %s deleted: the page left the details state (§5.4)", d.in.VideoID)
	d.shot(ctx, "04-after-delete")
	return nil
}

// confirmDeleted watches the dialog go away. §3's abort proof is the dialog count moving 1 → 0, so that is the
// requirement; what the page shows after a delete that was not cancelled was never recorded, so the URL and the
// title editor are reported rather than asserted.
// UNPROBED: the page state after a completed delete — the 2026-11-04 probe stopped at the enabled button.
func (d *details) confirmDeleted(ctx context.Context, forever youtube.Finding) error {
	var last detailsState
	err := d.p.Poll(ctx, statePause, d.t.Action, func(c context.Context) (bool, error) {
		s, serr := d.state(c)
		if serr != nil {
			return false, serr
		}
		last = s
		return s.shownDialog() == nil, nil
	})
	where := last.URL
	if u, uerr := d.p.URL(ctx); uerr == nil && u != "" {
		where = u
	}
	if err != nil {
		if errors.Is(err, youtube.ErrAbandonedTab) {
			return err
		}
		return fmt.Errorf("%w: %s was pressed once (%s) and %s is still on the screen, so the delete is not confirmed and pressing again is forbidden — check the channel content list (§5.4); %s",
			youtube.ErrTransient, txtDeleteForever, forever.String(), locDialog, last.summary())
	}
	if tf, _ := d.p.Find(ctx, youtube.Locator{CSS: locTitleEditor}); tf.OK {
		d.out.Record("title editor after the delete", tf)
	}
	d.note("the dialog is gone (count 1 → 0, §3); the tab is at %s and the page title reads %q", truncate(where, 90), firstNonEmpty(last.Title, "(unread)"))
	return nil
}

// state reads the page's interactive surface through the embedded probe. Every menu and dialog decision is made
// on this read, because every one of them has to ignore what the page is not showing (§4 trap 4, §5.5).
func (d *details) state(ctx context.Context) (detailsState, error) {
	var s detailsState
	err := d.p.Run(ctx, "details_state.js", map[string]any{
		"dialog":   locDialog,
		"checkbox": locConsent,
		"menuItem": locMenuItem,
		"notices":  []string{locSnackbar, locNotice},
	}, &s)
	return s, err
}

// waitState polls that read until ok is satisfied or the one-action budget runs out.
func (d *details) waitState(ctx context.Context, ok func(detailsState) (bool, error)) (detailsState, error) {
	var last detailsState
	err := d.p.Poll(ctx, statePause, d.t.Action, func(c context.Context) (bool, error) {
		s, err := d.state(c)
		if err != nil {
			return false, err
		}
		last = s
		return ok(s)
	})
	return last, err
}

// claim takes the journal slot for one details mutation, before the press that performs it.
func (d *details) claim(kind string) (youtube.JournalSlot, error) {
	return d.in.Journal.Claim(kind)
}

// fail marks a claimed slot abandoned and hands the reason back, so a run that changed nothing still leaves a
// trace of what it tried to change. The video id is bound either way: after the irreversible press, a failed
// run must still say which video it may have deleted.
func (d *details) fail(slot youtube.JournalSlot, err error) error {
	if d.in.Journal != nil && slot.SlotID != "" {
		if jerr := d.in.Journal.Abandon(slot.SlotID, d.in.VideoID, err.Error()); jerr != nil {
			d.warn("journal could not abandon slot %s: %v", slot.SlotID, jerr)
		}
	}
	return err
}

// shownNodes is the menu as the page displays it, for a step line that cannot over-report a closed menu.
func shownNodes(nodes []pageNode) []pageNode {
	var out []pageNode
	for _, n := range nodes {
		if n.Shown {
			out = append(out, n)
		}
	}
	return out
}
