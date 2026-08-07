package whiteboard

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"zen-mcp/internal/mcpcfg"
)

// SlugInfo holds the resolved whiteboard slug and title.
type SlugInfo struct {
	Slug  string
	Title string
}

// ResolveProjectSlug derives a whiteboard slug from the workspace.
func ResolveProjectSlug(workspace string) SlugInfo {
	// Step 5: alias override (checked first)
	if entry, ok := loadAliasMap()[workspace]; ok {
		if s, ok := entry.(string); ok {
			return SlugInfo{Slug: s, Title: s}
		}
		if m, ok := entry.(map[string]any); ok {
			slug, _ := m["slug"].(string)
			title, _ := m["title"].(string)
			if slug != "" {
				return SlugInfo{Slug: slug, Title: title}
			}
		}
	}

	// Step 1: git remote URL hash
	if remote := gitRemoteURL(workspace); remote != "" {
		hash := sha1.Sum([]byte(remote))
		slug := hex.EncodeToString(hash[:])[:8]
		title := slugify(filepath.Base(remote))
		if strings.HasSuffix(title, ".git") {
			title = title[:len(title)-4]
		}
		return SlugInfo{Slug: slug, Title: title}
	}

	// Step 2: package.json name
	if pkg := readJSONSafe(filepath.Join(workspace, "package.json")); pkg != nil {
		if name, ok := pkg["name"].(string); ok && name != "" {
			return SlugInfo{Slug: slugify(name), Title: name}
		}
	}

	// Step 3: Cargo.toml
	if cargo := readTextSafe(filepath.Join(workspace, "Cargo.toml")); cargo != "" {
		if m := regexpMatch(`^name\s*=\s*"([^"]+)"`, cargo); m != "" {
			return SlugInfo{Slug: slugify(m), Title: m}
		}
	}

	// Step 3: go.mod
	if goMod := readTextSafe(filepath.Join(workspace, "go.mod")); goMod != "" {
		if m := regexpMatch(`^module\s+(\S+)`, goMod); m != "" {
			name := m
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			return SlugInfo{Slug: slugify(name), Title: name}
		}
	}

	// Step 4: folder basename
	name := filepath.Base(workspace)
	return SlugInfo{Slug: slugify(name), Title: name}
}

var aliasMapCache map[string]any

func loadAliasMap() map[string]any {
	if aliasMapCache != nil {
		return aliasMapCache
	}
	aliasMapCache = map[string]any{}
	if data, err := os.ReadFile(filepath.Join(mcpcfg.ProjectRoot, "config.json")); err == nil {
		var cfg map[string]any
		if json.Unmarshal(data, &cfg) == nil {
			if aliases, ok := cfg["whiteboard_aliases"].(map[string]any); ok {
				aliasMapCache = aliases
			}
		}
	}
	return aliasMapCache
}

func gitRemoteURL(workspace string) string {
	cmd := exec.Command("git", "-C", workspace, "remote", "get-url", "origin")
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readJSONSafe(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var v map[string]any
	if json.Unmarshal(data, &v) == nil {
		return v
	}
	return nil
}

func readTextSafe(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func regexpMatch(pattern, s string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = regexp.MustCompile(`^-+|-+$`).ReplaceAllString(s, "")
	return s
}
