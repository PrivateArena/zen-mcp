package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"zen-mcp/internal/bridge"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolresponse"
	"zen-mcp/internal/youtube"
	"zen-mcp/internal/youtube/pages"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// youtubeBrowser adapts the shared bridge caller to the narrow interface the youtube package drives. It is a
// variable so a test can swap in a scripted browser — the same reason the colab code injects its client.
var youtubeBrowser youtube.Browser = bridgeBrowser{}

type bridgeBrowser struct{}

func (bridgeBrowser) Call(ctx context.Context, action string, params map[string]any) (map[string]any, error) {
	return bridge.CallBridge(ctx, action, params)
}

// youtubeActionSpec is the per-action contract, in the shape the colab tool already uses: required params,
// params that must not be sent, and the closed value sets. Validation happens before any browser call, so a
// malformed request never opens a tab.
type youtubeActionSpec struct {
	required  []string
	forbidden []string
	enums     map[string][]string
	journal   string // mutation kind to claim a slot for, "" when the action reads nothing
	// unprobed names params whose page control was never recorded in a probe, so the action refuses them
	// outright instead of silently ignoring them — the rule that refuses an unsupported control, checked here
	// because it needs no browser.
	unprobed []string
}

var youtubeActionSpecs = map[string]youtubeActionSpec{
	"upload": {
		// made_for_kids is required because the page demands the answer: the wizard does not advance without
		// it and shows nothing but inline text. Guessing it on the user's behalf is a legal assertion about
		// their content, so the tool refuses instead.
		required:  []string{"file", "made_for_kids"},
		forbidden: []string{"video_id"},
		enums: map[string][]string{
			"visibility": {"private", "unlisted", "public", "schedule"},
		},
		journal: "upload",
	},
	"upload_status": {
		required:  []string{"video_id"},
		forbidden: []string{"file", "title", "description", "visibility", "publish_at", "made_for_kids"},
	},
	"read": {
		required:  []string{"video_id"},
		forbidden: []string{"file", "text", "comment_id", "title", "description", "visibility", "publish_at", "made_for_kids"},
		// The Sort by trigger is in the probe notes; the menu it opens never was (watch.md §1), and neither was
		// the control that loads the next page of a long thread. So neither is offered as behaviour.
		unprobed: []string{"order", "page_token"},
	},
	"comment": {
		required:  []string{"video_id", "text"},
		forbidden: []string{"file", "comment_id", "title", "description", "visibility", "publish_at", "made_for_kids"},
		journal:   "comment",
	},
	"reply": {
		// comment_id is required because the row is addressed by the lc id the page itself carries (watch.md
		// §1); a reply aimed by author or position could answer the wrong person.
		required:  []string{"video_id", "comment_id", "text"},
		forbidden: []string{"file", "title", "description", "visibility", "publish_at", "made_for_kids"},
		journal:   "comment",
	},
}

func defYoutube(workspace string, deps Deps) ToolDef {
	descriptions := []string{"upload", "upload_status", "read", "comment", "reply"}
	return ToolDef{
		Name:  "youtube",
		Title: "YouTube channel tool (page automation)",
		Description: "Act on one configured YouTube channel by driving Firefox through the firefox-bridge — every capability is a real page visit, " +
			"so what this tool reports is exactly what a human would see. The identity comes from youtube.container in config.json and is verified on every run. " +
			"Actions: upload (the Studio wizard: attach, details, checks, visibility, schedule), upload_status (what this machine recorded today), " +
			"read (the comments a watch page renders), comment and reply (write one, confirmed by re-reading the thread).",
		Schema: jsonSchema(map[string]any{
			"action":        strEnumProp("YouTube action.", descriptions),
			"file":          strProp("[upload](required) Absolute path of the video file to attach. One attach per run, never retried: it creates the video"),
			"made_for_kids": boolProp("[upload](required) The COPPA answer Studio requires before it will advance past Details"),
			"title":         strProp("[upload] Video title; omit to keep the name Studio prefilled from the filename"),
			"description":   strProp("[upload] Description text"),
			"visibility":    strEnumProp("[upload] How the video is published. schedule needs publish_at and commits through the calendar.", []string{"private", "unlisted", "public", "schedule"}),
			"publish_at":    strProp(`[upload schedule] When to publish, RFC3339 with an explicit offset, e.g. "2026-11-09T10:00:00+07:00". Upload only`),
			"video_id":      strProp("[read comment reply](required) The video to act on: an 11-character id, or any watch/youtu.be URL to take it from"),
			"text":          strProp("[comment reply](required) The comment text to post, at most 10000 characters. One submit press per run, never retried"),
			"comment_id":    strProp(`[reply](required) The lc id of the row to answer, exactly as read returned it, e.g. "UgwEFzl5ZWYy-wHKInF4AaABAg"`),
			"max":           intProp("[read] Most rows to report; 0 means every row the page rendered. Defaults to youtube.maxComments"),
			"order":         strProp("[read] Not yet supported: the Sort by menu has never been probed, so the page's current sort stands"),
			"page_token":    strProp("[read] Not yet supported: the control that loads the next page of a thread has never been probed"),
			"tags":          arrayStringProp("[upload] Not yet supported: the tags control has not been probed, so the tool refuses rather than guesses"),
			"category":      strProp("[upload] Not yet supported: the category select has not been probed"),
			"thumbnail":     strProp("[upload] Not yet supported: the custom-thumbnail control has not been probed"),
			"dry_run":       boolProp("[upload] Navigate, verify the identity and resolve every control, then stop without attaching anything. [comment reply] Read the thread and locate the box, then stop without pressing anything"),
			"explain":       boolProp("Include the full locator findings for every element the run touched"),
			"keep_tab":      boolProp("Leave the tab open afterwards; by default it is closed"),
		}, []string{"action"}),
		LongRunning: true,
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleYoutube(ctx, workspace, deps, req), nil
		},
	}
}

// HandleYoutube validates, then runs one action against the page.
func HandleYoutube(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)
	if action == "" {
		return toolresponse.WrapErrorWithContext(ctx, "youtube", fmt.Errorf("action is required"), start)
	}
	spec, ok := youtubeActionSpecs[action]
	if !ok {
		known := make([]string, 0, len(youtubeActionSpecs))
		for k := range youtubeActionSpecs {
			known = append(known, k)
		}
		return toolresponse.WrapErrorWithContext(ctx, "youtube",
			fmt.Errorf("unknown action %q; this tool does: %s", action, strings.Join(known, ", ")), start)
	}
	if err := validateYoutube(action, args, spec); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "youtube", err, start)
	}

	svc, err := newYoutubeService()
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "youtube", err, start)
	}

	switch action {
	case "upload":
		return youtubeUpload(ctx, svc, args, spec, start)
	case "upload_status":
		return youtubeUploadStatus(ctx, svc, args, start)
	case "read", "comment", "reply":
		return youtubeWatch(ctx, svc, args, action, start)
	}
	return toolresponse.WrapErrorWithContext(ctx, "youtube", fmt.Errorf("action %q is declared but not wired", action), start)
}

// validateYoutube is the cheap, browser-free half of the contract: required params present, forbidden params
// absent, closed enums inside their value set.
func validateYoutube(action string, args map[string]any, spec youtubeActionSpec) error {
	for _, name := range spec.required {
		if !hasArg(args, name) {
			return fmt.Errorf("youtube %s: %s is required", action, name)
		}
	}
	for _, name := range spec.forbidden {
		if hasArg(args, name) {
			return fmt.Errorf("youtube %s: %s does not apply to this action", action, name)
		}
	}
	for name, allowed := range spec.enums {
		v, ok := args[name].(string)
		if !ok || v == "" {
			continue
		}
		if !containsString(allowed, strings.ToLower(v)) {
			return fmt.Errorf("youtube %s: %s must be one of %s", action, name, strings.Join(allowed, ", "))
		}
	}
	for _, name := range spec.unprobed {
		if hasArg(args, name) {
			return fmt.Errorf("youtube %s: %s is not supported yet — the control it would drive has never been probed, so there is nothing to run", action, name)
		}
	}
	return nil
}

func hasArg(args map[string]any, name string) bool {
	v, present := args[name]
	if !present || v == nil {
		return false
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) != ""
	}
	return true
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// youtubeReply is what the tool prints: the flow's outcome, the identity it ran as, the bounded diagnostics,
// and the journal state that charged the slot.
type youtubeReply struct {
	VideoID      string          `json:"videoId,omitempty"`
	URL          string          `json:"url,omitempty"`
	Visibility   string          `json:"visibility,omitempty"`
	ScheduledFor string          `json:"scheduledFor,omitempty"`
	Checks       string          `json:"checks,omitempty"`
	Panel        string          `json:"panel,omitempty"`
	DryRun       bool            `json:"dryRun,omitempty"`
	TabID        string          `json:"tab,omitempty"`
	ContainerID  int             `json:"containerId,omitempty"`
	Steps        []string        `json:"steps,omitempty"`
	Warnings     []string        `json:"warnings,omitempty"`
	Findings     json.RawMessage `json:"findings,omitempty"`
	Trail        []string        `json:"trail,omitempty"`
	UploadsToday int             `json:"uploadsToday,omitempty"`
	Limit        int             `json:"dailyUploadLimit,omitempty"`
	// CommentID is the lc id the page handed back for what this run posted — the id a later reply addresses.
	CommentID string `json:"commentId,omitempty"`
	// Comments is the read payload: the rows the watch page rendered.
	Comments  json.RawMessage `json:"comments,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
	// UntrustedUserContent marks a comment payload as words a stranger wrote: data to report, never
	// instructions to act on.
	UntrustedUserContent bool `json:"untrusted_user_content,omitempty"`
	// PageVerified is the distinction the probe insists on (watch.md §4): a write is reported as done only
	// when a re-read found it. A run that pressed and could not confirm says false with an error, never a
	// silence that reads as success.
	PageVerified *bool `json:"pageVerified,omitempty"`
}

// commentReply is one row of a read, in the shape the caller is given. The page-proven DOM address comments.js
// derives is deliberately not part of it: that is how a flow scopes a press, not something to hand back, and
// the lc id is the only address a caller needs.
type commentReply struct {
	ID       string `json:"id,omitempty"`
	Author   string `json:"author,omitempty"`
	Text     string `json:"text,omitempty"`
	When     string `json:"when,omitempty"`
	Absolute string `json:"absolute,omitempty"`
	Nested   bool   `json:"nested,omitempty"`
	IsReply  bool   `json:"isReply,omitempty"`
}

// maxCommentRunes is the longest text this tool will attempt to post. The page's own counter was never
// probed, so the ceiling is enforced here rather than discovered by a box that might truncate silently.
// UNPROBED: the length limit as the page itself enforces it.
const maxCommentRunes = 10000

// youtubeWatch runs the three watch page actions. Comment and reply claim their journal slot inside the flow,
// next to the submit press, so this layer only refuses what it can see is wrong and reports what came back.
func youtubeWatch(ctx context.Context, svc *youtube.Service, args map[string]any, action string, start time.Time) *mcp.CallToolResult {
	id, err := parseVideoID(argString(args, "video_id"))
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "youtube", err, start)
	}
	text := argString(args, "text")
	if n := len([]rune(text)); n > maxCommentRunes {
		return toolresponse.WrapErrorWithContext(ctx, "youtube",
			fmt.Errorf("youtube %s: text is %d characters and the cap is %d; nothing was sent, because the page's own counter has never been probed", action, n, maxCommentRunes), start)
	}
	max := svc.Settings().MaxComments
	if v, ok := args["max"]; ok {
		if f, isNum := v.(float64); isNum {
			max = int(f)
		}
	}

	out, tab, err := svc.Watch(ctx, youtube.WatchInput{
		VideoID:   id,
		Action:    action,
		Text:      text,
		CommentID: argString(args, "comment_id"),
		DryRun:    dryRun(args),
	}, pages.Watch)

	// The tab is policy, not page state, so it is closed here — a dry run included, since it still opened a
	// real one. A failed run keeps it, so the human can see what the page did.
	if err == nil && !argBool(args, "keep_tab") && tab.ID != "" {
		if _, cerr := bridge.CallBridge(ctx, "close_tab", map[string]any{"index": tab.ID}); cerr != nil {
			out.Warnings = append(out.Warnings, "the watch tab stayed open: "+cerr.Error())
		}
	}

	reply := youtubeReply{
		VideoID: out.VideoID, URL: out.URL, CommentID: out.CommentID, DryRun: dryRun(args),
		TabID: tab.ID, ContainerID: tab.ContainerID,
	}
	if action != "read" {
		// A write that returns without error has re-read the thread and found its own post in it.
		yes, no := true, false
		reply.PageVerified = &yes
		if err != nil {
			reply.PageVerified = &no
		}
	}
	if action == "read" {
		reply.UntrustedUserContent = true
		rows, kept, total, cerr := capCommentRows(out.Data, max)
		if cerr != nil {
			out.Warnings = append(out.Warnings, cerr.Error())
		} else {
			reply.Comments, reply.Truncated = rows, kept < total
		}
	}
	// Taken last: the tab-close notice and a failed decode are both warnings the flow did not know about.
	reply.Steps, reply.Warnings = out.Steps, out.Warnings
	for _, e := range svc.Trail(12) {
		reply.Trail = append(reply.Trail, fmt.Sprintf("%s %s -> %s", e.Action, e.Subject, e.Result))
	}
	if argBool(args, "explain") {
		reply.Findings, _ = json.Marshal(out.Findings)
	}
	if err != nil {
		// The reply still carries everything the run learned: for a write, pageVerified:false beside the error
		// is the difference between "nothing happened" and "a comment exists that you must go and look at".
		raw, _ := json.MarshalIndent(reply, "", "  ")
		return toolresponse.WrapErrorWithContext(ctx, "youtube", fmt.Errorf("%w\n%s", err, string(raw)), start)
	}
	return toolresponse.WrapSuccess(ctx, "youtube", reply, start)
}

// capCommentRows trims a read to max rows and re-encodes it without the page-internal row address. max <= 0
// means every row the page rendered; the second result is how many were kept of how many the page showed.
func capCommentRows(raw json.RawMessage, max int) (json.RawMessage, int, int, error) {
	if len(raw) == 0 {
		return nil, 0, 0, fmt.Errorf("the comment read returned nothing to decode")
	}
	var got struct {
		Rows []commentReply `json:"rows"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		return nil, 0, 0, fmt.Errorf("decode the comment read: %w", err)
	}
	keep := got.Rows
	if max > 0 && len(keep) > max {
		keep = keep[:max]
	}
	out, err := json.Marshal(keep)
	if err != nil {
		return nil, 0, len(got.Rows), fmt.Errorf("encode the comment rows: %w", err)
	}
	return out, len(keep), len(got.Rows), nil
}

func youtubeUpload(ctx context.Context, svc *youtube.Service, args map[string]any, spec youtubeActionSpec, start time.Time) *mcp.CallToolResult {
	kids, _ := args["made_for_kids"].(bool)
	in := youtube.Upload{
		File:        argString(args, "file"),
		Title:       argString(args, "title"),
		Description: argString(args, "description"),
		Visibility:  strings.ToLower(argString(args, "visibility")),
		MadeForKids: &kids,
		// Fail to the safe side: an unrecognised dry_run value means a dry run, never an upload.
		DryRun: dryRun(args),
	}
	if raw := argString(args, "publish_at"); raw != "" {
		at, err := parsePublishAt(raw, svc.Settings().Timezone)
		if err != nil {
			return toolresponse.WrapErrorWithContext(ctx, "youtube", err, start)
		}
		in.PublishAt = at
	}
	if len(argStrings(args, "tags")) > 0 || argString(args, "category") != "" || argString(args, "thumbnail") != "" {
		return toolresponse.WrapErrorWithContext(ctx, "youtube",
			fmt.Errorf("tags, category and thumbnail are not supported yet: those controls were never probed, so there is no selector to run. Set them in Studio or wait for the details-page flow"), start)
	}

	// A wizard is long; the context the server gives the tool is the outer bound, and the flow's own budgets
	// are derived from config, never from a guessed default.
	out, tab, err := svc.Upload(ctx, in, pages.UploadWizard)

	// The tab is policy, not page state, so it is closed here rather than by the flow — including a dry
	// run, which still opened a real tab. It stays open when the run failed (so the human can look) or when
	// keep_tab asks for it.
	if err == nil && !argBool(args, "keep_tab") && tab.ID != "" {
		if _, cerr := bridge.CallBridge(ctx, "close_tab", map[string]any{"index": tab.ID}); cerr != nil {
			out.Warnings = append(out.Warnings, "the wizard tab stayed open: "+cerr.Error())
		}
	}

	reply := youtubeReply{
		VideoID: out.VideoID, URL: out.URL, Visibility: out.Visibility, ScheduledFor: out.ScheduledFor,
		Checks: out.Checks, Panel: out.Panel, DryRun: in.DryRun,
		TabID: tab.ID, ContainerID: tab.ContainerID, Steps: out.Steps, Warnings: out.Warnings,
		UploadsToday: svc.TodayUploads(), Limit: svc.UploadLimit(),
	}
	for _, e := range svc.Trail(12) {
		reply.Trail = append(reply.Trail, fmt.Sprintf("%s %s -> %s", e.Action, e.Subject, e.Result))
	}
	if argBool(args, "explain") {
		reply.Findings, _ = json.Marshal(out.Findings)
	}
	if err != nil {
		// The reply still carries everything the run learned: a stuck wizard's video id is the difference
		// between "nothing happened" and "an unrecorded draft exists on the channel".
		payload, _ := json.MarshalIndent(reply, "", "  ")
		return toolresponse.WrapErrorWithContext(ctx, "youtube", fmt.Errorf("%w\n%s", err, string(payload)), start)
	}
	return toolresponse.WrapSuccess(ctx, "youtube", reply, start)
}

func youtubeUploadStatus(ctx context.Context, svc *youtube.Service, args map[string]any, start time.Time) *mcp.CallToolResult {
	id := argString(args, "video_id")
	slot, found := svc.SlotForVideo(id)
	if !found {
		return toolresponse.WrapErrorWithContext(ctx, "youtube",
			fmt.Errorf("no upload for video %q is recorded in this machine's journal (today: %d upload slot(s)). This action reports what this tool did — it does not browse the channel on request",
				id, svc.TodayUploads()), start)
	}
	return toolresponse.WrapSuccess(ctx, "youtube", map[string]any{
		"videoId":      id,
		"slotId":       slot.SlotID,
		"status":       slot.Status,
		"claimedAt":    slot.ClaimedAt.Format(time.RFC3339),
		"detail":       slot.Detail,
		"uploadsToday": svc.TodayUploads(),
		"dailyLimit":   svc.Settings().MaxUploadsPerDay,
		"pageVerified": false,
		"note":         "listing a video on the channel content page is Stage 3's probe; until it is frozen this action reports only the local record",
	}, start)
}

// parsePublishAt accepts an RFC3339 timestamp with an explicit offset, which is the shape the plan fixes, and
// rejects an ambiguous one rather than assuming a zone for a publish date.
func parsePublishAt(raw, zone string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf(`publish_at must be RFC3339 with an offset, e.g. "2026-11-09T10:00:00+07:00" (got %q)`, raw)
	}
	if zone != "" {
		if loc, lerr := time.LoadLocation(zone); lerr == nil {
			t = t.In(loc)
		}
	}
	return t, nil
}

// parseVideoID accepts what a human pastes: a bare 11-character id (the probe fixture is YSNFGgh0aAs) or a URL
// carrying it in v= — the form the probe used — or after youtu.be/, the form this tool's own upload reply hands
// back. Like parsePublishAt it is settled before the browser is touched, because guessing at a video means
// acting on the wrong one.
func parseVideoID(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if isVideoID(s) {
		return s, nil
	}
	if i := strings.Index(s, "v="); i >= 0 {
		if id := cutParam(s[i+2:]); isVideoID(id) {
			return id, nil
		}
	}
	if i := strings.Index(s, "youtu.be/"); i >= 0 {
		if id := cutParam(s[i+len("youtu.be/"):]); isVideoID(id) {
			return id, nil
		}
	}
	return "", fmt.Errorf(`video_id must be an 11-character video id or a URL carrying it (watch?v=<id>, youtu.be/<id>); got %q`, trimID(raw))
}

// cutParam takes the id out of a query string: everything up to the next parameter separator.
func cutParam(s string) string {
	if i := strings.IndexAny(s, "&#?/ "); i >= 0 {
		return s[:i]
	}
	return s
}

// isVideoID is the id shape as the page reports it: 11 characters of the URL-safe alphabet, the pattern
// wizard_state.js already matches for a video id in a link.
func isVideoID(s string) bool {
	if len(s) != 11 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

func trimID(s string) string {
	r := []rune(s)
	if len(r) > 60 {
		return string(r[:57]) + "..."
	}
	return s
}

func newYoutubeService() (*youtube.Service, error) {
	cfg := mcpcfg.Get().Youtube
	path, err := youtube.JournalPath()
	if err != nil {
		return nil, err
	}
	loc := time.Local
	if l, lerr := time.LoadLocation(cfg.Timezone); lerr == nil {
		loc = l
	}
	j, err := youtube.OpenJournal(path, cfg.MaxUploadsPerDay, loc)
	if err != nil {
		return nil, err
	}
	return youtube.NewService(youtubeBrowser, youtube.Settings{
		Container:         cfg.Container,
		ContainerID:       cfg.ContainerID,
		MaxUploadsPerDay:  cfg.MaxUploadsPerDay,
		DefaultVisibility: cfg.DefaultVisibility,
		Timezone:          cfg.Timezone,
		AllowFocusBound:   cfg.AllowFocusBoundActions,
		PageTimeout:       time.Duration(cfg.PageTimeoutMs) * time.Millisecond,
		WizardTimeout:     time.Duration(cfg.WizardTimeoutMs) * time.Millisecond,
		ProcessTimeout:    time.Duration(cfg.ProcessTimeoutMs) * time.Millisecond,
		MaxUploadBytes:    cfg.MaxUploadBytes,
		MaxDiagBytes:      cfg.MaxDiagBytes,
		MaxComments:       cfg.MaxComments,
		MaxScrolls:        cfg.MaxScrolls,
		EvidenceDir:       youtube.DefaultEvidenceDir(),
	}, j, nil), nil
}

func argString(args map[string]any, k string) string {
	s, _ := args[k].(string)
	return strings.TrimSpace(s)
}

// dryRun reads the flag that stands between a probe and a real upload. Anything that is not an explicit
// false is treated as true: a client that sends "yes"/"1"/"" or a value this tool cannot read must never
// cause an attach, because an attach cannot be undone by a retry policy.
func dryRun(args map[string]any) bool {
	v, present := args["dry_run"]
	if !present || v == nil {
		return false // absence is not a request for a dry run; the caller simply wants the real thing
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "false", "0", "no", "off", "":
			return false
		}
		return true
	case float64:
		return x != 0
	}
	return true
}

func argBool(args map[string]any, k string) bool {
	b, _ := args[k].(bool)
	return b
}

func argStrings(args map[string]any, k string) []string {
	switch v := args[k].(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
