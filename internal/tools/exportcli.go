package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/toolresponse"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// ExportCLIOptions is the intent of one export_cli call. The generator itself
// lives in internal/terminal (it owns the wrapper templates and the config
// defaults), so this tool only states what to run and reports what came back.
type ExportCLIOptions struct {
	// Short forces short -a param aliases on or off. nil means "the caller did not
	// say", which lets the climode_short config value decide — the same tri-state
	// the terminal export-cli command implements.
	Short *bool
	// Clean removes the generated wrappers and their PATH links instead of writing
	// them.
	Clean bool
}

// verb names the run in the report and in the error text.
func (o ExportCLIOptions) verb() string {
	if o.Clean {
		return "clean"
	}
	return "export"
}

func defExportCLI(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "export_cli",
		Title:       "Export CLI Wrappers",
		Description: "Regenerate the z*/zen-* CLI wrapper scripts in cli/ (and their ~/.local/bin links) for every registered MCP tool — the terminal export-cli command. Run it after adding, renaming, or re-schema-ing a tool so the matching z* command exists.",
		Schema: jsonSchema(map[string]any{
			"short": boolProp("Emit short -a param aliases; omitted means the climode_short config value decides"),
			"clean": boolProp("Remove the generated wrappers and their ~/.local/bin links instead of exporting"),
		}, []string{}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleExportCLIAction(ctx, workspace, deps, req), nil
		},
	}
}

// HandleExportCLIAction runs the wrapper generator and returns its report
// verbatim. The report is a line or two of plain text, so it needs no markdown
// document of its own.
func HandleExportCLIAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()

	if deps.ExportCLI == nil {
		// Without the injected generator this server cannot write wrappers, and a
		// silent success would leave the caller believing the CLI is current.
		return toolresponse.WrapErrorWithContext(ctx, "export_cli", fmt.Errorf("cli export is not wired on this server; run the terminal export-cli command instead"), start)
	}

	short, err := exportCLIFlag("short", args["short"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "export_cli", err, start)
	}
	clean, err := exportCLIFlag("clean", args["clean"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "export_cli", err, start)
	}
	opts := ExportCLIOptions{Short: short, Clean: clean != nil && *clean}

	report, err := deps.ExportCLI(opts)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "export_cli", fmt.Errorf("cli %s failed: %v", opts.verb(), err), start)
	}
	text := strings.TrimSpace(report)
	if text == "" {
		// The generator always reports what it wrote or why it could not; silence
		// means the injection is broken, which must not look like a clean run.
		return toolresponse.WrapErrorWithContext(ctx, "export_cli", fmt.Errorf("cli %s produced no report", opts.verb()), start)
	}

	logfilter.Infof("[export_cli] %s: %s", opts.verb(), strings.ReplaceAll(text, "\n", " | "))
	return toolresponse.WrapSuccess(ctx, "export_cli", text+"\n", start)
}

// exportCLIFlag normalizes an optional boolean argument into a tri-state pointer:
// nil is "not supplied", so the config default still applies. An MCP client sends
// a real JSON boolean while a CLI wrapper can only ever send a string, and a JSON
// number degenerates to 0/1 — all three shapes are accepted, anything else is an
// explicit caller mistake.
func exportCLIFlag(name string, v any) (*bool, error) {
	if v == nil {
		return nil, nil
	}
	switch tv := v.(type) {
	case bool:
		return &tv, nil
	case string:
		s := strings.ToLower(strings.TrimSpace(tv))
		if s == "" {
			return nil, nil
		}
		if b, ok := exportCLIWordFlag(s); ok {
			return &b, nil
		}
		if b, err := strconv.ParseBool(s); err == nil {
			return &b, nil
		}
		return nil, fmt.Errorf("%s must be true or false, got %q", name, v)
	default:
		if n, ok := bridgeIntArg(v); ok && (n == 0 || n == 1) {
			b := n == 1
			return &b, nil
		}
		return nil, fmt.Errorf("%s must be true or false, got %v", name, v)
	}
}

// exportCLIWordFlag understands the words a human types where JSON's true/false is
// awkward. The second return is false for anything that is not a known word.
func exportCLIWordFlag(s string) (value, known bool) {
	switch s {
	case "yes", "y", "on":
		return true, true
	case "no", "n", "off":
		return false, true
	}
	return false, false
}
