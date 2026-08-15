package codegraph

import (
	ts_lua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type luaPlugin struct {
	basePlugin
}

// creates a new Lua language plugin
func newLuaPlugin() LanguagePlugin {
	return &luaPlugin{}
}

// returns the supported file extensions
func (p *luaPlugin) Extensions() []string { return []string{".lua"} }
// returns the language name
func (p *luaPlugin) LanguageName() string { return "lua" }

// initializes the package
func (p *luaPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_lua.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

// parses source bytes into nodes and relations
func (p *luaPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
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

	ExtractQueryMatches(language, root, src, "(chunk (function_declaration name: (identifier) @name) @def)", "function", &nodes)
	ExtractQueryMatches(language, root, src, "(chunk (function_statement name: (identifier) @name) @def)", "function", &nodes)
	ExtractQueryMatches(language, root, src, "(chunk (local_function name: (identifier) @name) @def)", "function", &nodes)

	return DeduplicateNodes(nodes), relations, nil
}
