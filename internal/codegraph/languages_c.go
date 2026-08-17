package codegraph

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
)

type cPlugin struct {
	basePlugin
}

func newCPlugin() LanguagePlugin {
	return &cPlugin{}
}

func (p *cPlugin) Extensions() []string { return []string{".c"} }
func (p *cPlugin) LanguageName() string { return "c" }

func (p *cPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_c.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

func (p *cPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
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

	// Top-level functions.
	ExtractQueryMatches(language, root, src, "(translation_unit (function_definition declarator: (function_declarator declarator: (identifier) @name)) @def)", "function", &nodes)

	// Functions returning pointers: int *foo(void) { ... }
	ExtractQueryMatches(language, root, src, "(translation_unit (function_definition declarator: (pointer_declarator declarator: (function_declarator declarator: (identifier) @name)) @def))", "function", &nodes)

	// Structs with a body (skip forward declarations).
	ExtractQueryMatches(language, root, src, "(struct_specifier name: (type_identifier) @name body: (field_declaration_list)) @def", "struct", &nodes)

	// Enums.
	ExtractQueryMatches(language, root, src, "(enum_specifier name: (type_identifier) @name body: (enumerator_list)) @def", "enum", &nodes)

	// Typedef'd types.
	ExtractQueryMatches(language, root, src, "(translation_unit (type_definition declarator: (type_identifier) @name) @def)", "type", &nodes)

	extractCRelations(root, nil, &relations, src)

	return DeduplicateNodes(nodes), relations, nil
}

func extractCRelations(node *tree_sitter.Node, currentFn *string, relations *[]ParsedRelation, src []byte) {
	if node == nil {
		return
	}

	kind := node.Kind()
	var fnName string
	if currentFn != nil {
		fnName = *currentFn
	}

	if kind == "function_definition" {
		name := cFunctionName(node, src)
		if name != "" {
			fnName = name
		}
	}

	if kind == "call_expression" && fnName != "" {
		fnNode := node.ChildByFieldName("function")
		if fnNode != nil {
			callee := fnNode.Utf8Text(src)
			// Function pointer calls: (*ptr)(args) -> strip deref and parens.
			callee = trimCPointerCall(callee)
			if callee != "" {
				*relations = append(*relations, ParsedRelation{
					SourceName: fnName,
					TargetName: callee,
					Relation:   "calls",
				})
			}
		}
	} else if kind == "preproc_include" {
		pathNode := node.ChildByFieldName("path")
		if pathNode != nil {
			target := pathNode.Utf8Text(src)
			target = trimQuotesAndAngles(target)
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
			extractCRelations(child, &fnName, relations, src)
		}
	}
}

// cFunctionName extracts the identifier from a function_definition declarator,
// walking through pointer_declarator and function_declarator wrappers.
func cFunctionName(node *tree_sitter.Node, src []byte) string {
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
		case "pointer_declarator":
			decl = decl.ChildByFieldName("declarator")
		case "identifier":
			return decl.Utf8Text(src)
		default:
			return ""
		}
	}
	return ""
}

func trimCPointerCall(s string) string {
	out := s
	for len(out) >= 3 && out[0] == '(' && out[1] == '*' {
		out = out[2:]
		if len(out) > 0 && out[len(out)-1] == ')' {
			out = out[:len(out)-1]
		}
		out = trimSpaceString(out)
	}
	return trimSpaceString(out)
}

func trimQuotesAndAngles(s string) string {
	out := s
	if len(out) > 0 && (out[0] == '<' || out[0] == '"' || out[0] == '\'') {
		out = out[1:]
	}
	if len(out) > 0 && (out[len(out)-1] == '>' || out[len(out)-1] == '"' || out[len(out)-1] == '\'') {
		out = out[:len(out)-1]
	}
	return trimSpaceString(out)
}

func trimSpaceString(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
