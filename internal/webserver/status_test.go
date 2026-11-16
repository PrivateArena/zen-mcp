package webserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureStatusPath copies the recorded status document into a temp dir, because Read takes a
// path and every test wants an independent one.
func fixtureStatusPath(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "fleet-status.fixture.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".zen"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := filepath.Join(dir, ".zen", "fleet-status.json")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

func TestReadFixture(t *testing.T) {
	st, err := Read(fixtureStatusPath(t))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if st.Schema != 1 || st.BootID != "boot-7f3c1a" || st.PID == 0 {
		t.Errorf("header fields wrong: schema=%d boot=%q pid=%d", st.Schema, st.BootID, st.PID)
	}
	if st.PassSeq != 812 || st.IntervalMS != 3000 {
		t.Errorf("pass_seq=%d interval_ms=%d, want 812 / 3000", st.PassSeq, st.IntervalMS)
	}
	if len(st.Desired) != 4 || len(st.Sites) != 5 || len(st.RecentRequests) != 5 {
		t.Fatalf("counts: desired=%d sites=%d recent=%d", len(st.Desired), len(st.Sites), len(st.RecentRequests))
	}
	if len(st.Staged) != 1 {
		t.Errorf("pending not decoded: %#v", st.Staged)
	}

	// The rejected entry must be visible, not absent: that is the whole reason desired[]
	// carries refused files (plan 2 A-5).
	d, ok := st.DesiredFor("/media/jang/home/Deve/zen-webserver/www/memory.yaml")
	if !ok || d.Verdict != "rejected" || len(d.Findings) != 1 {
		t.Fatalf("rejected desired entry missing: %#v ok=%v", d, ok)
	}
	if d.Findings[0].Code != "theme.missing" || d.Findings[0].Severity != "error" {
		t.Errorf("finding decoded wrong: %#v", d.Findings[0])
	}

	s, err := ResolveSite(st, "catalog")
	if err != nil {
		t.Fatalf("ResolveSite: %v", err)
	}
	if s.Port != 3006 || s.State != "running" || s.AppliedSHA256 == "" || s.AppliedReqID == "" {
		t.Errorf("site decoded wrong: %#v", s)
	}
	if s.LastOK == "" || s.NextAttempt != "" {
		t.Errorf("optional time fields wrong: last_ok=%q next_attempt=%q", s.LastOK, s.NextAttempt)
	}
	gym, err := ResolveSite(st, "gym")
	if err != nil {
		t.Fatalf("ResolveSite(gym): %v", err)
	}
	if !gym.Expose || gym.State != "backoff" || gym.LastErr == "" {
		t.Errorf("backoff site wrong: %#v", gym)
	}
}

// TestReadIsMalformedBodyIsAnError pins the rule that a broken document never arrives as a
// zero Status: an agent that read {} out of a truncated file would report a healthy empty
// fleet.
func TestReadMalformedIsAnError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "fleet-status.json")
	for _, body := range []string{"", "{", `{"schema":`, `[1,2]`, `{"sites": "nope"}`} {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		st, err := Read(p)
		if err == nil {
			t.Errorf("body %q parsed as %#v, want an error", body, st)
		}
		if st.Sites != nil || st.Desired != nil {
			t.Errorf("body %q returned a non-zero Status beside an error", body)
		}
	}
	if _, err := Read(filepath.Join(dir, "absent.json")); err == nil || !strings.Contains(err.Error(), "absent.json") {
		t.Errorf("missing file: %v", err)
	}
}

func TestStatusPathFor(t *testing.T) {
	if got, want := StatusPathFor("/srv/www"), filepath.Join("/srv/www", ".zen", "fleet-status.json"); got != want {
		t.Errorf("StatusPathFor = %q, want %q", got, want)
	}
	// Same string the engine computes (zen-webserver main.go: fleetStatusRel).
	if got := StatusPathFor("www"); !strings.HasSuffix(got, filepath.Join(".zen", "fleet-status.json")) {
		t.Errorf("StatusPathFor(www) = %q", got)
	}
}

// TestResolveSitePrefersID documents the ordering that the fleet's current shape forces:
// every config in www/ leaves settings.site_id empty, so a site_id-first match would resolve
// nothing at all. note-ai is in the fixture with BOTH fields set, which is what proves the
// site_id fallback still works once the fleet fills it in.
func TestResolveSitePrefersID(t *testing.T) {
	st, err := Read(fixtureStatusPath(t))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got, err := ResolveSite(st, "catalog"); err != nil || got.Port != 3006 {
		t.Fatalf("by id: %#v %v", got, err)
	}
	if got, err := ResolveSite(st, "note-ai"); err != nil || got.Port != 3034 {
		t.Fatalf("by id where site_id also matches: %#v %v", got, err)
	}
	// An id that is only a site_id still resolves (second pass).
	st.Sites = append(st.Sites, SiteState{ID: "shop", SiteID: "my-shop", Port: 3050})
	if got, err := ResolveSite(st, "my-shop"); err != nil || got.ID != "shop" {
		t.Fatalf("by site_id: %#v %v", got, err)
	}

	_, err = ResolveSite(st, "nope")
	if err == nil {
		t.Fatal("unknown site resolved without an error")
	}
	msg := err.Error()
	// The error must name the available ids (plan 2 step 14's acceptance line).
	for _, want := range []string{"nope", "catalog", "memory", "gym", "wiki", "note-ai"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not name %q: %s", want, msg)
		}
	}
	if _, err := ResolveSite(st, "   "); err == nil {
		t.Error("blank site accepted")
	}
}

// TestLatestRequestNewestEntryWins is plan 2 A-4's readback: an auto-rollback appends a SECOND
// entry under the same request id, and reporting the first one would tell the agent a broken
// publish is still live.
func TestLatestRequestNewestEntryWins(t *testing.T) {
	st, err := Read(fixtureStatusPath(t))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	r, ok := LatestRequest(st, "r-01hg7t9p4r9w3d5g7i9k1l3o5p")
	if !ok {
		t.Fatal("double-entered request not found")
	}
	if r.Result != "failed" || !strings.Contains(r.Error, "rolled_back") {
		t.Errorf("got %#v, want the newest (rolled-back) entry", r)
	}
	if _, ok := LatestRequest(st, "r-does-not-exist"); ok {
		t.Error("unknown request id reported as found")
	}
}

func TestStaleFor(t *testing.T) {
	st, err := Read(fixtureStatusPath(t))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	justAfter := time.Date(2026, 11, 16, 7, 20, 5, 0, time.UTC) // 14:20:05+07:00
	age, err := st.StaleFor(justAfter)
	if err != nil || age > time.Minute {
		t.Errorf("fresh document: age=%s err=%v", age, err)
	}

	// 3x the reconcile interval is the staleness bound (plan 1 F-18): past it, the document is
	// UNKNOWN, not merely old.
	later := justAfter.Add(10 * time.Second)
	if _, err := st.StaleFor(later); err == nil || !strings.Contains(err.Error(), "UNKNOWN") {
		t.Errorf("stale document accepted: %v", err)
	}

	st.GeneratedAt = ""
	if _, err := st.StaleFor(justAfter); err == nil {
		t.Error("document without generated_at accepted")
	}
}
