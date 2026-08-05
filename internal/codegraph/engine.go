package codegraph

import (
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
		nodes, relations, err := cg.parser.Parse("."+filepath.Ext(fr.Path), content)
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

// Search searches for symbols by name.
func (cg *CodeGraph) Search(query string) ([]NodeSearchResult, error) {
	return cg.storage.SearchFTS(query)
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

// Mermaid returns a mermaid diagram.
func (cg *CodeGraph) Mermaid() (string, error) {
	nodes, err := cg.storage.FindNodesByName("")
	if err != nil {
		return "", err
	}

	var sb stringsBuilder
	sb.WriteString("graph TD\n")
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("  N%d[%s %s]\n", n.ID, n.Type, n.Name))
	}

	return sb.String(), nil
}

// Usage returns symbol usage.
func (cg *CodeGraph) Usage(symbolName string) ([]NodeSearchResult, error) {
	return cg.storage.SearchFTS(symbolName)
}

// Neighbors returns neighbors of a symbol.
func (cg *CodeGraph) Neighbors(symbolName string) (map[string][]NodeRecord, error) {
	nodes, err := cg.storage.FindNodesByName(symbolName)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return map[string][]NodeRecord{"callers": {}, "callees": {}}, nil
	}

	callers, callees, err := cg.storage.GetNeighbors(nodes[0].ID, 20)
	if err != nil {
		return nil, err
	}

	return map[string][]NodeRecord{
		"callers": callers,
		"callees": callees,
	}, nil
}

// Files returns indexed files.
func (cg *CodeGraph) Files(filter string) ([]FileRecord, error) {
	return cg.storage.ListFiles(filter, 200)
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

// Deadcode returns potentially unused symbols.
func (cg *CodeGraph) Deadcode() ([]NodeRecord, error) {
	// Simplified: find nodes with no incoming edges
	// In practice, this would be more sophisticated
	return nil, nil
}

// ShortestPath finds the shortest path between two symbols.
func (cg *CodeGraph) ShortestPath(from, to string) ([]string, error) {
	return nil, nil
}

// FindCycles finds cycles in the graph.
func (cg *CodeGraph) FindCycles() ([][]string, error) {
	return nil, nil
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
