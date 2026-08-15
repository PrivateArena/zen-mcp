package codegraph

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
)

type cPlugin struct {
	basePlugin
}

// creates a new C language plugin
func newCPlugin() LanguagePlugin {
	return &cPlugin{}
}

// returns the supported file extensions
func (p *cPlugin) Extensions() []string { return []string{".c", ".h"} }
// returns the language name
func (p *cPlugin) LanguageName() string { return "c" }

// initializes the package
func (p *cPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_c.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

// parses source bytes into nodes and relations
func (p *cPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	parser, language := p.getParser()
	if parser == nil || language == nil {
		return nil, nil, nil
	}

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, nil, nil
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	ExtractQueryMatches(language, root, src, "(translation_unit (function_definition name: (identifier) @name) @def)", "function", &nodes)
	ExtractQueryMatches(language, root, src, "(translation_unit (struct_specifier name: (type_identifier) @name) @def)", "struct", &nodes)

	return DeduplicateNodes(nodes), relations, nil
}
