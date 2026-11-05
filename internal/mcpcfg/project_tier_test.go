package mcpcfg

import (
	"os"
	"path/filepath"
	"testing"
)

const globalYoutubeConfig = `{
  "youtube": {
    "container": "pachoigame",
    "containerId": 20,
    "containers": {"pachoigame": 20},
    "maxComments": 500,
    "pageTimeoutMs": 45000
  },
  "log_level": "info"
}`

// withProjectTier writes the <ws>/.zenmcp/config.json overlay of a workspace and
// returns the workspace root.
func withProjectTier(t *testing.T, workspaceRoot, body string) string {
	t.Helper()
	dir := filepath.Join(workspaceRoot, ProjectDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ProjectConfigName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return workspaceRoot
}

func TestProjectConfigFilePath(t *testing.T) {
	if got := ProjectConfigFilePath(""); got != "" {
		t.Errorf("empty workspace = %q, want no path", got)
	}
	if got := ProjectConfigFilePath("/srv/proj"); got != filepath.Join("/srv/proj", ".zenmcp", "config.json") {
		t.Errorf("path = %q", got)
	}
}

// The point of the project tier: one key named, every sibling inherited.
func TestProjectTierOverridesOneKeyAndKeepsSiblings(t *testing.T) {
	withConfig(t, globalYoutubeConfig)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ws := withProjectTier(t, t.TempDir(), `{"youtube":{"container":"pachoigame2"}}`)

	yt := For(ws).Youtube
	if yt.Container != "pachoigame2" {
		t.Errorf("workspace container = %q, want pachoigame2", yt.Container)
	}
	if yt.ContainerID != 20 || yt.MaxComments != 500 || yt.PageTimeoutMs != 45000 {
		t.Errorf("siblings not inherited from the global tier: %+v", yt)
	}
	if got := Get().Youtube.Container; got != "pachoigame" {
		t.Errorf("global config was mutated by the project tier: container = %q", got)
	}
}

// A workspace with no project file is served the process-wide config unchanged.
func TestProjectTierAbsentIsTheGlobalConfig(t *testing.T) {
	withConfig(t, globalYoutubeConfig)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := For(t.TempDir()); got != Get() {
		t.Errorf("workspace without a tier returned a derived config: %+v", got.Youtube.Container)
	}
	if got := For(""); got != Get() {
		t.Error("empty workspace must return the process-wide config")
	}
}

func TestProjectTierCorruptFileFallsBackToGlobal(t *testing.T) {
	withConfig(t, globalYoutubeConfig)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ws := withProjectTier(t, t.TempDir(), `{"youtube": {"container":`)

	if got := For(ws); got != Get() {
		t.Errorf("unparsable project tier changed the effective config: %+v", got.Youtube)
	}
	if _, err := LoadFor(ws); err != nil {
		t.Errorf("LoadFor with a broken project tier: err = %v, want the file ignored", err)
	}
}

// The override is not limited to a known block: any nested key of the config can
// be re-declared by a project.
func TestProjectTierMergesAnyNestedKey(t *testing.T) {
	withConfig(t, `{
	  "bridge": {"maxConcurrent": 1, "queueTimeoutMs": 120000},
	  "pooling": {"enabled": false, "ttlMs": 60000},
	  "enabled_tools": {"capture": false}
	}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ws := withProjectTier(t, t.TempDir(), `{
	  "bridge": {"maxConcurrent": 4},
	  "pooling": {"tools": ["shell"]},
	  "enabled_tools": {"think": false},
	  "codegraph_watcher": true
	}`)

	c := For(ws)
	if c.Bridge.MaxConcurrent != 4 || c.Bridge.QueueTimeoutMs != 120000 {
		t.Errorf("bridge tier = %+v, want maxConcurrent 4 over the global queueTimeoutMs", c.Bridge)
	}
	if c.Pooling.Enabled {
		t.Errorf("pooling.enabled = true, want the global false inherited")
	}
	if len(c.Pooling.Tools) != 1 || c.Pooling.Tools[0] != "shell" {
		t.Errorf("pooling.tools = %v, want the project value", c.Pooling.Tools)
	}
	if v, ok := c.EnabledTools["capture"]; !ok || v {
		t.Errorf("enabled_tools = %v, want the global capture:false kept", c.EnabledTools)
	}
	if v, ok := c.EnabledTools["think"]; !ok || v {
		t.Errorf("enabled_tools = %v, want the project think:false added", c.EnabledTools)
	}
	if !c.CodegraphWatcher {
		t.Error("scalar key override was dropped")
	}
	if c.McpPort != Get().McpPort {
		t.Error("unrelated global value changed")
	}
}

// Editing the project file takes effect on the next call, no restart.
func TestProjectTierEditAppliesWithoutRestart(t *testing.T) {
	withConfig(t, globalYoutubeConfig)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ws := withProjectTier(t, t.TempDir(), `{"youtube":{"container":"a"}}`)
	if got := For(ws).Youtube.Container; got != "a" {
		t.Fatalf("container = %q, want a", got)
	}

	withProjectTier(t, ws, `{"youtube":{"container":"b-longer-name"}}`)
	if got := For(ws).Youtube.Container; got != "b-longer-name" {
		t.Errorf("after edit container = %q, want b-longer-name", got)
	}

	// Deleting the tier stops it applying at once.
	if err := os.Remove(filepath.Join(ws, ProjectDirName, ProjectConfigName)); err != nil {
		t.Fatal(err)
	}
	if got := For(ws); got != Get() {
		t.Errorf("deleted tier still applies: container = %q", got.Youtube.Container)
	}
}

// A live reload of config.json must not freeze the derived tiers.
func TestGlobalReloadKeepsTheProjectTierOnTop(t *testing.T) {
	withConfig(t, globalYoutubeConfig)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ws := withProjectTier(t, t.TempDir(), `{"youtube":{"container":"pachoigame2"}}`)
	if got := For(ws).Youtube.ContainerID; got != 20 {
		t.Fatalf("containerId = %d, want 20", got)
	}

	dir := withConfig(t, globalYoutubeConfig)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{
	  "youtube": {"container":"pachoigame","containerId":77,"containers":{"pachoigame":77},"maxComments":500,"pageTimeoutMs":45000},
	  "log_level": "info"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	yt := For(ws).Youtube
	if yt.Container != "pachoigame2" || yt.ContainerID != 77 {
		t.Errorf("after reload = (%q, %d), want the project container over the new global id", yt.Container, yt.ContainerID)
	}
}

// Two projects, two overrides: neither sees the other's config.
func TestProjectTiersAreIndependent(t *testing.T) {
	withConfig(t, globalYoutubeConfig)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := withProjectTier(t, t.TempDir(), `{"youtube":{"container":"chan-a"}}`)
	b := withProjectTier(t, t.TempDir(), `{"youtube":{"container":"chan-b"}}`)

	if got := For(a).Youtube.Container; got != "chan-a" {
		t.Errorf("workspace A = %q", got)
	}
	if got := For(b).Youtube.Container; got != "chan-b" {
		t.Errorf("workspace B = %q after reading A", got)
	}
	// Cached, and still distinct.
	if got := For(a).Youtube.Container; got != "chan-a" {
		t.Errorf("cached workspace A = %q", got)
	}
}

// The cache is keyed by a request-supplied path, so it is bounded.
func TestTierCacheIsBounded(t *testing.T) {
	withConfig(t, globalYoutubeConfig)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for i := 0; i < tierCacheMax+25; i++ {
		ws := withProjectTier(t, t.TempDir(), `{"youtube":{"container":"chan"}}`)
		if got := For(ws).Youtube.Container; got != "chan" {
			t.Fatalf("workspace %d = %q", i, got)
		}
	}
	tierMu.RLock()
	n := len(tiers)
	tierMu.RUnlock()
	if n > tierCacheMax {
		t.Errorf("cached tiers = %d, want at most %d", n, tierCacheMax)
	}
}

// mtime and size alone are not a content signal: a sync or backup restore puts the
// old ones back on new bytes. The tier can carry a channel identity, so it is
// stamped by content instead.
func TestProjectTierSurvivesRestoredMtime(t *testing.T) {
	withConfig(t, globalYoutubeConfig)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ws := withProjectTier(t, t.TempDir(), `{"youtube":{"container":"chan-1"}}`)
	if got := For(ws).Youtube.Container; got != "chan-1" {
		t.Fatalf("container = %q", got)
	}

	path := ProjectConfigFilePath(ws)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	before := info.ModTime()
	withProjectTier(t, ws, `{"youtube":{"container":"chan-2"}}`)
	if after, err := os.Stat(path); err != nil || after.Size() != info.Size() {
		t.Fatalf("the test lost its premise: size changed")
	}
	if err := os.Chtimes(path, before, before); err != nil {
		t.Fatal(err)
	}

	if got := For(ws).Youtube.Container; got != "chan-2" {
		t.Errorf("container = %q after a same-size edit with restored mtime, want chan-2", got)
	}
}
