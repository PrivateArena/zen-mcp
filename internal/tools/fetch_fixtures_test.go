package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Extraction-quality fixtures.
//
// The protocol contract of the fetch flow lives in fetch_test.go, where a stub bridge
// answers canned bodies: that can prove the call sequence, but not whether the payload
// is any good. The MDN regression — a browser-compatibility table missing entirely, two
// thirds of the payload page chrome — was invisible to every existing test, because the
// tool happily returned a truncated body dump. These fixtures assert the payload itself:
// must-include and must-exclude snippets plus a byte budget, which is the metric that
// tracks what the tool is for.
//
// They drive the real Firefox bridge (default 127.0.0.1:9999, see mcpcfg), so they are
// opt-in:
//
//	ZFETCH_LIVE=1 go test ./internal/tools -run TestFetchExtract -v

// zfetchLiveEnv gates the live extraction fixtures.
const zfetchLiveEnv = "ZFETCH_LIVE"

// extractionFixture is one page plus the assertions that define a good payload for it.
type extractionFixture struct {
	url string
	// title names the page in the failure message.
	title string
	// mustInclude are snippets only the real content contains.
	mustInclude []string
	// mustExclude are snippets only page chrome contains.
	mustExclude []string
	// maxBytes bounds the payload, so a chrome dump cannot come back as extra content.
	maxBytes int
}

// liveFetchDoc runs the real fetch flow and parses its response the way every other
// bridge content test does: provenance terms, then the body.
func liveFetchDoc(t *testing.T, args map[string]any) bridgeDoc {
	t.Helper()
	if os.Getenv(zfetchLiveEnv) == "" {
		t.Skipf("%s is not set; extraction fixtures need the live Firefox bridge", zfetchLiveEnv)
	}
	res := HandleFetchAction(context.Background(), t.TempDir(), Deps{}, fetchReq(args))
	if res != nil && res.IsError {
		t.Fatalf("fetch failed: %s", probeSnippet(resultText(t, res)))
	}
	return docOf(t, res)
}

func runExtractionFixture(t *testing.T, fixture extractionFixture, args map[string]any) bridgeDoc {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	args["url"] = fixture.url
	args["max_chars"] = fetchMaxChars

	doc := liveFetchDoc(t, args)
	// The extraction report belongs in the provenance line: a caller must be able to
	// tell an article from a whole-body dump without reading the body at all.
	t.Logf("%s: %d bytes, %d lines | %s", fixture.title, len(doc.body),
		strings.Count(doc.body, "\n")+1, resultTerms(t, doc))

	for _, want := range fixture.mustInclude {
		if !strings.Contains(doc.body, want) {
			t.Errorf("%s: payload is missing %q", fixture.title, want)
		}
	}
	for _, unwanted := range fixture.mustExclude {
		if strings.Contains(doc.body, unwanted) {
			t.Errorf("%s: payload carries chrome %q", fixture.title, unwanted)
		}
	}
	if fixture.maxBytes > 0 && len(doc.body) > fixture.maxBytes {
		t.Errorf("%s: payload is %d bytes, over the %d budget (chrome leaking back in?)",
			fixture.title, len(doc.body), fixture.maxBytes)
	}
	return doc
}

// TestFetchExtractsShadowDOMFixture pins the extraction rules against a checked-in page,
// served over loopback so the run needs neither the network nor a warm cache. Every
// marker is one of the shapes the extractor used to get wrong.
func TestFetchExtractsShadowDOMFixture(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Join("testdata"))))
	defer server.Close()

	fixture := extractionFixture{
		url:   server.URL + "/fetch_shadow_fixture.html",
		title: "local shadow fixture",
		mustInclude: []string{
			"PROSE_MARKER_ALPHA",
			// A table that exists only two shadow roots deep (the MDN shape).
			"SHADOW_TABLE_HEAD",
			"SHADOW_TABLE_CELL_42",
			"SHADOW_OUTER_TEXT",
			// Light DOM reaches the page through <slot>, and only there.
			"SLOT_LIGHT_TEXT_77",
			// Closed roots are readable from the chrome frame script.
			"CLOSED_SHADOW_TEXT_99",
			// An unhydrated placeholder has no shadow tree, so its light DOM is the content.
			"FALLBACK_TEXT_55",
			"LIGHT_TABLE_CELL_11",
			"DETAILS_SUMMARY_MARKER",
			"DETAILS_BODY_MARKER",
			"ORDERED_ONE",
			"ORDERED_THREE",
			// Relative links must reach the caller already resolved against the page.
			server.URL + "/other/page.html",
			"https://example.com/fixed",
		},
		mustExclude: []string{
			"NAV_JUNK_ONE",
			"FOOTER_JUNK_MARKER",
			// Duplicated content: a host's light children when its shadow tree already
			// won, and a slot's fallback when something was assigned to it.
			"UNASSIGNED_LIGHT_MUST_NOT_APPEAR",
			"SLOT_FALLBACK_MUST_NOT_APPEAR",
			// A relative href the caller cannot resolve, and a heading self-anchor that
			// answers nothing and costs a line per heading.
			"](../other/page.html)",
			"](#install)",
		},
		maxBytes: 8000,
	}
	runExtractionFixture(t, fixture, nil)
}

// TestFetchExtractsLivePages is the regression suite for the pages that motivated the
// extraction work.
func TestFetchExtractsLivePages(t *testing.T) {
	for _, fixture := range liveExtractionFixtures() {
		t.Run(fixture.title, func(t *testing.T) {
			runExtractionFixture(t, fixture, nil)
		})
	}
}

func liveExtractionFixtures() []extractionFixture {
	return []extractionFixture{
		{
			// The compat tables live in <mdn-compat-table-lazy> shadow roots; the sidebar
			// dumps and footer are what the payload used to be mostly made of.
			url:         "https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API",
			title:       "mdn_fetch_api_shadow_tables",
			mustInclude: []string{"Fetch API", "fetchLater", "Chrome", "Firefox", "Safari", "Edge"},
			mustExclude: []string{
				"Skip to main content",
				"HTML: Markup language",
				"Learn web development",
				"Report a problem with this content",
			},
			maxBytes: 60000,
		},
		{
			// A table-heavy reference page: wide tables must survive as markdown tables.
			url:         "https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Status",
			title:       "mdn_http_status_tables",
			mustInclude: []string{"404", "503"},
			mustExclude: []string{"Skip to main content", "Report a problem with this content"},
			maxBytes:    60000,
		},
		{
			// A plain blog page, where article detection already worked: the good path
			// must not regress into a body dump.
			url:         "https://overreacted.io/before-you-memo/",
			title:       "blog_article_body",
			mustInclude: []string{"memo"},
			mustExclude: []string{"404 Page not found"},
			maxBytes:    40000,
		},
	}
}

// resultTerms flattens the parsed provenance for the log line.
func resultTerms(t *testing.T, doc bridgeDoc) string {
	t.Helper()
	terms := make([]string, 0, len(doc.terms))
	for label, value := range doc.terms {
		terms = append(terms, label+"="+value)
	}
	return strings.Join(terms, " ")
}
