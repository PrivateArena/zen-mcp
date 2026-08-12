package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"zen-mcp/internal/mcpcfg"

	"gopkg.in/yaml.v3"
)

// LoadPromptDefinitions loads all prompt definitions from YAML files, skills, and defaults.
func LoadPromptDefinitions() ([]PromptDefinition, error) {
	byName := make(map[string]PromptDefinition)

	for _, p := range DEFAULT_PROMPTS {
		byName[p.Name] = p
	}

	legacy, err := loadYAMLFile(mcpcfg.ProjectRoot, "prompts.yaml")
	if err == nil && legacy != nil {
		for _, p := range legacy {
			if p.Name != "" {
				byName[p.Name] = p
			}
		}
	}

	modular, err := loadModularPrompts()
	if err == nil {
		for _, p := range modular {
			if p.Name != "" {
				byName[p.Name] = p
			}
		}
	}

	skillPrompts, err := generateSkillPrompts()
	if err == nil {
		for _, p := range skillPrompts {
			if p.Name != "" {
				byName[p.Name] = p
			}
		}
	}

	out := make([]PromptDefinition, 0, len(byName))
	for _, p := range byName {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out, nil
}

// GetPromptDefinition returns a single prompt by name.
func GetPromptDefinition(name string) (PromptDefinition, bool) {
	defs, err := LoadPromptDefinitions()
	if err != nil {
		return PromptDefinition{}, false
	}
	for _, p := range defs {
		if p.Name == name {
			return p, true
		}
	}
	return PromptDefinition{}, false
}

func loadYAMLFile(dir, name string) ([]PromptDefinition, error) {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var defs []PromptDefinition
	if err := yaml.Unmarshal(data, &defs); err != nil {
		return nil, err
	}
	return defs, nil
}

func loadModularPrompts() ([]PromptDefinition, error) {
	promptsDir := mcpcfg.PromptDir()
	if _, err := os.Stat(promptsDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(promptsDir)
	if err != nil {
		return nil, err
	}

	var results []PromptDefinition
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(promptsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var defs []PromptDefinition
		if err := yaml.Unmarshal(data, &defs); err != nil {
			continue
		}
		results = append(results, defs...)
	}
	return results, nil
}

func generateSkillPrompts() ([]PromptDefinition, error) {
	skillsDir := mcpcfg.SkillsDir()
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}

	var results []PromptDefinition
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") {
			continue
		}

		var skillFile string
		var skillID string

		if entry.IsDir() || (entry.Type()&os.ModeSymlink != 0 && isSymlinkDir(skillsDir, entry.Name())) {
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

		results = append(results, PromptDefinition{
			Name:          fmt.Sprintf("_%s", id),
			Description:   desc,
			Arguments:     []PromptArgument{{Name: "i", Description: "Context or specific instructions for this skill", Required: false}},
			Template:      fmt.Sprintf("Activate skill: %s\n\nTask: {{i}}", title),
			EnabledSkills: []string{skillID},
		})
	}
	return results, nil
}

func isSymlinkDir(dir, name string) bool {
	target := filepath.Join(dir, name)
	info, err := os.Stat(target)
	return err == nil && info.IsDir()
}

type frontmatter struct {
	Name        string
	Description string
	Framework   string
	Trigger     string
	Triggers    []string
}

func parseFrontmatter(content string) frontmatter {
	fm := frontmatter{}
	lines := strings.Split(content, "\n")
	inFM := false
	var currentKey string

	for _, line := range lines {
		if !inFM {
			if strings.TrimSpace(line) == "---" {
				inFM = true
			}
			continue
		}
		if strings.TrimSpace(line) == "---" {
			break
		}
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			currentKey = strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, "\"'")
			switch currentKey {
			case "name":
				fm.Name = value
			case "description":
				fm.Description = value
			case "framework":
				fm.Framework = value
			case "trigger":
				fm.Trigger = value
			case "triggers":
				// handled below
			}
		} else if strings.HasPrefix(strings.TrimSpace(line), "-") && currentKey == "triggers" {
			val := strings.TrimSpace(line[1:])
			val = strings.Trim(val, "\"'")
			fm.Triggers = append(fm.Triggers, val)
		}
	}
	return fm
}
