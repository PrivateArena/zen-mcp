package codegraph

import (
	"os"
	"path/filepath"
	"testing"
)

// A three-script Godot project: an inner base class, a subclass reaching into an
// autoload singleton and a preloaded script, and the autoload script itself.
const godotProjectManifest = `; Engine configuration file.
config_version=5

[application]

config/name="Probe"
run/main_scene="res://scenes/main.tscn"

[autoload]

GameData="*res://scripts/game_data.gd"
OldSave="res://scripts/save.gd"

[editor_plugins]

enabled=PackedStringArray("res://addons/probe/plugin.cfg")
`

const godotBaseScript = `class_name RoVisual
extends Node3D

func place(pos: Vector3) -> void:
	position = pos
`

const godotSubclassScript = `class_name Pushcart
extends RoVisual

const DataScript = preload("res://scripts/game_data.gd")

var body: RoVisual

func follow(target: RoVisual) -> void:
	target.place(Vector3.ZERO)
	GameData.set_meta("pushed", 1)
	_track(GameData.SKILLS)

func _track(skills: Dictionary) -> void:
	pass
`

const godotAutoloadScript = `extends Node

const SKILLS: = {}

func set_meta(key: String, value: Variant) -> void:
	pass
`

func indexGodotFixture(t *testing.T) *CodeGraph {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fixtures := map[string]string{
		"project.godot":            godotProjectManifest,
		"scripts/ro_visual.gd":     godotBaseScript,
		"scripts/pushcart.gd":      godotSubclassScript,
		"scripts/game_data.gd":     godotAutoloadScript,
		"scripts/not_a_script.gdc": "GODOT_BYTECODE",
	}
	for name, content := range fixtures {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	cg, err := NewCodeGraph(dir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	t.Cleanup(func() { cg.Close() })
	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}
	return cg
}

type gdEdge struct{ source, target, targetPath, relation string }

func gdEdges(t *testing.T, cg *CodeGraph) []gdEdge {
	t.Helper()
	nodes, err := cg.storage.GetAllNodes()
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	byID := map[int64]NodeRecord{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	var out []gdEdge
	for _, e := range cg.storage.GetAllEdges("", 0) {
		src, okSrc := byID[e.SourceID]
		tgt, okTgt := byID[e.TargetID]
		if !okSrc || !okTgt {
			continue
		}
		out = append(out, gdEdge{source: src.Name, target: tgt.Name, targetPath: tgt.Path, relation: e.Relation})
	}
	return out
}

func (e gdEdge) String() string {
	return e.relation + " " + e.source + "->" + e.target + "@" + e.targetPath
}

func TestGdscriptCrossFileGraph(t *testing.T) {
	cg := indexGodotFixture(t)

	edges := gdEdges(t, cg)
	needed := []string{
		"extends Pushcart->RoVisual@scripts/ro_visual.gd",
		"calls follow->place@scripts/ro_visual.gd",
		"references follow->GameData@project.godot",
		"calls follow->set_meta@scripts/game_data.gd",
		"references follow->SKILLS@scripts/game_data.gd",
		"references follow->RoVisual@scripts/ro_visual.gd",
	}
	seen := map[string]bool{}
	for _, e := range edges {
		seen[e.String()] = true
	}
	for _, want := range needed {
		if !seen[want] {
			t.Errorf("missing edge %s\nhave: %v", want, edges)
		}
	}

	if has := seen["references _track->Vector3@scripts/ro_visual.gd"]; has {
		t.Error("an engine builtin type must not resolve to a project symbol")
	}
}

func TestGdscriptPreloadIsAnImport(t *testing.T) {
	cg := indexGodotFixture(t)

	imports := cg.storage.GetAllImports()
	found := false
	for _, ir := range imports {
		if ir.ImportPath == "res://scripts/game_data.gd" {
			found = true
		}
	}
	if !found {
		t.Fatalf("preload path not recorded, got %+v", imports)
	}
	if !cg.storage.IsFileImported("scripts/game_data.gd") {
		t.Error("the preloaded script should count as imported, not an orphan")
	}
}

func TestGdscriptIndexedSymbolsAndManifest(t *testing.T) {
	cg := indexGodotFixture(t)

	nodes, err := cg.storage.GetAllNodes()
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	want := map[string]string{
		"RoVisual":                "scripts/ro_visual.gd",
		"Pushcart":                "scripts/pushcart.gd",
		"follow":                  "scripts/pushcart.gd",
		"SKILLS":                  "scripts/game_data.gd",
		"GameData":                "project.godot",
		"OldSave":                 "project.godot",
		"scenes/main.tscn":        "project.godot",
		"addons/probe/plugin.cfg": "project.godot",
	}
	files, _ := cg.Files("", 0)
	pathByID := map[int64]string{}
	for _, f := range files {
		pathByID[f.ID] = f.Path
	}
	for name, path := range want {
		var hit *NodeRecord
		for i := range nodes {
			if nodes[i].Name == name && pathByID[nodes[i].FileID] == path {
				hit = &nodes[i]
				break
			}
		}
		if hit == nil {
			t.Errorf("no symbol %q indexed in %s", name, path)
			continue
		}
		if name == "OldSave" && hit.Signature != "autoload scripts/save.gd (disabled)" {
			t.Errorf("disabled autoload signature = %q", hit.Signature)
		}
		if name == "GameData" && hit.Signature != "autoload scripts/game_data.gd" {
			t.Errorf("autoload signature = %q", hit.Signature)
		}
	}

	for _, n := range nodes {
		if n.Path == "scripts/game_data.gd" && n.Type == "class" {
			t.Error("a script without class_name must not produce a class symbol")
		}
		if filepath.Base(n.Path) == "not_a_script.gdc" {
			t.Error("compiled .gdc bytecode must not be indexed")
		}
	}
}

func TestGdscriptCalledSymbolIsNotDeadCode(t *testing.T) {
	cg := indexGodotFixture(t)

	symbols, err := cg.Deadcode()
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}
	for _, s := range symbols {
		if s.Name == "place" || s.Name == "SKILLS" || s.Name == "GameData" {
			t.Errorf("%s %s is used across files but reported dead", s.Type, s.Name)
		}
	}
}
