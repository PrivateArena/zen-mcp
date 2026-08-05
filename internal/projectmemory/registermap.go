package projectmemory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/jang/zen-mcp/internal/logfilter"
	"github.com/jang/zen-mcp/internal/mcpcfg"
)

// RegisterProjectInMap ports registerProjectInMap: records a project in the
// global map.json (only when it has a .zenmcp dir), stamping lastVisited.
func RegisterProjectInMap(projectPath string, dependencies []string) {
	defer func() {
		if r := recover(); r != nil {
			logfilter.Info("[ProjectMemory] Failed to register project in map: " + stringifyPanic(r))
		}
	}()
	zenDir := filepath.Join(projectPath, ".zenmcp")
	if _, err := os.Stat(zenDir); err != nil {
		return
	}

	mapFile := mcpcfg.MapFilePath()
	mapData := map[string]any{}
	if raw, err := os.ReadFile(mapFile); err == nil {
		_ = json.Unmarshal(raw, &mapData)
	}

	entry := map[string]any{}
	if existing, ok := mapData[projectPath].(map[string]any); ok {
		entry = existing
	}
	entry["lastVisited"] = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if dependencies != nil {
		entry["dependencies"] = dependencies
	}

	newMap := map[string]any{projectPath: entry}
	for k, v := range mapData {
		if k != projectPath {
			newMap[k] = v
		}
	}

	data, err := json.MarshalIndent(newMap, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(mapFile, data, 0o644); err != nil {
		logfilter.Info("[ProjectMemory] Failed to register project in map: " + err.Error())
	}
}

func stringifyPanic(r any) string {
	if err, ok := r.(error); ok {
		return err.Error()
	}
	return "unknown error"
}
