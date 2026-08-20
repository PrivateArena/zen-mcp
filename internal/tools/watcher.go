package tools

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"zen-mcp/internal/codegraph"
	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
)

// maxWatcherDirs caps the number of recursive fsnotify directory watches so a
// pathological project tree (generated/vendored directories) can never exhaust
// the OS inotify watch limit or drive the watcher loop into a CPU spin. Beyond
// the cap the watcher stops adding directories but keeps serving the watches it
// already registered.
const maxWatcherDirs = 2000

// watcherIgnoreDirs are never recursed into: git internals plus well-known
// dependency/generated directories that routinely hold tens of thousands of
// files. They are merged with the project's codegraph_ignore config.
var watcherIgnoreDirs = []string{
	".git", "node_modules", "dist", "build", ".zen", ".zenmcp", ".venv",
	"venv", "__pycache__", ".next", ".nuxt", ".output", ".idea", ".vscode",
	"coverage",
}

// watcherConfig carries the codegraph watcher tuning knobs resolved from the
// live config.json.
type watcherConfig struct {
	enabled  bool
	debounce time.Duration
	autoLint bool
}

// defaultWatcherDebounce is applied when codegraph_watcher_debounce_ms is
// unset or non-positive, so a misconfigured value can never turn the watcher
// into a tight re-index loop.
const defaultWatcherDebounce = 5 * time.Minute

// readWatcherConfig resolves watcher settings from the merged typed config,
// falling back to the raw config.json file (the pre-existing lookup) when the
// typed config is unavailable. A non-positive debounce falls back to 5 minutes.
func readWatcherConfig() watcherConfig {
	cfg := watcherConfig{debounce: defaultWatcherDebounce}
	if c := mcpcfg.Get(); c != nil {
		cfg.enabled = c.CodegraphWatcher
		cfg.autoLint = c.CodegraphWatcherAutoLint
		if c.CodegraphWatcherDebounceMs > 0 {
			cfg.debounce = time.Duration(c.CodegraphWatcherDebounceMs) * time.Millisecond
		}
		return cfg
	}

	data, err := os.ReadFile(filepath.Join(mcpcfg.ProjectRoot, "config.json"))
	if err != nil {
		return cfg
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return cfg
	}
	if v, ok := raw["codegraph_watcher"].(bool); ok {
		cfg.enabled = v
	}
	if v, ok := raw["codegraph_watcher_auto_lint"].(bool); ok {
		cfg.autoLint = v
	}
	if v, ok := raw["codegraph_watcher_debounce_ms"].(float64); ok && v > 0 {
		cfg.debounce = time.Duration(v) * time.Millisecond
	}
	return cfg
}

// watcherRegistry dedupes watchers per graph root. Two workspace aliases that
// resolve to the same root (e.g. "default" and the absolute path) must share a
// single fsnotify loop instead of double-indexing every change.
var watcherRegistry sync.Map // graph root string -> *codegraphWatcher

// codegraphWatcher watches one workspace root for source changes and triggers
// debounced incremental re-indexes. It only ever runs for codegraph-compatible
// folders (an existing codegraph.db plus a map.json registration), so system
// roots and unregistered trees with thousands of files are never watched.
type codegraphWatcher struct {
	root     string
	autoLint bool
	debounce time.Duration
	fs       *fsnotify.Watcher
	ignore   map[string]bool
	watches  int

	mu      sync.Mutex
	pending map[string]struct{} // root-relative changed file paths
	timer   *time.Timer
	closed  bool

	stopOnce sync.Once
	done     chan struct{}
}

// startCodegraphWatcher begins watching root for workspace when the watcher is
// enabled and the folder is codegraph-compatible. It is idempotent per graph
// root: an already-running watcher for root is returned as-is. It returns nil
// when disabled, ineligible, or fsnotify cannot be initialized, so callers can
// treat a nil watcher as "nothing to do".
func startCodegraphWatcher(workspace, root string, cfg watcherConfig) *codegraphWatcher {
	if !cfg.enabled {
		return nil
	}
	if existing, ok := watcherRegistry.Load(root); ok {
		return existing.(*codegraphWatcher)
	}
	if !isCodegraphCompatible(root) {
		logfilter.Infof("[CodeGraphWatcher] skip %s: not codegraph-compatible (codegraph.db and/or map.json registration missing)", root)
		return nil
	}

	fs, err := fsnotify.NewWatcher()
	if err != nil {
		logfilter.Infof("[CodeGraphWatcher] fsnotify init failed: %v", err)
		return nil
	}

	w := &codegraphWatcher{
		root:     root,
		autoLint: cfg.autoLint,
		debounce: cfg.debounce,
		fs:       fs,
		ignore:   watcherIgnoreSet(),
		pending:  make(map[string]struct{}),
		done:     make(chan struct{}),
	}

	if !w.addRootWatches() {
		_ = fs.Close()
		return nil
	}

	existing, loaded := watcherRegistry.LoadOrStore(root, w)
	if loaded {
		// A concurrent start won the registry: drop our duplicate loop.
		_ = fs.Close()
		return existing.(*codegraphWatcher)
	}
	go w.loop()
	logfilter.Infof("[CodeGraphWatcher] watching %s (debounce=%s, auto_lint=%v)", root, w.debounce, w.autoLint)
	return w
}

// StopWatcherForRoot shuts down the watcher registered for root, if any. Safe
// to call for roots that were never watched.
func StopWatcherForRoot(root string) {
	if v, ok := watcherRegistry.Load(root); ok {
		v.(*codegraphWatcher).Stop()
	}
}

// Stop shuts the watcher down and closes its fsnotify handle. Safe to call
// multiple times and from any goroutine. Closing the fsnotify handle makes the
// event loop exit, which in turn closes done exactly once.
func (w *codegraphWatcher) Stop() {
	w.stopOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		if w.timer != nil {
			w.timer.Stop()
		}
		w.mu.Unlock()
		watcherRegistry.Delete(w.root)
		_ = w.fs.Close()
	})
}

// isCodegraphCompatible reports whether root can be safely watched for
// incremental updates: its codegraph.db must already exist (so re-indexes are
// hash/mtime-diff incremental, not full rescans) and it must be registered in
// map.json (so only explicit Zen projects are ever watched — never /, /root,
// /home, or other file-heavy system trees).
func isCodegraphCompatible(root string) bool {
	if _, err := os.Stat(filepath.Join(root, ".zenmcp", "codegraph.db")); err != nil {
		return false
	}
	return isRegisteredInMap(root)
}

// isRegisteredInMap reports whether root (with or without a trailing slash) is
// a key in the global map.json registry.
func isRegisteredInMap(root string) bool {
	data, err := os.ReadFile(mcpcfg.MapFilePath())
	if err != nil {
		return false
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return false
	}
	target := strings.TrimRight(filepath.ToSlash(root), "/")
	for key := range keys {
		if strings.TrimRight(filepath.ToSlash(key), "/") == target {
			return true
		}
	}
	return false
}

// watcherIgnoreSet merges the built-in skip directories with the project's
// codegraph_ignore config so the watcher respects the same boundaries as the
// indexer.
func watcherIgnoreSet() map[string]bool {
	set := make(map[string]bool, len(watcherIgnoreDirs))
	for _, d := range watcherIgnoreDirs {
		set[d] = true
	}
	if c := mcpcfg.Get(); c != nil {
		for _, pattern := range c.CodegraphIgnore {
			// Only bare directory names from codegraph_ignore participate in
			// directory skipping; file globs (*.min.js) and path patterns are
			// already handled by the indexer's own ignore logic.
			p := strings.TrimSuffix(pattern, "/")
			if p == "" || strings.Contains(p, "*") || strings.Contains(p, "/") {
				continue
			}
			set[p] = true
		}
	}
	return set
}

// addRootWatches recursively registers fsnotify directory watches under root,
// skipping ignored directories and enforcing maxWatcherDirs. Returns false when
// no directory could be watched (e.g. root vanished).
func (w *codegraphWatcher) addRootWatches() bool {
	before := w.watches
	_ = filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != w.root && w.ignore[d.Name()] {
			return fs.SkipDir
		}
		if w.watches >= maxWatcherDirs {
			return fs.SkipDir
		}
		if err := w.fs.Add(path); err == nil {
			w.watches++
		}
		return nil
	})
	return w.watches > before
}

// addTree registers watches for a freshly created directory subtree so new
// directories created under a watched root are picked up without a restart.
func (w *codegraphWatcher) addTree(dir string) {
	if w.ignore[filepath.Base(dir)] {
		return
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != dir && w.ignore[d.Name()] {
			return fs.SkipDir
		}
		if w.watches >= maxWatcherDirs {
			return fs.SkipDir
		}
		if err := w.fs.Add(path); err == nil {
			w.watches++
		}
		return nil
	})
}

// loop drains fsnotify events until the watcher is stopped.
func (w *codegraphWatcher) loop() {
	defer close(w.done)
	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			w.handleEvent(ev)
		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			if err != nil {
				logfilter.Infof("[CodeGraphWatcher] fsnotify error: %v", err)
			}
		}
	}
}

// handleEvent filters a single fsnotify event down to supported source files
// under the watched root and queues them for the debounced flush.
func (w *codegraphWatcher) handleEvent(ev fsnotify.Event) {
	if ev.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			w.addTree(ev.Name)
			w.queueNewTreeFiles(ev.Name)
			return
		}
	}
	if ev.Op&fsnotify.Chmod != 0 {
		// Metadata-only: content-hash diff would skip it anyway.
		return
	}
	rel, err := filepath.Rel(w.root, ev.Name)
	if err != nil {
		return
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return
	}
	if isSkippedPath(rel, w.ignore) {
		return
	}
	if !isSupportedWatcherFile(rel) {
		return
	}
	w.queue(rel)
}

// queueNewTreeFiles queues every supported source file inside a freshly created
// directory, covering the race where a file is written before the directory
// watch is registered and would otherwise be missed until the next event.
func (w *codegraphWatcher) queueNewTreeFiles(dir string) {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != dir && w.ignore[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(w.root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if isSupportedWatcherFile(rel) {
			w.queue(rel)
		}
		return nil
	})
}

// queue records a changed file and (re)arms the debounce timer so a burst of
// events (editor save, git checkout, build output) coalesces into one index.
func (w *codegraphWatcher) queue(rel string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.pending[rel] = struct{}{}
	if w.timer == nil {
		w.timer = time.AfterFunc(w.debounce, w.flush)
	} else {
		w.timer.Reset(w.debounce)
	}
}

// flush runs after the debounce window: it lints git-changed files (when auto
// lint is enabled) and triggers an incremental index over the affected graphs.
func (w *codegraphWatcher) flush() {
	w.mu.Lock()
	if w.closed || len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}
	changed := make([]string, 0, len(w.pending))
	for p := range w.pending {
		changed = append(changed, p)
	}
	w.pending = make(map[string]struct{})
	w.timer = nil
	w.mu.Unlock()

	logfilter.Infof("[CodeGraphWatcher] %d file change(s) detected in %s", len(changed), w.root)

	if w.autoLint {
		w.lintChanged(changed)
	}
	w.indexChanged(changed)
}

// lintChanged formats only the changed files that git actually reports as
// modified/staged/untracked, so formatters never sweep the whole tree. Files
// git does not report (or a non-git folder) are skipped untouched.
func (w *codegraphWatcher) lintChanged(changed []string) {
	gitChanged, ok := gitChangedFiles(w.root)
	if !ok || len(gitChanged) == 0 {
		return
	}
	for _, rel := range changed {
		if !gitChanged[rel] {
			continue
		}
		w.lintFile(rel)
	}
}

// gitChangedFiles returns the set of worktree files git reports as modified,
// staged, or untracked, keyed by slash-relative path. The boolean result
// reports whether root is actually a git repository.
func gitChangedFiles(root string) (map[string]bool, bool) {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	set := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) <= 3 {
			continue
		}
		// Porcelain emits "<XY> <path>"; the path begins after the 3-char
		// status prefix and may be quoted when it contains special chars.
		path := strings.Trim(strings.TrimSpace(line[3:]), `"`)
		if path != "" {
			set[filepath.ToSlash(path)] = true
		}
	}
	return set, true
}

// lintFile runs the language-appropriate formatter over a single file. Only
// formatters present on PATH are invoked; a missing formatter is skipped
// silently so auto-lint degrades gracefully on minimal systems.
func (w *codegraphWatcher) lintFile(rel string) {
	tool, args := linterFor(rel)
	if tool == "" {
		return
	}
	full := filepath.Join(w.root, rel)
	args = append(args, full)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Dir = w.root
	if out, err := cmd.CombinedOutput(); err != nil {
		logfilter.Infof("[CodeGraphWatcher] lint %s failed (%v): %s", rel, err, strings.TrimSpace(string(out)))
	}
}

// linterFor maps a source file to its canonical formatting tool. Formatters are
// idempotent (they write nothing when the file is already formatted), so a
// formatter's own write event cannot loop forever.
func linterFor(rel string) (string, []string) {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".go":
		if lookPath("goimports") != "" {
			return "goimports", []string{"-w"}
		}
		if lookPath("gofmt") != "" {
			return "gofmt", []string{"-w"}
		}
	case ".rs":
		if lookPath("rustfmt") != "" {
			return "rustfmt", nil
		}
	case ".py":
		if lookPath("black") != "" {
			return "black", nil
		}
	case ".ts", ".tsx", ".js", ".jsx", ".mjs":
		if lookPath("prettier") != "" {
			return "prettier", []string{"--write"}
		}
	case ".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hh", ".hxx":
		if lookPath("clang-format") != "" {
			return "clang-format", []string{"-i"}
		}
	case ".rb":
		if lookPath("standardrb") != "" {
			return "standardrb", []string{"-f"}
		}
	case ".lua":
		if lookPath("stylua") != "" {
			return "stylua", nil
		}
	}
	return "", nil
}

// lookPath returns the absolute path of name when it is executable on PATH, or
// "". It is a variable so tests can simulate formatter availability without
// depending on what is installed on the machine.
var lookPath = func(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// indexChanged runs an incremental index over each distinct graph root whose
// tree contains a changed file. Index() is incremental by design (mtime+hash
// diff), so unchanged files are never re-parsed and a burst of edits costs
// exactly the changed files' parse work.
func (w *codegraphWatcher) indexChanged(changed []string) {
	if len(changed) == 0 {
		return
	}
	roots := discoverGraphRoots(w.root)
	seen := make(map[string]bool)
	var queued []string
	for _, rel := range changed {
		full := filepath.Join(w.root, rel)
		best := bestGraphRootFor(full, roots)
		if best != "" && !seen[best] {
			seen[best] = true
			queued = append(queued, best)
		}
	}

	for _, r := range queued {
		cg, err := codegraph.NewCodeGraph(r)
		if err != nil {
			logfilter.Infof("[CodeGraphWatcher] open graph for %s failed: %v", r, err)
			continue
		}
		result, err := cg.Index()
		_ = cg.Close()
		if err != nil {
			logfilter.Infof("[CodeGraphWatcher] index %s failed: %v", r, err)
			continue
		}
		logfilter.Infof("[CodeGraphWatcher] indexed %s: %d file(s) re-parsed, %d deleted", r, result.Indexed, result.Deleted)
	}
}

// bestGraphRootFor returns the deepest graph root whose tree contains full, or
// "" when no root owns it. A file under a sub-graph (its own .zenmcp) is owned
// by that sub-graph, not by ROOT.
func bestGraphRootFor(full string, roots []string) string {
	best := ""
	for _, r := range roots {
		relRoot, err := filepath.Rel(r, full)
		if err != nil || relRoot == ".." || strings.HasPrefix(relRoot, "../") {
			continue
		}
		if len(r) > len(best) {
			best = r
		}
	}
	return best
}

// isSkippedPath reports whether any path segment is an ignored directory.
func isSkippedPath(rel string, ignore map[string]bool) bool {
	for _, part := range strings.Split(rel, "/") {
		if ignore[part] {
			return true
		}
	}
	return false
}

// supportedWatcherExts is the set of extensions codegraph can index, built once
// from the tree-sitter parser so the watcher and indexer agree on what counts
// as a source file.
var supportedWatcherExts = func() map[string]bool {
	m := make(map[string]bool)
	for _, e := range codegraph.GetParser().GetSupportedExtensions() {
		m[e] = true
	}
	return m
}()

// isSupportedWatcherFile reports whether rel names a codegraph-indexable file.
// It mirrors the indexer's exclusions so the watcher never queues a file the
// indexer would skip.
func isSupportedWatcherFile(rel string) bool {
	lower := strings.ToLower(rel)
	for _, sfx := range []string{".min.js", ".min.ts", ".min.mjs", ".bundle.js", ".bundle.ts", ".bundle.mjs"} {
		if strings.HasSuffix(lower, sfx) {
			return false
		}
	}
	return supportedWatcherExts[strings.ToLower(filepath.Ext(rel))]
}
