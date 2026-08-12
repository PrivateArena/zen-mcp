package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"zen-mcp/internal/shared"
	"zen-mcp/internal/toolregistry"
	"zen-mcp/internal/toolresponse"
	"zen-mcp/internal/tools"
)

func callResultText(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var msg struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode call response: %v (body: %s)", err, rec.Body.String())
	}
	if msg.Error != nil {
		t.Fatalf("JSON-RPC error: %s", msg.Error.Message)
	}
	if len(msg.Result.Content) == 0 {
		t.Fatalf("call response has no content: %s", rec.Body.String())
	}
	return msg.Result.Content[0].Text
}

// TestPoolEndToEndHTTP proves the full production path: a wrapped tool call
// over HTTP returns a {status,pool_id} payload; the pool tool, called in a
// SEPARATE HTTP request, polls that SAME pool_id and finally replays the stored
// result verbatim. It guards the core contract: exactly one pool_id exists for
// a job and polling never mints a second one.
func TestPoolEndToEndHTTP(t *testing.T) {
	withPoolingWireConfig(t, `{
  "pooling": {"enabled": true, "tools": ["slow-tool"], "elapsedMs": 60, "ttlMinutes": 5, "maxJobs": 8},
  "tool_suggestions_enabled": false,
  "gatekeeper_enabled": false
}`)

	deps := RouteDeps{
		CreateMCPServer: func(id string) *mcpserver.MCPServer {
			s := mcpserver.NewMCPServer("zen-tools", "2.4.1",
				mcpserver.WithToolCapabilities(true),
				mcpserver.WithResourceCapabilities(false, false),
				mcpserver.WithPromptCapabilities(false),
			)
			slow := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				time.Sleep(150 * time.Millisecond)
				return toolresponse.WrapSuccess(ctx, "slow-tool", map[string]any{"result": "e2e-done"}, time.Now()), nil
			}
			// Mimic RegisterAllTools: wrap once, inject ToolContext, then AddTool.
			wrapped := wrapIfPooled("slow-tool", slow)
			s.AddTool(mcp.NewTool("slow-tool"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				ctx = toolresponse.WithToolContext(ctx, toolresponse.ToolContext{ToolName: "slow-tool", Params: req.GetArguments()})
				return wrapped(ctx, req)
			})
			// Register the real pool tool (from the tools package, so its
			// Handler uses the same process-wide pooling.Global() registry).
			for _, def := range tools.AllDefs("", tools.Deps{}) {
				if def.Name != "pool" {
					continue
				}
				s.AddTool(mcp.NewTool(def.Name), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					ctx = toolresponse.WithToolContext(ctx, toolresponse.ToolContext{ToolName: "pool", Params: req.GetArguments()})
					return def.Handler(ctx, req)
				})
				break
			}
			return s
		},
		Registry:              toolregistry.Create(),
		Shared:                shared.NewStore(),
		PendingCollaborations: tools.NewCollaborationRegistry(),
		StartTime:             time.Now(),
		Tag:                   "e2e",
	}
	mux := http.NewServeMux()
	SetupRoutes(mux, deps)

	doReq := func(body []byte) *httptest.ResponseRecorder {
		return doRequest(mux, http.MethodPost, "/mcp", body, "application/json")
	}

	initRec := doReq(mcpInitializeRequest(1))
	if initRec.Code != http.StatusOK {
		t.Fatalf("initialize failed: %d", initRec.Code)
	}

	// 1. Call the slow tool over HTTP; it exceeds 60ms and returns running.
	callRec := doReq([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"slow-tool","arguments":{}}}`))
	firstText := callResultText(t, callRec)
	var first map[string]any
	if err := json.Unmarshal([]byte(firstText), &first); err != nil {
		t.Fatalf("first call not JSON payload: %q", firstText)
	}
	if first["status"] != "running" {
		t.Fatalf("first call status = %v, want running (text: %s)", first["status"], firstText)
	}
	id, _ := first["pool_id"].(string)
	if id == "" {
		t.Fatalf("first call missing pool_id: %s", firstText)
	}

	// 2. Immediately poll via the pool tool (separate HTTP request). The job is
	//    still running, so the poll must return running with the SAME pool_id.
	pollRec := doReq([]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"pool","arguments":{"action":"poll","pool_id":"` + id + `"}}}`))
	pollText := callResultText(t, pollRec)
	var poll map[string]any
	if err := json.Unmarshal([]byte(pollText), &poll); err != nil {
		t.Fatalf("poll not JSON payload: %q", pollText)
	}
	if poll["status"] != "running" {
		t.Fatalf("interim poll status = %v, want running (text: %s)", poll["status"], pollText)
	}
	if poll["pool_id"] != id {
		t.Fatalf("poll returned pool_id %v, want the SAME %q — double pool_id bug", poll["pool_id"], id)
	}

	// Across both running payloads there must be exactly ONE distinct pool id.
	re := regexp.MustCompile(`[a-z-]+_[0-9]+`)
	ids := map[string]bool{}
	ids[re.FindString(firstText)] = true
	ids[re.FindString(pollText)] = true
	if len(ids) != 1 || !ids[id] {
		t.Fatalf("running payloads must reference one distinct pool id (=%q), saw %v", id, ids)
	}

	// 3. Wait for the job to finish, then poll again: it must replay the stored
	//    result verbatim, with no pool_id anywhere in it.
	time.Sleep(300 * time.Millisecond)
	doneRec := doReq([]byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"pool","arguments":{"action":"poll","pool_id":"` + id + `"}}}`))
	doneText := callResultText(t, doneRec)
	if !strings.Contains(doneText, "e2e-done") {
		t.Fatalf("final poll should replay the stored result, got: %s", doneText)
	}
	if strings.Contains(doneText, "pool-") {
		t.Errorf("completed result must not carry any pool_id, got: %s", doneText)
	}
}
