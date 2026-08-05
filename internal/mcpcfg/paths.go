package mcpcfg

import (
	"os"
	"path/filepath"
)

var ProjectRoot string

func init() {
	ProjectRoot = resolveProjectRoot()
}

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

func ConfigFilePath() string {
	return filepath.Join(ProjectRoot, "config.json")
}

func WikiConfigFilePath() string {
	return filepath.Join(ProjectRoot, "wiki.json")
}

func MapFilePath() string {
	return filepath.Join(ProjectRoot, "map.json")
}

func PromptDir() string {
	return filepath.Join(ProjectRoot, "resources", "prompts")
}

func SkillsDir() string {
	return filepath.Join(ProjectRoot, "resources", "skills")
}

func TelemetryDir() string {
	return filepath.Join(ProjectRoot, "telemetry")
}
