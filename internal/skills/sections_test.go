package skills

import (
	"strconv"
	"strings"
	"testing"
)

// skillDoc mirrors the shape of a real skill: frontmatter, an emoji heading, an
// example block whose "# comment" lines must not read as headings, and a nested
// subsection under the section a caller filters for.
const skillDoc = `---
name: demo
description: "demo skill"
---

# 🦊 Demo Skill API

Intro paragraph.

## 🛠 CORE ACTIONS

| Action | Description |
|--------|-------------|
| ping   | check      |

` + "```zsh" + `
# not a heading
zdemo -a ping   # -> { "success": true }
` + "```" + `

---

## 🧭 NAVIGATION

### navigate

Opens a page.

### refresh

Reloads it.

## DATA

Reads storage.
`

func TestParseSections_SkipsFencedCommentsAndFrontmatter(t *testing.T) {
	sections := ParseSections(skillDoc)

	var titles []string
	for _, s := range sections {
		titles = append(titles, s.Title)
	}
	joined := strings.Join(titles, " | ")

	if strings.Contains(joined, "not a heading") {
		t.Errorf("fenced code comment parsed as heading: %s", joined)
	}
	for _, want := range []string{"🛠 CORE ACTIONS", "🧭 NAVIGATION", "navigate", "DATA"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected section %q in %s", want, joined)
		}
	}
	for _, s := range sections {
		if strings.HasPrefix(s.Title, "name:") || strings.HasPrefix(s.Title, "description") {
			t.Errorf("frontmatter parsed as heading: %q", s.Title)
		}
	}
}

func TestParseSections_SpansCoverSubsections(t *testing.T) {
	sections := ParseSections(skillDoc)
	lines := strings.Split(skillDoc, "\n")

	var nav *Section
	for i := range sections {
		if sections[i].Level == 2 && strings.Contains(sections[i].Title, "NAVIGATION") {
			nav = &sections[i]
		}
	}
	if nav == nil {
		t.Fatal("NAVIGATION section not found")
	}
	span := strings.Join(lines[nav.Start:nav.End], "\n")
	if !strings.Contains(span, "Reloads it.") {
		t.Error("NAVIGATION span does not include its subsection body")
	}
	if strings.Contains(span, "Reads storage.") {
		t.Error("NAVIGATION span runs into the following sibling section")
	}
}

func TestFilterSections_PhraseSelector(t *testing.T) {
	res := FilterSections(skillDoc, []string{"core actions"})

	if len(res.Blocks) != 1 {
		t.Fatalf("expected 1 match, got %v", res.Blocks)
	}
	if !strings.Contains(res.Content, "zdemo -a ping") {
		t.Error("filtered content lost the section body")
	}
	if strings.Contains(res.Content, "Reads storage.") {
		t.Error("filter leaked an unrelated section")
	}
	if res.Lines >= res.Total {
		t.Errorf("filter reported no savings: %d of %d lines", res.Lines, res.Total)
	}
}

func TestFilterSections_SubstringAndOutOfOrder(t *testing.T) {
	for _, selector := range []string{"navigation", "actions core", "CORE"} {
		res := FilterSections(skillDoc, []string{selector})
		if len(res.Blocks) == 0 {
			t.Errorf("selector %q matched nothing", selector)
		}
	}
}

func TestFilterSections_SubtreeFollowsParent(t *testing.T) {
	res := FilterSections(skillDoc, []string{"navigation"})
	for _, want := range []string{"Opens a page.", "Reloads it."} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("parent filter missing subsection content %q", want)
		}
	}
}

func TestFilterSections_PathAndOutlineNumber(t *testing.T) {
	res := FilterSections(skillDoc, []string{"navigation/refresh"})
	if len(res.Blocks) != 1 || !strings.Contains(res.Content, "Reloads it.") {
		t.Errorf("path selector failed: %+v", res.Blocks)
	}

	sections := ParseSections(skillDoc)
	dataIndex := 0
	for _, s := range sections {
		if s.Title == "DATA" {
			dataIndex = s.Index
		}
	}
	byNumber := FilterSections(skillDoc, []string{strconv.Itoa(dataIndex)})
	if len(byNumber.Blocks) != 1 || !strings.Contains(byNumber.Content, "Reads storage.") {
		t.Errorf("outline-number selector failed for index %d: %+v", dataIndex, byNumber.Blocks)
	}
}

func TestFilterSections_NoMatchReportsUnmatched(t *testing.T) {
	res := FilterSections(skillDoc, []string{"core actions", "teleportation"})
	if len(res.Blocks) == 0 {
		t.Error("valid selector in a mixed call was dropped")
	}
	if len(res.Unmatched) != 1 || res.Unmatched[0] != "teleportation" {
		t.Errorf("expected teleportation reported unmatched, got %v", res.Unmatched)
	}

	none := FilterSections(skillDoc, []string{"teleportation"})
	if len(none.Blocks) != 0 || none.Content != "" {
		t.Error("expected no matches and empty content for a hopeless selector")
	}

	if got := SuggestSections(ParseSections(skillDoc), []string{"data storage"}, 3); len(got) == 0 {
		t.Error("SuggestSections returned nothing for a near miss")
	}
}

func TestSectionOutline(t *testing.T) {
	outline := SectionOutline(ParseSections(skillDoc))
	if !strings.Contains(outline, "CORE ACTIONS") || !strings.Contains(outline, "navigate") {
		t.Errorf("outline missing headings:\n%s", outline)
	}
	// Every line is addressable by the range it reports, in <idx> <level> <title> <range> order.
	for _, want := range []string{"## 🧭 NAVIGATION", "### navigate"} {
		if !strings.Contains(outline, want) {
			t.Errorf("outline line missing %q:\n%s", want, outline)
		}
	}
	nav := outline
	for _, line := range strings.Split(nav, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		span := fields[len(fields)-1]
		if _, _, ok := parseLineRange(span); !ok {
			t.Errorf("outline line does not end in a start:end span: %q", line)
		}
	}
}

// The reported span is what a caller reads back with sed, so the separator rule that
// markdown puts before the *next* heading must not be inside it.
func TestSectionRangeExcludesSeparatorRule(t *testing.T) {
	lines := strings.Split(skillDoc, "\n")
	for _, s := range ParseSections(skillDoc) {
		if s.End <= s.Start {
			t.Fatalf("section %q has an empty span", s.Title)
		}
		last := strings.TrimSpace(lines[s.End-1])
		if last == "" || last == "---" {
			t.Errorf("section %q range ends on a filler line: %s", s.Title, s.Range())
		}
	}
}

func TestFilterSections_LineRangeSelector(t *testing.T) {
	var target Section
	for _, s := range ParseSections(skillDoc) {
		if strings.Contains(s.Title, "NAVIGATION") {
			target = s
		}
	}
	if target.Title == "" {
		t.Fatal("NAVIGATION section not found")
	}

	res := FilterSections(skillDoc, []string{target.Range()})
	if len(res.Blocks) != 1 || !strings.Contains(res.Content, "Reloads it.") {
		t.Errorf("range selector %s failed: %+v", target.Range(), res.Blocks)
	}
	if strings.Contains(res.Content, "Reads storage.") {
		t.Error("range selector ran past its span")
	}
}

func TestFilterSections_ReportsSpans(t *testing.T) {
	res := FilterSections(skillDoc, []string{"core actions", "data"})
	if len(res.Spans) != 2 {
		t.Fatalf("expected 2 rendered blocks, got %v", res.Spans)
	}
	if !strings.HasPrefix(res.Spans[0], "1") && !strings.Contains(res.Spans[0], ":") {
		t.Errorf("span %q is not a start:end pair", res.Spans[0])
	}
	lines := strings.Split(skillDoc, "\n")
	for _, span := range res.Spans {
		start, end, ok := parseLineRange(span)
		if !ok {
			t.Fatalf("unparsable span %q", span)
		}
		if start > end || end > len(lines) {
			t.Errorf("span %q is outside the document (%d lines)", span, len(lines))
		}
		if !strings.Contains(res.Content, strings.TrimSpace(lines[start-1])) {
			t.Errorf("span %q does not start inside the returned content", span)
		}
	}
}

// The document title names the whole file, not a section of it: selecting it by phrase
// would undo the narrowing the caller asked for.
func TestPhraseDoesNotSelectTheDocumentTitle(t *testing.T) {
	res := FilterSections(skillDoc, []string{"demo skill api"})
	if len(res.Blocks) != 0 {
		t.Errorf("the H1 document title should not be selectable by phrase, got %v", res.Blocks)
	}

	byIndex := FilterSections(skillDoc, []string{"1"})
	if len(byIndex.Blocks) == 0 {
		t.Error("the document title stays addressable by its outline number")
	}
}

// A heading whose own text contains "/" is reachable by its phrase, not only by path.
func TestSelectorWithSlashFallsBackToPhrase(t *testing.T) {
	doc := "## Toast/Status Overlays\n\nBody.\n\n## Other\n\nText.\n"
	res := FilterSections(doc, []string{"toast/status overlays"})
	if len(res.Blocks) != 1 || !strings.Contains(res.Content, "Body.") {
		t.Errorf("slash heading unreachable: %+v", res.Blocks)
	}
}

func TestHeadingTitleClosingSequence(t *testing.T) {
	doc := "## C#\n\nx\n\n## Rank # 3 #\n\ny\n\n## Goals ##\n\nz\n"
	sections := ParseSections(doc)
	want := []string{"C#", "Rank # 3", "Goals"}
	if len(sections) != len(want) {
		t.Fatalf("expected %d sections, got %d", len(want), len(sections))
	}
	for i, title := range want {
		if sections[i].Title != title {
			t.Errorf("section %d title = %q, want %q", i, sections[i].Title, title)
		}
	}
}

// Headings inside an HTML comment are commented out, and a rule of any width is not
// content, so neither may end up inside a section or its reported range.
func TestCommentedHeadingsAndWideRules(t *testing.T) {
	doc := "# A\n\nprose\n\n<!--\n## Hidden\n-->\n\n## B\n\nbody\n\n-----\n\n## C\n\nlast\n"
	sections := ParseSections(doc)
	lines := strings.Split(doc, "\n")
	for _, s := range sections {
		if strings.Contains(s.Title, "Hidden") {
			t.Error("a commented-out heading became a section")
		}
		if last := strings.TrimSpace(lines[s.End-1]); last == "-----" {
			t.Errorf("section %q range ends on a rule: %s", s.Title, s.Range())
		}
	}
}

func TestLineCountMatchesWc(t *testing.T) {
	if got := LineCount("a\nb\nc\n"); got != 3 {
		t.Errorf("trailing newline produced %d lines", got)
	}
	if got := LineCount("a\nb\nc"); got != 3 {
		t.Errorf("unterminated last line produced %d lines", got)
	}
}

func TestTopSectionTitles(t *testing.T) {
	titles := TopSectionTitles(ParseSections(skillDoc))
	// The lone "# Demo Skill API" title is not a table of contents; the ## level is.
	if len(titles) != 3 {
		t.Fatalf("expected 3 section titles, got %v", titles)
	}
}

func TestTopSectionTitlesOfHeadinglessDocument(t *testing.T) {
	if titles := TopSectionTitles(ParseSections("just prose\nno headings\n")); titles != nil {
		t.Errorf("expected no titles, got %v", titles)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
