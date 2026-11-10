package skills

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// A skill file is one long markdown document; an agent usually needs two or
// three of its sections, not all 1200 lines. Sections are addressable by their
// heading, so the parser has to agree with what a reader sees as a heading -
// which means ignoring the "# comment" lines inside the fenced examples, the text
// inside an HTML comment, and the YAML frontmatter block at the top of the file.
// Setext headings (a title underlined with === or ---) are not recognised: no skill
// uses them, and the underline would collide with the frontmatter and rule lines.

// Section is one ATX heading and the span of lines it owns: its heading line up
// to the line before the next heading of the same or a shallower level.
type Section struct {
	Index int      // 1-based position in document order, as printed by SectionOutline
	Level int      // 1 for "#", 6 for "######"
	Title string   // heading text without the leading hashes
	Path  []string // this heading's title plus its ancestors' titles, outermost first
	Start int      // 0-based index of the heading line
	End   int      // 0-based index one past the last line of the span
}

var atxHeadingRE = regexp.MustCompile(`^ {0,3}(#{1,6}) +(.*)$`)

// ParseSections returns every heading in content with its line span, in document
// order. Lines inside fenced code blocks and the frontmatter block are not headings.
func ParseSections(content string) []Section {
	lines := strings.Split(content, "\n")

	type found struct {
		level int
		title string
		start int
	}
	var foundList []found
	var fence string
	inComment := false
	frontmatterEnd := -1
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				frontmatterEnd = i
				break
			}
		}
	}

	for i, line := range lines {
		if frontmatterEnd > 0 && i <= frontmatterEnd {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if inComment {
			if strings.Contains(trimmed, "-->") {
				inComment = false
			}
			continue
		}
		if fence != "" {
			if isFenceClose(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if marker := fenceOpen(trimmed); marker != "" {
			fence = marker
			continue
		}
		if strings.Contains(trimmed, "<!--") {
			if !strings.Contains(trimmed, "-->") {
				inComment = true
			}
			continue
		}

		m := atxHeadingRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		foundList = append(foundList, found{
			level: len(m[1]),
			title: headingTitle(m[2]),
			start: i,
		})
	}

	sections := make([]Section, 0, len(foundList))
	var ancestors []found
	for i, f := range foundList {
		for len(ancestors) > 0 && ancestors[len(ancestors)-1].level >= f.level {
			ancestors = ancestors[:len(ancestors)-1]
		}
		path := make([]string, 0, len(ancestors)+1)
		for _, a := range ancestors {
			path = append(path, a.title)
		}
		path = append(path, f.title)

		end := len(lines)
		for j := i + 1; j < len(foundList); j++ {
			if foundList[j].level <= f.level {
				end = foundList[j].start
				break
			}
		}
		// Markdown puts a "---" rule before the *following* heading, and that rule
		// belongs to neither section: excluded, an outline range ends on a line the
		// section does not own, and `sed -n 'start,end p'` returns noise.
		for end > f.start+1 && isFillerLine(lines[end-1]) {
			end--
		}

		sections = append(sections, Section{
			Index: i + 1,
			Level: f.level,
			Title: f.title,
			Path:  path,
			Start: f.start,
			End:   end,
		})
		ancestors = append(ancestors, f)
	}
	return sections
}

// headingTitle strips an ATX closing sequence - a trailing run of "#" that follows a
// space ("## Goals # #"). A "#" glued to the text is part of it: "## C#", "## F#".
func headingTitle(raw string) string {
	title := strings.TrimSpace(raw)
	if !strings.HasSuffix(title, "#") {
		return title
	}
	stripped := strings.TrimRight(title, "#")
	if strings.HasSuffix(stripped, " ") || stripped == "" {
		return strings.TrimSpace(stripped)
	}
	return title
}

// isFillerLine reports whether a line carries no content: blank, or a horizontal rule
// used as a section separator ("---", "-----", "- - -").
func isFillerLine(line string) bool {
	compact := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if compact == "" {
		return true
	}
	for _, marker := range []string{"-", "*", "_"} {
		if len(compact) >= 3 && strings.TrimLeft(compact, marker) == "" {
			return true
		}
	}
	return false
}

// LineCount is the number of lines a document has, without the phantom element a
// trailing newline produces, so it agrees with `wc -l`.
func LineCount(content string) int {
	lines := strings.Count(content, "\n")
	if lines > 0 && !strings.HasSuffix(content, "\n") {
		lines++
	}
	return lines
}

// Range is the section's line span in the form an outline prints and a filter takes
// back: 1-based, both ends inclusive, so `sed -n 'start,end p'` reproduces it.
func (s Section) Range() string {
	return fmt.Sprintf("%d:%d", s.Start+1, s.End)
}

// fenceOpen returns the fence marker of an opening code fence, "" otherwise.
func fenceOpen(trimmed string) string {
	for _, marker := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, marker) {
			n := 0
			for n < len(trimmed) && trimmed[n] == marker[0] {
				n++
			}
			return trimmed[:n]
		}
	}
	return ""
}

// isFenceClose reports whether a line is the closing fence for marker: only fence
// characters, at least as long as the opener, with no info string.
func isFenceClose(trimmed, marker string) bool {
	if !strings.HasPrefix(trimmed, marker[:1]) || len(trimmed) < len(marker) {
		return false
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != marker[0] {
			return false
		}
	}
	return true
}

// normalizeTitle lowercases a heading and reduces everything that is not a letter
// or a digit to a single space, so "## 🛠 CORE ACTIONS" and "`web_eval`" are
// addressable as "core actions" and "web eval" - emoji, backticks and punctuation
// are decoration the caller should not have to reproduce.
func normalizeTitle(s string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			pendingSpace = false
			continue
		}
		pendingSpace = true
	}
	return b.String()
}

// selectorMatch reports whether a normalized selector addresses a heading. Either
// the whole selector is a fragment of the heading, or every word of it appears in
// the heading in any order ("actions core" still finds "CORE ACTIONS").
func selectorMatch(normSelector, normTitle string) bool {
	if normSelector == "" || normTitle == "" {
		return false
	}
	if strings.Contains(normTitle, normSelector) {
		return true
	}
	words := strings.Fields(normSelector)
	for _, w := range words {
		if !strings.Contains(normTitle, w) {
			return false
		}
	}
	return true
}

// pathMatch resolves a "parent/child" selector against a section's heading path.
// Each segment addresses one level, so "navigation/navigate" cannot pick up the
// `web_eval` heading that also sits under a "Navigate" title elsewhere.
func pathMatch(selector string, section Section) bool {
	segments := strings.Split(selector, "/")
	if len(segments) > len(section.Path) {
		return false
	}
	offset := len(section.Path) - len(segments)
	for i, seg := range segments {
		normSeg := normalizeTitle(seg)
		if normSeg == "" {
			return false
		}
		if !selectorMatch(normSeg, normalizeTitle(section.Path[offset+i])) {
			return false
		}
	}
	return true
}

// documentTitleIndex returns the heading that names the whole document - a lone
// level-1 heading above the rest - or -1. A phrase must not select it: "-f firefox"
// on a document called "Firefox MCP Bridge API" would return every line, which is
// exactly what a filter is asked to avoid. It stays reachable by outline number or
// line range, where the caller can see what it is choosing.
func documentTitleIndex(sections []Section) int {
	title, count := -1, 0
	for i, s := range sections {
		if s.Level == 1 {
			title, count = i, count+1
		}
	}
	if count == 1 && len(sections) > 1 {
		return title
	}
	return -1
}

// matchSelectors maps selectors onto sections. A bare number is an outline index,
// "start:end" is a line range from the outline, a "/" separated string is a heading
// path, anything else is a heading phrase. Returns the matched section indexes and
// the selectors that matched nothing.
func matchSelectors(sections []Section, selectors []string) ([]int, []string) {
	matched := map[int]bool{}
	var unmatched []string
	docTitle := documentTitleIndex(sections)

	for _, raw := range selectors {
		selector := strings.TrimSpace(raw)
		if selector == "" {
			continue
		}
		hit := false

		if start, end, ok := parseLineRange(selector); ok {
			// A range addresses the sections that *start* inside it, never their
			// ancestors - otherwise the document's H1 owns every line and asking for
			// one section would return the whole file.
			for i, s := range sections {
				if s.Start+1 >= start && s.Start+1 <= end {
					matched[i] = true
					hit = true
				}
			}
			if !hit {
				// The range fell inside prose: take the deepest section that owns it.
				best, deepest := -1, 0
				for i, s := range sections {
					if s.Start+1 <= end && s.End >= start && s.Level > deepest {
						best, deepest = i, s.Level
					}
				}
				if best >= 0 {
					matched[best] = true
					hit = true
				}
			}
		} else if n, err := strconv.Atoi(selector); err == nil {
			for i, s := range sections {
				if s.Index == n {
					matched[i] = true
					hit = true
				}
			}
		} else {
			// A "/" selector addresses the heading path first, then falls back to the
			// phrase reading: some headings carry the separator themselves
			// ("## Toast/Status Overlays") and would otherwise be unreachable.
			if strings.Contains(selector, "/") {
				for i, s := range sections {
					if pathMatch(selector, s) {
						matched[i] = true
						hit = true
					}
				}
			}
			if !hit {
				normSelector := normalizeTitle(selector)
				for i, s := range sections {
					if i == docTitle {
						continue
					}
					if selectorMatch(normSelector, normalizeTitle(s.Title)) {
						matched[i] = true
						hit = true
					}
				}
			}
		}

		if !hit {
			unmatched = append(unmatched, selector)
		}
	}

	// A section implies its subsections - asking for "## NAVIGATION" and losing the
	// `### navigate` detail under it would return a table of contents without body.
	spans := make([]Section, 0, len(matched))
	for i := range sections {
		if matched[i] {
			spans = append(spans, sections[i])
		}
	}
	for _, span := range spans {
		for i, s := range sections {
			if s.Start >= span.Start && s.Start < span.End {
				matched[i] = true
			}
		}
	}

	indexes := make([]int, 0, len(matched))
	for i := range matched {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	return indexes, unmatched
}

// parseLineRange reads the "start:end" span an outline hands back.
func parseLineRange(selector string) (int, int, bool) {
	parts := strings.Split(selector, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || start < 1 || end < start {
		return 0, 0, false
	}
	return start, end, true
}

// SectionFilterResult is the outcome of narrowing a skill down to selected sections.
type SectionFilterResult struct {
	Content   string   // the matched sections, in document order
	Blocks    []string // heading title each rendered block starts at
	Spans     []string // line range of each block, in Section.Range form
	Unmatched []string // selectors that matched nothing
	Kept      int      // sections kept, matched headings plus their subsections
	Lines     int      // lines of Content
	Total     int      // lines in the document
	Sections  int      // sections in the document
}

// FilterSections returns only the sections of content the selectors address.
func FilterSections(content string, selectors []string) SectionFilterResult {
	sections := ParseSections(content)
	indexes, unmatched := matchSelectors(sections, selectors)

	lines := strings.Split(content, "\n")
	result := SectionFilterResult{Unmatched: unmatched, Total: LineCount(content), Sections: len(sections)}
	if len(indexes) == 0 {
		return result
	}
	result.Kept = len(indexes)

	// Contiguous matches render as one block; a gap between matches is dropped, which
	// is the whole point of the filter.
	var blocks []string
	blockStart, blockEnd := 0, -1
	flush := func() {
		if blockEnd < 0 {
			return
		}
		blocks = append(blocks, strings.Join(lines[blockStart:blockEnd], "\n")+"\n")
		result.Spans = append(result.Spans, fmt.Sprintf("%d:%d", blockStart+1, blockEnd))
		blockEnd = -1
	}
	for _, i := range indexes {
		s := sections[i]
		if s.Start > blockEnd {
			flush()
			blockStart = s.Start
			result.Blocks = append(result.Blocks, s.Title)
		}
		blockEnd = max(blockEnd, s.End)
	}
	flush()

	joined := strings.Join(blocks, "\n")
	result.Content = joined
	result.Lines = strings.Count(joined, "\n")
	return result
}

// SectionOutline renders the addressable section index of a skill: one line per
// heading with its outline number, level, title and line span, so a caller can pick
// a section by any of the three - or read it straight off the file with `sed`.
func SectionOutline(sections []Section) string {
	var sb strings.Builder
	for _, s := range sections {
		title := s.Title
		if title == "" {
			title = "(untitled)"
		}
		sb.WriteString(fmt.Sprintf("%3d %s %s %s\n", s.Index, strings.Repeat("#", s.Level), title, s.Range()))
	}
	return sb.String()
}

// SuggestSections guesses which headings a missed selector was aiming at, by
// shared-word overlap, so a failed filter teaches the caller the real section names
// instead of only reporting that it matched nothing.
func SuggestSections(sections []Section, selectors []string, limit int) []string {
	var suggestions []string
	seen := map[string]bool{}
	docTitle := documentTitleIndex(sections)

	for _, raw := range selectors {
		words := strings.Fields(normalizeTitle(raw))
		type scored struct {
			title string
			score int
		}
		var ranked []scored
		for i, s := range sections {
			if i == docTitle {
				continue // never suggest a section a phrase filter cannot select
			}
			normTitle := normalizeTitle(s.Title)
			score := 0
			for _, w := range words {
				if strings.Contains(normTitle, w) {
					score++
				}
			}
			if score == 0 || seen[normTitle] {
				continue
			}
			ranked = append(ranked, scored{s.Title, score})
		}
		sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
		for i, r := range ranked {
			if i >= limit {
				break
			}
			seen[normalizeTitle(r.title)] = true
			suggestions = append(suggestions, r.title)
		}
	}
	return suggestions
}

// TopSectionTitles returns the section titles a document is organised by: the
// shallowest heading level that carries more than one heading, so a single document
// title does not stand in for the table of contents. It is what a caller sees when a
// filter found nothing.
func TopSectionTitles(sections []Section) []string {
	if len(sections) == 0 {
		return nil
	}

	counts := map[int]int{}
	for _, s := range sections {
		counts[s.Level]++
	}
	level := 0
	for l := 1; l <= 6; l++ {
		if counts[l] > 1 {
			level = l
			break
		}
	}
	if level == 0 {
		level = sections[0].Level
	}

	var titles []string
	for _, s := range sections {
		if s.Level == level {
			titles = append(titles, s.Title)
		}
	}
	return titles
}
