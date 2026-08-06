package tokenoptimizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jang/zen-mcp/internal/analysis"
	"github.com/jang/zen-mcp/internal/logfilter"
	"github.com/jang/zen-mcp/internal/projectmemory"
)

const virtualizeLimit = 24 * 1024

var vocabRe = regexp.MustCompile(`[a-z0-9_\-]{3,20}`)

var stopWords = map[string]bool{
	"function": true, "const": true, "return": true, "import": true, "string": true,
	"number": true, "public": true, "export": true, "class": true, "let": true,
	"interface": true, "false": true, "true": true, "null": true, "undefined": true,
	"from": true, "this": true, "void": true, "async": true, "await": true,
	"awaiting": true, "with": true, "index": true, "type": true, "object": true,
	"array": true, "boolean": true, "default": true, "module": true, "require": true,
	"the": true, "and": true, "a": true, "to": true, "of": true, "in": true,
	"is": true, "that": true, "it": true, "he": true, "was": true, "for": true,
	"on": true, "are": true, "as": true, "his": true, "they": true,
	"i": true, "at": true, "be": true, "have": true,
	"or": true, "one": true, "had": true, "by": true, "word": true, "but": true,
	"not": true, "what": true, "all": true, "were": true, "we": true, "when": true,
	"your": true, "can": true, "said": true, "there": true, "use": true, "an": true,
	"each": true, "which": true, "she": true, "do": true, "how": true, "their": true,
	"if": true, "will": true, "up": true, "other": true, "about": true, "out": true,
	"many": true, "then": true, "them": true, "these": true, "so": true, "some": true,
	"her": true, "would": true, "make": true, "like": true, "him": true, "into": true,
	"time": true, "has": true, "look": true, "two": true, "more": true, "write": true,
	"go": true, "see": true,
}

// CheckAndVirtualizeOutput ports checkAndVirtualizeOutput: when a tool's text
// output exceeds 24KB, index it into virtual_store and return a compact JSON
// handle for the context tool. workspaceRoot is injected by the caller
// (toolresponse) from the current shared state.
func CheckAndVirtualizeOutput(toolName, text, workspaceRoot string) string {
	byteLength := len([]byte(text))
	if byteLength <= virtualizeLimit {
		return text
	}
	if strings.HasPrefix(text, "[CONTEXT VIRTUALIZED") ||
		(strings.HasPrefix(text, "{") && strings.Contains(text, `"index_handle"`)) {
		return text
	}
	if toolName == "skill" || toolName == "skills" || toolName == "memory" {
		return text
	}
	if workspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return text
		}
		workspaceRoot = cwd
	}

	dbPath := filepath.Join(workspaceRoot, ".zenmcp", "context.db")
	virtID := projectmemory.IndexVirtualContext(dbPath, projectmemory.VirtualContextData{
		SourceTool: toolName,
		Intent:     "Virtualization for " + toolName,
		Payload:    text,
	})
	if virtID == "" {
		return text
	}

	result := analysis.AnalyzeOutput(text)
	if err := analysis.StoreOutputAnalysis(dbPath, virtID, result); err != nil {
		logfilter.Debug("[TokenOptimizer] Failed to store output analysis: " + err.Error())
	}

	distinctTerms := extractDistinctVocabulary(text)
	lineCount := len(strings.Split(text, "\n"))
	kbSize := formatKb(byteLength)

	out, err := json.MarshalIndent(map[string]any{
		"status":       "success",
		"summary":      "Successfully virtualized large output from tool '" + toolName + "'.",
		"index_handle": virtID,
		"analysis": map[string]any{
			"file_type":      result.FileType.Type,
			"subtype":        orUndefined(result.FileType.Subtype),
			"confidence":     result.FileType.Confidence,
			"reading_tool":   result.ReadingAdvice.Tool,
			"reading_advice": result.ReadingAdvice.Explanation,
			"warning":        result.ReadingAdvice.Warning,
			"line_count":     result.LineCount,
			"volume_kb":      kbSize,
		},
		"vocabulary_preview": distinctTerms,
		"match_count":        lineCount,
		"volume_kb":          kbSize,
		"action_required":    "Use 'context' tool with query set to '" + virtID + "'.",
	}, "", "  ")
	if err != nil {
		return text
	}
	return string(out)
}

func orUndefined(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func formatKb(byteLength int) string {
	kb := float64(byteLength) / 1024
	return twoDecimals(kb)
}

func twoDecimals(v float64) string {
	n := int64(v*100 + 0.5)
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	return sign + itoa(int(n/100)) + "." + pad2(int(n%100))
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func extractDistinctVocabulary(text string) []string {
	words := vocabRe.FindAllString(strings.ToLower(text), -1)
	seen := map[string]bool{}
	var unique []string
	for _, w := range words {
		if stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		unique = append(unique, w)
		if len(unique) >= 15 {
			break
		}
	}
	return unique
}
