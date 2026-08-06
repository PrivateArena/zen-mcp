package codegraph

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

type javaPlugin struct {
	basePlugin
}

func newJavaPlugin() LanguagePlugin {
	return &javaPlugin{}
}

func (p *javaPlugin) Extensions() []string { return []string{".java"} }
func (p *javaPlugin) LanguageName() string { return "java" }

func (p *javaPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_java.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

func (p *javaPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
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

	ExtractQueryMatches(language, root, src, "(program (class_declaration name: (identifier) @name) @def)", "class", &nodes)
	ExtractQueryMatches(language, root, src, "(program (interface_declaration name: (identifier) @name) @def)", "interface", &nodes)
	ExtractQueryMatches(language, root, src, "(program (enum_declaration name: (identifier) @name) @def)", "enum", &nodes)
	ExtractQueryMatches(language, root, src, "(class_body (method_declaration name: (identifier) @name) @def)", "method", &nodes)
	ExtractQueryMatches(language, root, src, "(interface_body (method_declaration name: (identifier) @name) @def)", "method", &nodes)

	return DeduplicateNodes(nodes), relations, nil
}
