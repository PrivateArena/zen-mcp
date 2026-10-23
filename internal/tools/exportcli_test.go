package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func exportReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "export_cli",
			Arguments: args,
		},
	}
}

// exportSpy is an injected generator stand-in. It records every option set it
// receives and can be told to fail, so the tool is testable without ever touching
// cli/ or ~/.local/bin.
type exportSpy struct {
	mu     sync.Mutex
	report string
	err    error
	calls  []ExportCLIOptions
}

func newExportSpy(report string, err error) *exportSpy {
	return &exportSpy{report: report, err: err}
}

func (s *exportSpy) run(opts ExportCLIOptions) (string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, opts)
	s.mu.Unlock()

	if s.err != nil {
		return "ERROR: half a report", s.err
	}
	return s.report, nil
}

func (s *exportSpy) last(t *testing.T) ExportCLIOptions {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.calls) == 0 {
		t.Fatal("generator was never called")
	}
	return s.calls[len(s.calls)-1]
}

func (s *exportSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func exportDeps(spy *exportSpy) Deps {
	deps := Deps{}
	if spy != nil {
		deps.ExportCLI = spy.run
	}
	return deps
}

func TestExportCLIReportsWhenGeneratorIsNotWired(t *testing.T) {
	// A server without the injected generator must say so instead of pretending the
	// CLI is now up to date.
	res := HandleExportCLIAction(context.Background(), "/ws", Deps{}, exportReq(nil))
	text := resultText(t, res)
	if !strings.Contains(text, "cli export is not wired") {
		t.Errorf("expected the unwired error, got %q", text)
	}
}

func TestExportCLIReturnsTheGeneratorReport(t *testing.T) {
	spy := newExportSpy("Exported 19 tools → cli\nSymlinked to /home/dev/.local/bin\n", nil)

	res := HandleExportCLIAction(context.Background(), "/ws", exportDeps(spy), exportReq(nil))

	text := resultText(t, res)
	for _, want := range []string{"Exported 19 tools", "Symlinked to"} {
		if !strings.Contains(text, want) {
			t.Errorf("report must reach the caller (%s), got %q", want, text)
		}
	}
	if spy.count() != 1 {
		t.Errorf("generator calls = %d, want 1", spy.count())
	}
}

func TestExportCLIDefaultsToConfigShortAndPlainExport(t *testing.T) {
	spy := newExportSpy("Exported 19 tools → cli\n", nil)

	HandleExportCLIAction(context.Background(), "/ws", exportDeps(spy), exportReq(nil))

	opts := spy.last(t)
	// nil (not false): the climode_short config decides, exactly like the terminal
	// command with no flag.
	if opts.Short != nil {
		t.Errorf("short = %v, want nil so the config default applies", *opts.Short)
	}
	if opts.Clean {
		t.Error("clean must default to false")
	}
}

func TestExportCLIShortFlagIsTriState(t *testing.T) {
	// The distinction that matters: an explicit false must NOT collapse to "absent",
	// or a caller could never force long aliases on a climode_short=true server.
	spy := newExportSpy("Exported 19 tools → cli\n", nil)

	HandleExportCLIAction(context.Background(), "/ws", exportDeps(spy), exportReq(map[string]any{"short": false}))

	opts := spy.last(t)
	if opts.Short == nil {
		t.Fatal("explicit short=false must reach the generator as a decision, not as nil")
	}
	if *opts.Short {
		t.Error("short = true, want false")
	}
}

func TestExportCLIFlagsAcceptEveryTransportShape(t *testing.T) {
	cases := []struct {
		name       string
		args       map[string]any
		wantShort  *bool
		wantClean  bool
		neverCalls bool
	}{
		{name: "boolean true", args: map[string]any{"short": true}, wantShort: boolPtrTest(true)},
		{name: "string true (CLI wrapper)", args: map[string]any{"short": "true"}, wantShort: boolPtrTest(true)},
		{name: "string mixed case", args: map[string]any{"short": "TrUe"}, wantShort: boolPtrTest(true)},
		{name: "string yes", args: map[string]any{"short": "yes"}, wantShort: boolPtrTest(true)},
		{name: "string on", args: map[string]any{"short": "on"}, wantShort: boolPtrTest(true)},
		{name: "string false", args: map[string]any{"short": "false"}, wantShort: boolPtrTest(false)},
		{name: "string no", args: map[string]any{"short": "no"}, wantShort: boolPtrTest(false)},
		{name: "string off", args: map[string]any{"short": "off"}, wantShort: boolPtrTest(false)},
		{name: "number 1", args: map[string]any{"short": float64(1)}, wantShort: boolPtrTest(true)},
		{name: "number 0", args: map[string]any{"short": float64(0)}, wantShort: boolPtrTest(false)},
		{name: "empty string is absent", args: map[string]any{"short": "   "}, wantShort: nil},
		{name: "clean string", args: map[string]any{"clean": "true"}, wantClean: true},
		{name: "clean boolean", args: map[string]any{"clean": true}, wantClean: true},
		{name: "garbage short rejected", args: map[string]any{"short": "maybe"}, neverCalls: true},
		{name: "object rejected", args: map[string]any{"clean": map[string]any{"a": "b"}}, neverCalls: true},
		{name: "out of range number rejected", args: map[string]any{"short": float64(7)}, neverCalls: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := newExportSpy("Exported 19 tools → cli\n", nil)
			res := HandleExportCLIAction(context.Background(), "/ws", exportDeps(spy), exportReq(tc.args))

			if tc.neverCalls {
				if spy.count() != 0 {
					t.Fatalf("args %v must be rejected before the generator; calls = %d", tc.args, spy.count())
				}
				if text := resultText(t, res); !strings.Contains(text, "must be true or false") {
					t.Errorf("args %v: expected a flag error, got %q", tc.args, text)
				}
				return
			}

			opts := spy.last(t)
			if tc.wantShort == nil {
				if opts.Short != nil {
					t.Errorf("short = %v, want the config default (nil)", *opts.Short)
				}
			} else if opts.Short == nil || *opts.Short != *tc.wantShort {
				t.Errorf("short = %v, want %v", opts.Short, *tc.wantShort)
			}
			if opts.Clean != tc.wantClean {
				t.Errorf("clean = %v, want %v", opts.Clean, tc.wantClean)
			}
		})
	}
}

func TestExportCLICleanReportsAsACleanRun(t *testing.T) {
	spy := newExportSpy("Cleaned 19 wrapper artifacts from cli/ and /home/dev/.local/bin\n", nil)

	res := HandleExportCLIAction(context.Background(), "/ws", exportDeps(spy), exportReq(map[string]any{"clean": true}))

	if opts := spy.last(t); !opts.Clean {
		t.Error("clean must reach the generator")
	}
	if text := resultText(t, res); !strings.Contains(text, "Cleaned") {
		t.Errorf("clean report = %q", text)
	}
}

func TestExportCLISurfacesGeneratorFailure(t *testing.T) {
	spy := newExportSpy("", fmt.Errorf("could not create ./cli: not a directory"))

	res := HandleExportCLIAction(context.Background(), "/ws", exportDeps(spy), exportReq(nil))

	text := resultText(t, res)
	if !strings.Contains(text, "cli export failed") || !strings.Contains(text, "could not create ./cli") {
		t.Errorf("expected the generator failure, got %q", text)
	}
}

func TestExportCLIEmptyReportIsAFailure(t *testing.T) {
	// The generator always reports something; an empty report means broken wiring,
	// and calling that a success would hide a stale CLI.
	spy := newExportSpy("   \n", nil)

	res := HandleExportCLIAction(context.Background(), "/ws", exportDeps(spy), exportReq(map[string]any{"clean": true}))

	if text := resultText(t, res); !strings.Contains(text, "produced no report") {
		t.Errorf("expected the empty-report error, got %q", text)
	}
}

func TestExportCLIOptionsVerbNamesTheRun(t *testing.T) {
	if got := (ExportCLIOptions{}).verb(); got != "export" {
		t.Errorf("verb() = %q, want export", got)
	}
	if got := (ExportCLIOptions{Clean: true}).verb(); got != "clean" {
		t.Errorf("clean verb() = %q, want clean", got)
	}
}

func TestExportCLIToolRegisteredInAllDefs(t *testing.T) {
	spy := newExportSpy("Exported 19 tools \u2192 cli\n", nil)

	defs := AllDefs("/ws", Deps{})
	var found *ToolDef
	for i := range defs {
		if defs[i].Name == "export_cli" {
			found = &defs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("export_cli tool not registered in AllDefs")
	}
	if found.Handler == nil {
		t.Fatal("export_cli tool has no handler")
	}
	if got, ok := found.Schema["required"].([]string); !ok || len(got) != 0 {
		t.Errorf("export_cli must require no parameter, got %v", found.Schema["required"])
	}
	props, _ := found.Schema["properties"].(map[string]any)
	for _, name := range []string{"short", "clean"} {
		if _, ok := props[name]; !ok {
			t.Errorf("export_cli schema missing %s property", name)
		}
	}

	// The registered def carries the injected generator, so the CLI path behaves like
	// the direct handler call.
	res, err := defExportCLI("/ws", exportDeps(spy)).Handler(context.Background(), exportReq(nil))
	if err != nil {
		t.Fatalf("handler returned go error: %v", err)
	}
	if text := resultText(t, res); !strings.Contains(text, "Exported 19 tools") {
		t.Errorf("registered handler report = %q", text)
	}

	// The static registry copy has no generator, which must be reported instead of a
	// panic or a fake success.
	res, err = found.Handler(context.Background(), exportReq(nil))
	if err != nil {
		t.Fatalf("handler returned go error: %v", err)
	}
	if text := resultText(t, res); !strings.Contains(text, "not wired") {
		t.Errorf("unwired registry copy must report itself, got %q", text)
	}
}

func boolPtrTest(b bool) *bool { return &b }
