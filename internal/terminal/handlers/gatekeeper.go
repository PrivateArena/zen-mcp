package handlers

import (
	"fmt"
	"strings"

	"zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("accept", func(args []string) error {
		d := terminal.GetDeps()
		if d.Gatekeeper == nil {
			return fmt.Errorf("gatekeeper not initialized")
		}
		id := ""
		if len(args) > 0 {
			id = args[0]
		}
		pending := d.Gatekeeper.GetPendingConfirmations()
		if len(pending) == 0 {
			terminal.Logf("No pending confirmation found.")
			return nil
		}
		if id == "" && len(pending) > 1 {
			terminal.Logf("ERROR: %d confirmations pending. Specify which to accept: accept <id>", len(pending))
			for _, item := range pending {
				terminal.Logf(" - [%s] %s", item.ID, item.Description)
			}
			return nil
		}
		if d.Gatekeeper.AcceptConfirmation(id) {
			terminal.Logf("Confirmation accepted.")
		} else {
			terminal.Logf("No pending confirmation found.")
		}
		return nil
	})

	terminal.Register("yes", func(args []string) error {
		d := terminal.GetDeps()
		if d.Gatekeeper == nil {
			return fmt.Errorf("gatekeeper not initialized")
		}
		pending := d.Gatekeeper.GetPendingConfirmations()
		if len(pending) == 0 {
			terminal.Logf("No pending confirmation found.")
			return nil
		}
		if d.Gatekeeper.AcceptConfirmation("") {
			terminal.Logf("Confirmation accepted.")
		} else {
			terminal.Logf("No pending confirmation found.")
		}
		return nil
	})

	terminal.Register("y", func(args []string) error {
		d := terminal.GetDeps()
		if d.Gatekeeper == nil {
			return fmt.Errorf("gatekeeper not initialized")
		}
		pending := d.Gatekeeper.GetPendingConfirmations()
		if len(pending) == 0 {
			terminal.Logf("No pending confirmation found.")
			return nil
		}
		if d.Gatekeeper.AcceptConfirmation("") {
			terminal.Logf("Confirmation accepted.")
		} else {
			terminal.Logf("No pending confirmation found.")
		}
		return nil
	})

	terminal.Register("reject", func(args []string) error {
		d := terminal.GetDeps()
		if d.Gatekeeper == nil {
			return fmt.Errorf("gatekeeper not initialized")
		}
		firstArg := ""
		if len(args) > 0 {
			firstArg = args[0]
		}
		pending := d.Gatekeeper.GetPendingConfirmations()

		var id string
		var suggestion string
		hasID := firstArg != "" && func() bool {
			for _, p := range pending {
				if p.ID == firstArg {
					return true
				}
			}
			return false
		}()

		if hasID {
			id = firstArg
			suggestion = strings.Join(args[1:], " ")
		} else if len(pending) > 1 {
			terminal.Logf("ERROR: %d confirmations pending. Specify which to reject: reject <id> [suggestion...]", len(pending))
			for _, item := range pending {
				terminal.Logf(" - [%s] %s", item.ID, item.Description)
			}
			return nil
		} else {
			suggestion = strings.Join(args, " ")
		}

		if d.Gatekeeper.RejectConfirmation(id, suggestion) {
			terminal.Logf("Confirmation rejected.%s", func() string {
				if suggestion != "" {
					return fmt.Sprintf(" Suggestion sent: \"%s\"", suggestion)
				}
				return ""
			}())
		} else {
			terminal.Logf("No pending confirmation found.")
		}
		return nil
	})

	terminal.Register("no", func(args []string) error {
		d := terminal.GetDeps()
		if d.Gatekeeper == nil {
			return fmt.Errorf("gatekeeper not initialized")
		}
		pending := d.Gatekeeper.GetPendingConfirmations()
		if len(pending) == 0 {
			terminal.Logf("No pending confirmation found.")
			return nil
		}
		suggestion := strings.Join(args, " ")
		if d.Gatekeeper.RejectConfirmation("", suggestion) {
			terminal.Logf("Confirmation rejected.%s", func() string {
				if suggestion != "" {
					return fmt.Sprintf(" Suggestion sent: \"%s\"", suggestion)
				}
				return ""
			}())
		} else {
			terminal.Logf("No pending confirmation found.")
		}
		return nil
	})

	terminal.Register("n", func(args []string) error {
		d := terminal.GetDeps()
		if d.Gatekeeper == nil {
			return fmt.Errorf("gatekeeper not initialized")
		}
		pending := d.Gatekeeper.GetPendingConfirmations()
		if len(pending) == 0 {
			terminal.Logf("No pending confirmation found.")
			return nil
		}
		suggestion := strings.Join(args, " ")
		if d.Gatekeeper.RejectConfirmation("", suggestion) {
			terminal.Logf("Confirmation rejected.%s", func() string {
				if suggestion != "" {
					return fmt.Sprintf(" Suggestion sent: \"%s\"", suggestion)
				}
				return ""
			}())
		} else {
			terminal.Logf("No pending confirmation found.")
		}
		return nil
	})

	terminal.Register("pending", func(args []string) error {
		d := terminal.GetDeps()
		if d.Gatekeeper == nil {
			return fmt.Errorf("gatekeeper not initialized")
		}
		list := d.Gatekeeper.GetPendingConfirmations()
		if len(list) == 0 {
			terminal.Logf("No pending confirmations.")
			return nil
		}
		terminal.Logf("\nPENDING CONFIRMATIONS:")
		for _, item := range list {
			terminal.Logf(" - [%s] %s", item.ID, item.Description)
		}
		return nil
	})
}
