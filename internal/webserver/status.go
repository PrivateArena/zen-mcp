// Package webserver is zen-mcp's half of the zen-webserver integration.
//
// It is three narrow things and nothing else: a reader of the fleet status document
// (status.go), a loopback-only client for one site's admin API (client.go), and the writer
// of fleet *intent files* plus the wait for their verdict (intents.go). The split is the
// plan's (plan 2 §5.1, §5.2): zen-mcp owns ergonomics, the engine owns semantics, and the
// two modules may never import each other — `zen-mcp` is a bare module path with everything
// under internal/, so the only integration surfaces are HTTP and files. Nothing in this
// package knows about the engine's Go types; every struct here is a hand-mirrored JSON shape
// whose authority is the engine's own pkg/fleetstatus / pkg/fleet, and each carries the field
// comments that say so. When a shape changes, the drift tests in internal/tools
// (webserver_schema_test.go) fail this repo's build, which is where plan 2 A-10 wants the
// failure to land.
package webserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// statusFileName is the document the reconciler publishes every pass (plan 1 §4.3). It is
// the ONLY port source in this repo (plan 2 A-18): a site's port belongs to its config YAML,
// not to zen-mcp's source, and a literal port here would silently break the moment an
// operator moves a site.
const statusFileName = "fleet-status.json"

// StatusPathFor is <fleetDir>/.zen/fleet-status.json — the same string the engine computes in
// main.go:fleetStatusPath. One fleet directory, one readback.
func StatusPathFor(fleetDir string) string {
	return filepath.Join(fleetDir, ".zen", statusFileName)
}

// Status mirrors pkg/fleetstatus.Status.
//
// Staleness is the CONSUMER's job (plan 1 F-18): after a crash the file survives untouched,
// so a reader must treat generated_at older than 3x reconcile_interval_ms as UNKNOWN. Age is
// reported by Status.StaleFor rather than being silently applied, because "the reconciler is
// behind" and "the reconciler is dead" call for different actions.
type Status struct {
	Schema      int    `json:"schema"`
	BootID      string `json:"boot_id"` // random per process start
	PID         int    `json:"pid"`
	PassSeq     int64  `json:"pass_seq"` // monotonic within a boot
	GeneratedAt string `json:"generated_at"`
	IntervalMS  int    `json:"reconcile_interval_ms"`

	Desired        []Desired       `json:"desired"`
	Sites          []SiteState     `json:"sites"`
	RecentRequests []RequestResult `json:"recent_requests"` // ring, last 64
	// Staged is the engine's `pending` key: intent files it has seen but not answered.
	Staged []string `json:"pending,omitempty"`
}

// Desired mirrors pkg/fleetstatus.Desired — one entry per YAML the scan saw, INCLUDING
// rejected ones, which is what makes "absent" distinguishable from "refused" (plan 2 A-5).
type Desired struct {
	ConfigPath string    `json:"config_path"`
	SHA256     string    `json:"sha256"`
	Verdict    string    `json:"verdict"` // ok | rejected | ignored
	Findings   []Finding `json:"findings,omitempty"`
	SeenAt     time.Time `json:"seen_at"`
}

// Finding mirrors pkg/fleetstatus.Finding.
type Finding struct {
	Severity string `json:"severity"` // error | warn | info
	Code     string `json:"code"`     // stable id, e.g. "port.conflict"
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// SiteState mirrors pkg/fleetstatus.SiteState: what is actually live for one site.
type SiteState struct {
	// ID is the config FILENAME STEM; SiteID is settings.site_id, which is empty for every
	// config in the fleet today. That asymmetry is why Resolve matches ID first (see below).
	ID            string `json:"id"`
	SiteID        string `json:"site_id"`
	ConfigPath    string `json:"config_path"`
	Port          int    `json:"port"`
	Expose        bool   `json:"expose"`
	State         string `json:"state"` // starting|running|backoff|stopped|orphan
	AppliedSHA256 string `json:"applied_sha256"`
	AppliedReqID  string `json:"applied_request_id,omitempty"`

	Generation  int64  `json:"generation"`
	LastErr     string `json:"last_error,omitempty"`
	NextAttempt string `json:"next_attempt,omitempty"`
	LastOK      string `json:"last_ok,omitempty"`
}

// RequestResult mirrors pkg/fleetstatus.RequestResult — one entry of the apply ledger.
type RequestResult struct {
	RequestID string `json:"request_id"`
	Result    string `json:"result"` // applied | failed | rejected | expired
	At        string `json:"at"`
	Error     string `json:"error,omitempty"`
}

// Read parses the status document. A malformed body is an error, never a zero Status: the
// engine's own reader says it best, and the reason matters more here than there — this
// package's callers decide whether a fleet is healthy, and "empty document" must not look
// like "unreadable document" to them (pkg/fleetstatus.Read).
func Read(path string) (Status, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Status{}, fmt.Errorf("webserver: read fleet status %s: %w", path, err)
	}
	var st Status
	if err := json.Unmarshal(b, &st); err != nil {
		return Status{}, fmt.Errorf("webserver: parse fleet status %s: %w", path, err)
	}
	return st, nil
}

// StaleFor reports how long ago the document was written, or an error when it is unreadable
// or older than the 3x reconcile-interval staleness bound plan 1 F-18 puts on it. A zero
// interval means "the document does not say", so only generated_at is checked.
func (st Status) StaleFor(now time.Time) (time.Duration, error) {
	if st.GeneratedAt == "" {
		return 0, fmt.Errorf("webserver: fleet status has no generated_at")
	}
	at, err := time.Parse(time.RFC3339Nano, st.GeneratedAt)
	if err != nil {
		return 0, fmt.Errorf("webserver: fleet status generated_at %q: %w", st.GeneratedAt, err)
	}
	age := now.Sub(at)
	if stale := 3 * time.Duration(st.IntervalMS) * time.Millisecond; st.IntervalMS > 0 && age > stale {
		return age, fmt.Errorf("webserver: fleet status is %s old, over the %s staleness bound (boot %s pass %d): treat every answer as UNKNOWN",
			age.Truncate(time.Second), stale, st.BootID, st.PassSeq)
	}
	return age, nil
}

// ResolveSite turns what a caller typed into one live site record.
//
// It matches `id` (the config filename stem) BEFORE `site_id`, because settings.site_id is
// empty for every config the fleet runs today while `id` is always the YAML stem — the
// reverse order would make the tool fail on the fleet as it exists. A future fleet that fills
// in site_id keeps working, and plan 2 §5.2's gloss ("site id, or the config filename stem")
// is satisfied by the second pass.
func ResolveSite(st Status, idOrSiteID string) (SiteState, error) {
	want := strings.TrimSpace(idOrSiteID)
	if want == "" {
		return SiteState{}, fmt.Errorf("webserver: a site is required (this action has no default site)")
	}
	for _, s := range st.Sites {
		if s.ID == want {
			return s, nil
		}
	}
	for _, s := range st.Sites {
		if s.SiteID != "" && s.SiteID == want {
			return s, nil
		}
	}
	return SiteState{}, fmt.Errorf("webserver: site %q is not in the fleet status document; available: %s", want, siteList(st))
}

// siteList names what the document actually knows, so the error is actionable rather than a
// dead end (plan 2 step 14's acceptance line: "an error naming the available ids").
func siteList(st Status) string {
	if len(st.Sites) == 0 {
		return "none (the fleet status document lists no live sites)"
	}
	names := make([]string, 0, len(st.Sites))
	for _, s := range st.Sites {
		switch {
		case s.SiteID != "" && s.SiteID != s.ID:
			names = append(names, s.ID+"|"+s.SiteID)
		default:
			names = append(names, s.ID)
		}
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// LatestRequest looks a request up in the ledger. The NEWEST entry with the id wins, because
// an auto-rollback (plan 2 A-4) appends a SECOND entry under the same request id — a
// first-match lookup would report "applied" for a publish that was rolled back.
func LatestRequest(st Status, requestID string) (RequestResult, bool) {
	for i := len(st.RecentRequests) - 1; i >= 0; i-- {
		if r := st.RecentRequests[i]; r.RequestID == requestID {
			return r, true
		}
	}
	return RequestResult{}, false
}

// Site returns the live state of one site id without the loopback checks of Client.Resolve.
func (st Status) Site(idOrSiteID string) (SiteState, error) { return ResolveSite(st, idOrSiteID) }

// DesiredFor finds the scan entry for one config path, which is where a rejected publish's
// findings live.
func (st Status) DesiredFor(configPath string) (Desired, bool) {
	for _, d := range st.Desired {
		if d.ConfigPath == configPath {
			return d, true
		}
	}
	return Desired{}, false
}
