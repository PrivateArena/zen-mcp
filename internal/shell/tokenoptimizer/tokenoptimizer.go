// Package tokenoptimizer ports src/lib/shell/token-optimizer.ts: per-command
// output compaction (compactGit*, compactLs, compactGrep, compactCat,
// compactTestOutput, ...), chained-command safe optimization, the shell
// output blacklist, savings estimation and token-profiles.json actions
// (replace/file; delegate is deferred to the M5 web-agent bridge).
package tokenoptimizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jang/zen-mcp/internal/logfilter"
)

// Config carries the token_optimization + shell_output_blacklist config.
type Config struct {
	Enabled              bool
	UltraCompact         bool
	MaxChainedLength     int
	DeduplicateThreshold int
	ProfilesPath         string
	Blacklist            []BlacklistEntry
}

type BlacklistEntry struct {
	Match      string
	IsRegex    bool
	MaxLines   int
	DropOutput bool
	Label      string
}

type Options struct {
	UltraCompact      bool
	SkipOptimization  bool
}

type TokenProfile struct {
	Name   string        `json:"name"`
	Match  ProfileMatch  `json:"match"`
	Action ProfileAction `json:"action"`
}

type ProfileMatch struct {
	Command string `json:"command"`
	Type    string `json:"type"` // contains | exact | regex
}

type ProfileAction struct {
	Type     string   `json:"type"` // replace | delegate | file
	Find     *string  `json:"find"`
	Replace  *string  `json:"replace"`
	IsRegex  *bool    `json:"is_regex"`
	Flags    *string  `json:"flags"`
	Provider *string  `json:"provider"`
	Prompt   *string  `json:"prompt"`
	App      *string  `json:"app"`
	Container *string `json:"container"`
	Upload   json.RawMessage `json:"upload"`
	Path     *string  `json:"path"`
	Message  *string  `json:"message"`
}

func CountTokens(text string) int {
	return (len([]byte(text)) + 3) / 4
}

func GetSavings(original, filtered string) int {
	orig := CountTokens(original)
	if orig == 0 {
		return 0
	}
	f := CountTokens(filtered)
	return int((float64(orig-f)/float64(orig))*100 + 0.5)
}

// ---------------------------------------------------------------------------
// Compaction helpers
// ---------------------------------------------------------------------------

func deduplicateLines(text string) string {
	lines := strings.Split(text, "\n")
	counts := map[string]int{}
	var unique []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if n, ok := counts[trimmed]; ok {
			counts[trimmed] = n + 1
		} else {
			counts[trimmed] = 1
			unique = append(unique, line)
		}
	}
	result := strings.Join(unique, "\n")
	for line, count := range counts {
		if count > 1 {
			result = strings.Replace(result, line, line+" (×"+itoa(count)+")", 1)
		}
	}
	return result
}

func compactGitStatus(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var modified, deleted, untracked, staged []string
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "M ") || strings.HasPrefix(line, " M"):
			modified = append(modified, strings.TrimSpace(line[2:]))
		case strings.HasPrefix(line, "D "):
			deleted = append(deleted, strings.TrimSpace(line[2:]))
		case strings.HasPrefix(line, "??"):
			untracked = append(untracked, strings.TrimSpace(line[2:]))
		case strings.HasPrefix(line, "A ") || strings.HasPrefix(line, "M\t"):
			staged = append(staged, strings.TrimSpace(line[2:]))
		}
	}
	var sections []string
	if len(staged) > 0 {
		sections = append(sections, sectionLine("Staged", staged))
	}
	if len(modified) > 0 {
		sections = append(sections, sectionLine("Modified", modified))
	}
	if len(deleted) > 0 {
		sections = append(sections, sectionLine("Deleted", deleted))
	}
	if len(untracked) > 0 {
		sections = append(sections, sectionLine("Untracked", untracked))
	}
	if len(sections) == 0 {
		return "✓ Clean"
	}
	return strings.Join(sections, "\n")
}

func sectionLine(name string, items []string) string {
	first := items
	if len(first) > 10 {
		first = first[:10]
	}
	s := name + " (" + itoa(len(items)) + "): " + strings.Join(first, ", ")
	if len(items) > 10 {
		s += "..."
	}
	return s
}

var statRe = regexp.MustCompile(`^(\S+)\s*\|\s*(\d+)\s*([+-]+)`)

func compactGitDiff(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return ""
	}
	if len(lines) < 40 {
		return strings.TrimSpace(output)
	}

	fileCount, insertions, deletions := 0, 0, 0
	var fileChanges []string
	for _, line := range lines {
		m := statRe.FindStringSubmatch(line)
		if m != nil {
			fileCount++
			count := atoi(m[2])
			signs := m[3]
			if strings.Contains(signs, "+") {
				insertions += count
			}
			if strings.Contains(signs, "-") {
				deletions += count
			}
			fileChanges = append(fileChanges, m[1]+": "+signs+itoa(count))
		}
	}

	if fileCount == 0 {
		return strings.Join(lines[:30], "\n") + "\n... [diff truncated, " + itoa(len(lines)) + " lines total]"
	}

	result := itoa(fileCount) + " files: +" + itoa(insertions) + "/-" + itoa(deletions) + "\n"
	if len(fileChanges) > 15 {
		result += strings.Join(fileChanges[:15], "\n")
		result += "\n... +" + itoa(len(fileChanges)-15) + " more files"
	} else {
		result += strings.Join(fileChanges, "\n")
	}
	return result
}

func compactGitLog(output string, options Options) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if options.UltraCompact {
		first := lines
		if len(first) > 10 {
			first = first[:10]
		}
		out := make([]string, 0, len(first))
		for _, l := range first {
			r := []rune(l)
			if len(r) > 60 {
				r = r[:60]
			}
			out = append(out, string(r))
		}
		return strings.Join(out, "\n")
	}
	if len(lines) > 15 {
		lines = lines[:15]
	}
	return strings.Join(lines, "\n")
}

func compactLs(output string, options Options) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if options.UltraCompact {
		out := make([]string, 0, len(lines))
		for _, l := range lines {
			parts := strings.Fields(strings.TrimSpace(l))
			if len(parts) == 0 {
				out = append(out, strings.TrimSpace(l))
			} else {
				out = append(out, parts[len(parts)-1])
			}
		}
		return strings.Join(out, "\n")
	}

	isDetailed := false
	if len(lines) > 0 {
		if strings.Contains(lines[0], "total") {
			isDetailed = true
		} else if m := regexp.MustCompile(`^[d-]\S+\s+\d+`).MatchString(lines[0]); m {
			isDetailed = true
		}
	}

	if isDetailed {
		var items []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || trimmed == "total" || regexp.MustCompile(`^\d+$`).MatchString(trimmed) {
				continue
			}
			parts := strings.Fields(trimmed)
			name := parts[len(parts)-1]
			isDir := strings.HasPrefix(strings.TrimSpace(line), "d") || strings.Contains(parts[0], "drwx")
			if isDir {
				items = append(items, name+"/")
			} else {
				items = append(items, name)
			}
		}
		return strings.Join(items, "\n")
	}
	return strings.Join(lines, "\n")
}

var listModeRe = regexp.MustCompile(`^[a-zA-Z]:\\|\/|^[^\/]+$`)
var fileColonRe = regexp.MustCompile(`^([^:]+):`)
var grepLineRe = regexp.MustCompile(`^([^:]+):(\d+):?(.*)$`)

func compactGrep(output string, options Options) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return ""
	}
	isListMode := listModeRe.MatchString(lines[0])

	if isListMode || options.UltraCompact {
		files := map[string]bool{}
		for _, line := range lines {
			if m := fileColonRe.FindStringSubmatch(line); m != nil {
				files[m[1]] = true
			} else if strings.Contains(line, "/") {
				parts := strings.Split(line, "/")
				files[parts[len(parts)-1]] = true
			} else {
				files[line] = true
			}
		}
		maxFiles := 30
		if options.UltraCompact {
			maxFiles = 10
		}
		fileList := make([]string, 0, len(files))
		for f := range files {
			fileList = append(fileList, f)
		}
		sort.Strings(fileList)
		if len(fileList) > maxFiles {
			fileList = fileList[:maxFiles]
		}
		result := strings.Join(fileList, "\n")
		if len(files) > maxFiles {
			result += "\n... +" + itoa(len(files)-maxFiles) + " more"
		}
		return result
	}

	fileCounts := map[string]int{}
	fileMatches := map[string][]string{}
	for _, line := range lines {
		m := grepLineRe.FindStringSubmatch(line)
		if m != nil {
			file := m[1]
			content := m[3]
			if r := []rune(content); len(r) > 80 {
				content = string(r[:80])
			}
			content = strings.TrimSpace(content)
			fileCounts[file]++
			limit := 5
			if options.UltraCompact {
				limit = 2
			}
			if len(fileMatches[file]) < limit {
				fileMatches[file] = append(fileMatches[file], m[2]+": "+content)
			}
		} else if strings.Contains(line, ":") {
			file := strings.Split(line, ":")[0]
			fileCounts[file]++
		}
	}

	if len(fileCounts) == 0 {
		if len(lines) > 30 {
			lines = lines[:30]
		}
		return strings.Join(lines, "\n")
	}

	maxFilesToShow := 15
	if options.UltraCompact {
		maxFilesToShow = 5
	}
	// deterministic order for stable output
	keys := make([]string, 0, len(fileCounts))
	for k := range fileCounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	shown := 0
	for _, file := range keys {
		if shown >= maxFilesToShow {
			break
		}
		parts := strings.Split(file, "/")
		shortFile := parts[len(parts)-1]
		if shortFile == "" {
			shortFile = file
		}
		result.WriteString(shortFile + ": " + itoa(fileCounts[file]))
		if ms := fileMatches[file]; len(ms) > 0 {
			result.WriteString("\n  " + strings.Join(ms, "\n  "))
		}
		result.WriteString("\n")
		shown++
	}
	if len(fileCounts) > maxFilesToShow {
		result.WriteString("... +" + itoa(len(fileCounts)-maxFilesToShow) + " more files\n")
	}
	return strings.TrimSpace(result.String())
}

func compactCat(output string, options Options) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	totalLines := len(lines)
	if options.UltraCompact {
		keepLines := totalLines
		if totalLines > 100 {
			keepLines = 50
		}
		result := strings.Join(lines[:keepLines], "\n")
		if totalLines > keepLines {
			return result + "\n... +" + itoa(totalLines-keepLines) + " lines"
		}
		return result
	}
	if totalLines > 500 {
		return strings.Join(lines[:400], "\n") + "\n\n... +" + itoa(totalLines-400) + " more lines (use head/tail/range to see specific parts)"
	}
	return output
}

var benchRe = regexp.MustCompile(`^(\S+)-?\d+\s+(\d+)\s+(\d+)\s+ns\/op(?:\s+(\d+)\s+B\/op)?(?:\s+(\d+)\s+allocs\/op)?`)

type benchResult struct {
	name   string
	ns     int
	allocs int
}

func compactGoBench(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var benchmarks []benchResult
	hasFailures := false
	for _, line := range lines {
		m := benchRe.FindStringSubmatch(line)
		if m != nil {
			benchmarks = append(benchmarks, benchResult{name: m[1], ns: atoi(m[3]), allocs: atoiOr(m[5], 0)})
		}
		if strings.Contains(strings.ToLower(line), "fail") || strings.Contains(strings.ToLower(line), "error") {
			hasFailures = true
		}
	}

	if hasFailures {
		var failureLines []string
		for _, l := range lines {
			lower := strings.ToLower(l)
			if strings.Contains(lower, "fail") || strings.Contains(lower, "error") ||
				strings.Contains(lower, "panic") || strings.Contains(lower, "fatal") || strings.HasPrefix(l, "---") {
				failureLines = append(failureLines, l)
			}
		}
		result := strings.Join(failureLines[:30], "\n")
		if len(failureLines) > 30 {
			result += "\n... +" + itoa(len(failureLines)-30) + " more failures"
		}
		return result
	}

	if len(benchmarks) == 0 {
		var summary []string
		for _, l := range lines {
			if strings.HasPrefix(l, "ok") || strings.HasPrefix(l, "PASS") || strings.HasPrefix(l, "FAIL") || strings.Contains(l, "ns/op") {
				summary = append(summary, l)
			}
			if len(summary) >= 5 {
				break
			}
		}
		if len(summary) > 0 {
			return strings.Join(summary, "\n")
		}
		return strings.TrimSpace(output)
	}

	sort.Slice(benchmarks, func(i, j int) bool { return benchmarks[i].ns < benchmarks[j].ns })
	var result strings.Builder
	result.WriteString("Benchmark results (" + itoa(len(benchmarks)) + " benchmarks):\n")
	for _, b := range benchmarks[:min(10, len(benchmarks))] {
		ns := itoa(b.ns) + "ns"
		if b.ns >= 1000 {
			ns = formatMicros(float64(b.ns) / 1000)
		}
		result.WriteString("  " + b.name + ": " + ns + "/op")
		if b.allocs > 0 {
			result.WriteString(" " + itoa(b.allocs) + " allocs")
		}
		result.WriteString("\n")
	}
	if len(benchmarks) > 10 {
		result.WriteString("  ... +" + itoa(len(benchmarks)-10) + " more\n")
	}
	return strings.TrimSpace(result.String())
}

func formatMicros(v float64) string {
	return strings.TrimSuffix(strings.TrimSuffix(formatFloat(v, 1), "0"), ".") + "µs"
}

func formatFloat(v float64, prec int) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func compactTestOutput(output, command string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	lowerOutput := strings.ToLower(output)

	isRust := strings.Contains(command, "cargo test")
	isJest := strings.Contains(command, "jest") || strings.Contains(command, "npm test") || strings.Contains(command, "yarn test")
	isPytest := strings.Contains(command, "pytest") || strings.Contains(command, "python -m pytest")
	isGo := strings.Contains(command, "go test")
	isGoBench := isGo && (strings.Contains(command, "bench") || strings.Contains(command, "-bench"))
	isVitest := strings.Contains(command, "vitest")

	if isGoBench {
		return compactGoBench(output)
	}

	if !isGo && (strings.Contains(lowerOutput, "test result: ok") ||
		(strings.Contains(lowerOutput, "ok") && !strings.Contains(lowerOutput, "fail") && !strings.Contains(lowerOutput, "error"))) {
		return "✓ All tests passed"
	}

	var failures, passed []string
	failedCount, passedCount := 0, 0

	testLines := lines
	if isGo {
		testLines = nil
		for _, line := range lines {
			lower := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(lower, "pass") || strings.HasPrefix(lower, "fail") ||
				strings.HasPrefix(lower, "ok") || strings.HasPrefix(lower, "---") ||
				strings.HasPrefix(lower, "===") || strings.Contains(lower, "coverage:") ||
				strings.Contains(lower, "--- fail") || strings.Contains(lower, "--- pass") ||
				strings.Contains(lower, "error") || strings.Contains(lower, "panic") ||
				strings.Contains(lower, "fatal") ||
				regexp.MustCompile(`^\s*===[=]+\s+(run|pass|fail)`).MatchString(lower) ||
				regexp.MustCompile(`^\s*---[ -]+\s+(pass|fail|skip)`).MatchString(lower) {
				testLines = append(testLines, line)
			}
		}
	}

	if isGo {
		var failureLines []string
		for _, l := range testLines {
			if strings.HasPrefix(l, "FAIL") || regexp.MustCompile(`^\s*---[ -]+\s*FAIL`).MatchString(l) ||
				strings.HasPrefix(l, "panic:") || strings.HasPrefix(l, "fatal error:") {
				failureLines = append(failureLines, l)
			}
		}
		if len(failureLines) > 0 {
			var failingTests []string
			for _, l := range failureLines {
				if regexp.MustCompile(`^\s*---[ -]+\s*FAIL`).MatchString(l) {
					failingTests = append(failingTests, l)
				}
			}
			count := len(failureLines)
			if len(failingTests) > 0 {
				count = len(failingTests)
			}
			result := "FAILED: " + itoa(count) + " test(s)\n"
			for _, f := range failureLines[:min(10, len(failureLines))] {
				result += "  " + strings.TrimSpace(f) + "\n"
			}
			return strings.TrimSpace(result)
		}
		var passLines []string
		for _, l := range testLines {
			if strings.Contains(l, ": pass") || strings.HasPrefix(l, "--- PASS") || strings.HasPrefix(l, "ok") {
				passLines = append(passLines, l)
			}
		}
		if len(passLines) > 0 {
			return "✓ All tests passed"
		}
	}

	for _, line := range testLines {
		lower := strings.ToLower(line)
		switch {
		case isPytest:
			if strings.Contains(lower, "passed") && strings.Contains(lower, "failed") {
				passMatch := regexp.MustCompile(`(\d+)\s+passed`).FindStringSubmatch(line)
				failMatch := regexp.MustCompile(`(\d+)\s+failed`).FindStringSubmatch(line)
				if passMatch != nil {
					passedCount = atoi(passMatch[1])
				}
				if failMatch != nil {
					failedCount = atoi(failMatch[1])
				}
			}
			if strings.Contains(lower, "failed") {
				m := regexp.MustCompile(`^(.*?)(?:\s+FAILED|\s+ERROR)`).FindStringSubmatch(line)
				if m != nil {
					failures = append(failures, strings.TrimSpace(m[1]))
				}
			}
		case isGo:
			if strings.Contains(command, "bench") {
				if strings.HasPrefix(lower, "ok") {
					return "✓ Benchmarks passed"
				}
				if strings.HasPrefix(lower, "fail") {
					failedCount++
					failures = append(failures, strings.TrimSpace(line))
				}
			} else {
				if strings.HasPrefix(lower, "fail") {
					failedCount++
					failures = append(failures, strings.TrimSpace(strings.Replace(line, "FAIL", "", 1)))
				} else if strings.Contains(lower, "pass") && !strings.Contains(lower, "fail") {
					passedCount++
				}
			}
		case isJest || isVitest:
			if strings.Contains(line, "✓") || strings.Contains(line, "PASS") {
				passed = append(passed, strings.TrimSpace(replaceCheckmarks(line)))
			} else if strings.Contains(line, "✗") || strings.Contains(line, "FAIL") || strings.Contains(line, "✕") {
				failures = append(failures, strings.TrimSpace(replaceCheckmarks(line)))
			}
		case isRust:
			if strings.Contains(lower, "test result:") {
				if strings.Contains(lower, "ok") {
					return "✓ All tests passed"
				}
				m := regexp.MustCompile(`(\d+)\s+failed`).FindStringSubmatch(line)
				if m != nil {
					failedCount = atoi(m[1])
				}
			}
			if strings.Contains(lower, "test ") && strings.Contains(lower, "... f") {
				failures = append(failures, strings.TrimSpace(line))
			}
		}
	}

	var result string
	switch {
	case failedCount > 0:
		result = "FAILED: " + itoa(failedCount) + "/" + itoa(failedCount+passedCount) + " tests\n"
		for _, f := range failures[:min(10, len(failures))] {
			result += "  • " + f + "\n"
		}
		if len(failures) > 10 {
			result += "  ... +" + itoa(len(failures)-10) + " more\n"
		}
	case len(failures) > 0:
		result = "FAILED: " + itoa(len(failures)) + " tests\n"
		for _, f := range failures[:min(10, len(failures))] {
			result += "  • " + f + "\n"
		}
	default:
		result = "✓ All tests passed"
	}
	return strings.TrimSpace(result)
}

func replaceCheckmarks(s string) string {
	for _, r := range []rune{'✓', '✗', '✕'} {
		s = strings.ReplaceAll(s, string(r), "")
	}
	return s
}

func compactRuff(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || (len(lines) == 1 && strings.TrimSpace(lines[0]) == "") {
		return "✓ No issues found"
	}

	var arr []map[string]any
	if json.Unmarshal([]byte(output), &arr) == nil && arr != nil {
		byFile := map[string]int{}
		byRule := map[string]int{}
		for _, issue := range arr {
			file := anyString(issue, "filename", "file", "location", "unknown")
			rule := anyString(issue, "code", "rule", "", "unknown")
			byFile[file]++
			byRule[rule]++
		}
		result := "Found " + itoa(len(arr)) + " issues:\n"
		keys := sortedKeys(byRule)
		for _, rule := range keys {
			result += "  " + rule + ": " + itoa(byRule[rule]) + "\n"
		}
		return strings.TrimSpace(result)
	}

	ruffLineRe := regexp.MustCompile(`^([^:]+):(\d+):(\d+):\s*(.+)`)
	byFile := map[string]int{}
	var issues []string
	for _, line := range lines {
		m := ruffLineRe.FindStringSubmatch(line)
		if m != nil {
			parts := strings.Split(m[1], "/")
			file := parts[len(parts)-1]
			if file == "" {
				file = m[1]
			}
			byFile[file]++
			if len(issues) < 10 {
				issues = append(issues, file+":"+m[2]+": "+m[4])
			}
		}
	}
	if len(byFile) == 0 {
		first := lines
		if len(first) > 10 {
			first = first[:10]
		}
		return strings.Join(first, "\n")
	}
	result := "Found " + itoa(len(lines)) + " issues:\n"
	for _, file := range sortedKeys(byFile) {
		result += "  " + file + ": " + itoa(byFile[file]) + "\n"
	}
	if len(issues) > 0 {
		result += "\nSample:\n"
		for _, issue := range issues[:min(5, len(issues))] {
			result += "  • " + issue + "\n"
		}
	}
	return strings.TrimSpace(result)
}

func anyString(m map[string]any, keys ...string) string {
	last := keys[len(keys)-1]
	keys = keys[:len(keys)-1]
	for _, key := range keys {
		if key == "" {
			continue
		}
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
		if loc, ok := m["location"].(map[string]any); ok {
			if v, ok := loc[key].(string); ok && v != "" {
				return v
			}
		}
	}
	return last
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func compactJq(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 2 {
		var v any
		if json.Unmarshal([]byte(strings.TrimSpace(output)), &v) == nil {
			return compactJSONStructure(v, 0)
		}
		return strings.TrimSpace(output)
	}
	if len(lines) > 50 {
		return strings.Join(lines[:10], "\n") + "\n... +" + itoa(len(lines)-10) + " more lines"
	}
	return output
}

func compactJSONStructure(v any, depth int) string {
	if depth > 3 {
		return "..."
	}
	switch val := v.(type) {
	case []any:
		if len(val) == 0 {
			return "[]"
		}
		limit := 3
		if depth == 0 {
			limit = 10
		}
		var inner []string
		for i, item := range val {
			if i >= limit {
				break
			}
			inner = append(inner, compactJSONStructure(item, depth+1))
		}
		result := "[\n  " + strings.Join(inner, ", ")
		if len(val) > limit {
			result += "\n  ... +" + itoa(len(val)-limit) + " more\n]"
		} else {
			result += "\n]"
		}
		return result
	case map[string]any:
		if len(val) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		limit := 5
		if depth == 0 {
			limit = 15
		}
		var preview []string
		for i, k := range keys {
			if i >= limit {
				break
			}
			preview = append(preview, k+": "+compactJSONStructure(val[k], depth+1))
		}
		result := "{ " + strings.Join(preview, ", ")
		if len(keys) > limit {
			result += ", ..."
		}
		return result + " }"
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func compactGitAdd(output string) string {
	if strings.Contains(output, "No files") {
		return "✓ No files to stage"
	}
	return "✓ Staged"
}

func compactGitCommit(output string) string {
	m := regexp.MustCompile(`^(\S{7,})`).FindStringSubmatch(output)
	hash := ""
	if m != nil {
		hash = m[1]
		if len(hash) > 8 {
			hash = hash[:8]
		}
	}
	if hash != "" {
		return "✓ " + hash
	}
	return "✓ Committed"
}

func compactGitPush(output string) string {
	if strings.Contains(output, "Everything up-to-date") {
		return "✓ Up to date"
	}
	m := regexp.MustCompile(`(\S+)\s*->\s*(\S+)`).FindStringSubmatch(output)
	if m != nil {
		return "✓ " + m[2]
	}
	return "✓ Pushed"
}

// ---------------------------------------------------------------------------
// optimizeOutput
// ---------------------------------------------------------------------------

func optimizeChainedCommand(output string, cfg Config) string {
	trimmed := strings.TrimSpace(output)
	threshold := cfg.DeduplicateThreshold
	if threshold <= 0 {
		threshold = 3
	}
	result := deduplicateWithThreshold(trimmed, threshold)
	result = collapseNewlines(result)
	maxLen := cfg.MaxChainedLength
	if maxLen <= 0 {
		maxLen = 50 * 1024
	}
	if len(result) > maxLen {
		result = result[:maxLen] +
			"\n\n... [output truncated: " + itoa((len(result)+1023)/1024) + "KB → " + itoa((maxLen+1023)/1024) + "KB]"
	}
	return result
}

func collapseNewlines(s string) string {
	re := regexp.MustCompile(`\n{3,}`)
	return re.ReplaceAllString(s, "\n\n")
}

func deduplicateWithThreshold(text string, minCount int) string {
	lines := strings.Split(text, "\n")
	counts := map[string]int{}
	var unique []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			unique = append(unique, line)
			continue
		}
		if n, ok := counts[trimmed]; ok {
			counts[trimmed] = n + 1
		} else {
			counts[trimmed] = 1
			unique = append(unique, line)
		}
	}
	if len(counts) == 0 {
		return text
	}
	result := strings.Join(unique, "\n")
	hasDuplicates := false
	for line, count := range counts {
		if count > minCount {
			hasDuplicates = true
			result = regexp.MustCompile(regexpQuoteMeta(line)).ReplaceAllString(result, line+" (×"+itoa(count)+")")
		}
	}
	if hasDuplicates {
		return result
	}
	return text
}

func regexpQuoteMeta(s string) string {
	return regexp.QuoteMeta(s)
}

func safeGlobalOptimize(output string) string {
	lines := strings.Split(output, "\n")
	var optimized []string
	lastWasEmpty := false
	multiSpaceRe := regexp.MustCompile(`\s{2,}`)
	for _, line := range lines {
		stripped := strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(stripped) == "" {
			if !lastWasEmpty && len(optimized) > 0 {
				optimized = append(optimized, "")
				lastWasEmpty = true
			}
			continue
		}
		finalLine := stripped
		if len([]rune(stripped)) > 200 {
			finalLine = multiSpaceRe.ReplaceAllString(stripped, " ")
		}
		optimized = append(optimized, finalLine)
		lastWasEmpty = false
	}
	return strings.Join(optimized, "\n")
}

func OptimizeOutput(command, output string, options Options, cfg Config) string {
	trimmed := strings.TrimSpace(command)

	isChained := containsAny(trimmed, "&&", "||", ";")
	hasPipe := strings.Contains(trimmed, "|")
	if isChained || hasPipe {
		return optimizeChainedCommand(output, cfg)
	}

	parts := strings.Fields(trimmed)
	firstWord := ""
	subcommand := ""
	if len(parts) > 0 {
		firstWord = parts[0]
	}
	if len(parts) > 1 {
		subcommand = parts[1]
	}

	if firstWord == "git" {
		switch subcommand {
		case "status":
			return compactGitStatus(output)
		case "diff":
			return compactGitDiff(output)
		case "log":
			return compactGitLog(output, options)
		case "add":
			return compactGitAdd(output)
		case "commit":
			return compactGitCommit(output)
		case "push":
			return compactGitPush(output)
		case "pull":
			if strings.Contains(output, "Already up to date") {
				return "✓ Up to date"
			}
			lines := strings.Split(strings.TrimSpace(output), "\n")
			if len(lines) > 5 {
				lines = lines[:5]
			}
			return strings.Join(lines, "\n")
		}
	}

	if firstWord == "ls" || firstWord == "ll" || firstWord == "la" {
		return compactLs(output, options)
	}

	if firstWord == "tree" {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		if options.UltraCompact {
			var items []string
			for _, l := range lines {
				if strings.Contains(l, "├──") || strings.Contains(l, "└──") || strings.Contains(l, "│") {
					items = append(items, l)
				}
			}
			var dirs, files []string
			cleanRe := regexp.MustCompile(`^[│\s]+[└├┄]+`)
			cleanRe2 := regexp.MustCompile(`[└├┄]\s*`)
			for _, item := range items[:min(25, len(items))] {
				name := strings.TrimSpace(cleanRe2.ReplaceAllString(cleanRe.ReplaceAllString(item, ""), ""))
				if name == "" {
					continue
				}
				if strings.HasSuffix(name, "/") || (strings.Contains(item, "/") && !regexp.MustCompile(`\.[a-z]+$`).MatchString(item)) {
					dirs = append(dirs, name)
				} else {
					files = append(files, name)
				}
			}
			root := "tree"
			if len(lines) > 0 {
				sp := strings.Split(lines[0], "/")
				root = sp[len(sp)-1]
			}
			result := root
			if len(dirs) > 0 {
				result += "\n" + strings.Join(dirs, "\n")
			}
			if len(files) > 0 {
				result += "\n" + strings.Join(files[:15], "\n")
			}
			if len(files) > 15 {
				result += "\n+" + itoa(len(files)-15) + " files"
			}
			return result
		}
		if len(lines) > 50 {
			return strings.Join(lines[:40], "\n") + "\n..."
		}
		return output
	}

	if firstWord == "cat" {
		return compactCat(output, options)
	}

	if firstWord == "rg" || firstWord == "grep" || firstWord == "ag" {
		return compactGrep(output, options)
	}

	isTestCommand := testCommand(firstWord, subcommand)
	if isTestCommand {
		return compactTestOutput(output, command)
	}

	if firstWord == "go" && (subcommand == "test" || strings.HasPrefix(subcommand, "test-")) {
		return compactTestOutput(output, command)
	}

	if firstWord == "go" || firstWord == "npm" || firstWord == "yarn" || firstWord == "pnpm" ||
		firstWord == "bun" || firstWord == "cargo" || firstWord == "python" {
		return output
	}

	if firstWord == "ruff" {
		return compactRuff(output)
	}

	if firstWord == "jq" {
		return compactJq(output)
	}

	globallyOptimized := safeGlobalOptimize(output)
	fallbackThreshold := cfg.DeduplicateThreshold
	if fallbackThreshold <= 0 {
		fallbackThreshold = 3
	}
	fallbackThreshold += 2
	deduplicated := deduplicateWithThreshold(globallyOptimized, fallbackThreshold)
	if deduplicated != globallyOptimized {
		return deduplicated
	}
	if len([]byte(globallyOptimized)) < len([]byte(output)) {
		return globallyOptimized
	}
	return output
}

func testCommand(firstWord, subcommand string) bool {
	switch firstWord {
	case "npm", "yarn", "pnpm":
		if subcommand == "test" || strings.HasPrefix(subcommand, "test:") {
			return true
		}
		return strings.HasPrefix(subcommand, "run ") && (strings.Contains(subcommand, "test") || strings.HasSuffix(subcommand, ":test"))
	case "bun":
		if subcommand == "test" {
			return true
		}
		return strings.HasPrefix(subcommand, "run ") && strings.Contains(subcommand, "test")
	case "cargo":
		return subcommand == "test" || subcommand == "bench"
	case "pytest", "jest", "vitest":
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Blacklist
// ---------------------------------------------------------------------------

// ApplyBlacklist returns a replacement for output when an entry matches, or
// nil when no entry matches.
func ApplyBlacklist(command, output string, blacklist []BlacklistEntry) *string {
	if len(blacklist) == 0 {
		return nil
	}
	cmd := strings.TrimSpace(command)
	for _, entry := range blacklist {
		matched := false
		if entry.IsRegex {
			if re, err := regexp.Compile(entry.Match); err == nil {
				matched = re.MatchString(cmd)
			} else {
				matched = strings.Contains(cmd, entry.Match)
			}
		} else {
			matched = strings.Contains(cmd, entry.Match)
		}
		if !matched {
			continue
		}
		label := entry.Label
		if label == "" {
			label = entry.Match
		}
		if entry.DropOutput {
			s := "[output suppressed — blacklisted command: " + label + "]"
			return &s
		}
		cap := entry.MaxLines
		if cap <= 0 {
			cap = 30
		}
		lines := strings.Split(output, "\n")
		if len(lines) <= cap {
			return &output
		}
		s := strings.Join(lines[:cap], "\n") +
			"\n... [truncated by blacklist rule \"" + label + "\": " + itoa(len(lines)) + " → " + itoa(cap) + " lines]"
		return &s
	}
	return nil
}

// ---------------------------------------------------------------------------
// Token profiles
// ---------------------------------------------------------------------------

type ProfileResult struct {
	Stdout  string
	Stderr  string
	Applied bool
}

func ApplyTokenProfiles(command, stdout, stderr string, options Options, cfg Config) ProfileResult {
	if options.SkipOptimization {
		return ProfileResult{Stdout: stdout, Stderr: stderr, Applied: false}
	}
	profilesPath := cfg.ProfilesPath
	if profilesPath == "" {
		profilesPath = "token-profiles.json"
	}
	raw, err := os.ReadFile(profilesPath)
	if err != nil {
		return ProfileResult{Stdout: stdout, Stderr: stderr, Applied: false}
	}
	var profiles []TokenProfile
	if json.Unmarshal(raw, &profiles) != nil {
		logfilter.Debug("[TokenOptimizer] Failed to parse token-profiles.json")
		return ProfileResult{Stdout: stdout, Stderr: stderr, Applied: false}
	}

	finalStdout, finalStderr := stdout, stderr
	applied := false
	cmd := strings.TrimSpace(command)

	for _, profile := range profiles {
		if profile.Match.Command == "" || profile.Action.Type == "" {
			continue
		}
		matched := false
		switch profile.Match.Type {
		case "exact":
			matched = cmd == strings.TrimSpace(profile.Match.Command)
		case "regex":
			if re, err := regexp.Compile(profile.Match.Command); err == nil {
				matched = re.MatchString(cmd)
			} else {
				matched = strings.Contains(cmd, profile.Match.Command)
			}
		default:
			matched = strings.Contains(cmd, profile.Match.Command)
		}
		if !matched {
			continue
		}
		logfilter.Info(`[TokenOptimizer] Matched profile: "` + profile.Name + `"`)

		switch profile.Action.Type {
		case "replace":
			if profile.Action.Find == nil {
				continue
			}
			findVal := *profile.Action.Find
			replaceVal := ""
			if profile.Action.Replace != nil {
				replaceVal = *profile.Action.Replace
			}
			isRegex := profile.Action.IsRegex != nil && *profile.Action.IsRegex
			if isRegex {
				flags := ""
				if profile.Action.Flags != nil {
					flags = *profile.Action.Flags
				}
				if re, err := compileWithFlags(findVal, flags); err == nil {
					finalStdout = re.ReplaceAllString(finalStdout, replaceVal)
					finalStderr = re.ReplaceAllString(finalStderr, replaceVal)
					applied = true
				}
			} else {
				finalStdout = strings.ReplaceAll(finalStdout, findVal, replaceVal)
				finalStderr = strings.ReplaceAll(finalStderr, findVal, replaceVal)
				applied = true
			}
		case "delegate":
			// Deferred to M5 (agent-bridge). Fall through to generic handling.
			logfilter.Debug("[TokenOptimizer] Delegate profile deferred (M5 web-agent bridge): " + profile.Name)
		case "file":
			targetPath := "/tmp/zenmcp-do-not-read/"
			if profile.Action.Path != nil {
				targetPath = *profile.Action.Path
			}
			filePath := redirectToFile(command, targetPath, finalStdout, finalStderr)
			if filePath != "" {
				template := "[Output redirected to file: {path}]"
				if profile.Action.Message != nil {
					template = *profile.Action.Message
				}
				finalStdout = strings.ReplaceAll(template, "{path}", filePath)
				finalStderr = ""
				applied = true
			}
		}
	}
	return ProfileResult{Stdout: finalStdout, Stderr: finalStderr, Applied: applied}
}

func compileWithFlags(pattern, flags string) (*regexp.Regexp, error) {
	if strings.Contains(flags, "i") {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

func redirectToFile(command, targetPath, stdout, stderr string) string {
	lastSegment := ""
	if idx := strings.LastIndexAny(targetPath, "/\\"); idx >= 0 {
		lastSegment = targetPath[idx+1:]
	} else {
		lastSegment = targetPath
	}
	isDir := strings.HasSuffix(targetPath, "/") || strings.HasSuffix(targetPath, "\\") || !strings.Contains(lastSegment, ".")

	if isDir {
		if _, err := os.Stat(targetPath); err != nil {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				logfilter.Debug("[TokenOptimizer] Failed to create directory " + targetPath + ": " + err.Error())
			}
		}
		appName := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(command)), " ", "-")
		appName = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(appName, "-")
		appName = strings.Trim(appName, "-")
		if appName == "" {
			appName = "output"
		}
		ext := "txt"
		if strings.Contains(command, "diff") {
			ext = "diff"
		} else if strings.Contains(command, "status") {
			ext = "status"
		}
		filePath := filepath.Join(targetPath, appName+"-"+timestampName()+"."+ext)
		return writeProfileFile(filePath, command, stdout, stderr)
	}

	parentDir := filepath.Dir(targetPath)
	if _, err := os.Stat(parentDir); err != nil {
		if err := os.MkdirAll(parentDir, 0o755); err != nil {
			logfilter.Debug("[TokenOptimizer] Failed to create parent directory " + parentDir + ": " + err.Error())
		}
	}
	return writeProfileFile(targetPath, command, stdout, stderr)
}

func writeProfileFile(filePath, command, stdout, stderr string) string {
	content := "COMMAND: " + command + "\n\nSTDOUT:\n" + stdout + "\n\nSTDERR:\n" + stderr + "\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		logfilter.Debug("[TokenOptimizer] Failed to write output to file: " + err.Error())
		return ""
	}
	logfilter.Info("[TokenOptimizer] Saved output to file " + filePath)
	return filePath
}

func timestampName() string {
	return time.Now().UTC().Format("2006-01-02-150405")
}

// helpers
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
