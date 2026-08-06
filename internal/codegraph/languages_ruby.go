package codegraph

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
)

type rubyPlugin struct {
	basePlugin
}

func newRubyPlugin() LanguagePlugin {
	return &rubyPlugin{}
}

func (p *rubyPlugin) Extensions() []string { return []string{".rb"} }
func (p *rubyPlugin) LanguageName() string { return "ruby" }

func (p *rubyPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_ruby.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

func (p *rubyPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
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

	ExtractQueryMatches(language, root, src, "(program (class name: (constant) @name) @def)", "class", &nodes)
	ExtractQueryMatches(language, root, src, "(program (method name: (identifier) @name) @def)", "method", &nodes)
	ExtractQueryMatches(language, root, src, "(program (method name: (constant) @name) @def)", "method", &nodes)

	return DeduplicateNodes(nodes), relations, nil
}
