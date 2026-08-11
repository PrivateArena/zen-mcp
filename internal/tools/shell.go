package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/shell/exec"
	"zen-mcp/internal/shell/tokenoptimizer"
	"zen-mcp/internal/toolresponse"
)

func defShell(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "shell",
		Title:       "Smart Shell Command (Token-Optimized)",
		Description: "Execute shell commands with token optimization (60-80% savings). ALWAYS prefer over raw command exec. Long-running jobs (> 60s by default) return {\"status\":\"running\",\"pool_id\":...} — poll with the pool tool (action=\"poll\").",
		Schema: jsonSchema(map[string]any{
			"command": strProp("Shell command"),
		}, []string{"command"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleShellAction(ctx, workspace, deps, req), nil
		},
	}
}

func HandleShellAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	command, _ := args["command"].(string)

	workspaceRoot := resolveWorkspaceFromDeps("", workspace)

	execDir := workspaceRoot
	if execDir == "" {
		cwd, _ := os.Getwd()
		execDir = cwd
	}

	if err := deps.Gatekeeper.ValidatePathSafety(execDir, "Shell Context"); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "shell", err, start)
	}
	if err := deps.Gatekeeper.ValidateCommandPayload(command, execDir); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "shell", err, start)
	}

	cfg := mcpcfg.Get()
	actTimeoutMs := cfg.Sandbox.ActivityTimeoutMs
	if actTimeoutMs <= 0 {
		actTimeoutMs = 30000
	}

	res := exec.Run(command, execDir, 0, actTimeoutMs)
	stdout := res.Stdout
	stderr := res.Stderr
	exitCode := res.ExitCode

	optimized := stdout
	savings := 0
	finalStderr := stderr
	profileApplied := false

	tokenOptEnabled := cfg.TokenOptimization.Enabled == nil || *cfg.TokenOptimization.Enabled
	skipOptimize := false
	ultraCompact := cfg.TokenOptimization.UltraCompact != nil && *cfg.TokenOptimization.UltraCompact

	if tokenOptEnabled {
		profileResult := tokenoptimizer.ApplyTokenProfiles(command, stdout, finalStderr,
			tokenoptimizer.Options{SkipOptimization: skipOptimize, UltraCompact: ultraCompact},
			tokenOptConfig(*cfg))
		if profileResult.Applied {
			optimized = profileResult.Stdout
			finalStderr = profileResult.Stderr
			profileApplied = true
		}
	}

	if !skipOptimize && tokenOptEnabled && stdout != "" && !profileApplied {
		optimized = tokenoptimizer.OptimizeOutput(command, stdout,
			tokenoptimizer.Options{UltraCompact: ultraCompact}, tokenOptConfig(*cfg))
		savings = tokenoptimizer.GetSavings(stdout, optimized)
	}

	if len(cfg.ShellOutputBlacklist) > 0 {
		if out := tokenoptimizer.ApplyBlacklist(command, optimized, toBlacklist(cfg.ShellOutputBlacklist)); out != nil {
			optimized = *out
		}
		if out := tokenoptimizer.ApplyBlacklist(command, finalStderr, toBlacklist(cfg.ShellOutputBlacklist)); out != nil {
			finalStderr = *out
		}
	}

	finalStdout := optimized

	isGit := strings.HasPrefix(strings.TrimSpace(command), "git")
	typ := "shell"
	if isGit {
		typ = "git"
	}
	logSessionEvent(workspace, typ, command, stdout+stderr)

	savingsVal := any(nil)
	if savings > 0 {
		savingsVal = itoaStr(savings) + "%"
	}

	return toolresponse.WrapSuccess(ctx, "shell", map[string]any{
		"command":         command,
		"stdout":          finalStdout,
		"stderr":          finalStderr,
		"exitCode":        exitCode,
		"aborted":         res.Aborted,
		"timedOut":        nullableString(res.TimedOut),
		"actTimeoutMs":    actTimeoutMs,
		"timeout":         nil,
		"savings":         savingsVal,
		"originalLength":  len(stdout),
		"optimizedLength": len(optimized),
		"filtered":        nil,
	}, start)
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func itoaStr(n int) string {
	return itoa(n)
}

func tokenOptConfig(cfg mcpcfg.ZenConfig) tokenoptimizer.Config {
	t := cfg.TokenOptimization
	return tokenoptimizer.Config{
		Enabled:              t.Enabled == nil || *t.Enabled,
		UltraCompact:         t.UltraCompact != nil && *t.UltraCompact,
		MaxChainedLength:     derefInt2(t.MaxChainedLength, 51200),
		DeduplicateThreshold: derefInt2(t.DeduplicateThreshold, 3),
		ProfilesPath:         profilePath(t.ProfilesPath),
		Blacklist:            toBlacklist(cfg.ShellOutputBlacklist),
	}
}

func profilePath(p string) string {
	if p == "" {
		p = "token-profiles.json"
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(mcpcfg.ProjectRoot, p)
}

func derefInt2(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func toBlacklist(entries []mcpcfg.BlacklistEntry) []tokenoptimizer.BlacklistEntry {
	out := make([]tokenoptimizer.BlacklistEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, tokenoptimizer.BlacklistEntry{
			Match:      e.Match,
			IsRegex:    e.IsRegex,
			MaxLines:   e.MaxLines,
			DropOutput: e.DropOutput,
			Label:      e.Label,
		})
	}
	return out
}
