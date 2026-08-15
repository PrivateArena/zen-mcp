package codegraph

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

type rustPlugin struct {
	basePlugin
}

// creates a new Rust language plugin
func newRustPlugin() LanguagePlugin {
	return &rustPlugin{}
}

// returns the supported file extensions
func (p *rustPlugin) Extensions() []string { return []string{".rs"} }
// returns the language name
func (p *rustPlugin) LanguageName() string { return "rust" }

// initializes the package
func (p *rustPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_rust.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

// parses source bytes into nodes and relations
func (p *rustPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
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

	ExtractQueryMatches(language, root, src, "(source_file (struct_item name: (type_identifier) @name) @def)", "struct", &nodes)
	ExtractQueryMatches(language, root, src, "(source_file (enum_item name: (type_identifier) @name) @def)", "enum", &nodes)
	ExtractQueryMatches(language, root, src, "(source_file (trait_item name: (type_identifier) @name) @def)", "trait", &nodes)
	ExtractQueryMatches(language, root, src, "(source_file (type_item name: (type_identifier) @name) @def)", "type", &nodes)
	ExtractQueryMatches(language, root, src, "(impl_item body: (declaration_list (function_item name: (identifier) @name) @def))", "method", &nodes)
	ExtractQueryMatches(language, root, src, "(trait_item body: (declaration_list (function_item name: (identifier) @name) @def))", "method", &nodes)
	ExtractQueryMatches(language, root, src, "(trait_item body: (declaration_list (function_signature_item name: (identifier) @name) @def))", "method", &nodes)
	ExtractQueryMatches(language, root, src, "(source_file (function_item name: (identifier) @name) @def)", "function", &nodes)

	extractRustRelations(root, nil, &relations, src)

	return DeduplicateNodes(nodes), relations, nil
}

// extracts relations from Rust syntax tree nodes
func extractRustRelations(node *tree_sitter.Node, currentFn *string, relations *[]ParsedRelation, src []byte) {
	if node == nil {
		return
	}

	kind := node.Kind()
	var fnName string
	if currentFn != nil {
		fnName = *currentFn
	}

	if kind == "function_item" {
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			fnName = nameNode.Utf8Text(src)
		}
	}

	if kind == "call_expression" && fnName != "" {
		fnNode := node.ChildByFieldName("function")
		if fnNode != nil {
			callee := fnNode.Utf8Text(src)
			if fnNode.Kind() == "field_expression" {
				fieldNode := fnNode.ChildByFieldName("field")
				if fieldNode != nil {
					callee = fieldNode.Utf8Text(src)
				}
			}
			*relations = append(*relations, ParsedRelation{
				SourceName: fnName,
				TargetName: callee,
				Relation:   "calls",
			})
		}
	} else if kind == "use_declaration" {
		expandRustUse(node, relations, "", src)
		return
	} else if kind == "macro_invocation" && fnName != "" {
		macroNode := node.ChildByFieldName("macro")
		if macroNode != nil {
			*relations = append(*relations, ParsedRelation{
				SourceName: fnName,
				TargetName: macroNode.Utf8Text(src) + "!",
				Relation:   "calls",
			})
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil {
			extractRustRelations(child, &fnName, relations, src)
		}
	}
}

// expands Rust use statement imports
func expandRustUse(node *tree_sitter.Node, relations *[]ParsedRelation, prefix string, src []byte) {
	if node == nil {
		return
	}

	kind := node.Kind()

	if kind == "use_declaration" {
		useTree := node.NamedChild(0)
		if useTree != nil {
			expandRustUse(useTree, relations, prefix, src)
		}
	} else if kind == "use_tree" {
		pathNode := node.ChildByFieldName("path")
		nameNode := node.ChildByFieldName("name")
		listNode := node.ChildByFieldName("list")

		if pathNode != nil && listNode != nil {
			newPrefix := prefix
			if prefix != "" {
				newPrefix = prefix + "::" + pathNode.Utf8Text(src)
			} else {
				newPrefix = pathNode.Utf8Text(src)
			}
			expandRustUse(listNode, relations, newPrefix, src)
		} else if nameNode != nil {
			fullPath := nameNode.Utf8Text(src)
			if prefix != "" {
				fullPath = prefix + "::" + fullPath
			}
			if fullPath != "self" && fullPath != "super" {
				*relations = append(*relations, ParsedRelation{
					SourceName: "module",
					TargetName: fullPath,
					Relation:   "imports",
				})
			}
		} else if pathNode != nil {
			fullPath := pathNode.Utf8Text(src)
			if prefix != "" {
				fullPath = prefix + "::" + fullPath
			}
			*relations = append(*relations, ParsedRelation{
				SourceName: "module",
				TargetName: fullPath,
				Relation:   "imports",
			})
		}
	} else if kind == "use_tree_list" {
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child != nil {
				expandRustUse(child, relations, prefix, src)
			}
		}
	}
}
