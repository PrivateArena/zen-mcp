package codegraph

import (
	"strings"
)

// Godot wires a script into the running game from project.godot instead of with an
// import statement: an `[autoload]` entry such as
//
//	GameData="*res://scripts/autoload/game_data.gd"
//
// makes the identifier `GameData` legal in every script of the project, and a
// GDScript file never names that relationship — the two files are linked only
// through the manifest. The manifest therefore contributes the symbols the
// gdscript plugin's receiver references resolve to, plus the entry-point scene and
// the enabled editor plugins.

// parseGodotProject reads the singleton and scene declarations out of a
// project.godot (Godot's INI-dialect configuration file).
func parseGodotProject(content string) []ParsedNode {
	var nodes []ParsedNode
	section := ""

	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)

		switch {
		case section == "autoload":
			nodes = append(nodes, godotAutoloadNode(key, value, i+1))
		case section == "application" && key == "run/main_scene":
			if path := godotResourcePath(value); path != "" {
				nodes = append(nodes, godotResourceNode("scene", path, "main scene", line, i+1))
			}
		case section == "editor_plugins" && key == "enabled":
			for _, cfg := range godotResourceLists(value) {
				nodes = append(nodes, godotResourceNode("plugin", cfg, "editor plugin", line, i+1))
			}
		}
	}

	return nodes
}

// godotAutoloadNode records one singleton. Its name is deliberately the bare global
// identifier, because that is what call sites in the scripts reference.
func godotAutoloadNode(name, value string, line int) ParsedNode {
	path := strings.TrimSpace(value)
	enabled := strings.HasPrefix(strings.Trim(path, `"`), "*")
	clean := godotResourcePath(path)

	signature := "autoload " + clean
	if !enabled {
		signature += " (disabled)"
	}
	qualified := "godot:" + name

	return ParsedNode{
		Type:          "autoload",
		Name:          name,
		QualifiedName: &qualified,
		Signature:     signature,
		Docstring:     "Godot autoload singleton, reachable as a global in every script.",
		StartLine:     line,
		EndLine:       line,
		Content:       name + "=" + strings.TrimSpace(value),
	}
}

func godotResourceNode(nodeType, path, label, content string, line int) ParsedNode {
	qualified := "godot:" + path
	return ParsedNode{
		Type:          nodeType,
		Name:          path,
		QualifiedName: &qualified,
		Signature:     label + " " + path,
		StartLine:     line,
		EndLine:       line,
		Content:       content,
	}
}

// godotResourcePath strips the quoting and the res:// scheme, leaving the
// project-relative path the scanner indexes.
func godotResourcePath(value string) string {
	v := strings.TrimSpace(value)
	v = strings.Trim(v, `"`)
	v = strings.TrimPrefix(v, "*")
	v = strings.TrimPrefix(v, "res://")
	return strings.TrimSpace(v)
}

// godotResourceLists collects the quoted resource paths of a PackedStringArray value.
func godotResourceLists(value string) []string {
	var out []string
	for _, part := range strings.Split(value, `"`) {
		if path := godotResourcePath(part); path != "" && strings.Contains(path, "/") {
			out = append(out, path)
		}
	}
	return out
}
