package analysis

// ReadingAdvice mirrors the TS ReadingAdvice shape.
type ReadingAdvice struct {
	Tool        string  `json:"tool"`
	Explanation string  `json:"explanation"`
	Warning     *string `json:"warning"`
}

// SuggestReadingTool ports suggestReadingTool from tool-suggestion.ts.
func SuggestReadingTool(ft FileTypeResult) ReadingAdvice {
	switch ft.Type {
	case "json":
		return ReadingAdvice{
			Tool:        "jq",
			Explanation: "Use 'jq keys' for top-level keys or 'jq path(..)' to explore deep paths. Never view raw JSON.",
			Warning:     strPtr("NEVER view raw JSON. Use jq to query specific paths."),
		}
	case "yaml":
		return ReadingAdvice{
			Tool:        "yq",
			Explanation: `Use 'yq keys' for top-level keys or 'yq eval ".. | select(has(\"keys\")) | keys"' for deep exploration. Never view raw YAML.`,
			Warning:     strPtr("NEVER view raw YAML. Use yq to query specific paths."),
		}
	case "xml":
		return ReadingAdvice{
			Tool:        "xq / xmllint",
			Explanation: "Use 'xq .' (via yq) to convert to JSON then jq, or 'xmllint --format' to pretty-print structure. Never view raw XML.",
			Warning:     strPtr("NEVER view raw XML. Use xq/xmllint to navigate structure."),
		}
	case "html":
		return ReadingAdvice{
			Tool:        "browser",
			Explanation: "View rendered HTML via browser.navigate or browser.screenshot for visual inspection.",
		}
	case "markdown":
		return ReadingAdvice{
			Tool:        "browser or file.read",
			Explanation: "Markdown is human-readable. Use file.read for raw or browser for rendered view.",
		}
	case "code":
		sub := ""
		if ft.Subtype != "" {
			sub = " Syntax highlighting available for " + ft.Subtype + "."
		}
		return ReadingAdvice{
			Tool:        "file.read with offset/limit",
			Explanation: "Use file.read with offset/limit to view specific sections." + sub,
		}
	case "log":
		return ReadingAdvice{
			Tool:        "grep / tail / head",
			Explanation: "Use grep to filter relevant entries, tail for recent lines, or head for the first lines. Avoid reading the full log at once.",
		}
	case "csv":
		return ReadingAdvice{
			Tool:        "column -t -s, / csvtool",
			Explanation: "Use 'column -t -s,' for readable table output or csvtool for programmatic access.",
		}
	case "diff":
		return ReadingAdvice{
			Tool:        "diffstat / colordiff",
			Explanation: "Use diffstat for summary statistics, colordiff for highlighted view, or file.read for raw content.",
		}
	case "binary":
		return ReadingAdvice{
			Tool:        "xxd / strings / file",
			Explanation: "Binary content detected. Use file(1) to identify type, strings(1) to extract readable text, or xxd for hex dump.",
			Warning:     strPtr("NEVER read binary output directly — it will corrupt the terminal session."),
		}
	default:
		return ReadingAdvice{
			Tool:        "file.read",
			Explanation: "Plain text content. Use file.read with offset/limit to avoid large reads.",
		}
	}
}

// returns a pointer to the given string
func strPtr(s string) *string { return &s }
