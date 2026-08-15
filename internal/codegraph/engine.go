package codegraph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"zen-mcp/internal/logfilter"
)

// CodeGraph is the main code graph engine.
type CodeGraph struct {
	storage *Storage
	parser  *Parser
	scanner *Scanner
	rootDir string

	// indexMu serializes Index runs so a concurrent Index on the same graph
	// cannot interleave Phase-2 deletes with Phase-3 edge resolution.
	indexMu sync.Mutex
}

// NewCodeGraph creates a new code graph engine.
func NewCodeGraph(rootDir string) (*CodeGraph, error) {
	dotZenDir := filepath.Join(rootDir, ".zenmcp")
	if err := os.MkdirAll(dotZenDir, 0o755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dotZenDir, "codegraph.db")
	storage, err := NewStorage(dbPath)
	if err != nil {
		return nil, err
	}

	parser := GetParser()
	scanner := NewScanner(storage, rootDir)
	scanner.SetParser(parser)

	return &CodeGraph{
		storage: storage,
		parser:  parser,
		scanner: scanner,
		rootDir: rootDir,
	}, nil
}

// Close closes the database.
func (cg *CodeGraph) Close() error {
	if cg.storage != nil {
		return cg.storage.Close()
	}
	return nil
}

// IndexResult represents the result of an indexing operation.
type IndexResult struct {
	Indexed int
	Total   int
	Deleted int
}

// fileParseResult carries one file's parse output through the Index pipeline.
type fileParseResult struct {
	fr          FileRecord
	fileID      int64
	nodeRecords []NodeRecord
	relations   []ParsedRelation
}

// Index performs a full or incremental index.
func (cg *CodeGraph) Index() (*IndexResult, error) {
	cg.indexMu.Lock()
	defer cg.indexMu.Unlock()

	indexStart := time.Now()

	files, err := cg.scanner.GetFilesToProcess()
	if err != nil {
		return nil, err
	}
	logfilter.Infof("[CodeGraph] Scan: %d file(s) to process in %s", len(files), time.Since(indexStart))

	result := &IndexResult{
		Total: len(files),
	}

	// Phase 1: parse every changed file and collect nodes + relations
	var parseResults []fileParseResult

	phaseStart := time.Now()
	totalNodes := 0
	totalRelations := 0
	slowest, slowestPath := time.Duration(0), ""
	recordSlowest := func(path string, d time.Duration) {
		if d > slowest {
			slowest = d
			slowestPath = path
		}
	}

	for _, fr := range files {
		fileStart := time.Now()

		// Single-read pipeline: the scanner already read the bytes for hashing.
		content := fr.content
		if len(content) == 0 {
			content, err = os.ReadFile(filepath.Join(cg.rootDir, fr.Path))
			if err != nil {
				logfilter.Debugf("[CodeGraph] skip %s: read failed: %v", fr.Path, err)
				continue
			}
		}

		fileID, err := cg.storage.UpsertFile(fr)
		if err != nil {
			logfilter.Debugf("[CodeGraph] skip %s: upsert failed: %v", fr.Path, err)
			continue
		}

		nodes, relations, err := cg.parser.Parse(filepath.Ext(fr.Path), content)
		if err != nil {
			logfilter.Debugf("[CodeGraph] skip %s: parse failed: %v", fr.Path, err)
			continue
		}

		for i := range nodes {
			if nodes[i].QualifiedName == nil || *nodes[i].QualifiedName == nodes[i].Name {
				qn := fr.Path + "::" + nodes[i].Name
				nodes[i].QualifiedName = &qn
			}
			nodes[i].Content = truncate(nodes[i].Content, 1000)
		}

		nodeRecords := make([]NodeRecord, 0, len(nodes))
		for i := range nodes {
			nr := NodeRecord{
				FileID:        fileID,
				Type:          nodes[i].Type,
				Name:          nodes[i].Name,
				Language:      fr.Language,
				QualifiedName: *nodes[i].QualifiedName,
				Signature:     nodes[i].Signature,
				Docstring:     nodes[i].Docstring,
				StartLine:     nodes[i].StartLine,
				EndLine:       nodes[i].EndLine,
				Content:       nodes[i].Content,
			}
			nodeRecords = append(nodeRecords, nr)
		}

		parseResults = append(parseResults, fileParseResult{
			fr:          fr,
			fileID:      fileID,
			nodeRecords: nodeRecords,
			relations:   relations,
		})

		totalNodes += len(nodes)
		totalRelations += len(relations)
		fileDur := time.Since(fileStart)
		recordSlowest(fr.Path, fileDur)
		logfilter.Debugf("[CodeGraph] parse %s: %d node(s), %d relation(s) in %s", fr.Path, len(nodes), len(relations), fileDur)
	}
	logfilter.Infof("[CodeGraph] Phase 1 (parse): %d file(s), %d node(s), %d relation(s) in %s%s",
		len(parseResults), totalNodes, totalRelations, time.Since(phaseStart), slowestSuffix(slowestPath, slowest))

	for i := range parseResults {
		for j := range parseResults[i].relations {
			parseResults[i].relations[j].SourceFile = parseResults[i].fr.Path
		}
	}

	// Phase 2: atomically replace each changed file's data in a single
	// transaction — upsert nodes in place, drop stale nodes, remove only
	// source-side edges. Incoming edges from unchanged files are preserved.
	phaseStart = time.Now()
	slowest, slowestPath = 0, ""
	for _, pr := range parseResults {
		fileStart := time.Now()
		if err := cg.storage.ReindexFileData(pr.fileID, pr.nodeRecords); err != nil {
			logfilter.Debugf("[CodeGraph] reindex %s failed: %v", pr.fr.Path, err)
			continue
		}
		recordSlowest(pr.fr.Path, time.Since(fileStart))
	}
	logfilter.Infof("[CodeGraph] Phase 2 (write nodes): %d file(s) in %s%s",
		len(parseResults), time.Since(phaseStart), slowestSuffix(slowestPath, slowest))

	// Phase 3: build edges now that every target node exists in the DB.
	// Resolution is scoped: same file first, then same directory, then a
	// global fallback tagged INFERRED, with a fan-out cap so common symbol
	// names cannot materialize a Cartesian edge product.
	phaseStart = time.Now()
	processedRelations := 0
	edgeCount := 0
	slowest, slowestPath = 0, ""
	resolver := newNodesResolver(cg.storage)
	for _, pr := range parseResults {
		fileStart := time.Now()
		var edgeRecords []EdgeRecord
		for _, rel := range pr.relations {
			if rel.Relation == "imports" {
				importPath := strings.Trim(rel.TargetName, "\"'`")
				_ = cg.storage.RecordImport(pr.fileID, importPath, rel.IsSideEffect)
				continue
			}
			processedRelations++

			sourceNodes, srcConf := resolveNodesForScope(resolver, rel.SourceName, pr.fr.Path, true)
			if len(sourceNodes) == 0 {
				continue
			}
			targetNodes, tgtConf := resolveNodesForScope(resolver, rel.TargetName, pr.fr.Path, false)
			if len(targetNodes) == 0 {
				continue
			}

			confidence := "EXTRACTED"
			if srcConf == "INFERRED" || tgtConf == "INFERRED" {
				confidence = "INFERRED"
			}

			emitted := 0
			for _, sn := range sourceNodes {
				for _, tn := range targetNodes {
					if emitted >= maxEdgeFanout {
						break
					}
					edgeRecords = append(edgeRecords, EdgeRecord{
						SourceID:   sn.ID,
						TargetID:   tn.ID,
						Relation:   rel.Relation,
						Metadata:   rel.Metadata,
						Confidence: confidence,
					})
					emitted++
				}
				if emitted >= maxEdgeFanout {
					break
				}
			}
		}
		edgeCount += len(edgeRecords)
		if len(edgeRecords) > 0 {
			cg.storage.InsertEdges(edgeRecords)
		}
		recordSlowest(pr.fr.Path, time.Since(fileStart))
	}
	logfilter.Infof("[CodeGraph] Phase 3 (edges): %d relation(s) -> %d edge(s) across %d file(s) in %s%s",
		processedRelations, edgeCount, len(parseResults), time.Since(phaseStart), slowestSuffix(slowestPath, slowest))

	result.Indexed = len(parseResults)

	// C4: drop index entries for files that no longer exist on disk.
	phaseStart = time.Now()
	result.Deleted = cg.cleanupDeletedFiles(parseResults)
	logfilter.Infof("[CodeGraph] Cleanup deleted: %d file(s) in %s", result.Deleted, time.Since(phaseStart))

	phaseStart = time.Now()
	cg.parseManifests()
	logfilter.Infof("[CodeGraph] Manifests: %s", time.Since(phaseStart))

	logfilter.Infof("[CodeGraph] Index total: %s (%d indexed, %d deleted)", time.Since(indexStart), result.Indexed, result.Deleted)

	return result, nil
}

// slowestSuffix renders the per-phase slowest-file annotation for the phase
// summary log lines, or an empty string when no file was processed.
func slowestSuffix(path string, d time.Duration) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf("; slowest: %s (%s)", path, d)
}

// maxEdgeFanout caps the number of edges emitted for a single relation so a
// common symbol name cannot produce a Cartesian product of edges.
const maxEdgeFanout = 64

// nodeResolver resolves node lookups by name for one Index run. Phase 3
// resolves each relation endpoint against every indexed node; a naive
// implementation issues one SQL query per endpoint (two per relation), which
// dominates index time. The resolver instead loads the full node index into
// memory once (a single query) and serves every lookup from it, so edge
// building issues zero per-relation database queries. It is created per Index
// call and never shared across goroutines or runs, so a fresh resolver never
// serves stale node rows.
type nodeResolver struct {
	storage *Storage
	index   map[string][]NodeRecord
	cache   map[string][]NodeRecord
}

// newNodesResolver loads the complete node index for one Index run. The
// backing tables must be stable for the resolver's lifetime (Phase 3 runs
// after Phase 2 has written every changed file's nodes). If the one-shot index
// load fails, the resolver degrades to per-name queries so a transient read
// error degrades the result exactly like the pre-index behavior instead of
// aborting the index.
func newNodesResolver(storage *Storage) *nodeResolver {
	r := &nodeResolver{
		storage: storage,
		cache:   make(map[string][]NodeRecord),
	}

	all, err := storage.GetAllNodes()
	if err != nil {
		logfilter.Errorf("[CodeGraph] build node index failed, falling back to per-name queries: %v", err)
		return r
	}

	index := make(map[string][]NodeRecord, len(all))
	for i := range all {
		n := all[i]
		index[n.Name] = append(index[n.Name], n)
		if n.QualifiedName != "" && n.QualifiedName != n.Name {
			index[n.QualifiedName] = append(index[n.QualifiedName], n)
		}
	}
	r.index = index
	return r
}

// find returns every node matching name, reusing the result on repeat lookups.
// name and qualified_name are indexed separately so the result is equivalent
// to the FindNodesByName predicate (name = ? OR qualified_name = ?).
func (r *nodeResolver) find(name string) []NodeRecord {
	if nodes, ok := r.cache[name]; ok {
		return nodes
	}
	var nodes []NodeRecord
	if r.index != nil {
		nodes = r.index[name]
	} else {
		nodes, _ = r.storage.FindNodesByName(name)
	}
	r.cache[name] = nodes
	return nodes
}

// resolveNodesForScope resolves relation endpoints with file-scope awareness:
// same file first, then same directory, then the global name match as a
// fallback. requireSameFile forces an in-file match for edge sources, since a
// relation is always emitted from the file being parsed.
func resolveNodesForScope(resolver *nodeResolver, name, scopeFile string, requireSameFile bool) ([]NodeRecord, string) {
	all := resolver.find(name)
	if len(all) == 0 {
		return nil, "EXTRACTED"
	}

	var sameFile, sameDir []NodeRecord
	scopeDir := filepath.Dir(scopeFile)
	for _, n := range all {
		switch {
		case n.Path == scopeFile:
			sameFile = append(sameFile, n)
		case filepath.Dir(n.Path) == scopeDir:
			sameDir = append(sameDir, n)
		}
	}

	if len(sameFile) > 0 {
		return sameFile, "EXTRACTED"
	}
	if requireSameFile {
		return nil, "EXTRACTED"
	}
	if len(sameDir) > 0 {
		return sameDir, "EXTRACTED"
	}
	return all, "INFERRED"
}

// cleanupDeletedFiles removes index records for files that no longer exist on
// disk and returns how many were dropped.
func (cg *CodeGraph) cleanupDeletedFiles(processed []fileParseResult) int {
	processedSet := make(map[string]bool, len(processed))
	for _, pr := range processed {
		processedSet[pr.fr.Path] = true
	}

	deleted := 0
	for _, fr := range cg.storage.GetAllFiles() {
		if processedSet[fr.Path] {
			continue
		}
		if _, err := os.Stat(filepath.Join(cg.rootDir, fr.Path)); err != nil {
			if cg.storage.DeleteFile(fr.ID) == nil {
				deleted++
			}
		}
	}
	return deleted
}

// parseManifests parses manifest files (package.json, go.mod, Cargo.toml, pom.xml)
// and inserts their nodes and edges directly into storage. Each manifest is
// gated by its own hash/mtime so incremental runs skip unchanged manifests.
func (cg *CodeGraph) parseManifests() {
	manifestFiles := []string{"package.json", "go.mod", "Cargo.toml", "pom.xml"}
	resolver := newNodesResolver(cg.storage)
	for _, manifest := range manifestFiles {
		fullPath := filepath.Join(cg.rootDir, manifest)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		if info.Size() > maxFileSize {
			continue
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		hashBytes := sha256.Sum256(data)
		hashStr := hex.EncodeToString(hashBytes[:])
		mtime := info.ModTime().UnixMilli()

		if cached := cg.storage.GetFileByPath(manifest); cached != nil && cached.Hash == hashStr && cached.MTime == mtime {
			continue
		}

		fileNodes, fileRelations := parseManifestContent(string(data), manifest)
		if len(fileNodes) == 0 {
			continue
		}

		fr := FileRecord{
			Path:     manifest,
			Language: "manifest",
			Hash:     hashStr,
			MTime:    mtime,
		}
		fileID, err := cg.storage.UpsertFile(fr)
		if err != nil {
			continue
		}

		nodeRecords := make([]NodeRecord, 0, len(fileNodes))
		for _, n := range fileNodes {
			nodeRecords = append(nodeRecords, NodeRecord{
				FileID:        fileID,
				Type:          n.Type,
				Name:          n.Name,
				Language:      "manifest",
				QualifiedName: derefString(n.QualifiedName),
				Signature:     n.Signature,
				Docstring:     n.Docstring,
				StartLine:     n.StartLine,
				EndLine:       n.EndLine,
				Content:       n.Content,
			})
		}
		if err := cg.storage.ReindexFileData(fileID, nodeRecords); err != nil {
			continue
		}

		var edgeRecords []EdgeRecord
		for _, rel := range fileRelations {
			sourceNodes, srcConf := resolveNodesForScope(resolver, rel.SourceName, manifest, false)
			targetNodes, tgtConf := resolveNodesForScope(resolver, rel.TargetName, manifest, false)
			if len(sourceNodes) == 0 || len(targetNodes) == 0 {
				continue
			}
			confidence := "EXTRACTED"
			if srcConf == "INFERRED" || tgtConf == "INFERRED" {
				confidence = "INFERRED"
			}
			emitted := 0
			for _, sn := range sourceNodes {
				for _, tn := range targetNodes {
					if emitted >= maxEdgeFanout {
						break
					}
					edgeRecords = append(edgeRecords, EdgeRecord{
						SourceID:   sn.ID,
						TargetID:   tn.ID,
						Relation:   rel.Relation,
						Metadata:   rel.Metadata,
						Confidence: confidence,
					})
					emitted++
				}
				if emitted >= maxEdgeFanout {
					break
				}
			}
		}
		if len(edgeRecords) > 0 {
			cg.storage.InsertEdges(edgeRecords)
		}
	}
}

func parseManifestContent(content, filename string) ([]ParsedNode, []ParsedRelation) {
	var nodes []ParsedNode
	var relations []ParsedRelation

	switch filename {
	case "package.json":
		var pkg struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if err := json.Unmarshal([]byte(content), &pkg); err != nil {
			return nodes, relations
		}
		deps := pkg.Dependencies
		if deps == nil {
			deps = make(map[string]string)
		}
		for k, v := range pkg.DevDependencies {
			deps[k] = v
		}
		for pkgName, ver := range deps {
			qn := "npm:" + pkgName
			nodes = append(nodes, ParsedNode{
				Type:          "EXTERNAL",
				Name:          pkgName,
				QualifiedName: &qn,
				Signature:     pkgName + "@" + ver,
				StartLine:     1,
				EndLine:       1,
				Content:       "npm package: " + pkgName,
			})
		}

	case "go.mod":
		re := regexp.MustCompile(`^\s*require\s+(.+?)(?:\s+(.+?))?$`)
		for _, line := range strings.Split(content, "\n") {
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				mod := strings.TrimSpace(matches[1])
				mod = strings.TrimSuffix(mod, "/v2")
				mod = strings.TrimSuffix(mod, "/v3")
				qn := "go:" + mod
				nodes = append(nodes, ParsedNode{
					Type:          "EXTERNAL",
					Name:          mod,
					QualifiedName: &qn,
					Signature:     mod + " " + strings.TrimSpace(matches[2]),
					StartLine:     1,
					EndLine:       1,
					Content:       "go module: " + mod,
				})
			}
		}

	case "Cargo.toml":
		depSection := content
		if idx := strings.Index(content, "[dependencies]"); idx >= 0 {
			depSection = content[idx:]
		}
		if idx := strings.Index(depSection, "["); idx > 0 {
			depSection = depSection[:idx]
		}
		for _, line := range strings.Split(depSection, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "[") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				crate := strings.TrimSpace(parts[0])
				qn := "cargo:" + crate
				nodes = append(nodes, ParsedNode{
					Type:          "EXTERNAL",
					Name:          crate,
					QualifiedName: &qn,
					Signature:     "crate: " + crate,
					StartLine:     1,
					EndLine:       1,
					Content:       "cargo crate: " + crate,
				})
			}
		}

	case "pom.xml":
		re := regexp.MustCompile(`<dependency>[\s\S]*?<groupId>(.+?)</groupId>[\s\S]*?<artifactId>(.+?)</artifactId>`)
		matches := re.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if len(m) >= 3 {
				groupId := strings.TrimSpace(m[1])
				artifactId := strings.TrimSpace(m[2])
				gav := groupId + ":" + artifactId
				qn := "maven:" + gav
				nodes = append(nodes, ParsedNode{
					Type:          "EXTERNAL",
					Name:          artifactId,
					QualifiedName: &qn,
					Signature:     gav,
					StartLine:     1,
					EndLine:       1,
					Content:       "maven artifact: " + gav,
				})
			}
		}
	}

	return nodes, relations
}

// Search searches for symbols by name with optional limit.
func (cg *CodeGraph) Search(query string, limit int) ([]NodeSearchResult, error) {
	rows, err := cg.storage.db.Query(`
		SELECT n.id, n.name, n.qualified_name, n.type, f.path, n.start_line, n.end_line
		FROM nodes_fts fts
		JOIN nodes n ON fts.rowid = n.id
		JOIN files f ON n.file_id = f.id
		WHERE nodes_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, sanitizeFtsQuery(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []NodeSearchResult
	for rows.Next() {
		var r NodeSearchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.QualifiedName, &r.Type, &r.Path, &r.StartLine, &r.EndLine); err != nil {
			continue
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// GetRepositoryMap returns a JSON string of the repository map matching TS format.
func (cg *CodeGraph) GetRepositoryMap(maxItems int) (string, error) {
	if maxItems <= 0 {
		maxItems = 10
	}

	// Languages
	langRows, err := cg.storage.db.Query(`SELECT language, COUNT(*) as count FROM files GROUP BY language ORDER BY count DESC`)
	if err != nil {
		return "", err
	}
	defer langRows.Close()

	languages := make(map[string]int)
	for langRows.Next() {
		var lang string
		var count int
		if err := langRows.Scan(&lang, &count); err == nil {
			languages[lang] = count
		}
	}
	if err := langRows.Err(); err != nil {
		return "", err
	}

	// Top Directories
	pathRows, err := cg.storage.db.Query(`SELECT path FROM files`)
	if err != nil {
		return "", err
	}
	defer pathRows.Close()

	var allPaths []string
	for pathRows.Next() {
		var p string
		if err := pathRows.Scan(&p); err == nil {
			allPaths = append(allPaths, p)
		}
	}
	if err := pathRows.Err(); err != nil {
		return "", err
	}

	dirCounts := make(map[string]int)
	for _, p := range allPaths {
		parts := strings.Split(p, "/")
		if len(parts) > 1 {
			dir := strings.Join(parts[:len(parts)-1], "/") + "/"
			dirCounts[dir]++
		}
	}

	type dirEntry struct {
		dir   string
		count int
	}
	var dirs []dirEntry
	for d, c := range dirCounts {
		dirs = append(dirs, dirEntry{d, c})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].count > dirs[j].count
	})
	majorPaths := make([]string, 0, maxItems)
	for i := 0; i < len(dirs) && i < maxItems; i++ {
		majorPaths = append(majorPaths, dirs[i].dir)
	}

	// Hotspots
	hotspotRows, err := cg.storage.db.Query(`
		SELECT s.qualified_name, s.name, s.type, f.path, s.start_line,
			   (COALESCE(ein.cnt, 0) + COALESCE(eout.cnt, 0)) as degree
		FROM nodes s
		JOIN files f ON s.file_id = f.id
		LEFT JOIN (SELECT target_id, COUNT(*) as cnt FROM edges GROUP BY target_id) ein ON s.id = ein.target_id
		LEFT JOIN (SELECT source_id, COUNT(*) as cnt FROM edges GROUP BY source_id) eout ON s.id = eout.source_id
		WHERE s.type != 'module'
		ORDER BY degree DESC
		LIMIT ?
	`, maxItems)
	if err != nil {
		return "", err
	}
	defer hotspotRows.Close()

	type hotspot struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		File   string `json:"file"`
		Degree int    `json:"degree"`
	}
	var hotspots []hotspot
	for hotspotRows.Next() {
		var h hotspot
		if err := hotspotRows.Scan(&h.Name, &h.Type, &h.File, &h.Degree); err == nil {
			hotspots = append(hotspots, h)
		}
	}
	if err := hotspotRows.Err(); err != nil {
		return "", err
	}

	// File Hotspots
	fileHotspotRows, err := cg.storage.db.Query(`
		SELECT f.path, COUNT(e.id) as incoming_references
		FROM files f
		JOIN nodes n ON n.file_id = f.id
		JOIN edges e ON e.target_id = n.id
		GROUP BY f.id
		ORDER BY incoming_references DESC
		LIMIT ?
	`, maxItems)
	if err != nil {
		return "", err
	}
	defer fileHotspotRows.Close()

	type fileHotspot struct {
		Path       string `json:"path"`
		References int    `json:"references"`
	}
	var hotspotFiles []fileHotspot
	for fileHotspotRows.Next() {
		var fh fileHotspot
		if err := fileHotspotRows.Scan(&fh.Path, &fh.References); err == nil {
			hotspotFiles = append(hotspotFiles, fh)
		}
	}
	if err := fileHotspotRows.Err(); err != nil {
		return "", err
	}

	// Heavy Files
	heavyRows, err := cg.storage.db.Query(`
		SELECT f.path, MAX(n.end_line) as line_count
		FROM files f
		JOIN nodes n ON n.file_id = f.id
		GROUP BY f.id
		ORDER BY line_count DESC
		LIMIT ?
	`, maxItems)
	if err != nil {
		return "", err
	}
	defer heavyRows.Close()

	type heavyFile struct {
		Path  string `json:"path"`
		Lines int    `json:"lines"`
	}
	var heavyFiles []heavyFile
	for heavyRows.Next() {
		var hf heavyFile
		if err := heavyRows.Scan(&hf.Path, &hf.Lines); err == nil {
			heavyFiles = append(heavyFiles, hf)
		}
	}
	if err := heavyRows.Err(); err != nil {
		return "", err
	}

	// Complex Files
	complexRows, err := cg.storage.db.Query(`
		SELECT f.path, COUNT(n.id) as symbol_count
		FROM files f
		JOIN nodes n ON n.file_id = f.id
		WHERE n.type != 'module'
		GROUP BY f.id
		ORDER BY symbol_count DESC
		LIMIT ?
	`, maxItems)
	if err != nil {
		return "", err
	}
	defer complexRows.Close()

	type complexFile struct {
		Path    string `json:"path"`
		Symbols int    `json:"symbols"`
	}
	var complexFiles []complexFile
	for complexRows.Next() {
		var cf complexFile
		if err := complexRows.Scan(&cf.Path, &cf.Symbols); err == nil {
			complexFiles = append(complexFiles, cf)
		}
	}
	if err := complexRows.Err(); err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("# Repository Map\n\n")

	sb.WriteString("## Languages\n\n")
	sb.WriteString("| Language | Count |\n")
	sb.WriteString("|----------|-------|\n")
	for lang, count := range languages {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", lang, count))
	}
	sb.WriteString("\n")

	sb.WriteString("## Major Paths\n\n")
	for _, p := range majorPaths {
		sb.WriteString(fmt.Sprintf("- `%s`\n", p))
	}
	sb.WriteString("\n")

	if len(hotspots) > 0 {
		sb.WriteString("## Hotspots\n\n")
		sb.WriteString("| Name | Type | File | Degree |\n")
		sb.WriteString("|------|------|------|--------|\n")
		for _, h := range hotspots {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d |\n", h.Name, h.Type, h.File, h.Degree))
		}
		sb.WriteString("\n")
	}

	if len(hotspotFiles) > 0 {
		sb.WriteString("## Hotspot Files\n\n")
		sb.WriteString("| Path | References |\n")
		sb.WriteString("|------|------------|\n")
		for _, hf := range hotspotFiles {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", hf.Path, hf.References))
		}
		sb.WriteString("\n")
	}

	if len(heavyFiles) > 0 {
		sb.WriteString("## Heavy Files\n\n")
		sb.WriteString("| Path | Lines |\n")
		sb.WriteString("|------|-------|\n")
		for _, hf := range heavyFiles {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", hf.Path, hf.Lines))
		}
		sb.WriteString("\n")
	}

	if len(complexFiles) > 0 {
		sb.WriteString("## Complex Files\n\n")
		sb.WriteString("| Path | Symbols |\n")
		sb.WriteString("|------|---------|\n")
		for _, cf := range complexFiles {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", cf.Path, cf.Symbols))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// Map returns the graph map as markdown.
func (cg *CodeGraph) Map() (string, error) {
	files, err := cg.storage.ListFiles("", 0)
	if err != nil {
		return "", err
	}

	var sb stringsBuilder
	sb.WriteString(fmt.Sprintf("# Code Graph Map\n\n"))
	sb.WriteString(fmt.Sprintf("Root: %s\n\n", cg.rootDir))
	sb.WriteString(fmt.Sprintf("Files: %d\n\n", len(files)))

	for _, fr := range files {
		sb.WriteString(fmt.Sprintf("- `%s` (%s)\n", fr.Path, fr.Language))
	}

	return sb.String(), nil
}

// GetSkeleton returns the skeleton for a specific file from the index.
func (cg *CodeGraph) GetSkeleton(relPath string) (string, error) {
	file := cg.storage.GetFileByPath(relPath)
	if file == nil {
		return fmt.Sprintf("File not found in index: %s", relPath), nil
	}

	nodes, err := cg.storage.GetNodesForFile(file.ID)
	if err != nil {
		return "", err
	}

	if len(nodes) == 0 {
		return fmt.Sprintf("File %s has no indexed symbols.", relPath), nil
	}

	var sb stringsBuilder
	sb.WriteString(fmt.Sprintf("File Skeleton: %s\n", relPath))
	sb.WriteString(fmt.Sprintf("Language: %s\n", file.Language))
	sb.WriteString("----------------------------------------\n")

	for _, n := range nodes {
		loc := fmt.Sprintf("(lines %d-%d)", n.StartLine, n.EndLine)
		sig := strings.TrimSpace(n.Signature)
		if sig != "" {
			oneLineSig := strings.Join(strings.Fields(sig), " ")
			sb.WriteString(fmt.Sprintf("%s %s %s %s\n", n.Type, n.Name, oneLineSig, loc))
		} else {
			sb.WriteString(fmt.Sprintf("%s %s %s\n", n.Type, n.Name, loc))
		}
		if n.Docstring != "" {
			sb.WriteString(fmt.Sprintf("  \"%s\"\n", n.Docstring))
		}
	}

	return sb.String(), nil
}

// Skeletons returns symbol skeletons.
func (cg *CodeGraph) Skeletons() (string, error) {
	files, err := cg.storage.ListFiles("", 0)
	if err != nil {
		return "", err
	}

	var sb stringsBuilder
	sb.WriteString("# Code Graph Skeletons\n\n")

	for _, fr := range files {
		nodes, err := cg.storage.GetNodesForFile(fr.ID)
		if err != nil {
			continue
		}
		for _, n := range nodes {
			sb.WriteString(fmt.Sprintf("## %s %s at %s:%d\n", n.Type, n.Name, n.QualifiedName, n.StartLine))
			if n.Signature != "" {
				sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", n.Signature))
			}
		}
	}

	return sb.String(), nil
}

// SymbolsByDocstring returns every indexed symbol, joined with its file path,
// filtered by docstring presence. hasDoc=true selects documented symbols,
// hasDoc=false selects symbols missing a docstring (the docstring-maintenance
// ledger). Ordered by file path then line for stable, groupable output.
func (cg *CodeGraph) SymbolsByDocstring(hasDoc bool, limit int) ([]NodeRecord, error) {
	return cg.storage.ListSymbolsByDocstring(hasDoc, limit)
}

// GenerateMermaid returns a mermaid diagram with optional query filter and limit.
func (cg *CodeGraph) GenerateMermaid(query string, limit int) (string, error) {
	var nodes []NodeRecord
	var err error
	if query != "" {
		results, _ := cg.Search(query, limit)
		for _, r := range results {
			n, _ := cg.storage.FindNodesByName(r.Name)
			if len(n) > 0 {
				nodes = append(nodes, n[0])
			}
		}
	} else {
		nodes, err = cg.storage.FindNodesByName("")
		if err != nil {
			return "", err
		}
	}

	var sb stringsBuilder
	sb.WriteString("graph TD\n")
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("  N%d[%s %s]\n", n.ID, n.Type, n.Name))
	}

	return sb.String(), nil
}

// Mermaid returns a mermaid diagram.
func (cg *CodeGraph) Mermaid() (string, error) {
	return cg.GenerateMermaid("", 0)
}

// FindUsage returns symbol usage with limit.
func (cg *CodeGraph) FindUsage(symbolName string, limit int) ([]NodeSearchResult, error) {
	if limit <= 0 {
		limit = 50
	}
	return cg.storage.SearchFTS(symbolName)
}

// Usage returns symbol usage.
func (cg *CodeGraph) Usage(symbolName string) ([]NodeSearchResult, error) {
	return cg.FindUsage(symbolName, 50)
}

// GetNeighbors returns neighbors of a symbol with configurable limit.
func (cg *CodeGraph) GetNeighbors(query string, limit int) (map[string][]NodeRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	nodes, err := cg.storage.FindNodesByName(query)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return map[string][]NodeRecord{"callers": {}, "callees": {}}, nil
	}

	callers, callees, err := cg.storage.GetNeighbors(nodes[0].ID, limit)
	if err != nil {
		return nil, err
	}

	return map[string][]NodeRecord{
		"callers": callers,
		"callees": callees,
	}, nil
}

// Neighbors returns neighbors of a symbol.
func (cg *CodeGraph) Neighbors(symbolName string) (map[string][]NodeRecord, error) {
	return cg.GetNeighbors(symbolName, 20)
}

// Files returns indexed files, optionally filtered by path query.
func (cg *CodeGraph) Files(query string, limit int) ([]FileRecord, error) {
	allFiles, err := cg.storage.ListFiles("", 0)
	if err != nil {
		return nil, err
	}

	var filtered []FileRecord
	if query != "" {
		for _, f := range allFiles {
			if f.Path == query || strings.HasPrefix(f.Path, query+"/") || strings.Contains(f.Path, query) {
				filtered = append(filtered, f)
			}
		}
	} else {
		filtered = allFiles
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// ScanDiskFiles returns all disk files under the graph root.
func (cg *CodeGraph) ScanDiskFiles() ([]string, error) {
	return cg.scanner.GetDiskFiles()
}

// Explain returns information about a symbol.
func (cg *CodeGraph) Explain(symbolName string) (string, error) {
	nodes, err := cg.storage.FindNodesByName(symbolName)
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return fmt.Sprintf("Symbol %q not found", symbolName), nil
	}

	n := nodes[0]
	return fmt.Sprintf("%s %s at %s:%d-%d\nSignature: %s\nDoc: %s",
		n.Type, n.Name, n.QualifiedName, n.StartLine, n.EndLine, n.Signature, n.Docstring), nil
}

// RelatedFiles returns related files with edge metadata.
func (cg *CodeGraph) RelatedFiles(filePath string, limit int) ([]RelatedRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	return cg.storage.GetRelatedForFile(filePath, limit)
}

// Related returns related symbols.
func (cg *CodeGraph) Related(symbolName string) ([]NodeRecord, error) {
	nodes, err := cg.storage.FindNodesByName(symbolName)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	callers, callees, err := cg.storage.GetNeighbors(nodes[0].ID, 20)
	if err != nil {
		return nil, err
	}
	related := append(callers, callees...)
	return related, nil
}

// FindDeadCode returns dead code analysis results.
func (cg *CodeGraph) FindDeadCode(query string, limit int) (*DeadcodeResult, error) {
	files, _ := cg.storage.ListFiles("", 0)
	filePaths := make([]string, 0, len(files))
	for _, f := range files {
		filePaths = append(filePaths, f.Path)
	}

	// Find dead symbols using storage method
	result := cg.storage.FindDeadCode(query, limit)

	return result, nil
}

// Deadcode returns potentially unused symbols.
func (cg *CodeGraph) Deadcode() ([]NodeRecord, error) {
	result, _ := cg.FindDeadCode("", 0)
	return result.Symbols, nil
}

// FindShortestPath finds the shortest path between two symbols with depth limit.
func (cg *CodeGraph) FindShortestPath(from, to string, limit int) (*ShortestPathResult, error) {
	return cg.storage.FindShortestPath(from, to, limit)
}

// ShortestPath finds the shortest path between two symbols.
func (cg *CodeGraph) ShortestPath(from, to string) ([]string, error) {
	result, _ := cg.FindShortestPath(from, to, 6)
	if !result.Found {
		return nil, nil
	}
	lines := make([]string, 0, len(result.Path))
	for _, step := range result.Path {
		lines = append(lines, fmt.Sprintf("%s --%s--> %s", step.SourceName, step.Relation, step.TargetName))
	}
	return lines, nil
}

// FindCycles finds cycles in the graph.
func (cg *CodeGraph) FindCycles() ([]CycleRecord, error) {
	return cg.storage.FindCycles()
}

// Markdown returns the full graph as markdown.
func (cg *CodeGraph) Markdown() (string, error) {
	files, err := cg.storage.ListFiles("", 0)
	if err != nil {
		return "", err
	}

	var sb stringsBuilder
	sb.WriteString("# Code Graph\n\n")

	for range files {
		nodes, _ := cg.storage.FindNodesByName("")
		for _, n := range nodes {
			sb.WriteString(fmt.Sprintf("- %s %s at %s:%d\n", n.Type, n.Name, n.QualifiedName, n.StartLine))
		}
	}

	return sb.String(), nil
}

// Impact returns the impact of a symbol.
func (cg *CodeGraph) Impact(symbolName string) (string, error) {
	nodes, err := cg.storage.FindNodesByName(symbolName)
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return fmt.Sprintf("Symbol %q not found", symbolName), nil
	}

	callers, callees, err := cg.storage.GetNeighbors(nodes[0].ID, 100)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Symbol %s has %d callers and %d callees", symbolName, len(callers), len(callees)), nil
}

// Status returns indexing status.
func (cg *CodeGraph) Status() (map[string]any, error) {
	dotZenDir := filepath.Join(cg.rootDir, ".zenmcp")
	dbPath := filepath.Join(dotZenDir, "codegraph.db")

	lastModified := "Unknown"
	if fi, err := os.Stat(dbPath); err == nil {
		lastModified = fi.ModTime().UTC().Format(time.RFC3339)
	}

	counts := cg.storage.GetStats()
	nearby := cg.findNearbyIndices(2)

	advice := "If the index was generated recently, do NOT re-index unless code has significantly changed to save token/compute. Use 'search' or 'mermaid' directly."
	if len(nearby) > 0 {
		advice = fmt.Sprintf("Found %d other indices in subfolders. Use 'workspace' to switch context if needed. If the current index was generated recently, do NOT re-index unless code has significantly changed.", len(nearby))
	}

	return map[string]any{
		"workingDir":    cg.rootDir,
		"dbPath":        dbPath,
		"lastIndexed":   lastModified,
		"counts":        counts,
		"nearbyIndices": nearby,
		"advice":        advice,
	}, nil
}

func (cg *CodeGraph) findNearbyIndices(depth int) []string {
	if depth <= 0 {
		depth = 2
	}
	var indices []string
	var scan func(dir string, currentDepth int)
	scan = func(dir string, currentDepth int) {
		if currentDepth > depth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == "node_modules" || strings.HasPrefix(name, ".") {
				continue
			}
			subDir := filepath.Join(dir, name)
			dbPath := filepath.Join(subDir, ".zenmcp", "codegraph.db")
			if _, err := os.Stat(dbPath); err == nil {
				indices = append(indices, subDir)
			} else {
				scan(subDir, currentDepth+1)
			}
		}
	}
	scan(cg.rootDir, 1)
	return indices
}

func derefString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	for maxLen > 0 && !utf8.RuneStart(s[maxLen]) {
		maxLen--
	}
	return s[:maxLen]
}

// stringsBuilder is a simple string builder wrapper
type stringsBuilder struct {
	parts []string
}

func (sb *stringsBuilder) WriteString(s string) {
	sb.parts = append(sb.parts, s)
}

func (sb *stringsBuilder) String() string {
	return strings.Join(sb.parts, "")
}
