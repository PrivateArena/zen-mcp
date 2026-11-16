package webserver

// The client half of the integration: one site's loopback admin API (plan 2 §5.1).
//
// Two facts shape everything below, and both come from the engine rather than from taste.
//
// Authentication: NONE. A loopback peer is admitted to the whole admin surface without a
// credential — the tracked trade-off plan 1 §8 F-22 records, and still true after plan 2's
// step 13 credential work, because a credential is what an OFF-box or agent-header peer has
// to present. Anything that is not loopback gets a 404, not a 403 (admin_net.go's
// adminAllowedFrom runs before the credential gate), so "403 + missing cookie" is not a state
// this client can ever observe. That is why there is no cookie jar here: sending the owner
// cookie from a loopback request would add a secret to a request that does not need one and
// would make the future credentials step a change to every call site instead of one. When
// credentials arrive they belong to the actor, not to this transport.
//
// Errors are shaped: every admin failure answers with an AdminError body (pkg/admin_api/
// errors.go) carrying a stable machine-readable Code. A 404 from this surface therefore means
// "this build does not serve that endpoint", not "the site is down" — plan 2 A-15's point
// that an agent must not retry into a wall. The code string is what callers branch on, so it
// survives decoding as a typed error rather than being flattened into a message.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// apiPrefix is the engine's admin mount point. The idiom is the engine's own
// (zen-mcp internal/whiteboard/client.go: apiURL = base + "/_zen/api" + "/suffix"); one
// string here keeps every endpoint assembly honest.
const apiPrefix = "/_zen/api"

// defaultTimeout is the per-call budget when a caller does not choose one. It matches the
// bridge's own default so a hung site is reported as a timeout rather than as a hang.
const defaultTimeout = 10 * time.Second

// maxReplyBytes bounds one response body. An admin reply is a JSON document about a site,
// not a file transfer; a body past this is a bug or a hostile listener, and reading it is how
// a tool runs out of memory.
const maxReplyBytes = 8 << 20

// Error codes the admin surface answers with (pkg/admin_api/errors.go, plus the handler-local
// read_only_query_rejected). Stable on the engine's side: skills and the generated CLI match
// on these strings, so they are spelled out here rather than inlined at a call site.
const (
	ErrUnknownEndpoint       = "unknown_admin_endpoint"
	ErrMethodNotAllowed      = "method_not_allowed"
	ErrActorNotPermitted     = "actor_not_permitted"
	ErrAdminCPDisabled       = "admincp_disabled"
	ErrInvalidRequest        = "invalid_request"
	ErrNotPermitted          = "not_permitted"
	ErrReadOnlyQueryRejected = "read_only_query_rejected"
)

// APIError is one failed admin call.
//
// It mirrors AdminError's JSON body, and additionally carries the raw body: some engine
// handlers still answer with a plain http.Error (a text/plain line, no code), and a client
// that assumed every reply is admin-shaped would report that real failure as an empty one.
type APIError struct {
	Status  int    `json:"status"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Path    string `json:"path,omitempty"`
	// Body is the undecoded reply, kept whenever it is not admin JSON (Code is then "").
	Body string `json:"-"`
}

func (e *APIError) Error() string {
	switch {
	case e.Code != "" && e.Path != "":
		return fmt.Sprintf("webserver: admin %d %s at %s: %s", e.Status, e.Code, e.Path, e.Message)
	case e.Code != "":
		return fmt.Sprintf("webserver: admin %d %s: %s", e.Status, e.Code, e.Message)
	case e.Body != "":
		return fmt.Sprintf("webserver: admin %d: %s", e.Status, e.Body)
	default:
		return fmt.Sprintf("webserver: admin %d with no error body", e.Status)
	}
}

// CodeOf returns the admin error code carried anywhere in err's chain, or "" when err is not
// an APIError (or is one decoded from a non-JSON body). This is the branch point callers want:
// `if CodeOf(err) == ErrUnknownEndpoint { do not retry }`.
func CodeOf(err error) string {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

// IsNotImplemented reports the one 404 that must never be retried: the endpoint does not
// exist in this build of the engine (A-15). A site that is merely down answers with a
// connection error, not with this code.
func IsNotImplemented(err error) bool { return CodeOf(err) == ErrUnknownEndpoint }

// Client calls one fleet's site-admin APIs.
//
// HTTP is deliberately its OWN client, never http.DefaultClient and never the bridge's: the
// bridge's 10 s client is shared by every browser tool, and plan 2 A-20's rule that a site in
// a backoff must not consume it applies here too — a dead port must cost this client a
// timeout, nothing more. The transport is cloned for the same reason (a pooled connection to
// a hung site must not sit in the process-wide pool).
type Client struct {
	// StatusPath is the fleet status document this client resolves site ids against — the
	// only port source in this repo (plan 2 A-18).
	StatusPath string
	HTTP       *http.Client
	Timeout    time.Duration
}

// New returns a client for one fleet. A non-positive timeout means the 10 s default, which is
// longer than any admin handler's normal work and shorter than the reconciler's patience.
func New(statusPath string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	tr, ok := http.DefaultTransport.(*http.Transport)
	var transport http.RoundTripper = http.DefaultTransport
	if ok {
		transport = tr.Clone()
	}
	return &Client{
		StatusPath: statusPath,
		HTTP:       &http.Client{Timeout: timeout, Transport: transport},
		Timeout:    timeout,
	}
}

// timeout is the per-request budget, tolerating a hand-built Client with no Timeout set.
func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return defaultTimeout
}

// httpClient is the same tolerance applied to the transport.
func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: c.timeout()}
}

// Resolve turns a site id into the loopback base URL its admin API lives on, or refuses.
//
// Both refusals are load-bearing. A STALE document is refused rather than used (plan 1 F-18):
// after a crash the file survives untouched, so its port may belong to a process that no
// longer exists, which is exactly the condition that makes an agent retry into a dead port. A
// NON-LOOPBACK host is refused because the engine 404s every off-box peer — building a URL for
// one would turn a clear "not reachable this way" into a confusing 404.
func (c *Client) Resolve(ctx context.Context, siteID string) (string, error) {
	if c.StatusPath == "" {
		return "", errors.New("webserver: no fleet status document configured (Client.StatusPath is empty)")
	}
	st, err := Read(c.StatusPath)
	if err != nil {
		return "", err
	}
	// The staleness bound is computed from the DOCUMENT's own reconcile interval, never from a
	// constant here: a fleet retuned to a 10 s pass is not stale at 25 s.
	if _, err := st.StaleFor(time.Now()); err != nil {
		return "", fmt.Errorf("webserver: cannot resolve site %q from %s: %w", siteID, c.StatusPath, err)
	}
	site, err := ResolveSite(st, siteID)
	if err != nil {
		return "", err
	}
	return LoopbackBase(loopbackHost, site.Port)
}

// loopbackHost is the only host this client will ever build a URL for.
const loopbackHost = "127.0.0.1"

// LoopbackBase assembles http://<host>:<port> and refuses anything that is not a loopback
// listener. It is exported because the check is the interesting part: a caller that learned a
// host somewhere else (a config, a status field added later) reaches the same gate as Resolve.
func LoopbackBase(host string, port int) (string, error) {
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("webserver: site port %d is not usable", port)
	}
	h := strings.TrimSpace(host)
	if h == "" {
		h = loopbackHost
	}
	// A literal loopback address, not "localhost": the latter is a name, and a hosts-file or
	// resolver change would turn the loopback guarantee into a remote call.
	if ip := net.ParseIP(h); ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("webserver: refusing to call %s:%d: only a literal loopback address is admitted, and a non-loopback peer gets a 404 rather than a 403", h, port)
	}
	return "http://" + h + ":" + strconv.Itoa(port), nil
}

// apiURL is the engine's own assembly: base + /_zen/api + /suffix. The suffix is normalised so
// "doctor", "/doctor" and "whiteboard/state" all land on the documented path.
func apiURL(base, suffix string) string {
	suffix = strings.TrimPrefix(strings.TrimSpace(suffix), "/")
	return strings.TrimRight(base, "/") + apiPrefix + "/" + suffix
}

// Call performs one admin request against one site and returns the raw success body.
//
// body == nil means GET — the read endpoints (api/list, api/get, api/doctor, api/catalog)
// answer it, and the engine reads r.URL.Query() before the body, so the query string belongs
// in the suffix (see Doctor). A non-nil body means POST with a JSON body, which is how every
// mutating endpoint is spelled. No cookie is attached; see the package comment for why.
func (c *Client) Call(ctx context.Context, siteID, suffix string, body any) (json.RawMessage, error) {
	base, err := c.Resolve(ctx, siteID)
	if err != nil {
		return nil, err
	}
	return c.CallBase(ctx, base, suffix, body)
}

// CallBase is Call with the base URL supplied, for a caller that already resolved a site (or
// is testing against a loopback listener). The URL assembly, the method choice and the error
// decoding are the same code, which is the point: no second implementation to drift.
func (c *Client) CallBase(ctx context.Context, base, suffix string, body any) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.StatusPath == "" && base == "" {
		return nil, errors.New("webserver: no base URL and no fleet status document")
	}
	u := apiURL(base, suffix)

	perCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	var (
		method   = http.MethodGet
		reader   io.Reader
		sendBody = body != nil
	)
	if sendBody {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("webserver: encode request for %s: %w", u, err)
		}
		method, reader = http.MethodPost, bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(perCtx, method, u, reader)
	if err != nil {
		return nil, fmt.Errorf("webserver: build %s %s: %w", method, u, err)
	}
	if sendBody {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		// A transport failure is a distinct outcome from an admin-shaped 5xx: it means the
		// listener never answered, which is what a site in backoff looks like from the outside.
		return nil, fmt.Errorf("webserver: %s %s: %w", method, u, err)
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxReplyBytes))
	if readErr != nil {
		return nil, fmt.Errorf("webserver: read reply from %s: %w", u, readErr)
	}
	if resp.StatusCode >= 400 {
		return nil, decodeAdminError(resp.StatusCode, u, raw)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		// An empty 2xx is the F-20/A-1 bug this surface exists to close; reported rather than
		// handed back as a successful zero-length document an agent reads as "done".
		return nil, fmt.Errorf("webserver: %s %s answered %d with an empty body", method, u, resp.StatusCode)
	}
	return json.RawMessage(raw), nil
}

// decodeAdminError turns a failure body into an APIError, tolerating the handlers that still
// use plain http.Error (text/plain) by keeping the body and leaving Code empty.
func decodeAdminError(status int, u string, raw []byte) error {
	var ae APIError
	if err := json.Unmarshal(raw, &ae); err == nil && (ae.Code != "" || ae.Message != "") {
		if ae.Status == 0 {
			ae.Status = status
		}
		if ae.Path == "" {
			if parsed, perr := url.Parse(u); perr == nil {
				ae.Path = parsed.Path
			}
		}
		return &ae
	}
	body := strings.TrimSpace(string(raw))
	if body == "" {
		body = http.StatusText(status)
	}
	return &APIError{Status: status, Message: body, Body: body}
}

// Doctor calls GET api/doctor?deep=<true|false>, the engine's in-process health report and the
// verify gate of plan 2 §5.1. The query form is what the handler reads (r.URL.Query().Get
// ("deep")), so the parameter goes in the URL and the body stays nil.
func (c *Client) Doctor(ctx context.Context, siteID string, deep bool) (json.RawMessage, error) {
	return c.Call(ctx, siteID, "doctor"+query(map[string]string{"deep": strconv.FormatBool(deep)}), nil)
}

// Catalog calls GET api/catalog: the site's self-description (endpoints, config grammar,
// template vars). It is the answer to "what can I do to this site" without a hardcoded list.
func (c *Client) Catalog(ctx context.Context, siteID string) (json.RawMessage, error) {
	return c.Call(ctx, siteID, "catalog", nil)
}

// List calls GET api/list with the caller's pagination/sort parameters.
func (c *Client) List(ctx context.Context, siteID string, params url.Values) (json.RawMessage, error) {
	return c.Call(ctx, siteID, "list"+encodeValues(params), nil)
}

// Get calls GET api/get?slug=<slug>.
func (c *Client) Get(ctx context.Context, siteID, slug string) (json.RawMessage, error) {
	return c.Call(ctx, siteID, "get"+query(map[string]string{"slug": slug}), nil)
}

// query builds a URL query string from literal pairs (the leading "?" is included when any
// value is non-empty).
func query(pairs map[string]string) string {
	v := url.Values{}
	for k, val := range pairs {
		if val != "" {
			v.Set(k, val)
		}
	}
	return encodeValues(v)
}

func encodeValues(v url.Values) string {
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

// Unmarshal decodes one Call's body into a target, in the package so callers do not each
// re-invent "the reply is JSON bytes or an error".
func Unmarshal(raw json.RawMessage, into any) error {
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("webserver: decode admin reply as %T: %w", into, err)
	}
	return nil
}
