package codegraph

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

type goPlugin struct {
	basePlugin
}

func newGoPlugin() LanguagePlugin {
	return &goPlugin{}
}

func (p *goPlugin) Extensions() []string { return []string{".go"} }
func (p *goPlugin) LanguageName() string { return "go" }

func (p *goPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_go.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

func (p *goPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	parser, language := p.getParser()
	if parser == nil || language == nil {
		return nil, nil, nil
	}

	tree := parser.Parse(src, nil)
	if tree == nil || tree.RootNode() == nil {
		return nil, nil, nil
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	ExtractQueryMatches(language, root, src, "(source_file (function_declaration name: (identifier) @name) @def)", "function", &nodes)
	ExtractQueryMatches(language, root, src, "(source_file (method_declaration name: (field_identifier) @name) @def)", "method", &nodes)
	ExtractQueryMatches(language, root, src, "(source_file (type_declaration (type_spec name: (type_identifier) @name type: (struct_type)) @def))", "struct", &nodes)
	ExtractQueryMatches(language, root, src, "(source_file (type_declaration (type_spec name: (type_identifier) @name type: (interface_type)) @def))", "interface", &nodes)
	ExtractQueryMatches(language, root, src, "(source_file (type_declaration (type_spec name: (type_identifier) @name) @def))", "type", &nodes)
	ExtractQueryMatches(language, root, src, "(interface_type (method_spec name: (field_identifier) @name) @def)", "method", &nodes)

	extractGoRelations(root, nil, &relations, src)

	return DeduplicateNodes(nodes), relations, nil
}

func extractGoRelations(node *tree_sitter.Node, currentFn *string, relations *[]ParsedRelation, src []byte) {
	if node == nil {
		return
	}

	kind := node.Kind()
	var fnName string
	if currentFn != nil {
		fnName = *currentFn
	}

	if kind == "function_declaration" || kind == "method_declaration" {
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			fnName = nameNode.Utf8Text(src)
		}
	}

	if kind == "call_expression" && fnName != "" {
		fnNode := node.ChildByFieldName("function")
		if fnNode != nil {
			callee := fnNode.Utf8Text(src)
			if fnNode.Kind() == "selector_expression" {
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
	} else if kind == "import_spec" {
		pathNode := node.ChildByFieldName("path")
		if pathNode != nil {
			mod := pathNode.Utf8Text(src)
			*relations = append(*relations, ParsedRelation{
				SourceName: "module",
				TargetName: mod,
				Relation:   "imports",
			})
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil {
			extractGoRelations(child, &fnName, relations, src)
		}
	}
}
