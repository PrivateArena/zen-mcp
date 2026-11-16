package webserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// literalIntent is the exact request a publish looks like, with every optional field set so the
// round-trip test can see all eleven keys.
func literalIntent() Intent {
	trash := true
	return Intent{
		RequestID:    "r-01hg7t9p4r9w3d5g7i9k1l3o5p",
		Action:       IntentPublish,
		SiteID:       "note-ai",
		ConfigPath:   "/srv/www/note-ai.yaml",
		ConfigFile:   "/srv/www/.zen/requests/staged/r-01hg7t9p4r9w3d5g7i9k1l3o5p.yaml",
		ExpectSHA256: "9f2c1d0e3b4a5768abcd00112233445566778899aabbccddeeff0011223344ff",
		SnapshotTo:   "before-renovate",
		SnapshotID:   "note-ai.1731",
		Trash:        &trash,
		RequestedAt:  time.Date(2026, 11, 16, 7, 20, 0, 0, time.UTC),
		DeadlineAt:   time.Date(2026, 11, 16, 7, 20, 30, 0, time.UTC),
	}
}

// TestIntentWireKeys is the drift test the whole file exists for. The engine unmarshals this
// JSON strictly, so the assertion is on the KEY SET as a whole — an extra key (a renamed one,
// an omitted `expect_sha256`) fails here instead of surfacing as a rejected publish days later.
func TestIntentWireKeys(t *testing.T) {
	raw, err := json.Marshal(literalIntent())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal own output: %v", err)
	}
	want := []string{
		"request_id", "action", "site_id", "config_path", "config_file", "expect_sha256",
		"snapshot_to", "snapshot_id", "trash", "requested_at", "deadline_at",
	}
	if len(got) != len(want) {
		t.Errorf("key count: got %d (%s), want %d", len(got), strings.Join(keysOf(got), ","), len(want))
	}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("missing wire key %q; got %s", k, strings.Join(keysOf(got), ","))
		}
	}
	for k := range got {
		found := false
		for _, w := range want {
			if k == w {
				found = true
			}
		}
		if !found {
			t.Errorf("unexpected wire key %q", k)
		}
	}

	// The empty-CAS case: `expect_sha256` must still be present, spelled exactly, because "" is
	// the create-only signal rather than an absent field.
	min := Intent{RequestID: "r-x", Action: IntentStop, SiteID: "a", RequestedAt: time.Now().UTC(), DeadlineAt: time.Now().Add(time.Minute)}
	rawMin, err := json.Marshal(min)
	if err != nil {
		t.Fatalf("marshal minimal: %v", err)
	}
	var gotMin map[string]json.RawMessage
	if err := json.Unmarshal(rawMin, &gotMin); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := gotMin["expect_sha256"]; !ok {
		t.Errorf("expect_sha256 was omitted; keys are %s", strings.Join(keysOf(gotMin), ","))
	} else if string(v) != `""` {
		t.Errorf("expect_sha256 = %s, want an empty string", v)
	}
	for _, omitted := range []string{"config_file", "snapshot_to", "snapshot_id", "trash"} {
		if _, ok := gotMin[omitted]; ok {
			t.Errorf("%q should be omitempty but was emitted: %s", omitted, rawMin)
		}
	}
	if !strings.Contains(string(rawMin), `"action":"stop"`) {
		t.Errorf("action not emitted: %s", rawMin)
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestWriteIntentRoundTrip: what lands on disk must decode back to the same request, and in the
// engine's 2-space indented form.
func TestWriteIntentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := literalIntent()
	id, err := WriteIntent(dir, in)
	if err != nil {
		t.Fatalf("WriteIntent: %v", err)
	}
	if id != in.RequestID {
		t.Errorf("returned %q, want the request id %q", id, in.RequestID)
	}
	p := IntentPath(dir, id)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("intent not filed at %s: %v", p, err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(raw), "\n  \"request_id\"") {
		t.Errorf("intent is not 2-space indented JSON:\n%s", raw)
	}
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("intent mode %v", fi.Mode().Perm())
	}
	back, err := ReadIntent(p)
	if err != nil {
		t.Fatalf("ReadIntent: %v", err)
	}
	if back.RequestID != in.RequestID || back.Action != in.Action || back.SiteID != in.SiteID ||
		back.ConfigPath != in.ConfigPath || back.ConfigFile != in.ConfigFile ||
		back.ExpectSHA256 != in.ExpectSHA256 || back.SnapshotTo != in.SnapshotTo ||
		back.SnapshotID != in.SnapshotID || back.Trash == nil || *back.Trash != *in.Trash {
		t.Errorf("round trip lost a field:\n got %+v\nwant %+v", back, in)
	}
	if !back.RequestedAt.Equal(in.RequestedAt) || !back.DeadlineAt.Equal(in.DeadlineAt) {
		t.Errorf("times did not round-trip: %s / %s", back.RequestedAt, back.DeadlineAt)
	}
	// The .tmp directory exists (it is the atomicity mechanism) and holds nothing after.
	ents, err := os.ReadDir(filepath.Join(dir, ".tmp"))
	if err != nil {
		t.Fatalf(".tmp missing: %v", err)
	}
	if len(ents) != 0 {
		t.Errorf("%d leftovers in .tmp", len(ents))
	}
}

// TestWriteIntentIsExclusive is plan 2 A-3 and A-1 read as a file test: a second intent under a
// taken id refuses, and the incumbent file is untouched byte for byte — an id collision must
// never destroy an audit record.
func TestWriteIntentIsExclusive(t *testing.T) {
	dir := t.TempDir()
	first := literalIntent()
	if _, err := WriteIntent(dir, first); err != nil {
		t.Fatalf("first WriteIntent: %v", err)
	}
	p := IntentPath(dir, first.RequestID)
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Same id, different content: the overwrite is refused and reported as os.ErrExist.
	second := literalIntent()
	second.ConfigPath = "/srv/www/other.yaml"
	second.RequestedAt = time.Now().UTC()
	got, err := WriteIntent(dir, second)
	if err == nil {
		t.Fatalf("second WriteIntent returned id %q with no error", got)
	}
	collision := err
	if !errors.Is(collision, os.ErrExist) {
		t.Errorf("error is not os.ErrExist: %v", collision)
	}
	if got != "" {
		t.Errorf("a refused write returned an id %q", got)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("the incumbent intent changed:\n%s\n---\n%s", before, after)
	}
	if !strings.Contains(collision.Error(), first.RequestID) {
		t.Errorf("error does not name the colliding id: %v", collision)
	}
	// The rejected attempt leaves no final-name file behind and nothing in .tmp.
	if ents, err := os.ReadDir(filepath.Join(dir, ".tmp")); err == nil && len(ents) != 0 {
		t.Errorf("%d files left in .tmp by the refused write", len(ents))
	}

	// A different id files alongside it.
	third := literalIntent()
	third.RequestID = NewRequestID()
	if _, err := WriteIntent(dir, third); err != nil {
		t.Fatalf("a distinct id must file: %v", err)
	}
	if _, err := os.Stat(IntentPath(dir, third.RequestID)); err != nil {
		t.Errorf("third intent not filed: %v", err)
	}
}

func TestWriteIntentValidatesBeforeTouchingDisk(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   Intent
	}{
		{"no action", Intent{RequestID: "r-1", SiteID: "a"}},
		{"unknown action", Intent{RequestID: "r-1", SiteID: "a", Action: "reboot"}},
		{"no site", Intent{RequestID: "r-1", Action: IntentStop}},
		{"no request id", Intent{SiteID: "a", Action: IntentStop}},
		{"publish without staged config", Intent{RequestID: "r-1", SiteID: "a", Action: IntentPublish}},
	} {
		dir := t.TempDir()
		id, err := WriteIntent(dir, tc.in)
		if err == nil {
			t.Errorf("%s: accepted, filed %q", tc.name, id)
		}
		if id != "" {
			t.Errorf("%s: returned an id %q", tc.name, id)
		}
		if ents, err := os.ReadDir(dir); err == nil && len(ents) != 0 {
			t.Errorf("%s: created files in a directory that must stay empty: %d", tc.name, len(ents))
		}
	}
	// RequestedAt defaults to now in UTC.
	dir := t.TempDir()
	in := Intent{RequestID: NewRequestID(), SiteID: "a", Action: IntentStop}
	if _, err := WriteIntent(dir, in); err != nil {
		t.Fatalf("WriteIntent: %v", err)
	}
	back, err := ReadIntent(IntentPath(dir, in.RequestID))
	if err != nil {
		t.Fatalf("ReadIntent: %v", err)
	}
	if back.RequestedAt.IsZero() || back.RequestedAt.Location() != time.UTC {
		t.Errorf("requested_at defaulted to %v", back.RequestedAt)
	}
}

// TestNewRequestID pins the format the engine's ids interoperate with: prefix, length, alphabet,
// and the time ordering that lets the intake directory be read as a queue (plan 2 A-1).
func TestNewRequestID(t *testing.T) {
	const alphabet = "0123456789abcdefghjkmnpqrshtvwxy"
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		id := NewRequestID()
		if !strings.HasPrefix(id, "r-") {
			t.Fatalf("id %q has no r- prefix", id)
		}
		body := id[2:]
		if len(body) != 26 {
			t.Fatalf("id %q has %d characters after the prefix, want 26", id, len(body))
		}
		for _, r := range body {
			if !strings.ContainsRune(alphabet, r) {
				t.Fatalf("id %q contains %q, outside Crockford's alphabet", id, r)
			}
		}
		for _, bad := range "ilou" {
			if strings.ContainsRune(body, bad) {
				t.Errorf("id %q contains the excluded rune %q", id, bad)
			}
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}

	// Time ordering: the first 6 bytes are big-endian milliseconds, so ids minted in different
	// milliseconds compare lexically in mint order.
	a := NewRequestID()
	time.Sleep(5 * time.Millisecond)
	b := NewRequestID()
	if !(a < b) {
		t.Errorf("ids do not sort by time: %q then %q", a, b)
	}
	if len(a) != len(b) {
		t.Errorf("variable-length ids: %q %q", a, b)
	}
}

func TestStageConfigSanitizesNames(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ id, wantName string }{
		{"dummy", "dummy.yaml"},
		{"note-ai", "note-ai.yaml"},
		{"r-x/../evil", "r-x-..-evil.yaml"},
		{"../../etc/passwd", "..-..-etc-passwd.yaml"},
		{"weird name/scope", "weird-name-scope.yaml"},
		{"", "intent.yaml"},
		{"日本語", "---.yaml"},
	} {
		got, err := StageConfig(dir, tc.id, []byte("settings:\n  site_name: x\n"))
		if err != nil {
			t.Fatalf("StageConfig(%q): %v", tc.id, err)
		}
		if filepath.Base(got) != tc.wantName {
			t.Errorf("StageConfig(%q) named %q, want %q", tc.id, filepath.Base(got), tc.wantName)
		}
		if filepath.Dir(got) != filepath.Join(dir, "staged") {
			t.Errorf("StageConfig(%q) = %q, outside the staging directory", tc.id, got)
		}
		fi, err := os.Stat(got)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("staged config mode %v, want 0600", fi.Mode().Perm())
		}
		// No traversal: the result is exactly one component inside staged/ — a sanitized name may
		// CONTAIN dots, but it is never "."/".." and never carries a separator.
		rel, err := filepath.Rel(filepath.Join(dir, "staged"), got)
		if err != nil || rel != filepath.Base(got) || rel == "." || rel == ".." || filepath.Dir(rel) != "." {
			t.Errorf("StageConfig(%q) escaped: %q rel=%q err=%v", tc.id, got, rel, err)
		}
		raw, err := os.ReadFile(got)
		if err != nil || !strings.Contains(string(raw), "site_name: x") {
			t.Errorf("staged bytes missing: %q %v", raw, err)
		}
	}
	// The long-id cap keeps one name to a single filesystem component.
	long := strings.Repeat("a", 400)
	p, err := StageConfig(dir, long, []byte("x: 1\n"))
	if err != nil {
		t.Fatalf("long id: %v", err)
	}
	if n := len(filepath.Base(p)); n > maxSanitizedNameLen+len(".yaml") {
		t.Errorf("sanitized name is %d characters: %s", n, p)
	}
}

// TestWaitSeesVerdict: the ledger entry is the answer, and it arrives from the document rather
// than from a response body.
func TestWaitSeesVerdict(t *testing.T) {
	st := freshStatus(3099)
	st.RecentRequests = []RequestResult{{RequestID: "r-01hg7t9p4r9w3d5g7i9k1l3o5p", Result: "applied", At: time.Now().Format(time.RFC3339Nano)}}
	path := writeStatus(t, st)

	got, res, err := Wait(context.Background(), path, "r-01hg7t9p4r9w3d5g7i9k1l3o5p", "dummy", time.Now().Add(5*time.Second))
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.Result != "applied" || res.RequestID == "" {
		t.Errorf("verdict: %+v", res)
	}
	if got.PassSeq != st.PassSeq {
		t.Errorf("document not returned: pass %d, want %d", got.PassSeq, st.PassSeq)
	}

	// The newest entry wins, which is how a rolled-back publish reports rolled_back rather than
	// the "applied" the first pass wrote.
	st.RecentRequests = append(st.RecentRequests, RequestResult{RequestID: "r-01hg7t9p4r9w3d5g7i9k1l3o5p", Result: "rolled_back", Error: "restored dummy.1731 after: self-check failed"})
	path2 := writeStatus(t, st)
	_, res, err = Wait(context.Background(), path2, "r-01hg7t9p4r9w3d5g7i9k1l3o5p", "dummy", time.Now().Add(5*time.Second))
	if err != nil {
		t.Fatalf("Wait (rolled back): %v", err)
	}
	if res.Result != "rolled_back" {
		t.Errorf("got %+v, want the newest entry", res)
	}

	// A caller that filed no intent but knows the site: the site's own applied_request_id
	// answers (A-14).
	bySite := freshStatus(3099)
	bySite.Sites[0].AppliedReqID = "r-site-only"
	_, res, err = Wait(context.Background(), writeStatus(t, bySite), "r-site-only", "dummy", time.Now().Add(2*time.Second))
	if err != nil || res.RequestID != "r-site-only" {
		t.Errorf("site-applied fallback: %+v %v", res, err)
	}
}

// TestWaitNoVerdictIsDistinctFromRejected: on the deadline, Wait returns the LAST document it
// read plus an error that says "no verdict", which a caller must be able to tell apart from a
// rejection.
func TestWaitNoVerdictIsDistinctFromRejected(t *testing.T) {
	st := freshStatus(3099)
	st.BootID = "boot-live"
	st.PassSeq = 99
	st.RecentRequests = []RequestResult{{RequestID: "r-someone-else", Result: "applied"}}
	path := writeStatus(t, st)

	deadline := time.Now().Add(600 * time.Millisecond)
	got, res, err := Wait(context.Background(), path, "r-never-answered", "dummy", deadline)
	if err == nil {
		t.Fatalf("a missing verdict returned %+v", res)
	}
	if !IsNoVerdict(err) {
		t.Errorf("error does not wrap ErrNoVerdict: %v", err)
	}
	if !errors.Is(err, ErrNoVerdict) {
		t.Error("errors.Is(err, ErrNoVerdict) is false")
	}
	if res.RequestID != "" || res.Result != "" {
		t.Errorf("a timeout invented a verdict: %+v", res)
	}
	// The last document read comes back, so the caller can report what the fleet was doing.
	if got.BootID != "boot-live" || got.PassSeq != 99 {
		t.Errorf("last document lost: boot=%q pass=%d", got.BootID, got.PassSeq)
	}
	msg := err.Error()
	if !strings.Contains(msg, "r-never-answered") || !strings.Contains(msg, "boot-live") || !strings.Contains(msg, "99") {
		t.Errorf("timeout error is not actionable: %s", msg)
	}

	// An unreadable document is a third state, and it says so.
	_, _, err = Wait(context.Background(), filepath.Join(t.TempDir(), "absent.json"), "r-x", "dummy", time.Now().Add(300*time.Millisecond))
	if !IsNoVerdict(err) || !strings.Contains(err.Error(), "unreadable") {
		t.Errorf("unreadable document: %v", err)
	}

	// A cancelled context stops the poll at once rather than running to the deadline.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, _, err = Wait(ctx, path, "r-never-answered", "dummy", time.Now().Add(time.Hour))
	if !IsNoVerdict(err) || !errors.Is(err, context.Canceled) {
		t.Errorf("cancellation: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("Wait ignored the cancellation for %s", time.Since(start))
	}
}

func TestSha256File(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.yaml")
	if err := os.WriteFile(p, []byte("settings:\n  site_name: x\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	sum, err := sha256File(p)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}
	// Lowercase hex, 64 characters, no prefix — the form expect_sha256 and applied_sha256 use.
	if len(sum) != 64 || strings.ToLower(sum) != sum {
		t.Errorf("sum %q is not 64 lowercase hex characters", sum)
	}
	again, _ := sha256File(p)
	if again != sum {
		t.Error("not stable")
	}
	// The known digest of the empty input, which pins the algorithm rather than its formatting.
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, err := sha256File(empty); err != nil || got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("empty-file digest = %q (%v)", got, err)
	}
	if _, err := sha256File(filepath.Join(dir, "absent")); err == nil || !strings.Contains(err.Error(), "absent") {
		t.Errorf("missing file: %v", err)
	}
}

func TestIntentExpired(t *testing.T) {
	now := time.Now()
	if !(Intent{DeadlineAt: now.Add(-time.Second)}).Expired(now) {
		t.Error("a passed deadline is not expired")
	}
	if (Intent{DeadlineAt: now.Add(time.Minute)}).Expired(now) {
		t.Error("a future deadline is expired")
	}
	if (Intent{}).Expired(now) {
		t.Error("a zero deadline is expired")
	}
}
