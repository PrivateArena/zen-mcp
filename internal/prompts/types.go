package prompts

const skillPromptPrefix = "_"

// PromptArgument represents a prompt argument definition.
type PromptArgument struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

// PromptDefinition represents a prompt definition.
type PromptDefinition struct {
	Name                string           `yaml:"name"`
	Description         string           `yaml:"description"`
	Arguments           []PromptArgument `yaml:"arguments"`
	Template            string           `yaml:"template"`
	EnabledSkills       []string         `yaml:"enabledSkills"`
	EnableMemoryContext *bool            `yaml:"enableMemoryContext"`
	EnableSkillTrigger  *bool            `yaml:"enableSkillTrigger"`
	EnableSkillName     *bool            `yaml:"enableSkillName"`
	SuggestSkills       *bool            `yaml:"suggestSkills"`
	Adaptive            *bool            `yaml:"adaptive"`
	DefaultPersona      string           `yaml:"defaultPersona"`
}

// Skill represents a skill definition.
type Skill struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Triggers    []string `yaml:"triggers"`
}

// DEFAULT_PROMPTS are the built-in prompts.
var DEFAULT_PROMPTS = []PromptDefinition{
	{
		Name:        "init",
		Description: "Initialize a task and automatically detect relevant skills",
		Arguments: []PromptArgument{
			{Name: "i", Description: "The task description or query", Required: true},
		},
		Template: "Task: {{i}}\n\nSystem: Scanning for relevant skills...",
	},
	{
		Name:        "debug-proxy-site",
		Description: "Investigate why a website is broken in zen-core",
		Arguments: []PromptArgument{
			{Name: "i", Description: "The URL to investigate", Required: true},
		},
		Template: `I need to debug why {{i}} is not working correctly with zen-core.
Please follow this plan:

1. **Production Test**: Use productionTester to fetch {{i}} and see the actual response code and errors.
2. **Rule Test**: Use ruleTester to check if any rules in networkrules are blocking {{i}}.
3. **Log Check**: Use logViewer to search for "BLOCK" or "ERROR" related to this domain in the zen-core logs.
4. **Analysis**: Provide specific instructions on how to modify the Go code if a false positive is found.`,
	},
	{
		Name:        "debug-zen-midi",
		Description: "Diagnose visual or process issues in zen-midi",
		Arguments:   []PromptArgument{},
		Template: `I am seeing issues with zen-midi. Please investigate:

1. **Process Check**: Use processManager to check if 'zen-midi' is running and what its uptime is.
2. **Visual Capture**:
   - If running: Use screenshotCapture (mode: 'window' or 'selection') to capture the current state.
   - Or use videoRecorder for a 5-second clip if it's an animation issue.
3. **Log Analysis**: Use logViewer to tail the last 50 lines of the zen-midi log file.
4. **Report**: Summarize the findings.`,
	},
}

// boolPtr is a helper function
func boolPtr(b bool) *bool {
	return &b
}
