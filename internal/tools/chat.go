package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolresponse"
	wspkg "zen-mcp/internal/workspace"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// Settings for the chat tool: the lite surface of the bridge's `chat` action, which
// browser.go exposes as one action among dozens. Chat is the flagship bridge flow -
// Navigate → Capture → Upload → Prompt → Poll → Extract - run inside the provider's
// own web UI, so it needs no API key and carries the logged-in session.
const (
	// chatDefaultPollingMs is the answer budget sent as `timeout` when
	// toolConfigs.chat does not set one. The bridge turns it into
	// maxPolls = timeout / pollInterval, so a short budget is a truncated answer
	// rather than a fast failure - which is why the floor is minutes, not the
	// request-shaped default the other tools get.
	chatDefaultPollingMs = 2400000
	// chatMaxInlineChars bounds a reply that could not be spilled to a file, so a
	// write failure can never dump an unbounded answer into the context.
	chatMaxInlineChars = 50000
	// chatDefaultFileThresholdKb is the built-in spill threshold used when the config
	// leaves chat_file_threshold_kb unset (0).
	chatDefaultFileThresholdKb = 200
	// chatMaxQuestionChars bounds the question quoted in a batch section heading, so
	// one long prompt cannot bloat the document the way a provenance term could.
	chatMaxQuestionChars = 80
)

func defChat(workspace string, deps Deps) ToolDef {
	return ToolDef{
		Name:        "chat",
		Title:       "AI Chat (Firefox Bridge)",
		Description: "Ask a web AI provider through its own logged-in browser session via the Firefox bridge: the prompt is typed, the answer polled and extracted, and the whole reply returned as markdown. provider accepts one name or an array, and provider/message are paired round-robin, so a single call can query several providers in parallel. No API key: it uses the browser session.",
		LongRunning: true,
		Schema: jsonSchema(map[string]any{
			"provider": arrayStringProp("[chat] AI provider(s): \"gemini\", \"claude\", \"chatgpt\", \"duckduckgo\", \"perplexity\", \"grok\", \"nvidia\", \"github\", …; one name, or a comma-separated list to query them in parallel"),
			// Untyped on purpose: a prompt is free text, and the CLI comma-splits every
			// array-typed param, which would silently turn "Compare A, B" into two
			// questions. One prompt per call from the shell; arrays go in with --json.
			"message":         map[string]any{"description": "[chat] Prompt text; an array of prompts (send it with --json) is queried in parallel and paired with provider round-robin when the two lists differ in length"},
			"upload_files":    arrayStringProp("[chat] Local file path(s) to attach (the provider receives them as uploads); relative paths resolve against the workspace"),
			"screenshot":      map[string]any{"description": "[chat] Attach a screenshot instead of a file: true/\"viewport\" (active tab), \"full\" (whole page), \".selector\" (element), or a screenshot object ({\"byRole\":\"button\"}, also accepted as a JSON string). Omit it entirely for a text-only question"},
			"container":       strProp("[chat] Firefox container name or id that selects the identity for the tab chat opens (default: the provider's own)"),
			"index":           strProp("[chat] TabRegistry id (\"t_…\") or tab index to run the chat on; omit to let chat open its own tab. An id also gives the call its own in-progress lock key instead of the per-provider one"),
			"download_dir":    strProp("[chat] Directory to save the artifacts the provider offers for download (chatgpt/gemini code files)"),
			"timeout":         intProp("[chat] Polling budget for the answer in ms; a slow provider is cut off at this budget, not failed faster"),
			"path":            arrayStringProp("[chat] The bridge's own attachment key; an explicit path wins over upload_files"),
			"take_screenshot": boolProp("[chat] Alias of screenshot=true"),
		}, []string{"provider", "message"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleChatAction(ctx, workspace, deps, req), nil
		},
	}
}

// HandleChatAction runs one bridge chat and returns the reply under a provenance
// line. It talks to the bridge directly rather than through the browser tool: the
// browser path stringifies `message` (fmt.Sprint), which silently turns an array of
// prompts into one "[q1 q2]" question, and the transport layer already decodes
// entities in every message shape.
func HandleChatAction(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()

	providers, err := chatListParam("provider", args["provider"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "chat", err, start)
	}
	messages, err := chatListParam("message", args["message"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "chat", err, start)
	}
	screenshot, err := chatScreenshotParam(args["screenshot"], args["take_screenshot"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "chat", err, start)
	}
	attachments, err := chatAttachmentsParam(args["upload_files"], args["path"], workspace)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "chat", err, start)
	}
	// The bridge takes exactly one attachment source: a successful screenshot
	// capture overwrites `path`, so accepting both would silently drop the files the
	// caller meant the provider to read.
	if screenshot != nil && attachments != nil {
		return toolresponse.WrapErrorWithContext(ctx, "chat", fmt.Errorf("screenshot and upload_files cannot be used together: pick the page you are on or the files on disk"), start)
	}
	timeoutMs, err := chatTimeoutParam(args["timeout"])
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "chat", err, start)
	}

	params := map[string]any{
		"provider": chatWireValue(providers),
		"message":  chatWireValue(messages),
	}
	if screenshot != nil {
		params["screenshot"] = screenshot
	}
	if attachments != nil {
		// `path` is the bridge's key; upload_files is the agent-facing name.
		params["path"] = attachments
	}
	if container := chatContainerParam(args["container"]); container != "" {
		params["container"] = container
	}
	if index := chatIndexParam(args["index"]); index != nil {
		params["index"] = index
	}
	if dir := chatDownloadDirParam(args["download_dir"], workspace); dir != "" {
		params["download_dir"] = dir
	}
	if timeoutMs > 0 {
		params["timeout"] = timeoutMs
	} else {
		params["timeout"] = chatBudgetMs()
	}

	res, err := bridge.CallBridge(ctx, "chat", params)
	if err != nil {
		// A multi-provider batch that answered on some providers and failed on others
		// comes back as HTTP 500 with the successful replies inside: those answers are
		// worth more than the error alone. The gap is still reported, because the
		// caller asked for a comparison it did not fully get.
		if body, incomplete, ok := chatBatchPartial(err, messages); ok {
			doc := chatDocument(workspace, deps, body, providers, messages, params, nil, true, incomplete)
			logSessionEvent(workspace, "chat", fmt.Sprintf("chat: %s (%s)", chatLabel(providers), incomplete), body)
			return toolresponse.WrapSuccess(ctx, "chat", doc, start)
		}
		return toolresponse.WrapErrorWithContext(ctx, "chat", chatCallError(providers, err), start)
	}
	if detail, failed := chatFailure(res); failed {
		return toolresponse.WrapErrorWithContext(ctx, "chat", fmt.Errorf("chat with %s failed: %s", chatLabel(providers), detail), start)
	}

	body, batch, err := chatBody(res, providers, messages)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "chat", err, start)
	}
	doc := chatDocument(workspace, deps, body, providers, messages, params, res, batch, "")
	logSessionEvent(workspace, "chat", fmt.Sprintf("chat: %s", chatLabel(providers)), body)
	logfilter.Debugf("[chat] %s -> %d bytes", chatLabel(providers), len(body))
	return toolresponse.WrapSuccess(ctx, "chat", doc, start)
}

// chatListParam normalizes a string-or-array chat argument. An MCP client sends a
// real array; the CLI wrapper sends a string, splitting comma lists for the
// array-typed params (provider, upload_files). Empty entries are dropped: a trailing
// comma from a shell is not a question.
func chatListParam(name string, v any) ([]string, error) {
	if v == nil {
		return nil, fmt.Errorf("%s is required", name)
	}
	var items []any
	switch typed := v.(type) {
	case string:
		items = []any{typed}
	case []any:
		items = typed
	case []string:
		items = make([]any, 0, len(typed))
		for _, s := range typed {
			items = append(items, s)
		}
	default:
		return nil, fmt.Errorf("%s must be a string or an array of strings, got %v", name, v)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch item.(type) {
		case map[string]any, []any:
			return nil, fmt.Errorf("%s must be plain text, got the nested value %v", name, item)
		}
		if text := strings.TrimSpace(scalarHeaderValue(item)); text != "" {
			out = append(out, text)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s must carry at least one non-empty value", name)
	}
	return out, nil
}

// chatWireValue renders a normalized list the way the bridge parses it: one string
// stays a string, several values stay an array. Keeping the single-value form is what
// avoids the bridge's batch path - which fans out, closes its tabs and captures one
// shared screenshot - for the common one-question call.
func chatWireValue(list []string) any {
	if len(list) == 1 {
		return list[0]
	}
	out := make([]any, 0, len(list))
	for _, s := range list {
		out = append(out, s)
	}
	return out
}

// chatScreenshotParam normalizes the typed screenshot argument. This is the one chat
// param whose type matters: every CLI flag arrives as a string, and the bridge
// truth-tests the value, so "false" would otherwise mean "take a screenshot". A
// JSON object arrives as a string from the shell and is parsed back, because the
// bridge reads a bare string as a CSS selector.
//
// An absent or blank screenshot leaves room for the take_screenshot alias; an
// explicit false is the caller's answer and vetoes it.
func chatScreenshotParam(v any, alias any) (any, error) {
	if chatBlank(v) {
		v = alias
	}
	switch typed := v.(type) {
	case nil:
		return nil, nil
	case bool:
		if !typed {
			return nil, nil
		}
		return true, nil
	case map[string]any:
		if len(typed) == 0 {
			return nil, nil
		}
		return typed, nil
	case string:
		text := strings.TrimSpace(typed)
		switch strings.ToLower(text) {
		case "":
			return nil, nil
		case "true", "1", "yes", "on", "viewport":
			return true, nil
		case "false", "0", "no", "off", "none":
			return nil, nil
		}
		if strings.HasPrefix(text, "{") {
			var obj map[string]any
			if err := json.Unmarshal([]byte(text), &obj); err != nil {
				return nil, fmt.Errorf("screenshot must be a screenshot object in JSON, got unparseable %q", typed)
			}
			if len(obj) == 0 {
				return nil, nil
			}
			return obj, nil
		}
		return text, nil
	default:
		return nil, fmt.Errorf("screenshot must be true, \"full\", a CSS selector or a screenshot object, got %v", v)
	}
}

// chatAttachmentsParam normalizes upload_files (agent-facing) and path (the bridge's
// name, kept so a caller migrating from the browser tool is not rejected), and
// resolves relative paths against the workspace the way the browser tool does. An
// explicit `path` beats the alias, as in the browser tool, and a blank value on either
// side is "not supplied": forwarding "" would hand the bridge the workspace root as an
// attachment.
func chatAttachmentsParam(v any, path any, workspace string) (any, error) {
	if !chatBlank(path) {
		v = path
	} else if chatBlank(v) {
		return nil, nil
	}
	switch v.(type) {
	case string, []any, []string:
	default:
		return nil, fmt.Errorf("upload_files (or its path alias) must be a path or an array of paths, got %v", v)
	}
	resolved := resolveUploadFiles(v, workspace)
	if chatBlank(resolved) {
		return nil, fmt.Errorf("upload_files (or its path alias) must carry at least one non-empty file path")
	}
	return resolved, nil
}

// chatBlank reports whether a chat argument carries no value at all: absent, an
// empty-or-whitespace string, or a list with nothing but blanks in it.
func chatBlank(v any) bool {
	switch typed := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		for _, item := range typed {
			if !chatBlank(item) {
				return false
			}
		}
		return true
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func chatContainerParam(v any) string {
	return strings.TrimSpace(searchStringArg(v))
}

// chatIndexParam keeps the caller's tab handle. A TabRegistry id addresses that tab
// for the whole flow and keys the bridge's in-progress lock on provider+tab instead
// of provider alone; a batch ignores it, since it opens one tab per sub-chat.
func chatIndexParam(v any) any {
	switch typed := v.(type) {
	case nil:
		return nil
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return text
		}
		return nil
	default:
		return typed
	}
}

// chatDownloadDirParam resolves the artifact directory against the workspace so a
// relative download_dir lands in the project, not in the browser's cwd.
func chatDownloadDirParam(v any, workspace string) string {
	dir := strings.TrimSpace(searchStringArg(v))
	if dir == "" {
		return ""
	}
	return resolveUploadPath(dir, workspace)
}

// chatTimeoutParam normalizes the caller's polling budget.
func chatTimeoutParam(v any) (int, error) {
	if v == nil {
		return 0, nil
	}
	ms, ok := bridgeIntArg(v)
	if !ok || ms <= 0 {
		return 0, fmt.Errorf("timeout must be a positive number of milliseconds, got %v", v)
	}
	return ms, nil
}

// chatBudgetMs is the answer budget this tool hands the bridge. toolConfigs.chat owns
// it, and only an explicit entry counts: GetToolConfig substitutes the global request
// timeout, and the built-in browser default is 120 s - a budget that would silently
// truncate every long answer instead of failing fast.
func chatBudgetMs() int {
	if ms := configuredToolTimeout("chat"); ms > 0 {
		return ms
	}
	return chatDefaultPollingMs
}

// configuredToolTimeout returns a tool's explicitly configured timeout, or 0 when it
// has no timeout configured at all (neither toolConfigs nor the legacy toolTimeouts).
func configuredToolTimeout(name string) int {
	cfg := mcpcfg.Get()
	if cfg == nil {
		return 0
	}
	raw, ok := cfg.ToolConfigs[name]
	if !ok {
		if ms, legacy := cfg.ToolTimeouts[name]; legacy {
			return ms
		}
		return 0
	}
	var tc mcpcfg.ToolConfig
	if err := json.Unmarshal(raw, &tc); err == nil && tc.Timeout > 0 {
		return tc.Timeout
	}
	var ms int
	if err := json.Unmarshal(raw, &ms); err == nil {
		return ms
	}
	return 0
}

// chatBody extracts the reply text of a chat report and reports whether it rendered a
// fan-out. A single chat answers {success, provider, response}; a batch answers
// {success, responses:[...]} whose items carry the same fields, so both shapes render
// as markdown, one naming the provider per answer.
func chatBody(res map[string]any, providers, messages []string) (string, bool, error) {
	if items, ok := res["responses"].([]any); ok {
		if len(items) == 0 {
			return "", false, fmt.Errorf("chat returned no responses")
		}
		return chatBatchBody(items, messages), true, nil
	}
	if reply := strings.TrimSpace(searchStringArg(res["response"])); reply != "" {
		return reply, false, nil
	}
	// A chat that answered successfully with no text is the bridge's worst failure:
	// the prompt was submitted and spent a session, but nothing was read back.
	if _, hasReply := res["response"]; hasReply {
		return "", false, fmt.Errorf("chat with %s captured an empty answer: the provider did not render a reply, or its page changed", chatLabel(providers))
	}
	return "", false, fmt.Errorf("chat with %s returned no answer field: %s", chatLabel(providers), requestReason(chatPlainText(res)))
}

// chatBatchBody renders one section per sub-chat. A failed item keeps its own error
// instead of erasing the answers around it, because a batch where one provider is
// down still produced the comparison the caller asked for.
func chatBatchBody(items []any, messages []string) string {
	var b strings.Builder
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			fmt.Fprintf(&b, "## %s\n\n%s\n\n", chatSectionTitle(i, len(items), messages, ""), requestReason(fmt.Sprint(raw)))
			continue
		}
		provider := strings.TrimSpace(searchStringArg(item["provider"]))
		fmt.Fprintf(&b, "## %s\n\n", chatSectionTitle(i, len(items), messages, provider))
		if reply := strings.TrimSpace(searchStringArg(item["response"])); reply != "" {
			b.WriteString(reply + "\n\n")
		} else {
			detail := strings.TrimSpace(searchStringArg(item["error"]))
			if detail == "" {
				detail = "no answer captured"
			}
			fmt.Fprintf(&b, "failed: %s\n\n", detail)
		}
		if files := chatDownloadsTerm(item["downloads"]); files != "" {
			fmt.Fprintf(&b, "downloaded: %s\n\n", files)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// chatSectionTitle names a batch section. One answer needs no heading beyond the
// provider; several do, and the bridge reports no question of its own - so the
// question this call sent for that slot is quoted back, using the same wrap-around
// pairing the bridge applies.
func chatSectionTitle(i, total int, messages []string, provider string) string {
	title := provider
	if title == "" {
		title = fmt.Sprintf("query %d", i+1)
	}
	if total < 2 {
		return title
	}
	if len(messages) == 0 {
		return fmt.Sprintf("%s (%d/%d)", title, i+1, total)
	}
	question := messages[i%len(messages)]
	if len(question) > chatMaxQuestionChars {
		cut := chatMaxQuestionChars
		for cut > 0 && !utf8.RuneStart(question[cut]) {
			cut--
		}
		question = question[:cut] + "…"
	}
	return fmt.Sprintf("%s: %s", title, strings.Join(strings.Fields(question), " "))
}

// chatBatchPartial recovers the successful half of a failed batch and reports how much
// of it is missing. The bridge marks a batch with any failing sub-chat HTTP 500, and
// CallBridge surfaces that as an error whose body is the whole report - successful
// replies included.
func chatBatchPartial(err error, messages []string) (string, string, bool) {
	items, ok := chatBatchItems(err)
	if !ok {
		return "", "", false
	}
	replies, failures := 0, make([]string, 0, len(items))
	for i, raw := range items {
		item, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		if strings.TrimSpace(searchStringArg(item["response"])) != "" {
			replies++
			continue
		}
		failures = append(failures, chatItemFailure(i, item))
	}
	if replies == 0 {
		return "", "", false
	}
	return chatBatchBody(items, messages), fmt.Sprintf("%d of %d failed: %s", len(failures), len(items), strings.Join(failures, "; ")), true
}

// chatBatchItems decodes the batch report out of a bridge failure, if the error
// carries one.
func chatBatchItems(err error) ([]any, bool) {
	var fail *bridge.ErrBridgeFailure
	if !errors.As(err, &fail) {
		return nil, false
	}
	var report struct {
		Responses []any `json:"responses"`
	}
	if json.Unmarshal([]byte(fail.Body), &report) != nil || len(report.Responses) == 0 {
		return nil, false
	}
	return report.Responses, true
}

// chatItemFailure names the provider and reason behind one failed sub-chat, so a
// partial batch says what is missing instead of just how much.
func chatItemFailure(i int, item map[string]any) string {
	provider := strings.TrimSpace(searchStringArg(item["provider"]))
	if provider == "" {
		provider = fmt.Sprintf("query %d", i+1)
	}
	detail := strings.TrimSpace(searchStringArg(item["error"]))
	if detail == "" {
		detail = "no answer captured"
	}
	return fmt.Sprintf("%s: %s", provider, requestReason(detail))
}

// chatFailure reports the failure of a chat that still answered HTTP 200. The bridge
// normally fails with HTTP >= 400, so this only catches a stale bridge build.
func chatFailure(res map[string]any) (string, bool) {
	if success, ok := res["success"].(bool); !ok || success {
		return "", false
	}
	if detail := strings.TrimSpace(searchStringArg(res["error"])); detail != "" {
		return detail, true
	}
	if items, ok := res["responses"].([]any); ok && len(items) > 0 {
		failures := make([]string, 0, len(items))
		for i, raw := range items {
			if item, isMap := raw.(map[string]any); isMap {
				failures = append(failures, chatItemFailure(i, item))
			}
		}
		if len(failures) > 0 {
			return strings.Join(failures, "; "), true
		}
	}
	return requestReason(chatPlainText(res)), true
}

// chatCallError reframes a failed chat round-trip. The bridge answers its own
// rejections with HTTP >= 400 and the reason in the body, which is the part worth
// reading: "Unknown provider", "Chat with gemini already in progress", a login
// redirect, or an exhausted polling budget.
func chatCallError(providers []string, err error) error {
	detail := err.Error()
	var fail *bridge.ErrBridgeFailure
	if errors.As(err, &fail) && strings.TrimSpace(fail.Body) != "" {
		detail = chatErrorBody(fail.Body)
	}
	return fmt.Errorf("chat with %s failed: %s", chatLabel(providers), detail)
}

// chatErrorBody pulls the reasons out of a bridge error body: a single-chat report
// states one error, a batch report states one per sub-chat. Either way the HTTP
// envelope is not what the caller needs to read.
func chatErrorBody(body string) string {
	text := strings.TrimSpace(body)
	var report struct {
		Error     string `json:"error"`
		Responses []any  `json:"responses"`
	}
	if json.Unmarshal([]byte(text), &report) != nil {
		return requestReason(text)
	}
	if detail := strings.TrimSpace(report.Error); detail != "" {
		return detail
	}
	if len(report.Responses) > 0 {
		failures := make([]string, 0, len(report.Responses))
		for i, raw := range report.Responses {
			if item, ok := raw.(map[string]any); ok {
				failures = append(failures, chatItemFailure(i, item))
			}
		}
		if len(failures) > 0 {
			return strings.Join(failures, "; ")
		}
	}
	return requestReason(text)
}

// chatDocument assembles the response: one provenance line naming what actually ran,
// then the reply verbatim. A reply over the configured threshold is spilled to a file
// first, so the provenance always survives even when the text does not.
func chatDocument(workspace string, deps Deps, body string, providers, messages []string, params, res map[string]any, batch bool, incomplete string) string {
	if bounded, replaced := chatReplyDocument("chat", body, workspace, deps); replaced {
		body = bounded
	}
	return renderBridgeMarkdown(
		bridgeProvenance(
			bridgeTerm("provider", chatProviderTerm(res, providers, batch)),
			bridgeTerm("queries", chatQueriesTerm(providers, messages)),
			bridgeTerm("attachment", chatAttachmentTerm(params, res)),
			bridgeTerm("downloaded", chatDownloadsTerm(res["downloads"])),
			bridgeTerm("incomplete", incomplete),
		),
		body,
	)
}

// chatQueriesTerm reports how many questions actually ran, so the caller can tell a
// single answer from a parallel fan-out.
func chatQueriesTerm(providers, messages []string) string {
	queries := len(providers)
	if len(messages) > queries {
		queries = len(messages)
	}
	if queries < 2 {
		return ""
	}
	return fmt.Sprintf("%d", queries)
}

// chatProviderTerm names the provider the bridge answered with. A fan-out keeps each
// provider inside its own section instead, so the requested list is not restated as if
// one provider had answered everything.
func chatProviderTerm(res map[string]any, asked []string, batch bool) string {
	if single := strings.TrimSpace(searchStringArg(res["provider"])); single != "" {
		return single
	}
	if batch {
		return ""
	}
	return strings.Join(asked, ", ")
}

// chatAttachmentTerm names the file the provider really received. The bridge echoes the
// path it used in `screenshot`, which is the screenshot it took, the uploaded file or
// list of them, and absent for a text-only question.
func chatAttachmentTerm(params, res map[string]any) string {
	if attachment := chatPathListText(res["screenshot"]); attachment != "" {
		return attachment
	}
	return chatPathListText(params["path"])
}

// chatPathListText renders a path, a list of paths, or nothing, as one provenance
// term. Every shape the bridge can echo is a path or an array of them.
func chatPathListText(v any) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []string:
		return strings.Join(typed, ", ")
	case []any:
		names := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				names = append(names, s)
			}
		}
		return strings.Join(names, ", ")
	default:
		return ""
	}
}

// chatDownloadsTerm reports what the artifact harvest saved. The bridge only includes
// the downloads block when the call asked for a download_dir, so a block with an empty
// file list is a harvest that found nothing - which the caller must see rather than
// assume landed somewhere.
func chatDownloadsTerm(v any) string {
	details, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if files := chatPathListText(details["files"]); files != "" {
		return files
	}
	return "nothing"
}

func chatLabel(providers []string) string {
	if len(providers) == 0 {
		return "the provider"
	}
	return strings.Join(providers, ", ")
}

// chatPlainText flattens a payload into the text that would reach the model, which is
// what the size threshold must measure.
func chatPlainText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(encoded)
}

// chatReplyDocument bounds a long answer for the model context: past
// chat_file_threshold_kb it is written to the chat output dir and the caller gets the
// pointer instead of the text. A long answer that came back is worth more as a file
// path than as a context flood, and the caller can still read it on demand. A file
// write that fails is reported as a clamped inline answer, so nothing is lost without
// a ceiling; a directory that cannot be created at all leaves the answer untouched.
func chatReplyDocument(action, reply, workspace string, deps Deps) (string, bool) {
	thresholdKb := mcpcfg.Get().ChatFileThresholdKb
	if thresholdKb <= 0 {
		thresholdKb = chatDefaultFileThresholdKb
	}
	if len(reply) <= thresholdKb*1024 {
		return reply, false
	}
	chatDir := ""
	if cfg := mcpcfg.Get(); cfg.ChatOutputPath != nil && *cfg.ChatOutputPath != "" {
		chatDir = *cfg.ChatOutputPath
	} else {
		ws := wspkg.ResolveWorkspace(workspace, workspace, deps.Store)
		chatDir = filepath.Join(ws, ".zenmcp", "chat")
	}
	if err := os.MkdirAll(chatDir, 0755); err != nil {
		return reply, false
	}
	fp := filepath.Join(chatDir, fmt.Sprintf("%s-%d.md", action, time.Now().UnixMilli()))
	if writeErr := os.WriteFile(fp, []byte(reply), 0644); writeErr != nil {
		if len(reply) > chatMaxInlineChars {
			cut := chatMaxInlineChars
			for cut > 0 && !utf8.RuneStart(reply[cut]) {
				cut--
			}
			return reply[:cut] + fmt.Sprintf("\n... [TRUNCATED: file write failed, showing first %d of %d chars]", chatMaxInlineChars, len(reply)), true
		}
		return reply, false
	}
	return fmt.Sprintf("Response saved to `%s` (%.1f KB)", fp, float64(len(reply))/1024.0), true
}
