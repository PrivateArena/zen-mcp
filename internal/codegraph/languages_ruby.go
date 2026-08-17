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

	// Classes and modules (unanchored: matches at any nesting depth).
	ExtractQueryMatches(language, root, src, "(class name: (constant) @name) @def", "class", &nodes)
	ExtractQueryMatches(language, root, src, "(module name: (constant) @name) @def", "module", &nodes)

	// Instance methods.
	ExtractQueryMatches(language, root, src, "(method name: (identifier) @name) @def", "method", &nodes)

	// Singleton methods (def self.foo).
	ExtractQueryMatches(language, root, src, "(singleton_method name: (identifier) @name) @def", "method", &nodes)

	extractRubyRelations(root, nil, &relations, src)

	return DeduplicateNodes(nodes), relations, nil
}

func extractRubyRelations(node *tree_sitter.Node, currentFn *string, relations *[]ParsedRelation, src []byte) {
	if node == nil {
		return
	}

	kind := node.Kind()
	var fnName string
	if currentFn != nil {
		fnName = *currentFn
	}

	if kind == "method" {
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			fnName = nameNode.Utf8Text(src)
		}
	} else if kind == "singleton_method" {
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			fnName = "self." + nameNode.Utf8Text(src)
		}
	}

	if kind == "call" {
		methodNode := node.ChildByFieldName("method")
		if methodNode != nil {
			method := methodNode.Utf8Text(src)
			if method == "require" || method == "require_relative" {
				if argsNode := node.ChildByFieldName("arguments"); argsNode != nil && argsNode.NamedChildCount() > 0 {
					mod := argsNode.NamedChild(0).Utf8Text(src)
					mod = trimQuotesAndAngles(mod)
					*relations = append(*relations, ParsedRelation{
						SourceName: "",
						TargetName: mod,
						Relation:   "imports",
					})
				}
			} else if fnName != "" && method != "" {
				*relations = append(*relations, ParsedRelation{
					SourceName: fnName,
					TargetName: method,
					Relation:   "calls",
				})
			}
		}
	} else if kind == "identifier" && fnName != "" {
		// Bare method call (no receiver) as a statement.
		if parent := node.Parent(); parent != nil && parent.Kind() == "body_statement" {
			callee := node.Utf8Text(src)
			if callee != "" {
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
			extractRubyRelations(child, &fnName, relations, src)
		}
	}
}
