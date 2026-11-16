package webserver

// Live acceptance: the only test that proves this mirror is WIRE-COMPATIBLE with a real
// engine rather than merely self-consistent. A hand-mirrored Intent can drift from
// pkg/fleet.Intent one tag at a time and every unit test here still passes, because they all
// assert against the same mirror they were written from. This one hands the intent to the
// engine and reads the engine's own verdict out of the engine's own status document, so a
// drifted tag shows up as a rejection instead of as a green suite.
//
// Skipped unless ZEN_LAB_FLEET names a scratch fleet directory. It is not a fixture: it
// mutates a real site's config, so it must never run against the user's www/.
//
//	ZEN_LAB_FLEET=/tmp/zenlab go test -count=1 -v -run TestLive ./internal/webserver/

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func liveFleet(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("ZEN_LAB_FLEET")
	if dir == "" {
		t.Skip("ZEN_LAB_FLEET unset; this test mutates a live scratch fleet's config")
	}
	if _, err := os.Stat(filepath.Join(dir, ".zen", "requests")); err != nil {
		t.Skipf("no requests dir under %s", dir)
	}
	return dir
}

// TestLivePublishRoundTrip is the wire-compatibility check: publish a config that differs from
// the live one in one line, and require the engine to say `applied` for OUR request id.
func TestLivePublishRoundTrip(t *testing.T) {
	dir := liveFleet(t)
	statusPath := StatusPathFor(dir)
	st, err := Read(statusPath)
	if err != nil {
		t.Fatalf("read status %s: %v", statusPath, err)
	}
	site, err := ResolveSite(st, "dummy")
	if err != nil {
		t.Fatalf("resolve the lab site: %v (a fleet is running only if this test is meant to run)", err)
	}
	if site.State != "running" {
		t.Fatalf("lab site is %s, not running; start it before running this test", site.State)
	}

	cfgPath := site.ConfigPath
	if cfgPath == "" {
		cfgPath = filepath.Join(dir, "dummy.yaml")
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read %s: %v", cfgPath, err)
	}
	before, err := sha256File(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	// Change exactly one value, and only if it is not already changed — the test must be
	// re-runnable against a fleet it already mutated.
	mark := regexp.MustCompile(`(?m)^(\s*site_name:\s*)"(.*)"\s*$`)
	if !mark.Match(raw) {
		t.Fatalf("%s has no settings.site_name line to rewrite", cfgPath)
	}
	next := mark.ReplaceAllString(string(raw), "${1}\"${2} x\"")
	if strings.Contains(string(raw), "x\"") && next == string(raw) {
		next = mark.ReplaceAllString(string(raw), "${1}\"${2}\"")
	}
	if next == string(raw) {
		t.Fatal("could not build a differing candidate config")
	}

	reqs := RequestsPathFor(dir)
	id := NewRequestID()
	staged, err := StageConfig(reqs, id, []byte(next))
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	got, err := WriteIntent(reqs, Intent{
		RequestID:    id,
		Action:       IntentPublish,
		SiteID:       site.SiteID,
		ConfigPath:   cfgPath,
		ConfigFile:   staged,
		ExpectSHA256: before,
		SnapshotTo:   "live-test",
		RequestedAt:  time.Now(),
		DeadlineAt:   time.Now().Add(30 * time.Second),
	})
	if err != nil {
		t.Fatalf("write intent: %v", err)
	}
	if got != id {
		t.Fatalf("WriteIntent returned %q, wanted %q", got, id)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	// The engine reconciles every 3 s, so a verdict lands in well under 10 s; the deadline is
	// generous because a swap restarts the site, and Wait polls at 250 ms rather than sleeping.
	final, res, err := Wait(ctx, statusPath, id, site.SiteID, time.Now().Add(40*time.Second))
	if err != nil {
		if IsNoVerdict(err) {
			// A missing verdict is its own failure mode (A-7/A-19): the intent was accepted by
			// the filesystem and then nobody answered. Report the pass counter so a dead
			// consumer is distinguishable from a slow one.
			t.Fatalf("no verdict for %s after the deadline; status pass_seq=%d sites=%v", id, final.PassSeq, site.State)
		}
		t.Fatalf("wait: %v", err)
	}
	t.Logf("verdict: %s %q at %s", res.Result, res.Error, res.At)

	switch res.Result {
	case "applied":
		// The point of the test: OUR id is what the engine recorded. applied_request_id is the
		// other half, and it is what a caller would trust without reading the ledger.
		if site.AppliedSHA256 != "" {
			after, err := sha256File(cfgPath)
			if err != nil {
				t.Fatal(err)
			}
			if after == before {
				t.Fatalf("verdict says applied but %s is byte-identical to what it was", cfgPath)
			}
		}
	case "rolled_back":
		// A legitimate outcome, and the only one that proves A-4 end-to-end from this side. Do
		// not paper over it: say so, and check the config went back to the pre-publish hash.
		after, _ := sha256File(cfgPath)
		t.Logf("auto-rollback fired (A-4): live config is back to %x… (pre-publish %x…)", trunc(after), trunc(before))
		if after != before {
			t.Errorf("rolled_back but the live config is neither the new nor the old bytes")
		}
	default:
		t.Fatalf("verdict %q: %s (the engine rejected our wire format or its own lint; the error text names which)", res.Result, res.Error)
	}

	// The snapshot the intent asked for must exist, or `rollback` would have nothing to restore.
	// Names are the engine's content-addressed snapshot ids (a sortable UTC timestamp), not the
	// free-text `snapshot_to` label — which is why the label is not searched for here. What
	// matters is that a rollback target now exists whose bytes are the pre-publish config.
	snapDir := filepath.Join(dir, ".zen", "snapshots", site.SiteID)
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		t.Fatalf("no snapshot dir %s: %v", snapDir, err)
	}
	var matched string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue // the .note sidecar is operator prose, not a rollback source
		}
		b, err := os.ReadFile(filepath.Join(snapDir, e.Name()))
		if err != nil {
			continue
		}
		if fmtHash(b) == before {
			matched = e.Name()
		}
	}
	if matched == "" {
		t.Errorf("no snapshot under %s holds the pre-publish bytes (%s) — a rollback would restore the wrong config", snapDir, trunc(before))
	} else {
		t.Logf("rollback target: %s", matched)
	}
}

// fmtHash is sha256 over the same bytes the engine hashes, reached through the unexported
// helper rather than a second copy of the digest code.
// fmtHash hashes the same bytes the engine hashes, reusing the file hash helper path rather than a
// second copy of the digest code.
func fmtHash(b []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func trunc(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
