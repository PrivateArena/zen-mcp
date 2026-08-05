package prompts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jang/zen-mcp/internal/mcpcfg"
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

	skills, err := LoadSkills()
	if err != nil {
		return nil, err
	}

	matchedIDs := make(map[string]bool)
	for _, skill := range skills {
		if enableName && phraseMatch(matchText, skill.ID) {
			matchedIDs[skill.ID] = true
		}
		if enableTrigger && skill.Triggers != nil {
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

// LoadSkills loads all skills from the skills directory.
func LoadSkills() ([]Skill, error) {
	skillsDir := mcpcfg.SkillsDir()
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}

	var skills []Skill
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") {
			continue
		}

		var skillFile string
		var skillID string

		if entry.IsDir() {
			skillFile = filepath.Join(skillsDir, entry.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); os.IsNotExist(err) {
				continue
			}
			skillID = entry.Name()
		} else if strings.HasSuffix(entry.Name(), ".md") {
			skillFile = filepath.Join(skillsDir, entry.Name())
			skillID = strings.TrimSuffix(entry.Name(), ".md")
		} else {
			continue
		}

		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		fm := parseFrontmatter(string(data))
		id := fm.Name
		if id == "" {
			id = skillID
		}
		title := fm.Name
		if title == "" {
			title = skillID
		}
		desc := fm.Description
		if desc == "" {
			desc = title
		}
		skills = append(skills, Skill{
			ID:          id,
			Title:       title,
			Description: desc,
			Triggers:    fm.Triggers,
		})
	}
	return skills, nil
}

// LoadSkillContent loads the content of a skill by ID.
func LoadSkillContent(skillID string) (string, error) {
	skillsDir := mcpcfg.SkillsDir()

	// Try directory first
	dirPath := filepath.Join(skillsDir, skillID, "SKILL.md")
	if data, err := os.ReadFile(dirPath); err == nil {
		return string(data), nil
	}

	// Try file
	filePath := filepath.Join(skillsDir, skillID+".md")
	if data, err := os.ReadFile(filePath); err == nil {
		return string(data), nil
	}

	return "", fmt.Errorf("skill not found: %s", skillID)
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
