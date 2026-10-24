package tools

import (
	"context"
	"fmt"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/logfilter"
)

// Tab-addressed page flows.
//
// A page flow (fetch, search, browser read) is a sequence of bridge calls that all
// have to land on one page. The bridge's TabRegistry makes that addressable:
// `navigate` creates its own background tab and reports its id, and the id is then
// carried on every step, so nothing in the sequence depends on which tab is
// selected and concurrent flows cannot read each other's pages. The routing side
// lives in internal/bridge/address.go.

// navigateParamsTab is the navigate payload of a tab flow: its own background tab,
// never the caller's selection.
func navigateParamsTab(targetURL string, timeoutMs int, container string) map[string]any {
	params := map[string]any{
		"url":      targetURL,
		"back_tab": true,
		"timeout":  timeoutMs,
	}
	if container != "" {
		params["container"] = container
	}
	return params
}

// requireTabID reads the TabRegistry id out of a navigate report. A bridge that
// reports none cannot run a tab flow: without an id the follow-up calls would
// silently address the selected tab, which is the failure these flows exist to
// remove, so it is reported instead of papered over.
func requireTabID(nav map[string]any, targetURL string) (string, error) {
	tabID := bridge.TabID(nav)
	if tabID == "" {
		return "", fmt.Errorf("navigate to %s reported no tab id: the bridge tab registry is unavailable, update the userChrome bridge", targetURL)
	}
	return tabID, nil
}

// withTab returns a copy of params addressed to one registry tab.
func withTab(params map[string]any, tabID string) map[string]any {
	out := make(map[string]any, len(params)+1)
	for k, v := range params {
		if k != "index" {
			out[k] = v
		}
	}
	out["index"] = tabID
	return out
}

// addressStep addresses a params map to tabID, leaving it untouched for the
// active-tab flow that has no id to address with.
func addressStep(params map[string]any, tabID string) map[string]any {
	if tabID == "" {
		return params
	}
	return withTab(params, tabID)
}

// tabStepParams is what a bridge call must not inherit from a tab flow: the
// navigation arguments are consumed by navigate, not by the frame action.
func tabStepParams(params map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for k, v := range params {
		switch k {
		case "url", "new_tab", "back_tab", "keep_tab", "index":
			continue
		}
		out[k] = v
	}
	return out
}

// closeTabFlow closes the tab a flow opened. Best effort: the tab is either the
// flow's own or already gone, and a failed cleanup must not fail the extraction.
func closeTabFlow(ctx context.Context, tabID string) {
	if tabID == "" {
		return
	}
	if _, err := bridge.CallBridge(ctx, "close_tab", map[string]any{"index": tabID}); err != nil {
		logfilter.Debugf("[tabflow] could not close tab %s: %v", tabID, err)
	}
}

// verifyTab checks the provenance echo of a bridge response. Every frame action
// reports the tab it really ran on, so an id that is not the flow's own means the
// payload came from a stranger's page and must never be attributed to this URL.
// An absent echo is an older bridge and is accepted.
func verifyTab(res map[string]any, tabID, subject string) error {
	if tabID == "" {
		return nil // active-tab flow: nothing was addressed, so there is nothing to verify
	}
	if got := bridge.TabID(res); got != "" && got != tabID {
		return fmt.Errorf("%s was read from tab %s instead of %s", subject, got, tabID)
	}
	return nil
}

// keepTabArg reports whether the caller asked to keep the tab open.
func keepTabArg(v any) bool {
	b, _ := v.(bool)
	return b
}
