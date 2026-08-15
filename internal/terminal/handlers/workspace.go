package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"zen-mcp/internal/terminal"
	"zen-mcp/internal/workspace"
)

// initializes the package
func init() {
	terminal.Register("root", func(args []string) error {
		return cd(args)
	})

	terminal.Register("cd", func(args []string) error {
		return cd(args)
	})
}

// cd mirrors the TS `cd` handler. The argument is resolved through the
// map.json alias/heuristic resolver, so `cd zen-mcp` or `cd server mcp`
// resolves to a registered full path; unknown input falls back to a
// cwd-joined relative path (checked for existence by the caller).
func cd(args []string) error {
	force := false
	var pathParts []string
	for _, a := range args {
		if a == "--force" {
			force = true
			continue
		}
		if !strings.HasPrefix(a, "--") {
			pathParts = append(pathParts, a)
		}
	}
	if len(pathParts) == 0 {
		return fmt.Errorf("usage: %s <path> [--force]", "cd")
	}

	d := terminal.GetDeps()
	if d.Store == nil {
		return fmt.Errorf("store not initialized")
	}

	cwd, _ := os.Getwd()
	resolvedPath := workspace.ResolveWorkspacePath(strings.Join(pathParts, " "), cwd)
	current := terminal.Ws()

	if force {
		finalPath := resolvedPath
		if !exists(finalPath) {
			finalPath = filepath.Join(cwd, strings.Join(pathParts, " "))
		}
		if !exists(finalPath) {
			return fmt.Errorf("%s does not exist", finalPath)
		}
		d.Store.Set("workspace-root", finalPath)
		terminal.Logf("OK: Workspace root -> %s", finalPath)
		terminal.Logf("Reload config.json to apply to future sessions")
		return nil
	}

	if resolvedPath != current && exists(resolvedPath) {
		d.Store.Set("workspace-root", resolvedPath)
		terminal.Logf("OK: Workspace root -> %s", resolvedPath)
	} else if !exists(resolvedPath) {
		terminal.Logf("ERROR: %s does not exist. Workspace root unchanged: %s", resolvedPath, current)
	} else {
		terminal.Logf("OK: Workspace root -> %s", current)
	}
	return nil
}

// exists is a helper function
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
