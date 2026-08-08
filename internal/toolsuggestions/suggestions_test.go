package toolsuggestions

import (
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

func TestGetToolSuggestionKnown(t *testing.T) {
	if GetToolSuggestion("browser") == nil {
		t.Error("browser suggestion missing")
	}
	if GetToolSuggestion("nope") != nil {
		t.Error("unknown tool should return nil")
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

