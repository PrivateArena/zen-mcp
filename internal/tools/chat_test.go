package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"zen-mcp/internal/mcpcfg"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func chatReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "chat",
			Arguments: args,
		},
	}
}

// chatAnswer is the text one sub-chat extracts. It carries markdown, newlines and
// quotes so a JSON re-wrap of the reply is impossible to miss.
const chatAnswer = "Gemini says: **two** things matter.\n\n\t- token efficiency\n\t- \"correctness\""

func chatSingleResponse(provider, reply string) string {
	return fmt.Sprintf(`{"success":true,"provider":%q,"response":%q}`, provider, reply)
}

var chatOKResponses = map[string]string{"chat": chatSingleResponse("gemini", chatAnswer)}

// chatRun drives the tool against the default stub answer.
func chatRun(t *testing.T, args map[string]any) (bridgeDoc, *bridgeStub) {
	t.Helper()
	doc, stub, _ := chatRunWith(t, t.TempDir(), args, nil, nil)
	return doc, stub
}

// chatRunWith drives the tool with a canned bridge answer and returns the parsed
// document, the recorded calls and the raw result.
func chatRunWith(t *testing.T, workspace string, args map[string]any, responses map[string]string, statuses map[string]int) (bridgeDoc, *bridgeStub, *mcp.CallToolResult) {
	t.Helper()
	stub := chatStubAt(t, responses, statuses)
	res := HandleChatAction(context.Background(), workspace, Deps{}, chatReq(args))
	return docOf(t, res), stub, res
}

// chatCall drives the tool where a failure is expected, so the answer is read as text
// instead of parsed as a document.
func chatCall(t *testing.T, args map[string]any, responses map[string]string, statuses map[string]int) (*bridgeStub, string) {
	t.Helper()
	stub := chatStubAt(t, responses, statuses)
	res := HandleChatAction(context.Background(), t.TempDir(), Deps{}, chatReq(args))
	return stub, resultText(t, res)
}

func chatStubAt(t *testing.T, responses map[string]string, statuses map[string]int) *bridgeStub {
	t.Helper()
	if responses == nil {
		responses = chatOKResponses
	}
	if statuses == nil {
		return bridgeStubAt(t, responses)
	}
	return bridgeStubStatusAt(t, responses, statuses)
}

func TestChatRunsOneBridgeCallAndReturnsTheReply(t *testing.T) {
	doc, stub := chatRun(t, map[string]any{"provider": "gemini", "message": "What is a closure?"})

	// The whole point of the tool: one bridge "chat". Chat already owns the flow
	// (navigate, upload, submit, poll, extract), so a wrapper that added its own
	// navigate/read pair would drive a second page.
	if got, want := strings.Join(stub.actions(), ","), "chat"; got != want {
		t.Fatalf("bridge call sequence = %q, want %q", got, want)
	}

	call := stub.byAction("chat")
	if call["provider"] != "gemini" {
		t.Errorf("provider = %v", call["provider"])
	}
	if call["message"] != "What is a closure?" {
		t.Errorf("message = %v", call["message"])
	}
	// The bridge turns `timeout` into its polling budget, so the call must carry a
	// minutes-scale budget rather than a request timeout.
	ms, ok := call["timeout"].(float64)
	if !ok || ms < 60000 {
		t.Errorf("timeout = %v, want a minutes-scale polling budget", call["timeout"])
	}
	if want := float64(chatBudgetMs()); ms != want {
		t.Errorf("timeout = %v, want the configured budget %v", ms, want)
	}

	if doc.terms["provider"] != "gemini" {
		t.Errorf("provider term = %q", doc.terms["provider"])
	}
	wantVerbatim(t, doc.body, chatAnswer)
}

func TestChatSendsOnlyWhatWasAskedFor(t *testing.T) {
	_, stub := chatRun(t, map[string]any{"provider": "gemini", "message": "hello"})

	call := stub.byAction("chat")
	// Every unset optional must stay off the wire: the bridge picks the container and
	// the tab itself, and a null would override that with "no value".
	for _, absent := range []string{"path", "screenshot", "container", "index", "download_dir", "upload_files", "take_screenshot"} {
		if _, present := call[absent]; present {
			t.Errorf("%s must be omitted when not requested, got %v", absent, call[absent])
		}
	}
	// One question is not a batch: the bridge's batch path closes its tabs and shares
	// one screenshot across the fan-out, so the single-value shapes must stay strings.
	if _, isList := call["message"].([]any); isList {
		t.Errorf("single message must reach the bridge as a string, got %#v", call["message"])
	}
}

func TestChatFansOutOverProviderArray(t *testing.T) {
	responses := map[string]string{"chat": `{"success":true,"responses":[` +
		`{"success":true,"provider":"gemini","response":"from gemini"},` +
		`{"success":true,"provider":"claude","response":"from claude"}]}`}

	doc, stub, _ := chatRunWith(t, t.TempDir(), map[string]any{
		"provider": []any{"gemini", "claude"},
		"message":  "Explain closures in JS",
	}, responses, nil)

	call := stub.byAction("chat")
	if got, want := call["provider"], []any{"gemini", "claude"}; !reflect.DeepEqual(got, want) {
		t.Errorf("provider = %#v, want %#v", got, want)
	}
	// A single question against two providers is still a fan-out: the message stays a
	// string so the bridge can rotate it over the provider list.
	if call["message"] != "Explain closures in JS" {
		t.Errorf("message = %#v, want the prompt string", call["message"])
	}

	// Each answer is named by its own section, so the line above the replies must not
	// restate the provider list as if one provider had answered everything.
	if doc.terms["provider"] != "" {
		t.Errorf("provider term = %q, want it left to the sections", doc.terms["provider"])
	}
	if doc.terms["queries"] != "2" {
		t.Errorf("queries term = %q, want 2 (the caller must see how many answers ran)", doc.terms["queries"])
	}
	for _, want := range []string{"## gemini: Explain closures in JS", "from gemini", "## claude: Explain closures in JS", "from claude"} {
		if !strings.Contains(doc.body, want) {
			t.Errorf("body missing %q: %q", want, probeSnippet(doc.body))
		}
	}
}

func TestChatPairsMessageArrayRoundRobin(t *testing.T) {
	responses := map[string]string{"chat": `{"success":true,"responses":[` +
		`{"success":true,"provider":"gemini","response":"answer one"},` +
		`{"success":true,"provider":"gemini","response":"answer two"}]}`}

	doc, stub, _ := chatRunWith(t, t.TempDir(), map[string]any{
		"provider": "gemini",
		"message":  []any{"what is A?", "what is B?"},
	}, responses, nil)

	call := stub.byAction("chat")
	if got, want := call["message"], []any{"what is A?", "what is B?"}; !reflect.DeepEqual(got, want) {
		t.Errorf("message = %#v, want %#v", got, want)
	}
	// The bridge reports no question back, so the section titles are the only thing
	// that can say which answer belongs to which question.
	for _, want := range []string{"## gemini: what is A?", "answer one", "## gemini: what is B?", "answer two"} {
		if !strings.Contains(doc.body, want) {
			t.Errorf("body missing %q: %q", want, probeSnippet(doc.body))
		}
	}
	if doc.terms["queries"] != "2" {
		t.Errorf("queries term = %q, want 2", doc.terms["queries"])
	}
}

func TestChatNormalizesTypedScreenshot(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  any
	}{
		{"bool", true, true},
		{"true string", "true", true},
		// The CLI sends every flag as a string and the bridge truth-tests the value,
		// so "false" means "yes" unless it is normalized away.
		{"false string", "false", nil},
		{"bool false", false, nil},
		{"viewport", "viewport", true},
		{"full page", "full", "full"},
		{"selector", ".thumbnail", ".thumbnail"},
		{"object", map[string]any{"byRole": "button"}, map[string]any{"byRole": "button"}},
		{"object string", `{"byRole":"button","full_page":true}`, map[string]any{"byRole": "button", "full_page": true}},
		{"empty object", map[string]any{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stub := chatRun(t, map[string]any{"provider": "gemini", "message": "analyze", "screenshot": tc.value})
			call := stub.byAction("chat")
			if !reflect.DeepEqual(call["screenshot"], tc.want) {
				t.Errorf("screenshot = %#v, want %#v", call["screenshot"], tc.want)
			}
			if _, leaked := call["take_screenshot"]; leaked {
				t.Errorf("take_screenshot must not reach the bridge: %#v", call)
			}
		})
	}
}

func TestChatScreenshotAliasFollowsTheCanonicalKey(t *testing.T) {
	cases := []struct {
		name    string
		args    map[string]any
		want    any
		present bool
	}{
		{"alias alone", map[string]any{"take_screenshot": true}, true, true},
		// A blank canonical value is "not supplied", so the alias still speaks.
		{"blank canonical", map[string]any{"screenshot": "", "take_screenshot": true}, true, true},
		{"selector wins over alias", map[string]any{"screenshot": ".thumb", "take_screenshot": true}, ".thumb", true},
		// An explicit false is the caller's answer, not a missing value: it vetoes.
		{"false canonical", map[string]any{"screenshot": false, "take_screenshot": true}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"provider": "gemini", "message": "hello"}
			for k, v := range tc.args {
				args[k] = v
			}
			_, stub, _ := chatRunWith(t, t.TempDir(), args, nil, nil)
			got, present := stub.byAction("chat")["screenshot"]
			if present != tc.present {
				t.Fatalf("screenshot present = %v, want %v (%#v)", present, tc.present, got)
			}
			if tc.present && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("screenshot = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestChatRejectsABadScreenshotType(t *testing.T) {
	stub, text := chatCall(t, map[string]any{"provider": "gemini", "message": "hello", "screenshot": float64(2)}, nil, nil)
	if !strings.Contains(text, "screenshot must be") {
		t.Errorf("error = %q", probeSnippet(text))
	}
	if stub.byAction("chat") != nil {
		t.Error("a bad screenshot must not reach the bridge")
	}
}

func TestChatRejectsAMalformedObjectScreenshot(t *testing.T) {
	stub, text := chatCall(t, map[string]any{"provider": "gemini", "message": "hello", "screenshot": "{byRole:button}"}, nil, nil)
	if !strings.Contains(text, "must be a screenshot object in JSON") {
		t.Errorf("error = %q", probeSnippet(text))
	}
	if stub.byAction("chat") != nil {
		t.Error("a malformed screenshot object must not reach the bridge")
	}
}

func TestChatUploadFilesBecomeAbsolutePathsWithoutAnAliasLeak(t *testing.T) {
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(nested, "spec.md")

	responses := map[string]string{"chat": `{"success":true,"provider":"gemini","response":"read it"}`}
	doc, stub, _ := chatRunWith(t, workspace, map[string]any{
		"provider":     "gemini",
		"message":      "review this",
		"upload_files": []any{"PROJECT_OVERVIEW.md", abs},
	}, responses, nil)

	call := stub.byAction("chat")
	// The bridge reads files as `path`, and a relative path would resolve against the
	// browser process, not the project.
	paths, ok := call["path"].([]any)
	if !ok || len(paths) != 2 {
		t.Fatalf("path = %#v, want the two resolved paths", call["path"])
	}
	if paths[0] != filepath.Join(workspace, "PROJECT_OVERVIEW.md") {
		t.Errorf("relative path not resolved against the workspace: %v", paths[0])
	}
	if paths[1] != abs {
		t.Errorf("absolute path rewritten: %v", paths[1])
	}
	if _, leaked := call["upload_files"]; leaked {
		t.Errorf("upload_files must be rewritten to path, got %#v", call)
	}
	// The caller sees which files the provider actually received.
	if doc.terms["attachment"] != strings.Join([]string{filepath.Join(workspace, "PROJECT_OVERVIEW.md"), abs}, ", ") {
		t.Errorf("attachment term = %q", doc.terms["attachment"])
	}
}

func TestChatPathAliasIsAcceptedAndBlanksAreNotValues(t *testing.T) {
	workspace := t.TempDir()
	resolved := filepath.Join(workspace, "SPEC.md")
	responses := map[string]string{"chat": `{"success":true,"provider":"gemini","response":"read"}`}

	_, stub, _ := chatRunWith(t, workspace, map[string]any{
		"provider": "gemini", "message": "review", "path": "SPEC.md",
	}, responses, nil)
	if got := stub.byAction("chat")["path"]; got != resolved {
		t.Errorf("path alias = %v, want the workspace-relative path resolved to %v", got, resolved)
	}

	_, stub, _ = chatRunWith(t, workspace, map[string]any{
		"provider": "gemini", "message": "review", "upload_files": "OTHER.md", "path": "SPEC.md",
	}, responses, nil)
	if got := stub.byAction("chat")["path"]; got != resolved {
		t.Errorf("path = %v, want the explicit bridge key to win over the alias", got)
	}

	// A present-but-blank upload_files is "not supplied": forwarding it would attach the
	// workspace directory, and it must not silence the alias either.
	_, stub, _ = chatRunWith(t, workspace, map[string]any{
		"provider": "gemini", "message": "review", "upload_files": "  ", "path": "SPEC.md",
	}, responses, nil)
	if got := stub.byAction("chat")["path"]; got != resolved {
		t.Errorf("blank upload_files silenced the path alias: %v", got)
	}
	for _, blank := range []any{"", []any{"  ", ""}, []any{}} {
		_, stub, _ = chatRunWith(t, workspace, map[string]any{
			"provider": "gemini", "message": "review", "upload_files": blank,
		}, responses, nil)
		if _, present := stub.byAction("chat")["path"]; present {
			t.Errorf("blank upload_files %#v must not be sent as an attachment", blank)
		}
	}
}

func TestChatBlankScreenshotDoesNotClaimAConflict(t *testing.T) {
	// An empty screenshot carries no attachment, so the mutual-exclusion guard must not
	// stop the call and must not drop the upload.
	_, stub, _ := chatRunWith(t, t.TempDir(), map[string]any{
		"provider": "gemini", "message": "hello", "screenshot": "", "upload_files": "spec.md",
	}, nil, nil)
	call := stub.byAction("chat")
	if call == nil {
		t.Fatal("a blank screenshot must not block the call")
	}
	if _, present := call["screenshot"]; present {
		t.Errorf("blank screenshot must be omitted, got %v", call["screenshot"])
	}
	if _, present := call["path"]; !present {
		t.Errorf("the upload must survive a blank screenshot, got %#v", call)
	}
}

func TestChatReportsTheScreenshotItAttached(t *testing.T) {
	responses := map[string]string{"chat": `{"success":true,"provider":"gemini","response":"layout noted","screenshot":"/tmp/mcp_screenshot_chat.png"}`}
	doc, _, _ := chatRunWith(t, t.TempDir(), map[string]any{
		"provider": "gemini", "message": "audit this page", "screenshot": "full",
	}, responses, nil)

	if doc.terms["attachment"] != "/tmp/mcp_screenshot_chat.png" {
		t.Errorf("attachment term = %q, want the path the bridge echoed", doc.terms["attachment"])
	}
}

func TestChatJoinsTheAttachmentListTheBridgeEchoes(t *testing.T) {
	// The bridge echoes the uploadPath it used, which for a multi-file upload is an
	// array: the term must join it, not print Go's slice formatting.
	responses := map[string]string{"chat": `{"success":true,"provider":"gemini","response":"done","screenshot":["/ws/a.md","/ws/b.md"]}`}
	doc, _, _ := chatRunWith(t, t.TempDir(), map[string]any{"provider": "gemini", "message": "review"}, responses, nil)

	if got, want := doc.terms["attachment"], "/ws/a.md, /ws/b.md"; got != want {
		t.Errorf("attachment = %q, want %q", got, want)
	}
}

func TestChatReportsTheDownloadedArtifacts(t *testing.T) {
	responses := map[string]string{"chat": `{"success":true,"provider":"gemini","response":"done",` +
		`"downloads":{"succeeded":true,"files":["/out/a.py","/out/b.py"]}}`}
	doc, _, _ := chatRunWith(t, t.TempDir(), map[string]any{
		"provider": "chatgpt", "message": "write code", "download_dir": "out",
	}, responses, nil)

	if got, want := doc.terms["downloaded"], "/out/a.py, /out/b.py"; got != want {
		t.Errorf("downloaded = %q, want %q", got, want)
	}
}

func TestChatReportsAnEmptyDownloadHarvest(t *testing.T) {
	// A download_dir that saved nothing must be visible: otherwise the caller assumes
	// the artifact landed somewhere it never did.
	responses := map[string]string{"chat": `{"success":true,"provider":"gemini","response":"done","downloads":{"succeeded":false,"files":[]}}`}
	doc, _, _ := chatRunWith(t, t.TempDir(), map[string]any{
		"provider": "chatgpt", "message": "write code", "download_dir": "out",
	}, responses, nil)

	if got, want := doc.terms["downloaded"], "nothing"; got != want {
		t.Errorf("downloaded = %q, want %q", got, want)
	}
}

func TestChatPassesContainerAndTabHandle(t *testing.T) {
	_, stub := chatRun(t, map[string]any{
		"provider":  "gemini",
		"message":   "hello",
		"container": "  Work  ",
		"index":     "t_stub_1",
	})

	call := stub.byAction("chat")
	if call["container"] != "Work" {
		t.Errorf("container = %v, want the trimmed name", call["container"])
	}
	// An explicit tab is what lets two chats to one provider run at once: the bridge
	// keys its in-progress lock on provider+tab.
	if call["index"] != "t_stub_1" {
		t.Errorf("index = %v, want the tab handle", call["index"])
	}
}

func TestChatForwardsANumericTabIndex(t *testing.T) {
	_, stub := chatRun(t, map[string]any{"provider": "gemini", "message": "hello", "index": float64(3)})

	if got := stub.byAction("chat")["index"]; got != float64(3) {
		t.Errorf("index = %#v, want the positional tab index forwarded", got)
	}
}

func TestChatResolvesDownloadDir(t *testing.T) {
	_, stub := chatRun(t, map[string]any{
		"provider":     "chatgpt",
		"message":      "write a python script",
		"download_dir": "artifacts",
	})

	call := stub.byAction("chat")
	dir, ok := call["download_dir"].(string)
	if !ok || !filepath.IsAbs(dir) {
		t.Fatalf("download_dir = %#v, want an absolute directory", call["download_dir"])
	}
}

func TestChatRejectsContradictoryAttachments(t *testing.T) {
	stub, text := chatCall(t, map[string]any{
		"provider":     "gemini",
		"message":      "compare",
		"upload_files": "a.md",
		"screenshot":   true,
	}, nil, nil)
	if !strings.Contains(text, "cannot be used together") {
		t.Errorf("expected the conflict error, got %q", probeSnippet(text))
	}
	if stub.byAction("chat") != nil {
		t.Error("a contradictory call must not reach the bridge")
	}
}

func TestChatRequiresProviderAndMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"no provider", map[string]any{"message": "hello"}, "provider is required"},
		{"no message", map[string]any{"provider": "gemini"}, "message is required"},
		{"blank message", map[string]any{"provider": "gemini", "message": "   "}, "at least one non-empty value"},
		{"nested message", map[string]any{"provider": "gemini", "message": []any{map[string]any{"q": 1}}}, "plain text"},
		{"negative budget", map[string]any{"provider": "gemini", "message": "hi", "timeout": -5}, "positive number of milliseconds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub, text := chatCall(t, tc.args, nil, nil)
			if !strings.Contains(text, tc.want) {
				t.Errorf("error = %q, want it to mention %q", probeSnippet(text), tc.want)
			}
			if stub.byAction("chat") != nil {
				t.Error("an invalid call must not reach the bridge")
			}
		})
	}
}

func TestChatReportsBridgeRejection(t *testing.T) {
	// 429 is the bridge's "Chat with gemini already in progress": the reason lives in
	// the error body, and dumping the whole HTTP envelope would bury it.
	stub, text := chatCall(t, map[string]any{"provider": "gemini", "message": "hello"},
		map[string]string{"chat": `{"success":false,"error":"Chat with gemini already in progress"}`},
		map[string]int{"chat": http.StatusTooManyRequests})

	for _, want := range []string{"chat with gemini failed", "already in progress"} {
		if !strings.Contains(text, want) {
			t.Errorf("error = %q, want it to carry %q", probeSnippet(text), want)
		}
	}
	if strings.Contains(text, "429") || strings.Contains(text, "Bridge failure") {
		t.Errorf("error leaks the HTTP envelope instead of the reason: %q", probeSnippet(text))
	}
	if stub.byAction("chat") == nil {
		t.Fatal("the call must have been attempted")
	}
}

func TestChatUnknownProvider(t *testing.T) {
	_, text := chatCall(t, map[string]any{"provider": "geminii", "message": "hello"},
		map[string]string{"chat": `{"success":false,"error":"Unknown provider: geminii"}`},
		map[string]int{"chat": http.StatusBadRequest})
	if !strings.Contains(text, "Unknown provider: geminii") {
		t.Errorf("error = %q", probeSnippet(text))
	}
}

func TestChatReportsAFailedChatOnHTTP200(t *testing.T) {
	// A bridge build that answers success:false with HTTP 200 must not be read as an
	// empty answer, still less as a success.
	_, text := chatCall(t, map[string]any{"provider": "gemini", "message": "hello"},
		map[string]string{"chat": `{"success":false,"error":"SUBMISSION_LOCK acquire timed out"}`}, nil)
	for _, want := range []string{"chat with gemini failed", "SUBMISSION_LOCK acquire timed out"} {
		if !strings.Contains(text, want) {
			t.Errorf("error = %q, want it to carry %q", probeSnippet(text), want)
		}
	}
}

func TestChatKeepsSuccessfulRepliesOfFailedBatch(t *testing.T) {
	// The bridge answers a half-failed batch with HTTP 500, so a naive error path throws
	// away the answers that did arrive next to the one that did not.
	body := `{"success":false,"responses":[` +
		`{"success":true,"provider":"gemini","response":"the gemini answer"},` +
		`{"success":false,"provider":"claude","error":"Session expired"}]}`
	doc, _, res := chatRunWith(t, t.TempDir(), map[string]any{
		"provider": []any{"gemini", "claude"},
		"message":  "one question",
	}, map[string]string{"chat": body}, map[string]int{"chat": http.StatusInternalServerError})

	if res.IsError {
		t.Fatalf("a batch with an answer inside must not be reported as a bare failure: %q", probeSnippet(resultText(t, res)))
	}
	for _, want := range []string{"the gemini answer", "claude", "Session expired"} {
		if !strings.Contains(doc.body, want) {
			t.Errorf("body missing %q: %q", want, probeSnippet(doc.body))
		}
	}
	// A half-empty comparison must not read exactly like a complete one.
	if !strings.Contains(doc.terms["incomplete"], "1 of 2 failed") || !strings.Contains(doc.terms["incomplete"], "Session expired") {
		t.Errorf("incomplete term = %q, want the missing provider and its reason", doc.terms["incomplete"])
	}
}

func TestChatKeepsEveryReasonOfAFullyFailedBatch(t *testing.T) {
	body := `{"success":false,"responses":[` +
		`{"success":false,"provider":"gemini","error":"Polling timed out and no response was captured by the poller"},` +
		`{"success":false,"provider":"claude","error":"Session expired - the provider redirected to its login page"}]}`
	_, text := chatCall(t, map[string]any{
		"provider": []any{"gemini", "claude"},
		"message":  "one question",
	}, map[string]string{"chat": body}, map[string]int{"chat": http.StatusInternalServerError})

	// Each reason is what the caller can act on, and a batch report carries no
	// top-level error to fall back on: losing the second one would hide the login
	// problem behind the first.
	for _, want := range []string{"gemini: Polling timed out", "claude: Session expired"} {
		if !strings.Contains(text, want) {
			t.Errorf("error = %q, want it to carry %q", probeSnippet(text), want)
		}
	}
}

func TestChatEmptyAnswerIsAnError(t *testing.T) {
	// A successful round-trip with no extracted text is the expensive failure: the
	// prompt was submitted and nothing came back.
	_, text := chatCall(t, map[string]any{"provider": "gemini", "message": "hello"},
		map[string]string{"chat": `{"success":true,"provider":"gemini","response":""}`}, nil)
	if !strings.Contains(text, "captured an empty answer") {
		t.Errorf("error = %q", probeSnippet(text))
	}
}

func TestChatUnreadableReportIsAnError(t *testing.T) {
	_, text := chatCall(t, map[string]any{"provider": "gemini", "message": "hello"},
		map[string]string{"chat": `{"success":true,"provider":"gemini"}`}, nil)
	if !strings.Contains(text, "no answer field") {
		t.Errorf("error = %q", probeSnippet(text))
	}
}

func TestChatSpillsALongAnswerToAFile(t *testing.T) {
	dir := t.TempDir()
	withChatFileOutput(t, dir, 1)

	long := strings.Repeat("lorem ipsum dolor sit amet ", 400) // ~11 KB, past the 1 KB threshold
	doc, stub, _ := chatRunWith(t, t.TempDir(), map[string]any{"provider": "gemini", "message": "write an essay"},
		map[string]string{"chat": chatSingleResponse("gemini", long)}, nil)
	if stub.byAction("chat") == nil {
		t.Fatal("chat never reached the bridge")
	}

	// The caller must still be able to act: the pointer replaces the text, and the
	// provenance line survives the spill.
	if doc.terms["provider"] != "gemini" {
		t.Errorf("provenance lost across the spill: %v", doc.terms)
	}
	if !strings.HasPrefix(doc.body, "Response saved to `") {
		t.Errorf("body = %q, want the pointer to the spilled file", probeSnippet(doc.body))
	}

	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one spilled answer file, got %v (%v)", files, err)
	}
	if !strings.HasPrefix(files[0].Name(), "chat-") {
		t.Errorf("spilled file named %q, want the chat action prefix", files[0].Name())
	}
	saved, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	// The reply is trimmed on the way in, so the file holds the answer exactly as the
	// caller's document does.
	if string(saved) != strings.TrimSpace(long) {
		t.Errorf("spilled file holds %d bytes, want the full %d byte answer", len(saved), len(strings.TrimSpace(long)))
	}
}

// withChatFileOutput points the chat spill target at a temp dir with a low threshold,
// so the bound is testable without a 200 KB fixture.
func withChatFileOutput(t *testing.T, dir string, thresholdKb int) {
	t.Helper()
	orig := mcpcfg.Get()
	cfg := *orig
	out := dir
	cfg.ChatFileThresholdKb = thresholdKb
	cfg.ChatOutputPath = &out
	mcpcfg.Config.Store(&cfg)
	t.Cleanup(func() { mcpcfg.Config.Store(orig) })
}

func TestChatHonoursACallerBudget(t *testing.T) {
	_, stub := chatRun(t, map[string]any{"provider": "gemini", "message": "hello", "timeout": float64(30000)})

	if got := stub.byAction("chat")["timeout"]; got != float64(30000) {
		t.Errorf("timeout = %v, want the requested budget untouched", got)
	}
}

func TestChatBudgetComesFromItsOwnToolConfig(t *testing.T) {
	orig := mcpcfg.Get()
	cfg := *orig
	t.Cleanup(func() { mcpcfg.Config.Store(orig) })

	// Only an explicit toolConfigs.chat entry may drive the polling budget: the
	// built-in browser default is 120 s, which truncates an answer instead of failing
	// fast, and GetToolConfig would hand that out as if it were configured.
	cfg.ToolConfigs = map[string]json.RawMessage{
		"browser": json.RawMessage(`{"timeout":120000}`),
		"chat":    json.RawMessage(`{"timeout":600000}`),
	}
	mcpcfg.Config.Store(&cfg)
	if got := chatBudgetMs(); got != 600000 {
		t.Errorf("chatBudgetMs = %d, want the chat tool's own budget", got)
	}

	cfg.ToolConfigs = map[string]json.RawMessage{"browser": json.RawMessage(`{"timeout":120000}`)}
	mcpcfg.Config.Store(&cfg)
	if got := chatBudgetMs(); got != chatDefaultPollingMs {
		t.Errorf("chatBudgetMs = %d, want the %d ms floor, never the 120 s browser default", got, chatDefaultPollingMs)
	}

	// The legacy timeout map is a configured budget as well.
	cfg.ToolConfigs = nil
	cfg.ToolTimeouts = map[string]int{"chat": 300000}
	mcpcfg.Config.Store(&cfg)
	if got := chatBudgetMs(); got != 300000 {
		t.Errorf("chatBudgetMs = %d, want the legacy toolTimeouts budget", got)
	}
}

func TestChatDefinitionIsChatOnly(t *testing.T) {
	def := defChat("/ws", Deps{})
	if def.Name != "chat" {
		t.Fatalf("tool name = %q", def.Name)
	}
	// The lite tool has no action axis: chat is the only thing it does, so a caller
	// cannot mis-spell it and the CLI wrapper gets one flag set.
	if _, hasAction := def.Schema["properties"].(map[string]any)["action"]; hasAction {
		t.Error("the chat tool must not expose an action parameter")
	}
	for _, required := range []string{"provider", "message"} {
		if !stringIn(def.Schema["required"].([]string), required) {
			t.Errorf("%s must be required: the bridge rejects the call without it", required)
		}
	}
	if !def.LongRunning {
		t.Error("a chat is minutes of page work, so the tool must be advertised as long-running")
	}
	if !stringIn(allDefNames(), "chat") {
		t.Error("chat is not registered in AllDefs, so neither the MCP server nor a zchat wrapper exists")
	}
}

func allDefNames() []string {
	defs := AllDefs("/ws", Deps{})
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}

func stringIn(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
