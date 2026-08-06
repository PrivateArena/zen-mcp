package codegraph

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// CodeGraph is the main code graph engine.
type CodeGraph struct {
	storage *Storage
	parser  *Parser
	scanner *Scanner
	rootDir string
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

// Index performs a full or incremental index.
func (cg *CodeGraph) Index() (*IndexResult, error) {
	files, err := cg.scanner.GetFilesToProcess()
	if err != nil {
		return nil, err
	}

	result := &IndexResult{
		Total: len(files),
	}

	for _, fr := range files {
		content, err := os.ReadFile(filepath.Join(cg.rootDir, fr.Path))
		if err != nil {
			continue
		}

		fileID, err := cg.storage.UpsertFile(fr)
		if err != nil {
			continue
		}

		// Clear old nodes/edges for this file
		cg.storage.DeleteNodesForFile(fileID)
		cg.storage.DeleteEdgesForFile(fileID)

		// Parse and insert nodes
		nodes, relations, err := cg.parser.Parse(filepath.Ext(fr.Path), content)
		if err != nil {
			continue
		}

		for i := range nodes {
			if nodes[i].QualifiedName == nil || *nodes[i].QualifiedName == nodes[i].Name {
				qn := fr.Path + "::" + nodes[i].Name
				nodes[i].QualifiedName = &qn
			}
			nodes[i].Content = truncate(nodes[i].Content, 1000)
		}

		// Build node records for batch insert
		nodeRecords := make([]NodeRecord, 0, len(nodes))
		nodeIDMap := make(map[string]int64)
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

		// Batch insert nodes
		if len(nodeRecords) > 0 {
			_, err := cg.storage.InsertNodes(nodeRecords)
			if err != nil {
				continue
			}
		}

		// Re-fetch IDs for relation building
		insertedNodes, _ := cg.storage.GetNodesForFile(fileID)
		for i := range insertedNodes {
			nodeIDMap[insertedNodes[i].Name+":"+insertedNodes[i].QualifiedName] = insertedNodes[i].ID
		}

		// Collect relations for batch insert
		var edgeRecords []EdgeRecord
		for _, rel := range relations {
			sourceKey := rel.SourceName
			targetKey := rel.TargetName
			sourceNodes, _ := cg.storage.FindNodesByName(sourceKey)
			targetNodes, _ := cg.storage.FindNodesByName(targetKey)

			if len(sourceNodes) > 0 && len(targetNodes) > 0 {
				for _, sn := range sourceNodes {
					for _, tn := range targetNodes {
						edgeRecords = append(edgeRecords, EdgeRecord{
							SourceID:   sn.ID,
							TargetID:   tn.ID,
							Relation:   rel.Relation,
							Metadata:   rel.Metadata,
							Confidence: "EXTRACTED",
						})
					}
				}
			}
		}

		if len(edgeRecords) > 0 {
			cg.storage.InsertEdges(edgeRecords)
		}

		result.Indexed++
	}

	// Phase 1.5: Parse manifest files
	cg.parseManifests()

	// Phase 2: Batch generate embeddings for all gathered nodes
	// (Skipped in Go - embeddings are handled by TS daemon)

	return result, nil
}

// parseManifests parses manifest files (package.json, go.mod, Cargo.toml, pom.xml)
func (cg *CodeGraph) parseManifests() ([]NodeRecord, []ParsedRelation) {
	var nodes []NodeRecord
	var relations []ParsedRelation

	manifestFiles := []string{"package.json", "go.mod", "Cargo.toml", "pom.xml"}
	for _, manifest := range manifestFiles {
		fullPath := filepath.Join(cg.rootDir, manifest)
		if _, err := os.Stat(fullPath); err != nil {
			continue
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		fileNodes, fileRelations := parseManifestContent(string(data), manifest)
		for _, n := range fileNodes {
			nodes = append(nodes, NodeRecord{
				Type:          n.Type,
				Name:          n.Name,
				Language:      "manifest",
				QualifiedName: derefString(n.QualifiedName),
				Signature:     n.Signature,
				StartLine:     n.StartLine,
				EndLine:       n.EndLine,
				Content:       n.Content,
			})
		}
		relations = append(relations, fileRelations...)
	}

	return nodes, relations
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
		SELECT n.id, n.name, n.type, f.path, n.start_line, n.end_line
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
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Path, &r.StartLine, &r.EndLine); err != nil {
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

	result := map[string]interface{}{
		"languages":    languages,
		"majorPaths":   majorPaths,
		"hotspots":     hotspots,
		"hotspotFiles": hotspotFiles,
		"heavyFiles":   heavyFiles,
		"complexFiles": complexFiles,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
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
	if limit <= 0 {
		limit = 200
	}

	// Find all files
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
	result, _ := cg.FindDeadCode("", 200)
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
	files := cg.storage.GetAllFiles()

	langCounts := map[string]int{}
	for _, fr := range files {
		langCounts[fr.Language]++
	}

	return map[string]any{
		"files_indexed": len(files),
		"languages":     langCounts,
	}, nil
}

func derefString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

func detectLanguageFromPath(relPath string) string {
	ext := strings.ToLower(filepath.Ext(relPath))
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs":
		return "typescript"
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".lua":
		return "lua"
	}
	return "unknown"
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
