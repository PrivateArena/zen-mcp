package pages_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zen-mcp/internal/youtube"
	"zen-mcp/internal/youtube/pages"
)

// A scripted Studio that behaves the way the live wizard behaved (docs/youtube-pages/upload-wizard.md):
// the transfer only reports after the attach, the panel only advances when the COPPA question is answered,
// and the schedule date only commits when the calendar is clicked. So a flow that passes here cannot be
// relying on a click's success flag, and one that fails here is failing for a real page reason.
type studio struct {
	t *testing.T

	calls      []string
	attaches   int
	clicks     []string
	panels     []string
	closeCount int

	panel         string
	audience      bool
	checked       bool
	checksDone    bool
	dateCommitted bool
	dateText      string
	timeText      string
	doneClicked   bool
	shareVideo    string
	failFill      bool
	hideFileInput bool
	nextDisabled  bool
	evidence      string
	wizardTimeout time.Duration
}

func (s *studio) Call(ctx context.Context, action string, params map[string]any) (map[string]any, error) {
	s.calls = append(s.calls, action)
	switch action {
	case "navigate":
		return ok(map[string]any{"id": "t_wiz", "containerId": 20, "status": 200,
			"url": "https://studio.youtube.com/channel/UCZZei3idefNE6mAUXs2Eh8A/videos/upload"}), nil
	case "upload":
		if s.hideFileInput {
			return fail("Element not found: no file input in this dialog"), nil
		}
		s.attaches++
		return ok(map[string]any{"success": true, "filename": "output.mp4", "size": 301582}), nil
	case "click":
		return s.click(params)
	case "get_text":
		return ok(map[string]any{"el": map[string]any{"tag": "div", "visible": true}, "text": "Next"}), nil
	case "close_tab":
		s.closeCount++
		return ok(nil), nil
	case "screenshot":
		path, _ := params["path"].(string)
		if !strings.HasPrefix(path, "/") {
			return fail("path must be absolute"), nil
		}
		s.evidence = path
		return ok(nil), nil
	case "web_eval":
		return s.eval(params)
	}
	return ok(nil), nil
}

func (s *studio) click(params map[string]any) (map[string]any, error) {
	label := describe(params)
	s.clicks = append(s.clicks, label)
	switch {
	case strings.Contains(label, "made-for-kids") || strings.Contains(label, "made for kids"):
		s.audience = true
	case label == "calendar-cell" || strings.Contains(label, "datepicker"):
		// Opening the picker; the date itself is committed by calendar.js.
	case strings.Contains(label, "#done-button"):
		s.doneClicked = true
		if s.shareVideo == "" {
			s.shareVideo = "BeCRc-x5B6E" // Studio mints an id the moment the wizard commits
		}
	case strings.Contains(label, "#next-button"):
		if !s.audience {
			// The live wizard does not move and prints only inline text.
			s.nextDisabled = true
		} else {
			s.advance()
		}
	case strings.Contains(label, "Private") || strings.Contains(label, "Unlisted") || strings.Contains(label, "Public"):
		s.checked = true
	}
	return ok(map[string]any{"el": map[string]any{"tag": "button", "text": label}}), nil
}

func (s *studio) advance() {
	switch s.panel {
	case "Details":
		s.panel = "Video elements"
	case "Video elements":
		s.panel = "Checks"
		s.checksDone = true
	case "Checks":
		s.panel = "Visibility"
	}
	s.panels = append(s.panels, s.panel)
}

func (s *studio) eval(params map[string]any) (map[string]any, error) {
	code, _ := params["code"].(string)
	// Dispatch on the opening comment of each embedded program — the one string per script that is stable
	// while its body is edited. An unrecognised payload is a hard failure, so a renamed script cannot
	// silently fall through to another branch.
	switch {
	case strings.Contains(code, "cheap verification channel"): // body_text.js
		return ok(map[string]any{"result": map[string]any{
			"url":  "https://studio.youtube.com/channel/UCZZei3idefNE6mAUXs2Eh8A/videos/upload",
			"text": s.bodyText(), "len": 40,
		}}), nil

	case strings.Contains(code, "one verification probe"): // wizard_state.js
		state := map[string]any{
			"panel": s.panel, "videoId": s.shareVideo, "saved": true,
			"links": linkIf(s.shareVideo), "checksComplete": s.checksDone,
			"progress": []string{"Upload complete ... Processing will begin shortly"},
		}
		if s.doneClicked {
			state["scheduled"] = "Video scheduled Your video will be set to public on November 9, 2026 at 12:00 AM"
		}
		return ok(map[string]any{"result": state}), nil

	case strings.Contains(code, "deep-query probe"): // lookup.js
		return ok(map[string]any{"result": s.lookup(code)}), nil

	case strings.Contains(code, "Text entry with proof"): // fill.js
		var args struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(payload(code), &args); err != nil {
			s.t.Fatalf("fill payload: %v", err)
		}
		if s.failFill {
			return ok(map[string]any{"result": map[string]any{"ok": false, "method": "insertText", "text": "", "err": "read-back differs"}}), nil
		}
		s.noteFilled(args.Text)
		return ok(map[string]any{"result": map[string]any{"ok": true, "method": "insertText", "text": args.Text}}), nil

	case strings.Contains(code, "Commits a date"): // calendar.js
		var args map[string]any
		if err := json.Unmarshal(payload(code), &args); err != nil {
			s.t.Fatalf("calendar payload: %v", err)
		}
		want := fmt.Sprintf("%v %v", args["day"], titleCase(fmt.Sprint(args["month"])))
		if !s.dateCommitted {
			return ok(map[string]any{"result": map[string]any{"clicked": false, "committed": false, "reason": "the picker is not open"}}), nil
		}
		s.dateText = want
		return ok(map[string]any{"result": map[string]any{"clicked": true, "committed": true, "after": want, "before": "Nov 3, 2026"}}), nil
	}
	s.t.Fatalf("unrecognised web_eval payload:\n%s", code)
	return nil, nil
}

func (s *studio) noteFilled(text string) {
	if strings.Contains(text, "AM") || strings.Contains(text, "PM") {
		s.timeText = text
	}
}

func (s *studio) lookup(code string) map[string]any {
	var args struct {
		L struct {
			CSS    string `json:"css"`
			Root   string `json:"root"`
			ByText string `json:"text"`
		} `json:"l"`
	}
	if err := json.Unmarshal(payload(code), &args); err != nil {
		s.t.Fatalf("lookup payload: %v", err)
	}
	l := args.L
	found := func(extra ...map[string]any) map[string]any {
		base := map[string]any{
			"ok": true, "count": 1, "css": l.CSS, "tag": "div",
			"text": labelFor(l), "state": map[string]any{"exists": true, "visible": true, "disabled": false},
		}
		for _, e := range extra {
			for k, v := range e {
				base[k] = v
			}
		}
		return base
	}
	switch {
	case strings.Contains(l.CSS, "input[type=file]"):
		if s.hideFileInput {
			return map[string]any{"ok": false, "count": 0, "reason": "no element"}
		}
		return found(map[string]any{"state": map[string]any{"exists": true, "visible": false, "disabled": false}})
	case strings.Contains(l.CSS, "#done-button"):
		label := s.commitLabel()
		return found(map[string]any{"text": label, "state": map[string]any{"exists": true, "visible": true, "disabled": false}})
	case strings.Contains(l.CSS, "#next-button"):
		return found(map[string]any{"state": map[string]any{"exists": true, "visible": true, "disabled": s.nextDisabled}})
	case strings.Contains(l.CSS, "#datepicker-trigger"):
		return found(map[string]any{"text": "Nov 3, 2026"})
	case strings.Contains(l.CSS, ".calendar-day"):
		return found()
	case l.ByText != "" && strings.Contains(l.Root, "made-for-kids"):
		return found(map[string]any{"state": map[string]any{"exists": true, "visible": true, "checked": s.audience}})
	case l.ByText != "":
		return found(map[string]any{"state": map[string]any{"exists": true, "visible": true, "checked": s.checked}})
	}
	return found()
}

// commitLabel is what the footer button says in the current mode.
func (s *studio) commitLabel() string {
	if s.panel == "Visibility" && s.dateCommitted {
		return "Schedule"
	}
	return "Publish"
}

func (s *studio) bodyText() string {
	texts := []string{"output (upload probe)", "Saved as private", "Upload complete ... Processing will begin shortly"}
	if s.checksDone {
		texts = append(texts, "Checks complete. No issues found.")
	}
	if s.panel == "Visibility" {
		texts = append(texts, "Schedule as public", s.dateText)
	}
	return strings.Join(texts, " ")
}

// page is a fresh wizard in the given starting state.
func (s *studio) page() *youtube.Page {
	diag, err := youtube.NewDiag("pachoigame", 8192, s.t.TempDir())
	if err != nil {
		s.t.Fatal(err)
	}
	p := youtube.NewPage(s, youtube.Tab{ID: "t_wiz", ContainerID: 20}, diag)
	p.RequireIdentity(20, "UCZZei3idefNE6mAUXs2Eh8A")
	return p
}

func TestUploadWizardHappyPath(t *testing.T) {
	s := &studio{t: t, panel: "Details"}
	in := youtube.Upload{
		File: fixture(t), Title: "output (upload probe)", Description: "probe run",
		MadeForKids: boolPtr(false), Visibility: "private", Evidence: t.TempDir(),
	}
	out, err := pages.UploadWizard(context.Background(), s.page(), in, youtube.Timings{Action: 200 * time.Millisecond, Wizard: 2 * time.Second})
	if err != nil {
		t.Fatalf("wizard: %v (steps %v)", err, out.Steps)
	}
	if s.attaches != 1 {
		t.Fatalf("attach calls = %d, want exactly 1 (a retry would create a second video)", s.attaches)
	}
	if s.panel != "Visibility" {
		t.Fatalf("ended on panel %q", s.panel)
	}
	if !s.doneClicked {
		t.Fatal("the commit button was never pressed")
	}
	// The order is the contract: details are filled and the audience answered before the first Next.
	if err := order(t, s.clicks, "made for kids", "#next-button"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(s.calls, ","), "screenshot") {
		t.Fatal("no evidence screenshot was written")
	}
	if !strings.HasSuffix(s.evidence, ".png") || !strings.HasPrefix(s.evidence, "/") {
		t.Fatalf("evidence path = %q, want an absolute .png (a relative one silently writes nothing)", s.evidence)
	}
	if out.Checks == "" {
		t.Error("the checks result is missing from the outcome")
	}
}

// §6: a scheduled upload must never be committed for a date the caller did not ask for.
func TestUploadWizardRefusesAWrongScheduleDate(t *testing.T) {
	s := &studio{t: t, panel: "Details", dateCommitted: false}
	in := youtube.Upload{
		File: fixture(t), MadeForKids: boolPtr(false), Visibility: "schedule",
		PublishAt: time.Date(2026, 11, 9, 10, 0, 0, 0, time.FixedZone("ICT", 7*3600)),
	}
	_, err := pages.UploadWizard(context.Background(), s.page(), in, youtube.Timings{Action: 200 * time.Millisecond, Wizard: time.Second})
	if err == nil {
		t.Fatal("the wizard committed a schedule whose date never committed")
	}
	if s.doneClicked {
		t.Error("#done-button was pressed even though the date did not take — that is the double-publish bug")
	}
	if !errors.Is(err, youtube.ErrWizardStuck) && !errors.Is(err, youtube.ErrElement) {
		t.Fatalf("err = %v, want a stuck-wizard or element error", err)
	}
}

func TestUploadWizardClicksTheCalendarForTheRightDay(t *testing.T) {
	s := &studio{t: t, panel: "Details", dateCommitted: true, shareVideo: "BeCRc-x5B6E"}
	in := youtube.Upload{
		File: fixture(t), MadeForKids: boolPtr(false), Visibility: "schedule",
		PublishAt: time.Date(2026, 11, 9, 10, 0, 0, 0, time.FixedZone("ICT", 7*3600)),
	}
	out, err := pages.UploadWizard(context.Background(), s.page(), in, youtube.Timings{Action: 200 * time.Millisecond, Wizard: 2 * time.Second})
	if err != nil {
		t.Fatalf("wizard: %v (steps %v)", err, out.Steps)
	}
	if s.dateText != "9 Nov 2026" {
		t.Fatalf("committed date = %q, want the clicked 9 Nov 2026", s.dateText)
	}
	if out.VideoID != "BeCRc-x5B6E" {
		t.Fatalf("outcome = %+v, want the id the page reported", out)
	}
	if !strings.Contains(s.commitLabel(), "Schedule") {
		t.Fatalf("the button label seen was %q", s.commitLabel())
	}
	// The page's own words are the last word on the schedule, so a mismatch is surfaced.
	if len(out.Warnings) == 0 {
		t.Error("the page said 12:00 AM while the request said 10:00 AM, and nothing warned about it")
	}
}

// The COPPA gate. Without an answer the wizard sits on Details with nothing but inline text, so the flow
// must refuse before pressing Next rather than discover it three clicks later.
func TestUploadWizardRequiresTheAudienceAnswer(t *testing.T) {
	s := &studio{t: t, panel: "Details"}
	in := youtube.Upload{File: fixture(t), Visibility: "private"} // MadeForKids unset
	_, err := pages.UploadWizard(context.Background(), s.page(), in, youtube.Timings{Action: 200 * time.Millisecond, Wizard: time.Second})
	if !errors.Is(err, youtube.ErrWizardStuck) {
		t.Fatalf("err = %v, want the stuck-wizard sentinel naming the required question", err)
	}
	if !strings.Contains(err.Error(), "made_for_kids") {
		t.Fatalf("err = %v, want it to name the parameter the caller must set", err)
	}
	if s.attaches != 1 {
		t.Fatalf("attaches = %d, want the attach to have happened before the refusal", s.attaches)
	}
	for _, c := range s.clicks {
		if strings.Contains(c, "Next") {
			t.Fatal("it pressed Next without answering the required question")
		}
	}
}

// A contenteditable that reports success while writing nothing is the failure mode that made the live run
// lose a title; the flow must catch it from the read-back.
func TestUploadWizardCatchesAWriteThatDidNotLand(t *testing.T) {
	s := &studio{t: t, panel: "Details", failFill: true}
	in := youtube.Upload{File: fixture(t), Title: "output (upload probe)", MadeForKids: boolPtr(false), Visibility: "private"}
	_, err := pages.UploadWizard(context.Background(), s.page(), in, youtube.Timings{Action: 200 * time.Millisecond, Wizard: time.Second})
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("err = %v, want the title read-back failure", err)
	}
}

// A dry run must stop at the attach target.
func TestUploadWizardDryRunNeverAttaches(t *testing.T) {
	s := &studio{t: t, panel: "Details"}
	in := youtube.Upload{File: fixture(t), MadeForKids: boolPtr(false), Visibility: "private", DryRun: true}
	out, err := pages.UploadWizard(context.Background(), s.page(), in, youtube.Timings{Action: 200 * time.Millisecond, Wizard: time.Second})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if s.attaches != 0 {
		t.Fatalf("a dry run attached %d file(s)", s.attaches)
	}
	if len(out.Findings) == 0 {
		t.Error("a dry run returned no locator findings, so --explain has nothing to show")
	}
}

// If the dialog never offers a file input, that is the wizard-not-open case, and it must be reported as
// such without touching the browser further.
func TestUploadWizardReportsAWizardThatDidNotOpen(t *testing.T) {
	s := &studio{t: t, panel: "Details", hideFileInput: true}
	in := youtube.Upload{File: fixture(t), MadeForKids: boolPtr(false), Visibility: "private"}
	_, err := pages.UploadWizard(context.Background(), s.page(), in, youtube.Timings{Action: 200 * time.Millisecond, Wizard: time.Second})
	if !errors.Is(err, youtube.ErrWizardStuck) {
		t.Fatalf("err = %v, want the stuck-wizard classification", err)
	}
	if s.attaches != 0 {
		t.Fatalf("it attached anyway: %d", s.attaches)
	}
}

func describe(params map[string]any) string {
	for _, k := range []string{"selector", "by_text"} {
		if v, ok := params[k].(string); ok && v != "" {
			return v
		}
	}
	return fmt.Sprint(params)
}

func labelFor(l struct {
	CSS    string `json:"css"`
	Root   string `json:"root"`
	ByText string `json:"text"`
}) string {
	if l.ByText != "" {
		return l.ByText
	}
	if strings.Contains(l.CSS, "#next-button") {
		return "Next"
	}
	return l.CSS
}

func order(t *testing.T, clicks []string, first, second string) error {
	i, j := -1, -1
	for n, c := range clicks {
		if i < 0 && strings.Contains(c, first) {
			i = n
		}
		if i >= 0 && j < 0 && strings.Contains(c, second) && n > i {
			j = n
		}
	}
	if i < 0 || j < 0 {
		return fmt.Errorf("clicks %v never show %q before %q", clicks, first, second)
	}
	return nil
}

func payload(code string) []byte {
	start := strings.Index(code, "let args=")
	if start < 0 {
		return []byte("{}")
	}
	rest := code[start+len("let args="):]
	depth, end := 0, 0
	for i, r := range rest {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if depth == 0 && end > 0 {
			break
		}
	}
	return []byte(rest[:end])
}

// titleCase mirrors what Studio prints in a month header: "NOV 2026" reads as "Nov 2026".
func titleCase(s string) string {
	s = strings.ToLower(s)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func linkIf(id string) []string {
	if id == "" {
		return nil
	}
	return []string{"https://youtu.be/" + id}
}

func ok(data map[string]any) map[string]any {
	if data == nil {
		data = map[string]any{}
	}
	return map[string]any{"success": true, "data": data}
}

func fail(msg string) map[string]any {
	return map[string]any{"success": false, "error": msg}
}

func boolPtr(b bool) *bool { return &b }

func fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "output.mp4")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
