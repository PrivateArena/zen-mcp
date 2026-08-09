package prompts

import (
	"regexp"
	"sort"
	"strings"
)

// CLIToolMap maps MCP tool names to their CLI wrapper script names.
var CLIToolMap = map[string]string{
	"browser":        "zen-browser",
	"capture":        "zen-capture",
	"codegraph":      "zen-codegraph",
	"colab":          "zen-colab",
	"context":        "zen-context",
	"memory":         "zen-memory",
	"memory_isolate": "zen-memory_isolate",
	"memory_shared":  "zen-memory_shared",
	"run":            "zen-run",
	"shell":          "zen-shell",
	"skill":          "zen-skill",
	"think":          "zen-think",
	"ui-vision":      "zen-ui-vision",
	"workspace":      "zen-workspace",
}

// CLITool returns the CLI wrapper name for an MCP tool name.
// Unknown tools fall back to "zen-" + name.
func CLITool(mcpName string) string {
	if cli, ok := CLIToolMap[mcpName]; ok {
		return cli
	}
	return "zen-" + mcpName
}

// TransformRule pairs a compiled regexp with its replacement.
type TransformRule struct {
	re          *regexp.Regexp
	replacement string
	useFunc     bool
	replFunc    func([]string) string
}

var transformRules []TransformRule

func init() {
	transformRules = []TransformRule{
		// Rule 7 — MCP skill activation (injected block) — most specific
		{re: regexp.MustCompile("Activate MCP skill id=([a-zA-Z0-9_-]+)"), replacement: "Activate skill id=$1 via `zen-skill --action get --id=$1`"},
		// Rule 8 — MCP Tool skill reference
		{re: regexp.MustCompile("Please use MCP Tool skill id="), replacement: "Please use `zen-skill --action get --id="},
		// Rule 9 — Inline skill id backticks
		{re: regexp.MustCompile("`skill id=([a-zA-Z0-9_-]+)`"), replacement: "`zen-skill --action get --id=$1`"},
		// Rule 6 — MCP shell reference (specific prefix)
		{re: regexp.MustCompile("Always use the MCP `([^`]+)`"), replacement: "Always use the `zen-$1` CLI"},
		// Rule 4 — MCP tool backtick reference
		{re: regexp.MustCompile("MCP `([a-zA-Z_][a-zA-Z0-9_-]*)`"), replacement: "`zen-$1`"},
		// Rule 5 — MCP tool bare reference
		{re: regexp.MustCompile("MCP ([a-zA-Z_][a-zA-Z0-9_-]*) tool"), replacement: "zen-$1 CLI"},
		// Rule 1 — Functional notation in backticks (prompts)
		{
			re: regexp.MustCompile("`([a-zA-Z_][a-zA-Z0-9_-]*)\\(\\{\\s*([^}]+)\\s*\\}\\)`"),
			useFunc:  true,
			replFunc: func(matches []string) string { return "`" + renderCLIFlags(matches[1], matches[2]) + "`" },
		},
		// Rule 2 — Functional notation without backticks (skills code blocks)
		{
			re:      regexp.MustCompile("(?m)^([a-zA-Z_][a-zA-Z0-9_-]*)\\(\\{\\s*([^}]+)\\s*\\}\\)$"),
			useFunc: true,
			replFunc: func(matches []string) string {
				return renderCLIFlags(matches[1], matches[2])
			},
		},
		// Rule 3 — Object-dot notation (skills TypeScript)
		{
			re:      regexp.MustCompile("mcp\\.([a-zA-Z_][a-zA-Z0-9_-]*)\\.([a-zA-Z_][a-zA-Z0-9_-]*)\\(\\{\\s*([^}]+)\\s*\\}\\)"),
			useFunc: true,
			replFunc: func(matches []string) string {
				tool := matches[1]
				method := matches[2]
				inner := matches[3]
				cli := CLITool(tool)
				params := parseKeyValuePairs(inner)
				if params == nil {
					return cli + " --" + method + " '" + inner + "'"
				}
				hasComplex := false
				for _, v := range params {
					if strings.ContainsAny(v, "[{") {
						hasComplex = true
						break
					}
				}
				if hasComplex {
					return cli + " --" + method + " '" + buildJSONObject(params) + "'"
				}
				keys := make([]string, 0, len(params))
				for k := range params {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				parts := make([]string, 0, len(keys))
				for _, k := range keys {
					parts = append(parts, "--"+k+" "+quoteIfSpaced(params[k]))
				}
				return cli + " --" + method + " " + strings.Join(parts, " ")
			},
		},
	}
}

// TransformMCPToCLI transforms MCP RPC tool-call examples in text to their CLI equivalents.
// It is idempotent: if the text already contains CLI form and no MCP RPC patterns, it returns the input unchanged.
func TransformMCPToCLI(text string) string {
	if strings.Contains(text, "zen-") && !strings.Contains(text, "mcp.") && !strings.Contains(text, "MCP ") && !strings.Contains(text, "Activate MCP") && !strings.Contains(text, "Please use MCP") && !strings.Contains(text, "({") {
		return text
	}

	for _, rule := range transformRules {
		if rule.useFunc {
			text = rule.re.ReplaceAllStringFunc(text, func(match string) string {
				matches := rule.re.FindStringSubmatch(match)
				if matches == nil {
					return match
				}
				return rule.replFunc(matches)
			})
		} else {
			text = rule.re.ReplaceAllString(text, rule.replacement)
		}
	}
	return text
}

func parseKeyValuePairs(inner string) map[string]string {
	re := regexp.MustCompile(`(\w+)\s*:\s*(?:'([^']*)'|"([^"]*)"|(\w+)|\.\.\.)`)
	matches := re.FindAllStringSubmatch(inner, -1)
	params := make(map[string]string)
	for _, m := range matches {
		key := m[1]
		var val string
		switch {
		case m[2] != "":
			val = m[2]
		case m[3] != "":
			val = m[3]
		case m[4] != "":
			val = m[4]
		default:
			continue
		}
		params[key] = val
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func buildJSONObject(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"` + k + `":"` + params[k] + `"`)
	}
	b.WriteString("}")
	return b.String()
}

func quoteIfSpaced(v string) string {
	if strings.ContainsAny(v, " \t") {
		return "'" + v + "'"
	}
	return v
}

func renderCLIFlags(tool, inner string) string {
	cli := CLITool(tool)
	params := parseKeyValuePairs(inner)
	if params == nil {
		return cli + " --json '" + inner + "'"
	}
	hasComplex := false
	for _, v := range params {
		if strings.ContainsAny(v, "[{") {
			hasComplex = true
			break
		}
	}
	if hasComplex {
		return cli + " --json '" + buildJSONObject(params) + "'"
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, "--"+k+" "+quoteIfSpaced(params[k]))
	}
	return cli + " " + strings.Join(parts, " ")
}
