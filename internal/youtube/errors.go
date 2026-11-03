package youtube

import (
	"errors"
	"fmt"
	"strings"
)

// Error taxonomy (plan §4.3). A flow never parses a bridge error string; every transport failure goes
// through Classify exactly once and carries a sentinel from then on.
var (
	ErrIdentity       = errors.New("youtube: identity mismatch")
	ErrElement        = errors.New("youtube: element not found")
	ErrWait           = errors.New("youtube: page did not settle")
	ErrAbandonedTab   = errors.New("youtube: tab is gone or not ours to drive")
	ErrDailyLimit     = errors.New("youtube: daily upload limit reached")
	ErrWizardStuck    = errors.New("youtube: wizard stuck")
	ErrUploadTooLarge = errors.New("youtube: file too large to attach")
	ErrBusy           = errors.New("youtube: another long upload holds the slot")
)

type ErrNotDone struct{ VideoID string }

func (e ErrNotDone) Error() string {
	return fmt.Sprintf("video %s is not done processing", e.VideoID)
}

type ErrIdentityMismatch struct{ Want, Got string }

func (ErrIdentityMismatch) Unwrap() error { return ErrIdentity }

func (e ErrIdentityMismatch) Error() string {
	return fmt.Sprintf("identity mismatch: expected %s, the tab reports %s", e.Want, e.Got)
}

type ErrNotFound struct{ Locator string }

func (ErrNotFound) Unwrap() error { return ErrElement }

func (e ErrNotFound) Error() string { return fmt.Sprintf("element not found: %s", e.Locator) }

type ErrTimeout struct{ What, After, Last string }

func (ErrTimeout) Unwrap() error { return ErrWait }

func (e ErrTimeout) Error() string {
	return fmt.Sprintf("page did not settle: %s within %s; last observed %s", e.What, e.After, e.Last)
}

type ErrAbandoned struct{ Detail string }

func (ErrAbandoned) Unwrap() error { return ErrAbandonedTab }

func (e ErrAbandoned) Error() string {
	return fmt.Sprintf("tab is gone or not ours to drive: %s", e.Detail)
}

type ErrStuck struct{ Text string }

func (ErrStuck) Unwrap() error { return ErrWizardStuck }

func (e ErrStuck) Error() string {
	return fmt.Sprintf("wizard stuck: the page still shows %q", e.Text)
}

// Classify maps a raw bridge or context error onto the taxonomy. The inputs are the exact strings the
// bridge replies with — "Element not found: <selector>", "Timeout waiting for …", "No tab id" — so this
// is the only place they are matched, and TestClassifyBridgeStrings pins them to the real replies.
//
// A reply carrying success:false is NOT an action error (it is the probe's own answer), so this must be
// called with transport errors only.
func Classify(raw string) error {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	low := strings.ToLower(s)
	switch {
	case strings.Contains(s, "Identity mismatch"), strings.Contains(low, "no tab id"),
		strings.Contains(low, "expected container"), strings.Contains(low, "landed on"):
		return ErrIdentityMismatch{Want: "the tab the bridge reported", Got: s}
	case strings.Contains(s, "Element not found"), strings.Contains(low, "no element for"),
		strings.Contains(low, "not visible"), strings.Contains(low, "no match"):
		return ErrNotFound{Locator: s}
	case strings.Contains(low, "timeout"), strings.Contains(low, "did not settle"),
		strings.Contains(low, "not done processing"), strings.Contains(low, "still shows"), strings.Contains(low, "never showed"):
		return ErrTimeout{What: s, After: "the configured timeout"}
	case strings.Contains(low, "unauthorized"), strings.Contains(low, "not a valid tabregistry id"),
		strings.Contains(low, "tab not found"), strings.Contains(low, "context canceled"):
		return ErrAbandoned{Detail: s}
	}
	return errors.New(s)
}
