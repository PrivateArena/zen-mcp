package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"zen-mcp/internal/codegraph"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

type layeredGraphEntry struct {
	graph   *codegraph.CodeGraph
	root    string
	label   string
	index   int
	dbMTime time.Time
}

type layeredGraphSession struct {
	workspaceRoot string
	entries       []layeredGraphEntry
}

var graphRegistry = new(sync.Map)

func discoverGraphRoots(workspaceRoot string) []string {
	roots := []string{workspaceRoot}

	var scan func(dir string)
	scan = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == "node_modules" || name == ".git" || name == "dist" || name == "build" {
				continue
			}
			fullPath := filepath.Join(dir, name)
			stat, err := os.Stat(fullPath)
			if err != nil {
				continue
			}
			if stat.IsDir() {
				if name == ".zenmcp" {
					dbPath := filepath.Join(fullPath, "codegraph.db")
					if _, err := os.Stat(dbPath); err == nil && dir != workspaceRoot {
						roots = append(roots, dir)
					}
				} else {
					scan(fullPath)
				}
			}
		}
	}

	scan(workspaceRoot)

	seen := make(map[string]bool)
	var unique []string
	for _, r := range roots {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}
	sort.Slice(unique, func(i, j int) bool {
		if unique[i] == workspaceRoot {
			return true
		}
		if unique[j] == workspaceRoot {
			return false
		}
		return unique[i] < unique[j]
	})
	return unique
}

func getSessionByWorkspace(workspace string) (*layeredGraphSession, error) {
	root := workspace
	if r := resolveWorkspaceFromDeps("", workspace); r != "" {
		root = r
	}

	if s, ok := graphRegistry.Load(workspace); ok {
		session := s.(*layeredGraphSession)
		if session.workspaceRoot == root && !sessionStale(session) {
			return session, nil
		}
		ClearSessionGraph(session)
	}

	watcherEnabled := false

	configPath := filepath.Join(mcpcfg.ProjectRoot, "config.json")
	if data, err := os.ReadFile(configPath); err == nil {
		var cfg map[string]any
		if json.Unmarshal(data, &cfg) == nil {
			if v, ok := cfg["codegraph_watcher"].(bool); ok {
				watcherEnabled = v
			}
		}
	}

	rootEntries := discoverGraphRoots(root)
	entries := make([]layeredGraphEntry, 0, len(rootEntries))
	for i, r := range rootEntries {
		g, err := codegraph.NewCodeGraph(r)
		if err != nil {
			continue
		}
		label := "ROOT"
		if r != root {
			rel, _ := filepath.Rel(root, r)
			label = rel
		}
		dbPath := filepath.Join(r, ".zenmcp", "codegraph.db")
		var dbMTime time.Time
		if st, err := os.Stat(dbPath); err == nil {
			dbMTime = st.ModTime()
		}
		entries = append(entries, layeredGraphEntry{
			graph:   g,
			root:    r,
			label:   label,
			index:   i + 1,
			dbMTime: dbMTime,
		})
	}

	session := &layeredGraphSession{
		workspaceRoot: root,
		entries:       entries,
	}
	graphRegistry.Store(workspace, session)

	if watcherEnabled && len(entries) > 0 {
		rootEntry := entries[0]
		dbPath := filepath.Join(rootEntry.root, ".zenmcp", "codegraph.db")
		if _, err := os.Stat(filepath.Join(rootEntry.root, ".git")); err == nil {
			if _, err := os.Stat(dbPath); err == nil {
				// Watcher start deferred until Go engine exposes it
			}
		}
	}

	return session, nil
}

// sessionStale reports whether a cached layered session no longer matches the
// on-disk graph layout: a sub-graph root appeared/disappeared, or any
// sub-graph's codegraph.db was recreated/re-indexed since the session opened
// its handles. Stale sessions are rebuilt so actions always query the current
// root + sub-graph databases in parallel, mirroring the TypeScript behavior.
func sessionStale(session *layeredGraphSession) bool {
	roots := discoverGraphRoots(session.workspaceRoot)
	if len(roots) != len(session.entries) {
		return true
	}
	for _, r := range roots {
		matched := false
		for _, e := range session.entries {
			if e.root == r {
				matched = true
				break
			}
		}
		if !matched {
			return true
		}
	}
	for _, e := range session.entries {
		dbPath := filepath.Join(e.root, ".zenmcp", "codegraph.db")
		st, err := os.Stat(dbPath)
		if err != nil {
			return true
		}
		if !st.ModTime().Equal(e.dbMTime) {
			return true
		}
	}
	return false
}

func ClearSessionGraph(session *layeredGraphSession) {
	for _, entry := range session.entries {
		_ = entry.graph.Close()
	}
}

func ClearSessionGraphByWorkspace(workspace string) {
	graphRegistry.Range(func(key, value any) bool {
		session := value.(*layeredGraphSession)
		if session.workspaceRoot == workspace {
			ClearSessionGraph(session)
			graphRegistry.Delete(key)
		}
		return true
	})
}

func getTargetGraphs(session *layeredGraphSession, isolate int) ([]layeredGraphEntry, error) {
	if isolate == 0 {
		return session.entries, nil
	}
	idx := isolate - 1
	if idx < 0 || idx >= len(session.entries) {
		var scopes []string
		for _, e := range session.entries {
			scopes = append(scopes, fmt.Sprintf("  [%d] %s (%s)", e.index, e.label, e.root))
		}
		return nil, fmt.Errorf("isolate=%d out of range. Available scopes:\n%s", isolate, strings.Join(scopes, "\n"))
	}
	return []layeredGraphEntry{session.entries[idx]}, nil
}

func expandQueryPaths(query string, session *layeredGraphSession) []string {
	rawPaths := strings.Split(query, ",")
	var parts []string
	for _, p := range rawPaths {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return nil
	}

	expanded := make(map[string]bool)

	for _, target := range session.entries {
		files, _ := target.graph.Files("", 0)
		indexedPaths := make(map[string]bool)
		for _, f := range files {
			indexedPaths[f.Path] = true
		}

		scopePrefix := ""
		if target.root != session.workspaceRoot {
			rel, _ := filepath.Rel(session.workspaceRoot, target.root)
			scopePrefix = rel + "/"
		}

		for _, p := range parts {
			subPath := p
			if scopePrefix != "" && strings.HasPrefix(p, scopePrefix) {
				subPath = p[len(scopePrefix):]
			}

			if indexedPaths[subPath] {
				expanded[subPath] = true
				continue
			}

			dirPrefix := subPath
			if !strings.HasSuffix(dirPrefix, "/") {
				dirPrefix += "/"
			}
			found := false
			for fp := range indexedPaths {
				if fp == subPath || strings.HasPrefix(fp, dirPrefix) {
					expanded[fp] = true
					found = true
				}
			}
			if !found {
				for _, f := range files {
					if f.Path == subPath || strings.HasPrefix(f.Path, subPath+"/") {
						expanded[f.Path] = true
					}
				}
			}
		}
	}

	result := make([]string, 0, len(expanded))
	for p := range expanded {
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}

type layeredResult struct {
	label string
	data  string
}

func formatLayered(results []layeredResult, single bool) string {
	if single || len(results) == 1 {
		return results[0].data
	}
	var parts []string
	for _, r := range results {
		parts = append(parts, fmt.Sprintf("=== [scope: %s] ===\n%s", r.label, r.data))
	}
	return strings.Join(parts, "\n\n")
}

func resolveRefactorTarget(session *layeredGraphSession, isolate int) (layeredGraphEntry, string, error) {
	target := session.entries[0]
	warning := ""
	if isolate != 0 {
		targets, err := getTargetGraphs(session, isolate)
		if err != nil {
			return layeredGraphEntry{}, "", err
		}
		target = targets[0]
	} else if len(session.entries) > 1 {
		warning = "[WARNING] isolate=0 (default) selected. Defaulting refactor to ROOT scope. Use isolate=N to target sub-scopes.\n\n"
	}
	return target, warning, nil
}

func actionIndex(session *layeredGraphSession, isolate int) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		result, err := target.graph.Index()
		if err != nil {
			parts = append(parts, fmt.Sprintf("Scope [%s]: index failed: %v.", target.label, err))
			continue
		}
		stats, _ := target.graph.Status()
		fileCount := 0
		if counts, ok := stats["counts"].(map[string]int); ok {
			fileCount = counts["fileCount"]
		}
		if result.Deleted > 0 {
			parts = append(parts, fmt.Sprintf("Scope [%s]: indexed %d file(s), skipped %d, removed %d stale file(s); %d total files.", target.label, result.Indexed, result.Total-result.Indexed, result.Deleted, fileCount))
		} else {
			parts = append(parts, fmt.Sprintf("Scope [%s]: indexed %d file(s); %d total files.", target.label, result.Indexed, fileCount))
		}
	}
	return strings.Join(parts, "\n"), nil
}

func actionMap(session *layeredGraphSession, isolate int, limit *int) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	results := make([]layeredResult, 0, len(targets))
	l := 0
	if limit != nil {
		l = *limit
	}
	for _, target := range targets {
		data, _ := target.graph.GetRepositoryMap(l)
		results = append(results, layeredResult{label: target.label, data: data})
	}
	return formatLayered(results, isolate != 0), nil
}

func actionNeighbors(session *layeredGraphSession, isolate int, query string, limit *int) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	results := make([]layeredResult, 0, len(targets))
	l := 20
	if limit != nil {
		l = *limit
	}
	for _, target := range targets {
		neighbors, err := target.graph.GetNeighbors(query, l)
		if err != nil {
			continue
		}
		data, _ := json.Marshal(neighbors)
		results = append(results, layeredResult{label: target.label, data: string(data)})
	}
	if len(results) == 0 {
		return "", fmt.Errorf(`No neighbors found for "%s" in any of the active scopes.`, query)
	}
	return formatLayered(results, isolate != 0), nil
}

func actionUsage(session *layeredGraphSession, isolate int, query string) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	results := make([]layeredResult, 0, len(targets))
	for _, target := range targets {
		usages, err := target.graph.FindUsage(query, 50)
		if err != nil {
			continue
		}
		if len(usages) > 0 {
			data, _ := json.Marshal(usages)
			results = append(results, layeredResult{label: target.label, data: string(data)})
		}
	}
	if len(results) == 0 {
		return "", fmt.Errorf(`No usages found for "%s" in any of the active scopes. Make sure the index is up to date and the symbol name is exact.`, query)
	}
	return formatLayered(results, isolate != 0), nil
}

func fmtFileList(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	groups := make(map[string][]string)
	for _, p := range paths {
		dir := filepath.Dir(p)
		base := filepath.Base(p)
		group := dir
		if dir == "." {
			group = "/"
		}
		groups[group] = append(groups[group], base)
	}
	var dirs []string
	for d := range groups {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	var lines []string
	for _, dir := range dirs {
		files := groups[dir]
		sort.Strings(files)
		lines = append(lines, fmt.Sprintf("[%s]", dir))
		for _, f := range files {
			lines = append(lines, fmt.Sprintf(" %s", f))
		}
	}
	return strings.Join(lines, "\n")
}

func actionFiles(session *layeredGraphSession, isolate int, query string, limit *int) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	results := make([]layeredResult, 0, len(targets))

	expanded := make([]string, 0)
	if query != "" {
		expanded = expandQueryPaths(query, session)
	}

	l := 0
	if limit != nil {
		l = *limit
	}

	for _, target := range targets {
		var paths []string
		if query != "" {
			if len(expanded) > 0 {
				if len(expanded) > l {
					paths = expanded[:l]
				} else {
					paths = expanded
				}
			} else {
				fileList, _ := target.graph.Files(query, l)
				for _, f := range fileList {
					paths = append(paths, f.Path)
				}
			}
		} else {
			fileList, _ := target.graph.Files("", l)
			for _, f := range fileList {
				paths = append(paths, f.Path)
			}
		}
		count := len(paths)
		header := fmt.Sprintf("%d file(s)", count)
		if query != "" {
			header += fmt.Sprintf(` matching "%s"`, query)
		}
		header += ":"
		results = append(results, layeredResult{
			label: target.label,
			data:  header + "\n" + fmtFileList(paths),
		})
	}
	return formatLayered(results, isolate != 0), nil
}

func actionRelated(session *layeredGraphSession, isolate int, query string, limit *int) (string, error) {
	expanded := expandQueryPaths(query, session)
	if len(expanded) == 0 {
		return "", fmt.Errorf(`No indexed files found for query "%s". Ensure the files are indexed.`, query)
	}
	filePath := expanded[0]

	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	results := make([]layeredResult, 0, len(targets))
	l := 50
	if limit != nil {
		l = *limit
	}
	for _, target := range targets {
		related, err := target.graph.RelatedFiles(filePath, l)
		if err != nil {
			continue
		}
		if len(related) > 0 {
			grouped := make(map[string][]codegraph.RelatedRecord)
			var order []string
			for _, r := range related {
				if _, ok := grouped[r.RelatedPath]; !ok {
					order = append(order, r.RelatedPath)
				}
				grouped[r.RelatedPath] = append(grouped[r.RelatedPath], r)
			}
			var lines []string
			for _, path := range order {
				rels := grouped[path]
				lang := rels[0].RelatedLanguage
				if lang == "" {
					lang = "unknown"
				}
				lines = append(lines, fmt.Sprintf("[%s] (%s)", path, lang))
				for _, r := range rels {
					arrow := "→"
					if r.Direction == "incoming" {
						arrow = "←"
					}
					lines = append(lines, fmt.Sprintf("  %s %s: %s (%s)", arrow, r.Relation, r.SymbolName, r.SymbolType))
				}
				lines = append(lines, "")
			}
			results = append(results, layeredResult{label: target.label, data: strings.TrimSpace(strings.Join(lines, "\n"))})
		}
	}
	if len(results) == 0 {
		targets, err := getTargetGraphs(session, isolate)
		if err != nil {
			return "", err
		}
		target := targets[0]
		fileList, _ := target.graph.Files(filePath, 1)
		fileInIndex := false
		for _, f := range fileList {
			if f.Path == filePath {
				fileInIndex = true
				break
			}
		}
		advice := fmt.Sprintf(`No related files found for "%s".`, query)
		if !fileInIndex {
			advice += fmt.Sprintf(` The file is not present in the index for scope "%s". Run 'codegraph index' to refresh.`, target.label)
		} else {
			advice += fmt.Sprintf(` The file is indexed but has no cross-file edges detected. Try 'codegraph skeletons "%s"' to verify the file has parsed symbols, or re-index.`, query)
		}
		return "", fmt.Errorf("%s", advice)
	}
	return formatLayered(results, isolate != 0), nil
}

func actionSearch(session *layeredGraphSession, isolate int, query string, limit *int) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	results := make([]layeredResult, 0, len(targets))
	l := 10
	if limit != nil {
		l = *limit
	}
	for _, target := range targets {
		graphResults, _ := target.graph.Search(query, l)
		data, _ := json.Marshal(graphResults)
		results = append(results, layeredResult{label: target.label, data: string(data)})
	}
	return formatLayered(results, isolate != 0), nil
}

func actionStatus(session *layeredGraphSession, isolate int) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	results := make([]layeredResult, 0, len(targets))
	for _, target := range targets {
		stats, _ := target.graph.Status()
		data, _ := json.Marshal(stats)
		results = append(results, layeredResult{label: target.label, data: string(data)})
	}
	text := formatLayered(results, isolate != 0)
	if isolate == 0 && len(session.entries) > 1 {
		var scopes []string
		for _, e := range session.entries {
			scopes = append(scopes, fmt.Sprintf("[%d] %s", e.index, e.label))
		}
		text += "\n\nAvailable scopes: " + strings.Join(scopes, " | ") + "\nUse isolate=N to target a specific scope."
	}
	return text, nil
}

func actionSkeletons(session *layeredGraphSession, isolate int, query string) (string, error) {
	expanded := expandQueryPaths(query, session)
	if len(expanded) == 0 {
		return "", fmt.Errorf(`No indexed files found for query "%s". Ensure the files are indexed (run codegraph index first).`, query)
	}

	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	var output []string
	for _, p := range expanded {
		found := false
		for _, target := range targets {
			skel, err := target.graph.GetSkeleton(p)
			if err == nil && skel != "" && !strings.HasPrefix(skel, "File not found") {
				output = append(output, skel)
				found = true
				break
			}
		}
		if !found {
			output = append(output, fmt.Sprintf("File not found in any active scope: %s", p))
		}
	}

	trimmed := strings.TrimSpace(strings.Join(output, "\n\n"))
	if trimmed == "" {
		return "", fmt.Errorf(`Could not retrieve skeleton for "%s" in any of the active scopes. Expanded paths: [%s]`, query, strings.Join(expanded, ", "))
	}
	return trimmed, nil
}

func actionMermaid(session *layeredGraphSession, isolate int, query string, limit *int) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(targets))
	l := 60
	if limit != nil {
		l = *limit
	}
	for _, target := range targets {
		diagram, _ := target.graph.GenerateMermaid(query, l)
		note := ""
		if query == "" {
			note = `# Tip: add a query like "src/auth" to scope to a directory.` + "\n\n"
		}
		parts = append(parts, fmt.Sprintf("### Scope: %s\n```mermaid\n%s%s\n```", target.label, note, diagram))
	}
	return strings.Join(parts, "\n\n"), nil
}

func actionMarkdown(session *layeredGraphSession, isolate int, query string) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return "", fmt.Errorf("No scopes available for markdown dump.")
	}

	showFileList := true
	fullDumpAll := false
	cfg := mcpcfg.Get()
	showFileList = cfg.CodegraphMarkdownFiles
	fullDumpAll = cfg.CodegraphMarkdownFulldump

	fullDumpPaths := make(map[string]bool)
	if query != "" {
		expanded := expandQueryPaths(query, session)
		for _, p := range expanded {
			fullDumpPaths[p] = true
		}
	}

	target := targets[0]
	diskFiles, _ := target.graph.ScanDiskFiles()

	doFullDump := len(fullDumpPaths) > 0 || fullDumpAll
	dumpModeText := ""
	if len(fullDumpPaths) > 0 {
		dumpModeText = fmt.Sprintf("Full content for %d file(s), skeletons for %d file(s)", len(fullDumpPaths), len(diskFiles)-len(fullDumpPaths))
	} else if fullDumpAll {
		dumpModeText = "Full content for all files (fulldump)"
	} else {
		dumpModeText = "Skeletons for all files"
	}

	var mdParts []string
	mdParts = append(mdParts, fmt.Sprintf("# CodeGraph Markdown Dump — %s", target.label))
	mdParts = append(mdParts, fmt.Sprintf("**Root**: `%s`", target.root))
	mdParts = append(mdParts, fmt.Sprintf("**Files**: %d", len(diskFiles)))
	mdParts = append(mdParts, fmt.Sprintf("**Dump mode**: %s", dumpModeText))
	mdParts = append(mdParts, "")

	if showFileList {
		mdParts = append(mdParts, "## Project Structure", "", "```", fmtFileList(diskFiles), "```", "")
	}

	mdParts = append(mdParts, "---", "")

	skeletonCache := make(map[string]string)
	for _, relPath := range diskFiles {
		if doFullDump && (len(fullDumpPaths) == 0 || fullDumpPaths[relPath]) {
			content, err := os.ReadFile(filepath.Join(target.root, relPath))
			if err != nil {
				content = fmt.Appendf(nil, "(error reading file: %s: %v)", relPath, err)
			}
			lang := filepath.Ext(relPath)
			if lang != "" {
				lang = lang[1:]
			}
			mdParts = append(mdParts, fmt.Sprintf("## `%s`", relPath), "", fmt.Sprintf("```%s", lang), string(content), "```", "")
		} else {
			skeleton := skeletonCache[relPath]
			if skeleton == "" {
				skel, err := target.graph.GetSkeleton(relPath)
				if err != nil {
					skel = fmt.Sprintf("(error getting skeleton for %s: %v)", relPath, err)
				}
				skeletonCache[relPath] = skel
			}
			if skeleton != "" && !strings.HasPrefix(skeleton, "File not found") && !strings.HasPrefix(skeleton, "(error") {
				mdParts = append(mdParts, fmt.Sprintf("## `%s`", relPath), "", "```", skeleton, "```", "")
			}
		}
	}

	dumpDir := cfg.CodegraphDumpDir
	if dumpDir == "" {
		dumpDir = "/tmp/zen-mcp/codegraph/dump"
	}
	os.MkdirAll(dumpDir, 0755)
	now := time.Now()
	timestamp := fmt.Sprintf("%04d%02d%02d-%02d%02d%02d", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())
	outputPath := filepath.Join(dumpDir, fmt.Sprintf("codegraph-markdown-%s.md", timestamp))
	os.WriteFile(outputPath, []byte(strings.Join(mdParts, "\n")), 0644)

	return outputPath, nil
}

func actionDeadcode(session *layeredGraphSession, isolate int, query string, limit *int) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(targets))
	l := 0
	if limit != nil {
		l = *limit
	}
	for _, target := range targets {
		result, _ := target.graph.FindDeadCode(query, l)
		symbols := result.Symbols
		orphanFiles := result.OrphanFiles

		if len(symbols) == 0 && len(orphanFiles) == 0 {
			parts = append(parts, fmt.Sprintf("Scope [%s]: No dead code found.", target.label))
			continue
		}

		lines := []string{
			fmt.Sprintf("Dead Code Report — scope: [%s]", target.label),
			fmt.Sprintf("Total dead symbols: %d", len(symbols)),
		}
		if len(orphanFiles) > 0 {
			lines = append(lines, fmt.Sprintf("Orphan files (zero imports): %d", len(orphanFiles)))
		}
		lines = append(lines, "")

		if len(symbols) > 0 {
			lines = append(lines, "── Symbols ──", "")
			var currentPath string
			for _, s := range symbols {
				if s.Path != currentPath {
					currentPath = s.Path
					lines = append(lines, currentPath)
				}
				lines = append(lines, fmt.Sprintf("  %s (%s) L%d-%d", s.Name, s.Type, s.StartLine, s.EndLine))
			}
			lines = append(lines, "")
		}

		if len(orphanFiles) > 0 {
			lines = append(lines, "── Orphan Files (zero imports) ──", "")
			var currentLang string
			for _, f := range orphanFiles {
				if f.Language != currentLang {
					currentLang = f.Language
					lines = append(lines, fmt.Sprintf("[%s]", currentLang))
				}
				lines = append(lines, fmt.Sprintf("  %s", f.Path))
			}
			lines = append(lines, "")
		}

		parts = append(parts, strings.Join(lines, "\n"))
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("No scopes available for dead code analysis.")
	}
	results := make([]layeredResult, 0, len(parts))
	for i, p := range parts {
		label := ""
		if i < len(targets) {
			label = targets[i].label
		} else {
			label = fmt.Sprintf("scope%d", i+1)
		}
		results = append(results, layeredResult{label: label, data: p})
	}
	return formatLayered(results, isolate != 0), nil
}

func actionExplain(session *layeredGraphSession, isolate int, query string) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}

	qualifiedMatch := regexp.MustCompile(`^(.+\.\w+):(.+)$`).FindStringSubmatch(query)
	exactFile := ""
	exactName := ""
	searchScope := query
	if len(qualifiedMatch) > 0 {
		exactFile = qualifiedMatch[1]
		exactName = qualifiedMatch[2]
		searchScope = exactName
	}

	type explainResult struct {
		target layeredGraphEntry
		symbol codegraph.NodeSearchResult
	}
	var allResults []explainResult
	for _, target := range targets {
		raw, _ := target.graph.Search(searchScope, 10)
		for _, symbol := range raw {
			if exactFile != "" && (symbol.Path != exactFile || symbol.Name != exactName) {
				continue
			}
			allResults = append(allResults, explainResult{target: target, symbol: symbol})
		}
	}

	if len(allResults) == 0 {
		return "", fmt.Errorf(`Symbol "%s" not found in any active scope. Try re-indexing or check the exact name.`, query)
	}

	nameCount := make(map[string]int)
	for _, r := range allResults {
		nameCount[r.symbol.Name]++
	}
	if len(allResults) > 1 && (nameCount[allResults[0].symbol.Name] > 1) {
		lines := []string{
			fmt.Sprintf(`Multiple matches found for "%s". Use a qualified name to disambiguate:`, query),
			"",
		}
		for _, r := range allResults {
			lines = append(lines, fmt.Sprintf("  [%s:%d] %s (%s)", r.symbol.Path, r.symbol.StartLine, r.symbol.Name, r.symbol.Type))
		}
		lines = append(lines, "", "Rerun with qualified name: explain('src/path/to/file.ts:symbolName')")
		return strings.Join(lines, "\n"), nil
	}

	target, best := allResults[0].target, allResults[0].symbol
	lines := []string{
		fmt.Sprintf("Scope: %s", target.label),
		fmt.Sprintf("Symbol: %s", best.Name),
		fmt.Sprintf("Type: %s", best.Type),
		"",
	}

	if best.Path != "" {
		lines = append(lines, fmt.Sprintf("Location: %s:%d-%d", best.Path, best.StartLine, best.EndLine))
		skel, _ := target.graph.GetSkeleton(best.Path)
		if skel != "" {
			lines = append(lines, "", "--- File skeleton ---", skel)
		} else {
			lines = append(lines, "", "--- (skeleton unavailable) ---")
		}
	}

	seen := make(map[string]bool)
	hop1Callers := []codegraph.NodeRecord{}
	hop1Callees := []codegraph.NodeRecord{}
	if neighbors, err := target.graph.GetNeighbors(best.Name, 10); err == nil {
		hop1Callers = neighbors["callers"]
		hop1Callees = neighbors["callees"]
	}

	formatNeighbor := func(n codegraph.NodeRecord, relation string, hop string) string {
		return fmt.Sprintf("  %s %s (%s:%d) --%s--> %s", hop, n.Name, n.Path, n.StartLine, relation, best.Name)
	}

	if len(hop1Callers) > 0 {
		lines = append(lines, "", fmt.Sprintf("Callers (1-hop, %d):", len(hop1Callers)))
		for _, n := range hop1Callers {
			key := n.Name + ":" + n.Path
			if seen[key] {
				continue
			}
			seen[key] = true
			lines = append(lines, formatNeighbor(n, "calls", "←"))
		}
	}
	if len(hop1Callees) > 0 {
		lines = append(lines, "", fmt.Sprintf("Callees (1-hop, %d):", len(hop1Callees)))
		for _, n := range hop1Callees {
			key := n.Name + ":" + n.Path
			if seen[key] {
				continue
			}
			seen[key] = true
			lines = append(lines, formatNeighbor(n, "calls", "→"))
		}
	}

	type hop2Node struct {
		name        string
		path        string
		relation    string
		hopRelation string
	}
	var hop2Nodes []hop2Node
	for _, n := range append(hop1Callers, hop1Callees...) {
		if len(hop2Nodes) >= 20 {
			break
		}
		key := n.Name + ":" + n.Path
		delete(seen, key)
		if neighbors2, err := target.graph.GetNeighbors(n.Name, 5); err == nil {
			for _, c := range append(neighbors2["callers"], neighbors2["callees"]...) {
				if len(hop2Nodes) >= 20 {
					break
				}
				k2 := c.Name + ":" + c.Path
				if seen[k2] || k2 == key {
					continue
				}
				seen[k2] = true
				hop2Nodes = append(hop2Nodes, hop2Node{
					name:        c.Name,
					path:        c.Path,
					relation:    "calls",
					hopRelation: "calls",
				})
			}
		}
	}

	if len(hop2Nodes) > 0 {
		lines = append(lines, "", fmt.Sprintf("2-hop neighbors (%d, capped at 20):", len(hop2Nodes)))
		for _, n := range hop2Nodes {
			lines = append(lines, fmt.Sprintf("  • %s (%s) --%s--> [via %s]", n.name, n.path, n.relation, n.hopRelation))
		}
	}

	return strings.Join(lines, "\n"), nil
}

func actionImpact(session *layeredGraphSession, isolate int, query string) (string, error) {
	targets, err := getTargetGraphs(session, isolate)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(targets))

	for _, target := range targets {
		rootDir := target.root
		gitDir := filepath.Join(rootDir, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			parts = append(parts, fmt.Sprintf("Scope [%s]: Not a git repository — impact analysis requires git.", target.label))
			continue
		}

		var changedFiles []string
		if query != "" && !strings.HasPrefix(query, "HEAD") {
			changedFiles = expandQueryPaths(query, session)
			if len(changedFiles) == 0 {
				changedFiles = strings.Split(query, ",")
				for i := range changedFiles {
					changedFiles[i] = strings.TrimSpace(changedFiles[i])
				}
				var filtered []string
				for _, f := range changedFiles {
					if f != "" {
						filtered = append(filtered, f)
					}
				}
				changedFiles = filtered
			}
		} else {
			ref := query
			if ref == "" {
				ref = "HEAD~1"
			}
			cmd := exec.Command("git", "diff", "--name-only", ref)
			cmd.Dir = rootDir
			out, err := cmd.Output()
			if err != nil {
				parts = append(parts, fmt.Sprintf("Scope [%s]: Failed to run git diff for ref \"%s\".", target.label, ref))
				continue
			}
			changedFiles = strings.Split(string(out), "\n")
			for i := range changedFiles {
				changedFiles[i] = strings.TrimSpace(changedFiles[i])
			}
			var filtered []string
			for _, f := range changedFiles {
				if f != "" {
					filtered = append(filtered, f)
				}
			}
			changedFiles = filtered
		}

		if len(changedFiles) == 0 {
			parts = append(parts, fmt.Sprintf("Scope [%s]: No changed files detected.", target.label))
			continue
		}

		allAffectedSymbols := make(map[string]bool)
		allAffectedFiles := make(map[string]bool)
		changedFileSet := make(map[string]bool)
		for _, f := range changedFiles {
			changedFileSet[f] = true
		}

		for _, f := range changedFiles {
			fileList, _ := target.graph.Files(f, 1)
			for _, fr := range fileList {
				if fr.Path == f {
					allAffectedFiles[f] = true
				}
			}
		}

		type bfsItem struct {
			file  string
			depth int
		}
		bfsQueue := make([]bfsItem, 0, len(changedFiles))
		bfsVisited := make(map[string]bool)
		for _, f := range changedFiles {
			bfsQueue = append(bfsQueue, bfsItem{f, 0})
			bfsVisited[f] = true
		}
		for len(bfsQueue) > 0 {
			current := bfsQueue[0]
			bfsQueue = bfsQueue[1:]
			if current.depth >= 3 {
				continue
			}
			related, _ := target.graph.RelatedFiles(current.file, 50)
			for _, r := range related {
				if !bfsVisited[r.RelatedPath] {
					bfsVisited[r.RelatedPath] = true
					allAffectedFiles[r.RelatedPath] = true
					allAffectedSymbols[r.SymbolName] = true
					bfsQueue = append(bfsQueue, bfsItem{r.RelatedPath, current.depth + 1})
				}
			}
		}

		totalFiles, _ := target.graph.Files("", 0)
		impactRatio := 0.0
		if len(totalFiles) > 0 {
			impactRatio = float64(len(allAffectedFiles)) / float64(len(totalFiles))
		}
		riskLevel := "low"
		if impactRatio > 0.3 {
			riskLevel = "high"
		} else if impactRatio > 0.1 {
			riskLevel = "med"
		}

		lines := []string{
			fmt.Sprintf("Scope: [%s]", target.label),
			fmt.Sprintf("Changed files: %d", len(changedFiles)),
			fmt.Sprintf("Affected symbols: %d", len(allAffectedSymbols)),
			fmt.Sprintf("Affected files (direct + transitive to depth=3): %d", len(allAffectedFiles)),
			fmt.Sprintf("Impact ratio: %.1f%% of indexed files", impactRatio*100),
			fmt.Sprintf("Risk level: %s", riskLevel),
			"",
			"── Changed Files ──",
		}
		for _, f := range changedFiles {
			lines = append(lines, fmt.Sprintf("  • %s", f))
		}

		if len(allAffectedSymbols) > 0 {
			var sortedSymbols []string
			for s := range allAffectedSymbols {
				sortedSymbols = append(sortedSymbols, s)
			}
			sort.Strings(sortedSymbols)
			if len(sortedSymbols) > 30 {
				sortedSymbols = sortedSymbols[:30]
			}
			lines = append(lines, "", fmt.Sprintf("── Affected Symbols (top %d) ──", len(sortedSymbols)))
			for _, s := range sortedSymbols {
				lines = append(lines, fmt.Sprintf("  • %s", s))
			}
			if len(allAffectedSymbols) > 30 {
				lines = append(lines, fmt.Sprintf("  ... and %d more", len(allAffectedSymbols)-30))
			}
		}

		parts = append(parts, strings.Join(lines, "\n"))
	}

	return strings.Join(parts, "\n\n"), nil
}

func actionShortestPath(session *layeredGraphSession, isolate int, query, format string, limit *int) (string, error) {
	parts := strings.SplitN(query, ",", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("query must be 'from,to' for shortestPath")
	}
	from := strings.TrimSpace(parts[0])
	to := strings.TrimSpace(parts[1])
	target, _, err := resolveRefactorTarget(session, isolate)
	if err != nil {
		return "", err
	}
	l := 6
	if limit != nil {
		l = *limit
	}
	result, _ := target.graph.FindShortestPath(from, to, l)
	if format == "json" {
		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data), nil
	}
	if !result.Found {
		return fmt.Sprintf("No path found from \"%s\" to \"%s\".", from, to), nil
	}
	lines := make([]string, 0, len(result.Path))
	for _, step := range result.Path {
		src := step.SourceName
		if step.SourceFile != "" {
			src = fmt.Sprintf("%s (%s:%d)", step.SourceName, step.SourceFile, step.SourceLine)
		}
		tgt := step.TargetName
		if step.TargetFile != "" {
			tgt = fmt.Sprintf("%s (%s:%d)", step.TargetName, step.TargetFile, step.TargetLine)
		}
		lines = append(lines, fmt.Sprintf("%s --%s--> %s", src, step.Relation, tgt))
	}
	return strings.Join(lines, "\n"), nil
}

func actionFindCycles(session *layeredGraphSession, isolate int) (string, error) {
	target, _, err := resolveRefactorTarget(session, isolate)
	if err != nil {
		return "", err
	}
	cycles, _ := target.graph.FindCycles()
	if len(cycles) == 0 {
		return "No circular dependencies found.", nil
	}
	lines := make([]string, 0, len(cycles))
	for i, c := range cycles {
		files := strings.Join(c.Files, " → ")
		edgeParts := make([]string, 0, len(c.Edges))
		for _, e := range c.Edges {
			edgeParts = append(edgeParts, fmt.Sprintf("%s → %s", e.From, e.To))
		}
		lines = append(lines, fmt.Sprintf("Cycle %d:\n  Files: %s\n  Edges: %s", i+1, files, strings.Join(edgeParts, ", ")))
	}
	return strings.Join(lines, "\n\n"), nil
}

func HandleCodegraphAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)
	if action == "" {
		return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("action is required"), start)
	}

	session, err := getSessionByWorkspace(workspace)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("failed to open codegraph: %s", err.Error()), start)
	}

	getStr := func(key string) string {
		if v, ok := args[key].(string); ok {
			return v
		}
		return ""
	}
	getNum := func(key string) *int {
		if v, ok := args[key].(float64); ok && v > 0 {
			i := int(v)
			return &i
		}
		return nil
	}
	query := getStr("query")
	format := getStr("format")
	limit := getNum("limit")
	isolate := 0
	if v, ok := args["isolate"].(float64); ok {
		isolate = int(v)
	}

	if isolate != 0 {
		if _, err := getTargetGraphs(session, isolate); err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
	}

	run := func(fn func() (string, error)) *mcp.CallToolResult {
		text, err := fn()
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", err, start)
		}
		return toolresponse.WrapSuccess(ctx, "codegraph", text, start)
	}

	switch action {
	case "index":
		return run(func() (string, error) { return actionIndex(session, isolate) })
	case "search":
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("query is required for search"), start)
		}
		return run(func() (string, error) { return actionSearch(session, isolate, query, limit) })
	case "status":
		return run(func() (string, error) { return actionStatus(session, isolate) })
	case "map":
		return run(func() (string, error) { return actionMap(session, isolate, limit) })
	case "skeletons":
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("file path(s) (query) required for skeleton action"), start)
		}
		return run(func() (string, error) { return actionSkeletons(session, isolate, query) })
	case "mermaid":
		return run(func() (string, error) { return actionMermaid(session, isolate, query, limit) })
	case "markdown":
		return run(func() (string, error) { return actionMarkdown(session, isolate, query) })
	case "usage":
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("symbol name (query) is required for usage action"), start)
		}
		return run(func() (string, error) { return actionUsage(session, isolate, query) })
	case "neighbors":
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("symbol name (query) is required for neighbors action"), start)
		}
		return run(func() (string, error) { return actionNeighbors(session, isolate, query, limit) })
	case "files":
		return run(func() (string, error) { return actionFiles(session, isolate, query, limit) })
	case "explain":
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("symbol name (query) is required for explain action"), start)
		}
		return run(func() (string, error) { return actionExplain(session, isolate, query) })
	case "deadcode":
		return run(func() (string, error) { return actionDeadcode(session, isolate, query, limit) })
	case "shortestPath":
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("query (source,target) required for shortestPath action"), start)
		}
		return run(func() (string, error) { return actionShortestPath(session, isolate, query, format, limit) })
	case "findCycles":
		return run(func() (string, error) { return actionFindCycles(session, isolate) })
	case "impact":
		return run(func() (string, error) { return actionImpact(session, isolate, query) })
	case "related":
		if query == "" {
			return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("file path (query) is required for related action"), start)
		}
		return run(func() (string, error) { return actionRelated(session, isolate, query, limit) })
	default:
		return toolresponse.WrapErrorWithContext(ctx, "codegraph", fmt.Errorf("unknown action: %s", action), start)
	}
}

func defCodegraph(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "codegraph",
		Title:       "Code Graph",
		Description: "Code graph engine. Actions: index, search, status, map, skeletons, mermaid, usage, neighbors, files, explain, related, deadcode, shortestPath, findCycles, markdown, impact.",
		Schema: jsonSchema(map[string]any{
			"action": strEnumProp("Codegraph action.", []string{
				"index", "search", "status", "map", "skeletons", "mermaid",
				"usage", "neighbors", "files", "explain", "related",
				"deadcode", "shortestPath", "findCycles", "markdown", "impact",
			}),
			"query":   strProp("Search query or symbol name"),
			"format":  strEnumProp("Output format: text (default) or json", []string{"text", "json"}),
			"limit":   numProp("Result limit"),
			"isolate": numProp("Graph isolate level (0 = root)"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleCodegraphAction(ctx, workspace, deps, req), nil
		},
	}
}
