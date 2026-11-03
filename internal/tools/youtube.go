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
}

func defYoutube(workspace string, deps Deps) ToolDef {
	descriptions := []string{"upload", "upload_status"}
	return ToolDef{
		Name:  "youtube",
		Title: "YouTube channel tool (page automation)",
		Description: "Act on one configured YouTube channel by driving Firefox through the firefox-bridge — every capability is a real page visit, " +
			"so what this tool reports is exactly what a human would see. The identity comes from youtube.container in config.json and is verified on every run. " +
			"Actions: upload (the Studio wizard: attach, details, checks, visibility, schedule) and upload_status (what this machine recorded today).",
		Schema: jsonSchema(map[string]any{
			"action":        strEnumProp("YouTube action.", descriptions),
			"file":          strProp("[upload](required) Absolute path of the video file to attach. One attach per run, never retried: it creates the video"),
			"made_for_kids": boolProp("[upload](required) The COPPA answer Studio requires before it will advance past Details"),
			"title":         strProp("[upload] Video title; omit to keep the name Studio prefilled from the filename"),
			"description":   strProp("[upload] Description text"),
			"visibility":    strEnumProp("[upload] How the video is published. schedule needs publish_at and commits through the calendar.", []string{"private", "unlisted", "public", "schedule"}),
			"publish_at":    strProp(`[upload schedule] When to publish, RFC3339 with an explicit offset, e.g. "2026-11-09T10:00:00+07:00"`),
			"tags":          arrayStringProp("[upload] Not yet supported: the tags control has not been probed, so the tool refuses rather than guesses"),
			"category":      strProp("[upload] Not yet supported: the category select has not been probed"),
			"thumbnail":     strProp("[upload] Not yet supported: the custom-thumbnail control has not been probed"),
			"video_id":      strProp("[upload_status] The id to look up in this machine's journal"),
			"dry_run":       boolProp("[upload] Navigate, verify the identity and resolve every control, then stop without attaching anything"),
			"explain":       boolProp("Include the full locator findings for every element the run touched"),
			"keep_tab":      boolProp("[upload] Leave the wizard tab open afterwards; by default it is closed"),
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
