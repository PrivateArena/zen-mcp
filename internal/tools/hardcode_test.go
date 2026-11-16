package tools

// A test that reads the repository's own source, which looks like the wrong kind of test until
// you notice what it is guarding. Plan 1 F-18 was a port written in two places — the YAML and a
// Go literal — and plan 2 A-8 is the same defect one layer over: a site's port lives in its config
// file, so any loopback number in this package is either a fixture or a second owner of a value
// that the fleet already owns. The compiler cannot see that, and neither can a reviewer two weeks
// from now, but this can.
//
// The allowlist is the honest part. A guard that permits "test files" wholesale would hide a real
// instance forever, so every entry names the file and the reason, and a new one has to be argued
// for in a diff.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// hardcodedLoopback matches a literal loopback URL with a port. It deliberately does not try to
// distinguish "site port" from "MCP transport port" by value: intent is not inferable from a number,
// which is exactly why the allowlist below is explicit.
var hardcodedLoopback = regexp.MustCompile(`127\.0\.0\.1:[0-9]{2,5}|localhost:[0-9]{2,5}|\[::1\]:[0-9]{2,5}`)

// portAllowlist is every place a loopback literal may appear, each with its reason. Two non-test
// entries, both real: this process starts the zenCap capture service itself, so that address is not
// a fleet site; and board_endpoint.go holds the documented fallbacks used only when no status
// document exists. Everything else is a test fixture, named file by file — a guard that allowed
// "_test.go" as a class would hide a fixture that grew into a real client.
var portAllowlist = map[string]string{
	"internal/tools/capture.go":                  "the zenCap capture service this process starts, not a fleet site",
	"internal/tools/board_endpoint.go":           "the fallback ports themselves, used only with no status document (plan 2 A-8)",
	"internal/webserver/client_test.go":          "httptest listener standing in for a site",
	"internal/tools/capture_test.go":             "zenCap fixture ports",
	"internal/tools/bridge_race_test.go":         "fake bridge listener",
	"internal/tools/request_test.go":             "fixture URL",
	"internal/tools/fetch_test.go":               "fixture URL",
	"internal/tools/fetch_fixtures_test.go":      "fixture URL, one inside a comment",
	"internal/tools/search_test.go":              "fixture URL",
	"internal/mcpcfg/config_test.go":             "this process's own daemon port in a config fixture",
	"internal/terminal/exportcli_test.go":        "generated CLI fixture pointing at this process's CLI port",
	"internal/terminal/exportcli_array_test.go":  "generated CLI fixture",
	"internal/terminal/exportcli_json_test.go":   "generated CLI fixture",
	"internal/terminal/exportcli_run_test.go":    "generated CLI fixture",
	"internal/terminal/exportcli_dryrun_test.go": "generated CLI fixture",
	"internal/terminal/exportcli_upload_test.go": "generated CLI fixture",
}

func TestNoHardcodedSitePorts(t *testing.T) {
	root := repoRoot(t)
	re := hardcodedLoopback
	misses := map[string][]string{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "cli", "testdata", "dist", "_obj":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, "internal/") {
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if _, known := portAllowlist[rel]; known {
			return nil
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if re.MatchString(line) {
				misses[rel] = append(misses[rel], fmt.Sprintf("%d: %s", i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	if len(misses) > 0 {
		files := make([]string, 0, len(misses))
		for f := range misses {
			files = append(files, f)
		}
		sort.Strings(files)
		var b strings.Builder
		for _, f := range files {
			fmt.Fprintf(&b, "\n  %s\n    %s", f, strings.Join(misses[f], "\n    "))
		}
		t.Errorf("hardcoded loopback endpoint outside the allowlist (plan 2 A-8):%s\n\nA site's port belongs in its YAML and in the fleet status document, nowhere else. If this is one of this process's own listeners, add the file to portAllowlist with a reason; if it is a site, resolve it through webserver.Resolve instead.", b.String())
	}
}

// TestAllowlistHasNoDeadEntries keeps the list honest in the other direction: an entry for a file
// that no longer contains a literal is how a guard quietly stops covering what it claims to.
func TestAllowlistHasNoDeadEntries(t *testing.T) {
	root := repoRoot(t)
	for rel := range portAllowlist {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("portAllowlist names %s, which does not exist", rel)
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Error(err)
			continue
		}
		if !hardcodedLoopback.Match(raw) {
			t.Errorf("portAllowlist names %s, which contains no loopback literal; drop the entry or the guard is lying about what it covers", rel)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not find the module root")
	return ""
}
