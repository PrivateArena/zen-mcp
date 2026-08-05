---
name: theme-maker
description: Extracts, categorizes, and batch-replaces CSS color schemes using token-efficient tools and AST parsing for flawless UI restyling.
framework: "unspecified"
trigger: theme maker
---

# 🎨 Theme-Maker Skill

Leverages abstract syntax tree (AST) parsing, color normalization, and batch-replacement tools to safely audit, extract, and swap color palettes across codebases without breaking CSS rules or layout structures.

## 🔗 Chaining with `codebase-research`

Run `codebase-research` first to locate global style definitions (e.g., `variables.css`, `tailwind.config.js`, `global.scss`, or raw component style sheets). Do not run thematic modifications blindly across the entire codebase.

## 🚀 The "Audit, Categorize, & Replace" Workflow

### 1. Identify Target Stylesheets
From the `codebase-research` output, extract the precise file paths containing the core color declarations. 

### 2. Audit & Categorize (The Extraction Phase)
Invoke the `css_color_parser` tool on the target files. This tool parses the CSS AST, normalizes all color variants (`#hex`, `rgb()`, `hsl()`, `named-colors`) to a uniform format, and groups them by visual harmony.

```bash
# Analyze the stylesheet to see what colors are currently active
theme_tools.extract_colors(
  target_file="src/styles/global.css",
  group_by="harmony" # Options: harmony (default), lightness, or declaration_type
)

```

The tool returns a structured JSON layout mapping colors to functional UI blocks:

```json
{
  "grays": ["#ffffff", "#f4f4f5", "#18181b"],
  "primaries": ["#3b82f6", "rgba(59, 130, 246, 0.5)"],
  "accents": ["#ef4444"]
}

```

### 3. Review & Map the New Palette

Present the extracted layout to the user. Once the target mapping is finalized, construct a deterministic color-swap payload. **Never allow the agent to use raw regex string replacement**, as it can accidentally corrupt shorthand hex rules or partial numerical strings.

### 4. Execute Batch Replacement

Execute the `theme_tools.batch_replace` command. The underlying tool applies changes cleanly to the AST declarations.

```bash
# Perform a precise, safe color-scheme injection
theme_tools.batch_replace(
  target_file="src/styles/global.css",
  output_file="src/styles/global.css",
  replacements={
    "#3b82f6": "#10b981", 
    "#f4f4f5": "#0f172a"  
  }
)

```

---

## 🧠 Operational Directives

* **AST Guardrail**: Always use AST-based tools for style manipulation. Reject raw string `sed` or regex-based replacement requests on CSS/SCSS files to prevent layout breakage.
* **Color Normalization**: When matching colors for replacement, treat `#FFF`, `#ffffff`, and `rgb(255, 255, 255)` as identical values. Utilize internal color conversion (`colord` or similar library logic) before processing maps.
* **The Variable First Directive**: If the target stylesheet uses hardcoded inline colors, prioritize a **Refactor Phase** first. Extract the hardcoded colors into CSS Custom Properties (`:root` variables) or Tailwind tokens, swap the inline values to `var(--color-name)`, and then modify the variables block.
* **Alpha-Channel Preservation**: When modifying a base color that features variable transparency configurations (e.g., `rgba(59, 130, 246, 0.5)`), calculate and apply the matching transparency variant to the incoming theme color.
* **Deterministic Integration**: Validate compilation/build steps (e.g., running `npm run build` or style-linting) immediately following a batch replacement to guarantee syntax validity.

---

### Why this structure works perfectly for AI Agents:
1. **Deterministic Tools (`theme_tools.*`):** Agents fail when trying to calculate HSL/RGB distances or parsing raw text blocks with long lists of hex codes. Offloading this to a simple parser tool ensures reliability.
2. **The "Variable First Directive":** This teaches the agent to leave the codebase *better* than they found it by migrating legacy "bad" code (hardcoded colors) into modern code (CSS variables) automatically before changing themes.
3. **AST Over Regex Guardrail:** Forcing the agent to avoid regex string replacements protects your project from breaking layouts when a color hex inadvertently matches a hash string somewhere else in the document.
