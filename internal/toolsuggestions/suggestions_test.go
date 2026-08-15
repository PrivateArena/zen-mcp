package toolsuggestions

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

func schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":      map[string]any{"type": "string", "description": "The URL to open"},
			"selector": map[string]any{"type": "string", "description": "CSS selector"},
		},
		"required": []any{"url"},
	}
}

func GetAllToolNames() []string {
	names := make([]string, 0, len(suggestions))
	for name := range suggestions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func GetToolsByCategory() map[string][]ToolRef {
	categories := map[string][]string{
		"👁️ Vision & Capture":      {"capture", "colab"},
		"🌐 Browser & Automation":   {"browser"},
		"📁 Workspace & Storage":    {"workspace", "memory"},
		"🔧 Shell & Sandbox":        {"shell", "run"},
		"⚙️ Reasoning & Knowledge": {"think", "skills", "codegraph"},
		"🛠️ Server Management":     {"server"},
	}
	result := make(map[string][]ToolRef)
	for category, tools := range categories {
		var refs []ToolRef
		for _, name := range tools {
			if s, ok := suggestions[name]; ok {
				refs = append(refs, ToolRef{Name: name, Description: s.Description})
			}
		}
		result[category] = refs
	}
	return result
}

type ToolRef struct {
	Name        string
	Description string
}

func FormatToolReference() string {
	var b strings.Builder
	b.WriteString("# 🛠️ Tool Quick Reference\n\n")
	for _, category := range []string{"👁️ Vision & Capture", "🌐 Browser & Automation", "📁 Workspace & Storage", "🔧 Shell & Sandbox", "⚙️ Reasoning & Knowledge", "🛠️ Server Management"} {
		b.WriteString("## " + category + "\n")
		for _, tool := range GetToolsByCategory()[category] {
			b.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name, tool.Description))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func FindMistakeCorrection(toolName, errorMessage string, action string, schema any) *MistakeCorrection {
	suggestion := FormatSuggestion(toolName, errorMessage, action, schema)
	if suggestion == "" {
		return nil
	}
	correctUsage := fmt.Sprintf("%s({ ... })", toolName)
	if s := GetToolSuggestion(toolName); s != nil && s.ExampleUsage != "" {
		correctUsage = s.ExampleUsage
	}
	return &MistakeCorrection{Message: suggestion, CorrectUsage: correctUsage}
}

type MistakeCorrection struct {
	Message      string
	CorrectUsage string
}

func TestGetToolSuggestionKnown(t *testing.T) {
	if GetToolSuggestion("browser") == nil {
		t.Error("browser suggestion missing")
	}
	if GetToolSuggestion("nope") != nil {
		t.Error("unknown tool should return nil")
	}
}

func TestGetAllToolNames(t *testing.T) {
	names := GetAllToolNames()
	if len(names) != 12 {
		t.Errorf("expected 12 tools, got %d", len(names))
	}
	expected := map[string]bool{"workspace": true, "capture": true, "think": true, "memory": true, "context": true, "run": true, "shell": true, "skills": true, "codegraph": true, "browser": true, "server": true, "colab": true}
	for _, n := range names {
		if !expected[n] {
			t.Errorf("unexpected tool name %q", n)
		}
	}
}

func TestValidateToolCallMissingRequired(t *testing.T) {
	res := ValidateToolCall("browser", "navigate", map[string]any{}, schema())
	if res.Valid {
		t.Error("navigate without url should be invalid")
	}
	if len(res.MissingRequired) != 1 || res.MissingRequired[0] != "url" {
		t.Errorf("MissingRequired = %v", res.MissingRequired)
	}
	if !strings.Contains(res.Suggestion, "url") {
		t.Errorf("suggestion should mention url: %s", res.Suggestion)
	}
}

func TestValidateToolCallValid(t *testing.T) {
	res := ValidateToolCall("browser", "navigate", map[string]any{"url": "https://example.com"}, schema())
	if !res.Valid {
		t.Errorf("valid call rejected: %s", res.Suggestion)
	}
	if len(res.MissingRequired) != 0 {
		t.Errorf("unexpected missing: %v", res.MissingRequired)
	}
}

func TestValidateToolCallNoRule(t *testing.T) {
	res := ValidateToolCall("browser", "active_tab", map[string]any{}, schema())
	if !res.Valid {
		t.Error("action without rule should be valid")
	}
}

func TestValidateToolCallThinkRules(t *testing.T) {
	res := ValidateToolCall("think", "create_plan", map[string]any{}, nil)
	if res.Valid {
		t.Error("create_plan without tasks should be invalid")
	}
	if len(res.MissingRequired) != 2 {
		t.Errorf("MissingRequired = %v", res.MissingRequired)
	}
	res = ValidateToolCall("think", "create_plan", map[string]any{"project_name": "P", "tasks": []any{"a"}}, nil)
	if !res.Valid {
		t.Errorf("complete create_plan rejected: %s", res.Suggestion)
	}
}

func TestFormatSuggestionActionMatch(t *testing.T) {
	s := FormatSuggestion("browser", "something failed", "click", schema())
	if !strings.Contains(s, "selector") {
		t.Errorf("action match should surface selector help: %s", s)
	}
	if !strings.Contains(s, "📌 **Tool: browser**") {
		t.Errorf("header missing: %s", s)
	}
	if !strings.Contains(s, "Example Usage") {
		t.Errorf("example usage missing: %s", s)
	}
}

func TestFormatSuggestionMessageMatch(t *testing.T) {
	s := FormatSuggestion("browser", "url is required", "", schema())
	if !strings.Contains(s, "navigate") {
		t.Errorf("message regex should match navigate help: %s", s)
	}
}

func TestFormatSuggestionSchemaIntrospection(t *testing.T) {
	s := FormatSuggestion("browser", "generic error", "navigate", schema())
	if !strings.Contains(s, "Parameters for action") || !strings.Contains(s, "- **url**") {
		t.Errorf("schema introspection missing: %s", s)
	}
}

func TestFindMistakeCorrection(t *testing.T) {
	mc := FindMistakeCorrection("browser", "url is required", "navigate", schema())
	if mc == nil {
		t.Fatal("expected correction")
	}
	if !strings.Contains(mc.Message, "navigate") {
		t.Errorf("message should mention navigate: %s", mc.Message)
	}
	if mc.CorrectUsage != "browser({ action: \"active_tab\" })" {
		t.Errorf("CorrectUsage = %q", mc.CorrectUsage)
	}
}

func TestGetToolsByCategory(t *testing.T) {
	cats := GetToolsByCategory()
	browser := cats["🌐 Browser & Automation"]
	if len(browser) != 1 || browser[0].Name != "browser" {
		t.Errorf("browser category = %+v", browser)
	}
	total := 0
	for _, refs := range cats {
		total += len(refs)
	}
	if total != 11 {
		t.Errorf("expected 11 tool refs (context omitted, matches TS), got %d", total)
	}
}

func TestFormatToolReference(t *testing.T) {
	ref := FormatToolReference()
	if !strings.Contains(ref, "# 🛠️ Tool Quick Reference") {
		t.Errorf("reference header missing: %s", ref)
	}
	if !strings.Contains(ref, "**browser**:") {
		t.Errorf("reference missing browser entry: %s", ref)
	}
}

func TestSemanticPlaceholder(t *testing.T) {
	if p := SemanticPlaceholder("url", "string"); p != "https://example.com" {
		t.Errorf("url placeholder = %v", p)
	}
	if p := SemanticPlaceholder("selector", "string"); p != "button" {
		t.Errorf("selector placeholder = %v", p)
	}
	if p := SemanticPlaceholder("enabled", "boolean"); p != false {
		t.Errorf("boolean placeholder = %v", p)
	}
	if p := SemanticPlaceholder("count", "number"); p != 1 {
		t.Errorf("number placeholder = %v", p)
	}
	if p := SemanticPlaceholder("tags", "array"); p == nil {
		t.Errorf("array placeholder nil")
	}
	if p := SemanticPlaceholder("misc", "string"); p != "<string>" {
		t.Errorf("fallback placeholder = %v", p)
	}
}

func TestIntrospectSchema(t *testing.T) {
	s := introspectSchema(schema(), "navigate")
	if !strings.Contains(s, "Parameters for action") {
		t.Errorf("introspection header missing: %s", s)
	}
	if !strings.Contains(s, "- **url**: string (required)") {
		t.Errorf("required url line missing: %s", s)
	}
	if !strings.Contains(s, "- **selector**: string (optional)") {
		t.Errorf("optional selector line missing: %s", s)
	}
	if introspectSchema("not a map", "x") != "" {
		t.Error("non-map schema should yield empty")
	}
}

func TestMustJSON(t *testing.T) {
	out := MustJSON(map[string]any{"a": 1})
	if !strings.Contains(out, "\"a\"") {
		t.Errorf("MustJSON output: %s", out)
	}
}
