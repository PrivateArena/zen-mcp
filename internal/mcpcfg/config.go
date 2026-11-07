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

// PoolingConfig controls the opt-in tool-level job pool. When Enabled, a
// wrapped tool call that outlives ElapsedMs returns a {status,pool_id} payload
// instead of blocking forever; the pool tool long-polls the in-memory registry.
type PoolingConfig struct {
	Enabled    bool     `json:"enabled"`
	Tools      []string `json:"tools,omitempty"`
	ElapsedMs  int      `json:"elapsedMs"`
	TTLMinutes int      `json:"ttlMinutes,omitempty"`
	MaxJobs    int      `json:"maxJobs,omitempty"`
}

// BridgeConfig controls client-side access to the Firefox bridge. The bridge is
// a single-resource device (every action targets the active tab), so by default
// exactly one operation is allowed in flight, in arrival order. See
// internal/bridge/gate.go.
type BridgeConfig struct {
	// MaxConcurrent is how many bridge operations may run at the same time.
	// Values below 1 are ignored, 1 (the default) is strict one-by-one.
	MaxConcurrent int `json:"maxConcurrent,omitempty"`
	// QueueTimeoutMs bounds how long a call waits for its turn before failing
	// with "bridge busy" instead of hanging behind a wedged bridge.
	QueueTimeoutMs int `json:"queueTimeoutMs,omitempty"`
	// QueueAgentActions also puts the long-running chat/brainstorm actions through
	// the queue. Off by default because a chat can hold the browser for minutes.
	QueueAgentActions *bool `json:"queueAgentActions,omitempty"`
	// TabParallel lets calls that address a TabRegistry id (and navigations that
	// open their own background tab) run concurrently, each queued on its own tab
	// instead of on the browser. On by default; set false to force every bridge
	// call through the single global turn, e.g. against a bridge build that does
	// not report tab ids. See internal/bridge/address.go.
	TabParallel *bool `json:"tabParallel,omitempty"`
}

// YoutubeConfig is the identity-and-policy block of the youtube tool. The YouTube
// capability is a set of real page visits through the Firefox bridge, so the
// config carries what a human must decide once and an agent must never override:
// which container (i.e. which channel identity) the work happens in, the upload
// cadence, and whether OS-level interaction is permitted at all.
type YoutubeConfig struct {
	// Container is the Firefox contextual identity name; it selects the channel.
	Container string `json:"container"`
	// ContainerID is the numeric id of Container, asserted against the bridge's
	// navigate reply so a mis-typed name can never act as another identity.
	// Containers declares the other channels this machine may act as, as contextual
	// identity name to numeric id. A call's container param selects one; a name that
	// is not declared here (and is not Container) is refused rather than looked up,
	// because discovering an id from the live browser is how a typo becomes another
	// channel. Container/ContainerID stays the default pair, so a single-channel
	// config needs no map.
	Containers  map[string]int `json:"containers"`
	ContainerID int            `json:"containerId"`
	// MaxUploadsPerDay is the journal-enforced upload cadence.
	MaxUploadsPerDay int `json:"maxUploadsPerDay"`
	// DefaultVisibility applies when an upload names none.
	DefaultVisibility string `json:"defaultVisibility"`
	// Timezone names the zone a schedule resolves against, echoed for human
	// confirmation. Empty means the system zone.
	Timezone string `json:"timezone"`
	// MaxComments caps a comment read; 0 means read everything.
	MaxComments int `json:"maxComments"`
	// MaxScrolls is a runaway guard on the lazy-load scroll loop, not a feature
	// limit: hitting it returns partial results with truncated:true and a cursor.
	MaxScrolls int `json:"maxScrolls"`
	// AllowFocusBoundActions is the human-set escape hatch for OS-level actions
	// (trusted_*/native_*/coordinate clicks), which steal the cursor and take the
	// global bridge turn. Off by default: DOM-level interaction is the only route.
	AllowFocusBoundActions bool `json:"allowFocusBoundActions"`
	// PageTimeoutMs bounds a single wait on a page.
	PageTimeoutMs int `json:"pageTimeoutMs"`
	// WizardTimeoutMs bounds the upload wizard walk.
	WizardTimeoutMs int `json:"wizardTimeoutMs"`
	// ProcessTimeoutMs bounds processing of an uploaded video.
	ProcessTimeoutMs int `json:"processTimeoutMs"`
	// MaxUploadBytes rejects a file the wizard would only fail on later.
	MaxUploadBytes int64 `json:"maxUploadBytes"`
	// MaxDiagBytes caps the page markdown captured into a diagnosis bundle.
	MaxDiagBytes int `json:"maxDiagBytes"`
}

type ZenConfig struct {
	DaemonPort                   int                        `json:"daemonPort"`
	McpPort                      int                        `json:"mcpPort"`
	CliPort                      int                        `json:"cliPort"`
	ProxyPort                    int                        `json:"proxyPort"`
	FirefoxBridgePort            int                        `json:"firefoxBridgePort"`
	Bridge                       BridgeConfig               `json:"bridge"`
	Youtube                      YoutubeConfig              `json:"youtube"`
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
	Pooling                      PoolingConfig              `json:"pooling"`
	CodegraphIgnore              []string                   `json:"codegraph_ignore,omitempty"`
	CodegraphSkipEmbeddings      bool                       `json:"codegraph_skip_embeddings"`
	CodegraphDumpDir             string                     `json:"codegraph_dump_dir,omitempty"`
	CodegraphMermaidAlpha        bool                       `json:"codegraph_mermaid_alpha"`
	CodegraphMarkdownFiles       bool                       `json:"codegraph_markdown_files"`
	CodegraphMarkdownFulldump    bool                       `json:"codegraph_markdown_fulldump"`
	CodegraphWatcher             bool                       `json:"codegraph_watcher,omitempty"`
	CodegraphWatcherDebounceMs   int                        `json:"codegraph_watcher_debounce_ms,omitempty"`
	CodegraphWatcherAutoLint     bool                       `json:"codegraph_watcher_auto_lint,omitempty"`
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
	Mcp2Cli                      bool                       `json:"mcp2cli,omitempty"`
	CliModePrefix                string                     `json:"climode_prefix,omitempty"`
	CliModeShort                 bool                       `json:"climode_short"`
	HarnessTools                 []string                   `json:"harness_tools,omitempty"`
}

// DefaultHarnessTools are the agent-harness tool names referenced by zen-mcp's own
// prompt/skill text. They have no generated CLI wrapper, so the CLI-mode transform
// (see internal/prompts/climode.go) must leave their call form alone.
var DefaultHarnessTools = []string{"subagent"}

// DefaultCliModePrefix is the CLI wrapper script prefix applied when the
// climode_prefix config is unset, empty, or whitespace-only.
const DefaultCliModePrefix = "zen-"

func defaultConfig() ZenConfig {
	return ZenConfig{
		DaemonPort:        31337,
		McpPort:           3001,
		CliPort:           2999,
		ProxyPort:         31313,
		FirefoxBridgePort: 9999,
		Bridge: BridgeConfig{
			MaxConcurrent:     1,
			QueueTimeoutMs:    120000,
			QueueAgentActions: boolPtr(false),
			TabParallel:       boolPtr(true),
		},
		Youtube: YoutubeConfig{
			// No default identity: with no container configured the youtube tool must
			// refuse rather than act as whoever the browser happens to be logged in as.
			Container:              "",
			ContainerID:            0,
			MaxUploadsPerDay:       1,
			DefaultVisibility:      "private",
			Timezone:               "",
			MaxComments:            0,
			MaxScrolls:             40,
			AllowFocusBoundActions: false,
			PageTimeoutMs:          45000,
			WizardTimeoutMs:        900000,
			ProcessTimeoutMs:       1800000,
			MaxUploadBytes:         8 << 30,
			MaxDiagBytes:           8192,
		},
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
		Pooling: PoolingConfig{
			Enabled:    false,
			Tools:      []string{"shell", "browser", "run", "ui-vision"},
			ElapsedMs:  60000,
			TTLMinutes: 60,
			MaxJobs:    256,
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
		Mcp2Cli:             false,
		CliModePrefix:       DefaultCliModePrefix,
		CliModeShort:        false,
		HarnessTools:        DefaultHarnessTools,
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

// CliModePrefixOrDefault returns the configured CLI wrapper prefix, falling
// back to the default "zen-" when unset, empty, or whitespace-only. An empty
// prefix is unsafe (trim logic would match every file), so it is never used.
func CliModePrefixOrDefault() string {
	if c := Get(); c != nil {
		if p := strings.TrimSpace(c.CliModePrefix); p != "" {
			return p
		}
	}
	return DefaultCliModePrefix
}

// HarnessToolsOrDefault returns the harness tool names the CLI-mode transform must
// skip: DefaultHarnessTools plus the configured harness_tools, normalized to trimmed
// lowercase and deduplicated. The list is additive on purpose — the merge layer
// replaces arrays wholesale, so a project tier that names its own harness tools must
// not silently re-enable rewriting of the tools zen-mcp's prompts already use.
func HarnessToolsOrDefault() []string {
	seen := make(map[string]bool, len(DefaultHarnessTools)+4)
	out := make([]string, 0, len(DefaultHarnessTools)+4)
	add := func(names []string) {
		for _, n := range names {
			n = strings.ToLower(strings.TrimSpace(n))
			if n == "" || seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
		}
	}
	add(DefaultHarnessTools)
	if c := Get(); c != nil {
		add(c.HarnessTools)
	}
	return out
}

// IsHarnessTool reports whether name belongs to the configured harness-tool set.
func IsHarnessTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	for _, n := range HarnessToolsOrDefault() {
		if n == name {
			return true
		}
	}
	return false
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

func DaemonURL() string {
	c := Get()
	return "http://" + c.Host + ":" + strconv.Itoa(c.DaemonPort)
}

func ProxyURL() string {
	c := Get()
	return "http://" + c.Host + ":" + strconv.Itoa(c.ProxyPort)
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
		globalGeneration.Add(1)
		invalidateTiers()
		return err
	}
	Config.Store(&cfg)
	// The base tier moved, so every cached workspace overlay was derived from a
	// config that no longer exists.
	globalGeneration.Add(1)
	invalidateTiers()
	return nil
}

func LoadWikiConfig() WikiConfig {
	def := WikiConfig{Domains: map[string]WikiFilter{}}
	data, err := os.ReadFile(WikiConfigFilePath())
	if err != nil {
		return def
	}
	var wc WikiConfig
	if err := json.Unmarshal(data, &wc); err != nil {
		return def
	}
	if wc.Domains == nil {
		wc.Domains = map[string]WikiFilter{}
	}
	return wc
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

// mergeConfig applies a user layer on top of def. The layer is deep-merged:
// a partial block such as {"youtube":{"container":"pachoigame2"}} keeps every
// sibling value it does not name, so a project tier only has to spell out the
// keys it actually changes. Scalars and arrays replace wholesale, and an
// explicit null keeps the value from the tier below.
func mergeConfig(def ZenConfig, user map[string]any) (ZenConfig, error) {
	defBytes, _ := json.Marshal(def)
	var defMap map[string]any
	_ = json.Unmarshal(defBytes, &defMap)

	merged := applyLayer(copyMap(defMap), user)
	return decodeConfig(def, merged)
}

func applyLayer(merged, user map[string]any) map[string]any {
	if merged == nil {
		merged = map[string]any{}
	}
	for k, v := range user {
		if v == nil {
			continue
		}
		if dv, ok := merged[k].(map[string]any); ok {
			if uv, ok := v.(map[string]any); ok {
				merged[k] = mergeNested(dv, uv)
				continue
			}
		}
		merged[k] = v
	}
	return merged
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
