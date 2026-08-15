package toolsuggestions

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type ActionSuggestionRule struct {
	Trigger        string
	IsRegex        bool
	Help           string
	RequiredParams []string
	re             *regexp.Regexp
}

type ToolSuggestion struct {
	Name         string
	Description  string
	ExampleUsage string
	ActionRules  []ActionSuggestionRule
}

// regexRule is a helper function
func regexRule(pattern string) ActionSuggestionRule {
	return ActionSuggestionRule{Trigger: pattern, IsRegex: true, re: regexp.MustCompile(pattern)}
}

var suggestions = map[string]ToolSuggestion{
	"workspace": {
		Name: "workspace", Description: "Manage mounted projects, backups, and workspace roots.",
		ExampleUsage: `workspace({ path: "/abs/path/to/project" })`,
		ActionRules: []ActionSuggestionRule{
			{Trigger: "path", Help: "requires absolute \"path\".\n✅ Correct: workspace({ path: \"/abs/path/to/project\" })", RequiredParams: []string{"path"}},
		},
	},
	"capture": {
		Name: "capture", Description: "Capture screenshot of the desktop, window, or region.",
		ExampleUsage: `capture({ action: "screenshot", mode: "window" })`,
	},
	"think": {
		Name: "think", Description: "Sequential reasoning, task planning, and project state management.",
		ExampleUsage: `think({ action: "sequential_thinking", thought: "My reasoning step", thoughtNumber: 1, totalThoughts: 5 })`,
		ActionRules: []ActionSuggestionRule{
			{Trigger: "create_plan", Help: "action: \"create_plan\" requires \"project_name\" and \"tasks\" (array of strings).\n✅ Correct: think({ action: \"create_plan\", project_name: \"Refactor\", tasks: [\"Task 1\"] })", RequiredParams: []string{"project_name", "tasks"}},
			{Trigger: "sequential_thinking", Help: "action: \"sequential_thinking\" requires \"thought\" and \"thoughtNumber\".\n✅ Correct: think({ action: \"sequential_thinking\", thought: \"My thought\", thoughtNumber: 1, totalThoughts: 5 })", RequiredParams: []string{"thought", "thoughtNumber"}},
			{Trigger: "update_task", Help: "action: \"update_task\" requires \"id\" and \"status\".\n✅ Correct: think({ action: \"update_task\", id: 1, status: \"done\", notes: \"finished\" })", RequiredParams: []string{"id", "status"}},
			{Trigger: "add_task", Help: "action: \"add_task\" requires \"title\".\n✅ Correct: think({ action: \"add_task\", title: \"New Task\" })", RequiredParams: []string{"title"}},
		},
	},
	"memory": {
		Name: "memory", Description: "Persistent cross-session state management.",
		ExampleUsage: `memory({ action: "load" })`,
		ActionRules: []ActionSuggestionRule{
			{Trigger: "scope", Help: "action: \"scope\" requires a \"scope\" name. Options: \"paths\" (array), \"dependencies\" (array).\n✅ Correct: memory({ action: \"scope\", scope: \"note\", paths: [\"src\"] })", RequiredParams: []string{"scope"}},
			{Trigger: "search", Help: "action: \"search\" requires \"query\".\n✅ Correct: memory({ action: \"search\", query: \"search terms\" })", RequiredParams: []string{"query"}},
			{Trigger: "save", Help: "action: \"save\" persists session state.\n✅ Correct: memory({ action: \"save\", session_title: \"My Session\", objective: \"Session goal\" })"},
		},
	},
	"context": {
		Name: "context", Description: "Retrieve stored project memory context with optional file analysis.",
		ExampleUsage: `context({ query: "virt_..." })`,
		ActionRules: []ActionSuggestionRule{
			{Trigger: "retrieve_context", Help: "action: \"retrieve_context\" requires \"query\" (Virtual Retrieval ID, e.g., \"virt_...\").\n✅ Correct: context({ query: \"virt_...\" })", RequiredParams: []string{"query"}},
		},
	},
	"run": {
		Name: "run", Description: "Sandbox REPL for Node.js, Python, TypeScript, and Bash.",
		ExampleUsage: `run({ language: "python", code: "print(123 * 456)" })`,
	},
	"shell": {
		Name: "shell", Description: "Execute shell commands with native token-optimization.",
		ExampleUsage: `shell({ command: "npm run build" })`,
	},
	"skills": {
		Name: "skills", Description: "Retrieve standardized coding patterns and best practices.",
		ExampleUsage: `skills({ action: "list" })`,
		ActionRules: []ActionSuggestionRule{
			{Trigger: "get", Help: "action: \"get\" requires a skill \"id\".\n✅ Correct: skills({ action: \"get\", id: \"typescript-mcp-server\" })", RequiredParams: []string{"id"}},
		},
	},
	"codegraph": {
		Name: "codegraph", Description: "High-performance code indexing, symbol search, and relationship mapping.",
		ExampleUsage: `codegraph({ action: "index" })`,
		ActionRules: []ActionSuggestionRule{
			{Trigger: "search", Help: "action: \"search\" requires \"query\".\n✅ Correct: codegraph({ action: \"search\", query: \"symbolName\" })", RequiredParams: []string{"query"}},
			{Trigger: "skeletons", Help: "action: \"skeletons\" requires \"query\" set to relative path(s) (comma-separated).\n✅ Correct: codegraph({ action: \"skeletons\", query: \"src/index.ts\" })", RequiredParams: []string{"query"}},
			{Trigger: "neighbors", Help: "action: \"neighbors\" requires \"query\" set to a symbol name.\n✅ Correct: codegraph({ action: \"neighbors\", query: \"myFunction\" })", RequiredParams: []string{"query"}},
			{Trigger: "usage", Help: "action: \"usage\" requires \"query\" set to a symbol name.\n✅ Correct: codegraph({ action: \"usage\", query: \"myFunction\" })", RequiredParams: []string{"query"}},
			{Trigger: "related", Help: "action: \"related\" requires \"query\" set to a relative path(s) (comma-separated).\n✅ Correct: codegraph({ action: \"related\", query: \"src/index.ts\" })", RequiredParams: []string{"query"}},
			{Trigger: "explain", Help: "action: \"explain\" requires \"query\" set to a symbol name.\n✅ Correct: codegraph({ action: \"explain\", query: \"myFunction\" })", RequiredParams: []string{"query"}},
			{Trigger: "refactor_copy", Help: "action: \"refactor_copy\" requires \"target_path\" and \"sources\" (array of {path, start_line, end_line}).\n✅ Correct: codegraph({ action: \"refactor_copy\", target_path: \"dest.ts\", sources: [{ path: \"src.ts\", start_line: 1, end_line: 10 }] })", RequiredParams: []string{"target_path", "sources"}},
			{Trigger: "refactor_delete", Help: "action: \"refactor_delete\" requires \"targets\" (array of {path, blocks: [{start_line, end_line}]}).\n✅ Correct: codegraph({ action: \"refactor_delete\", targets: [{ path: \"file.ts\", blocks: [{ start_line: 5, end_line: 8 }] }] })", RequiredParams: []string{"targets"}},
		},
	},
	"browser": {
		Name: "browser", Description: "Firefox automation and AI chat bridge.",
		ExampleUsage: `browser({ action: "active_tab" })`,
		ActionRules: []ActionSuggestionRule{
			regexRule(`(?i)\bnavigate\b|url is required`).withHelp("action: \"navigate\" requires \"url\".\n✅ Correct: browser({ action: \"navigate\", url: \"https://google.com\" })").withRequired("url"),
			regexRule(`(?i)\bclick\b|\bhover\b|\bupload\b|selector is required`).withHelp("Interaction actions require a \"selector\" (or target like \"by_text\", \"by_label\", \"by_name\", \"by_role\").\n✅ Correct: browser({ action: \"click\", selector: \"button\" })"),
			regexRule(`(?i)\bchat\b|message.*required|provider.*required`).withHelp("action: \"chat\" requires \"message\" and \"provider\". Optional: \"path\" (file upload), \"screenshot\", \"strict\".\n✅ Correct: browser({ action: \"chat\", message: \"Hello\", provider: \"gemini\" })").withRequired("message", "provider"),
			regexRule(`(?i)\bbrainstorm\b|prompt is required`).withHelp("action: \"brainstorm\" requires \"prompt\".\n✅ Correct: browser({ action: \"brainstorm\", prompt: \"Write code for...\" })").withRequired("prompt"),
			regexRule(`(?i)\bweb_eval\b|\bchrome_eval\b`).withHelp("Evaluation actions require a JS string \"code\" returning a value.\n✅ Correct: browser({ action: \"web_eval\", code: \"return document.title;\" })").withRequired("code"),
		},
	},
	"server": {
		Name: "server", Description: "MCP server management, proxies, and wiki/whiteboard ingestion.",
		ExampleUsage: `server({ action: "status" })`,
		ActionRules: []ActionSuggestionRule{
			{Trigger: "wiki", Help: "action: \"wiki\" requires \"url\".\n✅ Correct: server({ action: \"wiki\", url: \"https://...\" })", RequiredParams: []string{"url"}},
			{Trigger: "wb", Help: "action: \"wb\" requires \"url\" and \"whiteboard_slug\".\n✅ Correct: server({ action: \"wb\", url: \"...\", whiteboard_slug: \"my-board\" })", RequiredParams: []string{"url", "whiteboard_slug"}},
			{Trigger: "zen", Help: "action: \"zen\" requires \"test_type\" (e.g., \"production\" and \"url\").\n✅ Correct: server({ action: \"zen\", test_type: \"production\", url: \"https://...\" })"},
		},
	},
	"colab": {
		Name: "colab", Description: "Collaborative UI capture and visual collaboration bridge.",
		ExampleUsage: `colab({ action: "collaborate" })`,
	},
}

// withHelp is a helper function
func (r ActionSuggestionRule) withHelp(help string) ActionSuggestionRule {
	r.Help = help
	return r
}

// withRequired is a helper function
func (r ActionSuggestionRule) withRequired(params ...string) ActionSuggestionRule {
	r.RequiredParams = params
	return r
}

// GetToolSuggestion is a helper function
func GetToolSuggestion(toolName string) *ToolSuggestion {
	s, ok := suggestions[toolName]
	if !ok {
		return nil
	}
	return &s
}

// actionMatch is a helper function
func actionMatch(rule ActionSuggestionRule, action string) bool {
	if rule.IsRegex {
		return rule.re.MatchString(action)
	}
	return rule.Trigger == action
}

// msgMatch is a helper function
func msgMatch(rule ActionSuggestionRule, msg string) bool {
	if rule.IsRegex {
		return rule.re.MatchString(msg)
	}
	return strings.Contains(msg, rule.Trigger)
}

type ValidationResult struct {
	Valid           bool
	MissingRequired []string
	Suggestion      string
}

// ValidateToolCall is a helper function
func ValidateToolCall(toolName, action string, suppliedParams map[string]any, schema any) ValidationResult {
	base := GetToolSuggestion(toolName)
	var missingRequired []string

	if base != nil {
		for _, rule := range base.ActionRules {
			if !actionMatch(rule, action) {
				continue
			}
			for _, param := range rule.RequiredParams {
				if v, ok := suppliedParams[param]; !ok || v == nil {
					missingRequired = append(missingRequired, param)
				}
			}
			break
		}
	}

	if len(missingRequired) > 0 {
		quoted := make([]string, 0, len(missingRequired))
		for _, p := range missingRequired {
			quoted = append(quoted, fmt.Sprintf("%q", p))
		}
		suggestion := fmt.Sprintf("Missing required parameter(s) for %q action %q: %s.", toolName, action, strings.Join(quoted, ", "))
		if base != nil {
			suggestion += "\n" + base.Description
		}
		return ValidationResult{Valid: false, MissingRequired: missingRequired, Suggestion: suggestion}
	}

	return ValidationResult{Valid: true, MissingRequired: []string{}, Suggestion: ""}
}

// FormatSuggestion is a helper function
func FormatSuggestion(toolName string, errorMessage string, action string, schema any) string {
	base := GetToolSuggestion(toolName)
	msg := strings.ToLower(errorMessage)

	specificHelp := ""
	if action != "" && base != nil {
		for _, rule := range base.ActionRules {
			if actionMatch(rule, action) {
				specificHelp = rule.Help
				break
			}
		}
	}
	if specificHelp == "" && base != nil {
		for _, rule := range base.ActionRules {
			if msgMatch(rule, msg) {
				specificHelp = rule.Help
				break
			}
		}
	}

	schemaInfo := introspectSchema(schema, action)

	desc := "No description available."
	if base != nil {
		desc = base.Description
	}
	example := ""
	if base != nil && base.ExampleUsage != "" {
		example = fmt.Sprintf("\n\n📝 Example Usage:\n```typescript\n%s\n```", base.ExampleUsage)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n\n📌 **Tool: %s**\n%s", toolName, desc)
	if specificHelp != "" {
		fmt.Fprintf(&b, "\n\n⚠️ **Action Issue:**\n%s", specificHelp)
	}
	b.WriteString(example)
	b.WriteString(schemaInfo)
	return b.String()
}

// introspectSchema is a helper function
func introspectSchema(schema any, action string) string {
	m, ok := schema.(map[string]any)
	if !ok {
		return ""
	}
	props, _ := m["properties"].(map[string]any)
	if len(props) == 0 {
		return ""
	}
	requiredSet := map[string]bool{}
	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	if action != "" {
		fmt.Fprintf(&b, "\n\n📋 **Parameters for action %q:**\n", action)
	} else {
		b.WriteString("\n\n📋 **Parameters:**\n")
	}
	for _, key := range keys {
		p, _ := props[key].(map[string]any)
		typeStr := "any"
		if t, ok := p["type"].(string); ok && t != "" {
			typeStr = t
		}
		req := "optional"
		if requiredSet[key] {
			req = "required"
		}
		desc, _ := p["description"].(string)
		line := fmt.Sprintf("- **%s**: %s (%s)", key, typeStr, req)
		if desc != "" {
			line += " - " + desc
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// SemanticPlaceholder is a helper function
func SemanticPlaceholder(key string, typeName string) any {
	lk := strings.ToLower(key)
	switch typeName {
	case "boolean":
		return false
	case "number":
		return 1
	case "array":
		return []any{}
	case "object":
		return map[string]any{}
	}
	switch {
	case strings.Contains(lk, "url"):
		return "https://example.com"
	case strings.Contains(lk, "path"):
		return "/abs/path/to/file"
	case strings.Contains(lk, "alias"):
		return "my-alias"
	case strings.Contains(lk, "query"):
		return "search term"
	case strings.Contains(lk, "command"):
		return "npm run build"
	case strings.Contains(lk, "code"):
		return "return document.title;"
	case strings.Contains(lk, "slug"):
		return "my-board"
	case strings.Contains(lk, "name"):
		return "my-project"
	case strings.Contains(lk, "title"):
		return "Task Title"
	case strings.Contains(lk, "thought"):
		return "I need to consider..."
	case strings.Contains(lk, "message"):
		return "Hello, how can you help?"
	case strings.Contains(lk, "provider"):
		return "gemini"
	case strings.Contains(lk, "selector"):
		return "button"
	case strings.Contains(lk, "prompt"):
		return "Explain this code"
	case strings.Contains(lk, "container"):
		return "Personal"
	}
	return "<string>"
}

// MustJSON is a helper function
func MustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
