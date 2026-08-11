package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/projectmemory"
	"zen-mcp/internal/shell/exec"
	"zen-mcp/internal/toolresponse"
)

func defRun(deps Deps) ToolDef {
	cfg := mcpcfg.Get()
	langs := sortedKeys2(cfg.Sandbox.Languages)

	return ToolDef{
		Name:  "run",
		Title: "Code Sandbox",
		Description: "Run code snippets in isolated temp files. Auto-cleanup. Activity-based timeout.",
		Schema: jsonSchema(map[string]any{
			"language":     strEnumProp("Language: "+strings.Join(langs, ", "), langs),
			"code":         strProp("Code snippet (no escaping)"),
			"stdin":        strProp("Data to pipe to stdin"),
			"timeout":      numProp("Idle-kill ms (default " + itoa(cfg.Sandbox.ActivityTimeoutMs) + ", ceiling " + itoa(cfg.Sandbox.TimeoutMs) + ")"),
			"useWorkspace": boolProp("Run in workspace root"),
			"logToMemory":  boolProp("Log to project-memory FTS"),
			"tail":         numProp("Last N lines"),
			"head":         numProp("First N lines"),
			"range": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"start": numProp("Start line (1-indexed)"),
					"end":   numProp("End line (1-indexed)"),
				},
				"description": "Line range (1-indexed)",
			},
		}, []string{"language", "code"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleRunAction(ctx, deps, req), nil
		},
	}
}

func HandleRunAction(ctx context.Context, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	cfg := mcpcfg.Get()
	langs := sortedKeys2(cfg.Sandbox.Languages)

	args := req.GetArguments()
	language, _ := args["language"].(string)
	code, _ := args["code"].(string)
	stdin, _ := args["stdin"].(string)
	useWorkspace := toBool(args["useWorkspace"])
	logToMemory := toBool(args["logToMemory"])
	tail := toInt(args["tail"])
	head := toInt(args["head"])
	var rng *struct{ Start, End int }
	if rv, ok := args["range"].(map[string]any); ok {
		rng = &struct{ Start, End int }{
			Start: toInt(rv["start"]),
			End:   toInt(rv["end"]),
		}
	}

	lang, ok := cfg.Sandbox.Languages[language]
	if !ok {
		return toolresponse.WrapErrorWithContext(ctx, "run",
			&runErr{msg: `Unknown language "` + language + `". Available: ` + strings.Join(langs, ", ")}, start)
	}

	var execDir string
	if useWorkspace || logToMemory {
		execDir, _ = deps.Store.Get("workspace-root")
		if execDir == "" {
			return toolresponse.WrapErrorWithContext(ctx, "run",
				&runErr{msg: "No active workspace root found. Ensure you are connected/initialized first."}, start)
		}
	}

	checkDir := execDir
	if checkDir == "" {
		cwd, _ := os.Getwd()
		checkDir = cwd
	}
	if err := deps.Gatekeeper.ValidatePathSafety(checkDir, "run Context"); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "run", err, start)
	}
	if err := deps.Gatekeeper.ValidateCommandPayload(code, checkDir); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "run", err, start)
	}

	activityMs := cfg.Sandbox.ActivityTimeoutMs
	if v := toInt(args["timeout"]); v > 0 {
		activityMs = v
	}
	hardMs := cfg.Sandbox.TimeoutMs

	tmpFile := filepath.Join(os.TempDir(), "zen-run-"+randHex()+"."+strings.TrimPrefix(lang.Ext, "."))
	if err := os.WriteFile(tmpFile, []byte(code), 0o644); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "run",
			&runErr{msg: "Failed to write temp file: " + err.Error()}, start)
	}
	defer os.Remove(tmpFile)

	runnerArgs := append(append([]string{}, lang.Args...), tmpFile)
	res := exec.RunSandbox(lang.Runner, runnerArgs, execDir, stdin, activityMs, hardMs)

	stdout := res.Stdout
	if rng != nil || head > 0 || tail > 0 {
		lines := strings.Split(stdout, "\n")
		if rng != nil {
			startLine := rng.Start
			if startLine < 1 {
				startLine = 1
			}
			endLine := rng.End
			if endLine > len(lines) {
				endLine = len(lines)
			}
			if startLine > endLine {
				startLine = endLine
			}
			stdout = strings.Join(lines[startLine-1:endLine], "\n")
		} else if head > 0 {
			n := head
			if n < 1 {
				n = 1
			}
			if len(lines) > n {
				lines = lines[:n]
			}
			stdout = strings.Join(lines, "\n")
		} else if tail > 0 {
			n := tail
			if n < 1 {
				n = 1
			}
			if len(lines) > n {
				lines = lines[len(lines)-n:]
			}
			stdout = strings.Join(lines, "\n")
		}
	}

	result := map[string]any{
		"command":      "run " + language,
		"stdout":       stdout,
		"stderr":       res.Stderr,
		"exitCode":     res.ExitCode,
		"timedOut":     nullableString(res.TimedOut),
		"actTimeoutMs": activityMs,
		"timeout":      hardMs,
	}

	if logToMemory {
		logText := toolresponse.RenderOutput("raw", result)
		if ws := execDir; ws != "" {
			dbPath := filepath.Join(ws, ".zenmcp", "context.db")
			projectmemory.LogProjectEvent(dbPath, "shell", "run: ran "+language+" script",
				"### Code\n"+code+"\n\n### Output\n"+logText, nil)
		}
	}

	return toolresponse.WrapSuccess(ctx, "run", result, start)
}

func sortedKeys2(m map[string]mcpcfg.SandboxLanguage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func randHex() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

type runErr struct{ msg string }

func (e *runErr) Error() string { return e.msg }
