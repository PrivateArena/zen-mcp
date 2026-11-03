// Package pages holds the flows. A flow may call only youtube.Page methods — no orchestration, no
// encoding of tool output, no direct bridge calls — which is what makes each one replayable against a
// scripted Page in a test and reusable from the terminal REPL.
package pages

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"zen-mcp/internal/youtube"
)

// Frozen locators. Every string here was copied out of a probe response recorded in
// docs/youtube-pages/upload-wizard.md, from the live run of 2026-11-02. Nothing in this file is inferred,
// and what was not probed is not here: the tags chips, the category select and the custom-thumbnail control
// are refused rather than guessed.
const (
	UploadURL = "https://www.youtube.com/upload?hl=en"

	locFilePicker  = "ytcp-uploads-file-picker#ytcp-uploads-dialog-file-picker input[type=file]"
	locTitle       = "#title-textarea div#textbox"
	locDescription = "#description-textarea div#textbox"
	locNext        = "#next-button button"
	locDone        = "#done-button button"

	locAudience    = "ytkc-made-for-kids-select"
	locVisibility  = "ytcp-video-visibility-select"
	locPrivacyRads = "#privacy-radios"

	locDateTrigger = "ytcp-text-dropdown-trigger#datepicker-trigger"
	locCalendarDay = ".calendar-day"
	locTimeInput   = "ytcp-datetime-picker input.style-scope.tp-yt-paper-input"
)

type wizard struct {
	p    *youtube.Page
	t    youtube.Timings
	in   youtube.Upload
	out  youtube.Outcome
	step int
}

func (w *wizard) note(format string, args ...any) {
	w.out.Steps = append(w.out.Steps, fmt.Sprintf("%d. %s", w.step, fmt.Sprintf(format, args...)))
}

func (w *wizard) warn(format string, args ...any) {
	w.out.Warnings = append(w.out.Warnings, fmt.Sprintf(format, args...))
}

func (w *wizard) shot(ctx context.Context, name string) {
	if w.in.Evidence == "" {
		return
	}
	if err := w.p.Screenshot(ctx, filepath.Join(w.in.Evidence, name+".png")); err != nil {
		w.warn("screenshot %s: %v", name, err)
	}
}

func (w *wizard) state(ctx context.Context) (wizardState, error) {
	var s wizardState
	err := w.p.Run(ctx, "wizard_state.js", map[string]any{}, &s)
	return s, err
}

// UploadWizard walks the Studio upload wizard end to end: attach → details → checks → visibility → commit.
//
// It performs exactly one attach and never retries it: the attach is the moment the video comes into
// existence, because Studio mints the id and autosaves a private draft before anything else happens. A
// failure after that point still returns whatever video id the page revealed, so the caller can bind it to
// the journal instead of leaving an unrecorded draft on the channel.
func UploadWizard(ctx context.Context, p *youtube.Page, in youtube.Upload, t youtube.Timings) (youtube.Outcome, error) {
	if t.Wizard <= 0 {
		t.Wizard = 15 * time.Minute
	}
	if t.Action <= 0 {
		t.Action = 45 * time.Second
	}
	w := &wizard{p: p, t: t, in: in}

	if err := w.open(ctx); err != nil {
		return w.out, err
	}
	target, err := w.attachTarget(ctx)
	if err != nil {
		return w.out, err
	}
	if in.DryRun {
		w.step++
		w.note("dry run: identity verified and attach target located; nothing was uploaded")
		return w.out, nil
	}
	if err := w.attach(ctx, target); err != nil {
		return w.out, err
	}
	if err := w.details(ctx); err != nil {
		return w.out, w.withID(ctx, err)
	}
	if err := w.advance(ctx, "Video elements"); err != nil {
		return w.out, w.withID(ctx, err)
	}
	if err := w.advance(ctx, "Checks"); err != nil {
		return w.out, w.withID(ctx, err)
	}
	if err := w.awaitChecks(ctx); err != nil {
		return w.out, w.withID(ctx, err)
	}
	if err := w.advance(ctx, "Visibility"); err != nil {
		return w.out, w.withID(ctx, err)
	}
	if err := w.setVisibility(ctx); err != nil {
		return w.out, w.withID(ctx, err)
	}
	if err := w.commit(ctx); err != nil {
		return w.out, w.withID(ctx, err)
	}
	return w.out, nil
}

// open navigates the wizard in the tab the caller owns, which asserts the identity twice over: the reply's
// containerId, and the channel id the redirect puts in the URL.
func (w *wizard) open(ctx context.Context) error {
	w.step++
	if err := w.p.Open(ctx, UploadURL); err != nil {
		return err
	}
	w.note("opened the upload wizard in the configured identity")
	return nil
}

// attachTarget locates the file input through R4. The input exists but is 0×0 and hidden, which is why
// visibility must never be a precondition for attaching.
func (w *wizard) attachTarget(ctx context.Context) (youtube.Locator, error) {
	w.step++
	loc := youtube.Locator{CSS: locFilePicker}
	f, err := w.p.Find(ctx, loc)
	if err != nil {
		return loc, err
	}
	w.out.Record("attach input", f)
	if !f.OK {
		state, _ := w.state(ctx)
		return loc, fmt.Errorf("%w: the upload dialog offered no file input (%s); the page is on %q at %s",
			youtube.ErrWizardStuck, loc, firstNonEmpty(state.Panel, "no panel"), state.URL)
	}
	if !f.State.Exists {
		return loc, fmt.Errorf("%w: %s is absent — the wizard did not open", youtube.ErrWait, loc)
	}
	w.note("attach target located: %s (hidden %v, count %d)", f.CSS, !f.State.Visible, f.Count)
	w.shot(ctx, "01-wizard-open")
	return loc, nil
}

// attach is one shot by construction — the call is not wrapped in any retry.
func (w *wizard) attach(ctx context.Context, loc youtube.Locator) error {
	w.step++
	info, err := w.p.Attach(ctx, loc, w.in.File)
	if err != nil {
		return fmt.Errorf("attach %s: %w", filepath.Base(w.in.File), err)
	}
	w.out.Attached = true
	w.note("attached %s (%v bytes)", filepath.Base(w.in.File), info["size"])
	w.shot(ctx, "02-attached")

	got, err := w.p.WaitAny(ctx, []string{"Upload complete", "Processing", "Checks", "100%"}, w.t.Wizard)
	if err != nil {
		return fmt.Errorf("attached but the transfer never reported a state: %w", err)
	}
	w.note("transfer reports %q", got)
	w.shot(ctx, "03-transfer")
	return nil
}

// details fills the first panel and answers the question that otherwise blocks Next with nothing but inline
// text on the page.
func (w *wizard) details(ctx context.Context) error {
	w.step++
	title := youtube.Locator{CSS: locTitle}
	if _, err := w.p.WaitState(ctx, title, w.t.Wizard, func(f youtube.Finding) bool { return f.OK && f.State.Exists }); err != nil {
		return fmt.Errorf("the details panel never appeared: %w", err)
	}
	if w.in.Title != "" {
		f, read, err := w.p.Type(ctx, title, w.in.Title)
		w.out.Record("title", f)
		if err != nil {
			return fmt.Errorf("title: %w", err)
		}
		w.note("title reads %q", read)
	}
	if w.in.Description != "" {
		desc := youtube.Locator{CSS: locDescription}
		f, read, err := w.p.Type(ctx, desc, w.in.Description)
		w.out.Record("description", f)
		if err != nil {
			return fmt.Errorf("description: %w", err)
		}
		w.note("description set (%d characters read back)", len([]rune(read)))
	}
	if len(w.in.Tags) > 0 || w.in.Category != "" || w.in.Thumbnail != "" {
		w.warn("tags, category and thumbnail are not frozen (never probed) so they were not set — use the details page editor")
	}
	if err := w.audience(ctx); err != nil {
		return err
	}
	w.shot(ctx, "04-details")
	return nil
}

// audience answers the COPPA question and proves the radio took the state, because a click that reports
// success and leaves the group unanswered costs the whole run at Next.
func (w *wizard) audience(ctx context.Context) error {
	w.step++
	if w.in.MadeForKids == nil {
		return fmt.Errorf("%w: made_for_kids must be true or false — the wizard will not advance without an answer", youtube.ErrWizardStuck)
	}
	answer := "No, it's not made for kids"
	if *w.in.MadeForKids {
		answer = "Yes, it's made for kids"
	}
	loc := youtube.Locator{Root: locAudience, ByText: answer}
	f, err := w.p.Find(ctx, loc)
	w.out.Record("audience", f)
	if err != nil {
		return err
	}
	if !f.OK {
		return fmt.Errorf("%w: %s — if the wizard resumed an existing draft its earlier answer stands", youtube.ErrElement, loc)
	}
	if _, err := w.p.Click(ctx, loc, "", 0); err != nil {
		return fmt.Errorf("answer the made-for-kids question: %w", err)
	}
	taken, err := w.p.WaitState(ctx, loc, w.t.Action, func(g youtube.Finding) bool {
		return g.OK && g.State.Checked != nil && *g.State.Checked
	})
	w.out.Record("audience answered", taken)
	if err != nil {
		return fmt.Errorf("the audience answer did not take: %w", err)
	}
	w.note("audience: %s", answer)
	return nil
}

// advance presses Next and verifies the panel actually changed by reading the wizard's own tab state, which
// is the evidence — the step headers are not.
func (w *wizard) advance(ctx context.Context, panel string) error {
	w.step++
	next := youtube.Locator{CSS: locNext}
	f, err := w.p.Find(ctx, next)
	w.out.Record("next", f)
	if err != nil {
		return err
	}
	if !f.OK {
		return fmt.Errorf("%w: no Next button (%s) — the wizard may not allow advancing from here", youtube.ErrElement, locNext)
	}
	if f.State.Disabled {
		state, serr := w.state(ctx)
		if serr == nil && len(state.Alerts) > 0 {
			return fmt.Errorf("%w: the page still says %q", youtube.ErrWizardStuck, strings.Join(state.Alerts, " | "))
		}
		return fmt.Errorf("%w: Next is disabled — an required field is unanswered", youtube.ErrWizardStuck)
	}
	if _, err := w.p.Click(ctx, next, "", 0); err != nil {
		return fmt.Errorf("press Next: %w", err)
	}
	err = w.p.Poll(ctx, 700*time.Millisecond, w.t.Wizard, func(c context.Context) (bool, error) {
		s, err := w.state(c)
		if err != nil {
			return false, err
		}
		return s.Panel == panel, nil
	})
	if err != nil {
		state, _ := w.state(ctx)
		return fmt.Errorf("Next was pressed but the active panel is %q, not %q: %w", state.Panel, panel, err)
	}
	w.out.Panel = panel
	w.note("panel: %s", panel)
	w.shot(ctx, "05-"+strings.ToLower(strings.ReplaceAll(panel, " ", "-")))
	return nil
}

// awaitChecks waits for the checks panel to come to rest and surfaces what it found. A copyright claim is
// reported, not swallowed — the wizard would continue regardless, but the caller must see it.
func (w *wizard) awaitChecks(ctx context.Context) error {
	w.step++
	var last wizardState
	err := w.p.Poll(ctx, 3*time.Second, w.t.Wizard, func(c context.Context) (bool, error) {
		s, err := w.state(c)
		if err != nil {
			return false, err
		}
		last = s
		return s.ChecksComplete || s.Copyright != "", nil
	})
	if err != nil {
		return fmt.Errorf("checks did not finish: %w (page: %s)", err, strings.Join(last.Progress, " | "))
	}
	w.out.Checks = "complete, no issues found"
	if last.Copyright != "" {
		w.out.Checks = last.Copyright
		w.warn("checks: %s", last.Copyright)
	}
	w.note("checks: %s", w.out.Checks)
	w.shot(ctx, "06-checks")
	return nil
}

// setVisibility chooses the final mode. The instant choices are clicked by their label — the channel that was
// verified live — and confirmed by the radio's own state rather than by the click reply.
func (w *wizard) setVisibility(ctx context.Context) error {
	w.step++
	mode := strings.ToLower(strings.TrimSpace(w.in.Visibility))
	if mode == "" {
		mode = "private"
	}
	w.out.Visibility = mode

	if mode != "schedule" {
		label := strings.ToUpper(mode[:1]) + mode[1:]
		loc := youtube.Locator{Root: locPrivacyRads, ByText: label}
		f, err := w.p.Find(ctx, loc)
		if err != nil {
			return err
		}
		if !f.OK {
			loc = youtube.Locator{Root: locVisibility, ByText: label}
			if f, err = w.p.Find(ctx, loc); err != nil {
				return err
			}
		}
		w.out.Record("visibility", f)
		if !f.OK {
			return fmt.Errorf("%w: no %q choice in the visibility panel", youtube.ErrElement, label)
		}
		if _, err := w.p.Click(ctx, loc, "", 0); err != nil {
			return fmt.Errorf("choose %s: %w", mode, err)
		}
		if _, err := w.p.WaitState(ctx, loc, w.t.Action, func(g youtube.Finding) bool {
			return g.OK && g.State.Checked != nil && *g.State.Checked
		}); err != nil {
			return fmt.Errorf("%s never became the selected visibility: %w", mode, err)
		}
		w.note("visibility: %s", mode)
		w.shot(ctx, "07-visibility")
		return nil
	}

	if w.in.PublishAt.IsZero() {
		return fmt.Errorf("visibility=schedule needs publish_at")
	}
	sched := youtube.Locator{Root: locVisibility, ByText: "Schedule"}
	if _, err := w.p.Find(ctx, sched); err != nil {
		return err
	}
	if _, err := w.p.Click(ctx, sched, "Schedule as", w.t.Action); err != nil {
		return fmt.Errorf("select the schedule mode: %w", err)
	}
	w.note("visibility: schedule")
	if err := w.scheduleDate(ctx); err != nil {
		return err
	}
	w.scheduleTime(ctx)
	w.shot(ctx, "07-visibility-scheduled")
	return nil
}

// scheduleDate commits the publish date by clicking the calendar cell. The picker's text field is display
// only: writing it shows the new text and then reverts when the dialog closes, which the live run recorded
// four ways. A DOM click on the cell is what the model accepts, and it needs no OS focus.
func (w *wizard) scheduleDate(ctx context.Context) error {
	trigger := youtube.Locator{CSS: locDateTrigger}
	f, err := w.p.Find(ctx, trigger)
	w.out.Record("date trigger", f)
	if err != nil {
		return err
	}
	if !f.OK {
		return fmt.Errorf("%w: %s", youtube.ErrElement, locDateTrigger)
	}
	before := f.Text
	if _, err := w.p.Click(ctx, trigger, "", 0); err != nil {
		return fmt.Errorf("open the date picker: %w", err)
	}
	if _, err := w.p.WaitState(ctx, youtube.Locator{CSS: locCalendarDay}, w.t.Action, func(g youtube.Finding) bool {
		return g.OK
	}); err != nil {
		return fmt.Errorf("the calendar never rendered: %w", err)
	}
	var cal struct {
		Clicked   bool   `json:"clicked"`
		Committed bool   `json:"committed"`
		After     string `json:"after"`
		Reason    string `json:"reason"`
	}
	if err := w.p.Run(ctx, "calendar.js", map[string]any{
		"month":   strings.ToUpper(w.in.PublishAt.Format("Jan 2006")),
		"day":     fmt.Sprint(w.in.PublishAt.Day()),
		"trigger": locDateTrigger,
	}, &cal); err != nil {
		return err
	}
	if !cal.Clicked {
		return fmt.Errorf("%w: %s is not selectable in the open calendar (%s)", youtube.ErrElement, w.in.PublishAt.Format("2 Jan 2006"), cal.Reason)
	}
	if !cal.Committed {
		return fmt.Errorf("%w: the schedule date still reads %q after clicking %s", youtube.ErrWizardStuck, cal.After, w.in.PublishAt.Format("2 Jan 2006"))
	}
	w.out.ScheduledFor = cal.After
	w.note("scheduled date: %s (was %q)", cal.After, before)
	return nil
}

// scheduleTime enters the clock time, and says plainly that it cannot be trusted: in the live run a typed
// time displayed in the field while the committed value stayed at the picker default. The truth is read back
// from the page after the commit and compared, so the reply states what YouTube will actually do.
func (w *wizard) scheduleTime(ctx context.Context) {
	w.step++
	want := clockTime(w.in.PublishAt)
	loc := youtube.Locator{CSS: locTimeInput}
	_, read, err := w.p.Type(ctx, loc, want)
	if err != nil {
		w.warn("the schedule time could not be entered (%v) — it stays at the picker default; verify before publishing", err)
		return
	}
	w.note("time typed as %q, field reads %q", want, read)
	if !sameClock(read, want) {
		w.warn("the time field reads %q rather than %q — the committed schedule is the one in scheduledFor", read, want)
	}
}

// commit presses the footer button, whose label is contextual (Publish / Save / Schedule), and reads the
// video id off the result dialog. Never retried: a second press would publish or schedule a second time.
func (w *wizard) commit(ctx context.Context) error {
	w.step++
	done := youtube.Locator{CSS: locDone}
	f, err := w.p.Find(ctx, done)
	w.out.Record("commit", f)
	if err != nil {
		return err
	}
	if !f.OK {
		return fmt.Errorf("%w: %s", youtube.ErrElement, locDone)
	}
	label := f.Text
	if _, err := w.p.Click(ctx, done, "", 0); err != nil {
		return fmt.Errorf("press %s: %w", label, err)
	}
	w.note("pressed %s", label)

	state, err := w.state(ctx)
	if err != nil {
		return err
	}
	if state.VideoID == "" {
		if _, err := w.p.WaitAny(ctx, []string{"Video link", "youtu.be/", "Video published", "Video scheduled", "All done"}, w.t.Wizard); err != nil {
			return fmt.Errorf("%w: pressed %s and the page never confirmed it (%s)", youtube.ErrWizardStuck, label, strings.Join(state.Progress, " | "))
		}
		if state, err = w.state(ctx); err != nil {
			return err
		}
	}
	w.out.VideoID = state.VideoID
	w.out.URL = firstLink(state.Links)
	if state.Scheduled != "" {
		w.out.ScheduledFor = state.Scheduled
	}
	w.shot(ctx, "08-result")
	if w.out.VideoID == "" {
		return fmt.Errorf("%w: the wizard committed but no video id surfaced — the upload is real, check the channel content page", youtube.ErrWait)
	}
	if want := clockTime(w.in.PublishAt); w.in.Visibility == "schedule" && w.out.ScheduledFor != "" && !strings.Contains(w.out.ScheduledFor, want) {
		w.warn("the page schedules %q, not the requested %s — the time field does not commit from the DOM", w.out.ScheduledFor, want)
	}
	w.note("video %s → %s", w.out.VideoID, w.out.URL)
	return nil
}

// withID attaches the video id the page revealed to a post-attach failure, so the caller can record the
// draft rather than leave the slot unbound.
func (w *wizard) withID(ctx context.Context, err error) error {
	if w.out.VideoID != "" {
		return fmt.Errorf("%w (video id %s)", err, w.out.VideoID)
	}
	state, serr := w.state(ctx)
	if serr == nil && state.VideoID != "" {
		w.out.VideoID = state.VideoID
		w.out.URL = firstLink(state.Links)
		return fmt.Errorf("%w (video id %s)", err, state.VideoID)
	}
	return err
}

type wizardState struct {
	Dialog         bool     `json:"dialog"`
	Panel          string   `json:"panel"`
	Progress       []string `json:"progress"`
	Saved          bool     `json:"saved"`
	ChecksComplete bool     `json:"checksComplete"`
	Copyright      string   `json:"copyright"`
	Scheduled      string   `json:"scheduled"`
	Alerts         []string `json:"alerts"`
	Share          string   `json:"share"`
	VideoID        string   `json:"videoId"`
	Links          []string `json:"links"`
	URL            string   `json:"url"`
}

// clockTime formats a time the way Studio's field displays it, with the narrow no-break space (U+202F) the
// page actually uses between the minutes and the meridiem.
func clockTime(t time.Time) string {
	s := t.Format("3:04 PM")
	if i := strings.Index(s, " "); i >= 0 {
		return s[:i] + " " + s[i+1:]
	}
	return s
}

func sameClock(got, want string) bool {
	norm := func(s string) string {
		s = strings.ReplaceAll(s, " ", " ")
		s = strings.ReplaceAll(s, " ", "")
		return strings.ToLower(strings.TrimSpace(s))
	}
	return norm(got) == norm(want)
}

// firstNonEmpty returns the first value with content, for error text that must say something true.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstLink(links []string) string {
	if len(links) == 0 {
		return ""
	}
	return links[0]
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
