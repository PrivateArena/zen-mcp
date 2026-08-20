package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/prompts"
	"zen-mcp/internal/terminal"
)

func init() {
	terminal.Register("prompt", func(args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: prompt <name> [args...]")
		}
		name := args[0]
		def, ok := prompts.GetPromptDefinition(name)
		if !ok {
			all, err := prompts.LoadPromptDefinitions()
			if err != nil {
				return fmt.Errorf("failed to load prompts: %v", err)
			}
			var available []string
			for _, p := range all {
				available = append(available, p.Name)
			}
			terminal.Logf("ERROR: Unknown prompt '%s'. Available: %s", name, strings.Join(available, ", "))
			return nil
		}

		bodyArgs := args[1:]
		promptArgs := make(map[string]string)
		if len(bodyArgs) > 0 && len(def.Arguments) > 0 {
			if len(def.Arguments) == 1 {
				promptArgs[def.Arguments[0].Name] = strings.Join(bodyArgs, " ")
			} else {
				for i := 0; i < len(def.Arguments); i++ {
					if i < len(bodyArgs) {
						promptArgs[def.Arguments[i].Name] = bodyArgs[i]
					} else if i == len(def.Arguments)-1 {
						promptArgs[def.Arguments[i].Name] = strings.Join(bodyArgs[i:], " ")
					}
				}
			}
		}

		previewSuffix := ""
		if len(bodyArgs) > 0 {
			previewSuffix = " " + strings.Join(bodyArgs, " ")
		}
		terminal.Logf("PREVIEW: %s%s", name, previewSuffix)

		wRoot := terminal.Ws()
		text, err := prompts.ResolvePrompt(def, promptArgs, wRoot)
		if err != nil {
			terminal.Logf("ERROR: %v", err)
			return nil
		}
		terminal.Logf("\n%s\n", text)
		return nil
	})

	terminal.Register("generate-commands", func(args []string) error {
		return generateCommands()
	})

	terminal.Register("export-commands", func(args []string) error {
		return generateCommands()
	})

	terminal.Register("export-prompts_pi", func(args []string) error {
		return generatePiPrompts()
	})
}

func generateCommands() error {
	promptsDir := filepath.Join(mcpcfg.ProjectRoot, "resources", "prompts")
	commandsDir := filepath.Join(mcpcfg.ProjectRoot, "resources", "commands")

	terminal.Logf("GENERATE-COMMANDS: Converting prompts to commands...")

	if _, err := os.Stat(promptsDir); os.IsNotExist(err) {
		terminal.Logf("ERROR: Prompts directory not found: %s", promptsDir)
		return nil
	}

	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		terminal.Logf("ERROR: Failed to create commands directory: %v", err)
		return nil
	}

	defs, err := prompts.LoadPromptDefinitions()
	if err != nil {
		terminal.Logf("ERROR: Failed to load prompts: %v", err)
		return nil
	}

	generated := 0
	for _, p := range defs {
		if p.Name == "" || p.Template == "" {
			continue
		}

		var argumentHint string
		if len(p.Arguments) > 0 {
			lines := make([]string, 0, len(p.Arguments))
			for _, a := range p.Arguments {
				lines = append(lines, fmt.Sprintf("  %s: %s", a.Name, a.Description))
			}
			argumentHint = "|-\n" + strings.Join(lines, "\n")
		}

		frontmatter := fmt.Sprintf("---\ndescription: %s\n", p.Description)
		if argumentHint != "" {
			frontmatter += fmt.Sprintf("argument-hint: %s\n", argumentHint)
		}
		frontmatter += "---\n"

		body := p.Template
		if len(p.EnabledSkills) > 0 {
			var parts []string
			for _, skillID := range p.EnabledSkills {
				content, err := prompts.LoadSkillContent(skillID)
				if err == nil && content != "" {
					parts = append(parts, content)
				}
			}
			if len(parts) > 0 {
				body = strings.Join(parts, "\n\n---\n")
			}
		}

		outPath := filepath.Join(commandsDir, p.Name+".md")
		if err := os.WriteFile(outPath, []byte(frontmatter+body), 0o644); err != nil {
			terminal.Logf("ERROR: Failed to write %s: %v", outPath, err)
			continue
		}
		generated++
	}

	terminal.Logf("OK: Generated %d command file(s) in %s", generated, commandsDir)
	return nil
}

// piFakeDollar is the fullwidth dollar sign (U+FF04). Pi's prompt-template
// argument expansion only matches the ASCII '$' (substituteArgs in
// prompt-templates.ts), so a fullwidth dollar survives expansion untouched
// while still reading as '$' to the model.
const piFakeDollar = "＄"

// escapePiDollar rewrites every literal ASCII dollar sign to piFakeDollar.
func escapePiDollar(s string) string {
	return strings.ReplaceAll(s, "$", piFakeDollar)
}

// convertToPiTemplate rewrites a prompt template into Pi prompt-template
// syntax. {{name}} placeholders for declared arguments become Pi positional
// args ($1, $2, ...) that Pi WILL substitute. {{PERSONA}} falls back to the
// prompt's default persona when one is set and is otherwise preserved verbatim
// (matching the shared plain-text parser's unknown-placeholder behavior). Any
// other literal dollar sign in the template is rewritten to piFakeDollar so Pi
// never mistakes shell code for its own argument syntax.
func convertToPiTemplate(template string, args []prompts.PromptArgument, defaultPersona string) string {
	idx := make(map[string]int, len(args))
	for i, a := range args {
		idx[a.Name] = i + 1
	}
	var b strings.Builder
	b.Grow(len(template))
	rest := template
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			b.WriteString(escapePiDollar(rest))
			break
		}
		b.WriteString(escapePiDollar(rest[:start]))
		after := rest[start+2:]
		end := strings.Index(after, "}}")
		if end < 0 {
			b.WriteString(escapePiDollar(rest[start:]))
			break
		}
		name := strings.TrimSpace(after[:end])
		switch {
		case idx[name] > 0:
			b.WriteString(fmt.Sprintf("$%d", idx[name]))
		case strings.EqualFold(name, "PERSONA") && defaultPersona != "":
			b.WriteString(escapePiDollar(defaultPersona))
		default:
			b.WriteString(rest[start : start+2+end+2])
		}
		rest = after[end+2:]
	}
	return b.String()
}

// generatePiPrompts converts every loaded prompt definition into a Pi prompt
// template (https://pi.dev/docs/latest/prompt-templates) under
// resources/prompts-pi/. The design mirrors generateCommands: one .md file per
// prompt with YAML frontmatter. Named arguments become Pi positional args
// ($1, $2, ...), the argument-hint uses <required> and [optional] markers,
// shell-code dollar signs are faked to a fullwidth dollar so Pi never
// substitutes them, and enabled skills are referenced via the zskill CLI
// instead of embedding their content, keeping the exported template lean.
func generatePiPrompts() error {
	promptsDir := filepath.Join(mcpcfg.ProjectRoot, "resources", "prompts")
	piDir := filepath.Join(mcpcfg.ProjectRoot, "resources", "prompts-pi")

	terminal.Logf("GENERATE-PROMPTS-PI: Converting prompts to Pi prompt templates...")

	if _, err := os.Stat(promptsDir); os.IsNotExist(err) {
		terminal.Logf("ERROR: Prompts directory not found: %s", promptsDir)
		return nil
	}

	if err := os.MkdirAll(piDir, 0o755); err != nil {
		terminal.Logf("ERROR: Failed to create prompts-pi directory: %v", err)
		return nil
	}

	defs, err := prompts.LoadPromptDefinitions()
	if err != nil {
		terminal.Logf("ERROR: Failed to load prompts: %v", err)
		return nil
	}

	generated := 0
	for _, p := range defs {
		if p.Name == "" || p.Template == "" {
			continue
		}

		var hintParts []string
		for _, a := range p.Arguments {
			if a.Required {
				hintParts = append(hintParts, fmt.Sprintf("<%s>", a.Name))
			} else {
				hintParts = append(hintParts, fmt.Sprintf("[%s]", a.Name))
			}
		}

		frontmatter := fmt.Sprintf("---\ndescription: %s\n", p.Description)
		if len(hintParts) > 0 {
			frontmatter += fmt.Sprintf("argument-hint: %s\n", strings.Join(hintParts, " "))
		}
		frontmatter += "---\n"

		body := convertToPiTemplate(p.Template, p.Arguments, p.DefaultPersona)
		if len(p.EnabledSkills) > 0 {
			body += prompts.SkillActivationBlock("zskill -a get -i %s", "Load the required skill(s) with the CLI when needed instead of working from memory:", p.EnabledSkills)
		}

		outPath := filepath.Join(piDir, p.Name+".md")
		if err := os.WriteFile(outPath, []byte(frontmatter+body), 0o644); err != nil {
			terminal.Logf("ERROR: Failed to write %s: %v", outPath, err)
			continue
		}
		generated++
	}

	terminal.Logf("OK: Generated %d Pi prompt template(s) in %s", generated, piDir)
	return nil
}
