package tools

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"zen-mcp/internal/analysis"
	"zen-mcp/internal/projectmemory"
	"zen-mcp/internal/toolresponse"
)

// defContext is a helper function
func defContext(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "context",
		Title:       "Context",
		Description: "Retrieve stored project memory context with optional file analysis.",
		Schema: jsonSchema(map[string]any{
			//"workspace": strProp("Project path (default: current session workspace)"),
			"query":     strProp("Retrieval ID"),
		}, []string{"query"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleContextAction(ctx, workspace, deps, req), nil
		},
	}
}

// HandleContextAction is a helper function
func HandleContextAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	inputWorkspace, _ := args["workspace"].(string)
	query, _ := args["query"].(string)

	actualWorkspace := resolveWorkspaceFromDeps(inputWorkspace, workspace)
	if actualWorkspace == "" {
		return toolresponse.WrapErrorWithContext(ctx, "context",
			errors.New("Workspace path is required but could not be determined. Please provide it explicitly or set a workspace root first."), start)
	}

	dbPath := filepath.Join(actualWorkspace, ".zenmcp", "context.db")
	return toolresponse.WrapSuccess(ctx, "context", actionRetrieveContext(dbPath, query), start)
}

// actionRetrieveContext ports actionRetrieveContext from project-memory.ts.
func actionRetrieveContext(dbPath, query string) map[string]any {
	content := projectmemory.RetrieveVirtualContext(dbPath, query, "")
	analysisResult := analysis.GetStoredAnalysis(dbPath, query)

	if analysisResult != nil {
		fileType := analysisResult.FileType
		ftLabel := fileType.Type
		if fileType.Subtype != "" {
			ftLabel = ftLabel + " (" + fileType.Subtype + ")"
		}
		reading := analysisResult.ReadingAdvice
		sample := analysisResult.Sample
		if len(sample) > 300 {
			sample = sample[:300]
		}
		return map[string]any{
			"content": content,
			"analysis": map[string]any{
				"file_type":      ftLabel,
				"confidence":     formatPercent(fileType.Confidence),
				"reading_tool":   reading.Tool,
				"reading_advice": reading.Explanation,
				"warning":        reading.Warning,
				"line_count":     analysisResult.LineCount,
				"byte_size":      analysisResult.ByteSize,
				"sample":         sample,
			},
		}
	}
	return map[string]any{"content": content}
}

// formatPercent is a helper function
func formatPercent(conf float64) string {
	pct := int(conf*100 + 0.5)
	return itoa(pct) + "%"
}

// itoa is a helper function
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
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
