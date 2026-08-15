package codegraph

import (
	"sync"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// LanguagePlugin abstracts a tree-sitter language backend with cached parser state.
type LanguagePlugin interface {
	Extensions() []string
	LanguageName() string
	Init() error
	Parse(src []byte) ([]ParsedNode, []ParsedRelation, error)
}

// basePlugin provides shared parser/language caching for all language plugins.
type basePlugin struct {
	mu       sync.Mutex
	parser   *tree_sitter.Parser
	language *tree_sitter.Language
}

// sets the parser and language for the plugin
func (b *basePlugin) setParser(parser *tree_sitter.Parser, language *tree_sitter.Language) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.parser = parser
	b.language = language
}

// returns the parser and language for the plugin
func (b *basePlugin) getParser() (*tree_sitter.Parser, *tree_sitter.Language) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.parser, b.language
}
