package youtube

import "time"

// The vocabulary shared by the flows (internal/youtube/pages) and the orchestrator. It lives in this package
// because pages imports it, not the other way round: the parent owns the types, the child owns the page
// logic, and the tool wires them together. That direction is also what keeps a flow free of orchestration —
// it cannot reach the journal even if it wanted to.

// Timings are the budgets a flow may spend, passed in so the flow never reads config.
type Timings struct {
	Action time.Duration // one bridge call
	Wizard time.Duration // the whole upload flow
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
	// Attached records that the attach landed, which is the moment a video begins to exist. It decides
	// whether a failed run keeps its journal slot charged or gives it back.
	Attached bool     `json:"attached,omitempty"`
	Steps    []string `json:"steps,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Findings []Probed `json:"findings,omitempty"`
}

// Record appends one probed element to the outcome.
func (o *Outcome) Record(role string, f Finding) {
	o.Findings = append(o.Findings, Probed{Role: role, Element: f})
}
