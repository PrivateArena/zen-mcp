package youtube

import (
	"encoding/json"
	"time"
)

// The vocabulary shared by the flows (internal/youtube/pages) and the orchestrator. It lives in this package
// because pages imports it, not the other way round: the parent owns the types, the child owns the page
// logic, and the tool wires them together. That direction is also what keeps a flow free of orchestration —
// it cannot reach the journal even if it wanted to.

// Timings are the budgets a flow may spend, passed in so the flow never reads config.
type Timings struct {
	Action time.Duration // one bridge call
	Wizard time.Duration // the whole upload flow
	// MaxScrolls is the runaway guard on a lazy-render loop — how many times a flow may drive the page down
	// while waiting for a lazily built section to exist at all. It is youtube.maxScrolls from config, handed
	// over here so the flow never reads config itself; 0 means the flow's own floor.
	MaxScrolls int
}

// Upload is the wizard's input. MadeForKids is a pointer because its absence is meaningful: the page refuses
// to advance without an answer, so a zero value cannot stand in for "unset".
type Upload struct {
	File        string
	Title       string
	Description string
	Tags        []string
	Category    string
	Thumbnail   string
	MadeForKids *bool
	Visibility  string // private | unlisted | public | schedule
	PublishAt   time.Time
	Evidence    string // directory for screenshots, "" to skip
	DryRun      bool
}

// WatchInput is one watch-page action: read a video's rendered comment rows, post a top-level comment, or
// answer one row. VideoID is already resolved to a bare id by the tool layer — a flow is never handed a URL
// it would have to trust.
type WatchInput struct {
	VideoID string
	Action  string // read | comment | reply
	Text    string
	// CommentID is the `lc` id of the row a reply answers, exactly as read (watch.md §1).
	CommentID string
	DryRun    bool
	Evidence  string
	// Journal is where a posted comment is recorded, claimed inside the flow at the press rather than by
	// Service before it: for an upload the attach is the first step and the mutation, but here the mutation
	// is one deep press inside a long walk, and §4.6's rule is to claim before the mutation, not before the
	// page load. Nil for a read and for a dry run, which mutate nothing.
	Journal *Journal
}

// Finding is one probed element, paired with the role it played in a flow, for --explain.
type RoleFinding struct {
	Role    string  `json:"role"`
	Element Finding `json:"finding"`
}

// Probed pairs a locator's role with everything R4 learned about the element that played it.
type Probed struct {
	Role    string  `json:"role"`
	Element Finding `json:"finding"`
}

// Outcome is what a flow reports. Findings carries the R4 record for every element it touched, so --explain
// renders the same data the flow acted on.
type Outcome struct {
	VideoID      string `json:"videoId,omitempty"`
	URL          string `json:"url,omitempty"`
	Panel        string `json:"panel,omitempty"`
	Visibility   string `json:"visibility,omitempty"`
	ScheduledFor string `json:"scheduledFor,omitempty"`
	Checks       string `json:"checks,omitempty"`
	// CommentID is the `lc` id of a row this run posted, read back off the page (watch-run.md §5). It is what
	// a later reply addresses, and its absence after a submit is exactly the unverified case.
	CommentID string `json:"commentId,omitempty"`
	// Attached records that the attach landed, which is the moment a video begins to exist. It decides
	// whether a failed run keeps its journal slot charged or gives it back.
	Attached bool     `json:"attached,omitempty"`
	Steps    []string `json:"steps,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Findings []Probed `json:"findings,omitempty"`
	// Data is the page's own answer, passed through untouched: a read flow reports what the probe read
	// rather than a Go-shaped paraphrase of it, so the reply cannot claim more than the page showed.
	Data json.RawMessage `json:"data,omitempty"`
}

// Record appends one probed element to the outcome.
func (o *Outcome) Record(role string, f Finding) {
	o.Findings = append(o.Findings, Probed{Role: role, Element: f})
}
