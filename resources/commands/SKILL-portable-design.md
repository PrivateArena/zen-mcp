---
description: Portable & Multi-Path Configuration Loading
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: Portable & Multi-Path Configuration Loading

---
name: portable-design
description: "Portable & Multi-Path Configuration Loading"
framework: golang
path: src/resources/skills/portable-design.md
---
# Skill: Portable & Multi-Path Configuration Loading

## Objective
Implement a robust, predictable, and discoverable configuration loading system that works across different deployment scenarios: **Portable Suites**, **CLI Tools**, and **Installed Applications**.

## The "Gold Standard" Priority List
To balance portability and system integration, search for `config.json` (or similar) in this specific order:

1.  **Binary Directory**: (Highest Priority) Checks if a config exists alongside the executable. This enables "Portable Mode" where the app and its settings are a single unit.
2.  **Current Working Directory (CWD)**: Allows for project-specific overrides. Useful when running a tool from a specific data folder.
3.  **User Config Directory**: (XDG/Home) Provides a global fallback for installed applications (e.g., `~/.config/appname/config.json`).

---

## Implementation (Go)

### 1. Resolving the Base Directory
Always use `os.Executable()` instead of `os.Args[0]` to find the binary, but account for `go run` where the binary is in a temporary folder.

```go
func getBinaryDir() string {
	exe, err := os.Executable()
	if err != nil {
		// Fallback to CWD if executable path cannot be resolved
		dir, _ := os.Getwd()
		return dir
	}
	
	dir := filepath.Dir(exe)
	
	// Detect 'go run' or temp builds
	if strings.Contains(exe, "go-build") || strings.Contains(dir, "Temp") {
		dir, _ = os.Getwd()
	}
	
	return dir
}
```

### 2. The Loading Logic
Iterate through potential paths and return the first one that exists.

```go
func loadConfig() *Config {
	binDir := getBinaryDir()
	
	configPaths := []string{
		filepath.Join(binDir, "config.json"),
		"config.json", // Working Directory
		filepath.Join(os.Getenv("HOME"), ".config", "myapp", "config.json"),
	}

	for _, path := range configPaths {
		absPath, _ := filepath.Abs(path)
		if _, err := os.Stat(absPath); err == nil {
			data, err := os.ReadFile(absPath)
			if err != nil {
				continue
			}
			
			var c Config
			if err := json.Unmarshal(data, &c); err == nil {
				// LOGGING IS CRITICAL FOR DISCOVERY
				log.Printf("[Config] Loaded from: %s", absPath)
				return &c
			}
		}
	}
	return nil
}
```

---

## Best Practices

### 🛡️ Discovery & Transparency
Because multi-path loading can be "magic," always log the **absolute path** of the loaded file to `stderr`. This helps users understand *which* file is actually being used without reading source code.

### 🔒 Portable Lock (`portable.dat`)
If your application must **strictly** remain in its folder (to avoid polluting the system), check for a dummy file named `portable.dat` in the binary directory. If present, ignore the User Config/Home paths.

### ⚙️ Fallback Behavior
If no config file is found:
1.  **Strict Tools**: Exit with an error.
2.  **User-Friendly Tools**: Create a `default_config.json` in the **Binary Directory** to show the user where settings should live.

### 🌐 Cross-Platform
Use `os.UserConfigDir()` instead of hardcoded `~/.config` for better compatibility with Windows (`AppData/Roaming`) and macOS (`Library/Application Support`).

---

## Summary
| Mode | Priority | Typical Use Case |
| :--- | :--- | :--- |
| **Portable** | 1 | USB stick, Project-local suite, ZIP distribution. |
| **Project** | 2 | Per-project settings (like `.eslintrc` or `go.mod`). |
| **System** | 3 | Package manager installs (`apt`, `brew`, `yay`). |


---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=portable-design`