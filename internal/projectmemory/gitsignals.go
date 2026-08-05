package projectmemory

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GitSignals struct {
	ModifiedFiles []string          `json:"modifiedFiles"`
	RecentCommits []GitCommitSignal `json:"recentCommits"`
	IsGitRepo     bool              `json:"isGitRepo"`
}

type GitCommitSignal struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

// GetGitSignals ports git-signals.ts getGitSignals.
func GetGitSignals(workspaceRoot, lastVisitedIso string) GitSignals {
	result := GitSignals{
		ModifiedFiles: []string{},
		RecentCommits: []GitCommitSignal{},
	}

	git := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceRoot
		out, err := cmd.Output()
		return string(out), err
	}

	// Check .git dir, falling back to rev-parse for parent repos.
	gitDir := filepath.Join(workspaceRoot, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		revOut, revErr := git("rev-parse", "--is-inside-work-tree")
		if revErr != nil || strings.TrimSpace(revOut) != "true" {
			return result
		}
	}
	result.IsGitRepo = true

	if statusOut, err := git("status", "--porcelain"); err == nil {
		for _, line := range strings.Split(statusOut, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if len(line) > 3 {
				result.ModifiedFiles = append(result.ModifiedFiles, strings.TrimSpace(line[3:]))
			}
		}
	}

	commitArgs := []string{"log", "-n", "5", `--pretty=format:%H|%an|%ad|%s`, "--date=iso"}
	if lastVisitedIso != "" {
		commitArgs = []string{"log", "--since=" + lastVisitedIso, `--pretty=format:%H|%an|%ad|%s`, "--date=iso"}
	}
	if logOut, err := git(commitArgs...); err == nil {
		for _, line := range strings.Split(string(logOut), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 4)
			commit := GitCommitSignal{}
			if len(parts) > 0 {
				commit.Hash = parts[0]
			}
			if len(parts) > 1 {
				commit.Author = parts[1]
			}
			if len(parts) > 2 {
				commit.Date = parts[2]
			}
			if len(parts) > 3 {
				commit.Message = parts[3]
			}
			result.RecentCommits = append(result.RecentCommits, commit)
		}
	}

	return result
}
