package youtube

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Find resolves a Locator and returns every address for it (R4): the CSS path the deep walk proved, the
// state that distinguishes "not yet" from "gone", and — merged from the bridge's own locator resolution
// — the AX/native channels. A locator that only resolves one of those ways is visible in the record,
// which is how a brittle selector gets caught before it reaches a flow.
func (p *Page) Find(ctx context.Context, l Locator) (Finding, error) {
	var f Finding
	if err := p.evalScript(ctx, "lookup.js", map[string]any{"l": l, "maxText": p.diag.MaxDiagBytes()}, &f); err != nil {
		return f, err
	}
	if f.OK {
		f.AX = p.probeBridgeLocator(ctx, l)
	}
	return f, nil
}

// probeBridgeLocator asks the bridge itself to resolve the same locator. `get_text` is read-only and its
// reply carries the element identity the bridge picked plus any native fallback it resorted to — the
// second and third channels of R4, free of charge.
func (p *Page) probeBridgeLocator(ctx context.Context, l Locator) map[string]any {
	reply, err := p.br.Call(ctx, "get_text", p.locatorParams(l, map[string]any{"index": p.tab.ID}))
	if err != nil || reply == nil {
		return nil
	}
	data, _ := reply["data"].(map[string]any)
	if data == nil {
		return nil
	}
	out := map[string]any{}
	if el, ok := data["el"].(map[string]any); ok {
		out["el"] = el
	}
	for _, k := range []string{"trigger_native_copy", "native", "text"} {
		if v, ok := data[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// locatorParams maps a Locator onto the bridge's own locator params, which are documented as
// alternative and cumulative. An explicit `name` is never overwritten by a `by_name` alias.
func (p *Page) locatorParams(l Locator, params map[string]any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	if l.CSS != "" {
		params["selector"] = l.CSS
		if l.Root != "" {
			params["selector"] = l.Root + " " + l.CSS
		}
	}
	if l.ByText != "" {
		params["by_text"] = l.ByText
	}
	if l.ByRole != "" {
		params["by_role"] = l.ByRole
	}
	if l.ByName != "" {
		if l.ByRole == "" {
			params["name"] = l.ByName
		} else {
			params["by_name"] = l.ByName
		}
	}
	if l.X != 0 || l.Y != 0 {
		params["x"] = l.X
		params["y"] = l.Y
	}
	return params
}

// Click presses one element and *proves the effect*: after the bridge click, the page must come to
// contain wantText (R5). Without that wait the same interaction can report success twice, once before
// the page has caught up, and a blind retry is exactly what duplicates a comment — so a click that does
// not verify is not attempted twice.
func (p *Page) Click(ctx context.Context, l Locator, wantText string, timeout time.Duration) (Finding, error) {
	f, err := p.Find(ctx, l)
	if err != nil {
		return f, err
	}
	if !f.OK {
		return f, ErrNotFound{Locator: l.String()}
	}
	reply, err := p.br.Call(ctx, "click", p.locatorParams(l, map[string]any{"index": p.tab.ID, "timeout": p.timeoutMS()}))
	press := "bridge"
	if bad := bridgeError(reply); bad != "" || err != nil {
		raw := bad
		if err != nil {
			raw = err.Error()
		}
		// The bridge resolves locators itself, and for a control that only the page-side deep walk can see
		// (a Polymer-scoped radio group, a virtual list cell) it reports not-found. That press was a no-op,
		// so falling through to the page channel cannot double-press: R5's order is cheap first, then DOM.
		if cErr := Classify(raw); errors.Is(cErr, ErrElement) || strings.Contains(strings.ToLower(raw), "not found") {
			if _, derr := p.DomClick(ctx, l); derr == nil {
				press = "dom"
			} else {
				return f, p.fail("click", l.String(), fmt.Errorf("the bridge could not press %s (%s) and the page channel failed too: %w", l, raw, derr))
			}
		} else {
			return f, p.fail("click", l.String(), fmt.Errorf("%s", raw))
		}
	} else if data := payload(reply); data != nil {
		if el, ok := data["el"].(map[string]any); ok {
			f.Native = el
		}
	}
	f.ClickMethod = press
	p.diag.Note("click", l.String(), f.String()+" via "+press)

	if wantText == "" {
		return f, nil
	}
	if err := p.WaitText(ctx, wantText, timeout); err != nil {
		return f, fmt.Errorf("clicked %s but the page never showed %q: %w", l.String(), wantText, err)
	}
	return f, nil
}

// DomClick presses an element by calling .click() on the page-proven path. This is the channel that
// works where a synthetic MouseEvent is swallowed and where the element has no stable selector at all —
// a virtual scrolling calendar cell — and it needs no OS focus, so it never disturbs the human.
func (p *Page) DomClick(ctx context.Context, l Locator) (Finding, error) {
	f, err := p.Find(ctx, l)
	if err != nil {
		return f, err
	}
	if !f.OK {
		return f, ErrNotFound{Locator: l.String()}
	}
	var out struct {
		Clicked bool   `json:"clicked"`
		Text    string `json:"text"`
	}
	if err := p.evalScript(ctx, "click.js", map[string]any{"l": l}, &out); err != nil {
		return f, err
	}
	if !out.Clicked {
		return f, ErrNotFound{Locator: l.String() + " (at click time)"}
	}
	f.ClickMethod = "dom"
	f.Text = out.Text
	p.diag.Note("dom_click", l.String(), f.String())
	return f, nil
}

// Type enters text into a real input or a contenteditable, then reads it back (R5).
//
// The bridge's own `type` action was observed to focus a Studio contenteditable and write nothing, so
// text entry goes through the page: replace the selection and `insertText`, which is the one sequence a
// rich editor's own model accepts. The read-back is what makes the call honest — a silent no-throw is
// reported as a failure, not as success.
func (p *Page) Type(ctx context.Context, l Locator, text string) (Finding, string, error) {
	f, err := p.Find(ctx, l)
	if err != nil {
		return f, "", err
	}
	if !f.OK {
		return f, "", ErrNotFound{Locator: l.String()}
	}
	var out struct {
		OK     bool   `json:"ok"`
		Method string `json:"method"`
		Text   string `json:"text"`
		Err    string `json:"err"`
	}
	if err := p.evalScript(ctx, "fill.js", map[string]any{"l": l, "text": text}, &out); err != nil {
		return f, "", err
	}
	if !out.OK {
		return f, out.Text, fmt.Errorf("typing into %s did not land (%s, method %s)", l.String(), out.Err, out.Method)
	}
	p.diag.Note("type", l.String(), fmt.Sprintf("%s -> %q", out.Method, truncate(out.Text, 40)))
	return f, out.Text, nil
}

// WaitText polls until the page's visible text contains phrase (R6). It waits on the *observable*, not
// on a fixed sleep.
func (p *Page) WaitText(ctx context.Context, phrase string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		if err := ctx.Err(); err != nil {
			return ErrAbandoned{Detail: err.Error()}
		}
		text, err := p.BodyText(ctx)
		if err == nil {
			last = text
			if strings.Contains(text, phrase) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return ErrTimeout{What: "page to contain " + quote(phrase), After: timeout.String(), Last: truncate(last, 160)}
		}
		select {
		case <-ctx.Done():
			return ErrAbandoned{Detail: ctx.Err().Error()}
		case <-time.After(750 * time.Millisecond):
		}
	}
}

// WaitState polls until the locator resolves and its state satisfies ok — the "did the panel actually
// change" check, which lives on tab state and not on the step headers.
func (p *Page) WaitState(ctx context.Context, l Locator, timeout time.Duration, ok func(Finding) bool) (Finding, error) {
	deadline := time.Now().Add(timeout)
	for {
		f, err := p.Find(ctx, l)
		if err == nil && ok(f) {
			return f, nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return f, ErrTimeout{What: l.String(), After: timeout.String(), Last: err.Error()}
			}
			return f, ErrTimeout{What: l.String(), After: timeout.String(), Last: f.String()}
		}
		select {
		case <-ctx.Done():
			return f, ErrAbandoned{Detail: ctx.Err().Error()}
		case <-time.After(600 * time.Millisecond):
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
