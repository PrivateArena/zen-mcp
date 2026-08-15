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

// initializes the package
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
}

// generateCommands is a helper function
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
