package codegraph

import (
	"testing"
)

const godotConfigSample = `; Engine configuration file.
config_version=5

[application]

config/name="Midgard Survivors"
run/main_scene="res://scenes/main.tscn"
config/features=PackedStringArray("4.7", "GL Compatibility")

[autoload]

Updater="*res://scripts/autoload/updater.gd"
GameData="*res://scripts/autoload/game_data.gd"
LegacySave="res://scripts/legacy_save.gd"

[internationalization]

locale/translation_remaps={
"res://assets/ui/btn.png": PackedStringArray("res://assets/en/btn.png:en", "res://assets/pt/btn.png:pt")
}

[editor_plugins]

enabled=PackedStringArray("res://addons/probe/plugin.cfg")
`

func TestParseGodotProject(t *testing.T) {
	nodes := parseGodotProject(godotConfigSample)

	wantType := map[string]string{
		"Updater":                 "autoload",
		"GameData":                "autoload",
		"LegacySave":              "autoload",
		"scenes/main.tscn":        "scene",
		"addons/probe/plugin.cfg": "plugin",
	}
	for name, nodeType := range wantType {
		n := nodeTypeOf(findGodotNode(nodes, name))
		if n == "" {
			t.Errorf("%s not indexed", name)
			continue
		}
		if n != nodeType {
			t.Errorf("%s type = %q, want %q", name, n, nodeType)
		}
	}

	if s := signatureOf(findGodotNode(nodes, "GameData")); s != "autoload scripts/autoload/game_data.gd" {
		t.Errorf("enabled autoload signature = %q", s)
	}
	if s := signatureOf(findGodotNode(nodes, "LegacySave")); s != "autoload scripts/legacy_save.gd (disabled)" {
		t.Errorf("disabled autoload signature = %q", s)
	}
	if q := qualifiedOf(findGodotNode(nodes, "GameData")); q != "godot:GameData" {
		t.Errorf("qualified name = %q", q)
	}

	for _, noise := range []string{"config/name", "locale/translation_remaps", "btn.png", "4.7"} {
		if findGodotNode(nodes, noise) != nil {
			t.Errorf("%q must not become a symbol", noise)
		}
	}
	if len(nodes) != len(wantType) {
		t.Errorf("got %d symbols, want %d: %+v", len(nodes), len(wantType), nodes)
	}
}

func findGodotNode(nodes []ParsedNode, name string) *ParsedNode {
	for i := range nodes {
		if nodes[i].Name == name {
			return &nodes[i]
		}
	}
	return nil
}

func nodeTypeOf(n *ParsedNode) string {
	if n == nil {
		return ""
	}
	return n.Type
}

func signatureOf(n *ParsedNode) string {
	if n == nil {
		return ""
	}
	return n.Signature
}

func qualifiedOf(n *ParsedNode) string {
	if n == nil {
		return ""
	}
	if n.QualifiedName == nil {
		return ""
	}
	return *n.QualifiedName
}
