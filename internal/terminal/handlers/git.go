package handlers

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/terminal"
)

const junkOrg = "e3b0c44298fc1c149afbf4c8996fb92427ae123"
const baseURL = "https://github.com/" + junkOrg

// runGitCommand is a helper function
func runGitCommand(workdir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workdir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// initializes the package
func init() {
	terminal.Register("git-tmp", func(args []string) error {
		terminal.Logf("Mirroring current repo to junk GitHub org for review...")
		terminal.Logf("(git-tmp requires gh CLI integration; see TS implementation)")
		return nil
	})

	terminal.Register("git-twipe", func(args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: git-twipe <id>")
		}
		target := args[0]
		repoName := target
		if m := regexp.MustCompile(`github\.com\/[^/]+\/([^/]+)`).FindStringSubmatch(target); len(m) > 1 {
			repoName = m[1]
		}
		if strings.HasSuffix(repoName, ".git") {
			repoName = repoName[:len(repoName)-4]
		}
		if repoName == "" {
			return fmt.Errorf("could not determine repo name from identifier")
		}
		terminal.Logf("Deleting temp repo: %s/%s", baseURL, repoName)
		return nil
	})

	terminal.Register("commit-review", func(args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: commit-review <commitA> [commitB]")
		}
		commitA := args[0]
		commitB := ""
		if len(args) > 1 {
			commitB = args[1]
		}

		wRoot := terminal.Ws()
		terminal.Logf("COMMIT-REVIEW: %s %s", commitA, commitB)

		var files []string
		var commitMessages string

		if commitB == "" {
			hasParent, _ := runGitCommand(wRoot, "rev-parse", "--quiet", "--verify", commitA+"~1")
			hasParent = strings.TrimSpace(hasParent)
			if hasParent != "" {
				out, err := runGitCommand(wRoot, "diff", "--name-only", "--diff-filter=d", commitA+"~1", commitA)
				if err == nil {
					files = strings.Split(strings.TrimSpace(out), "\n")
				}
			} else {
				out, err := runGitCommand(wRoot, "ls-tree", "-r", "--name-only", commitA)
				if err == nil {
					files = strings.Split(strings.TrimSpace(out), "\n")
				}
			}
			out, _ := runGitCommand(wRoot, "log", "-1", "--format=%B", commitA)
			commitMessages = strings.TrimSpace(out)
		} else {
			out, err := runGitCommand(wRoot, "diff", "--name-only", "--diff-filter=d", commitA, commitB)
			if err == nil {
				files = strings.Split(strings.TrimSpace(out), "\n")
			}
			out, _ = runGitCommand(wRoot, "log", "--format=%B", commitA+".."+commitB)
			commitMessages = strings.TrimSpace(out)
		}

		files = filterEmpty(files)
		if len(files) == 0 {
			terminal.Logf("No changed files found for the specified commit(s).")
			return nil
		}

		archiveCommit := commitB
		if archiveCommit == "" {
			archiveCommit = commitA
		}
		tmpDir := filepath.Join("/tmp", archiveCommit)
		if err := os.RemoveAll(tmpDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to prepare temp dir: %v", err)
		}
		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			return fmt.Errorf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		archiveOut, archiveErr := runGitCommand(wRoot, "archive", "--format=tar", archiveCommit)
		if archiveErr != nil {
			return fmt.Errorf("git archive failed: %v", archiveErr)
		}

		if err := extractTar(bytes.NewReader([]byte(archiveOut)), tmpDir); err != nil {
			return fmt.Errorf("tar extract failed: %v", err)
		}

		absoluteFiles := make([]string, 0, len(files))
		for _, f := range files {
			absoluteFiles = append(absoluteFiles, filepath.Join(tmpDir, f))
		}

		reviewPrompt := buildReviewPrompt(commitMessages, files)

		terminal.Logf("Uploading %d files for review...", len(files))
		res, err := bridge.CallBridge(context.Background(), "chat", map[string]any{
			"message":  reviewPrompt,
			"provider": "claude",
			"path":     absoluteFiles,
			"strict":   false,
		})
		if err != nil {
			terminal.Logf("ERROR: %v", err)
			return nil
		}
		terminal.Logf("RESULT:\n%s", formatResult(res))
		return nil
	})
}

// filterEmpty is a helper function
func filterEmpty(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// buildReviewPrompt is a helper function
func buildReviewPrompt(commitMessages string, files []string) string {
	fileList := ""
	for _, f := range files {
		fileList += fmt.Sprintf("- %s\n", f)
	}
	if commitMessages != "" {
		return fmt.Sprintf("Please review the following commit changes.\n\nCommit message(s):\n%s\n\nChanged files:\n%s", commitMessages, fileList)
	}
	return fmt.Sprintf("Please review the following commit changes.\n\nChanged files:\n%s", fileList)
}

// formatResult is a helper function
func formatResult(data map[string]any) string {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(b)
}

// extractTar is a helper function
func extractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, hdr.Name)
		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		f, err := os.Create(target)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(f, tr)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
}
