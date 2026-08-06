package codegraph

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type typescriptPlugin struct {
	basePlugin
}

func newTypeScriptPlugin() LanguagePlugin {
	return &typescriptPlugin{}
}

func (p *typescriptPlugin) Extensions() []string { return []string{".ts", ".tsx", ".js", ".jsx", ".mjs"} }
func (p *typescriptPlugin) LanguageName() string { return "typescript" }

func (p *typescriptPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_typescript.LanguageTypescript())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

func (p *typescriptPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
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

	ExtractQueryMatches(language, root, src, "(program (class_declaration (type_identifier) @name) @def)", "class", &nodes)
	ExtractQueryMatches(language, root, src, "(program (export_statement (class_declaration (type_identifier) @name) @def))", "class", &nodes)
	ExtractQueryMatches(language, root, src, "(program (interface_declaration (type_identifier) @name) @def)", "interface", &nodes)
	ExtractQueryMatches(language, root, src, "(program (export_statement (interface_declaration (type_identifier) @name) @def))", "interface", &nodes)
	ExtractQueryMatches(language, root, src, "(program (type_alias_declaration (type_identifier) @name) @def)", "type", &nodes)
	ExtractQueryMatches(language, root, src, "(program (export_statement (type_alias_declaration (type_identifier) @name) @def))", "type", &nodes)
	ExtractQueryMatches(language, root, src, "(program (enum_declaration (identifier) @name) @def)", "enum", &nodes)
	ExtractQueryMatches(language, root, src, "(program (export_statement (enum_declaration (identifier) @name) @def))", "enum", &nodes)
	ExtractQueryMatches(language, root, src, "(class_body (method_definition (property_identifier) @name) @def)", "method", &nodes)
	ExtractQueryMatches(language, root, src, "(program (function_declaration (identifier) @name) @def)", "function", &nodes)
	ExtractQueryMatches(language, root, src, "(program (export_statement (function_declaration (identifier) @name) @def))", "function", &nodes)
	ExtractQueryMatches(language, root, src, "(program (lexical_declaration (variable_declarator name: (identifier) @name value: (arrow_function)) @def))", "function", &nodes)
	ExtractQueryMatches(language, root, src, "(program (export_statement (lexical_declaration (variable_declarator name: (identifier) @name value: (arrow_function)) @def)))", "function", &nodes)

	extractTypeScriptRelations(root, nil, &relations, src)

	return DeduplicateNodes(nodes), relations, nil
}

func extractTypeScriptRelations(node *tree_sitter.Node, currentFn *string, relations *[]ParsedRelation, src []byte) {
	if node == nil {
		return
	}

	kind := node.Kind()
	var fnName string
	if currentFn != nil {
		fnName = *currentFn
	}

	if kind == "class_declaration" {
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			fnName = nameNode.Utf8Text(src)
		}
	} else if kind == "function_declaration" || kind == "method_definition" {
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			fnName = nameNode.Utf8Text(src)
		}
	} else if kind == "variable_declarator" {
		valueNode := node.ChildByFieldName("value")
		if valueNode != nil && valueNode.Kind() == "arrow_function" {
			nameNode := node.ChildByFieldName("name")
			if nameNode != nil {
				fnName = nameNode.Utf8Text(src)
			}
		}
	} else if kind == "arrow_function" && fnName == "" {
		parent := node.Parent()
		if parent != nil && parent.Kind() == "variable_declarator" {
			nameNode := parent.ChildByFieldName("name")
			if nameNode != nil {
				fnName = nameNode.Utf8Text(src)
			}
		}
	}

	if kind == "call_expression" && fnName != "" {
		fnNode := node.ChildByFieldName("function")
		if fnNode != nil {
			callee := fnNode.Utf8Text(src)
			if fnNode.Kind() == "member_expression" {
				objNode := fnNode.ChildByFieldName("object")
				propNode := fnNode.ChildByFieldName("property")
				if objNode != nil && propNode != nil && objNode.Kind() == "identifier" {
					callee = objNode.Utf8Text(src) + "." + propNode.Utf8Text(src)
				} else if propNode != nil {
					callee = propNode.Utf8Text(src)
				}
			}
			*relations = append(*relations, ParsedRelation{
				SourceName: fnName,
				TargetName: callee,
				Relation:   "calls",
			})
		}
	} else if kind == "import_declaration" {
		srcNode := node.ChildByFieldName("source")
		if srcNode != nil {
			*relations = append(*relations, ParsedRelation{
				SourceName: "module",
				TargetName: srcNode.Utf8Text(src),
				Relation:   "imports",
			})
		}
	} else if kind == "new_expression" {
		ctorNode := node.ChildByFieldName("constructor")
		if ctorNode != nil {
			ctx := fnName
			if ctx == "" {
				ctx = "module"
			}
			*relations = append(*relations, ParsedRelation{
				SourceName: ctx,
				TargetName: ctorNode.Utf8Text(src),
				Relation:   "calls",
			})
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil {
			extractTypeScriptRelations(child, &fnName, relations, src)
		}
	}
}
