package prompts

import (
	"regexp"
	"sort"
	"strings"

	"zen-mcp/internal/mcpcfg"
)

// cliPrefix returns the configured CLI wrapper prefix (default "zen-").
func cliPrefix() string {
	return mcpcfg.CliModePrefixOrDefault()
}

// CLITool returns the CLI wrapper name for an MCP tool name, honoring the
// configured climode_prefix. Unknown tools fall back to "<prefix>" + name.
func CLITool(mcpName string) string {
	return cliPrefix() + mcpName
}

// harnessToolSet snapshots the configured harness-tool set for one transform pass,
// so the rules do not re-read the config for every regexp match. Harness tools come
// from the agent rather than from zen-mcp and have no generated CLI wrapper, so the
// set lives in config.json (`harness_tools`, additive on top of
// mcpcfg.DefaultHarnessTools) instead of in Go source.
func harnessToolSet() map[string]bool {
	names := mcpcfg.HarnessToolsOrDefault()
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// TransformRule pairs a compiled regexp with its replacement.
type TransformRule struct {
	re          *regexp.Regexp
	replacement string
	useFunc     bool
	// toolGroup is the capture group holding the referenced tool name, or 0 when the
	// rule matches skill ids instead of tools. A name listed in harness_tools skips
	// the rule entirely (see harnessToolSet).
	toolGroup int
	replFunc  func([]string) string
}

var transformRules []TransformRule

func init() {
	transformRules = []TransformRule{
		// Rule 7 — MCP skill activation (injected block) — handles both
		// "Activate MCP skill id=X" and "Activate MCP `skill id=X`" forms.
		{
			re:      regexp.MustCompile("Activate MCP (`?)skill id=([a-zA-Z0-9_-]+)(`?)"),
			useFunc: true,
			replFunc: func(matches []string) string {
				return "Activate skill id=" + matches[2] + " via `" + CLITool("skill") + " --action get --id=" + matches[2] + "`"
			},
		},
		// Rule 8 — MCP Tool skill reference
		{
			re:      regexp.MustCompile("MCP Tool skill id=([a-zA-Z0-9_-]+)"),
			useFunc: true,
			replFunc: func(matches []string) string {
				return "`" + CLITool("skill") + " --action get --id=" + matches[1] + "`"
			},
		},
		// Rule 9 — Inline skill id backticks
		{
			re:      regexp.MustCompile("`skill id=([a-zA-Z0-9_-]+)`"),
			useFunc: true,
			replFunc: func(matches []string) string {
				return "`" + CLITool("skill") + " --action get --id=" + matches[1] + "`"
			},
		},
		// Rule 6 — MCP shell reference (specific prefix)
		{
			re:        regexp.MustCompile("Always use the MCP `([^`]+)`"),
			useFunc:   true,
			toolGroup: 1,
			replFunc: func(matches []string) string {
				return "Always use the `" + cliPrefix() + matches[1] + "` CLI"
			},
		},
		// Rule 4 — MCP tool backtick reference
		{
			re:        regexp.MustCompile("MCP `([a-zA-Z_][a-zA-Z0-9_-]*)`"),
			useFunc:   true,
			toolGroup: 1,
			replFunc: func(matches []string) string {
				return "`" + cliPrefix() + matches[1] + "`"
			},
		},
		// Rule 5 — MCP tool bare reference
		{
			re:        regexp.MustCompile("MCP ([a-zA-Z_][a-zA-Z0-9_-]*) tool"),
			useFunc:   true,
			toolGroup: 1,
			replFunc: func(matches []string) string {
				return cliPrefix() + matches[1] + " CLI"
			},
		},
		// Rule 1 — Functional notation in backticks (prompts)
		// Inner content regex allows one level of nested {} pairs (e.g. arrays).
		{
			re:        regexp.MustCompile("`([a-zA-Z_][a-zA-Z0-9_-]*)\\(\\{\\s*((?:[^{}]|\\{[^{}]*\\})*)\\s*\\}\\)`"),
			useFunc:   true,
			toolGroup: 1,
			replFunc: func(matches []string) string {
				return "`" + renderCLIFlags(matches[1], matches[2]) + "`"
			},
		},
		// Rule 2 — Functional notation without backticks (skills code blocks)
		{
			re:        regexp.MustCompile("(?m)^([a-zA-Z_][a-zA-Z0-9_-]*)\\(\\{\\s*((?:[^{}]|\\{[^{}]*\\})*)\\s*\\}\\)$"),
			useFunc:   true,
			toolGroup: 1,
			replFunc: func(matches []string) string {
				return renderCLIFlags(matches[1], matches[2])
			},
		},
		// Rule 3 — Object-dot notation (skills TypeScript)
		{
			re:        regexp.MustCompile("mcp\\.([a-zA-Z_][a-zA-Z0-9_-]*)\\.([a-zA-Z_][a-zA-Z0-9_-]*)\\(\\{\\s*((?:[^{}]|\\{[^{}]*\\})*)\\s*\\}\\)"),
			useFunc:   true,
			toolGroup: 1,
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
	prefix := cliPrefix()
	legacy := mcpcfg.DefaultCliModePrefix
	// The configured prefix is checked first; a legacy text that already used the
	// default "zen-" prefix is also skipped so changing climode_prefix keeps the
	// transformation idempotent across a config switch.
	if (strings.Contains(text, prefix) || (prefix != legacy && strings.Contains(text, legacy))) &&
		!strings.Contains(text, "mcp.") && !strings.Contains(text, "MCP ") && !strings.Contains(text, "Activate MCP") && !strings.Contains(text, "Please use MCP") && !strings.Contains(text, "({") {
		return text
	}

	harness := harnessToolSet()
	for _, rule := range transformRules {
		if rule.useFunc {
			text = rule.re.ReplaceAllStringFunc(text, func(match string) string {
				matches := rule.re.FindStringSubmatch(match)
				if matches == nil {
					return match
				}
				if rule.toolGroup > 0 && rule.toolGroup < len(matches) && harness[matches[rule.toolGroup]] {
					return match // harness tool: no wrapper exists, keep the original text
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
	re := regexp.MustCompile(`(\w+)\s*:\s*(?:'([^']*)'|"([^"]*)"|(\w+)|(<[^>]*>)|\.\.\.)`)
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
		case m[5] != "":
			val = m[5]
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
