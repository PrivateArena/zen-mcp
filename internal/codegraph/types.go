package codegraph

// ParsedNode represents a code symbol extracted by tree-sitter.
type ParsedNode struct {
	Type          string
	Name          string
	QualifiedName *string
	Signature     string
	Docstring     string
	StartLine     int
	EndLine       int
	Content       string
}

// ParsedRelation represents a relationship between two symbols.
type ParsedRelation struct {
	SourceName   string
	TargetName   string
	Relation     string
	Metadata     string
	SourceFile   string
	IsSideEffect bool
}

// RelatedRecord represents a related file/symbol edge.
type RelatedRecord struct {
	RelatedPath     string
	RelatedLanguage string
	Direction       string
	Relation        string
	SymbolName      string
	SymbolType      string
}

// DeadcodeResult represents dead code analysis output.
type DeadcodeResult struct {
	Symbols     []NodeRecord
	OrphanFiles []FileRecord
}

// ShortestPathStep represents one hop in a shortest path.
type ShortestPathStep struct {
	SourceName string
	SourceFile string
	SourceLine int
	TargetName string
	TargetFile string
	TargetLine int
	Relation   string
}

// ShortestPathResult represents the result of a shortest path query.
type ShortestPathResult struct {
	Found bool
	Path  []ShortestPathStep
}

// CycleEdge represents an edge within a cycle.
type CycleEdge struct {
	From string
	To   string
}

// CycleRecord represents a detected cycle.
type CycleRecord struct {
	Files []string
	Edges []CycleEdge
}
