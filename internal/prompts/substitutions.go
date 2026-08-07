package prompts

import (
	"fmt"
	"strings"
)

// SubstituteTemplate replaces {{arg}} placeholders in the template with values
// from args. Parsing is done by the plain-text parser in parser.go: only prompt
// placeholder syntax ({{name}}) is understood; regex-like sequences such as $1
// are kept as plain text and never interpreted.
func SubstituteTemplate(template string, args map[string]string, argDefs []PromptArgument) string {
	known := make(map[string]bool, len(argDefs))
	for _, arg := range argDefs {
		known[arg.Name] = true
	}
	return substitutePlaceholders(template, args, known)
}

// WarnMissingArgs logs a warning if required args are missing.
func WarnMissingArgs(promptName string, args map[string]string, argDefs []PromptArgument) {
	var missing []string
	for _, arg := range argDefs {
		if arg.Required && args[arg.Name] == "" {
			missing = append(missing, arg.Name)
		}
	}
	if len(missing) > 0 {
		fmt.Printf("[MCP] Prompt '%s' called with missing required args: %s\n", promptName, strings.Join(missing, ", "))
	}
}
