package analysis

import (
	"testing"
)

func TestDetectFileType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string
	}{
		{"empty", "", "text"},
		{"json", `{"key": "value"}`, "json"},
		{"json_array", `[1, 2, 3]`, "json"},
		{"html", "<!DOCTYPE html><html></html>", "html"},
		{"markdown", "# Hello\n- item\n", "markdown"},
		{"yaml", "key: value\n", "yaml"},
		{"xml", "<?xml version=\"1.0\"?><root></root>", "xml"},
		{"log", "2024-01-01T00:00:00 INFO hello\n", "log"},
		{"csv", "a,b,c\n1,2,3\n", "csv"},
		{"diff", "--- a/file\n+++ b/file\n@@ -1 +1 @@\n-old\n+new\n", "diff"},
		{"plain", "hello world", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectFileType(tt.input)
			if got.Type != tt.wantType {
				t.Errorf("DetectFileType(%q) type = %q, want %q", tt.name, got.Type, tt.wantType)
			}
		})
	}
}

func TestSuggestReadingTool(t *testing.T) {
	ft := FileTypeResult{Type: "json", Confidence: 0.9, Mime: "application/json"}
	advice := SuggestReadingTool(ft)
	if advice.Tool != "jq" {
		t.Errorf("SuggestReadingTool(json) tool = %q, want %q", advice.Tool, "jq")
	}
	if advice.Warning == nil {
		t.Errorf("SuggestReadingTool(json) warning = nil, want non-nil")
	}

	ft = FileTypeResult{Type: "markdown", Confidence: 0.8}
	advice = SuggestReadingTool(ft)
	if advice.Tool != "browser or file.read" {
		t.Errorf("SuggestReadingTool(markdown) tool = %q, want %q", advice.Tool, "browser or file.read")
	}
}
