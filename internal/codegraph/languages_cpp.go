package codegraph

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

type cppPlugin struct {
	basePlugin
}

func newCppPlugin() LanguagePlugin {
	return &cppPlugin{}
}

func (p *cppPlugin) Extensions() []string {
	return []string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx", ".h"}
}
func (p *cppPlugin) LanguageName() string { return "cpp" }

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

	// Free functions at top level.
	ExtractQueryMatches(language, root, src, "(translation_unit (function_definition declarator: (function_declarator declarator: (identifier) @name)) @def)", "function", &nodes)

	// Free functions inside namespace blocks.
	ExtractQueryMatches(language, root, src, "(namespace_definition (declaration_list (function_definition declarator: (function_declarator declarator: (identifier) @name)) @def))", "function", &nodes)

	// Structs with a body (skip forward declarations).
	ExtractQueryMatches(language, root, src, "(struct_specifier name: (type_identifier) @name body: (field_declaration_list)) @def", "struct", &nodes)

	// Enums.
	ExtractQueryMatches(language, root, src, "(enum_specifier name: (type_identifier) @name body: (enumerator_list)) @def", "enum", &nodes)

	// Classes (unanchored: matches at any nesting depth, including namespaces).
	ExtractQueryMatches(language, root, src, "(class_specifier name: (type_identifier) @name) @def", "class", &nodes)

	// Inline method definitions inside a class body.
	ExtractQueryMatches(language, root, src, "(class_specifier body: (field_declaration_list (function_definition declarator: (function_declarator declarator: (field_identifier) @name)) @def))", "method", &nodes)

	// Constructors (function name is a plain identifier inside the class body).
	ExtractQueryMatches(language, root, src, "(class_specifier body: (field_declaration_list (function_definition declarator: (function_declarator declarator: (identifier) @name)) @def))", "method", &nodes)

	// Destructors.
	ExtractQueryMatches(language, root, src, "(class_specifier body: (field_declaration_list (function_definition declarator: (function_declarator declarator: (destructor_name) @name)) @def))", "method", &nodes)

	// Out-of-line methods at top level: Type::method(...) { ... }
	ExtractQueryMatches(language, root, src, "(translation_unit (function_definition declarator: (function_declarator declarator: (qualified_identifier name: (identifier) @name)) @def))", "method", &nodes)

	// Out-of-line methods inside namespaces.
	ExtractQueryMatches(language, root, src, "(namespace_definition (declaration_list (function_definition declarator: (function_declarator declarator: (qualified_identifier name: (identifier) @name)) @def)))", "method", &nodes)

	// Namespace declarations.
	ExtractQueryMatches(language, root, src, "(namespace_definition name: (_) @name) @def", "namespace", &nodes)

	extractCppRelations(root, nil, &relations, src)

	return DeduplicateNodes(nodes), relations, nil
}

func extractCppRelations(node *tree_sitter.Node, currentFn *string, relations *[]ParsedRelation, src []byte) {
	if node == nil {
		return
	}

	kind := node.Kind()
	var fnName string
	if currentFn != nil {
		fnName = *currentFn
	}

	if kind == "function_definition" {
		name := cppFunctionName(node, src)
		if name != "" {
			fnName = name
		}
	}

	if kind == "call_expression" && fnName != "" {
		fnNode := node.ChildByFieldName("function")
		if fnNode != nil {
			callee := ""
			switch fnNode.Kind() {
			case "field_expression":
				if f := fnNode.ChildByFieldName("field"); f != nil {
					callee = f.Utf8Text(src)
				}
			case "qualified_identifier":
				if n := fnNode.ChildByFieldName("name"); n != nil {
					callee = n.Utf8Text(src)
				}
			case "pointer_expression":
				callee = fnNode.Utf8Text(src)
			default:
				callee = fnNode.Utf8Text(src)
			}
			if callee != "" {
				*relations = append(*relations, ParsedRelation{
					SourceName: fnName,
					TargetName: callee,
					Relation:   "calls",
				})
			}
		}
	} else if kind == "new_expression" && fnName != "" {
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
	} else if kind == "using_declaration" {
		if pathNode := node.NamedChild(0); pathNode != nil {
			*relations = append(*relations, ParsedRelation{
				SourceName: "",
				TargetName: pathNode.Utf8Text(src),
				Relation:   "imports",
			})
		}
	} else if kind == "preproc_include" {
		pathNode := node.ChildByFieldName("path")
		if pathNode != nil {
			target := trimQuotesAndAngles(pathNode.Utf8Text(src))
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
			extractCppRelations(child, &fnName, relations, src)
		}
	}
}

// cppFunctionName extracts the function name from a function_definition
// declarator, handling pointer_declarator / function_declarator wrappers,
// field_identifier (methods), identifier (ctors), and qualified_identifier.
func cppFunctionName(node *tree_sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	decl := node.ChildByFieldName("declarator")
	if decl == nil {
		return ""
	}
	for decl != nil {
		switch decl.Kind() {
		case "function_declarator":
			decl = decl.ChildByFieldName("declarator")
		case "pointer_declarator", "reference_declarator":
			decl = decl.ChildByFieldName("declarator")
		case "identifier", "field_identifier", "destructor_name":
			return decl.Utf8Text(src)
		case "qualified_identifier":
			if n := decl.ChildByFieldName("name"); n != nil {
				return n.Utf8Text(src)
			}
			return decl.Utf8Text(src)
		default:
			return ""
		}
	}
	return ""
}
