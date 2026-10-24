package bridge

import (
	"context"
	"strings"
	"sync"
	"time"

	"zen-mcp/internal/mcpcfg"
)

// Tab-addressed bridge calls.
//
// The bridge keeps a registry of every tab and hands out a stable id
// (JS/bridge/tab_manager.mjs): `navigate` reports that id, every frame action
// echoes it back together with the URL it really ran on, and an id that no longer
// resolves is a hard `tab_not_found` rather than a read of whatever tab happens to
// be selected. A caller that carries the id on every step therefore cannot land on
// another caller's page — which is the race the global Gate exists to prevent
// (see gate.go).
//
// So addressed calls skip the global queue and are serialized per tab id instead:
// two flows on two tabs run at the same time, two callers driving one tab still
// wait for each other. Actions that steer the real mouse/keyboard, or that change
// or depend on the tab selection, are not made safe by an id and keep the global
// turn (see FocusBound).
//
// `bridge.tabParallel: false` turns all of this off and puts every call back on
// the global queue, for a bridge build that does not report tab ids.

// tabIDPrefix is what the bridge's registry ids start with ("t_<time36>_<seq36>").
const tabIDPrefix = "t_"

// TabID returns the registry id a bridge response reports (navigate, active_tab,
// switch_tab, and the provenance echo of every frame action). "" when absent.
func TabID(res map[string]any) string {
	if res == nil {
		return ""
	}
	return registryID(res["id"])
}

// registryID returns v as a TabRegistry id, or "" for anything else. A numeric
// tab index is positional — it shifts when tabs open or close — so it is
// deliberately not treated as an address that licenses parallel calls.
func registryID(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, tabIDPrefix) || len(s) <= len(tabIDPrefix) {
		return ""
	}
	return s
}

// RegistryID reports whether a caller-supplied value is a TabRegistry id, ""
// otherwise. This is what licenses the parallel, per-tab routing of a call.
func RegistryID(v any) string { return registryID(v) }

// addressedTab returns the registry id a call is addressed to, "" when the call
// targets the active tab (no index) or a positional index.
func addressedTab(params map[string]any) string {
	if params == nil {
		return ""
	}
	return registryID(params["index"])
}

// opensOwnTab reports whether a call creates its own tab instead of touching the
// selected one. Only a background navigation qualifies: a foreground new_tab moves
// the selection, which is exactly the shared state the Gate protects.
func opensOwnTab(action string, params map[string]any) bool {
	if action != "navigate" && action != "new_tab" {
		return false
	}
	if boolParam(params, "back_tab") || boolParam(params, "background") || boolParam(params, "inBackground") {
		return true
	}
	// navigate's JS side also treats an explicit active:false as background.
	if active, ok := params["active"].(bool); ok && !active {
		return true
	}
	return false
}

func boolParam(params map[string]any, key string) bool {
	if params == nil {
		return false
	}
	v, _ := params[key].(bool)
	return v
}

// focusBoundActions are the bridge actions that drive the real cursor or keyboard
// through xdotool / windowUtils, or that need the target tab to be the selected
// one. An id does not make them independent of each other, so they keep the
// global turn even when addressed.
var focusBoundActions = map[string]bool{
	"human_click":           true,
	"trusted_click":         true,
	"native_click":          true,
	"hover":                 true,
	"trusted_hover":         true,
	"native_hover":          true,
	"drag_and_drop":         true,
	"trusted_drag_and_drop": true,
	"native_drag_drop":      true,
	"native_type":           true,
	"trusted_type":          true,
	"native_paste":          true,
	"trusted_paste":         true,
	"key_combo":             true,
	"trusted_key_combo":     true,
	"solve_challenge":       true,
	"screenshot":            true,
	"switch_tab":            true,
	"activate":              true,
}

// FocusBound reports whether an action needs exclusive use of the browser window
// regardless of which tab it is addressed to.
func FocusBound(action string) bool { return focusBoundActions[action] }

// tabParallelEnabled reports whether addressed calls may run concurrently. On by
// default; `bridge.tabParallel: false` restores strict one-by-one ordering.
func tabParallelEnabled() bool {
	c := mcpcfg.Get()
	return c == nil || c.Bridge.TabParallel == nil || *c.Bridge.TabParallel
}

// --- per-tab queues -----------------------------------------------------

// tabGateRef is one tab's queue plus a reference count, so an idle queue can be
// dropped without racing a caller that is about to acquire it.
type tabGateRef struct {
	gate *Gate
	refs int
}

var (
	tabGatesMu sync.Mutex
	tabGates   = map[string]*tabGateRef{}
)

// tabGateFor takes a reference on the queue for one tab id, creating it on first
// use. The caller must hand the reference back with untabGate.
func tabGateFor(tabID string) *Gate {
	tabGatesMu.Lock()
	defer tabGatesMu.Unlock()
	ref, ok := tabGates[tabID]
	if !ok {
		_, ceiling := bridgePolicy()
		g := NewGate(1, time.Duration(ceiling)*time.Millisecond)
		// follow the configured queue ceiling live, like the process-wide gate
		g.policy = tabGatePolicy
		ref = &tabGateRef{gate: g}
		tabGates[tabID] = ref
	}
	ref.refs++
	return ref.gate
}

func untabGate(tabID string) {
	tabGatesMu.Lock()
	defer tabGatesMu.Unlock()
	ref, ok := tabGates[tabID]
	if !ok {
		return
	}
	ref.refs--
	if ref.refs <= 0 {
		delete(tabGates, tabID)
	}
}

// tabGatePolicy keeps one tab strictly sequential and inherits the queue ceiling
// of the `bridge` config block.
func tabGatePolicy() (int, time.Duration) {
	_, ceiling := bridgePolicy()
	return 1, ceiling
}

// TabQueueStatus reports the queue of one tab id, for diagnostics.
func TabQueueStatus(tabID string) (Status, bool) {
	tabGatesMu.Lock()
	ref, ok := tabGates[tabID]
	tabGatesMu.Unlock()
	if !ok {
		return Status{}, false
	}
	return ref.gate.Status(), true
}

// --- turn scoping -------------------------------------------------------

type tabTurnKey struct{}

type tabTurn struct{ id string }

// TabTurn returns the tab id ctx holds a reservation for, "" when it holds none.
func TabTurn(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	t, _ := ctx.Value(tabTurnKey{}).(*tabTurn)
	if t == nil {
		return ""
	}
	return t.id
}

// WithTabTurn reserves one registry tab for a whole sequence of addressed calls
// and returns the context to run them on. Every CallBridge made with that context
// for the same tab id reuses the reservation, so navigate -> wait -> read cannot be
// interleaved with a second caller driving the same tab, while calls for other tab
// ids run concurrently.
//
// An empty tabID means "the active tab", which is not reservable: the context is
// returned unchanged with a no-op release, and the calls fall back to the global
// queue.
func WithTabTurn(ctx context.Context, tabID, label string) (context.Context, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if tabID == "" || TabTurn(ctx) == tabID {
		return ctx, func() {}, nil
	}
	release, err := tabGateFor(tabID).Acquire(ctx, label)
	if err != nil {
		untabGate(tabID)
		return ctx, nil, err
	}
	turnCtx := context.WithValue(ctx, tabTurnKey{}, &tabTurn{id: tabID})
	return turnCtx, func() {
		release()
		untabGate(tabID)
	}, nil
}
