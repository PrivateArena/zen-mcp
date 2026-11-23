package codegraph

import (
	"strings"
	"testing"
)

const gdscriptSample = `class_name LootRoll
extends RoVisual

signal rolled(item: int, rarity: String)
signal cancelled

const TICK: = 0.025
const Bot = preload("res://scripts/net/channel_bot.gd")
var world: World
static var _tex: Texture2D

func _init() -> void:
	super._init("LootRoll")

# Rolls one item and hands it to the caller.
@rpc("any_peer", "reliable")
static func _clean(looks: Variant) -> Array:
	var out: Array = []
	GameData.set_meta("rolls", 1)
	out.append(GameData.ITEMS[0])
	world.tick(1)
	rolled.emit(3, "legendary")
	cancelled.connect(_on_cancelled)
	add_child(Bot.new())
	for d in _views():
		await get_tree().create_timer(0.1).timeout
	return out

func _on_cancelled() -> void:
	pass

class Preview extends SubViewport:
	func frame() -> Control:
		return null
`

func parseGdscript(t *testing.T) ([]ParsedNode, []ParsedRelation) {
	t.Helper()
	p := newGdscriptPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	nodes, relations, err := p.Parse([]byte(gdscriptSample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return nodes, relations
}

func TestGdscriptPluginLanguage(t *testing.T) {
	p := newGdscriptPlugin()
	if got := p.LanguageName(); got != "gdscript" {
		t.Errorf("LanguageName = %q, want gdscript", got)
	}
	exts := p.Extensions()
	if len(exts) != 1 || exts[0] != ".gd" {
		t.Errorf("Extensions = %v, want [.gd]", exts)
	}
}

func TestGdscriptPluginNodes(t *testing.T) {
	nodes, _ := parseGdscript(t)

	types := map[string]bool{}
	for _, n := range nodes {
		types[n.Type+":"+n.Name] = true
	}
	for _, want := range []string{
		"class:LootRoll", "class:Preview",
		"function:_init", "function:_clean", "function:_on_cancelled", "method:frame",
		"signal:rolled", "signal:cancelled",
		"constant:TICK", "constant:Bot", "variable:world", "variable:_tex",
	} {
		if !types[want] {
			t.Errorf("missing symbol %s, have %s", want, strings.Join(gdSymbolKeys(nodes), " "))
		}
	}
	for _, unwanted := range []string{"variable:out", "variable:d", "variable:looks"} {
		if types[unwanted] {
			t.Errorf("function-local binding leaked into symbols: %s", unwanted)
		}
	}
}

func TestGdscriptPluginSignaturesAndDocs(t *testing.T) {
	nodes, _ := parseGdscript(t)
	for _, n := range nodes {
		switch n.Name {
		case "_clean":
			want := `@rpc("any_peer", "reliable") static func _clean(looks: Variant) -> Array`
			if strings.Join(strings.Fields(n.Signature), " ") != want {
				t.Errorf("_clean signature = %q, want %q", n.Signature, want)
			}
			if n.Docstring != "Rolls one item and hands it to the caller." {
				t.Errorf("_clean docstring = %q", n.Docstring)
			}
		case "Preview":
			if n.QualifiedName == nil || *n.QualifiedName != "Preview" {
				t.Errorf("Preview qualified name = %v, want Preview", n.QualifiedName)
			}
		case "frame":
			if n.QualifiedName == nil || *n.QualifiedName != "Preview.frame" {
				t.Errorf("frame qualified name = %v, want Preview.frame", n.QualifiedName)
			}
			if n.Type != "method" {
				t.Errorf("frame type = %q, want method", n.Type)
			}
		}
	}
}

func TestGdscriptPluginRelations(t *testing.T) {
	_, relations := parseGdscript(t)

	have := map[string]bool{}
	for _, r := range relations {
		have[r.Relation+" "+r.SourceName+"->"+r.TargetName] = true
	}
	for _, want := range []string{
		"extends LootRoll->RoVisual",
		"calls _init->_init",
		"calls _clean->set_meta",
		"references _clean->GameData",
		"references _clean->ITEMS",
		"calls _clean->tick",
		"references _clean->world",
		"references _clean->rolled",
		"calls _clean->connect",
		"references _clean->cancelled",
		"references _clean->_on_cancelled",
		"calls _clean->add_child",
		"references _clean->Bot",
		"calls _clean->append",
		"calls _clean->create_timer",
		"references _clean->Array",
		"references frame->Control",
	} {
		if !have[want] {
			t.Errorf("missing relation %q", want)
		}
	}

	imports := map[string]bool{}
	for _, r := range relations {
		if r.Relation == "imports" {
			imports[r.TargetName] = true
		}
	}
	if !imports["res://scripts/net/channel_bot.gd"] {
		t.Errorf("preload path not recorded as an import, got %v", imports)
	}
}

func TestGdscriptPluginNoSelfEdges(t *testing.T) {
	_, relations := parseGdscript(t)
	for _, r := range relations {
		if r.SourceName == r.TargetName && r.Relation != "calls" {
			t.Errorf("self-relation recorded: %+v", r)
		}
	}
}

func TestGdscriptPluginErrorFreeParse(t *testing.T) {
	p := newGdscriptPlugin()
	if err := p.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	nodes, relations, err := p.Parse([]byte(gdscriptSample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(nodes) == 0 || len(relations) == 0 {
		t.Fatalf("nothing extracted: %d nodes, %d relations", len(nodes), len(relations))
	}
}

func gdSymbolKeys(nodes []ParsedNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Type+":"+n.Name)
	}
	return out
}
