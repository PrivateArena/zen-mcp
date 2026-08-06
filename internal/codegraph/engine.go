package codegraph

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	parser := &Parser{}
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
		_ = cg.storage.DeleteNodesForFile(fileID)
		_ = cg.storage.DeleteEdgesForFile(fileID)

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
			id, err := cg.storage.InsertNode(nr)
			if err != nil {
				continue
			}
			nodeIDMap[nodes[i].Name+":"+derefString(nodes[i].QualifiedName)] = id
		}

		for _, rel := range relations {
			sourceKey := rel.SourceName
			targetKey := rel.TargetName
			// Try to find matching nodes
			sourceNodes, _ := cg.storage.FindNodesByName(sourceKey)
			targetNodes, _ := cg.storage.FindNodesByName(targetKey)

			if len(sourceNodes) > 0 && len(targetNodes) > 0 {
				for _, sn := range sourceNodes {
					for _, tn := range targetNodes {
						_ = cg.storage.InsertEdge(sn.ID, tn.ID, rel.Relation, rel.Metadata)
					}
				}
			}
		}

		result.Indexed++
	}

	return result, nil
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
	return results, nil
}

// GetRepositoryMap returns a JSON string of the repository map.
func (cg *CodeGraph) GetRepositoryMap(limit int) (string, error) {
	files, err := cg.storage.ListFiles("", limit)
	if err != nil {
		return "", err
	}

	type repoFile struct {
		Path     string `json:"path"`
		Language string `json:"language"`
	}
	repoFiles := make([]repoFile, 0, len(files))
	for _, fr := range files {
		repoFiles = append(repoFiles, repoFile{Path: fr.Path, Language: fr.Language})
	}
	data, err := json.Marshal(repoFiles)
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

	for range files {
		nodes, _ := cg.storage.FindNodesByName("")
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

	// Find nodes with no incoming edges (simplified deadcode detection)
	rows, err := cg.storage.db.Query(`
		SELECT n.id, n.file_id, n.type, n.name, n.language, f.path, n.qualified_name, n.signature, n.docstring, n.start_line, n.end_line, n.content
		FROM nodes n
		JOIN files f ON n.file_id = f.id
		WHERE NOT EXISTS (
			SELECT 1 FROM edges WHERE target_id = n.id
		)
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []NodeRecord
	for rows.Next() {
		var n NodeRecord
		if err := rows.Scan(&n.ID, &n.FileID, &n.Type, &n.Name, &n.Language, &n.Path, &n.QualifiedName, &n.Signature, &n.Docstring, &n.StartLine, &n.EndLine, &n.Content); err == nil {
			symbols = append(symbols, n)
		}
	}

	// Find orphan files (files with no nodes)
	rows, err = cg.storage.db.Query(`
		SELECT f.id, f.path, f.hash, f.mtime, f.language, f.is_test
		FROM files f
		LEFT JOIN nodes n ON f.id = n.file_id
		WHERE n.id IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orphanFiles []FileRecord
	for rows.Next() {
		var fr FileRecord
		if err := rows.Scan(&fr.ID, &fr.Path, &fr.Hash, &fr.MTime, &fr.Language, &fr.IsTest); err == nil {
			orphanFiles = append(orphanFiles, fr)
		}
	}

	return &DeadcodeResult{
		Symbols:     symbols,
		OrphanFiles: orphanFiles,
	}, nil
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
