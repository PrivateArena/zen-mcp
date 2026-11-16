package tools

// The board identity (plan 2 §4.2). What these tests pin down is the failure mode the design turns
// on: an X-AI-Agent header with no credential behind it is REFUSED by the engine, so a
// half-configured identity must degrade to the anonymous client rather than "improve" a working
// board write into a 403.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/whiteboard"
)

func boardConfigWorkspace(t *testing.T, agent, credFile string) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenmcp"), 0o755); err != nil {
		t.Fatal(err)
	}
	wb := map[string]any{"site": "note-ai", "fallbackPort": 3034}
	if agent != "" {
		wb["agent"] = agent
	}
	if credFile != "" {
		wb["credentialFile"] = credFile
	}
	b, _ := json.Marshal(map[string]any{"webserver": map[string]any{"whiteboards": map[string]any{"isolate": wb}}})
	if err := os.WriteFile(filepath.Join(ws, ".zenmcp", "config.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mcpcfg.ResetWorkspaceTiers() })
	return ws
}

// headerSpy records what the identity pair put on the wire.
func headerSpy(t *testing.T) (base string, got func() http.Header) {
	t.Helper()
	var mu sync.Mutex
	var last http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		last = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cards":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() http.Header { mu.Lock(); defer mu.Unlock(); return last }
}

func TestBoardClientSendsNoIdentityByDefault(t *testing.T) {
	base, got := headerSpy(t)
	c := whiteboard.NewClient(base, "slug", "Title", "slug")
	// No Agent, no Credential: the header must be absent, which is the whole reason the existing
	// memory tools keep working unchanged.
	if err := c.UpsertCard(context.Background(), "slug", "T", "body", ""); err != nil {
		t.Fatalf("write: %v", err)
	}
	h := got()
	if v := h.Get("X-AI-Agent"); v != "" {
		t.Errorf("X-AI-Agent = %q on an unconfigured client; a claim without a credential is refused by the engine", v)
	}
	if v := h.Get("Cookie"); v != "" {
		t.Errorf("Cookie = %q on an unconfigured client", v)
	}
}

// TestBoardIdentityCoversReadsToo: a read that carries no identity is admitted as the loopback
// owner, so allowed_actions would govern writes and not reads (plan 2 A-17's whole point).
func TestBoardIdentityCoversReadsToo(t *testing.T) {
	base, got := headerSpy(t)
	c := whiteboard.NewClient(base, "slug", "Title", "slug").WithIdentity("zen-mcp", "labcookie=abc123")
	if _, err := c.LoadBoardState(context.Background()); err != nil {
		t.Fatalf("load: %v", err)
	}
	h := got()
	if h.Get("X-AI-Agent") != "zen-mcp" || h.Get("Cookie") != "labcookie=abc123" {
		t.Errorf("a read carried no identity (agent=%q cookie=%q); it would be authenticated as the owner", h.Get("X-AI-Agent"), h.Get("Cookie"))
	}
}

func TestBoardClientSendsBothOrNeither(t *testing.T) {
	base, got := headerSpy(t)
	// Both halves present: header and cookie travel together, and the cookie is the file's content
	// verbatim (a `name=value` rule, not just the value).
	c := whiteboard.NewClient(base, "slug", "Title", "slug").WithIdentity("zen-mcp", "labcookie=abc123")
	if err := c.UpsertCard(context.Background(), "slug", "T", "body", ""); err != nil {
		t.Fatal(err)
	}
	h := got()
	if h.Get("X-AI-Agent") != "zen-mcp" {
		t.Errorf("X-AI-Agent = %q", h.Get("X-AI-Agent"))
	}
	if h.Get("Cookie") != "labcookie=abc123" {
		t.Errorf("Cookie = %q, want the whole name=value rule", h.Get("Cookie"))
	}

	// WithIdentity given only half an identity returns the original client: there is no
	// header-without-credential state to misconfigure into.
	half := whiteboard.NewClient(base, "s", "T", "s").WithIdentity("zen-mcp", "")
	if half.Agent != "" || half.Credential != "" {
		t.Error("WithIdentity accepted a header with no credential")
	}
}

func TestBoardClientRefusesReadableCredential(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.cookie")
	if err := os.WriteFile(path, []byte("labcookie=abc123\n"), 0o644); err != nil { // group-readable
		t.Fatal(err)
	}
	if _, err := readBoardCredential(path); err == nil {
		t.Error("a 0644 credential file was accepted; the engine refuses to read it, so sending it is a guaranteed 403")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := readBoardCredential(path)
	if err != nil {
		t.Fatalf("0600 file rejected: %v", err)
	}
	if v != "labcookie=abc123" {
		t.Errorf("credential = %q, want the trimmed rule", v)
	}
}

func TestBoardEntryDefaultsAreTheHistoricPorts(t *testing.T) {
	// No config at all: the boards must resolve to the sites they have always lived on, because
	// that is what makes landing the status-file resolution a no-op on an unconfigured machine.
	ws := t.TempDir()
	if got := boardEntry(ws, boardIsolate); got.Site != "note-ai" || got.FallbackPort != 3034 {
		t.Errorf("isolate default = %+v, want note-ai/3034", got)
	}
	if got := boardEntry(ws, boardShared); got.Site != "memory" || got.FallbackPort != 3035 {
		t.Errorf("shared default = %+v, want memory/3035", got)
	}
	if got := boardClient(ws, boardIsolate, "s", "T", "s"); got.Agent != "" {
		t.Error("an unconfigured workspace must not claim an agent identity")
	}

	// And the configured shape: these are the JSON key names an operator types into config.json,
	// so a typo in the struct tag is invisible everywhere except here.
	cred := filepath.Join(t.TempDir(), "b.cookie")
	if err := os.WriteFile(cred, []byte("labcookie=abc123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws2 := boardConfigWorkspace(t, "zen-mcp", cred)
	got := boardEntry(ws2, boardIsolate)
	if got.Agent != "zen-mcp" || got.CredentialFile != cred {
		t.Errorf("configured board = %+v; webserver.whiteboards.isolate.{agent,credentialFile} did not parse", got)
	}
	c := boardClient(ws2, boardIsolate, "s", "T", "s")
	if c.Agent != "zen-mcp" || c.Credential != "labcookie=abc123" {
		t.Errorf("boardClient did not apply the configured identity (agent=%q)", c.Agent)
	}
	// The isolate entry must not leak into the shared board's identity.
	if other := boardEntry(ws2, boardShared); other.Agent != "" {
		t.Error("the isolate board's agent identity leaked into the shared board")
	}
}
