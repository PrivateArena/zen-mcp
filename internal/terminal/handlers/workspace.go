package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("root", func(args []string, sessionID string) error {
		return cd(args, sessionID, true)
	})

	terminal.Register("cd", func(args []string, sessionID string) error {
		return cd(args, sessionID, false)
	})
}

func cd(args []string, sessionID string, isRoot bool) error {
	force := false
	pathArg := ""
	for _, a := range args {
		if a == "--force" {
			force = true
			continue
		}
		if pathArg == "" && !strings.HasPrefix(a, "--") {
			pathArg = a
		}
	}

	if pathArg == "" {
		return fmt.Errorf("usage: %s <path> [--force]", "cd")
	}

	d := terminal.GetDeps()
	if d.Sess == nil {
		return fmt.Errorf("session not initialized")
	}

	if force {
		cwd, _ := os.Getwd()
		finalPath := pathArg
		if !filepath.IsAbs(pathArg) {
			finalPath = filepath.Join(cwd, pathArg)
		}
		if _, err := os.Stat(finalPath); os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist", finalPath)
		}
		d.Sess.SetSessionWorkspaceRoot(sessionID, finalPath)
		terminal.Logf("OK: Workspace root -> %s", finalPath)
		terminal.Logf("Reload config.json to apply to future sessions")
		return nil
	}

	current := terminal.Ws(sessionID)
	target := pathArg
	if !filepath.IsAbs(target) {
		cwd, _ := os.Getwd()
		target = filepath.Join(cwd, target)
	}
	if target != current {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			terminal.Logf("ERROR: %s does not exist. Workspace root unchanged: %s", target, current)
			return nil
		}
		d.Sess.SetSessionWorkspaceRoot(sessionID, target)
		terminal.Logf("OK: Workspace root -> %s", target)
	} else {
		terminal.Logf("OK: Workspace root -> %s", current)
	}
	return nil
}
