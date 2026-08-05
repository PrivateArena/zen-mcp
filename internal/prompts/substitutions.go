package prompts

import (
	"fmt"
	"regexp"
	"strings"
)

// SubstituteTemplate replaces {{arg}} placeholders in the template with values from args.
func SubstituteTemplate(template string, args map[string]string, argDefs []PromptArgument) string {
	result := template
	for _, arg := range argDefs {
		value := args[arg.Name]
		if value == "" {
			value = ""
		}
		escapedName := regexp.QuoteMeta(arg.Name)
		re := regexp.MustCompile(fmt.Sprintf("{{%s}}", escapedName))
		result = re.ReplaceAllString(result, value)
	}
	return result
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
