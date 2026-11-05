package mcpcfg

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"zen-mcp/internal/logfilter"
)

// The workspace tier is the second layer of configuration: config.json in the
// zen-mcp folder is the machine-wide truth, <workspace>/.zenmcp/config.json is a
// project's local overlay, deep-merged on top of it for that workspace only.
// It is the same shape as a project-local .pi/ folder: a project declares the
// handful of keys it wants changed and inherits everything else.
const (
	// ProjectDirName is the per-project state folder inside a workspace root.
	ProjectDirName = ".zenmcp"
	// ProjectConfigName is the project tier file, next to the memory and
	// codegraph state that folder already holds.
	ProjectConfigName = "config.json"
)

// ProjectConfigFilePath returns the project tier path for a workspace root, or
// "" when no workspace is known.
func ProjectConfigFilePath(workspaceRoot string) string {
	if strings.TrimSpace(workspaceRoot) == "" {
		return ""
	}
	return filepath.Join(workspaceRoot, ProjectDirName, ProjectConfigName)
}

// ReadProjectConfigRaw returns the parsed project tier, or nil when there is no
// readable one. A broken file is reported as absent rather than fatal: a typo in
// one project must not degrade the machine-wide config for every other project.
func ReadProjectConfigRaw(workspaceRoot string) map[string]any {
	path := ProjectConfigFilePath(workspaceRoot)
	if path == "" {
		return nil
	}
	m, _, err := readProjectTier(path)
	if err != nil {
		logfilter.Debugf("[Config] Failed to parse %s: %v", path, err)
		return nil
	}
	return m
}

// readProjectTier reads the project tier and stamps its exact bytes. A missing
// file is not an error and carries no stamp, because "this project overrides
// nothing" is the normal case and must not cost a log line per call.
func readProjectTier(path string) (map[string]any, stamp, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, stamp{}, nil
		}
		return nil, stamp{}, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, contentStamp(data), err
	}
	return m, contentStamp(data), nil
}

// LoadFor returns the effective config for one workspace: the process-wide config
// (defaults + config.json) with <ws>/.zenmcp/config.json deep-merged on top. The
// result is a fresh value - callers must never write it back into the
// process-wide config, which several of them already mutate for tests.
func LoadFor(workspaceRoot string) (*ZenConfig, error) {
	base := Get()
	project := ReadProjectConfigRaw(workspaceRoot)
	if len(project) == 0 {
		return base, nil
	}
	cfg, err := mergeConfig(*base, project)
	if err != nil {
		return base, err
	}
	return &cfg, nil
}

// For is the per-call accessor: the workspace's effective config, cached until
// either tier's content changes on disk or the process-wide config is reloaded. A
// workspace without a project tier gets the process-wide config pointer back, so
// an unset workspace behaves exactly as it did before the tier existed. Callers
// that write fields of Get() in place instead of calling Load must call
// ResetWorkspaceTiers, so a cached overlay cannot outlive the value it merged onto.
func For(workspaceRoot string) *ZenConfig {
	base := Get()
	path := ProjectConfigFilePath(workspaceRoot)
	if path == "" {
		return base
	}

	key := workspaceKey(workspaceRoot)
	project, pStamp, err := readProjectTier(path)
	gStamp := fileStamp(ConfigFilePath())
	gen := globalGeneration.Load()

	tierMu.RLock()
	entry, cached := tiers[key]
	tierMu.RUnlock()
	if cached && entry.current(base, gen, pStamp, gStamp) {
		entry.used.Store(time.Now().UnixNano())
		return entry.cfg
	}

	if err != nil {
		// A tier this cannot read is not the tool call's problem: serve the
		// machine-wide config, and remember that against the same stamp so a
		// permanently unreadable path is reported once instead of every call.
		logfilter.Debugf("[Config] Workspace tier %s ignored: %v", path, err)
		return rememberTier(newTierEntry(key, base, base, pStamp, gStamp, gen))
	}
	if len(project) == 0 {
		// No tier, or an emptied one: drop any cached overlay so it stops
		// applying at once.
		if cached {
			tierMu.Lock()
			delete(tiers, key)
			tierMu.Unlock()
		}
		return base
	}

	cfg, err := mergeConfig(*base, project)
	if err != nil {
		logfilter.Debugf("[Config] Workspace tier %s ignored: %v", path, err)
		return rememberTier(newTierEntry(key, base, base, pStamp, gStamp, gen))
	}
	return rememberTier(newTierEntry(key, base, &cfg, pStamp, gStamp, gen))
}

// tierCacheMax bounds the cache: a long-lived server would otherwise accumulate
// one derived config per distinct workspace path it was ever called for, and that
// path comes from the request.
const (
	tierCacheMax  = 64
	tierCacheIdle = 30 * time.Minute
)

func newTierEntry(key string, base, cfg *ZenConfig, project, global stamp, gen uint64) *tierEntry {
	e := &tierEntry{key: key, cfg: cfg, base: base, project: project, global: global, gen: gen}
	e.used.Store(time.Now().UnixNano())
	return e
}

// rememberTier caches one resolution and returns its config. Caching is also how
// a refused merge stays quiet instead of being re-derived on every call.
func rememberTier(entry *tierEntry) *ZenConfig {
	tierMu.Lock()
	tiers[entry.key] = entry
	if len(tiers) > tierCacheMax {
		pruneTiersLocked(time.Now())
	}
	tierMu.Unlock()
	return entry.cfg
}

// pruneTiersLocked drops idle entries, and if the cache is still over its bound,
// the least recently used ones. Caller holds tierMu.
func pruneTiersLocked(now time.Time) {
	floor := now.Add(-tierCacheIdle).UnixNano()
	for key, entry := range tiers {
		if entry.used.Load() < floor {
			delete(tiers, key)
		}
	}
	for len(tiers) > tierCacheMax {
		var oldestKey string
		var oldest int64 = math.MaxInt64
		for key, entry := range tiers {
			if u := entry.used.Load(); u < oldest {
				oldestKey, oldest = key, u
			}
		}
		if oldestKey == "" {
			return
		}
		delete(tiers, oldestKey)
	}
}

// ResetWorkspaceTiers drops every cached tier. Load does this on every reload; it
// is exported for the caller that edits the process-wide config in place instead
// of reloading it, which the cache cannot observe.
func ResetWorkspaceTiers() { invalidateTiers() }

func workspaceKey(workspaceRoot string) string {
	clean := strings.TrimSpace(workspaceRoot)
	// Absolute first, so `.` and the full path of one project are one entry.
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	clean = filepath.Clean(clean)
	// ProjectRoot is part of the key because the base tier is derived from it,
	// and tests (and stdio runs) move it around at runtime.
	return ProjectRoot + "\x00" + ConfigFilePath() + "\x00" + clean
}

// globalGeneration bumps whenever the process-wide config is (re)loaded, so a
// reload invalidates every cached workspace tier without needing a file event
// for each project directory.
var globalGeneration atomic.Uint64

// stamp identifies one config file well enough to decide whether a merge is still
// current. Absent files are a distinct state (ok=false) from empty files.
type stamp struct {
	sum     [sha256.Size]byte
	modTime time.Time
	size    int64
	ok      bool
}

func (s stamp) equal(o stamp) bool {
	if s.ok != o.ok {
		return false
	}
	if !s.ok {
		return true
	}
	return s.sum == o.sum && s.modTime.Equal(o.modTime) && s.size == o.size
}

// contentStamp identifies the project tier by its bytes rather than by mtime and
// size: a sync or backup tool restores both while changing the content, and a
// project tier can carry the youtube identity a run acts as.
func contentStamp(data []byte) stamp {
	return stamp{sum: sha256.Sum256(data), size: int64(len(data)), ok: true}
}

// fileStamp is the freshness signal for the machine-wide tier, which is only
// re-read on reload - and a reload bumps globalGeneration.
func fileStamp(path string) stamp {
	if path == "" {
		return stamp{}
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return stamp{}
	}
	return stamp{modTime: info.ModTime(), size: info.Size(), ok: true}
}

// tierEntry is one workspace's merged config. base is the process-wide config the
// overlay was derived from; a reload swaps that pointer, and a pointer that is no
// longer the live config cannot still describe the workspace. cfg == base means
// this workspace was last served the machine-wide config as-is.
type tierEntry struct {
	key     string
	cfg     *ZenConfig
	base    *ZenConfig
	project stamp
	global  stamp
	gen     uint64
	used    atomic.Int64
}

func (e *tierEntry) current(base *ZenConfig, gen uint64, project, global stamp) bool {
	return e.base == base && e.gen == gen && e.project.equal(project) && e.global.equal(global)
}

var (
	tierMu sync.RWMutex
	tiers  = map[string]*tierEntry{}
)

func invalidateTiers() {
	tierMu.Lock()
	tiers = map[string]*tierEntry{}
	tierMu.Unlock()
}

// decodeConfig turns the merged map back into a typed config, re-applying the
// coercions and "nil map means default" fallbacks the struct cannot express.
func decodeConfig(def ZenConfig, merged map[string]any) (ZenConfig, error) {
	coercePorts(merged)
	if v, ok := merged["default_workspace_root"]; ok && v == nil {
		delete(merged, "default_workspace_root")
	}
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
