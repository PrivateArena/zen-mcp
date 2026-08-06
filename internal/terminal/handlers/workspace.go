package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jang/zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("root", func(args []string) error {
		return cd(args, true)
	})

	terminal.Register("cd", func(args []string) error {
		return cd(args, false)
	})
}

func cd(args []string, isRoot bool) error {
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
	if d.Store == nil {
		return fmt.Errorf("store not initialized")
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
		d.Store.Set("workspace-root", finalPath)
		terminal.Logf("OK: Workspace root -> %s", finalPath)
		terminal.Logf("Reload config.json to apply to future sessions")
		return nil
	}

	current := terminal.Ws()
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
		d.Store.Set("workspace-root", target)
		terminal.Logf("OK: Workspace root -> %s", target)
	} else {
		terminal.Logf("OK: Workspace root -> %s", current)
	}
	return nil
}
