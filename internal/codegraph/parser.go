package codegraph

import (
	"bytes"
	"fmt"
	"sort"
	"sync"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	ts_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	ts_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	ts_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
	ts_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	ts_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	ts_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	ts_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	ts_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	ts_lua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"
)

// Language plugins registry
var (
	plugins     map[string]LanguagePlugin
	pluginsOnce sync.Once
)

func initPlugins() {
	plugins = map[string]LanguagePlugin{
		".go":    newGoPlugin(),
		".py":    newPythonPlugin(),
		".ts":    newTypeScriptPlugin(),
		".tsx":   newTypeScriptPlugin(),
		".js":    newTypeScriptPlugin(),
		".jsx":   newTypeScriptPlugin(),
		".mjs":   newTypeScriptPlugin(),
		".rs":    newRustPlugin(),
		".java":  newJavaPlugin(),
		".c":     newCPlugin(),
		".cpp":   newCppPlugin(),
		".h":     newCPlugin(),
		".hpp":   newCppPlugin(),
		".rb":    newRubyPlugin(),
		".lua":   newLuaPlugin(),
	}
}

// Parser wraps tree-sitter parsing for codegraph.
type Parser struct {
	initialized bool
	mu          sync.Mutex
}

// Parse extracts nodes and relations from source code.
func (p *Parser) Parse(ext string, src []byte) ([]ParsedNode, []ParsedRelation, error) {
	pluginsOnce.Do(initPlugins)
	plugin, ok := plugins[ext]
	if !ok {
		return nil, nil, nil
	}
	return plugin.Parse(src)
}

// GetExtensionLanguage returns the language name for an extension.
func (p *Parser) GetExtensionLanguage(ext string) string {
	pluginsOnce.Do(initPlugins)
	if plugin, ok := plugins[ext]; ok {
		return plugin.LanguageName
	}
	return ""
}

// GetSupportedExtensions returns all supported extensions.
func (p *Parser) GetSupportedExtensions() []string {
	pluginsOnce.Do(initPlugins)
	exts := make([]string, 0, len(plugins))
	for ext := range plugins {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return exts
}

// --- Language plugins ---

type goPlugin struct{}

func newGoPlugin() LanguagePlugin {
	return LanguagePlugin{
		Extensions:   []string{".go"},
		LanguageName: "go",
		Parse:        parseGo,
	}
}

func parseGo(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	lang := tree_sitter.NewLanguage(ts_go.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	defer parser.Close()

	tree := parser.Parse(src, nil)
	if tree == nil || tree.RootNode() == nil {
		return nil, nil, fmt.Errorf("go parse failed")
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	extractQueryMatches(lang, root, src, "(source_file (function_declaration name: (identifier) @name) @def)", "function", &nodes)
	extractQueryMatches(lang, root, src, "(source_file (method_declaration name: (field_identifier) @name) @def)", "method", &nodes)
	extractQueryMatches(lang, root, src, "(source_file (type_declaration (type_spec name: (type_identifier) @name type: (struct_type)) @def))", "struct", &nodes)
	extractQueryMatches(lang, root, src, "(source_file (type_declaration (type_spec name: (type_identifier) @name type: (interface_type)) @def))", "interface", &nodes)
	extractQueryMatches(lang, root, src, "(source_file (type_declaration (type_spec name: (type_identifier) @name) @def))", "type", &nodes)
	extractQueryMatches(lang, root, src, "(interface_type (method_spec name: (field_identifier) @name) @def)", "method", &nodes)

	// Relations
	extractGoRelations(root, nil, &relations, src)

	return deduplicateNodes(nodes), relations, nil
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

// --- Python plugin ---

type pythonPlugin struct{}

func newPythonPlugin() LanguagePlugin {
	return LanguagePlugin{
		Extensions:   []string{".py"},
		LanguageName: "python",
		Parse:        parsePython,
	}
}

func parsePython(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	lang := tree_sitter.NewLanguage(ts_python.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	defer parser.Close()

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("python parse failed")
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	extractQueryMatches(lang, root, src, "(module (class_definition name: (identifier) @name) @def)", "class", &nodes)
	extractQueryMatches(lang, root, src, "(module (decorated_definition definition: (class_definition name: (identifier) @name)) @def)", "class", &nodes)
	extractQueryMatches(lang, root, src, "(module (function_definition name: (identifier) @name) @def)", "function", &nodes)
	extractQueryMatches(lang, root, src, "(module (decorated_definition definition: (function_definition name: (identifier) @name)) @def)", "function", &nodes)
	extractQueryMatches(lang, root, src, "(class_definition body: (block (function_definition name: (identifier) @name) @def))", "method", &nodes)
	extractQueryMatches(lang, root, src, "(class_definition body: (block (decorated_definition definition: (function_definition name: (identifier) @name)) @def))", "method", &nodes)

	// Relations
	extractPythonRelations(root, nil, &relations, src)

	return deduplicateNodes(nodes), relations, nil
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

// --- TypeScript/JS plugin ---

type typescriptPlugin struct{}

func newTypeScriptPlugin() LanguagePlugin {
	return LanguagePlugin{
		Extensions:   []string{".ts", ".tsx", ".js", ".jsx", ".mjs"},
		LanguageName: "typescript",
		Parse:        parseTypeScript,
	}
}

func parseTypeScript(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	lang := tree_sitter.NewLanguage(ts_typescript.LanguageTypescript())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	defer parser.Close()

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("typescript parse failed")
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	extractQueryMatches(lang, root, src, "(program (class_declaration (type_identifier) @name) @def)", "class", &nodes)
	extractQueryMatches(lang, root, src, "(program (export_statement (class_declaration (type_identifier) @name) @def))", "class", &nodes)
	extractQueryMatches(lang, root, src, "(program (interface_declaration (type_identifier) @name) @def)", "interface", &nodes)
	extractQueryMatches(lang, root, src, "(program (export_statement (interface_declaration (type_identifier) @name) @def))", "interface", &nodes)
	extractQueryMatches(lang, root, src, "(program (type_alias_declaration (type_identifier) @name) @def)", "type", &nodes)
	extractQueryMatches(lang, root, src, "(program (export_statement (type_alias_declaration (type_identifier) @name) @def))", "type", &nodes)
	extractQueryMatches(lang, root, src, "(program (enum_declaration (identifier) @name) @def)", "enum", &nodes)
	extractQueryMatches(lang, root, src, "(program (export_statement (enum_declaration (identifier) @name) @def))", "enum", &nodes)
	extractQueryMatches(lang, root, src, "(class_body (method_definition (property_identifier) @name) @def)", "method", &nodes)
	extractQueryMatches(lang, root, src, "(program (function_declaration (identifier) @name) @def)", "function", &nodes)
	extractQueryMatches(lang, root, src, "(program (export_statement (function_declaration (identifier) @name) @def))", "function", &nodes)
	extractQueryMatches(lang, root, src, "(program (lexical_declaration (variable_declarator name: (identifier) @name value: (arrow_function)) @def))", "function", &nodes)
	extractQueryMatches(lang, root, src, "(program (export_statement (lexical_declaration (variable_declarator name: (identifier) @name value: (arrow_function)) @def)))", "function", &nodes)

	extractTypeScriptRelations(root, nil, &relations, src)

	return deduplicateNodes(nodes), relations, nil
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

// --- Rust plugin ---

type rustPlugin struct{}

func newRustPlugin() LanguagePlugin {
	return LanguagePlugin{
		Extensions:   []string{".rs"},
		LanguageName: "rust",
		Parse:        parseRust,
	}
}

func parseRust(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	lang := tree_sitter.NewLanguage(ts_rust.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	defer parser.Close()

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("rust parse failed")
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	extractQueryMatches(lang, root, src, "(source_file (struct_item name: (type_identifier) @name) @def)", "struct", &nodes)
	extractQueryMatches(lang, root, src, "(source_file (enum_item name: (type_identifier) @name) @def)", "enum", &nodes)
	extractQueryMatches(lang, root, src, "(source_file (trait_item name: (type_identifier) @name) @def)", "trait", &nodes)
	extractQueryMatches(lang, root, src, "(source_file (type_item name: (type_identifier) @name) @def)", "type", &nodes)
	extractQueryMatches(lang, root, src, "(impl_item body: (declaration_list (function_item name: (identifier) @name) @def))", "method", &nodes)
	extractQueryMatches(lang, root, src, "(trait_item body: (declaration_list (function_item name: (identifier) @name) @def))", "method", &nodes)
	extractQueryMatches(lang, root, src, "(trait_item body: (declaration_list (function_signature_item name: (identifier) @name) @def))", "method", &nodes)
	extractQueryMatches(lang, root, src, "(source_file (function_item name: (identifier) @name) @def)", "function", &nodes)

	extractRustRelations(root, nil, &relations, src)

	return deduplicateNodes(nodes), relations, nil
}

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

// --- Java plugin ---

type javaPlugin struct{}

func newJavaPlugin() LanguagePlugin {
	return LanguagePlugin{
		Extensions:   []string{".java"},
		LanguageName: "java",
		Parse:        parseJava,
	}
}

func parseJava(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	lang := tree_sitter.NewLanguage(ts_java.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	defer parser.Close()

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("java parse failed")
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	extractQueryMatches(lang, root, src, "(program (class_declaration name: (identifier) @name) @def)", "class", &nodes)
	extractQueryMatches(lang, root, src, "(program (interface_declaration name: (identifier) @name) @def)", "interface", &nodes)
	extractQueryMatches(lang, root, src, "(program (enum_declaration name: (identifier) @name) @def)", "enum", &nodes)
	extractQueryMatches(lang, root, src, "(class_body (method_declaration name: (identifier) @name) @def)", "method", &nodes)
	extractQueryMatches(lang, root, src, "(interface_body (method_declaration name: (identifier) @name) @def)", "method", &nodes)

	return deduplicateNodes(nodes), relations, nil
}

// --- C plugin ---

type cPlugin struct{}

func newCPlugin() LanguagePlugin {
	return LanguagePlugin{
		Extensions:   []string{".c", ".h"},
		LanguageName: "c",
		Parse:        parseC,
	}
}

func parseC(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	lang := tree_sitter.NewLanguage(ts_c.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	defer parser.Close()

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("c parse failed")
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	extractQueryMatches(lang, root, src, "(translation_unit (function_definition name: (identifier) @name) @def)", "function", &nodes)
	extractQueryMatches(lang, root, src, "(translation_unit (struct_specifier name: (type_identifier) @name) @def)", "struct", &nodes)

	return deduplicateNodes(nodes), relations, nil
}

// --- C++ plugin ---

type cppPlugin struct{}

func newCppPlugin() LanguagePlugin {
	return LanguagePlugin{
		Extensions:   []string{".cpp", ".hpp"},
		LanguageName: "cpp",
		Parse:        parseCPP,
	}
}

func parseCPP(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	lang := tree_sitter.NewLanguage(ts_cpp.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	defer parser.Close()

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("cpp parse failed")
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	extractQueryMatches(lang, root, src, "(translation_unit (function_definition name: (identifier) @name) @def)", "function", &nodes)
	extractQueryMatches(lang, root, src, "(translation_unit (class_specifier name: (type_identifier) @name) @def)", "class", &nodes)
	extractQueryMatches(lang, root, src, "(class_specifier (field_declaration_list (function_definition name: (identifier) @name) @def))", "method", &nodes)

	return deduplicateNodes(nodes), relations, nil
}

// --- Ruby plugin ---

type rubyPlugin struct{}

func newRubyPlugin() LanguagePlugin {
	return LanguagePlugin{
		Extensions:   []string{".rb"},
		LanguageName: "ruby",
		Parse:        parseRuby,
	}
}

func parseRuby(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	lang := tree_sitter.NewLanguage(ts_ruby.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	defer parser.Close()

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("ruby parse failed")
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	extractQueryMatches(lang, root, src, "(program (class name: (constant) @name) @def)", "class", &nodes)
	extractQueryMatches(lang, root, src, "(program (method name: (identifier) @name) @def)", "method", &nodes)
	extractQueryMatches(lang, root, src, "(program (method name: (constant) @name) @def)", "method", &nodes)

	return deduplicateNodes(nodes), relations, nil
}

// --- Lua plugin ---

type luaPlugin struct{}

func newLuaPlugin() LanguagePlugin {
	return LanguagePlugin{
		Extensions:   []string{".lua"},
		LanguageName: "lua",
		Parse:        parseLua,
	}
}

func parseLua(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	lang := tree_sitter.NewLanguage(ts_lua.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	defer parser.Close()

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("lua parse failed")
	}
	defer tree.Close()

	root := tree.RootNode()
	nodes := make([]ParsedNode, 0)
	relations := make([]ParsedRelation, 0)

	extractQueryMatches(lang, root, src, "(chunk (function_declaration name: (identifier) @name) @def)", "function", &nodes)
	extractQueryMatches(lang, root, src, "(chunk (function_statement name: (identifier) @name) @def)", "function", &nodes)
	extractQueryMatches(lang, root, src, "(chunk (local_function name: (identifier) @name) @def)", "function", &nodes)

	return deduplicateNodes(nodes), relations, nil
}

// --- Shared helpers ---

func extractQueryMatches(lang *tree_sitter.Language, node *tree_sitter.Node, src []byte, queryStr string, symbolType string, nodes *[]ParsedNode) {
	if node == nil || lang == nil {
		return
	}

	query, err := tree_sitter.NewQuery(lang, queryStr)
	if err != nil {
		return
	}
	defer query.Close()

	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	matches := cursor.Matches(query, node, src)
	for {
		m := matches.Next()
		if m == nil {
			break
		}

		nameIdx, _ := query.CaptureIndexForName("name")
		defIdx, _ := query.CaptureIndexForName("def")

		var nameNode *tree_sitter.Node
		var defNode *tree_sitter.Node

		if int(nameIdx) >= 0 {
			captured := m.NodesForCaptureIndex(uint(nameIdx))
			if len(captured) > 0 {
				n := captured[0]
				nameNode = &n
			}
		}
		if int(defIdx) >= 0 {
			captured := m.NodesForCaptureIndex(uint(defIdx))
			if len(captured) > 0 {
				n := captured[0]
				defNode = &n
			}
		}

		if nameNode != nil && defNode != nil {
			name := nameNode.Utf8Text(src)
			sig, doc := extractDetails(defNode, src)
			qn := buildQualifiedName(defNode, name, node, src)

			*nodes = append(*nodes, ParsedNode{
				Type:        symbolType,
				Name:        name,
				QualifiedName: &qn,
				Signature:   sig,
				Docstring:   doc,
				StartLine:   int(defNode.StartPosition().Row) + 1,
				EndLine:     int(defNode.EndPosition().Row) + 1,
				Content:     defNode.Utf8Text(src),
			})
		}
	}
}

func extractDetails(defNode *tree_sitter.Node, src []byte) (string, string) {
	var sig string
	bodyNode := defNode.ChildByFieldName("body")
	if bodyNode == nil {
		bodyNode = defNode.ChildByFieldName("block")
	}
	if bodyNode != nil {
		sig = string(src[defNode.StartByte():bodyNode.StartByte()])
		sig = trimTrailingBraceOrColon(sig)
	} else {
		lines := bytes.Split(src[defNode.StartByte():defNode.EndByte()], []byte("\n"))
		if len(lines) > 0 {
			sig = string(bytesTrimSpace(lines[0]))
		}
	}

	var doc string
	prev := defNode.PrevNamedSibling()
	if prev != nil && (prev.Kind() == "comment" || prev.Kind() == "line_comment" || prev.Kind() == "block_comment") {
		doc = string(bytesTrimSpace([]byte(prev.Utf8Text(src))))
		doc = trimCommentMarkers(doc)
		if idx := bytesIndex([]byte(doc), []byte("\n")); idx >= 0 {
			doc = string(bytesTrimSpace([]byte(doc)[:idx]))
		}
	}

	return sig, doc
}

func trimTrailingBraceOrColon(s string) string {
	b := []byte(s)
	b = bytesTrimSpace(b)
	if len(b) > 0 && b[len(b)-1] == '{' {
		b = b[:len(b)-1]
	}
	if len(b) > 0 && b[len(b)-1] == ':' {
		b = b[:len(b)-1]
	}
	return string(bytesTrimSpace(b))
}

func trimCommentMarkers(s string) string {
	b := []byte(s)
	b = bytesReplaceAll(b, []byte("/*"), []byte(""))
	b = bytesReplaceAll(b, []byte("*/"), []byte(""))
	b = bytesReplaceAll(b, []byte("//"), []byte(""))
	b = bytesReplaceAll(b, []byte("#"), []byte(""))
	return string(bytesTrimSpace(b))
}

func buildQualifiedName(defNode *tree_sitter.Node, name string, root *tree_sitter.Node, src []byte) string {
	scopeTypes := map[string]bool{
		"class_declaration": true, "interface_declaration": true,
		"struct_specifier": true, "enum_specifier": true,
		"impl_item": true, "trait_item": true,
	}
	parts := []string{name}
	cursor := defNode.Parent()
	for cursor != nil && cursor != root {
		if scopeTypes[cursor.Kind()] {
			if nameNode := cursor.ChildByFieldName("name"); nameNode != nil {
				parts = append([]string{nameNode.Utf8Text(src)}, parts...)
			}
		}
		cursor = cursor.Parent()
	}
	if len(parts) > 1 {
		return joinQualified(parts)
	}
	return name
}

func joinQualified(parts []string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += "." + p
	}
	return out
}

func deduplicateNodes(nodes []ParsedNode) []ParsedNode {
	seen := map[string]bool{}
	out := make([]ParsedNode, 0, len(nodes))
	for _, n := range nodes {
		key := fmt.Sprintf("%s:%s:%d", n.Type, n.Name, n.StartLine)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartLine < out[j].StartLine
	})
	return out
}

// bytes helpers to avoid importing extra packages for small ops
func bytesTrimSpace(b []byte) []byte {
	return bytes.TrimSpace(b)
}

func bytesReplaceAll(b, old, new []byte) []byte {
	return bytes.ReplaceAll(b, old, new)
}

func bytesIndex(b, sub []byte) int {
	return bytes.Index(b, sub)
}
