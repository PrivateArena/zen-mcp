package codegraph

// ParsedNode represents a code symbol extracted by tree-sitter.
type ParsedNode struct {
	Type        string
	Name        string
	QualifiedName *string
	Signature   string
	Docstring   string
	StartLine   int
	EndLine     int
	Content     string
}

// ParsedRelation represents a relationship between two symbols.
type ParsedRelation struct {
	SourceName string
	TargetName string
	Relation   string
	Metadata   string
}

// LanguagePlugin abstracts a tree-sitter language backend.
type LanguagePlugin struct {
	Extensions   []string
	LanguageName string
	Parse        func(src []byte) ([]ParsedNode, []ParsedRelation, error)
}
