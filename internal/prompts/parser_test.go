package prompts

import "testing"

func TestParserReplacesKnownPlaceholder(t *testing.T) {
	out := substitutePlaceholders("Task: {{i}}", map[string]string{"i": "hello"}, map[string]bool{"i": true})
	if out != "Task: hello" {
		t.Errorf("got %q, want %q", out, "Task: hello")
	}
}

func TestParserReplacesMultiplePlaceholders(t *testing.T) {
	out := substitutePlaceholders(
		"a {{x}} b {{y}} c {{x}}",
		map[string]string{"x": "1", "y": "2"},
		map[string]bool{"x": true, "y": true},
	)
	if out != "a 1 b 2 c 1" {
		t.Errorf("got %q", out)
	}
}

func TestParserKeepsRegexLikeTextAsPlainText(t *testing.T) {
	in := `c=$(echo "{{i}}" | tr ',' ' '); set -- $c; f=$1; l=$NF; case "$f" in *..* ) x=${f%%.*}; y=${f##*.}; b="$x~1"; t="$y";; esac; git diff "$b" "$t"`
	out := substitutePlaceholders(in, map[string]string{"i": "abc123"}, map[string]bool{"i": true})
	if out != `c=$(echo "abc123" | tr ',' ' '); set -- $c; f=$1; l=$NF; case "$f" in *..* ) x=${f%%.*}; y=${f##*.}; b="$x~1"; t="$y";; esac; git diff "$b" "$t"` {
		t.Errorf("regex-like text must pass through untouched, got:\n%s", out)
	}
}

func TestParserKeepsUnknownPlaceholder(t *testing.T) {
	out := substitutePlaceholders("You are Zen, {{PERSONA}}. Task: {{i}}", map[string]string{"i": "go"}, map[string]bool{"i": true})
	if out != "You are Zen, {{PERSONA}}. Task: go" {
		t.Errorf("unknown placeholder must be preserved verbatim, got %q", out)
	}
}

func TestParserKnownArgEmptyValueBecomesEmpty(t *testing.T) {
	out := substitutePlaceholders("Context: {{m}}", map[string]string{}, map[string]bool{"m": true})
	if out != "Context: " {
		t.Errorf("known arg with empty value must render empty, got %q", out)
	}
}

func TestParserKeepsMalformedPlaceholderText(t *testing.T) {
	in := `placeholders (e.g. {{"{{var}}"}}, %s), markup tags`
	out := substitutePlaceholders(in, map[string]string{}, nil)
	if out != in {
		t.Errorf("malformed placeholder example must be preserved, got %q", out)
	}
}

func TestParserKeepsUnclosedPlaceholder(t *testing.T) {
	in := "open {{never closed"
	out := substitutePlaceholders(in, map[string]string{"never": "x"}, map[string]bool{"never": true})
	if out != in {
		t.Errorf("unclosed placeholder must be preserved, got %q", out)
	}
}

func TestParserPlaceholderNameWithDashAndUnderscore(t *testing.T) {
	out := substitutePlaceholders(
		"path={{patch-path}} inst={{instruments_path}}",
		map[string]string{"patch-path": "a/b", "instruments_path": "/samples"},
		map[string]bool{"patch-path": true, "instruments_path": true},
	)
	if out != "path=a/b inst=/samples" {
		t.Errorf("dash/underscore names not substituted, got %q", out)
	}
}

func TestParserDollarOnlyTextUntouched(t *testing.T) {
	in := `awk '{if ($0 == "" ) next; print $1 $2}'`
	out := substitutePlaceholders(in, map[string]string{}, nil)
	if out != in {
		t.Errorf("awk $1 $2 must be untouched, got %q", out)
	}
}

func TestSubstituteTemplateWrapsParser(t *testing.T) {
	argDefs := []PromptArgument{{Name: "i", Required: true}, {Name: "m"}}
	out := SubstituteTemplate(
		"Review: {{i}}; Context: {{m}}; awk $1 $NF",
		map[string]string{"i": "abc", "m": "note"},
		argDefs,
	)
	if out != "Review: abc; Context: note; awk $1 $NF" {
		t.Errorf("SubstituteTemplate misrendered, got %q", out)
	}
}
