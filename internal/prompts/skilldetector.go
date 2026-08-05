package prompts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jang/zen-mcp/internal/mcpcfg"
	"github.com/jang/zen-mcp/internal/skills"
)

// DetectedSkill represents a detected skill.
type DetectedSkill struct {
	ID      string
	Content string
}

// DetectSkills detects skills based on argument text, triggers, and names.
func DetectSkills(argText string, enableTrigger bool, enableName bool, alreadyInjected map[string]bool) ([]DetectedSkill, error) {
	if !enableTrigger && !enableName {
		return nil, nil
	}

	matchText := argText
	if isPlaceholderText(argText) {
		wsContext, err := getWorkspaceContext()
		if err == nil {
			matchText += wsContext
		}
	}

	skillEntries, err := skills.LoadSkills()
	if err != nil {
		return nil, err
	}

	matchedIDs := make(map[string]bool)
	for _, skill := range skillEntries {
		if enableName && phraseMatch(matchText, skill.ID) {
			matchedIDs[skill.ID] = true
		}
		if enableTrigger && len(skill.Triggers) > 0 {
			for _, trigger := range skill.Triggers {
				if phraseMatch(matchText, trigger) {
					matchedIDs[skill.ID] = true
					break
				}
			}
		}
	}

	var newCandidates []string
	for id := range matchedIDs {
		if !alreadyInjected[id] {
			newCandidates = append(newCandidates, id)
		}
	}

	maxSkills := 3
	if len(newCandidates) > maxSkills {
		newCandidates = newCandidates[:maxSkills]
	}

	var results []DetectedSkill
	for _, id := range newCandidates {
		content, err := LoadSkillContent(id)
		if err == nil && content != "" {
			alreadyInjected[id] = true
			results = append(results, DetectedSkill{ID: id, Content: content})
		}
	}

	return results, nil
}

func phraseMatch(text, phrase string) bool {
	if phrase == "" || text == "" {
		return false
	}
	escaped := regexp.QuoteMeta(phrase)
	pattern := fmt.Sprintf("(?:^|[^a-zA-Z0-9_])%s(?:[^a-zA-Z0-9_]|$)", escaped)
	matched, _ := regexp.MatchString(pattern, text)
	return matched
}

func isPlaceholderText(text string) bool {
	trimmed := strings.TrimSpace(text)
	return trimmed == "" ||
		regexp.MustCompile(`^\$\d+$`).MatchString(trimmed) ||
		regexp.MustCompile(`^\$\{\d+:[^}]*\}$`).MatchString(trimmed) ||
		regexp.MustCompile(`^\$\{\d+\}$`).MatchString(trimmed)
}

func getWorkspaceContext() (string, error) {
	wsRoot := mcpcfg.ProjectRoot
	if wsRoot == "" {
		return "", nil
	}

	var ctx strings.Builder
	ctx.WriteString(fmt.Sprintf("Workspace path: %s\n", wsRoot))

	pkgJsonPath := filepath.Join(wsRoot, "package.json")
	if data, err := os.ReadFile(pkgJsonPath); err == nil {
		ctx.WriteString(fmt.Sprintf("Project package.json content:\n%s\n", string(data[:min(len(data), 4000)])))
	}

	goModPath := filepath.Join(wsRoot, "go.mod")
	if data, err := os.ReadFile(goModPath); err == nil {
		ctx.WriteString(fmt.Sprintf("Project go.mod content:\n%s\n", string(data[:min(len(data), 4000)])))
	}

	return ctx.String(), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LoadSkills loads all skills from the skills directory (prompts wrapper).
func LoadSkills() ([]Skill, error) {
	entries, err := skills.LoadSkills()
	if err != nil {
		return nil, err
	}
	out := make([]Skill, 0, len(entries))
	for _, e := range entries {
		out = append(out, Skill{
			ID:          e.ID,
			Title:       e.Title,
			Description: e.Description,
			Triggers:    e.Triggers,
		})
	}
	return out, nil
}

// LoadSkillContent loads the content of a skill by ID with reference resolution.
func LoadSkillContent(skillID string) (string, error) {
	skill, err := skills.FindSkillByID(skillID)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(skill.Path)
	if err != nil {
		return "", err
	}

	skillDir := filepath.Dir(skill.Path)
	resolved := skills.ResolveSkillContent(string(content), skillDir, skillID)

	return fmt.Sprintf("# Skill: %s\n\n%s", skill.Title, resolved.Enriched), nil
}

func execGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = mcpcfg.ProjectRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
