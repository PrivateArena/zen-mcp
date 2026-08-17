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

	// Top-level classes, interfaces, and enums.
	ExtractQueryMatches(language, root, src, "(program (class_declaration name: (identifier) @name) @def)", "class", &nodes)
	ExtractQueryMatches(language, root, src, "(program (interface_declaration name: (identifier) @name) @def)", "interface", &nodes)
	ExtractQueryMatches(language, root, src, "(program (enum_declaration name: (identifier) @name) @def)", "enum", &nodes)

	// Methods declared in class / interface / enum bodies.
	ExtractQueryMatches(language, root, src, "(class_body (method_declaration name: (identifier) @name) @def)", "method", &nodes)
	ExtractQueryMatches(language, root, src, "(interface_body (method_declaration name: (identifier) @name) @def)", "method", &nodes)
	ExtractQueryMatches(language, root, src, "(enum_body_declarations (method_declaration name: (identifier) @name) @def)", "method", &nodes)

	// Constructors.
	ExtractQueryMatches(language, root, src, "(class_body (constructor_declaration name: (identifier) @name) @def)", "method", &nodes)

	extractJavaRelations(root, nil, &relations, src)

	return DeduplicateNodes(nodes), relations, nil
}

func extractJavaRelations(node *tree_sitter.Node, currentFn *string, relations *[]ParsedRelation, src []byte) {
	if node == nil {
		return
	}

	kind := node.Kind()
	var fnName string
	if currentFn != nil {
		fnName = *currentFn
	}

	if kind == "method_declaration" || kind == "constructor_declaration" {
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			fnName = nameNode.Utf8Text(src)
		}
	}

	if kind == "method_invocation" && fnName != "" {
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			callee := nameNode.Utf8Text(src)
			if callee != "" {
				*relations = append(*relations, ParsedRelation{
					SourceName: fnName,
					TargetName: callee,
					Relation:   "calls",
				})
			}
		}
	} else if kind == "object_creation_expression" && fnName != "" {
		typeNode := node.ChildByFieldName("type")
		if typeNode != nil {
			target := typeNode.Utf8Text(src)
			if target != "" {
				*relations = append(*relations, ParsedRelation{
					SourceName: fnName,
					TargetName: target,
					Relation:   "calls",
				})
			}
		}
	} else if kind == "import_declaration" {
		target := trimSpaceString(node.Utf8Text(src))
		target = trimJavaImport(target)
		if target != "" {
			*relations = append(*relations, ParsedRelation{
				SourceName: "",
				TargetName: target,
				Relation:   "imports",
			})
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil {
			extractJavaRelations(child, &fnName, relations, src)
		}
	}
}

func trimJavaImport(s string) string {
	// Strip "import " prefix and trailing ';' / ".*" wildcards.
	out := s
	if len(out) >= 7 && out[:7] == "import " {
		out = out[7:]
	}
	if len(out) > 0 && out[len(out)-1] == ';' {
		out = out[:len(out)-1]
	}
	return trimSpaceString(out)
}