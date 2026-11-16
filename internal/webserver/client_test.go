package webserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeStatus marshals a Status to a temp fleet-status.json and returns its path. Every
// Resolve/Call test needs a document whose port is the httptest listener's, so the documents are
// built here rather than in testdata.
func writeStatus(t *testing.T, st Status) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".zen"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	p := filepath.Join(dir, ".zen", "fleet-status.json")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}
	return p
}

// freshStatus is a document the staleness check accepts: generated now, on the fleet's own
// 3 s reconcile interval (3x of which is the bound).
func freshStatus(port int) Status {
	return Status{
		Schema:      1,
		BootID:      "boot-test",
		PID:         4242,
		PassSeq:     7,
		GeneratedAt: time.Now().Format(time.RFC3339Nano),
		IntervalMS:  3000,
		Sites:       []SiteState{{ID: "dummy", Port: port, State: "running", ConfigPath: "/tmp/dummy.yaml"}},
	}
}

// serverStatusPath points a client at an httptest listener: its host IS loopback, so
// Resolve's real checks run instead of a stubbed-out path.
func serverStatusPath(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse httptest URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("httptest port: %v", err)
	}
	if u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		t.Fatalf("httptest listener is not loopback: %s", u.Host)
	}
	return writeStatus(t, freshStatus(port))
}

func TestAPIURL(t *testing.T) {
	for _, tc := range []struct{ base, suffix, want string }{
		{"http://127.0.0.1:3099", "doctor", "http://127.0.0.1:3099/_zen/api/doctor"},
		{"http://127.0.0.1:3099", "/doctor", "http://127.0.0.1:3099/_zen/api/doctor"},
		{"http://127.0.0.1:3099/", "whiteboard/state", "http://127.0.0.1:3099/_zen/api/whiteboard/state"},
		{"http://127.0.0.1:3099", "doctor?deep=true", "http://127.0.0.1:3099/_zen/api/doctor?deep=true"},
	} {
		if got := apiURL(tc.base, tc.suffix); got != tc.want {
			t.Errorf("apiURL(%q,%q) = %q, want %q", tc.base, tc.suffix, got, tc.want)
		}
	}
}

// TestCallSelectsMethodAndBody is the "GET when body is nil, POST-with-JSON otherwise" rule the
// read endpoints depend on: the engine reads r.URL.Query() first and the body second, so a GET
// is valid for them, while every mutating endpoint is POST-only.
func TestCallSelectsMethodAndBody(t *testing.T) {
	type seen struct {
		method, path, query, ctype string
		body                       map[string]any
		raw                        string
	}
	var got []seen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := seen{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, ctype: r.Header.Get("Content-Type")}
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			s.raw = string(b)
			_ = json.Unmarshal(b, &s.body)
		}
		got = append(got, s)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := New(serverStatusPath(t, srv), 2*time.Second)
	if _, err := c.Call(context.Background(), "dummy", "catalog", nil); err != nil {
		t.Fatalf("GET call: %v", err)
	}
	if _, err := c.Call(context.Background(), "dummy", "save", map[string]any{"slug": "a"}); err != nil {
		t.Fatalf("POST call: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("handler saw %d requests", len(got))
	}
	if got[0].method != http.MethodGet || got[0].path != "/_zen/api/catalog" {
		t.Errorf("nil body did not produce GET /_zen/api/catalog: %s %s", got[0].method, got[0].path)
	}
	if got[0].ctype != "" {
		t.Errorf("a GET carried a content type: %q", got[0].ctype)
	}
	if got[1].method != http.MethodPost || got[1].path != "/_zen/api/save" {
		t.Errorf("non-nil body did not produce POST /_zen/api/save: %s %s", got[1].method, got[1].path)
	}
	if !strings.Contains(got[1].ctype, "application/json") {
		t.Errorf("POST content type = %q", got[1].ctype)
	}
	if got[1].body["slug"] != "a" {
		t.Errorf("POST body = %q", got[1].raw)
	}
}

// TestReadHelpersPutParamsInQuery: Doctor's deep flag and List/Get params are query parameters,
// because that is what the handlers read.
func TestReadHelpersPutParamsInQuery(t *testing.T) {
	var paths, queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		queries = append(queries, r.URL.RawQuery)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()
	c := New(serverStatusPath(t, srv), 2*time.Second)

	if _, err := c.Doctor(context.Background(), "dummy", true); err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if _, err := c.Catalog(context.Background(), "dummy"); err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if _, err := c.Get(context.Background(), "dummy", "standup-2026-11-14"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := c.List(context.Background(), "dummy", url.Values{"per_page": []string{"100"}}); err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []struct{ path, query string }{
		{"/_zen/api/doctor", "deep=true"},
		{"/_zen/api/catalog", ""},
		{"/_zen/api/get", "slug=standup-2026-11-14"},
		{"/_zen/api/list", "per_page=100"},
	}
	for i, w := range want {
		if i >= len(paths) {
			t.Fatalf("only %d requests arrived", len(paths))
		}
		if paths[i] != w.path || queries[i] != w.query {
			t.Errorf("request %d = %s?%s, want %s?%s", i, paths[i], queries[i], w.path, w.query)
		}
	}
}

// TestCallDecodesEveryErrorCode pins the machine-readable half of the error contract: a caller
// branches on Code, and A-15's "do not retry into a wall" is only implementable if the code
// survives the decode.
func TestCallDecodesEveryErrorCode(t *testing.T) {
	codes := []struct {
		status int
		code   string
	}{
		{http.StatusNotFound, ErrUnknownEndpoint},
		{http.StatusMethodNotAllowed, ErrMethodNotAllowed},
		{http.StatusForbidden, ErrActorNotPermitted},
		{http.StatusForbidden, ErrAdminCPDisabled},
		{http.StatusBadRequest, ErrInvalidRequest},
		{http.StatusBadRequest, ErrReadOnlyQueryRejected},
	}
	for _, tc := range codes {
		tc := tc
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(tc.status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  tc.status,
				"code":    tc.code,
				"message": "a machine-readable reason",
				"path":    r.URL.Path,
			})
		}))
		c := New(serverStatusPath(t, srv), 2*time.Second)
		body, err := c.Call(context.Background(), "dummy", "list", nil)
		srv.Close()

		if err == nil {
			t.Fatalf("%s: error swallowed, body %s", tc.code, body)
		}
		var ae *APIError
		if !errors.As(err, &ae) {
			t.Fatalf("%s: error is %T, want *APIError", tc.code, err)
		}
		if ae.Code != tc.code {
			t.Errorf("%s: decoded code %q", tc.code, ae.Code)
		}
		if ae.Status != tc.status {
			t.Errorf("%s: decoded status %d, want %d", tc.code, ae.Status, tc.status)
		}
		if ae.Message == "" || ae.Path != "/_zen/api/list" {
			t.Errorf("%s: message/path lost: %+v", tc.code, ae)
		}
		if CodeOf(err) != tc.code {
			t.Errorf("CodeOf = %q, want %q", CodeOf(err), tc.code)
		}
		if got := IsNotImplemented(err); got != (tc.code == ErrUnknownEndpoint) {
			t.Errorf("IsNotImplemented(%s) = %v", tc.code, got)
		}
		if len(body) != 0 {
			t.Errorf("%s: a failure also returned a body: %s", tc.code, body)
		}
	}
}

// TestCallToleratesPlainHTTPErrorBody: several engine handlers still answer with http.Error, so
// a text/plain 500 is real and must arrive as a usable error rather than a decode failure.
func TestCallToleratesPlainHTTPErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "multipart too large", http.StatusRequestEntityTooLarge)
	}))
	defer srv.Close()
	c := New(serverStatusPath(t, srv), 2*time.Second)

	_, err := c.Call(context.Background(), "dummy", "media/upload", map[string]any{"x": 1})
	if err == nil {
		t.Fatal("no error for a 413")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("error is %T, want *APIError", err)
	}
	if ae.Code != "" {
		t.Errorf("a non-admin body invented a code: %q", ae.Code)
	}
	if ae.Status != http.StatusRequestEntityTooLarge || !strings.Contains(ae.Body, "multipart too large") {
		t.Errorf("body not preserved: %+v", ae)
	}
	if CodeOf(err) != "" {
		t.Errorf("CodeOf on a non-admin body = %q", CodeOf(err))
	}
	if !strings.Contains(err.Error(), "multipart too large") {
		t.Errorf("Error() drops the body: %v", err)
	}
}

// TestCallEmpty2xxIsAnError: F-20/A-1 is the bug the engine's error shape exists to close, and
// a client that returned (nil, nil) would reproduce it for the agent.
func TestCallEmpty2xxIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := New(serverStatusPath(t, srv), 2*time.Second)
	if _, err := c.Call(context.Background(), "dummy", "query", nil); err == nil {
		t.Error("an empty 2xx answered with no error")
	}
}

func TestResolveAndLoopbackBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	c := New(serverStatusPath(t, srv), 2*time.Second)
	base, err := c.Resolve(context.Background(), "dummy")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := "http://127.0.0.1:" + u.Port(); base != want {
		t.Errorf("Resolve = %q, want %q", base, want)
	}

	for _, tc := range []struct {
		host string
		ok   bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"", true}, // unset means "the loopback default"
		{"10.0.0.5", false},
		{"192.168.1.20", false},
		{"example.com", false},
		{"localhost", false}, // a name is not a literal loopback address
	} {
		got, err := LoopbackBase(tc.host, 3099)
		if tc.ok && err != nil {
			t.Errorf("LoopbackBase(%q) refused: %v", tc.host, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("LoopbackBase(%q) accepted: %s", tc.host, got)
		}
	}
	if _, err := LoopbackBase("127.0.0.1", 0); err == nil {
		t.Error("port 0 accepted")
	}
}

func TestResolveRefusesStaleMissingAndUnknown(t *testing.T) {
	// A stale document is refused outright: its port may name a dead process, and F-18's whole
	// point is that this is what makes an agent retry into a closed port.
	stPath := writeStatus(t, freshStatus(3099))
	c := New(stPath, time.Second)
	if _, err := c.Resolve(context.Background(), "dummy"); err != nil {
		t.Fatalf("a fresh document must resolve: %v", err)
	}

	// Age the document by more than 3x its own reconcile interval (9 s) and check the error
	// names BOTH the age and the bound.
	stale := freshStatus(3099)
	stale.GeneratedAt = time.Now().Add(-30 * time.Second).Format(time.RFC3339Nano)
	c.StatusPath = writeStatus(t, stale)
	_, err := c.Resolve(context.Background(), "dummy")
	if err == nil {
		t.Fatal("a stale document resolved")
	}
	msg := err.Error()
	if !strings.Contains(msg, "stale") && !strings.Contains(msg, "UNKNOWN") {
		t.Errorf("stale error does not say the document is stale/UNKNOWN: %s", msg)
	}
	if !strings.Contains(msg, "9s") || !strings.Contains(msg, "30s") {
		t.Errorf("stale error must name the bound and the age: %s", msg)
	}

	c.StatusPath = filepath.Join(t.TempDir(), "absent", "fleet-status.json")
	if _, err := c.Resolve(context.Background(), "dummy"); err == nil || !strings.Contains(err.Error(), "fleet-status.json") {
		t.Errorf("missing document: %v", err)
	}

	c.StatusPath = ""
	if _, err := c.Resolve(context.Background(), "dummy"); err == nil {
		t.Error("an unconfigured status path resolved something")
	}

	c.StatusPath = writeStatus(t, freshStatus(3099))
	if _, err := c.Resolve(context.Background(), "no-such-site"); err == nil {
		t.Error("unknown site resolved")
	}
}

// TestCallResolvesBeforeNetworking: a refused resolution never reaches the socket, which is how
// "the site is down" stays distinguishable from "this tool will not ask".
func TestCallRefusedSiteNeverDials(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	c := New(serverStatusPath(t, srv), time.Second)

	if _, err := c.Call(context.Background(), "ghost", "doctor", nil); err == nil {
		t.Fatal("a site absent from the document was called")
	}
	if hits != 0 {
		t.Errorf("%d requests reached the listener for an unresolvable site", hits)
	}
}

func TestNewOwnsItsHTTPClient(t *testing.T) {
	c := New("/tmp/x.json", 0)
	if c.Timeout != defaultTimeout {
		t.Errorf("default timeout = %s, want %s", c.Timeout, defaultTimeout)
	}
	if c.HTTP == nil || c.HTTP == http.DefaultClient {
		t.Fatal("New shares http.DefaultClient (plan 2 A-20)")
	}
	if tr, ok := http.DefaultTransport.(*http.Transport); ok && c.HTTP.Transport == tr {
		t.Error("New shares the process-wide transport pool")
	}
	if d := 3 * time.Second; New("/tmp/x.json", d).Timeout != d {
		t.Error("an explicit timeout was ignored")
	}
	// A hand-built Client must still work: neither field is allowed to be assumed.
	hand := &Client{}
	if hand.timeout() != defaultTimeout || hand.httpClient() == nil {
		t.Error("a zero Client has no usable defaults")
	}
}

func TestUnmarshalReportsTargetType(t *testing.T) {
	var out struct {
		OK bool `json:"ok"`
	}
	if err := Unmarshal(json.RawMessage(`{"ok":true}`), &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !out.OK {
		t.Error("body not decoded")
	}
	err := Unmarshal(json.RawMessage(`<html>`), &out)
	if err == nil || !strings.Contains(err.Error(), "struct {") {
		t.Errorf("decode error does not name the target: %v", err)
	}
}
