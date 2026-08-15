package codegraph

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

type cppPlugin struct {
	basePlugin
}

// creates a new C++ language plugin
func newCppPlugin() LanguagePlugin {
	return &cppPlugin{}
}

// returns the supported file extensions
func (p *cppPlugin) Extensions() []string { return []string{".cpp", ".hpp"} }
// returns the language name
func (p *cppPlugin) LanguageName() string { return "cpp" }

// initializes the package
func (p *cppPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_cpp.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

// parses source bytes into nodes and relations
func (p *cppPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
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
	ExtractQueryMatches(language, root, src, "(translation_unit (class_specifier name: (type_identifier) @name) @def)", "class", &nodes)
	ExtractQueryMatches(language, root, src, "(class_specifier (field_declaration_list (function_definition name: (identifier) @name) @def))", "method", &nodes)

	return DeduplicateNodes(nodes), relations, nil
}
