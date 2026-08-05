package analysis

import (
	"encoding/json"
	"regexp"
	"strings"
)

type FileTypeResult struct {
	Type       string  `json:"type"`
	Subtype    string  `json:"subtype,omitempty"`
	Confidence float64 `json:"confidence"`
	Mime       string  `json:"mime,omitempty"`
}

// DetectFileType ports detectFileType from file-type.ts.
func DetectFileType(text string) FileTypeResult {
	sample := strings.TrimSpace(sliceRunes(text, 4096))
	if sample == "" {
		return FileTypeResult{Type: "text", Confidence: 0.5, Mime: "text/plain"}
	}

	if isBinary(sample) {
		return FileTypeResult{Type: "binary", Confidence: 0.95, Mime: "application/octet-stream"}
	}

	if r := tryJSON(sample); r != nil {
		return *r
	}
	if r := tryHTML(sample); r != nil {
		return *r
	}
	if r := tryXML(sample); r != nil {
		return *r
	}
	if r := tryYAML(sample); r != nil {
		return *r
	}
	if r := tryMarkdown(sample); r != nil {
		return *r
	}
	if r := tryLog(sample); r != nil {
		return *r
	}
	if r := tryCSV(sample); r != nil {
		return *r
	}
	if r := tryDiff(sample); r != nil {
		return *r
	}

	return FileTypeResult{Type: "text", Confidence: 0.9, Mime: "text/plain"}
}

func isBinary(sample string) bool {
	control := sliceRunes(sample, 1024)
	controlCount := 0
	for _, r := range control {
		if r < 32 && r != 9 && r != 10 && r != 13 {
			controlCount++
		}
	}
	return controlCount > len([]rune(control))*5/100
}

func tryJSON(sample string) *FileTypeResult {
	if !strings.HasPrefix(sample, "{") && !strings.HasPrefix(sample, "[") {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(sample), &v); err != nil {
		return nil
	}
	subtype := "object"
	if strings.HasPrefix(sample, "[") {
		subtype = "array"
	}
	return &FileTypeResult{Type: "json", Subtype: subtype, Confidence: 0.95, Mime: "application/json"}
}

func tryHTML(sample string) *FileTypeResult {
	lower := strings.ToLower(sample)
	if strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") ||
		regexp.MustCompile(`^<!\s*doctype\s+html`).MatchString(sample) {
		return &FileTypeResult{Type: "html", Confidence: 0.9, Mime: "text/html"}
	}
	return nil
}

func tryXML(sample string) *FileTypeResult {
	if !strings.HasPrefix(sample, "<?xml") && !regexp.MustCompile(`^<\w+[\s>]`).MatchString(sample) {
		return nil
	}
	if strings.Contains(strings.ToLower(sample), "<!doctype html>") {
		return nil
	}
	m := regexp.MustCompile(`<(\w+)[\s>]`).FindStringSubmatch(sample)
	if m != nil {
		return &FileTypeResult{Type: "xml", Subtype: m[1], Confidence: 0.8, Mime: "application/xml"}
	}
	return nil
}

func tryYAML(sample string) *FileTypeResult {
	if strings.HasPrefix(sample, "---\n") || strings.HasPrefix(sample, "---\r\n") {
		return &FileTypeResult{Type: "yaml", Confidence: 0.85, Mime: "application/x-yaml"}
	}
	firstLine := strings.SplitN(sample, "\n", 2)[0]
	if firstLine == "" || strings.Contains(sample, "{") {
		return nil
	}
	if regexp.MustCompile(`^[\w.-]+:\s`).MatchString(firstLine) {
		return &FileTypeResult{Type: "yaml", Confidence: 0.7, Mime: "application/x-yaml"}
	}
	return nil
}

func tryMarkdown(sample string) *FileTypeResult {
	if regexp.MustCompile(`^#{1,6}\s`).MatchString(sample) {
		return &FileTypeResult{Type: "markdown", Confidence: 0.8, Mime: "text/markdown"}
	}
	if regexp.MustCompile(`^[-*+]\s`).MatchString(sample) {
		return &FileTypeResult{Type: "markdown", Confidence: 0.6, Mime: "text/markdown"}
	}
	if regexp.MustCompile(`^\d+\.\s`).MatchString(sample) {
		return &FileTypeResult{Type: "markdown", Confidence: 0.5, Mime: "text/markdown"}
	}
	if regexp.MustCompile(`^>\s`).MatchString(sample) {
		return &FileTypeResult{Type: "markdown", Confidence: 0.7, Mime: "text/markdown"}
	}
	if strings.HasPrefix(sample, "```") {
		return &FileTypeResult{Type: "markdown", Confidence: 0.7, Mime: "text/markdown"}
	}
	if regexp.MustCompile(`^[-*_]{3,}\s*$`).MatchString(strings.SplitN(sample, "\n", 2)[0]) {
		return &FileTypeResult{Type: "markdown", Confidence: 0.6, Mime: "text/markdown"}
	}
	return nil
}

var (
	reLogTimestamp = regexp.MustCompile(`^\d{4}[-\/]\d{2}[-\/]\d{2}[T ]\d{2}:\d{2}`)
	reLogBracket   = regexp.MustCompile(`^\[(INFO|WARN|ERROR|DEBUG|TRACE)\b`)
	reLogBare      = regexp.MustCompile(`^(INFO|WARN|ERROR|DEBUG|TRACE)\b`)
)

func tryLog(sample string) *FileTypeResult {
	var lines []string
	for _, l := range strings.Split(sample, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	matchCount := 0
	for _, line := range lines {
		if len(line) > 20 {
			line = line[:20]
		}
		if reLogTimestamp.MatchString(line) || reLogBracket.MatchString(strings.TrimSpace(line)) || reLogBare.MatchString(strings.TrimSpace(line)) {
			matchCount++
		}
	}
	minCount := len(lines)
	if minCount > 5 {
		minCount = 5
	}
	if matchCount >= minCount {
		return &FileTypeResult{Type: "log", Confidence: 0.75}
	}
	return nil
}

func tryCSV(sample string) *FileTypeResult {
	var lines []string
	for _, l := range strings.Split(sample, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) < 2 {
		return nil
	}
	rows := lines
	if len(rows) > 10 {
		rows = rows[:10]
	}
	delimiter := guessDelimiter(rows)
	if delimiter == "" {
		return nil
	}
	counts := make([]int, len(rows))
	re := regexp.MustCompile(`,`)
	if delimiter == "tab" {
		re = regexp.MustCompile("\t")
	}
	allSame := true
	for i, r := range rows {
		counts[i] = len(re.FindAllString(r, -1))
		if i > 0 && counts[i] != counts[i-1] {
			allSame = false
		}
	}
	if allSame && counts[0] > 0 {
		mime := "text/csv"
		if delimiter == "tab" {
			mime = "text/tab-separated-values"
		}
		return &FileTypeResult{Type: "csv", Subtype: delimiter, Confidence: 0.7, Mime: mime}
	}
	return nil
}

func guessDelimiter(rows []string) string {
	commaRe := regexp.MustCompile(`,`)
	tabRe := regexp.MustCompile("\t")
	commaCounts := make([]int, len(rows))
	tabCounts := make([]int, len(rows))
	allComma, allTab := true, true
	for i, r := range rows {
		commaCounts[i] = len(commaRe.FindAllString(r, -1))
		tabCounts[i] = len(tabRe.FindAllString(r, -1))
		if commaCounts[i] == 0 || commaCounts[i] != commaCounts[0] {
			allComma = false
		}
		if tabCounts[i] == 0 || tabCounts[i] != tabCounts[0] {
			allTab = false
		}
	}
	if allComma {
		return "comma"
	}
	if allTab {
		return "tab"
	}
	return ""
}

func tryDiff(sample string) *FileTypeResult {
	if !regexp.MustCompile(`^---\s`).MatchString(sample) {
		return nil
	}
	lines := strings.Split(sample, "\n")
	hasPlus, hasAt := false, false
	for _, l := range lines {
		if strings.HasPrefix(l, "+++") {
			hasPlus = true
		}
		if strings.HasPrefix(l, "@@") {
			hasAt = true
		}
	}
	if hasPlus || hasAt {
		return &FileTypeResult{Type: "diff", Confidence: 0.9, Mime: "text/x-diff"}
	}
	return nil
}

func sliceRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
