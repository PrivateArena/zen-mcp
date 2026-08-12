package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zen-mcp/internal/mcpcfg"
)

func TestGenerateSkillPrompts_RegularDirectorySkill(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	skillsDir := filepath.Join(tmpDir, "resources", "skills")
	skillDir := filepath.Join(skillsDir, "dir-skill")
	os.MkdirAll(skillDir, 0o755)

	skillContent := `---
name: dir-skill
description: A directory-based skill
framework: test
---
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644)

	defs, err := generateSkillPrompts()
	if err != nil {
		t.Fatalf("generateSkillPrompts failed: %v", err)
	}

	var found bool
	for _, def := range defs {
		if def.Name == "SKILL-dir-skill" {
			found = true
			if def.Description != "A directory-based skill" {
				t.Errorf("unexpected description: %s", def.Description)
			}
			if len(def.EnabledSkills) != 1 || def.EnabledSkills[0] != "dir-skill" {
				t.Errorf("unexpected enabledSkills: %v", def.EnabledSkills)
			}
		}
	}
	if !found {
		t.Errorf("expected SKILL-dir-skill in results, got: %v", defs)
	}
}

func TestGenerateSkillPrompts_MarkdownFileSkill(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	skillsDir := filepath.Join(tmpDir, "resources", "skills")
	os.MkdirAll(skillsDir, 0o755)

	skillContent := `---
name: md-skill
description: A markdown-file skill
framework: test
---
`
	os.WriteFile(filepath.Join(skillsDir, "md-skill.md"), []byte(skillContent), 0o644)

	defs, err := generateSkillPrompts()
	if err != nil {
		t.Fatalf("generateSkillPrompts failed: %v", err)
	}

	var found bool
	for _, def := range defs {
		if def.Name == "SKILL-md-skill" {
			found = true
			if def.Description != "A markdown-file skill" {
				t.Errorf("unexpected description: %s", def.Description)
			}
		}
	}
	if !found {
		t.Errorf("expected SKILL-md-skill in results, got: %v", defs)
	}
}

func TestGenerateSkillPrompts_SymlinkedDirectorySkill(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	skillsDir := filepath.Join(tmpDir, "resources", "skills")
	os.MkdirAll(skillsDir, 0o755)

	realSkillDir := filepath.Join(tmpDir, "real-skills", "symlinked-skill")
	os.MkdirAll(realSkillDir, 0o755)

	skillContent := `---
name: symlinked-skill
description: A symlinked directory skill
framework: test
---
`
	os.WriteFile(filepath.Join(realSkillDir, "SKILL.md"), []byte(skillContent), 0o644)

	linkPath := filepath.Join(skillsDir, "symlinked-skill")
	if err := os.Symlink(realSkillDir, linkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	defs, err := generateSkillPrompts()
	if err != nil {
		t.Fatalf("generateSkillPrompts failed: %v", err)
	}

	var found bool
	for _, def := range defs {
		if def.Name == "SKILL-symlinked-skill" {
			found = true
			if def.Description != "A symlinked directory skill" {
				t.Errorf("unexpected description: %s", def.Description)
			}
			if len(def.EnabledSkills) != 1 || def.EnabledSkills[0] != "symlinked-skill" {
				t.Errorf("unexpected enabledSkills: %v", def.EnabledSkills)
			}
		}
	}
	if !found {
		t.Errorf("expected SKILL-symlinked-skill in results, got: %v", defs)
	}
}

func TestGenerateSkillPrompts_SymlinkToNonDirectory(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	skillsDir := filepath.Join(tmpDir, "resources", "skills")
	os.MkdirAll(skillsDir, 0o755)

	realFile := filepath.Join(tmpDir, "real-file.txt")
	os.WriteFile(realFile, []byte("not a dir"), 0o644)

	linkPath := filepath.Join(skillsDir, "broken-link")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	defs, err := generateSkillPrompts()
	if err != nil {
		t.Fatalf("generateSkillPrompts failed: %v", err)
	}

	for _, def := range defs {
		if strings.Contains(def.Name, "broken-link") {
			t.Errorf("symlink to non-directory should not appear in results, got: %v", def)
		}
	}
}

func TestGenerateSkillPrompts_SymlinkToMarkdownFile(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	skillsDir := filepath.Join(tmpDir, "resources", "skills")
	os.MkdirAll(skillsDir, 0o755)

	realFile := filepath.Join(tmpDir, "real-skill.md")
	os.WriteFile(realFile, []byte("---\nname: symlink-md\n---\n"), 0o644)

	linkPath := filepath.Join(skillsDir, "symlink-md.md")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	defs, err := generateSkillPrompts()
	if err != nil {
		t.Fatalf("generateSkillPrompts failed: %v", err)
	}

	var found bool
	for _, def := range defs {
		if def.Name == "SKILL-symlink-md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SKILL-symlink-md in results, got: %v", defs)
	}
}

func TestGenerateSkillPrompts_DirectoryWithoutSKILL(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	skillsDir := filepath.Join(tmpDir, "resources", "skills")
	skillDir := filepath.Join(skillsDir, "no-skill-md")
	os.MkdirAll(skillDir, 0o755)

	os.WriteFile(filepath.Join(skillDir, "README.md"), []byte("no SKILL.md here"), 0o644)

	defs, err := generateSkillPrompts()
	if err != nil {
		t.Fatalf("generateSkillPrompts failed: %v", err)
	}

	for _, def := range defs {
		if strings.Contains(def.Name, "no-skill-md") {
			t.Errorf("directory without SKILL.md should not appear in results, got: %v", def)
		}
	}
}

func TestGenerateSkillPrompts_SymlinkedDirectoryWithoutSKILL(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	skillsDir := filepath.Join(tmpDir, "resources", "skills")
	os.MkdirAll(skillsDir, 0o755)

	realSkillDir := filepath.Join(tmpDir, "real-skills", "symlink-no-skill")
	os.MkdirAll(realSkillDir, 0o755)
	os.WriteFile(filepath.Join(realSkillDir, "README.md"), []byte("no SKILL.md here"), 0o644)

	linkPath := filepath.Join(skillsDir, "symlink-no-skill")
	if err := os.Symlink(realSkillDir, linkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	defs, err := generateSkillPrompts()
	if err != nil {
		t.Fatalf("generateSkillPrompts failed: %v", err)
	}

	for _, def := range defs {
		if strings.Contains(def.Name, "symlink-no-skill") {
			t.Errorf("symlinked directory without SKILL.md should not appear in results, got: %v", def)
		}
	}
}
