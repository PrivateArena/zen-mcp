package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"zen-mcp/internal/mcpcfg"
)

// SkillsDir returns the skills directory path.
func SkillsDir() string {
	return filepath.Join(mcpcfg.ProjectRoot, "resources", "skills")
}

// ParseFrontmatter parses YAML frontmatter from markdown content.
func ParseFrontmatter(content string) map[string]string {
	fm := make(map[string]string)
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
			fm[currentKey] = value
		} else if strings.HasPrefix(strings.TrimSpace(line), "-") && currentKey == "triggers" {
			val := strings.TrimSpace(line[1:])
			val = strings.Trim(val, "\"'")
			if _, ok := fm["triggers"]; !ok {
				fm["triggers"] = val
			} else {
				fm["triggers"] += "," + val
			}
		}
	}

	return fm
}

// LoadSkills loads all skills from the skills directory.
func LoadSkills() ([]SkillRegistryEntry, error) {
	skillsDir := SkillsDir()
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}

	var skills []SkillRegistryEntry
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") {
			continue
		}

		entryPath := filepath.Join(skillsDir, entry.Name())
		stat, statErr := os.Stat(entryPath)
		isDir := statErr == nil && stat.IsDir()

		var skillFile string
		var skillID string

		if isDir {
			skillFile = filepath.Join(entryPath, "SKILL.md")
			if _, err := os.Stat(skillFile); os.IsNotExist(err) {
				continue
			}
			skillID = entry.Name()
		} else if strings.HasSuffix(entry.Name(), ".md") {
			skillFile = entryPath
			skillID = strings.TrimSuffix(entry.Name(), ".md")
		} else {
			continue
		}

		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		fm := ParseFrontmatter(string(data))
		id := fm["name"]
		if id == "" {
			id = skillID
		}
		title := fm["name"]
		if title == "" {
			title = skillID
		}
		desc := fm["description"]
		if desc == "" {
			desc = title
		}
		framework := fm["framework"]
		if framework == "" {
			framework = "unspecified"
		}

		var triggers []string
		if t, ok := fm["triggers"]; ok && t != "" {
			triggers = strings.Split(t, ",")
			for i := range triggers {
				triggers[i] = strings.TrimSpace(triggers[i])
			}
		} else if t, ok := fm["trigger"]; ok && t != "" {
			triggers = []string{t}
		}

		skills = append(skills, SkillRegistryEntry{
			ID:          id,
			Title:       title,
			Description: desc,
			Framework:   framework,
			Path:        skillFile,
			Triggers:    triggers,
		})
	}

	return skills, nil
}

// FindSkillByID finds a skill by ID.
func FindSkillByID(id string) (SkillRegistryEntry, error) {
	skills, err := LoadSkills()
	if err != nil {
		return SkillRegistryEntry{}, err
	}
	for _, s := range skills {
		if s.ID == id {
			return s, nil
		}
	}
	return SkillRegistryEntry{}, fmt.Errorf("skill %q not found", id)
}

// ScanKnowledgeBase scans a skill's knowledge base directory.
func ScanKnowledgeBase(skillBaseDir, skillID string) ([]ResolvedReference, error) {
	kbDir := filepath.Join(skillBaseDir, skillID+"_kb")
	if _, err := os.Stat(kbDir); os.IsNotExist(err) {
		return nil, nil
	}

	var refs []ResolvedReference
	err := filepath.WalkDir(kbDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(kbDir, path)
		refs = append(refs, ResolvedReference{
			AbsolutePath: path,
			RelativePath: relPath,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// ScanSkillFiles scans a skill directory for all files.
func ScanSkillFiles(skillDir string) []ResolvedReference {
	resolvedDir, err := filepath.EvalSymlinks(skillDir)
	if err != nil {
		resolvedDir = skillDir
	}

	var refs []ResolvedReference
	err = filepath.WalkDir(resolvedDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == resolvedDir {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(resolvedDir, path)
		refs = append(refs, ResolvedReference{
			AbsolutePath: path,
			RelativePath: relPath,
		})
		return nil
	})
	if err != nil {
		return nil
	}
	return refs
}

var commandRefRE = regexp.MustCompile("`/([a-z][a-z0-9-]*)`")

// ResolveSkillContent enriches skill content with command hints and file references.
func ResolveSkillContent(content, skillBaseDir, skillID string) ResolvedSkillContent {
	result := ResolvedSkillContent{
		Enriched: content,
	}

	// Enrich command references
	enriched := commandRefRE.ReplaceAllStringFunc(content, func(match string) string {
		cmdName := match[2 : len(match)-1] // strip `/
		if !isValidCommandRef(cmdName) {
			return match
		}
		hint := fmt.Sprintf("`skill (id=\"%s\")`", cmdName)
		result.CommandHints = append(result.CommandHints, cmdName)
		return match + " (→ " + hint + ")"
	})
	result.Enriched = enriched

	// Determine file references
	mainSkillFile := filepath.Join(skillBaseDir, "SKILL.md")
	if _, err := os.Stat(mainSkillFile); err == nil {
		// Directory-based skill: scan entire skill folder
		refs := ScanSkillFiles(skillBaseDir)
		// Exclude the main SKILL.md
		var filtered []ResolvedReference
		for _, ref := range refs {
			if ref.AbsolutePath != mainSkillFile {
				filtered = append(filtered, ref)
			}
		}
		result.FileReferences = filtered
	} else {
		// File-based skill: check for _kb folder
		kbRefs, _ := ScanKnowledgeBase(skillBaseDir, skillID)
		result.FileReferences = kbRefs
	}

	if len(result.FileReferences) > 0 {
		referencedLabel := "Bundled skill files"
		if _, err := os.Stat(mainSkillFile); err != nil {
			referencedLabel = "Knowledge base files"
		}
		var sb strings.Builder
		sb.WriteString("\n\n## Referenced Files\n")
		sb.WriteString(fmt.Sprintf("The following %s are part of this skill. Read them if needed:\n", referencedLabel))
		for _, ref := range result.FileReferences {
			sb.WriteString(fmt.Sprintf("- [%s](file://%s)\n", ref.RelativePath, ref.AbsolutePath))
		}
		result.Enriched += sb.String()
	}

	return result
}

func isValidCommandRef(name string) bool {
	if name == "" {
		return false
	}
	if strings.Contains(name, "/") {
		return false
	}
	firstSegment := strings.ToLower(name)
	if strings.Contains(firstSegment, "/") {
		return false
	}
	excluded := map[string]bool{
		"tmp": true, "dev": true, "proc": true, "sys": true, "etc": true,
		"var": true, "usr": true, "home": true, "bin": true, "lib": true,
		"opt": true, "mnt": true, "media": true, "run": true, "srv": true,
	}
	if excluded[firstSegment] {
		return false
	}
	return true
}
