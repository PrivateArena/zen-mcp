package tools

// Where the two memory boards live.
//
// Until plan 2 this was a literal: `http://127.0.0.1:3034` for the isolated board and `:3035` for
// the shared one. That is three sites of the same bug the whole exercise exists to remove — the
// board is a zen-webserver site like any other, its port is a value in a YAML file, and a literal
// here meant that rotating a site's port silently turned a memory write into an unreachable-host
// error, or worse, into a write to whatever else had taken the number (plan 2 A-8; plan 1 F-18 is
// the same failure from the engine's side).
//
// So this resolves through the fleet's status document, which is ground truth: the engine wrote it,
// it is updated on every reconcile, and it says which site is on which port *right now*. The
// fallback port is kept deliberately, because a fleet that has never been started has no status
// document — first boot, or a fleet an operator runs some other way — and refusing to start until a
// file appears would be a worse outcome than the literal it replaces. The default fallbacks are the
// ports those two sites have always had, so a machine with no status file behaves byte-identically
// to before this file existed. That is the property that makes the change safe to land.

import (
	"fmt"
	"time"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/webserver"
)

// boardKind selects which memory board a call is about.
type boardKind int

const (
	boardIsolate boardKind = iota
	boardShared
)

// Default board sites, mirroring the two entries the config block defaults to. Kept here so a
// zero-value config (no webserver block at all — the state of every existing install) still
// resolves to the same URL it used to have hardcoded.
const (
	defaultIsolateSite = "note-ai"
	defaultIsolatePort = 3034
	defaultSharedSite  = "memory"
	defaultSharedPort  = 3035
)

// boardBase returns the base URL for one memory board. It never fails: an unresolvable board falls
// back to the configured port, because these tools have always had a URL and callers treat an
// error here as "the board is down" rather than "the config is wrong".
func boardBase(workspace string, kind boardKind) string {
	site, port := defaultBoard(kind)

	var block mcpcfg.WebserverConfig
	if cfg := mcpcfg.For(workspace); cfg != nil {
		block = cfg.Webserver
	}
	if wb := block.Whiteboards; wb != nil {
		entry := wb.Isolate
		if kind == boardShared {
			entry = wb.Shared
		}
		if entry.Site != "" {
			site = entry.Site
		}
		if entry.FallbackPort > 0 {
			port = entry.FallbackPort
		}
	}

	if base, ok := boardBaseFromStatus(block, site); ok {
		return base
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// boardBaseFromStatus resolves a board's site through the fleet status document. Reports false when
// there is no fleet configured, no readable document, no such site, or a document too old to trust —
// the same staleness bound the engine's own health view uses (plan 1 F-18: three intervals).
func boardBaseFromStatus(block mcpcfg.WebserverConfig, site string) (string, bool) {
	path := block.StatusFile
	if path == "" && block.FleetDir != "" {
		path = webserver.StatusPathFor(block.FleetDir)
	}
	if path == "" {
		return "", false
	}
	st, err := webserver.Read(path)
	if err != nil {
		return "", false
	}
	// StaleFor applies the 3x-interval bound itself (plan 1 F-18); an unparseable or absent
	// generated_at is an error, and an untrustworthy document falls back to the configured port
	// rather than routing a board write somewhere guessed.
	if _, err := st.StaleFor(time.Now()); err != nil {
		return "", false
	}
	resolved, err := webserver.ResolveSite(st, site)
	if err != nil {
		return "", false
	}
	// The status document records a port, not a host: the fleet binds loopback, and LoopbackBase
	// supplies (and validates) that literal rather than trusting a caller to name one.
	base, err := webserver.LoopbackBase("", resolved.Port)
	if err != nil {
		return "", false
	}
	return base, true
}

func defaultBoard(kind boardKind) (string, int) {
	if kind == boardShared {
		return defaultSharedSite, defaultSharedPort
	}
	return defaultIsolateSite, defaultIsolatePort
}
