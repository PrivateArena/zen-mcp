package mcpcfg

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const realConfigJSON = `{
  "daemonPort": 31337,
  "proxyPort": 31313,
  "firefoxBridgePort": 9999,
  "cliPort": 2999,
  "mcpPort": 3001,
  "zenCapPort": 4444,
  "cdpPort": 9222,
  "wikiPort": 3005,
  "whiteboardPort": 3033,
  "host": "127.0.0.1",
  "mcpTimeoutMs": 2400000,
  "tool_suggestion_style": "lite",
  "log_level": "info",
  "codegraph_watcher": false,
  "codegraph_mermaid_alpha": true,
  "telemetry_enabled": true,
  "chat_file_threshold_kb": 5,
  "enabled_tools": {
    "capture": false,
    "colab": false,
    "run": false,
    "think": false,
    "memory_isolate": false,
    "memory_shared": false
  },
  "gatekeeper_enabled": true,
  "gatekeeper_interactive": false,
  "gatekeeper_interactive_auto": "accept",
  "gatekeeper_interactive_timeout": 60000,
  "gatekeeper_remember": true,
  "gatekeeper_remember_paths": ["/dev/null", "/tmp/"],
  "codegraph_skip_embeddings": true,
  "codegraph_markdown_files": true,
  "codegraph_markdown_fulldump": false,
  "bypassTools": ["session","memory","memory_isolate","memory_shared","workspace","think","run"],
  "shell_output_blacklist": [{"match":"sfizz_render","label":"sfizz_render (noisy scheduler warnings)","max_lines":2}],
  "token_optimization": {"enabled":true,"ultraCompact":false,"maxChainedLength":51200,"deduplicateThreshold":3,"profilesPath":"token-profiles.json"},
  "sandbox": {"timeoutMs":600000,"activityTimeoutMs":600000,"languages":{
    "python":{"ext":".py","runner":"python3"},
    "go":{"ext":".go","runner":"go","args":["run"]},
    "node":{"ext":".js","runner":"node"},
    "bash":{"ext":".sh","runner":"bash"},
    "typescript":{"ext":".ts","runner":"tsx"}
  }},
  "toolConfigs": {
    "browser": {"timeout":2400000,"format":"raw"},
    "colab": {"timeout":900000,"format":"raw"},
    "shell": {"timeout":900000,"format":"raw"},
    "codegraph": {"timeout":300000,"format":"md"},
    "think": {"timeout":600000,"format":"md"}
  },
  "prompt_memory_context": {"enabled":false,"limit":3,"timeoutMs":200,"excludeTypes":["shell","git"]},
  "prompt_features": {"enabled":true,"auto_workspace_root":true,"auto_set_workspace_root":true,"auto_project_scopes":true,"auto_project_scopes_array":true,"auto_project_scopes_limit":5},
  "chat_output_path": "/tmp/zen-mcp/chat"
}`

func withConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	old := ProjectRoot
	ProjectRoot = dir
	t.Cleanup(func() { ProjectRoot = old })
	return dir
}

func TestLoadMergesDefaultsAndUserConfig(t *testing.T) {
	withConfig(t, realConfigJSON)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := Get()
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"mcpPort", c.McpPort, 3001},
		{"mcpTimeoutMs", c.McpTimeoutMs, 2400000},
		{"logLevel", c.LogLevel, "info"},
		{"suggestionStyle", c.ToolSuggestionStyle, "lite"},
		{"suggestionsEnabledDefault", c.ToolSuggestionsEnabled, true},
		{"chatFileThresholdKb", c.ChatFileThresholdKb, 5},
		{"gatekeeperInteractive", c.GatekeeperInteractive, false},
		{"gatekeeperAuto", c.GatekeeperInteractiveAuto, "accept"},
		{"mermaidAlpha", c.CodegraphMermaidAlpha, true},
		{"sandboxTimeout", c.Sandbox.TimeoutMs, 600000},
		{"defaultTimeoutFromDefault", GetToolConfig("nonexistent").Timeout, 2400000},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
	if got := GetToolConfig("browser").Timeout; got != 2400000 {
		t.Errorf("browser timeout = %d, want 2400000", got)
	}
	if got := GetToolConfig("codegraph").Format; got != FormatMD {
		t.Errorf("codegraph format = %q, want md", got)
	}
	if got := GetToolConfig("think").Timeout; got != 600000 {
		t.Errorf("think timeout = %d, want 600000", got)
	}
	if len(c.EnabledTools) != 6 || c.EnabledTools["capture"] {
		t.Errorf("enabled_tools not merged correctly: %+v", c.EnabledTools)
	}
	if c.ChatOutputPath == nil || *c.ChatOutputPath != "/tmp/zen-mcp/chat" {
		t.Errorf("chat_output_path = %v", c.ChatOutputPath)
	}
	if len(c.GatekeeperRememberPaths) != 2 {
		t.Errorf("gatekeeper_remember_paths = %v", c.GatekeeperRememberPaths)
	}
	if got := DaemonURL(); got != "http://127.0.0.1:31337" {
		t.Errorf("DaemonURL = %q", got)
	}
}

func TestLoadMissingConfigUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	old := ProjectRoot
	ProjectRoot = dir
	defer func() { ProjectRoot = old }()
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := Get()
	if c.McpPort != 3001 || c.DaemonPort != 31337 || c.LogLevel != "debug" {
		t.Errorf("defaults wrong: %+v", c)
	}
	if !(c.Sandbox.Languages["go"].Args[0] == "run") {
		t.Errorf("default sandbox go args wrong: %+v", c.Sandbox.Languages["go"])
	}
	if GetToolConfig("browser").Timeout != 120000 {
		t.Errorf("default browser timeout = %d", GetToolConfig("browser").Timeout)
	}
}

func TestLoadBrokenConfigFallsBackToDefaults(t *testing.T) {
	withConfig(t, "{ not valid json")
	if err := Load(); err == nil {
		t.Fatal("expected error for broken config")
	}
	if c := Get(); c.McpPort != 3001 {
		t.Errorf("should keep defaults, got McpPort=%d", c.McpPort)
	}
}

func TestToolSuggestionsEnabledDisableMerge(t *testing.T) {
	withConfig(t, `{"tool_suggestions_enabled":false}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c := Get(); c.ToolSuggestionsEnabled {
		t.Error("tool_suggestions_enabled should be false when explicitly disabled")
	}
}

func TestToolSuggestionsEnabledDefaultTrue(t *testing.T) {
	withConfig(t, `{"mcpPort":3001}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c := Get(); !c.ToolSuggestionsEnabled {
		t.Error("tool_suggestions_enabled should default to true")
	}
}

func TestStringPortCoercion(t *testing.T) {
	withConfig(t, `{"daemonPort":"31337","mcpPort":"3001","whiteboardPort":"3033"}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := Get()
	if c.DaemonPort != 31337 || c.McpPort != 3001 || c.WhiteboardPort != 3033 {
		t.Errorf("port coercion failed: %+v", c)
	}
}

func TestToolTimeoutsLegacyFallback(t *testing.T) {
	withConfig(t, `{"toolTimeouts":{"custom_tool":99999}}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	// default toolConfigs shadows legacy toolTimeouts for known tools
	if got := GetToolConfig("browser").Timeout; got != 120000 {
		t.Errorf("browser timeout = %d, want 120000 (toolConfigs shadow)", got)
	}
	// unknown tool falls back to toolTimeouts
	if got := GetToolConfig("custom_tool").Timeout; got != 99999 {
		t.Errorf("custom_tool timeout = %d, want 99999", got)
	}
}

func TestDeepMergePartialNested(t *testing.T) {
	withConfig(t, `{"token_optimization":{"enabled":false},"sandbox":{"languages":{"python":{"ext":".py","runner":"python3","args":["-u"]}}}}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := Get()
	if c.TokenOptimization.Enabled == nil || *c.TokenOptimization.Enabled {
		t.Errorf("token_optimization.enabled should be false")
	}
	if c.TokenOptimization.ProfilesPath != "token-profiles.json" {
		t.Errorf("profilesPath should keep default, got %q", c.TokenOptimization.ProfilesPath)
	}
	py := c.Sandbox.Languages["python"]
	if py.Ext != ".py" || py.Runner != "python3" {
		t.Errorf("python merged wrong: %+v", py)
	}
	if c.Sandbox.Languages["go"].Runner != "go" {
		t.Errorf("sandbox language go should keep default, got %+v", c.Sandbox.Languages["go"])
	}
}

var reloadCount atomic.Int32

func TestReload(t *testing.T) {
	dir := withConfig(t, `{"mcpPort":4001}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if Get().McpPort != 4001 {
		t.Fatalf("initial McpPort = %d", Get().McpPort)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"mcpPort":4002}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if Get().McpPort != 4002 {
		t.Errorf("after reload McpPort = %d, want 4002", Get().McpPort)
	}
}

func TestWatchConfigFiresReload(t *testing.T) {
	dir := withConfig(t, `{"mcpPort":4101}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	reloadCount.Store(0)
	stop := WatchConfig(func() {
		reloadCount.Add(1)
		if err := Load(); err != nil {
			t.Errorf("reload Load: %v", err)
		}
	})
	defer stop()

	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"mcpPort":4102}`), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if reloadCount.Load() > 0 && Get().McpPort == 4102 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("watch reload did not fire; count=%d mcpPort=%d", reloadCount.Load(), Get().McpPort)
}

func TestWatchConfigIgnoreChmod(t *testing.T) {
	dir := withConfig(t, `{"mcpPort":4201}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	reloadCount.Store(0)
	stop := WatchConfig(func() { reloadCount.Add(1) })
	defer stop()

	path := filepath.Join(dir, "config.json")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond)
	if reloadCount.Load() != 0 {
		t.Errorf("chmod should not trigger reload, count=%d", reloadCount.Load())
	}
}

func TestLoadWikiConfig(t *testing.T) {
	dir := t.TempDir()
	old := ProjectRoot
	ProjectRoot = dir
	defer func() { ProjectRoot = old }()
	if err := os.WriteFile(filepath.Join(dir, "wiki.json"), []byte(`{"domains":{"en.wikipedia.org":{"engine":"mediawiki","min_char":500}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	wc := LoadWikiConfig()
	f, ok := wc.Domains["en.wikipedia.org"]
	if !ok || f.MinChar != 500 || f.Engine != "mediawiki" {
		t.Errorf("wiki config wrong: %+v", wc)
	}
	// missing wiki.json -> empty domains
	os.Remove(filepath.Join(dir, "wiki.json"))
	if wc := LoadWikiConfig(); wc.Domains == nil || len(wc.Domains) != 0 {
		t.Errorf("missing wiki.json should be empty, got %+v", wc.Domains)
	}
}

func TestConfigFilePath(t *testing.T) {
	dir := withConfig(t, `{}`)
	if got := ConfigFilePath(); !strings.HasPrefix(got, dir) {
		t.Errorf("ConfigFilePath = %q, want prefix %q", got, dir)
	}
	if got := PromptDir(); got != filepath.Join(dir, "resources", "prompts") {
		t.Errorf("PromptDir = %q", got)
	}
}

func TestPoolingConfigDefaults(t *testing.T) {
	withConfig(t, `{}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := Get().Pooling
	if c.Enabled {
		t.Error("pooling should default to disabled")
	}
	if len(c.Tools) != 4 || c.Tools[0] != "shell" {
		t.Errorf("pooling tools default wrong: %v", c.Tools)
	}
	if c.ElapsedMs != 60000 {
		t.Errorf("elapsedMs default = %d, want 60000", c.ElapsedMs)
	}
	if c.TTLMinutes != 60 {
		t.Errorf("ttlMinutes default = %d, want 60", c.TTLMinutes)
	}
	if c.MaxJobs != 256 {
		t.Errorf("maxJobs default = %d, want 256", c.MaxJobs)
	}
}

func TestPoolingConfigPartialMerge(t *testing.T) {
	// Partial override: tools replaced, elapsedMs/ttl/max preserved from defaults.
	withConfig(t, `{"pooling":{"enabled":true,"tools":["shell","run"]}}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := Get().Pooling
	if !c.Enabled {
		t.Error("pooling should be enabled")
	}
	if len(c.Tools) != 2 || c.Tools[0] != "shell" || c.Tools[1] != "run" {
		t.Errorf("pooling tools should be replaced, got %v", c.Tools)
	}
	if c.ElapsedMs != 60000 {
		t.Errorf("elapsedMs should keep default, got %d", c.ElapsedMs)
	}
	if c.TTLMinutes != 60 || c.MaxJobs != 256 {
		t.Errorf("ttl/max should keep defaults, got %d/%d", c.TTLMinutes, c.MaxJobs)
	}
}

func TestPoolingConfigFullOverride(t *testing.T) {
	withConfig(t, `{"pooling":{"enabled":true,"tools":["browser"],"elapsedMs":5000,"ttlMinutes":5,"maxJobs":8}}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := Get().Pooling
	if !c.Enabled || c.ElapsedMs != 5000 || c.TTLMinutes != 5 || c.MaxJobs != 8 {
		t.Errorf("full pooling override wrong: %+v", c)
	}
	if len(c.Tools) != 1 || c.Tools[0] != "browser" {
		t.Errorf("pooling tools wrong: %v", c.Tools)
	}
}

func TestCliModeConfigMerge(t *testing.T) {
	withConfig(t, `{"climode_prefix":"zn-","climode_short":true}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := Get()
	if c.CliModePrefix != "zn-" {
		t.Errorf("climode_prefix = %q, want zn-", c.CliModePrefix)
	}
	if !c.CliModeShort {
		t.Error("climode_short should be true when configured")
	}
}

func TestCliModeConfigDefaults(t *testing.T) {
	withConfig(t, `{}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := Get()
	if c.CliModePrefix != DefaultCliModePrefix {
		t.Errorf("default climode_prefix = %q, want %q", c.CliModePrefix, DefaultCliModePrefix)
	}
	if c.CliModeShort {
		t.Error("default climode_short should be false")
	}
}

func TestCliModePrefixOrDefault(t *testing.T) {
	withConfig(t, `{}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := CliModePrefixOrDefault(); got != DefaultCliModePrefix {
		t.Errorf("default prefix = %q, want %q", got, DefaultCliModePrefix)
	}

	// Explicitly empty prefixes are unsafe (empty match deletes every file),
	// so they must fall back to the default rather than being used.
	withConfig(t, `{"climode_prefix":""}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := CliModePrefixOrDefault(); got != DefaultCliModePrefix {
		t.Errorf("empty prefix should fall back to %q, got %q", DefaultCliModePrefix, got)
	}
	if got := Get().CliModePrefix; got != "" {
		t.Errorf("raw config field should stay empty after merge, got %q", got)
	}

	withConfig(t, `{"climode_prefix":"  zn-  "}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := CliModePrefixOrDefault(); got != "zn-" {
		t.Errorf("whitespace-trimmed prefix = %q, want zn-", got)
	}

	withConfig(t, `{"climode_prefix":"zn-"}`)
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := CliModePrefixOrDefault(); got != "zn-" {
		t.Errorf("custom prefix = %q, want zn-", got)
	}
}
