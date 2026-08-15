package mcpcfg

import (
	"os"
	"path/filepath"
)

var ProjectRoot string

// initializes the package
func init() {
	ProjectRoot = resolveProjectRoot()
}

// resolveProjectRoot is a helper function
func resolveProjectRoot() string {
	if env := os.Getenv("ZEN_PROJECT_ROOT"); env != "" {
		return env
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		if info, err := os.Stat(filepath.Join(dir, "config.json")); err == nil && !info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

// ConfigFilePath is a helper function
func ConfigFilePath() string {
	return filepath.Join(ProjectRoot, "config.json")
}

// WikiConfigFilePath is a helper function
func WikiConfigFilePath() string {
	return filepath.Join(ProjectRoot, "wiki.json")
}

// MapFilePath is a helper function
func MapFilePath() string {
	return filepath.Join(ProjectRoot, "map.json")
}

// PromptDir is a helper function
func PromptDir() string {
	return filepath.Join(ProjectRoot, "resources", "prompts")
}

// SkillsDir is a helper function
func SkillsDir() string {
	return filepath.Join(ProjectRoot, "resources", "skills")
}

// TelemetryDir is a helper function
func TelemetryDir() string {
	return filepath.Join(ProjectRoot, "telemetry")
}
