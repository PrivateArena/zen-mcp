package prompts

import "strings"

const (
	promptPlaceholderOpen  = "{{"
	promptPlaceholderClose = "}}"
)

// isPlaceholderName reports whether s is a valid prompt placeholder name:
// one or more characters from [A-Za-z0-9_-].
func isPlaceholderName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_':
		default:
			return false
		}
	}
	return true
}

// substitutePlaceholders is the plain-text prompt parser. It scans template
// literally for {{name}} placeholders and replaces each one whose name is in the
// known set with the corresponding value from args ("" when the arg is absent).
//
// Only prompt syntax is understood. Everything else is opaque plain text:
// regex-like sequences such as $1, $2 or $NF are never interpreted and pass
// through the parser untouched. Placeholders whose name is not in the known set
// (e.g. {{PERSONA}}) are preserved verbatim so later stages can resolve them.
func substitutePlaceholders(template string, args map[string]string, known map[string]bool) string {
	var b strings.Builder
	b.Grow(len(template))
	rest := template
	for {
		start := strings.Index(rest, promptPlaceholderOpen)
		if start < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:start])
		after := rest[start+len(promptPlaceholderOpen):]
		end := strings.Index(after, promptPlaceholderClose)
		if end < 0 {
			b.WriteString(rest[start:])
			break
		}
		name := strings.TrimSpace(after[:end])
		if isPlaceholderName(name) && known[name] {
			b.WriteString(args[name])
		} else {
			b.WriteString(rest[start : start+len(promptPlaceholderOpen)+end+len(promptPlaceholderClose)])
		}
		rest = after[end+len(promptPlaceholderClose):]
	}
	return b.String()
}
