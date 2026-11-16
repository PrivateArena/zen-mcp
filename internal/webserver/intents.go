package webserver

// The mutating half of the integration, and the ONLY way this repo may change a fleet:
// write an intent file, then read the verdict back out of the status document (plan 2 §5.2).
//
// Files, not a socket, and that is the engine's requirement rather than an inconvenience: no
// TCP listener anywhere in the fleet accepts a mutation, because zen-mcp itself authenticates
// nothing. `publish/stop/rm/rollback` are therefore file writes that the reconciler consumes at
// the top of its own pass — zen-mcp never applies a change, so there is exactly one mutator
// and no interleaving between a publish and a reconcile.
//
// Two properties make the file protocol safe rather than merely simple:
//
//   - O_EXCL creation of the intent file. Two agents cannot both believe they own one request
//     id, and a retried publish is a distinguishable event instead of a silent overwrite. This
//     is why NewRequestID is a byte-for-byte copy of the engine's: plan 2 A-1's failure mode is
//     a FORMAT collision destroying an audit record, which no amount of "close enough" prevents.
//   - ExpectSHA256, a compare-and-swap against the live config, which closes the ~3 s window in
//     which two agents edit one YAML (A-3). Get it from sha256File on the config the status
//     document names, or from the site's applied_sha256 — and send "" only for a create.
//
// Nothing below talks to a port. status.go's reader answers; this file asks.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Intent actions — the wire strings pkg/fleet's Intent.Action accepts. An unknown action is a
// consumed-and-rejected intent, not a compile error, so these are consts rather than free text.
const (
	IntentPublish  = "publish"  // write or replace a site config, admitted by lint
	IntentStop     = "stop"     // park a config out of the scan (site stops, nothing deleted)
	IntentStart    = "start"    // un-park a stopped config
	IntentRemove   = "rm"       // archive the config to .zen/trash and stop serving it
	IntentRollback = "rollback" // restore a snapshot over the config
)

// waitPollInterval is the read cadence of the status document: plan 2 A-19's "never a fixed
// multi-second sleep". 250 ms is well under the fleet's 3 s pass, so a verdict is picked up on
// the pass that writes it, and it is the engine's own cadence — polling harder buys nothing the
// reconciler has not already decided.
const waitPollInterval = 250 * time.Millisecond

// ErrNoVerdict means the reconciler never answered: no ledger entry appeared before the
// deadline. That is a DIFFERENT outcome from "rejected" — the engine may have died between the
// file write and the pass — and collapsing the two is how an agent is told its publish was
// refused when in fact nobody is home.
var ErrNoVerdict = errors.New("no verdict recorded")

// IsNoVerdict reports the timeout branch of Wait.
func IsNoVerdict(err error) bool { return errors.Is(err, ErrNoVerdict) }

// Intent mirrors pkg/fleet.Intent, key for key.
//
// The tags are the wire contract: the engine unmarshals this JSON strictly, so an extra key, a
// missing one or a renamed one is a wire break that surfaces as a rejected intent on the other
// side. ExpectSHA256 carries NO omitempty on purpose — the engine always emits it, and "" is a
// meaningful value (create-only, enforced as cas_conflict against an existing file). Drift here
// is invisible until a publish is refused, which is what the round-trip test in intents_test.go
// pins.
type Intent struct {
	// RequestID is NewRequestID()'s r-<26 crockford chars>: sortable, monotonic, and the key
	// the caller polls the status document for.
	RequestID string `json:"request_id"`
	Action    string `json:"action"` // publish|stop|start|rm|rollback
	SiteID    string `json:"site_id"`
	// ConfigPath is the site YAML the action acts on; empty means the engine derives it from
	// SiteID, which is the right answer unless the caller is creating a new site file.
	ConfigPath string `json:"config_path"`
	// ConfigFile is the staged YAML StageConfig wrote. Omitted for the non-publish actions.
	ConfigFile string `json:"config_file,omitempty"`
	// ExpectSHA256 is the CAS: the hash the live config must still carry. ALWAYS emitted.
	ExpectSHA256 string `json:"expect_sha256"`
	// SnapshotTo names the snapshot this intent takes of the state it replaces.
	SnapshotTo string `json:"snapshot_to,omitempty"`
	// SnapshotID selects the snapshot a rollback restores ("" = newest).
	SnapshotID string `json:"snapshot_id,omitempty"`
	// Trash, for rm, decides whether a removed config is archived (default) or not.
	Trash *bool `json:"trash,omitempty"`

	RequestedAt time.Time `json:"requested_at"`
	// DeadlineAt bounds the request's life: an intent whose deadline passed while the site was
	// in backoff is answered "expired" rather than firing later against a fleet the caller has
	// forgotten (A-15).
	DeadlineAt time.Time `json:"deadline_at"`
}

// ValidAction reports whether a is one of the fleet-mutating verbs.
func ValidAction(a string) bool {
	switch a {
	case IntentPublish, IntentStop, IntentStart, IntentRemove, IntentRollback:
		return true
	}
	return false
}

// validate is the engine's Intent.Validate, so a malformed request fails here, where the caller
// can still see it, instead of in a status file an hour later.
func (in Intent) validate() error {
	if !ValidAction(in.Action) {
		return fmt.Errorf("webserver: unknown intent action %q", in.Action)
	}
	if strings.TrimSpace(in.SiteID) == "" {
		return errors.New("webserver: intent needs a site_id")
	}
	if in.RequestID == "" {
		return errors.New("webserver: intent needs a request_id")
	}
	if in.Action == IntentPublish && strings.TrimSpace(in.ConfigFile) == "" {
		return errors.New("webserver: a publish intent needs config_file (the staged YAML)")
	}
	return nil
}

// Expired reports whether the deadline has passed as of now.
func (in Intent) Expired(now time.Time) bool {
	return !in.DeadlineAt.IsZero() && now.After(in.DeadlineAt)
}

const crockford = "0123456789abcdefghjkmnpqrshtvwxy"

// NewRequestID returns r-<26 lowercase Crockford base32 characters>: 48 bits of milliseconds
// since the epoch followed by 80 random bits.
//
// The algorithm, the alphabet (no i, l, o, u) and the bit packing are copied from
// pkg/fleet/intent.go verbatim, and that is load-bearing rather than stylistic (plan 2 A-1):
// the engine overwrites an intent whose id it already has, so ids must not merely be unique —
// they must be produced by the same function, and they must sort in mint order so the intake
// directory reads as a queue.
func NewRequestID() string {
	var b [16]byte
	ms := time.Now().UnixMilli()
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	if _, err := rand.Read(b[6:]); err != nil {
		// No entropy source: fall back to a nonce that is at least unique per call, exactly as
		// the engine does, rather than failing an intent that is otherwise well formed.
		copy(b[6:], []byte(hex.EncodeToString([]byte(fmt.Sprint(time.Now().UnixNano())))))
	}
	return "r-" + strings.ToLower(base32Crockford(b[:]))
}

// base32Crockford encodes 16 bytes into 26 characters without padding, 5 bits at a time from a
// shifting accumulator.
func base32Crockford(b []byte) string {
	var sb strings.Builder
	var acc uint32
	var bits uint
	for _, v := range b {
		acc = (acc << 8) | uint32(v)
		bits += 8
		for bits >= 5 {
			bits -= 5
			sb.WriteByte(crockford[(acc>>bits)&0x1F])
		}
	}
	if bits > 0 {
		sb.WriteByte(crockford[(acc<<(5-bits))&0x1F])
	}
	return strings.ToUpper(sb.String())
}

// WriteIntent files one request atomically and returns its request id.
//
// The order of operations is the whole function: create under O_EXCL in <dir>/.tmp so a
// partially written intent can never be observed by the reconciler, sync so the bytes are on the
// platter before the name exists, then STAT THE FINAL NAME and refuse if it is taken — the
// O_EXCL only guards the temporary name, and a rename over a pending intent is precisely the
// silent overwrite the request id exists to make impossible. The final name is never written
// directly, and an existing final name is reported as os.ErrExist so the caller's retry is a NEW
// id and a new ledger entry.
func WriteIntent(dir string, in Intent) (string, error) {
	if err := in.validate(); err != nil {
		return "", err
	}
	if in.RequestedAt.IsZero() {
		in.RequestedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("webserver: mkdir intent dir: %w", err)
	}
	tmp := filepath.Join(dir, ".tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return "", fmt.Errorf("webserver: mkdir intent tmp: %w", err)
	}

	name := in.RequestID + ".json"
	tmpPath := filepath.Join(tmp, name)
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("webserver: intent %s already exists (retry with a new request_id): %w", in.RequestID, err)
		}
		return "", fmt.Errorf("webserver: create intent: %w", err)
	}
	// 2-space indent, like the engine: these files are read in a terminal by whoever is
	// debugging the fleet at 03:14.
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(in); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("webserver: encode intent: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("webserver: sync intent: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("webserver: close intent: %w", err)
	}

	final := filepath.Join(dir, name)
	if _, err := os.Stat(final); err == nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("webserver: intent %s already exists (retry with a new request_id): %w", in.RequestID, os.ErrExist)
	}
	// Within one filesystem (dir/.tmp -> dir), so the rename is atomic: the reconciler either
	// sees a complete intent or does not see it.
	if err := os.Rename(tmpPath, final); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("webserver: publish intent: %w", err)
	}
	return in.RequestID, nil
}

// RequestsPathFor is <fleetDir>/.zen/requests — the same string the engine computes in
// pkg/fleet.RequestsDir, and the directory both WriteIntent and StageConfig act on. Like
// StatusPathFor, it is the one place this repo spells the layout out.
func RequestsPathFor(fleetDir string) string {
	return filepath.Join(filepath.Dir(StatusPathFor(fleetDir)), "requests")
}

// IntentPath is the file WriteIntent names for one request id.
func IntentPath(dir, requestID string) string { return filepath.Join(dir, requestID+".json") }

// ReadIntent decodes one intent file, which is how a caller verifies what it filed before the
// reconciler claims it (and how the intake directory is debugged at all).
func ReadIntent(path string) (Intent, error) {
	var in Intent
	b, err := os.ReadFile(path)
	if err != nil {
		return in, fmt.Errorf("webserver: read intent %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return in, fmt.Errorf("webserver: parse intent %s: %w", filepath.Base(path), err)
	}
	return in, in.validate()
}

// StageConfig writes the YAML bytes a publish intent will apply and returns the path that goes
// in Intent.ConfigFile. The bytes live in their own file so the hash the caller computed is the
// hash of what lands on disk, with no JSON escaping in between.
//
// dir is the fleet's requests directory (the one WriteIntent takes); the file lands under its
// staged/ subdirectory, which is where the engine's own StagingDir puts them.
func StageConfig(dir, id string, raw []byte) (string, error) {
	staged := filepath.Join(dir, "staged")
	if err := os.MkdirAll(staged, 0o700); err != nil {
		return "", fmt.Errorf("webserver: mkdir staged: %w", err)
	}
	name := sanitizeName(id) + ".yaml"
	tmp := filepath.Join(staged, ".tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return "", fmt.Errorf("webserver: mkdir staging tmp: %w", err)
	}
	tmpPath := filepath.Join(tmp, name)
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("webserver: create staged config: %w", err)
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("webserver: write staged config: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("webserver: sync staged config: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("webserver: close staged config: %w", err)
	}
	// Rename rather than write in place: an intent is consumed within one reconcile interval,
	// so a half-written YAML would otherwise be a plausible read for the reconciler.
	final := filepath.Join(staged, name)
	if err := os.Rename(tmpPath, final); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("webserver: publish staged config: %w", err)
	}
	return final, nil
}

// maxSanitizedNameLen bounds one staged file name. The engine's own sanitizeName has no cap; a
// caller-supplied id can be arbitrarily long, and a 4 KB file name is not a file name.
const maxSanitizedNameLen = 80

// sanitizeName mirrors pkg/fleet's sanitizeName: [A-Za-z0-9._-] survive, everything else — most
// importantly '/' — becomes '-', so an id can never escape the staging directory.
func sanitizeName(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			sb.WriteRune(r)
		default:
			sb.WriteByte('-')
		}
	}
	out := sb.String()
	if len(out) > maxSanitizedNameLen {
		out = out[:maxSanitizedNameLen]
	}
	if out == "" {
		return "intent"
	}
	return out
}

// Wait polls the status document until it carries a verdict for requestID, the deadline passes,
// or ctx ends. It returns the document it read, so the caller can report what the fleet was
// doing rather than only that it was not answering.
//
// The site's own applied_request_id also satisfies the wait: a caller that edited the YAML
// directly (A-14) has no ledger entry to look for under its id, and the engine records the
// request it applied on the site.
func Wait(ctx context.Context, statusPath, requestID, siteID string, deadline time.Time) (Status, RequestResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var (
		last     Status
		lastRead error
	)
	for {
		st, err := Read(statusPath)
		if err != nil {
			lastRead = err
		} else {
			last = st
			lastRead = nil
			if res, ok := LatestRequest(st, requestID); ok {
				return st, res, nil
			}
			if siteID != "" {
				if res, ok := appliedForSite(st, siteID); ok && res.RequestID == requestID {
					return st, res, nil
				}
			}
		}

		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return last, RequestResult{}, noVerdict(requestID, statusPath, deadline, last, lastRead, "deadline passed")
		}
		select {
		case <-ctx.Done():
			return last, RequestResult{}, fmt.Errorf("%w (%w)", noVerdict(requestID, statusPath, deadline, last, lastRead, "wait cancelled"), ctx.Err())
		case <-time.After(waitPollInterval):
		}
	}
}

// noVerdict builds the timeout error. It names the document's last pass and boot so "no
// verdict" is a report an agent can act on (is the reconciler alive?) rather than a bare
// timeout, and it wraps ErrNoVerdict so the caller can branch on it with IsNoVerdict.
func noVerdict(requestID, statusPath string, deadline time.Time, last Status, lastRead error, why string) error {
	if lastRead != nil {
		return fmt.Errorf("webserver: request %s: no verdict by %s (%s) and the status document %s is unreadable (%v): %w",
			requestID, deadline.Format(time.RFC3339), why, statusPath, lastRead, ErrNoVerdict)
	}
	return fmt.Errorf("webserver: request %s: no verdict by %s (%s; last boot %s pass %d): %w",
		requestID, deadline.Format(time.RFC3339), why, last.BootID, last.PassSeq, ErrNoVerdict)
}

// appliedForSite is the engine's fallback: the site's own record of the request it applied.
func appliedForSite(st Status, siteID string) (RequestResult, bool) {
	for _, s := range st.Sites {
		if (s.SiteID == siteID || s.ID == siteID) && s.AppliedReqID != "" {
			return RequestResult{RequestID: s.AppliedReqID, Result: "applied", At: st.GeneratedAt}, true
		}
	}
	return RequestResult{}, false
}

// sha256File computes the lowercase hex SHA-256 of a file — the value ExpectSHA256 and the
// status document's applied_sha256 carry. Streaming, because the same helper is handed a config
// and a snapshot directory entry interchangeably.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("webserver: open %s for hashing: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("webserver: hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
