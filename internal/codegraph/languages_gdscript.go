package codegraph

import (
	"strings"

	ts_gdscript "github.com/prestonknopp/tree-sitter-gdscript/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// gdscriptPlugin indexes Godot 4 GDScript sources.
//
// A .gd file is one class: `class_name Foo` registers the script as a global
// type, `extends Bar` names its base, and every top-level func/var/const belongs
// to that class — so declarations hang off the `source` root node rather than off
// a class node the way they do in Java or C#. Relations are read off the chain
// shapes this grammar produces for member access (`a.b.c()` is an `attribute` node
// wrapping one `attribute_call` per invocation), which is why extraction is one
// scope-tracking walk instead of per-kind queries.
type gdscriptPlugin struct {
	basePlugin
}

func newGdscriptPlugin() LanguagePlugin {
	return &gdscriptPlugin{}
}

func (p *gdscriptPlugin) Extensions() []string { return []string{".gd"} }
func (p *gdscriptPlugin) LanguageName() string { return "gdscript" }

func (p *gdscriptPlugin) Init() error {
	p.mu.Lock()
	if p.parser != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	lang := tree_sitter.NewLanguage(ts_gdscript.Language())
	parser := tree_sitter.NewParser()
	parser.SetLanguage(lang)
	p.setParser(parser, lang)
	return nil
}

func (p *gdscriptPlugin) Parse(src []byte) ([]ParsedNode, []ParsedRelation, error) {
	parser, _ := p.getParser()
	if parser == nil {
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

	// The script's own `class_name` is the scope of every top-level declaration,
	// including the `extends` line that follows it.
	walkGdscript(root, root, gdscriptScope{class: gdFileClassName(root, src)}, &nodes, &relations, src)

	return DeduplicateNodes(nodes), DeduplicateRelations(relations), nil
}

// gdFileClassName returns the `class_name` declared by the script itself, or "" for
// a plain `extends Node` script that registers no global type.
func gdFileClassName(root *tree_sitter.Node, src []byte) string {
	for i := uint(0); i < root.NamedChildCount(); i++ {
		if child := root.NamedChild(i); child != nil && child.Kind() == "class_name_statement" {
			return gdNodeName(child, src)
		}
	}
	return ""
}

// gdscriptScope is the declaration context of the node being visited: the
// innermost function a reference sits in (so `World.tick calls load_settings`
// attributes to `tick`, not to the file) and the innermost class, which names
// class-scope statements such as `const Bot = preload(…)`.
type gdscriptScope struct {
	fn    string
	class string
}

// sourceName is the symbol a relation emitted in this scope belongs to.
// Class-scope statements fall back to the class name, and file-scope ones to
// "module", which never resolves into an edge but keeps `imports` records valid.
func (s gdscriptScope) sourceName() string {
	switch {
	case s.fn != "":
		return s.fn
	case s.class != "":
		return s.class
	default:
		return "module"
	}
}

// gdSignalAnchors are the methods that name their receiver rather than their
// argument, so `gear_in.emit(…)` has to point at the `gear_in` signal.
var gdSignalAnchors = map[string]bool{"emit": true, "connect": true, "disconnect": true}

func walkGdscript(node, root *tree_sitter.Node, scope gdscriptScope, nodes *[]ParsedNode, relations *[]ParsedRelation, src []byte) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "class_name_statement":
		if name := gdNodeName(node, src); name != "" {
			appendGdNode(nodes, "class", name, node, root, src)
			gdClassSuffix(nodes, gdExtends(node, name, relations, src)) // `class_name X extends Y` inlines the base
			walkGdscriptChildren(node, root, gdscriptScope{class: name}, nodes, relations, src)
			return
		}

	case "class_definition":
		if name := gdNodeName(node, src); name != "" {
			appendGdNode(nodes, "class", name, node, root, src)
			gdClassSuffix(nodes, gdExtends(node, name, relations, src))
			walkGdscriptChildren(node, root, gdscriptScope{class: name}, nodes, relations, src)
			return
		}

	case "function_definition", "constructor_definition":
		name := gdNodeName(node, src)
		if name == "" && node.Kind() == "constructor_definition" {
			name = "_init" // the grammar gives constructors no name field
		}
		if name != "" {
			appendGdNode(nodes, gdFunctionType(node), name, node, root, src)
			walkGdscriptChildren(node, root, gdscriptScope{fn: name, class: scope.class}, nodes, relations, src)
			return
		}

	case "extends_statement":
		// A standalone `extends Foo`, on the line after `class_name Bar`. The base
		// written as part of a class declaration is handled there instead.
		if parent := node.Parent(); parent != nil && (parent.Kind() == "source" || parent.Kind() == "class_body") {
			if name := scope.class; name != "" {
				gdClassSuffix(nodes, gdExtends(node, name, relations, src))
			}
		}
		return

	case "signal_statement":
		if name := gdNodeName(node, src); name != "" {
			appendGdNode(nodes, "signal", name, node, root, src)
		}

	case "enum_definition":
		if name := gdNodeName(node, src); name != "" {
			appendGdNode(nodes, "enum", name, node, root, src)
		}

	case "const_statement", "variable_statement":
		// Only member-level declarations become symbols; indexing every
		// function-local `var` would bury a 20k-line script's skeleton in noise.
		if gdIsMemberDeclaration(node) {
			if name := gdNodeName(node, src); name != "" {
				kind := "variable"
				if node.Kind() == "const_statement" {
					kind = "constant"
				}
				appendGdNode(nodes, kind, name, node, root, src)
			}
		}

	case "type":
		// `var w: World`, `func f() -> Array[Player]`: a type name is a reference
		// to a `class_name` type. Engine builtins resolve to no node at all and
		// are dropped when edges are built.
		if target := gdTypeName(node, src); target != "" {
			gdReference(relations, scope, target, "type")
		}
		return

	case "call":
		gdHandleCall(node, scope, relations, src)

	case "attribute":
		gdHandleChain(node, scope, relations, src)
		gdWalkChainOperands(node, root, scope, nodes, relations, src)
		return
	}

	walkGdscriptChildren(node, root, scope, nodes, relations, src)
}

func walkGdscriptChildren(node, root *tree_sitter.Node, scope gdscriptScope, nodes *[]ParsedNode, relations *[]ParsedRelation, src []byte) {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkGdscript(node.NamedChild(i), root, scope, nodes, relations, src)
	}
}

// gdWalkChainOperands descends into the parts of an `a.b().c()` chain that can
// hold expressions of their own — argument lists and non-identifier heads such as
// `items[0]` or `get_tree()`. The chain's plain identifiers are already named by
// gdHandleChain.
func gdWalkChainOperands(node, root *tree_sitter.Node, scope gdscriptScope, nodes *[]ParsedNode, relations *[]ParsedRelation, src []byte) {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "identifier", "string":
		case "attribute_call":
			if args := child.ChildByFieldName("arguments"); args != nil {
				walkGdscript(args, root, scope, nodes, relations, src)
			}
		default:
			walkGdscript(child, root, scope, nodes, relations, src)
		}
	}
}

// gdHandleChain records the edges of a dotted access: `Save.data` references both
// the `Save` receiver and the `data` member, `GameData.set_meta(…)` calls
// `set_meta`, `Menus.new()` builds a `Menus`, and `peer_joined.emit(id)` references
// the `peer_joined` signal.
func gdHandleChain(node *tree_sitter.Node, scope gdscriptScope, relations *[]ParsedRelation, src []byte) {
	count := node.NamedChildCount()
	head, _ := gdChainName(node.NamedChild(0), src)
	if head == "self" || head == "super" {
		head = ""
	}

	var previous, last *tree_sitter.Node
	hadCall, headReferenced := false, false

	for i := uint(0); i < count; i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		last = child

		if child.Kind() != "attribute_call" {
			previous = child
			continue
		}
		hadCall = true

		method := gdCalleeName(child, src)
		if method == "" {
			continue
		}
		gdCall(relations, scope, method)
		if args := child.ChildByFieldName("arguments"); args != nil {
			gdArgReferences(args, scope, relations, src)
		}

		receiver, _ := gdChainName(previous, src)
		switch {
		case gdSignalAnchors[method] && receiver != "":
			// `peer_joined.emit(id)` — the signal is the receiver, not the argument.
			gdReference(relations, scope, receiver, "signal")
			headReferenced = receiver == head
		case method == "new" && i+1 == count && head != "":
			// `Menus.new()` constructs the class the chain is rooted in.
			gdReference(relations, scope, head, "constructor")
			headReferenced = true
		}
	}

	if !hadCall {
		if name, ok := gdChainName(last, src); ok {
			gdReference(relations, scope, name, "member")
		}
	}

	if !headReferenced && head != "" && count > 1 {
		// Autoload singletons and engine classes have no indexed node to resolve to,
		// so an unresolvable receiver relation costs a name lookup, never a bogus edge.
		gdReference(relations, scope, head, "receiver")
	}
}

// gdChainName names the symbol a chain operand stands for: an identifier, or the
// member being indexed in `GameData.ITEMS[0]` (an `attribute_subscript`) and in
// `queue[0]` (a `subscript`).
func gdChainName(node *tree_sitter.Node, src []byte) (string, bool) {
	if node == nil {
		return "", false
	}
	switch node.Kind() {
	case "identifier":
		return node.Utf8Text(src), true
	case "subscript", "attribute_subscript":
		if first := node.NamedChild(0); first != nil && first.Kind() == "identifier" {
			return first.Utf8Text(src), true
		}
	}
	return "", false
}

// gdArgReferences records the bare identifiers handed to a call, which is how a
// method reference reaches the graph: `pressed.connect(_on_pressed)`,
// `add_child(Menu)`.
func gdArgReferences(args *tree_sitter.Node, scope gdscriptScope, relations *[]ParsedRelation, src []byte) {
	for i := uint(0); i < args.NamedChildCount(); i++ {
		if arg := args.NamedChild(i); arg != nil && arg.Kind() == "identifier" {
			gdReference(relations, scope, arg.Utf8Text(src), "argument")
		}
	}
}

// gdHandleCall records a plain `name(args)` call plus the symbol arguments it is
// handed (`add_child(Menu)`, `button.pressed.connect(_on_pressed)`).
func gdHandleCall(node *tree_sitter.Node, scope gdscriptScope, relations *[]ParsedRelation, src []byte) {
	callee := gdCalleeName(node, src)
	if callee == "" {
		return
	}

	args := node.ChildByFieldName("arguments")
	if callee == "preload" || callee == "load" {
		// Godot has no import statement: a script pulls another in with
		// preload("res://path.gd"). The res:// specifier is kept verbatim — it is
		// the project-root-relative path with a prefix the importer matchers strip.
		if path := gdStringLiteral(args, src); path != "" {
			*relations = append(*relations, ParsedRelation{
				SourceName: scope.sourceName(),
				TargetName: path,
				Relation:   "imports",
			})
			return
		}
	}

	gdCall(relations, scope, callee)
	if args == nil {
		return
	}
	gdArgReferences(args, scope, relations, src)
}

func gdCall(relations *[]ParsedRelation, scope gdscriptScope, target string) {
	*relations = append(*relations, ParsedRelation{
		SourceName: scope.sourceName(),
		TargetName: target,
		Relation:   "calls",
	})
}

func gdReference(relations *[]ParsedRelation, scope gdscriptScope, target, metadata string) {
	if target == "" || target == scope.fn || target == scope.class {
		return
	}
	*relations = append(*relations, ParsedRelation{
		SourceName: scope.sourceName(),
		TargetName: target,
		Relation:   "references",
		Metadata:   metadata,
	})
}

// gdExtends links a class to its base type, which is how a script that extends a
// custom `class_name` type gets a real cross-file edge (`Pushcart` → `RoVisual`).
// It returns the base name, empty when the script declares no base.
func gdExtends(node *tree_sitter.Node, className string, relations *[]ParsedRelation, src []byte) string {
	if className == "" {
		return ""
	}
	extends := node.ChildByFieldName("extends")
	if extends == nil && node.Kind() == "extends_statement" {
		extends = node // a standalone statement carries the base type itself
	}
	if extends == nil {
		return ""
	}
	base := gdExtendsBase(extends, src)
	if base == "" {
		return ""
	}
	*relations = append(*relations, ParsedRelation{
		SourceName: className,
		TargetName: base,
		Relation:   "extends",
	})
	return base
}

// gdClassSuffix appends the base type to the class symbol recorded just before,
// so a skeleton line reads `class_name Pushcart extends RoVisual` instead of
// hiding the inheritance behind a separate edge.
func gdClassSuffix(nodes *[]ParsedNode, base string) {
	if base == "" || len(*nodes) == 0 {
		return
	}
	last := &(*nodes)[len(*nodes)-1]
	last.Signature += " extends " + base
}

func gdExtendsBase(extends *tree_sitter.Node, src []byte) string {
	// `extends_statement` exposes no field names, so the type child is found by kind.
	for i := uint(0); i < extends.NamedChildCount(); i++ {
		if child := extends.NamedChild(i); child != nil && child.Kind() == "type" {
			return gdTypeName(child, src)
		}
	}
	return ""
}

// gdTypeName renders a `type` node as the bare dotted name GDScript spells:
// `World`, `EOS.Result`, or the container of `Array[Player]` → `Array`.
func gdTypeName(node *tree_sitter.Node, src []byte) string {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "identifier":
			return child.Utf8Text(src)
		case "attribute":
			return strings.TrimSpace(child.Utf8Text(src))
		case "subscript":
			if head := child.NamedChild(0); head != nil && head.Kind() == "identifier" {
				return head.Utf8Text(src)
			}
		}
		return ""
	}
	return ""
}

// gdCalleeName extracts the invoked name from a `call` (leading expression) or an
// `attribute_call` (the identifier immediately before the argument list).
func gdCalleeName(node *tree_sitter.Node, src []byte) string {
	if node.Kind() == "call" {
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child == nil || child.Kind() == "arguments" {
				continue
			}
			if child.Kind() == "identifier" {
				return child.Utf8Text(src)
			}
			return ""
		}
		return ""
	}

	var name string
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil && child.Kind() == "identifier" {
			name = child.Utf8Text(src)
		}
	}
	return name
}

// gdStringLiteral returns the unquoted text of the first string argument, which is
// what turns `preload("res://scripts/core/foo.gd")` into an indexable import.
func gdStringLiteral(args *tree_sitter.Node, src []byte) string {
	if args == nil {
		return ""
	}
	for i := uint(0); i < args.NamedChildCount(); i++ {
		if child := args.NamedChild(i); child != nil && child.Kind() == "string" {
			return trimQuotesAndAngles(child.Utf8Text(src))
		}
	}
	return ""
}

func gdNodeName(node *tree_sitter.Node, src []byte) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	return strings.TrimSpace(nameNode.Utf8Text(src))
}

// gdFunctionType separates methods declared in an inner `class` body from the
// top-level functions of the script.
func gdFunctionType(node *tree_sitter.Node) string {
	if parent := node.Parent(); parent != nil && parent.Kind() == "class_body" {
		return "method"
	}
	return "function"
}

// gdIsMemberDeclaration reports whether a `var`/`const` sits at script or inner
// class scope rather than inside a function body.
func gdIsMemberDeclaration(node *tree_sitter.Node) bool {
	parent := node.Parent()
	if parent == nil {
		return false
	}
	return parent.Kind() == "source" || parent.Kind() == "class_body"
}

// appendGdNode records a symbol with the same signature, docstring and
// qualified-name treatment ExtractQueryMatches gives the query-driven plugins.
func appendGdNode(nodes *[]ParsedNode, nodeType, name string, defNode, root *tree_sitter.Node, src []byte) {
	sig, doc := extractDetails(defNode, src)
	if annotations, preamble := gdDeclPreamble(defNode, src); annotations != "" {
		sig = annotations + " " + sig
		if doc == "" {
			doc = preamble
		}
	} else if preamble != "" {
		doc = preamble
	}
	qn := buildQualifiedName(defNode, name, root, src)
	*nodes = append(*nodes, ParsedNode{
		Type:          nodeType,
		Name:          name,
		QualifiedName: &qn,
		Signature:     sig,
		Docstring:     doc,
		StartLine:     int(defNode.StartPosition().Row) + 1,
		EndLine:       int(defNode.EndPosition().Row) + 1,
		Content:       defNode.Utf8Text(src),
	})
}

// gdDeclPreamble walks the siblings stacked above a declaration. GDScript puts
// annotations *in front of* the declaration as siblings, so a doc comment above
// `@rpc(...) func _m()` is two siblings away — the plain previous-sibling lookup
// would only ever see the annotation.
func gdDeclPreamble(node *tree_sitter.Node, src []byte) (annotations, doc string) {
	var stacked []string
	for prev := node.PrevNamedSibling(); prev != nil; prev = prev.PrevNamedSibling() {
		switch prev.Kind() {
		case "annotation":
			stacked = append([]string{gdOneLine(prev.Utf8Text(src))}, stacked...)
		case "comment", "line_comment", "block_comment":
			if doc == "" {
				doc = strings.TrimSpace(trimCommentMarkers(gdOneLine(prev.Utf8Text(src))))
			}
			return strings.Join(stacked, " "), doc
		default:
			return strings.Join(stacked, " "), doc
		}
	}
	return strings.Join(stacked, " "), doc
}

func gdOneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
