// Package youtube drives the YouTube web surfaces — the Studio upload wizard, a channel's content
// list, the details editor, a watch page — through the firefox-bridge, the same way the terminal
// operator does: navigate, probe, click, verify.
//
// There is no second door into YouTube here. The tool's trust property is that everything observable
// about a video was learned by *visiting* a page, so this package must not gain an http.Client, a
// youtubei call, or a cookie-derived Authorization header. The guard against that is
// TestYoutubePackageHasNoHTTPClient.
//
// Layout, and the plan's §4.1 layering:
//
//	page.go / locator.go / finding.go   the Page primitive and its verified find
//	errors.go                           transport error classification
//	journal.go                          the one-shot slot, written before the mutation
//	diagnose.go                         bounded diagnostics
//	pages/                              the flows: Page calls only, no orchestration, no json.Marshal
//	service.go upload.go                the seams that make a flow runnable and testable
package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Browser is the whole dependency on the bridge: one action call, one JSON reply. Injected, so a
// flow test can script replies without a browser (plan §4.4).
type Browser interface {
	Call(ctx context.Context, action string, params map[string]any) (map[string]any, error)
}

// Tab identifies a browser tab plus the identity it actually opened in. Carrying containerId is what
// makes identity verification cheap: R1 is asserted once at Open and inherited by every later call.
type Tab struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	ContainerID int    `json:"containerId"`
	Status      int    `json:"status"`
}

// Page is the only browser surface a flow may use. Every method (a) addresses the tab explicitly by
// id, so a focus change cannot silently retarget the flow, and (b) records what it did into the Diag
// buffer.
type Page struct {
	br   Browser
	tab  Tab
	diag *Diag

	// wantContainerID, when non-zero, is asserted against the tab the navigate reply reports.
	wantContainerID int
	// wantChannel, when set, must appear in the URL of any Studio page we act on.
	wantChannel string

	actionTimeout time.Duration
}

func NewPage(br Browser, tab Tab, d *Diag) *Page {
	return &Page{br: br, tab: tab, diag: d, actionTimeout: 45 * time.Second}
}

func (p *Page) Tab() Tab         { return p.tab }
func (p *Page) Diag() *Diag      { return p.diag }
func (p *Page) Browser() Browser { return p.br }

// RequireIdentity pins the assertions Open must satisfy. It is called from config, never from a flow.
func (p *Page) RequireIdentity(containerID int, channelID string) {
	p.wantContainerID = containerID
	p.wantChannel = channelID
}

// Open navigates the *current* tab (R7: a flow that needs its own tab creates it with NewTab first),
// in a background tab by default so the human's focus is never stolen.
func (p *Page) Open(ctx context.Context, rawURL string) error {
	params := map[string]any{"url": rawURL, "back_tab": true, "timeout": p.timeoutMS()}
	if p.tab.ID != "" {
		params["index"] = p.tab.ID // re-navigating the tab we already own
		delete(params, "back_tab")
	}
	if c := p.diag.Container(); c != "" {
		params["container"] = c
	}
	reply, err := p.br.Call(ctx, "navigate", params)
	if err != nil {
		return p.fail("navigate", rawURL, err)
	}
	if bad := bridgeError(reply); bad != "" {
		return p.fail("navigate", rawURL, fmt.Errorf("%s", bad))
	}
	t := parseTab(reply)
	if t.ID == "" {
		return p.fail("navigate", rawURL, fmt.Errorf("navigate returned no tab id"))
	}
	p.tab = t
	p.diag.Note("navigate", rawURL, fmt.Sprintf("tab=%s containerId=%d status=%d", t.ID, t.ContainerID, t.Status))

	// R1 + R2 in one assertion: the identity is the one config asked for, and the tab we will act on
	// is the one the bridge just reported.
	if p.wantContainerID != 0 && t.ContainerID != p.wantContainerID {
		return ErrIdentityMismatch{Want: fmt.Sprintf("containerId %d", p.wantContainerID), Got: fmt.Sprintf("containerId %d on tab %s", t.ContainerID, t.ID)}
	}
	if p.wantChannel != "" && !strings.Contains(t.URL, p.wantChannel) {
		return ErrIdentityMismatch{Want: "channel " + p.wantChannel, Got: "landed on " + t.URL}
	}
	return nil
}

// NewTab opens a separate tab for an agent action, inheriting the container (R7) rather than reusing
// whatever tab the agent or a human happens to have focused.
func (p *Page) NewTab(ctx context.Context, rawURL string) (*Page, error) {
	params := map[string]any{"url": rawURL, "new_tab": true, "timeout": p.timeoutMS()}
	if c := p.diag.Container(); c != "" {
		params["container"] = c
	}
	reply, err := p.br.Call(ctx, "navigate", params)
	if err != nil {
		return nil, p.fail("navigate", rawURL, err)
	}
	if bad := bridgeError(reply); bad != "" {
		return nil, p.fail("navigate", rawURL, fmt.Errorf("%s", bad))
	}
	t := parseTab(reply)
	if t.ID == "" {
		return nil, p.fail("navigate", rawURL, fmt.Errorf("navigate returned no tab id"))
	}
	if p.wantContainerID != 0 && t.ContainerID != p.wantContainerID {
		return nil, ErrIdentityMismatch{Want: fmt.Sprintf("containerId %d", p.wantContainerID), Got: fmt.Sprintf("containerId %d on tab %s", t.ContainerID, t.ID)}
	}
	child := NewPage(p.br, t, p.diag)
	child.wantContainerID, child.wantChannel, child.actionTimeout = p.wantContainerID, p.wantChannel, p.actionTimeout
	p.diag.Note("new_tab", rawURL, "tab="+t.ID)
	return child, nil
}

// CloseTab closes this flow's tab. Safe on a background tab; the identity comes from the container.
func (p *Page) CloseTab(ctx context.Context) error {
	if p.tab.ID == "" {
		return nil
	}
	_, err := p.br.Call(ctx, "close_tab", map[string]any{"index": p.tab.ID})
	p.diag.Note("close_tab", p.tab.ID, errString(err))
	return err
}

// Eval runs program in the page and returns its value. program is a bare body — it may start with any
// statement, including `return`; Compose wraps it in the prelude that defines `args` for the embedded
// scripts' benefit.
func (p *Page) Eval(ctx context.Context, program string, args any) (json.RawMessage, error) {
	// The bridge unwraps a successful reply, so the value lives at the top level; a failure keeps the
	// {success:false, error:…} envelope. payload() reconciles the two shapes for every call site.
	reply, err := p.br.Call(ctx, "web_eval", map[string]any{
		"index": p.tab.ID,
		"code":  Compose(program, args),
	})
	if err != nil {
		return nil, p.fail("web_eval", trim(program), err)
	}
	if bad := bridgeError(reply); bad != "" {
		return nil, p.fail("web_eval", trim(program), fmt.Errorf("%s", bad))
	}
	raw, err := json.Marshal(pick(payload(reply), "result"))
	if err != nil {
		return nil, p.fail("web_eval", trim(program), err)
	}
	p.diag.Note("web_eval", trim(program), "ok")
	return raw, nil
}

// Run executes an embedded page program with args and decodes its return value into out. This is the
// only way a flow reaches the page with its own logic: the program is a linted .js file from
// internal/youtube/scripts, wrapped here in the prelude that defines `args`.
func (p *Page) Run(ctx context.Context, name string, args map[string]any, out any) error {
	raw, err := p.Eval(ctx, Script(name), args)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s reply: %w", name, err)
	}
	return nil
}

// EvalInto unmarshals an Eval result into out.
func (p *Page) EvalInto(ctx context.Context, program string, args any, out any) error {
	raw, err := p.Eval(ctx, program, args)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// BodyText is the cheap verification channel: a phrase the page must contain for a step to have
// really happened ("All changes saved", "No issues found", "is scheduled").
func (p *Page) BodyText(ctx context.Context) (string, error) {
	var out struct {
		URL  string `json:"url"`
		Text string `json:"text"`
	}
	if err := p.Run(ctx, "body_text.js", map[string]any{"max": p.diag.MaxDiagBytes()}, &out); err != nil {
		return "", err
	}
	return out.Text, nil
}

// URL reports the tab's current URL from the page's point of view.
func (p *Page) URL(ctx context.Context) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	if err := p.Run(ctx, "body_text.js", map[string]any{"max": 256}, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// Screenshot is evidence: "succeeded" is a string, an outcome is a file (plan §6). Relative paths are
// rejected because the bridge silently writes nothing when given one.
func (p *Page) Screenshot(ctx context.Context, path string) error {
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("screenshot path must be absolute, got %q", path)
	}
	_, err := p.br.Call(ctx, "screenshot", map[string]any{"index": p.tab.ID, "path": path})
	p.diag.Shot(path, err)
	return err
}

// Scroll scrolls the document (or one element) by real wheel events; the bridge does not compute
// window.scrollTo(y) strings, so a numeric y is used, never a script.
func (p *Page) Scroll(ctx context.Context, l Locator, y float64, block string) error {
	params := map[string]any{"index": p.tab.ID}
	params = p.locatorParams(l, params)
	params["y"] = y
	if block != "" {
		params["block"] = block
	}
	_, err := p.br.Call(ctx, "scroll", params)
	p.diag.Note("scroll", l.String(), errString(err))
	if err != nil {
		return Classify(err.Error())
	}
	return nil
}

// Attach is the one-shot file attach. It deliberately omits triggerDrop: with both the change event and
// the drop, Studio attaches the file twice.
func (p *Page) Attach(ctx context.Context, l Locator, path string) (map[string]any, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("attach path must be absolute, got %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("attach %s: %w", path, err)
	}
	params := p.locatorParams(l, map[string]any{"index": p.tab.ID, "path": path})
	reply, err := p.br.Call(ctx, "upload", params)
	if err != nil {
		return nil, p.fail("upload", path, err)
	}
	if bad := bridgeError(reply); bad != "" {
		return nil, p.fail("upload", path, fmt.Errorf("%s", bad))
	}
	data := payload(reply)
	p.diag.Note("upload", filepath.Base(path), fmt.Sprintf("%v", data["success"]))
	if data != nil {
		if ok, _ := data["success"].(bool); ok {
			return data, nil
		}
		return data, fmt.Errorf("%w: attach reported no success", ErrWait)
	}
	return data, fmt.Errorf("%w: empty attach reply", ErrWait)
}

// WaitAny polls until the page shows any of the phrases, and reports which one. This is how a transfer
// or a processing state is confirmed: several wordings, one observable.
func (p *Page) WaitAny(ctx context.Context, phrases []string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		text, err := p.BodyText(ctx)
		if err == nil {
			last = text
			for _, want := range phrases {
				if strings.Contains(text, want) {
					return want, nil
				}
			}
		}
		if time.Now().After(deadline) {
			return "", ErrTimeout{What: "page to show any of " + strings.Join(phrases, " | "), After: timeout.String(), Last: truncate(last, 160)}
		}
		select {
		case <-ctx.Done():
			return "", ErrAbandoned{Detail: ctx.Err().Error()}
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

// Poll repeats check until it is satisfied or the budget runs out. A flow expresses "the page came to
// look like this" instead of sleeping, which is what R6 requires of every long wait.
func (p *Page) Poll(ctx context.Context, every, timeout time.Duration, check func(context.Context) (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		ok, err := check(ctx)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrTimeout{What: "the page to reach the expected state", After: timeout.String()}
		}
		select {
		case <-ctx.Done():
			return ErrAbandoned{Detail: ctx.Err().Error()}
		case <-time.After(every):
		}
	}
}

func (p *Page) timeoutMS() int { return int(p.actionTimeout / time.Millisecond) }

func (p *Page) fail(action, subj string, err error) error {
	c := Classify(err.Error())
	p.diag.Note(action, subj, c.Error())
	return c
}

func pick(m map[string]any, k string) any {
	if m == nil {
		return nil
	}
	if v, ok := m[k]; ok {
		return v
	}
	return m
}

// bridgeError extracts the bridge's own failure text. A reply with success:false is *not* an action
// error, so it must never reach Classify as one — it is the probe's answer, and the caller decides
// what it means.
func bridgeError(reply map[string]any) string {
	if reply == nil {
		return "empty bridge reply"
	}
	if ok, has := reply["success"].(bool); has && !ok {
		if e, _ := reply["error"].(string); strings.TrimSpace(e) != "" {
			return e
		}
		return "bridge reported failure with no error text"
	}
	if e, _ := reply["error"].(string); strings.TrimSpace(e) != "" && e != "null" {
		// An error string alongside success:true is the probe's own throw, e.g. a SyntaxError.
		return e
	}
	return ""
}

// payload returns the body of a bridge reply. CallBridge hands back the unwrapped payload on success and
// the whole envelope on failure, and the two have different shapes; every read in this package goes through
// here so neither shape can be missed.
func payload(reply map[string]any) map[string]any {
	if reply == nil {
		return nil
	}
	if data, ok := reply["data"].(map[string]any); ok && data != nil {
		return data
	}
	if raw, ok := reply["data"]; ok && raw == nil {
		// An explicit data:null means the payload really was empty (a screenshot reply), not unwrapped.
		return map[string]any{}
	}
	return reply
}

func parseTab(reply map[string]any) Tab {
	data := payload(reply)
	if data == nil {
		return Tab{}
	}
	t := Tab{}
	t.ID, _ = data["id"].(string)
	t.URL, _ = data["url"].(string)
	t.Title, _ = data["title"].(string)
	switch v := data["containerId"].(type) {
	case float64:
		t.ContainerID = int(v)
	case int:
		t.ContainerID = v
	}
	switch v := data["status"].(type) {
	case float64:
		t.Status = int(v)
	case int:
		t.Status = v
	}
	return t
}

func errString(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

func trim(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 70 {
		return s[:67] + "..."
	}
	return s
}

// evalScript is the internal spelling of Run.
func (p *Page) evalScript(ctx context.Context, name string, args map[string]any, out any) error {
	return p.Run(ctx, name, args, out)
}
