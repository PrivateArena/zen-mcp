package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
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
	// requiredAny names params of which the action needs at least one. An edit_details carrying neither title
	// nor description has nothing for the page to save, and an enabled Save is the only signal that anything
	// changed at all (studio-details.md §5.3) — so the caller is told here, before a tab exists, not by the page.
	requiredAny []string
	// requireTrue names a boolean param that must arrive saying yes. An absent flag and a false one are the
	// same thing to the page — no confirmation — so both are refused, and refused before the browser is touched,
	// because the refusal is the entire point of the parameter (§5.4).
	requireTrue []string
}

var youtubeActionSpecs = map[string]youtubeActionSpec{
	"upload": {
		// made_for_kids is required because the page demands the answer: the wizard does not advance without
		// it and shows nothing but inline text. Guessing it on the user's behalf is a legal assertion about
		// their content, so the tool refuses instead.
		required:  []string{"file", "made_for_kids"},
		forbidden: []string{"video_id", "yes_i_understand"},
		enums: map[string][]string{
			"visibility": {"private", "unlisted", "public", "schedule"},
		},
		journal: "upload",
	},
	"upload_status": {
		required:  []string{"video_id"},
		forbidden: []string{"file", "title", "description", "visibility", "publish_at", "made_for_kids", "yes_i_understand"},
	},
	"read": {
		required:  []string{"video_id"},
		forbidden: []string{"file", "text", "comment_id", "title", "description", "visibility", "publish_at", "made_for_kids", "yes_i_understand"},
		// The Sort by trigger is in the probe notes; the menu it opens never was (watch.md §1), and neither was
		// the control that loads the next page of a long thread. So neither is offered as behaviour.
		unprobed: []string{"order", "page_token"},
	},
	"comment": {
		required:  []string{"video_id", "text"},
		forbidden: []string{"file", "comment_id", "title", "description", "visibility", "publish_at", "made_for_kids", "yes_i_understand"},
		journal:   "comment",
	},
	"reply": {
		// comment_id is required because the row is addressed by the lc id the page itself carries (watch.md
		// §1); a reply aimed by author or position could answer the wrong person.
		required:  []string{"video_id", "comment_id", "text"},
		forbidden: []string{"file", "title", "description", "visibility", "publish_at", "made_for_kids", "yes_i_understand"},
		journal:   "comment",
	},
	// Stage 4: the Studio details panel. Both actions address a video by id — §1's entry URL carries it, which is
	// why neither needs the channel content list — and both refuse every parameter that belongs to the other
	// pages, so a wrong parameter is a refusal before the browser is touched rather than a silently ignored one.
	"edit_details": {
		required:    []string{"video_id"},
		requiredAny: []string{"title", "description"},
		forbidden:   []string{"file", "publish_at", "text", "comment_id", "made_for_kids", "max", "order", "page_token", "yes_i_understand"},
		// The panel shows the visibility card, the tags field, the category select and the thumbnail (§2, "fields
		// present but not touched by this tool"); none of them was ever probed *as editable*, so asking for one
		// is refused rather than quietly dropped.
		unprobed: []string{"visibility", "tags", "category", "thumbnail"},
		journal:  "edit",
	},
	"delete_video": {
		required:    []string{"video_id"},
		requireTrue: []string{"yes_i_understand"},
		// title and description belong to the edit: a delete that accepted them would leave a caller believing a
		// field had been changed on the way to destroying the video.
		forbidden: []string{"file", "publish_at", "title", "description", "text", "comment_id", "made_for_kids", "max", "order", "page_token"},
		unprobed:  []string{"visibility", "tags", "category", "thumbnail"},
		journal:   "delete",
	},
}

func defYoutube(workspace string, deps Deps) ToolDef {
	descriptions := []string{"upload", "upload_status", "read", "comment", "reply", "edit_details", "delete_video"}
	return ToolDef{
		Name:  "youtube",
		Title: "YouTube channel tool (page automation)",
		Description: "Act on one configured YouTube channel by driving Firefox through the firefox-bridge — every capability is a real page visit, " +
			"so what this tool reports is exactly what a human would see. The channel is a Firefox contextual identity: youtube.container is the default and " +
			"container selects another declared in youtube.containers, so one server drives several channels; the containerId the bridge reports is asserted " +
			"against that declaration on every run and an undeclared name is refused. " +
			"Actions: upload (the Studio wizard: attach, details, checks, visibility, schedule), upload_status (what this machine recorded today), " +
			"read (the comments a watch page renders), comment and reply (write one, confirmed by re-reading the thread), " +
			"edit_details (change the title and/or description and commit them with one Save press), delete_video " +
			"(permanent, so it is refused unless yes_i_understand=true).",
		Schema: jsonSchema(map[string]any{
			"action":           strEnumProp("YouTube action.", descriptions),
			"file":             strProp("[upload](required) Absolute path of the video file to attach. One attach per run, never retried: it creates the video"),
			"made_for_kids":    boolProp("[upload](required) The COPPA answer Studio requires before it will advance past Details"),
			"title":            strProp("[upload] Video title; omit to keep the name Studio prefilled from the filename. [edit_details](required unless description is given) The title to save: it is written, read back, and only then committed"),
			"description":      strProp("[upload] Description text. [edit_details](required unless title is given) The description to save, written the same way and read back before Save is pressed"),
			"visibility":       strEnumProp("[upload] How the video is published. schedule needs publish_at and commits through the calendar. [edit_details delete_video] Not supported: the panel's visibility card was never probed as an editable control", []string{"private", "unlisted", "public", "schedule"}),
			"publish_at":       strProp(`[upload schedule] When to publish, RFC3339 with an explicit offset, e.g. "2026-11-09T10:00:00+07:00". Upload only`),
			"container":        strProp("Which channel to act as: a Firefox contextual-identity name declared in youtube.containers. Omit it to use youtube.container. An undeclared name is refused before anything opens, because an id looked up from the live browser is how a typo becomes another channel"),
			"video_id":         strProp("[read comment reply edit_details delete_video](required) The video to act on: an 11-character id, or any watch/youtu.be URL to take it from"),
			"text":             strProp("[comment reply](required) The comment text to post, at most 10000 characters. One submit press per run, never retried"),
			"comment_id":       strProp(`[reply](required) The lc id of the row to answer, exactly as read returned it, e.g. "UgwEFzl5ZWYy-wHKInF4AaABAg"`),
			"yes_i_understand": boolProp("[delete_video](required) States that the delete is permanent and cannot be undone. Anything other than true refuses the run before the page is opened, so nothing is navigated, pressed or deleted"),
			"max":              intProp("[read] Most rows to report; 0 means every row the page rendered. Defaults to youtube.maxComments"),
			"order":            strProp("[read] Not yet supported: the Sort by menu has never been probed, so the page's current sort stands"),
			"page_token":       strProp("[read] Not yet supported: the control that loads the next page of a thread has never been probed"),
			"tags":             arrayStringProp("[upload] Not yet supported: the tags control has not been probed, so the tool refuses rather than guesses"),
			"category":         strProp("[upload] Not yet supported: the category select has not been probed"),
			"thumbnail":        strProp("[upload] Not yet supported: the custom-thumbnail control has not been probed"),
			"dry_run":          boolProp("[upload] Navigate, verify the identity and resolve every control, then stop without attaching anything. [comment reply] Read the thread and locate the box, then stop without pressing anything. [edit_details delete_video] Read the fields, or find the Options button, then stop without typing, without opening the delete dialog and without pressing Save"),
			"explain":          boolProp("Include the full locator findings for every element the run touched"),
			"keep_tab":         boolProp("Leave the tab open afterwards; by default it is closed"),
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

	svc, err := newYoutubeService(workspace, argString(args, "container"))
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
	case "edit_details", "delete_video":
		return youtubeDetails(ctx, svc, args, action, start)
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
	// An edit with no field to write is refused here rather than by the page: the flow reaches the same
	// conclusion before it navigates (studio-details.md §5.3), and a caller who never sent a title should learn
	// that from a parameter error, not from a page visit.
	if len(spec.requiredAny) > 0 {
		found := false
		for _, name := range spec.requiredAny {
			if hasArg(args, name) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("youtube %s: at least one of %s is required", action, strings.Join(spec.requiredAny, ", "))
		}
	}
	for _, name := range spec.requireTrue {
		if !trueArg(args, name) {
			return fmt.Errorf("youtube %s: %s must be true — the change it authorises is permanent, so the run is refused, nothing was opened and nothing was pressed", action, name)
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
	VideoID      string `json:"videoId,omitempty"`
	URL          string `json:"url,omitempty"`
	Visibility   string `json:"visibility,omitempty"`
	ScheduledFor string `json:"scheduledFor,omitempty"`
	Checks       string `json:"checks,omitempty"`
	Panel        string `json:"panel,omitempty"`
	DryRun       bool   `json:"dryRun,omitempty"`
	TabID        string `json:"tab,omitempty"`
	ContainerID  int    `json:"containerId,omitempty"`
	// Container names the channel the run acted as. The reply has to say it: the same action can be aimed at
	// a different channel on every call, so "which account did this" is never left to be inferred.
	Container    string          `json:"container,omitempty"`
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
		TabID: tab.ID, ContainerID: tab.ContainerID, Container: svc.Settings().Container,
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

// detailsFlowAction is the flow's name for a tool action. The tool spells them edit_details and delete_video so
// an agent sees what they cost; the flow's vocabulary is the page's, and it is the closed set Service.Details
// accepts (studio-details.md §5).
func detailsFlowAction(action string) string {
	if action == "delete_video" {
		return "delete"
	}
	return "edit"
}

// youtubeDetails runs the two Studio details-panel actions. Like the comment flow, and unlike the upload wizard,
// it claims no journal slot here: the mutation is one press deep inside the flow (Save, or Delete forever), so the
// flow claims next to that press — and a dry run is handed no ledger at all, because it presses nothing.
func youtubeDetails(ctx context.Context, svc *youtube.Service, args map[string]any, action string, start time.Time) *mcp.CallToolResult {
	id, err := parseVideoID(argString(args, "video_id"))
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "youtube", err, start)
	}
	// Read once here and handed to the flow as DetailsInput.Confirmed — the one and only thing the parameter
	// becomes. validateYoutube already refused a run that did not confirm, so reaching this line with a value
	// that is not true means a caller bypassed the validator; refusing again costs nothing and a delete cannot
	// be undone.
	confirmed := trueArg(args, "yes_i_understand")
	if action == "delete_video" && !confirmed {
		return toolresponse.WrapErrorWithContext(ctx, "youtube",
			fmt.Errorf("youtube delete_video: yes_i_understand must be true — deleting a video is permanent and cannot be undone, so the overflow menu was never opened and nothing was pressed"), start)
	}
	dry := dryRun(args)

	out, tab, err := svc.Details(ctx, youtube.DetailsInput{
		VideoID:     id,
		Action:      detailsFlowAction(action),
		Title:       argString(args, "title"),
		Description: argString(args, "description"),
		DryRun:      dry,
		Confirmed:   confirmed,
	}, pages.Details)

	// The tab is policy, not page state, so it is closed here — a dry run included, since it still opened a
	// real one. A failed run keeps it, so the human can see what the page did: after a delete that reported a
	// failure, that tab is the evidence of whether the video is gone.
	if err == nil && !argBool(args, "keep_tab") && tab.ID != "" {
		if _, cerr := bridge.CallBridge(ctx, "close_tab", map[string]any{"index": tab.ID}); cerr != nil {
			out.Warnings = append(out.Warnings, "the details tab stayed open: "+cerr.Error())
		}
	}

	reply := youtubeReply{
		VideoID: out.VideoID, URL: out.URL, DryRun: dry,
		TabID: tab.ID, ContainerID: tab.ContainerID, Container: svc.Settings().Container,
	}
	if !dry {
		// A run that returns without error has watched the page confirm the change — the `Changes saved` snackbar
		// and the Save button going disabled (§2), or the delete dialog leaving the screen (§3). It is reported
		// verified for exactly that reason, never because a press reported success. A dry run presses nothing, so
		// there is no change for the page to have verified and the field is left out rather than answered true.
		yes, no := true, false
		reply.PageVerified = &yes
		if err != nil {
			reply.PageVerified = &no
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
		// The reply still carries everything the run learned. After a Save or a Delete-forever press that could not
		// be confirmed, pageVerified:false beside the error is the difference between "nothing happened" and "a
		// video may be gone; go and look", and pressing again is forbidden either way (§5.2, §5.4).
		raw, _ := json.MarshalIndent(reply, "", "  ")
		return toolresponse.WrapErrorWithContext(ctx, "youtube", fmt.Errorf("%w\n%s", err, string(raw)), start)
	}
	return toolresponse.WrapSuccess(ctx, "youtube", reply, start)
}

func youtubeUpload(ctx context.Context, svc *youtube.Service, args map[string]any, spec youtubeActionSpec, start time.Time) *mcp.CallToolResult {
	// A COPPA declaration is a legal answer, not a preference: an unreadable value is refused rather than
	// defaulted, because defaulting it is exactly how a video gets declared "not made for kids" by accident.
	kids, err := strictBool(args, "made_for_kids")
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "youtube", err, start)
	}
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
		TabID: tab.ID, ContainerID: tab.ContainerID, Container: svc.Settings().Container, Steps: out.Steps, Warnings: out.Warnings,
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
			fmt.Errorf("no upload for video %q is recorded in the journal of channel %q (today: %d upload slot(s)). This action reports what this tool did — it does not browse the channel on request, and every channel keeps its own ledger, so an upload made as another container does not appear here",
				id, svc.Settings().Container, svc.TodayUploads()), start)
	}
	return toolresponse.WrapSuccess(ctx, "youtube", map[string]any{
		"videoId":      id,
		"container":    svc.Settings().Container,
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

// resolveChannel picks the channel a call acts as: the configured default, or a pair declared in
// youtube.containers. Anything else is refused - resolving an unknown name against the live browser would let
// a typo act as whichever channel happens to answer, which is the failure this tool may never have.
func resolveChannel(cfg mcpcfg.YoutubeConfig, requested string) (string, int, error) {
	name := strings.TrimSpace(requested)
	if name == "" || name == cfg.Container {
		if cfg.Container == "" {
			return "", 0, fmt.Errorf("youtube: no channel to act as - set youtube.container in config.json, or declare one in youtube.containers and pass container")
		}
		return cfg.Container, cfg.ContainerID, nil
	}
	id, ok := cfg.Containers[name]
	if !ok {
		declared := make([]string, 0, len(cfg.Containers)+1)
		seen := map[string]bool{}
		for _, k := range append([]string{cfg.Container}, slices.Collect(maps.Keys(cfg.Containers))...) {
			if k != "" && !seen[k] {
				seen[k] = true
				declared = append(declared, k)
			}
		}
		sort.Strings(declared)
		return "", 0, fmt.Errorf("%w: container %q is not a declared channel; declare it in youtube.containers (declared: %s)", youtube.ErrUsage, name, strings.Join(declared, ", "))
	}
	if id <= 0 {
		return "", 0, fmt.Errorf("%w: youtube.containers maps %q to %d, and containerId 0 is the browser's default identity, which this tool will not act as", youtube.ErrUsage, name, id)
	}
	return name, id, nil
}

// newYoutubeService resolves the identity for one workspace: the machine-wide
// youtube block, unless <workspace>/.zenmcp/config.json overrides it. The
// per-call container param still wins over both - a project declares its default
// channel, an agent may name another one it has declared.
func newYoutubeService(workspace, requestedContainer string) (*youtube.Service, error) {
	cfg := mcpcfg.For(workspace).Youtube
	container, containerID, err := resolveChannel(cfg, requestedContainer)
	if err != nil {
		return nil, err
	}
	// The default channel keeps the original journal.json - forking its history onto a new file would reset
	// the cap it has already spent. Only a channel selected over the default gets its own ledger.
	ledger := container
	if ledger == cfg.Container {
		ledger = ""
	}
	path, err := youtube.JournalPath(ledger)
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
		Container:         container,
		ContainerID:       containerID,
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
	b, _ := strictBool(args, k)
	return b
}

// strictBool reads a boolean a caller has to mean. The generated CLI wrapper can only send strings, so the
// typed form is not enough: before this, --made_for_kids=true arrived as "true", read as false, and would have
// declared a video not made for kids when the human said the opposite. Absent reads as false with no error,
// because presence is validated separately; a value that is present but unreadable is an error, so a typo can
// never pick an answer by default.
func strictBool(args map[string]any, k string) (bool, error) {
	v, present := args[k]
	if !present || v == nil || v == "" {
		return false, nil
	}
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		parsed, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(x)))
		if err == nil {
			return parsed, nil
		}
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "yes", "y", "no", "n":
			return strings.HasPrefix(strings.ToLower(strings.TrimSpace(x)), "y"), nil
		}
	case float64:
		if x == 0 || x == 1 {
			return x == 1, nil
		}
	}
	return false, fmt.Errorf("%w: %s must be true or false, and %v is neither — the tool will not guess this answer", youtube.ErrUsage, k, v)
}

// trueArg reads a boolean parameter that has to say yes, accepting the textual forms a shell wrapper can only
// ever send (the z* scripts post every value as a string). It is deliberately the opposite of dryRun, which
// falls back to the safe side — a rehearsal — when it cannot read a value: here the safe side is *no*, because a
// value this tool cannot read must never stand in for a human's confirmation of an irreversible press. A JSON
// number is not read as a confirmation either; the schema asks for a boolean and a 1 is a caller guessing.
func trueArg(args map[string]any, k string) bool {
	switch v := args[k].(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		switch s {
		case "yes", "y", "on":
			return true
		}
		b, err := strconv.ParseBool(s)
		return err == nil && b
	}
	return false
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
