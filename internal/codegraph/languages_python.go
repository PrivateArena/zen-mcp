package codegraph

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

type pythonPlugin struct {
	basePlugin
}

func newPythonPlugin() LanguagePlugin {
	return &pythonPlugin{}
}

func (p *pythonPlugin) Extensions() []string { return []string{".py"} }
func (p *pythonPlugin) LanguageName() string { return "python" }

func (p *pythonPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_python.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

func (p *pythonPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
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

	ExtractQueryMatches(language, root, src, "(module (class_definition name: (identifier) @name) @def)", "class", &nodes)
	ExtractQueryMatches(language, root, src, "(module (decorated_definition definition: (class_definition name: (identifier) @name)) @def)", "class", &nodes)
	ExtractQueryMatches(language, root, src, "(module (function_definition name: (identifier) @name) @def)", "function", &nodes)
	ExtractQueryMatches(language, root, src, "(module (decorated_definition definition: (function_definition name: (identifier) @name)) @def)", "function", &nodes)
	ExtractQueryMatches(language, root, src, "(class_definition body: (block (function_definition name: (identifier) @name) @def))", "method", &nodes)
	ExtractQueryMatches(language, root, src, "(class_definition body: (block (decorated_definition definition: (function_definition name: (identifier) @name)) @def))", "method", &nodes)

	extractPythonRelations(root, nil, &relations, src)

	return DeduplicateNodes(nodes), relations, nil
}

func extractPythonRelations(node *tree_sitter.Node, currentFn *string, relations *[]ParsedRelation, src []byte) {
	if node == nil {
		return
	}

	kind := node.Kind()
	var fnName string
	if currentFn != nil {
		fnName = *currentFn
	}

	if kind == "function_definition" {
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			fnName = nameNode.Utf8Text(src)
		}
	}

	if kind == "call" && fnName != "" {
		fnNode := node.ChildByFieldName("function")
		if fnNode != nil {
			callee := fnNode.Utf8Text(src)
			if fnNode.Kind() == "attribute" {
				attrNode := fnNode.ChildByFieldName("attribute")
				if attrNode != nil {
					callee = attrNode.Utf8Text(src)
				}
			}
			*relations = append(*relations, ParsedRelation{
				SourceName: fnName,
				TargetName: callee,
				Relation:   "calls",
			})
		}
	} else if kind == "import_from_statement" {
		modNode := node.ChildByFieldName("module_name")
		if modNode != nil {
			*relations = append(*relations, ParsedRelation{
				SourceName: "module",
				TargetName: modNode.Utf8Text(src),
				Relation:   "imports",
			})
		}
	} else if kind == "import_statement" {
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child == nil {
				continue
			}
			if child.Kind() == "dotted_name" {
				*relations = append(*relations, ParsedRelation{
					SourceName: "module",
					TargetName: child.Utf8Text(src),
					Relation:   "imports",
				})
			} else if child.Kind() == "aliased_import" {
				nameNode := child.ChildByFieldName("name")
				if nameNode != nil {
					*relations = append(*relations, ParsedRelation{
						SourceName: "module",
						TargetName: nameNode.Utf8Text(src),
						Relation:   "imports",
					})
				}
			}
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil {
			extractPythonRelations(child, &fnName, relations, src)
		}
	}
}
