package youtube

import (
	"encoding/json"
	"strings"
)

// Compose wraps a page program in the same prelude the existing in-repo JS payloads use, so an
// embedded script can read its payload as `args`. Prelude text is never re-typed here — the shape of
// `(async()=>{...})()` is exactly what the bridge expects, and a program whose first token is not
// `return`/`var`/`let`/`const` would be invalid if the prologue were hand-written wrong (which is why
// TestComposeKeepsTheProgramCallable exists).
func Compose(program string, args any) string {
	enc, _ := json.Marshal(args)
	if strings.TrimSpace(string(enc)) == "null" {
		enc = []byte("{}")
	}
	// A statement list, never an IIFE. The bridge wraps the program in its own async function and returns what
	// that function returns, so a `return` nested inside (async()=>{…})() is invisible to it: the call comes
	// back success with no result at all, and the caller decodes a zero Finding that reads "no such element"
	// about an element the page is showing. Measured 2026-11-04: the same script and args returned nothing
	// wrapped and {ok:true,count:1} unwrapped. Keeping `args` as a leading let also satisfies the bridge's rule
	// that the first token is a statement keyword, which is why every program here starts with a comment.
	return "let args = " + string(enc) + ";\n" + program
}

// Locator is one page element addressed five ways at once — R4. A flow names what it wants; the
// resolver decides how to find it, in the order the plan fixes (selector → AX → text → point).
type Locator struct {
	// CSS is an ordinary selector, resolved through the deep walk below (R4), so it reaches inside a
	// closed shadow root. This is a *page-proven* path, never a guessed CSS-in-JS hash.
	CSS string `json:"css,omitempty"`
	// Root scopes the search to a subtree, which is how a duplicated label ("9" in three months) or a
	// virtual list item is disambiguated.
	Root string `json:"root,omitempty"`
	// ByText matches the element's own trimmed text — the by_text channel the TUI flows use.
	ByText string `json:"text,omitempty"`
	// ByRole/ByName address the accessibility tree, the channel that survives a DOM re-shape.
	ByRole string `json:"role,omitempty"`
	ByName string `json:"name,omitempty"`
	// Point targets a live bounding box: the only address for a cell of a virtual scrolling list.
	X float64 `json:"x,omitempty"`
	Y float64 `json:"y,omitempty"`
	// Nth picks among several matches, and Exact requires the full string rather than a substring.
	Nth     int  `json:"nth,omitempty"`
	Exact   bool `json:"exact,omitempty"`
	Visible bool `json:"-"`
}

func (l Locator) String() string {
	switch {
	case l.CSS != "" && l.Root != "":
		return l.Root + " >> " + l.CSS
	case l.CSS != "":
		return "css:" + l.CSS
	case l.ByText != "":
		return fmtText(l.Exact, l.ByText)
	case l.ByRole != "":
		return "role:" + l.ByRole + " name:" + l.ByName
	case l.X != 0 || l.Y != 0:
		return sprintfPoint(l.X, l.Y)
	}
	return "<empty locator>"
}

func fmtText(exact bool, s string) string {
	if exact {
		return "text=^" + s + "$"
	}
	return "text~" + s
}

func sprintfPoint(x, y float64) string {
	b, _ := json.Marshal([]float64{x, y})
	return "point:" + string(b)
}

// Finding is the R4 output: every address for one element, from one probe.
type Finding struct {
	OK      bool   `json:"ok"`
	Reason  string `json:"reason,omitempty"`
	Blocked bool   `json:"blocked,omitempty"` // a shadow root refused to be queried
	Count   int    `json:"count,omitempty"`

	Tag  string     `json:"tag,omitempty"`
	ID   string     `json:"id,omitempty"`
	Role string     `json:"role,omitempty"`
	Name string     `json:"name,omitempty"`
	CSS  string     `json:"css,omitempty"` // the page-proven path, usable as a later selector
	Text string     `json:"text,omitempty"`
	Rect [4]float64 `json:"rect,omitempty"`
	// Disabled is the button-level form of the same fact State.Disabled carries: for a control whose
	// readiness is the whole question (a submit button that stays disabled until the page has accepted a
	// write) it is read straight off the finding rather than through the nested state.
	Disabled bool         `json:"disabled,omitempty"`
	State    ElementState `json:"state"`

	// AX and Native are the other two R4 channels, read from the bridge's own action reply rather than
	// from the DOM, so a locator that only works one way is visible in the record.
	AX     map[string]any `json:"ax,omitempty"`
	Native map[string]any `json:"native,omitempty"`

	// ClickMethod is what actually worked for this element: a bridge click, or a DOM click through
	// web_eval (which is what the virtual calendar needs).
	ClickMethod string `json:"clickMethod,omitempty"`
}

// ElementState is R5's read-side: enough to tell "present but not yet ready" from "gone".
type ElementState struct {
	Exists   bool     `json:"exists"`
	Visible  bool     `json:"visible"`
	Disabled bool     `json:"disabled"`
	Checked  *bool    `json:"checked,omitempty"`
	Selected *bool    `json:"selected,omitempty"`
	Values   []string `json:"values,omitempty"`
}

func (f Finding) String() string {
	if !f.OK {
		return "not found: " + f.Reason
	}
	parts := []string{}
	if f.ID != "" {
		parts = append(parts, "#"+f.ID)
	}
	if f.Role != "" {
		parts = append(parts, "role="+f.Role)
	}
	if f.Name != "" {
		parts = append(parts, "name="+f.Name)
	}
	if f.Text != "" {
		parts = append(parts, quote(f.Text))
	}
	parts = append(parts, sprintfPoint(f.Rect[0], f.Rect[1]))
	return strings.Join(parts, " ")
}

func quote(s string) string { return `"` + s + `"` }
