package mcpcfg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

type OutputFormat string

const (
	FormatRaw  OutputFormat = "raw"
	FormatJSON OutputFormat = "json"
	FormatMD   OutputFormat = "md"
)

type ToolConfig struct {
	Timeout int          `json:"timeout"`
	Format  OutputFormat `json:"format"`
}

type BlacklistEntry struct {
	Match      string `json:"match"`
	IsRegex    bool   `json:"is_regex,omitempty"`
	MaxLines   int    `json:"max_lines,omitempty"`
	DropOutput bool   `json:"drop_output,omitempty"`
	Label      string `json:"label,omitempty"`
}

type TokenOptimizationConfig struct {
	Enabled              *bool  `json:"enabled,omitempty"`
	UltraCompact         *bool  `json:"ultraCompact,omitempty"`
	MaxChainedLength     *int   `json:"maxChainedLength,omitempty"`
	DeduplicateThreshold *int   `json:"deduplicateThreshold,omitempty"`
	ProfilesPath         string `json:"profilesPath,omitempty"`
}

type SandboxLanguage struct {
	Ext    string   `json:"ext"`
	Runner string   `json:"runner"`
	Args   []string `json:"args,omitempty"`
}

type SandboxConfig struct {
	TimeoutMs         int                        `json:"timeoutMs"`
	ActivityTimeoutMs int                        `json:"activityTimeoutMs"`
	Languages         map[string]SandboxLanguage `json:"languages"`
}

type PromptMemoryConfig struct {
	Enabled      *bool    `json:"enabled,omitempty"`
	Limit        *int     `json:"limit,omitempty"`
	TimeoutMs    *int     `json:"timeoutMs,omitempty"`
	ExcludeTypes []string `json:"excludeTypes,omitempty"`
}

type PromptFeatureConfig struct {
	Enabled                *bool `json:"enabled,omitempty"`
	AutoWorkspaceRoot      *bool `json:"auto_workspace_root,omitempty"`
	AutoSetWorkspaceRoot   *bool `json:"auto_set_workspace_root,omitempty"`
	AutoProjectScopes      *bool `json:"auto_project_scopes,omitempty"`
	AutoProjectScopesArray *bool `json:"auto_project_scopes_array,omitempty"`
	AutoProjectScopesLimit *int  `json:"auto_project_scopes_limit,omitempty"`
	MaxSkills              *int  `json:"max_skills,omitempty"`
	SkillStatic            *bool `json:"skill_static,omitempty"`
}

type WikiFilter struct {
	WhitelistUsername []string `json:"whitelist_username,omitempty"`
	MinChar           int      `json:"min_char,omitempty"`
	BlacklistWord     []string `json:"blacklist_word,omitempty"`
	Engine            string   `json:"engine,omitempty"`
}

type WikiConfig struct {
	Domains map[string]WikiFilter `json:"domains"`
}

type ZenConfig struct {
	DaemonPort                   int                        `json:"daemonPort"`
	McpPort                      int                        `json:"mcpPort"`
	CliPort                      int                        `json:"cliPort"`
	ProxyPort                    int                        `json:"proxyPort"`
	FirefoxBridgePort            int                        `json:"firefoxBridgePort"`
	ZenCapPort                   int                        `json:"zenCapPort"`
	CdpPort                      int                        `json:"cdpPort"`
	WhiteboardPort               int                        `json:"whiteboardPort"`
	WikiPort                     int                        `json:"wikiPort"`
	Host                         string                     `json:"host"`
	McpTimeoutMs                 int                        `json:"mcpTimeoutMs"`
	YtDlpCookieBank              string                     `json:"yt_dlp_cookie_bank"`
	YtDlpPath                    string                     `json:"yt_dlp_path"`
	YtDlpSubLang                 string                     `json:"yt_dlp_sub_lang"`
	YtDlpSubFormat               string                     `json:"yt_dlp_sub_format"`
	YtDlpExtractorArgs           string                     `json:"yt_dlp_extractor_args"`
	ShellOutputBlacklist         []BlacklistEntry           `json:"shell_output_blacklist,omitempty"`
	TokenOptimization            TokenOptimizationConfig    `json:"token_optimization"`
	BypassTools                  []string                   `json:"bypassTools,omitempty"`
	Sandbox                      SandboxConfig              `json:"sandbox"`
	ToolConfigs                  map[string]json.RawMessage `json:"toolConfigs,omitempty"`
	ToolTimeouts                 map[string]int             `json:"toolTimeouts,omitempty"`
	CodegraphIgnore              []string                   `json:"codegraph_ignore,omitempty"`
	CodegraphSkipEmbeddings      bool                       `json:"codegraph_skip_embeddings"`
	CodegraphDumpDir             string                     `json:"codegraph_dump_dir,omitempty"`
	CodegraphMermaidAlpha        bool                       `json:"codegraph_mermaid_alpha"`
	CodegraphMarkdownFiles       bool                       `json:"codegraph_markdown_files"`
	CodegraphMarkdownFulldump    bool                       `json:"codegraph_markdown_fulldump"`
	CodegraphWatcher             bool                       `json:"codegraph_watcher,omitempty"`
	CodegraphWatcherDebounceMs   int                        `json:"codegraph_watcher_debounce_ms,omitempty"`
	ToolSuggestionsEnabled       bool                       `json:"tool_suggestions_enabled"`
	ToolSuggestionStyle          string                     `json:"tool_suggestion_style"`
	LogLevel                     string                     `json:"log_level"`
	GatekeeperEnabled            bool                       `json:"gatekeeper_enabled"`
	GatekeeperInteractive        bool                       `json:"gatekeeper_interactive"`
	GatekeeperInteractiveAuto    string                     `json:"gatekeeper_interactive_auto"`
	GatekeeperInteractiveTimeout int                        `json:"gatekeeper_interactive_timeout"`
	GatekeeperRemember           bool                       `json:"gatekeeper_remember"`
	GatekeeperRememberPaths      []string                   `json:"gatekeeper_remember_paths,omitempty"`
	PromptMemoryContext          PromptMemoryConfig         `json:"prompt_memory_context"`
	PromptFeatures               PromptFeatureConfig        `json:"prompt_features"`
	DefaultWorkspaceRoot         string                     `json:"default_workspace_root,omitempty"`
	TelemetryEnabled             bool                       `json:"telemetry_enabled"`
	ChatFileThresholdKb          int                        `json:"chat_file_threshold_kb"`
	ChatOutputPath               *string                    `json:"chat_output_path,omitempty"`
	EnabledTools                 map[string]bool            `json:"enabled_tools,omitempty"`
}

func defaultConfig() ZenConfig {
	return ZenConfig{
		DaemonPort:         31337,
		McpPort:            3001,
		CliPort:            2999,
		ProxyPort:          31313,
		FirefoxBridgePort:  9999,
		ZenCapPort:         4444,
		CdpPort:            9222,
		WhiteboardPort:     3033,
		WikiPort:           3005,
		Host:               "127.0.0.1",
		McpTimeoutMs:       60000,
		YtDlpCookieBank:    "firefox:/media/jang/home/PortableApp/firefox/profile/",
		YtDlpPath:          "/home/jang/.local/bin/yt-dlp",
		YtDlpSubLang:       "en.*,en-US,en-GB,en-orig",
		YtDlpSubFormat:     "json3/vtt/best",
		YtDlpExtractorArgs: "youtube:player_client=tv,android,web",
		BypassTools:        []string{"session", "memory", "workspace", "think", "run"},
		TokenOptimization: TokenOptimizationConfig{
			Enabled:              boolPtr(true),
			UltraCompact:         boolPtr(false),
			MaxChainedLength:     intPtr(51200),
			DeduplicateThreshold: intPtr(3),
			ProfilesPath:         "token-profiles.json",
		},
		Sandbox: SandboxConfig{
			TimeoutMs:         120000,
			ActivityTimeoutMs: 30000,
			Languages: map[string]SandboxLanguage{
				"python":     {Ext: ".py", Runner: "python3"},
				"go":         {Ext: ".go", Runner: "go", Args: []string{"run"}},
				"node":       {Ext: ".js", Runner: "node"},
				"bash":       {Ext: ".sh", Runner: "bash"},
				"typescript": {Ext: ".ts", Runner: "tsx"},
			},
		},
		ToolConfigs: toolConfigDefaults(),
		ToolTimeouts: map[string]int{
			"browser": 120000, "colab": 120000, "shell": 60000, "run": 60000,
			"codegraph": 60000, "server": 90000, "capture": 60000,
			"memory": 30000, "memory_isolate": 30000, "memory_shared": 30000,
			"skills": 30000, "workspace": 30000, "think": 30000, "session": 30000,
		},
		CodegraphSkipEmbeddings:      true,
		CodegraphMermaidAlpha:        false,
		CodegraphMarkdownFiles:       true,
		CodegraphMarkdownFulldump:    false,
		ToolSuggestionsEnabled:       true,
		ToolSuggestionStyle:          "full",
		LogLevel:                     "debug",
		GatekeeperEnabled:            true,
		GatekeeperInteractive:        true,
		GatekeeperInteractiveAuto:    "reject",
		GatekeeperInteractiveTimeout: 60000,
		GatekeeperRemember:           true,
		PromptMemoryContext: PromptMemoryConfig{
			Enabled:      boolPtr(true),
			Limit:        intPtr(3),
			TimeoutMs:    intPtr(200),
			ExcludeTypes: []string{"shell", "git"},
		},
		PromptFeatures: PromptFeatureConfig{
			Enabled:                boolPtr(true),
			AutoWorkspaceRoot:      boolPtr(true),
			AutoSetWorkspaceRoot:   boolPtr(false),
			AutoProjectScopes:      boolPtr(true),
			AutoProjectScopesArray: boolPtr(false),
			AutoProjectScopesLimit: intPtr(5),
			MaxSkills:              intPtr(3),
			SkillStatic:            boolPtr(true),
		},
		TelemetryEnabled:    true,
		ChatFileThresholdKb: 200,
	}
}

func toolConfigDefaults() map[string]json.RawMessage {
	specs := map[string]ToolConfig{
		"browser":        {Timeout: 120000, Format: FormatRaw},
		"colab":          {Timeout: 120000, Format: FormatRaw},
		"shell":          {Timeout: 60000, Format: FormatRaw},
		"run":            {Timeout: 60000, Format: FormatRaw},
		"codegraph":      {Timeout: 60000, Format: FormatMD},
		"memory":         {Timeout: 30000, Format: FormatMD},
		"memory_isolate": {Timeout: 30000, Format: FormatMD},
		"memory_shared":  {Timeout: 30000, Format: FormatMD},
		"capture":        {Timeout: 60000, Format: FormatRaw},
		"think":          {Timeout: 30000, Format: FormatMD},
		"skills":         {Timeout: 30000, Format: FormatMD},
		"workspace":      {Timeout: 30000, Format: FormatRaw},
		"server":         {Timeout: 90000, Format: FormatRaw},
		"session":        {Timeout: 30000, Format: FormatRaw},
	}
	out := make(map[string]json.RawMessage, len(specs))
	for name, tc := range specs {
		b, err := json.Marshal(tc)
		if err != nil {
			continue
		}
		out[name] = b
	}
	return out
}

var Config atomic.Pointer[ZenConfig]

func init() {
	cfg := defaultConfig()
	Config.Store(&cfg)
}

func Get() *ZenConfig {
	return Config.Load()
}

func GetToolConfig(toolName string) ToolConfig {
	def := ToolConfig{Timeout: 60000, Format: FormatRaw}
	if c := Get(); c != nil && c.McpTimeoutMs > 0 {
		def.Timeout = c.McpTimeoutMs
	}
	c := Get()
	if c == nil {
		return def
	}
	if raw, ok := c.ToolConfigs[toolName]; ok {
		var tc ToolConfig
		if err := json.Unmarshal(raw, &tc); err == nil {
			if tc.Timeout > 0 {
				def.Timeout = tc.Timeout
			}
			if tc.Format != "" {
				def.Format = tc.Format
			}
			return def
		}
		var num int
		if err := json.Unmarshal(raw, &num); err == nil {
			def.Timeout = num
			return def
		}
	}
	if t, ok := c.ToolTimeouts[toolName]; ok {
		def.Timeout = t
	}
	return def
}

func FirefoxBridgeURL() string {
	c := Get()
	return "http://" + c.Host + ":" + strconv.Itoa(c.FirefoxBridgePort)
}

func ZenCapURL() string {
	c := Get()
	return "http://" + c.Host + ":" + strconv.Itoa(c.ZenCapPort)
}

func Load() error {
	def := defaultConfig()
	user, err := readJSONMap(ConfigFilePath())
	cfg := def
	if err == nil && user != nil {
		cfg, err = mergeConfig(def, user)
	}
	if err != nil {
		Config.Store(&def)
		return err
	}
	Config.Store(&cfg)
	return nil
}

func readJSONMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func mergeConfig(def ZenConfig, user map[string]any) (ZenConfig, error) {
	defBytes, _ := json.Marshal(def)
	var defMap map[string]any
	_ = json.Unmarshal(defBytes, &defMap)

	merged := copyMap(defMap)
	for k, v := range user {
		merged[k] = v
	}
	for _, key := range []string{"token_optimization", "sandbox", "toolConfigs", "toolTimeouts", "prompt_features", "prompt_memory_context", "enabled_tools"} {
		if uv, ok := user[key].(map[string]any); ok {
			dv, _ := defMap[key].(map[string]any)
			merged[key] = mergeNested(dv, uv)
		} else if uvRaw, ok := user[key]; ok && uvRaw == nil {
			merged[key] = defMap[key]
		}
	}
	if v, ok := merged["default_workspace_root"]; ok && v == nil {
		merged["default_workspace_root"] = defMap["default_workspace_root"]
	}

	coercePorts(merged)

	outBytes, err := json.Marshal(merged)
	if err != nil {
		return def, err
	}
	var out ZenConfig
	if err := json.Unmarshal(outBytes, &out); err != nil {
		return def, err
	}
	if out.ToolConfigs == nil {
		out.ToolConfigs = def.ToolConfigs
	}
	if out.ToolTimeouts == nil {
		out.ToolTimeouts = def.ToolTimeouts
	}
	if out.BypassTools == nil {
		out.BypassTools = def.BypassTools
	}
	return out, nil
}

func copyMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func mergeNested(def, user map[string]any) map[string]any {
	out := copyMap(def)
	if user == nil {
		return out
	}
	for k, v := range user {
		if dv, ok := out[k].(map[string]any); ok {
			if uv, ok := v.(map[string]any); ok {
				out[k] = mergeNested(dv, uv)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func coercePorts(m map[string]any) {
	for _, key := range []string{"daemonPort", "mcpPort", "cliPort", "proxyPort", "firefoxBridgePort", "zenCapPort", "cdpPort", "whiteboardPort", "wikiPort"} {
		if s, ok := m[key].(string); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				m[key] = n
			}
		}
	}
}

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

func WatchConfig(reload func()) func() {
	path := ConfigFilePath()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return func() {}
	}
	if err := watcher.Add(filepath.Dir(path)); err != nil {
		_ = watcher.Close()
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer watcher.Close()
		var timer *time.Timer
		var timerC <-chan time.Time
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if ev.Name == path && ev.Op&fsnotify.Chmod == 0 {
					if timer != nil {
						timer.Stop()
					}
					timer = time.NewTimer(300 * time.Millisecond)
					timerC = timer.C
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				_ = err
			case <-timerC:
				timerC = nil
				if reload != nil {
					reload()
				}
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}
