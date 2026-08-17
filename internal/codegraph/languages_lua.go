package codegraph

import (
	ts_lua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type luaPlugin struct {
	basePlugin
}

func newLuaPlugin() LanguagePlugin {
	return &luaPlugin{}
}

func (p *luaPlugin) Extensions() []string { return []string{".lua"} }
func (p *luaPlugin) LanguageName() string { return "lua" }

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

	// Named function declarations (function foo(), local function foo(),
	// function t.method(), function t:method()).
	ExtractQueryMatches(language, root, src, "(function_declaration name: [(identifier) (dot_index_expression) (method_index_expression)] @name) @def", "function", &nodes)

	// Table/module definitions: local M = {}
	ExtractQueryMatches(language, root, src, "(variable_declaration (assignment_statement (variable_list name: (identifier) @name) (expression_list (table_constructor))) @def)", "class", &nodes)

	extractLuaRelations(root, nil, &relations, src)

	return DeduplicateNodes(nodes), relations, nil
}

func extractLuaRelations(node *tree_sitter.Node, currentFn *string, relations *[]ParsedRelation, src []byte) {
	if node == nil {
		return
	}

	kind := node.Kind()
	var fnName string
	if currentFn != nil {
		fnName = *currentFn
	}

	if kind == "function_declaration" {
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			fnName = nameNode.Utf8Text(src)
		}
	}

	if kind == "function_call" && fnName != "" {
		nameNode := node.ChildByFieldName("name")
		if nameNode == nil && node.NamedChildCount() > 0 {
			nameNode = node.NamedChild(0)
		}
		if nameNode != nil {
			callee := nameNode.Utf8Text(src)
			if callee == "require" {
				if argsNode := node.ChildByFieldName("arguments"); argsNode != nil && argsNode.NamedChildCount() > 0 {
					mod := argsNode.NamedChild(0).Utf8Text(src)
					mod = trimQuotesAndAngles(mod)
					*relations = append(*relations, ParsedRelation{
						SourceName: "",
						TargetName: mod,
						Relation:   "imports",
					})
				}
			} else if callee != "" {
				*relations = append(*relations, ParsedRelation{
					SourceName: fnName,
					TargetName: callee,
					Relation:   "calls",
				})
			}
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil {
			extractLuaRelations(child, &fnName, relations, src)
		}
	}
}
