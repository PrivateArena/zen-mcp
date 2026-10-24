package tools

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"zen-mcp/internal/bridge"
)

// The tab-addressed flow contract, apart from the content tools: a flow opens its
// own tab, addresses every step with its registry id, hands the tab back, and gives
// up loudly rather than reading somebody else's page when any of that is not
// available.

func TestFlowClosesItsTabAfterExtraction(t *testing.T) {
	stub := bridgeStubAt(t, fetchOKResponses)

	HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/"}))

	tabID := stub.lastTabID()
	closed := stub.byAction("close_tab")
	if closed == nil {
		t.Fatalf("the flow left its tab behind; calls=%v", stub.actions())
	}
	if closed["index"] != tabID {
		t.Errorf("close_tab index = %v, want the flow's tab %v", closed["index"], tabID)
	}
	if got := bridge.RegistryID(closed["index"]); got == "" {
		t.Errorf("close_tab must address a registry id, got %v", closed["index"])
	}
}

func TestFlowKeepsTheTabWhenAsked(t *testing.T) {
	stub := bridgeStubAt(t, fetchOKResponses)

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{
		"url": "https://example.com/", "keep_tab": true,
	}))

	if doc := docOf(t, res); doc.body != fetchPageMarkdown {
		t.Errorf("body = %q", probeSnippet(doc.body))
	}
	if closed := stub.byAction("close_tab"); closed != nil {
		t.Errorf("keep_tab must leave the tab open, got %v", closed)
	}
}

func TestFlowClosesItsTabEvenWhenExtractionFails(t *testing.T) {
	// A failed read must not orphan a tab: the cleanup runs with the reservation,
	// not only on the success path.
	stub := bridgeStubStatusAt(t, map[string]string{
		"navigate":              fetchNavResponse,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  `{"success":false,"error":"TypeError: win is undefined"}`,
	}, map[string]int{"read": http.StatusInternalServerError})

	HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/"}))

	if closed := stub.byAction("close_tab"); closed == nil {
		t.Errorf("a failed read left its tab behind; calls=%v", stub.actions())
	}
}

func TestFlowRefusesABridgeWithoutTabRegistry(t *testing.T) {
	// Without an id the follow-up calls would silently address the selected tab, so
	// the flow reports that instead of reading a page it never asked for.
	stub := bridgeStubAt(t, map[string]string{
		"navigate": fetchNavResponse,
		"read":     fmt.Sprintf(`{"success":true,"data":{"content":%q}}`, fetchPageMarkdown),
	}).withoutTabRegistry()

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/"}))

	text := resultText(t, res)
	if !strings.Contains(text, "no tab id") {
		t.Errorf("expected the missing-registry error, got %q", probeSnippet(text))
	}
	if stub.byAction("read") != nil {
		t.Errorf("no step may run after an unaddressable navigation; calls=%v", stub.actions())
	}
}

func TestFlowRejectsAMismatchedProvenanceEcho(t *testing.T) {
	// The bridge echoes the tab a frame action really ran on. An echo that is not
	// the flow's own tab means the payload came from somewhere else, and it must
	// never be attributed to the requested URL.
	stub := bridgeStubAt(t, map[string]string{
		"navigate":              fetchNavResponse,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  `{"success":true,"id":"t_stranger_9","data":{"content":"someone else's page"}}`,
	})

	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(map[string]any{"url": "https://example.com/"}))

	text := resultText(t, res)
	if !strings.Contains(text, "t_stranger_9") || !strings.Contains(text, "instead of") {
		t.Errorf("expected the provenance mismatch error, got %q", probeSnippet(text))
	}
	if strings.Contains(text, "someone else's page") {
		t.Errorf("a foreign payload must never be returned: %q", probeSnippet(text))
	}
	if closed := stub.byAction("close_tab"); closed == nil {
		t.Errorf("the flow still must clean its own tab up; calls=%v", stub.actions())
	}
}

func TestBrowserReadAddressesTheGivenTab(t *testing.T) {
	// An index that is a registry id is the whole address: no navigation, and the
	// read goes to exactly that tab.
	stub := bridgeStubAt(t, map[string]string{
		"read": `{"success":true,"id":"t_mvm0rsyp_5v3","data":{"content":"# Held tab"}}`,
	})

	res := HandleBrowserAction(context.Background(), t.TempDir(), Deps{}, browserReq(map[string]any{
		"action": "read", "index": "t_mvm0rsyp_5v3",
	}))

	if got := resultText(t, res); !strings.Contains(got, "# Held tab") {
		t.Fatalf("read returned %q", probeSnippet(got))
	}
	read := stub.byAction("read")
	if read["index"] != "t_mvm0rsyp_5v3" {
		t.Errorf("read index = %v", read["index"])
	}
	if nav := stub.byAction("navigate"); nav != nil {
		t.Errorf("an addressed read must not navigate, got %v", nav)
	}
	if closed := stub.byAction("close_tab"); closed != nil {
		t.Errorf("a tab the caller owns must not be closed, got %v", closed)
	}
}

func TestBrowserReadNavigatesTheGivenTabInPlace(t *testing.T) {
	stub := bridgeStubAt(t, map[string]string{
		"navigate": `{"success":true,"id":"t_mvm0rsyp_5v3","url":"https://example.com/","status":200}`,
		"read":     `{"success":true,"id":"t_mvm0rsyp_5v3","data":{"content":"# Reloaded"}}`,
	})

	HandleBrowserAction(context.Background(), t.TempDir(), Deps{}, browserReq(map[string]any{
		"action": "read", "index": "t_mvm0rsyp_5v3", "url": "https://example.com/",
	}))

	nav := stub.byAction("navigate")
	if nav == nil {
		t.Fatal("the addressed tab was never navigated")
	}
	if nav["index"] != "t_mvm0rsyp_5v3" {
		t.Errorf("navigate must address the caller's tab, got %v", nav["index"])
	}
	if nav["back_tab"] != nil {
		t.Errorf("navigating an existing tab must not open a new one: %v", nav)
	}
	if closed := stub.byAction("close_tab"); closed != nil {
		t.Errorf("a tab the caller owns must not be closed, got %v", closed)
	}
}

func TestBrowserReadWithoutAnAddressStillUsesTheGlobalQueue(t *testing.T) {
	// The legacy shape — read whatever is selected — keeps the old serialization.
	stub := bridgeStubAt(t, map[string]string{
		"read": `{"success":true,"data":{"content":"# Active"}}`,
	})

	res := HandleBrowserAction(context.Background(), t.TempDir(), Deps{}, browserReq(map[string]any{"action": "read"}))

	if got := resultText(t, res); !strings.Contains(got, "# Active") {
		t.Fatalf("read returned %q", probeSnippet(got))
	}
	if read := stub.byAction("read"); read["index"] != nil {
		t.Errorf("an unaddressed read must not invent an index, got %v", read["index"])
	}
	if held := bridge.Held(context.Background()); held {
		t.Error("no reservation outside a request")
	}
	if status := bridge.QueueStatus(); status.Limit != 1 {
		t.Errorf("the global queue must stay one-at-a-time, got limit %d", status.Limit)
	}
}

func TestBrowserSearchFlowIsAddressedAndCleansUp(t *testing.T) {
	stub := bridgeStubAt(t, map[string]string{
		"navigate":              fetchNavResponse,
		"wait_for_network_idle": `{"success":true,"data":{"idle":true}}`,
		"read":                  `{"success":true,"data":{"content":"# SERP"}}`,
	})

	HandleBrowserAction(context.Background(), t.TempDir(), Deps{}, browserReq(map[string]any{
		"action": "search", "url": "hyperframes rendering",
	}))

	tabID := stub.lastTabID()
	calls := stub.actions()
	want := []string{"navigate", "wait_for_network_idle", "read", "close_tab"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i, action := range want {
		if calls[i] != action {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
	if read := stub.byAction("read"); read["index"] != tabID {
		t.Errorf("read is not addressed to the flow's tab: %v != %v", read["index"], tabID)
	}
	if read := stub.byAction("read"); read["url"] != nil {
		t.Errorf("the navigation argument must not leak into the read, got %v", read["url"])
	}
}
