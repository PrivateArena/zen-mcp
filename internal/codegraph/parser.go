package codegraph

import (
	"bytes"
	"fmt"
	"sort"
	"sync"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Parser wraps tree-sitter parsing for codegraph.
type Parser struct {
	plugins map[string]LanguagePlugin
}

var parserOnce sync.Once
var globalParser *Parser

// GetParser returns the singleton parser instance.
func GetParser() *Parser {
	parserOnce.Do(func() {
		globalParser = &Parser{
			plugins: map[string]LanguagePlugin{
				".go":   newGoPlugin(),
				".py":   newPythonPlugin(),
				".ts":   newTypeScriptPlugin(),
				".tsx":  newTypeScriptPlugin(),
				".js":   newTypeScriptPlugin(),
				".jsx":  newTypeScriptPlugin(),
				".mjs":  newTypeScriptPlugin(),
				".rs":   newRustPlugin(),
				".java": newJavaPlugin(),
				".c":    newCPlugin(),
				".cpp":  newCppPlugin(),
				".h":    newCPlugin(),
				".hpp":  newCppPlugin(),
				".rb":   newRubyPlugin(),
				".lua":  newLuaPlugin(),
			},
		}
	})
	return globalParser
}

// Parse extracts nodes and relations from source code.
func (p *Parser) Parse(ext string, src []byte) ([]ParsedNode, []ParsedRelation, error) {
	plugin, ok := p.plugins[ext]
	if !ok {
		return nil, nil, nil
	}
	if err := plugin.Init(); err != nil {
		return nil, nil, err
	}
	return plugin.Parse(src)
}

// GetExtensionLanguage returns the language name for an extension.
func (p *Parser) GetExtensionLanguage(ext string) string {
	if plugin, ok := p.plugins[ext]; ok {
		return plugin.LanguageName()
	}
	return ""
}

// GetSupportedExtensions returns all supported extensions.
func (p *Parser) GetSupportedExtensions() []string {
	exts := make([]string, 0, len(p.plugins))
	for ext := range p.plugins {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return exts
}

// --- Shared helpers ---

func ExtractQueryMatches(lang *tree_sitter.Language, node *tree_sitter.Node, src []byte, queryStr string, symbolType string, nodes *[]ParsedNode) {
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
				Type:          symbolType,
				Name:          name,
				QualifiedName: &qn,
				Signature:     sig,
				Docstring:     doc,
				StartLine:     int(defNode.StartPosition().Row) + 1,
				EndLine:       int(defNode.EndPosition().Row) + 1,
				Content:       defNode.Utf8Text(src),
			})
		}
	}
}

// extracts detailed information from syntax tree nodes
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

// trims trailing braces or colons from a string
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

// trims comment markers from a string
func trimCommentMarkers(s string) string {
	b := []byte(s)
	b = bytesReplaceAll(b, []byte("/*"), []byte(""))
	b = bytesReplaceAll(b, []byte("*/"), []byte(""))
	b = bytesReplaceAll(b, []byte("//"), []byte(""))
	b = bytesReplaceAll(b, []byte("#"), []byte(""))
	return string(bytesTrimSpace(b))
}

// builds a qualified name from node parts
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

// joinQualified is a helper function
func joinQualified(parts []string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += "." + p
	}
	return out
}

// DeduplicateNodes is a helper function
func DeduplicateNodes(nodes []ParsedNode) []ParsedNode {
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

// bytesReplaceAll is a helper function
func bytesReplaceAll(b, old, new []byte) []byte {
	return bytes.ReplaceAll(b, old, new)
}

// bytesIndex is a helper function
func bytesIndex(b, sub []byte) int {
	return bytes.Index(b, sub)
}
