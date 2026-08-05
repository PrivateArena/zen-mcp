package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSkillContent_KnowledgeBase(t *testing.T) {
	projectRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "config.json")); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			t.Fatal("could not find project root")
		}
		projectRoot = parent
	}

	skillPath := filepath.Join(projectRoot, "resources", "skills", "ublock.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("failed to read ublock.md: %v", err)
	}
	skillDir := filepath.Join(projectRoot, "resources", "skills")

	resolved := ResolveSkillContent(string(content), skillDir, "ublock")

	if len(resolved.FileReferences) == 0 {
		t.Error("Expected file references from ublock_kb, got none")
	}

	foundKB := false
	for _, ref := range resolved.FileReferences {
		if strings.Contains(ref.RelativePath, "cosmetic.md") ||
			strings.Contains(ref.RelativePath, "scriptlets.md") ||
			strings.Contains(ref.RelativePath, "static-filtering.md") {
			foundKB = true
			break
		}
	}
	if !foundKB {
		t.Error("Expected at least one reference from ublock_kb (cosmetic.md, scriptlets.md, or static-filtering.md)")
	}

	if !strings.Contains(resolved.Enriched, "## Referenced Files") {
		t.Error("Expected '## Referenced Files' section in enriched content")
	}

	if !strings.Contains(resolved.Enriched, "Knowledge base files") {
		t.Error("Expected 'Knowledge base files' label")
	}
}

func TestResolveSkillContent_SymlinkedDirectorySkill(t *testing.T) {
	projectRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "config.json")); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			t.Fatal("could not find project root")
		}
		projectRoot = parent
	}

	skillDir := filepath.Join(projectRoot, "resources", "skills", "skill-craft")
	skillFile := filepath.Join(skillDir, "SKILL.md")
	content, err := os.ReadFile(skillFile)
	if err != nil {
		t.Skipf("skill-craft skill not found: %v", err)
	}

	resolved := ResolveSkillContent(string(content), skillDir, "skill-craft")

	if len(resolved.FileReferences) == 0 {
		t.Fatal("Expected file references from skill-craft, got none")
	}

	foundGLOSSARY := false
	for _, ref := range resolved.FileReferences {
		if ref.RelativePath == "GLOSSARY.md" {
			foundGLOSSARY = true
			break
		}
	}
	if !foundGLOSSARY {
		t.Error("Expected GLOSSARY.md in skill-craft references")
	}

	if !strings.Contains(resolved.Enriched, "## Referenced Files") {
		t.Error("Expected '## Referenced Files' section for symlinked directory skill")
	}

	if !strings.Contains(resolved.Enriched, "Bundled skill files") {
		t.Error("Expected 'Bundled skill files' label")
	}
}

func TestLoadSkills_FindsExpectedSkills(t *testing.T) {
	entries, err := LoadSkills()
	if err != nil {
		t.Fatalf("LoadSkills failed: %v", err)
	}

	ids := make(map[string]bool)
	for _, e := range entries {
		ids[e.ID] = true
	}

	if !ids["grill-me"] {
		t.Error("Expected skill 'grill-me' to be loaded")
	}
	if !ids["daw-api"] {
		t.Error("Expected skill 'daw-api' to be loaded")
	}
	if !ids["ublock"] {
		t.Error("Expected skill 'ublock' to be loaded")
	}
}

func TestFindSkillByID_ExpectedIDs(t *testing.T) {
	tests := []struct {
		id        string
		expectErr bool
	}{
		{"grill-me", false},
		{"daw-api", false},
		{"nonexistent-skill", true},
		{"ublock", false},
	}

	for _, tt := range tests {
		_, err := FindSkillByID(tt.id)
		if tt.expectErr && err == nil {
			t.Errorf("FindSkillByID(%q) expected error, got nil", tt.id)
		}
		if !tt.expectErr && err != nil {
			t.Errorf("FindSkillByID(%q) unexpected error: %v", tt.id, err)
		}
	}
}
