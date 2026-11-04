package youtube_test

import (
	"context"
	"encoding/json"
	"errors"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zen-mcp/internal/youtube"
)

// The structural guard (plan §4.1): nothing in this package may reach YouTube except through a page visit.
// The build-tagged probe file in the same directory is the second half of this check.
func TestYoutubePackageHasNoHTTPClient(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob package files: %v", err)
	}
	bannedImports := map[string]bool{
		"net/http": true, "net/http/cookiejar": true, "crypto/hmac": true, "net/http/httputil": true,
	}
	bannedCode := []string{"youtubei", "SAPISIDHASH", "http.Client", "http.NewRequest", "document.cookie"}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || excludedByBuildTag(t, name) {
			continue
		}
		// Comments are excluded on purpose: this package's docs *name* the APIs it must never call, and
		// that is exactly the point of writing them down. Only executable text is scanned.
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, raw, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		var code strings.Builder
		if err := printer.Fprint(&code, token.NewFileSet(), file); err != nil {
			t.Fatalf("print %s: %v", name, err)
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if bannedImports[path] {
				t.Errorf("%s imports %s — this package's trust property is that everything came from a page visit", name, path)
			}
		}
		for _, b := range bannedCode {
			if strings.Contains(code.String(), b) {
				t.Errorf("%s contains %q in executable code", name, b)
			}
		}
	}
}

// excludedByBuildTag reports a file that the normal build does not compile. apiprobe_disallowed.go is such a
// file: it exists so the guard below can prove it has something to find.
func excludedByBuildTag(t *testing.T, name string) bool {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	first, _, _ := strings.Cut(string(raw), "\n")
	return strings.HasPrefix(strings.TrimSpace(first), "//go:build")
}

// The detector must be able to detect. Without this, the guard above could pass because it never looked.
func TestNoAPIGuardActuallyDetectsTheProbeFile(t *testing.T) {
	const probe = "apiprobe_disallowed.go"
	raw, err := os.ReadFile(probe)
	if err != nil {
		t.Skipf("probe file %s is absent; the guard has nothing to prove against", probe)
	}
	if !strings.Contains(string(raw), "net/http") {
		t.Fatalf("%s no longer contains the banned import, so the guard proves nothing", probe)
	}
	if !excludedByBuildTag(t, probe) {
		t.Fatalf("%s is compiled by the normal build — it must stay behind a build tag", probe)
	}
}

func TestEveryYoutubeScriptIsEmbeddedAndParseable(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; the embedded programs are linted by the same parser the browser uses")
	}
	names := youtube.Scripts()
	if len(names) < 5 {
		t.Fatalf("expected the flow programs to be embedded, found %v", names)
	}
	dir := t.TempDir()
	for _, name := range names {
		src := youtube.ScriptSource(name)
		if strings.TrimSpace(src) == "" {
			t.Fatalf("%s embeds to nothing", name)
		}
		// The embedded program is a function body, so the lint wraps it the same way Compose does.
		wrapped := youtube.Compose(src, map[string]any{"l": map[string]any{}})
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(wrapped), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		out, err := exec.Command(node, "--check", path).CombinedOutput()
		if err != nil {
			t.Errorf("%s does not parse: %v\n%s", name, err, out)
		}
	}
}

// A fragment whose first token is `return` must stay valid after wrapping — the bug that made every call
// through a hand-rolled prelude fail with "Illegal return statement".
func TestComposeKeepsTheProgramCallable(t *testing.T) {
	for _, first := range []string{"return 1;", "var x = 1;", "let y = 2;", "const z = 3;", "x = 4; return x;"} {
		got := youtube.Compose(first, map[string]any{"a": 1})
		// The bridge returns what the program's own top-level return yields, so the program must stay at the
		// top level: wrapping it in (async()=>{…})() buries the return and the call comes back with no result.
		if strings.Contains(got, "async") || strings.HasPrefix(strings.TrimSpace(got), "(") {
			t.Fatalf("%q was wrapped, which hides its return from the bridge: %s", first, got)
		}
		if !strings.HasPrefix(got, "let args = ") {
			t.Fatalf("%q lost the args prelude: %s", first, got)
		}
		if !strings.HasSuffix(got, first) {
			t.Fatalf("%q is not the program, preceded only by the prelude: %s", first, got)
		}
		if strings.Contains(got, "let args;") || strings.Contains(got, "args = ;") {
			t.Fatalf("%q got an empty-args form, which makes args undefined: %s", first, got)
		}
	}
	if got := youtube.Compose("return 1;", nil); !strings.Contains(got, "args = {}") {
		t.Fatalf("a nil payload must still define args, got %s", got)
	}
}

func TestClassifyBridgeStrings(t *testing.T) {
	cases := []struct {
		raw  string
		want error
	}{
		{"", nil},
		{"Element not found: #next-button button", youtube.ErrElement},
		{`no element for {"css":"#x"}`, youtube.ErrElement},
		{"Timeout waiting for element #done-button", youtube.ErrWait},
		{"the page never showed \"All changes saved\"", youtube.ErrWait},
		{"No tab id", youtube.ErrIdentity},
		{"expected containerId 20", youtube.ErrIdentity},
		{"not a valid TabRegistry id", youtube.ErrAbandonedTab},
		{"unauthorized", youtube.ErrAbandonedTab},
		{"bridge exploded", nil},
	}
	for _, c := range cases {
		got := youtube.Classify(c.raw)
		if c.want == nil {
			if got != nil && errors.Is(got, c.want) {
				continue
			}
			if errors.Is(got, youtube.ErrElement) || errors.Is(got, youtube.ErrWait) || errors.Is(got, youtube.ErrIdentity) || errors.Is(got, youtube.ErrAbandonedTab) {
				t.Errorf("Classify(%q) = %v, want an unclassified error", c.raw, got)
			}
			continue
		}
		if !errors.Is(got, c.want) {
			t.Errorf("Classify(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

// The typed sentinels must carry their own text and still answer to errors.Is.
func TestSentinelErrorsUnwrap(t *testing.T) {
	probe := []error{
		youtube.ErrIdentityMismatch{Want: "containerId 20", Got: "containerId 4"},
		youtube.ErrNotFound{Locator: "#next-button button"},
		youtube.ErrTimeout{What: "checks", After: "15m0s"},
		youtube.ErrAbandoned{Detail: "no such tab"},
		youtube.ErrStuck{Text: "You need to answer this question"},
	}
	want := []error{youtube.ErrIdentity, youtube.ErrElement, youtube.ErrWait, youtube.ErrAbandonedTab, youtube.ErrWizardStuck}
	for i, err := range probe {
		if !errors.Is(err, want[i]) {
			t.Errorf("%T does not unwrap to %v (%v)", err, want[i], err)
		}
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want[i].Error()[len("youtube: "):])) {
			t.Errorf("%T text %q does not name its sentinel", err, err.Error())
		}
	}
}

// §6: the daily cap, the claim-before-attach ordering, and the abandoned-upload rule.
func TestJournalChargesAbandonedUploadsAndRefusesTheNextOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	j, err := youtube.OpenJournal(path, 1, time.UTC)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	slot, err := j.Claim("upload")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if got := j.TodayUploads(); got != 1 {
		t.Fatalf("TodayUploads after one claim = %d, want 1", got)
	}
	if _, err := j.Claim("upload"); !errors.Is(err, youtube.ErrDailyLimit) {
		t.Fatalf("second claim = %v, want the daily limit", err)
	}
	// An abandoned upload is a video that exists, so it keeps costing the day.
	if err := j.Abandon(slot.SlotID, "BeCRc-x5B6E", "wizard stuck"); err != nil {
		t.Fatalf("abandon: %v", err)
	}
	if _, err := j.Claim("upload"); !errors.Is(err, youtube.ErrDailyLimit) {
		t.Fatalf("claim after an abandoned upload = %v, want the limit to still apply", err)
	}
	if s, ok := j.SlotForVideo("BeCRc-x5B6E"); !ok || s.Status != "abandoned" {
		t.Fatalf("the abandoned slot lost its video id: %+v", s)
	}

	// The ledger survives a restart, which is the only reason the cap means anything.
	again, err := youtube.OpenJournal(path, 1, time.UTC)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := again.TodayUploads(); got != 1 {
		t.Fatalf("TodayUploads after restart = %d, want 1", got)
	}
}

func TestJournalCompletesTheSlotItClaimed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	j, err := youtube.OpenJournal(path, 5, time.UTC)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	slot, err := j.Claim("upload")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := j.Complete(slot.SlotID, "vid_1"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	s, ok := j.SlotForVideo("vid_1")
	if !ok || s.Status != "completed" {
		t.Fatalf("slot = %+v (found %v), want completed", s, ok)
	}
}

// A corrupt ledger must not silently reset the cap.
func TestJournalRefusesToGuessOnACorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := youtube.OpenJournal(path, 1, time.UTC); err == nil {
		t.Fatal("a corrupt journal was accepted, which would reset the daily cap")
	}
}

// The refusal that makes the tool safe to expose at all.
func TestServiceRefusesWithoutAnIdentity(t *testing.T) {
	svc := youtube.NewService(newFakeBrowser(youtube.Tab{}), youtube.Settings{}, mustJournal(t), nil)
	if _, err := svc.Tab(context.Background(), "https://studio.youtube.com", false); !errors.Is(err, youtube.ErrNoIdentity) {
		t.Fatalf("Tab with no container = %v, want the identity refusal", err)
	}
	// A container name with no numeric id is the other half: guessing it would act as whoever is logged in.
	svc = youtube.NewService(newFakeBrowser(youtube.Tab{}), youtube.Settings{Container: "pachoigame"}, mustJournal(t), nil)
	if _, err := svc.Tab(context.Background(), "https://studio.youtube.com", false); !errors.Is(err, youtube.ErrIdentity) {
		t.Fatalf("Tab with no containerId = %v, want the identity refusal", err)
	}
}

// Stage 2: a watch action that cannot be right is refused before a tab opens, because the cheap half of the
// contract is argument-shaped and needs no page to check.
func TestServiceWatchRefusesABadRequestBeforeOpeningATab(t *testing.T) {
	br := newFakeBrowser(youtube.Tab{ID: "t_w", ContainerID: 20})
	svc := youtube.NewService(br, youtube.Settings{Container: "pachoigame", ContainerID: 20}, mustJournal(t), nil)
	for _, in := range []youtube.WatchInput{
		{Action: "read"},
		{VideoID: "YSNFGgh0aAs", Action: "top_comment"},
		{VideoID: "YSNFGgh0aAs", Action: "comment"},
		{VideoID: "YSNFGgh0aAs", Action: "reply", Text: "hi"},
	} {
		_, _, err := svc.Watch(context.Background(), in, func(context.Context, *youtube.Page, youtube.WatchInput, youtube.Timings) (youtube.Outcome, error) {
			return youtube.Outcome{}, errors.New("the flow ran for a request that should have been refused")
		})
		if !errors.Is(err, youtube.ErrUsage) {
			t.Errorf("%+v = %v, want the usage refusal", in, err)
		}
	}
	for _, c := range br.calls {
		if c == "navigate" {
			t.Fatal("a refused watch request still opened a tab")
		}
	}
}

// §6: the wizard must never commit a date it did not ask for. The service claims before the flow runs, and
// binds the id the *page* reported afterwards.
func TestServiceClaimsTheSlotBeforeTheFlowRuns(t *testing.T) {
	j := mustJournalCap(t, 1)
	browser := newFakeBrowser(youtube.Tab{ID: "t_run", ContainerID: 20, URL: "https://studio.youtube.com/channel/UCX/videos/upload"})
	svc := youtube.NewService(browser, youtube.Settings{
		Container: "pachoigame", ContainerID: 20, MaxUploadsPerDay: 1, DefaultVisibility: "private",
		PageTimeout: time.Second, WizardTimeout: time.Second, MaxDiagBytes: 4096,
	}, j, nil)

	var during int
	var seenID string
	out, _, err := svc.Upload(context.Background(), youtube.Upload{File: videoFixture(t), MadeForKids: boolPtr(false)},
		func(ctx context.Context, p *youtube.Page, in youtube.Upload, tim youtube.Timings) (youtube.Outcome, error) {
			during = svc.TodayUploads()
			seenID = p.Tab().ID
			return youtube.Outcome{VideoID: "BeCRc-x5B6E", URL: "https://youtu.be/BeCRc-x5B6E"}, nil
		})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if during != 1 {
		t.Fatalf("slots charged while the flow ran = %d, want 1 (the claim must precede the attach)", during)
	}
	if seenID != "t_run" {
		t.Fatalf("flow ran on tab %q, want the tab the service opened", seenID)
	}
	if out.VideoID != "BeCRc-x5B6E" {
		t.Fatalf("outcome = %+v", out)
	}
	slot, ok := j.SlotForVideo("BeCRc-x5B6E")
	if !ok || slot.Status != "completed" {
		t.Fatalf("journal after success = %+v (found %v), want completed", slot, ok)
	}
}

// A run that attached and then failed still leaves the video id on the record.
func TestServiceRecordsTheVideoIDOfAFailedUpload(t *testing.T) {
	j := mustJournalCap(t, 1)
	svc := youtube.NewService(newFakeBrowser(youtube.Tab{ID: "t_x", ContainerID: 20}), youtube.Settings{
		Container: "pachoigame", ContainerID: 20, MaxUploadsPerDay: 1, MaxDiagBytes: 4096,
	}, j, nil)
	_, _, err := svc.Upload(context.Background(), youtube.Upload{File: videoFixture(t), MadeForKids: boolPtr(true)},
		func(ctx context.Context, p *youtube.Page, in youtube.Upload, tim youtube.Timings) (youtube.Outcome, error) {
			return youtube.Outcome{VideoID: "VID_DRAFT"}, errors.New("wizard stuck: Next is disabled")
		})
	if err == nil {
		t.Fatal("want the flow's error")
	}
	slot, ok := j.SlotForVideo("VID_DRAFT")
	if !ok {
		t.Fatalf("the draft was not recorded: %v", err)
	}
	if slot.Status != "abandoned" {
		t.Fatalf("slot status = %q, want abandoned", slot.Status)
	}
	if got := j.TodayUploads(); got != 1 {
		t.Fatalf("the abandoned draft costs %d slots, want 1", got)
	}
	// And the cap now refuses the retry.
	_, _, err = svc.Upload(context.Background(), youtube.Upload{File: videoFixture(t)},
		func(ctx context.Context, p *youtube.Page, in youtube.Upload, tim youtube.Timings) (youtube.Outcome, error) {
			t.Error("the flow ran past the daily limit")
			return youtube.Outcome{}, nil
		})
	if !errors.Is(err, youtube.ErrDailyLimit) {
		t.Fatalf("second upload = %v, want the daily limit", err)
	}
}

// A dry run touches the browser but must not spend the one-shot slot.
func TestDryRunDoesNotConsumeTheCap(t *testing.T) {
	j := mustJournalCap(t, 1)
	svc := youtube.NewService(newFakeBrowser(youtube.Tab{ID: "t_d", ContainerID: 20}), youtube.Settings{
		Container: "pachoigame", ContainerID: 20, MaxUploadsPerDay: 1, MaxDiagBytes: 4096,
	}, j, nil)
	_, _, err := svc.Upload(context.Background(), youtube.Upload{File: videoFixture(t), DryRun: true},
		func(ctx context.Context, p *youtube.Page, in youtube.Upload, tim youtube.Timings) (youtube.Outcome, error) {
			return youtube.Outcome{Steps: []string{"dry run"}}, nil
		})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if got := j.TodayUploads(); got != 0 {
		t.Fatalf("a dry run charged %d slots", got)
	}
}

// An oversized file is refused before the browser is touched.
func TestOversizedUploadIsRefusedBeforeAnyPageWork(t *testing.T) {
	j := mustJournalCap(t, 1)
	br := newFakeBrowser(youtube.Tab{ID: "t_s", ContainerID: 20})
	svc := youtube.NewService(br, youtube.Settings{
		Container: "pachoigame", ContainerID: 20, MaxUploadsPerDay: 1, MaxUploadBytes: 8, MaxDiagBytes: 4096,
	}, j, nil)
	_, _, err := svc.Upload(context.Background(), youtube.Upload{File: videoFixture(t)},
		func(ctx context.Context, p *youtube.Page, in youtube.Upload, tim youtube.Timings) (youtube.Outcome, error) {
			t.Error("the flow ran with an oversized file")
			return youtube.Outcome{}, nil
		})
	if !errors.Is(err, youtube.ErrUploadTooLarge) {
		t.Fatalf("oversized upload = %v, want the size refusal", err)
	}
	for _, c := range br.calls {
		if c == "navigate" {
			t.Fatal("an oversized file still opened a tab")
		}
	}
}

// R1/R2: the identity assertion happens on the bridge's own reply, not on a string in the DOM.
func TestPageOpenRejectsTheWrongContainer(t *testing.T) {
	br := newFakeBrowser(youtube.Tab{ID: "t_wrong", ContainerID: 4, URL: "https://studio.youtube.com/channel/UCOTHER/videos/upload"})
	diag, err := youtube.NewDiag("pachoigame", 4096, "")
	if err != nil {
		t.Fatal(err)
	}
	p := youtube.NewPage(br, youtube.Tab{ID: "t_wrong", ContainerID: 4}, diag)
	p.RequireIdentity(20, "UCZZei3idefNE6mAUXs2Eh8A")
	if err := p.Open(context.Background(), "https://studio.youtube.com/channel/UCZZei3idefNE6mAUXs2Eh8A/videos/upload"); !errors.Is(err, youtube.ErrIdentity) {
		t.Fatalf("Open in container 4 = %v, want the identity refusal", err)
	}

	// The right container but the wrong channel is the other way this tool could act on a stranger's account.
	br = newFakeBrowser(youtube.Tab{ID: "t_ok", ContainerID: 20, URL: "https://studio.youtube.com/channel/UCOTHER/videos/upload"})
	p = youtube.NewPage(br, youtube.Tab{ID: "t_ok", ContainerID: 20}, diag)
	p.RequireIdentity(20, "UCZZei3idefNE6mAUXs2Eh8A")
	if err := p.Open(context.Background(), "https://www.youtube.com/upload"); !errors.Is(err, youtube.ErrIdentity) {
		t.Fatalf("Open on the wrong channel = %v, want the identity refusal", err)
	}
}

func TestPageAttachesOnceAndRefusesARelativePath(t *testing.T) {
	br := newFakeBrowser(youtube.Tab{ID: "t_a", ContainerID: 20})
	diag, _ := youtube.NewDiag("pachoigame", 4096, "")
	p := youtube.NewPage(br, youtube.Tab{ID: "t_a", ContainerID: 20}, diag)
	if _, err := p.Attach(context.Background(), youtube.Locator{CSS: "input[type=file]"}, "relative.mp4"); err == nil {
		t.Fatal("a relative attach path was accepted")
	}
	if _, err := p.Attach(context.Background(), youtube.Locator{CSS: "input[type=file]"}, "/definitely/not/here.mp4"); err == nil {
		t.Fatal("a missing file was accepted")
	}
	good, err := p.Attach(context.Background(), youtube.Locator{CSS: "input[type=file]"}, videoFixture(t))
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if good["success"] != true {
		t.Fatalf("attach reply = %v", good)
	}
	// The reply must not carry triggerDrop: drop plus change attaches the file twice.
	for _, c := range br.uploadParams {
		if _, ok := c["triggerDrop"]; ok {
			t.Fatalf("attach params carried triggerDrop: %v", c)
		}
		if c["path"] == "" || c["index"] != "t_a" {
			t.Fatalf("attach params = %v, want the absolute path and this tab", c)
		}
	}
}

// R5's resolver order: the bridge cannot reach every control (a Polymer-scoped radio group answers
// "Not found"), and that press was a no-op, so falling through to the page channel cannot double-press.
func TestClickFallsThroughToThePageChannel(t *testing.T) {
	br := newFakeBrowser(youtube.Tab{ID: "t_cl", ContainerID: 20})
	br.clickFails = `Error: Not found: undefined`
	diag, _ := youtube.NewDiag("pachoigame", 4096, "")
	p := youtube.NewPage(br, youtube.Tab{ID: "t_cl", ContainerID: 20}, diag)
	f, err := p.Click(context.Background(), youtube.Locator{Root: "ytkc-made-for-kids-select", ByText: "No, it's not made for kids"}, "", 0)
	if err != nil {
		t.Fatalf("click: %v", err)
	}
	if f.ClickMethod != "dom" {
		t.Fatalf("click method = %q, want the page channel after the bridge refused", f.ClickMethod)
	}
	if br.domClicks != 1 {
		t.Fatalf("page-channel presses = %d, want exactly 1", br.domClicks)
	}

	// A press that failed for another reason must not be re-attempted, or the second one could double.
	br2 := newFakeBrowser(youtube.Tab{ID: "t_cl2", ContainerID: 20})
	br2.clickFails = "Timeout waiting for the element to become clickable"
	p2 := youtube.NewPage(br2, youtube.Tab{ID: "t_cl2", ContainerID: 20}, diag)
	if _, err := p2.Click(context.Background(), youtube.Locator{CSS: "#done-button button"}, "", 0); err == nil {
		t.Fatal("a timed-out press was reported as success")
	}
	if br2.domClicks != 0 {
		t.Fatal("a press whose outcome is unknown was repeated through another channel")
	}
}

// A bridge reply can say success while carrying no result. Decoded quietly, that is a zero Finding, which
// tells the caller "the page has no such element" about an element it is showing — the failure mode that made
// a rendered Studio panel look like a missing one. It must be an error, and it must say what came back.
func TestEvalRefusesAReplyThatCarriesNoResult(t *testing.T) {
	br := newFakeBrowser(youtube.Tab{ID: "t_nr", ContainerID: 20})
	br.silent = true
	diag, _ := youtube.NewDiag("pachoigame", 4096, "")
	p := youtube.NewPage(br, youtube.Tab{ID: "t_nr", ContainerID: 20}, diag)
	_, err := p.Eval(context.Background(), "return {ok:true}", nil)
	if err == nil {
		t.Fatal("a result-less reply reported success")
	}
	if !strings.Contains(err.Error(), "no result") {
		t.Fatalf("error = %v, want it to say the reply carried no result", err)
	}
	// And the composed program is a statement list, so the bridge sees our return as its own.
	if got := youtube.Compose("return 1", map[string]any{"a": 1}); strings.Contains(got, "async") || strings.HasPrefix(got, "(") {
		t.Fatalf("composed program is wrapped, which the bridge cannot return from: %q", got)
	} else if !strings.HasPrefix(got, "let args = ") {
		t.Fatalf("composed program must open with a statement the bridge accepts, got %q", got)
	}
}

func TestPageScrollsByNumericOffsetNotAScript(t *testing.T) {
	br := newFakeBrowser(youtube.Tab{ID: "t_sc", ContainerID: 20})
	diag, _ := youtube.NewDiag("pachoigame", 4096, "")
	p := youtube.NewPage(br, youtube.Tab{ID: "t_sc", ContainerID: 20}, diag)
	if err := p.Scroll(context.Background(), youtube.Locator{CSS: "ytd-comment-thread-renderer"}, 900, "center"); err != nil {
		t.Fatalf("scroll: %v", err)
	}
	if len(br.scrollParams) != 1 {
		t.Fatalf("scroll calls = %d", len(br.scrollParams))
	}
	got := br.scrollParams[0]
	if _, isScript := got["script"]; isScript {
		t.Fatalf("scroll sent a script the bridge cannot run: %v", got)
	}
	if got["y"] != 900.0 || got["block"] != "center" || got["selector"] != "ytd-comment-thread-renderer" {
		t.Fatalf("scroll params = %v", got)
	}
}

func TestDiagIsBounded(t *testing.T) {
	diag, err := youtube.NewDiag("pachoigame", 256, "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 500; i++ {
		diag.Note("web_eval", strings.Repeat("x", 5000), strings.Repeat("y", 5000))
	}
	tail := diag.Tail(10)
	if len(tail) != 10 {
		t.Fatalf("tail = %d entries", len(tail))
	}
	for _, e := range tail {
		if len(e.Result) > 256+3 || len(e.Subject) > 256+3 {
			t.Fatalf("diagnostic entry is not bounded: %d/%d bytes", len(e.Subject), len(e.Result))
		}
	}
	if head := diag.Reqs(0, time.Minute); head != 60000 {
		t.Fatalf("Reqs with a full minute = %d ms", head)
	}
	// Shot notes the result, and Note takes the same lock — a deadlock the first screenshot of a run hits.
	diag.Shot("/tmp/01-evidence.png", nil)
	diag.Shot("/tmp/02-evidence.png", errors.New("bridge refused"))
	if got := len(diag.Screenshots()); got != 1 {
		t.Fatalf("recorded %d screenshots, want only the one that succeeded", got)
	}
	if spent := diag.Reqs(2*time.Minute, time.Minute); spent < 2000 {
		t.Fatalf("an exhausted budget produced timeout %d ms; the bridge reads 0 as forever", spent)
	}
}

func mustJournal(t *testing.T) *youtube.Journal { return mustJournalCap(t, 10) }

func mustJournalCap(t *testing.T, cap int) *youtube.Journal {
	t.Helper()
	j, err := youtube.OpenJournal(filepath.Join(t.TempDir(), "journal.json"), cap, time.UTC)
	if err != nil {
		t.Fatalf("journal: %v", err)
	}
	return j
}

func videoFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "output.mp4")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func boolPtr(b bool) *bool { return &b }

type fakeBrowser struct {
	tab          youtube.Tab
	calls        []string
	uploadParams []map[string]any
	scrollParams []map[string]any
	callsJSON    []string
	clickFails   string // the error text a click reply carries, "" for success
	silent       bool   // answer web_eval with success and no result at all
	domClicks    int    // page-channel presses, which must never follow an ambiguous bridge press
}

// newFakeBrowser builds the scripted bridge. Replies are the shape the real bridge answers with
// ({"success":…,"data":{…}}), and web_eval dispatches on which embedded program is in the payload — which
// is also a live check that every flow script is actually reachable by name.
func newFakeBrowser(tab youtube.Tab) *fakeBrowser { return &fakeBrowser{tab: tab} }

func (f *fakeBrowser) Call(ctx context.Context, action string, params map[string]any) (map[string]any, error) {
	f.calls = append(f.calls, action)
	raw, _ := json.Marshal(params)
	f.callsJSON = append(f.callsJSON, string(raw))
	switch action {
	case "navigate":
		return map[string]any{"success": true, "data": map[string]any{
			"id": f.tab.ID, "containerId": f.tab.ContainerID, "status": 200, "url": f.tab.URL, "title": "YouTube Studio",
		}}, nil
	case "upload":
		f.uploadParams = append(f.uploadParams, params)
		return map[string]any{"success": true, "data": map[string]any{"success": true, "size": 10, "filename": "output.mp4"}}, nil
	case "scroll":
		f.scrollParams = append(f.scrollParams, params)
		return map[string]any{"success": true, "data": nil}, nil
	case "get_text":
		return map[string]any{"success": true, "data": map[string]any{"el": map[string]any{"tag": "div", "visible": true}, "text": "x"}}, nil
	case "click":
		if f.clickFails != "" {
			return map[string]any{"success": false, "error": f.clickFails}, nil
		}
		return map[string]any{"success": true, "data": map[string]any{"el": map[string]any{"tag": "button"}}}, nil
	case "close_tab":
		return map[string]any{"success": true, "data": nil}, nil
	case "screenshot":
		return map[string]any{"success": true, "data": nil}, nil
	case "web_eval":
		if f.silent {
			return map[string]any{"success": true, "data": map[string]any{"logs": []any{}}}, nil
		}
		code, _ := params["code"].(string)
		var result any
		switch {
		case strings.Contains(code, "body_text.js"):
			result = map[string]any{"url": f.tab.URL, "text": "Upload complete Processing", "len": 24}
		case strings.Contains(code, "wizard_state.js"):
			result = map[string]any{"panel": "Visibility", "videoId": "BeCRc-x5B6E", "links": []string{"https://youtu.be/BeCRc-x5B6E"}, "saved": true, "checksComplete": true}
		case strings.Contains(code, "calendar.js"):
			result = map[string]any{"clicked": true, "committed": true, "after": "Nov 9, 2026"}
		case strings.Contains(code, "DOM-level click"):
			f.domClicks++
			return map[string]any{"success": true, "data": map[string]any{"result": map[string]any{"clicked": true, "text": "No, it's not made for kids"}}}, nil
		case strings.Contains(code, "DOM-level click"):
			f.domClicks++
			return map[string]any{"success": true, "data": map[string]any{"result": map[string]any{"clicked": true, "text": "No, it's not made for kids"}}}, nil
		case strings.Contains(code, "fill.js"):
			result = map[string]any{"ok": true, "method": "insertText", "text": "output (upload probe)"}
		default: // lookup.js and anything else
			result = map[string]any{"ok": true, "count": 1, "css": "#x", "text": "Next", "state": map[string]any{"exists": true, "visible": true, "disabled": false, "checked": true, "selected": true}}
		}
		return map[string]any{"success": true, "data": map[string]any{"result": result}}, nil
	}
	return map[string]any{"success": true, "data": nil}, nil
}

// The daily cap is a property of a channel, so a second channel must not read or spend the first one's ledger.
func TestJournalIsPerChannel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ZEN_YOUTUBE_JOURNAL", dir+"/journal.json")

	def, err := youtube.JournalPath("")
	if err != nil {
		t.Fatal(err)
	}
	other, err := youtube.JournalPath("second channel")
	if err != nil {
		t.Fatal(err)
	}
	if def != dir+"/journal.json" {
		t.Errorf("default ledger = %q, want the historic path so an existing journal is not orphaned", def)
	}
	if other == def {
		t.Fatal("a named channel shares the default ledger")
	}
	if filepath.Base(other) != "journal-second-channel.json" {
		t.Errorf("named ledger = %q, want a sanitized per-channel file", other)
	}

	// Two journals, two budgets: spending one leaves the other untouched.
	a, err := youtube.OpenJournal(def, 1, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	b, err := youtube.OpenJournal(other, 1, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Claim("upload"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Claim("upload"); !errors.Is(err, youtube.ErrDailyLimit) {
		t.Errorf("second claim on the spent channel = %v, want the daily limit", err)
	}
	if _, err := b.Claim("upload"); err != nil {
		t.Errorf("the other channel was charged for it: %v", err)
	}
}

// A contextual-identity name is free text and ends up in a file name, so nothing may traverse out of the state
// directory.
func TestSanitizeContainerCannotEscapeTheStateDir(t *testing.T) {
	for in, want := range map[string]string{
		"pachoigame":       "pachoigame",
		"../../etc/passwd": "etc-passwd",
		"work mail":        "work-mail",
		"":                 "",
		"...":              "",
	} {
		if got := youtube.SanitizeContainer(in); got != want {
			t.Errorf("youtube.SanitizeContainer(%q) = %q, want %q", in, got, want)
		}
	}
}
