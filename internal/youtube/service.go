package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Settings is the resolved youtube tool configuration. The tools layer builds it from mcpcfg so this
// package never depends on config plumbing (plan §2: "a missing config key never silently selects the
// wrong identity" — that refusal happens here, in MustIdentity).
type Settings struct {
	Container         string
	ContainerID       int
	MaxUploadsPerDay  int
	DefaultVisibility string
	Timezone          string
	AllowFocusBound   bool
	PageTimeout       time.Duration
	WizardTimeout     time.Duration
	ProcessTimeout    time.Duration
	MaxUploadBytes    int64
	MaxDiagBytes      int
	MaxComments       int
	// MaxScrolls is the runaway guard on a lazy-render scroll loop (youtube.maxScrolls); the flow falls back
	// to its own floor when it is unset.
	MaxScrolls  int
	EvidenceDir string
}

// Service owns everything a flow must not see: the bridge, the journal, the diagnostics, the clock. A tool
// handler calls exactly one method on it. Flows are passed in as functions because this package cannot
// import internal/youtube/pages — pages imports *this* package for its types, and that direction is what
// keeps orchestration out of the flows.
type Service struct {
	br      Browser
	set     Settings
	journal *Journal
	log     *slog.Logger
	last    *Diag
}

func NewService(br Browser, set Settings, j *Journal, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{br: br, set: set, journal: j, log: log}
}

func (s *Service) Settings() Settings { return s.set }

// ErrNoIdentity is the refusal that makes the tool safe to expose: with no container configured the tool
// would otherwise drive whatever identity the browser happens to have focused.
var ErrNoIdentity = fmt.Errorf("youtube: no channel identity configured — set youtube.container in config.json to the Firefox container that holds the channel")

// CheckIdentity validates the configured identity the way the plan requires: the container name is set, and
// the numeric id it must resolve to is known, so a misconfigured CONTAINER_MAP cannot silently move the tool
// into another account.
func (s *Service) CheckIdentity() error {
	if s.set.Container == "" {
		return ErrNoIdentity
	}
	if s.set.ContainerID == 0 {
		return fmt.Errorf("%w: youtube.container %q has no youtube.containerId, and guessing it from the browser's current identity is exactly what this tool must not do", ErrIdentity, s.set.Container)
	}
	return nil
}

// Tab creates this action's own tab (R7). A wizard gets a foreground tab because the upload page is throttled
// to death in a background one; everything else opens in the background so the human's cursor is never
// disturbed. allowFocusBoundActions gates the foreground variant — creating a tab steals focus for a moment,
// and the tool must say so rather than do it silently.
func (s *Service) Tab(ctx context.Context, rawURL string, foreground bool) (*Page, error) {
	if err := s.CheckIdentity(); err != nil {
		return nil, err
	}
	if foreground && !s.set.AllowFocusBound {
		foreground = false
	}
	diag, err := NewDiag(s.set.Container, s.set.MaxDiagBytes, s.set.EvidenceDir)
	if err != nil {
		return nil, err
	}
	p := NewPage(s.br, Tab{}, diag)
	p.RequireIdentity(s.set.ContainerID, "")
	p.actionTimeout = s.pageTimeout()

	params := map[string]any{"url": rawURL, "container": s.set.Container, "timeout": int(s.pageTimeout() / time.Millisecond)}
	if foreground {
		params["new_tab"] = true
	} else {
		params["back_tab"] = true
	}
	reply, err := s.br.Call(ctx, "navigate", params)
	if err != nil {
		return nil, Classify(err.Error())
	}
	if bad := bridgeError(reply); bad != "" {
		return nil, Classify(bad)
	}
	t := parseTab(reply)
	if t.ID == "" {
		raw, _ := json.Marshal(reply)
		return nil, ErrIdentityMismatch{Want: "a tab id from navigate", Got: "reply carried none: " + truncate(string(raw), 200)}
	}
	if t.ContainerID != s.set.ContainerID {
		return nil, ErrIdentityMismatch{
			Want: fmt.Sprintf("containerId %d (%s)", s.set.ContainerID, s.set.Container),
			Got:  fmt.Sprintf("containerId %d on tab %s", t.ContainerID, t.ID),
		}
	}
	diag.Note("navigate", rawURL, fmt.Sprintf("tab=%s containerId=%d", t.ID, t.ContainerID))
	p.tab = t
	s.last = diag
	return p, nil
}

// Upload runs the upload flow inside the state machine the plan fixes: claim a slot, attach, complete. The
// flow is injected so this package does not import the flows (see Service's comment).
//
// The claim is taken *before* the flow runs, because the attach itself creates the video. A run that dies
// afterwards leaves a real draft on the channel, and the slot stays charged either way.
func (s *Service) Upload(ctx context.Context, in Upload, flow func(context.Context, *Page, Upload, Timings) (Outcome, error)) (Outcome, Tab, error) {
	var out Outcome
	if err := s.CheckIdentity(); err != nil {
		return out, Tab{}, err
	}
	if in.File == "" {
		return out, Tab{}, fmt.Errorf("upload needs a video file")
	}
	if !in.DryRun && s.set.MaxUploadBytes > 0 {
		if size, err := fileSize(in.File); err != nil {
			return out, Tab{}, err
		} else if size > s.set.MaxUploadBytes {
			return out, Tab{}, fmt.Errorf("%w: %s is %d bytes, the cap is %d", ErrUploadTooLarge, in.File, size, s.set.MaxUploadBytes)
		}
	}
	if in.Visibility == "" {
		in.Visibility = s.set.DefaultVisibility
	}

	// Claim first, then touch the browser: the cap must hold even for a run that never reached the wizard,
	// and a run that dies mid-flight leaves a slot that is visibly abandoned rather than invisible.
	var slot JournalSlot
	if !in.DryRun {
		claimed, cerr := s.journal.Claim("upload")
		if cerr != nil {
			return out, Tab{}, cerr
		}
		slot = claimed
	}

	page, err := s.Tab(ctx, "about:blank", in.Visibility == "schedule" || !in.DryRun)
	if err != nil {
		// Nothing was created: no page was reached, so no video can exist. Releasing keeps a bridge outage
		// from spending the day's upload, while the event stays in the ledger.
		if slot.SlotID != "" {
			_ = s.journal.Release(slot.SlotID, "could not open a tab: "+err.Error())
		}
		return out, Tab{}, err
	}
	page.RequireIdentity(s.set.ContainerID, s.channelHint())
	if dir := s.EvidenceDirFor("upload"); dir != "" {
		in.Evidence = dir
	}

	timings := Timings{Action: s.pageTimeout(), Wizard: s.wizardTimeout()}
	out, err = flow(ctx, page, in, timings)

	if !in.DryRun {
		switch {
		case err == nil && out.VideoID != "":
			if jerr := s.journal.Complete(slot.SlotID, out.VideoID); jerr != nil {
				s.log.Warn("journal complete failed", "slot", slot.SlotID, "video", out.VideoID, "err", jerr)
				out.Warnings = append(out.Warnings, "journal could not record the completed upload: "+jerr.Error())
			}
		case err != nil && !out.Attached && out.VideoID == "":
			// The run never reached the attach and no video id surfaced, so nothing exists on the channel:
			// give the slot back rather than spend the day on a failure that created nothing.
			if jerr := s.journal.Release(slot.SlotID, err.Error()); jerr != nil {
				s.log.Warn("journal release failed", "slot", slot.SlotID, "err", jerr)
			}
		case err != nil:
			id := out.VideoID
			if id != "" {
				if jerr := s.journal.Complete(slot.SlotID, id); jerr != nil {
					s.log.Warn("journal bind failed", "slot", slot.SlotID, "video", id, "err", jerr)
				}
			}
			if jerr := s.journal.Abandon(slot.SlotID, id, err.Error()); jerr != nil {
				s.log.Warn("journal abandon failed", "slot", slot.SlotID, "err", jerr)
			}
		}
	}
	return out, page.Tab(), err
}

// Watch runs one watch-page action. Unlike Upload it claims no journal slot here: an upload's mutation is its
// very first step, the attach, so the claim belongs outside the flow — while a comment's mutation is one deep
// press, so §4.6's claim-before-mutate order puts the claim inside the flow, next to that press. The ledger is
// still owned here: the flow is handed the journal and nothing else, and a dry run is handed none at all.
func (s *Service) Watch(ctx context.Context, in WatchInput, flow func(context.Context, *Page, WatchInput, Timings) (Outcome, error)) (Outcome, Tab, error) {
	if err := s.CheckIdentity(); err != nil {
		return Outcome{}, Tab{}, err
	}
	if in.VideoID == "" {
		return Outcome{}, Tab{}, fmt.Errorf("%w: a watch page action needs a video_id", ErrUsage)
	}
	switch in.Action {
	case "read", "comment", "reply":
	default:
		return Outcome{}, Tab{}, fmt.Errorf(`%w: watch action %q is not read, comment or reply`, ErrUsage, in.Action)
	}
	if in.Action != "read" {
		if strings.TrimSpace(in.Text) == "" {
			return Outcome{}, Tab{}, fmt.Errorf("%w: %s needs text", ErrUsage, in.Action)
		}
		if in.Action == "reply" && strings.TrimSpace(in.CommentID) == "" {
			return Outcome{}, Tab{}, fmt.Errorf("%w: reply needs comment_id, the lc id a read returned", ErrUsage)
		}
	}

	// A read is a look, so it stays in a background tab; a write needs the section to really render, and a
	// background tab is where a lazy section is least reliable — foreground, subject to allowFocusBoundActions.
	// Every watch action needs a visible tab, dry runs included: measured 2026-11-03, the same URL gave 23
	// characters of body and no ytd-comments while the document was hidden, and the section with its rows while
	// visible. A background tab is therefore not a slower option here, it is a page that never arrives — so the
	// flow asks for a foreground tab unconditionally and Service.Tab still downgrades it unless
	// allowFocusBoundActions permits that, in which case the run fails fast naming that switch.
	foreground := true
	page, err := s.Tab(ctx, "about:blank", foreground)
	if err != nil {
		return Outcome{}, Tab{}, err
	}
	page.RequireIdentity(s.set.ContainerID, s.channelHint())
	if dir := s.EvidenceDirFor(in.Action); dir != "" {
		in.Evidence = dir
	}
	if in.Action != "read" && !in.DryRun {
		in.Journal = s.journal
	}
	out, err := flow(ctx, page, in, Timings{Action: s.pageTimeout(), Wizard: s.wizardTimeout(), MaxScrolls: s.set.MaxScrolls})
	return out, page.Tab(), err
}

// Details runs one Studio details-panel action. Like Watch it claims no slot here: the mutation is one press
// deep inside the flow (Save, or Delete forever), so §4.6's claim-before-mutate order puts the claim next to
// that press, inside the flow. The ledger is still owned here — the flow is handed the journal and nothing
// else, and a dry run is handed none at all, because it presses nothing and must consume nothing.
func (s *Service) Details(ctx context.Context, in DetailsInput, flow func(context.Context, *Page, DetailsInput, Timings) (Outcome, error)) (Outcome, Tab, error) {
	if err := s.CheckIdentity(); err != nil {
		return Outcome{}, Tab{}, err
	}
	if in.VideoID == "" {
		return Outcome{}, Tab{}, fmt.Errorf("%w: a details panel action needs a video_id", ErrUsage)
	}
	switch in.Action {
	case "edit":
		if strings.TrimSpace(in.Title) == "" && strings.TrimSpace(in.Description) == "" {
			return Outcome{}, Tab{}, fmt.Errorf("%w: edit_details needs title and/or description — there would be nothing for the page to save", ErrUsage)
		}
	case "delete":
		// Checked here as well as in the flow so the refusal happens before a tab is created at all: a request
		// without the confirmation parameter must not so much as open the page whose menu holds the delete.
		if !in.Confirmed {
			return Outcome{}, Tab{}, fmt.Errorf("%w: delete_video needs yes_i_understand=true — deleting a video is permanent, and nothing was opened or pressed", ErrUsage)
		}
	default:
		return Outcome{}, Tab{}, fmt.Errorf(`%w: details action %q is not edit or delete`, ErrUsage, in.Action)
	}

	// Foreground, on the reasoning Service.Watch records (watch.md trap 6: a hidden document is where YouTube
	// defers work rather than merely delaying it) — and this flow both writes into a contenteditable and presses
	// a Lit checkbox, both of which need the page to be live. Service.Tab still downgrades this to a background
	// tab unless youtube.allowFocusBoundActions allows a foreground one, so the focus policy is unchanged.
	// UNPROBED: whether the details panel renders its fields in a hidden tab; studio-details.md never asked.
	page, err := s.Tab(ctx, "about:blank", true)
	if err != nil {
		return Outcome{}, Tab{}, err
	}
	page.RequireIdentity(s.set.ContainerID, s.channelHint())
	if dir := s.EvidenceDirFor(in.Action); dir != "" {
		in.Evidence = dir
	}
	if !in.DryRun {
		in.Journal = s.journal
	}
	out, err := flow(ctx, page, in, Timings{Action: s.pageTimeout(), Wizard: s.wizardTimeout(), MaxScrolls: s.set.MaxScrolls})
	return out, page.Tab(), err
}

// TodayUploads is how much of the daily cap the journal says is spent, including abandoned slots.
func (s *Service) TodayUploads() int { return s.journal.TodayUploads() }

// UploadLimit is the cap the journal enforces — the journal owns it, not the caller.
func (s *Service) UploadLimit() int { return s.journal.Limit() }

// SlotForVideo looks a recorded upload up by the id the page reported.
func (s *Service) SlotForVideo(videoID string) (JournalSlot, bool) {
	return s.journal.SlotForVideo(videoID)
}

// Trail is the bounded diagnostic record of the action just run.
func (s *Service) Trail(n int) []DiagEntry {
	if s.last == nil {
		return nil
	}
	return s.last.Tail(n)
}

// Screenshots lists the evidence files the last action wrote.
func (s *Service) Screenshots() []string {
	if s.last == nil {
		return nil
	}
	return s.last.Screenshots()
}

func (s *Service) pageTimeout() time.Duration {
	if s.set.PageTimeout > 0 {
		return s.set.PageTimeout
	}
	return 45 * time.Second
}

func (s *Service) wizardTimeout() time.Duration {
	if s.set.WizardTimeout > 0 {
		return s.set.WizardTimeout
	}
	return 15 * time.Minute
}

// channelHint is the channel id this tool will act as, if it knows it. It is derived from the browser, not
// from config: the wizard's redirect puts it in the URL, and Open asserts it. Empty until Stage 2 freezes a
// way to read it from the Studio home page without uploading.
func (s *Service) channelHint() string { return "" }

func fileSize(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("video file: %w", err)
	}
	if st.IsDir() {
		return 0, fmt.Errorf("video file %s is a directory", path)
	}
	return st.Size(), nil
}

// EvidenceDirFor prepares a per-run screenshot directory and returns it. Creating it here rather than in the
// flow keeps the page layer free of the filesystem.
func (s *Service) EvidenceDirFor(prefix string) string {
	if s.set.EvidenceDir == "" {
		return ""
	}
	full := filepath.Join(s.set.EvidenceDir, prefix+"-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(full, 0o755); err != nil {
		s.log.Warn("evidence dir", "path", full, "err", err)
		return ""
	}
	return full
}
