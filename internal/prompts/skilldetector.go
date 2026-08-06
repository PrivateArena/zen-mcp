package prompts

import (
	"fmt"
	"os"
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
	skillsDir := skills.SkillsDir()

	var path string
	var title string

	candidate := filepath.Join(skillsDir, skillID+".md")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		path = candidate
	} else {
		candidate = filepath.Join(skillsDir, skillID, "SKILL.md")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			path = candidate
		}
	}

	if path == "" {
		return "", fmt.Errorf("skill %q not found", skillID)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	fm := skills.ParseFrontmatter(string(content))
	title = fm["name"]
	if title == "" {
		title = skillID
	}

	skillDir := filepath.Dir(path)
	resolved := skills.ResolveSkillContent(string(content), skillDir, skillID)

	return fmt.Sprintf("# Skill: %s\n\n%s", title, resolved.Enriched), nil
}
