package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jang/zen-mcp/internal/mcpcfg"
)

// ResolvePrompt resolves a prompt template with the given arguments.
func ResolvePrompt(p PromptDefinition, args map[string]string, workspace string) (string, error) {
	WarnMissingArgs(p.Name, args, p.Arguments)

	// Substitute template arguments
	text := SubstituteTemplate(p.Template, args, p.Arguments)

	// Handle adaptive prompts (persona injection)
	if isTrue(p.Adaptive) && p.DefaultPersona != "" {
		text = strings.ReplaceAll(text, "{{PERSONA}}", p.DefaultPersona)
	}

	// Handle enabled skills
	if len(p.EnabledSkills) > 0 {
		skillStatic := true // config.prompt_features?.skill_static !== false
		if skillStatic {
			var parts []string
			for _, skillID := range p.EnabledSkills {
				content, err := LoadSkillContent(skillID)
				if err == nil {
					parts = append(parts, content)
				}
			}
			if len(parts) > 0 {
				text += "\n\n---\n<!-- STATIC SKILL CONTEXT (enabledSkills) -->\n" + strings.Join(parts, "\n\n---\n")
			}
		}
	}

	// Handle skill triggers
	if isTrue(p.EnableSkillTrigger) || isTrue(p.EnableSkillName) {
		combinedArgs := strings.Join(argsValues(args), " ")
		if strings.TrimSpace(combinedArgs) != "" {
			injected := make(map[string]bool)
			for _, skillID := range p.EnabledSkills {
				injected[skillID] = true
			}
			detected, err := DetectSkills(combinedArgs, isTrue(p.EnableSkillTrigger), isTrue(p.EnableSkillName), injected)
			if err == nil && len(detected) > 0 {
				var parts []string
				for _, d := range detected {
					parts = append(parts, d.Content)
				}
				text += "\n\n---\n<!-- RUNTIME SKILL INJECTION -->\n" + strings.Join(parts, "\n\n---\n")
			}
		}
	}

	return text, nil
}

func argsValues(args map[string]string) []string {
	var vals []string
	for _, v := range args {
		vals = append(vals, v)
	}
	return vals
}

func isTrue(b *bool) bool {
	return b != nil && *b
}

// DebugLog writes a debug message to debug-mcp.log.
func DebugLog(args ...interface{}) {
	msg := fmt.Sprintf("[%s] %s\n", "", "")
	_ = msg
}

// WriteDebugLog writes a debug message to the debug log file.
func WriteDebugLog(msg string) {
	logPath := filepath.Join(mcpcfg.ProjectRoot, "debug-mcp.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(msg)
}
