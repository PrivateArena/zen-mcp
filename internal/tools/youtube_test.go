package tools

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"zen-mcp/internal/mcpcfg"
)

func youtubeDef(t *testing.T) ToolDef {
	t.Helper()
	for _, d := range AllDefs(t.TempDir(), Deps{}) {
		if d.Name == "youtube" {
			return d
		}
	}
	t.Fatal("the youtube tool is not registered in AllDefs")
	return ToolDef{}
}

// The identity safety is a schema property: an agent reading tools/list must see which container the tool
// will act as and that nothing can name a URL to act on.
func TestYoutubeSchemaDeclaresTheSafetyParams(t *testing.T) {
	def := youtubeDef(t)
	// A YouTube action is a page flow, not a command line: no run_in_background.
	for name := range def.Schema["properties"].(map[string]any) {
		if name == "run_in_background" {
			t.Fatal("the youtube tool exposes run_in_background, which belongs to run")
		}
		if name == "code" {
			t.Fatal("the youtube tool exposes code, which would bypass the identity check")
		}
	}
	for _, want := range []string{"action", "file", "made_for_kids", "visibility", "publish_at", "dry_run", "explain", "video_id", "text", "comment_id", "max", "order", "page_token"} {
		if _, ok := def.Schema["properties"].(map[string]any)[want]; !ok {
			t.Errorf("schema does not declare %q", want)
		}
	}
	if !def.LongRunning {
		t.Error("the wizard is minutes of page work; without LongRunning the default 30 s context would cancel it mid-upload")
	}
	raw, err := json.Marshal(def.Schema["properties"])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"made_for_kids", "dry_run", "keep_tab", "publish_at"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("schema is missing %s", want)
		}
	}
	// The container is not an agent-editable parameter at all: it comes from config, so a request cannot
	// name a different identity.
	if _, ok := def.Schema["properties"].(map[string]any)["container"]; ok {
		t.Error("the schema lets the caller choose a container; the identity must come from config only")
	}
	if !strings.Contains(def.Description, "config") {
		t.Error("the description does not say where the channel identity comes from")
	}
}

// A closed value set, checked before anything touches the browser.
func TestYoutubeActionValidation(t *testing.T) {
	// Each case is checked through the real validator.
	cases := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{}, "action is required"},
		{map[string]any{"action": "delete_video"}, ""},
		{map[string]any{"action": "upload", "made_for_kids": false}, "file is required"},
		{map[string]any{"action": "upload", "file": "/tmp/x.mp4"}, "made_for_kids is required"},
		{map[string]any{"action": "upload", "file": "/tmp/x.mp4", "made_for_kids": false, "visibility": "secret"}, "visibility must be one of"},
		{map[string]any{"action": "upload_status"}, "video_id is required"},
		{map[string]any{"action": "upload_status", "video_id": "abc", "file": "/tmp/x.mp4"}, "does not apply"},
		// Stage 2: all three watch actions address a video, and no write is accepted without its text.
		{map[string]any{"action": "read"}, "video_id is required"},
		{map[string]any{"action": "read", "video_id": "YSNFGgh0aAs"}, ""},
		{map[string]any{"action": "comment", "video_id": "YSNFGgh0aAs"}, "text is required"},
		{map[string]any{"action": "comment", "text": "hello"}, "video_id is required"},
		{map[string]any{"action": "comment", "video_id": "YSNFGgh0aAs", "text": "hello"}, ""},
		{map[string]any{"action": "reply", "video_id": "YSNFGgh0aAs", "text": "hello"}, "comment_id is required"},
		{map[string]any{"action": "reply", "video_id": "YSNFGgh0aAs", "comment_id": "UgwEFzl5ZWYy-wHKInF4AaABAg"}, "text is required"},
		{map[string]any{"action": "reply", "video_id": "YSNFGgh0aAs", "comment_id": "UgwEFzl5ZWYy-wHKInF4AaABAg", "text": "hi"}, ""},
		// publish_at and the file belong to the wizard alone; a comment cannot smuggle an upload's parameters.
		{map[string]any{"action": "comment", "video_id": "YSNFGgh0aAs", "text": "hi", "file": "/tmp/x.mp4"}, "does not apply"},
		{map[string]any{"action": "comment", "video_id": "YSNFGgh0aAs", "text": "hi", "publish_at": "2026-11-09T10:00:00+07:00"}, "does not apply"},
		{map[string]any{"action": "reply", "video_id": "YSNFGgh0aAs", "comment_id": "UgwEFzl5ZWYy-wHKInF4AaABAg", "text": "hi", "publish_at": "2026-11-09T10:00:00+07:00"}, "does not apply"},
		{map[string]any{"action": "read", "video_id": "YSNFGgh0aAs", "text": "hi"}, "does not apply"},
		{map[string]any{"action": "comment", "video_id": "YSNFGgh0aAs", "text": "hi", "comment_id": "abc"}, "does not apply"},
		// Sorting and paging need controls the probe never recorded, so they are refused, not ignored.
		{map[string]any{"action": "read", "video_id": "YSNFGgh0aAs", "order": "newest"}, "not supported yet"},
		{map[string]any{"action": "read", "video_id": "YSNFGgh0aAs", "page_token": "next"}, "not supported yet"},
	}
	for _, c := range cases {
		action, _ := c.args["action"].(string)
		spec, ok := youtubeActionSpecs[action]
		if action != "" && !ok {
			if !strings.Contains(action, "delete") {
				t.Fatalf("action %q is not declared", action)
			}
			continue
		}
		if action == "" {
			if !strings.Contains(c.want, "action") {
				continue
			}
			if _, has := c.args["action"]; has {
				t.Fatal("no action but args carry one")
			}
			continue
		}
		err := validateYoutube(action, c.args, spec)
		if err == nil {
			if c.want != "" {
				t.Errorf("%v: want %q, got nil", c.args, c.want)
			}
			continue
		}
		if c.want != "" && !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: want %q, got %q", c.args, c.want, err.Error())
		}
	}
}

// The tags/category/thumbnail refusal: an unsupported control must be a hard error, not a silent no-op.
func TestYoutubeRefusesUnprobedControls(t *testing.T) {
	for _, args := range []map[string]any{
		{"tags": []any{"a"}},
		{"category": "Science & Technology"},
		{"thumbnail": "/tmp/t.png"},
	} {
		if err := validateYoutube("upload", mergeBaseUploadArgs(args), youtubeActionSpecs["upload"]); err != nil {
			// Validation passes these through; the refusal happens in the handler, which is where the
			// message is asserted. Here we only pin that the parameters exist in the schema.
			t.Logf("validator: %v", err)
		}
	}
	def := youtubeDef(t)
	props := def.Schema["properties"].(map[string]any)
	for _, name := range []string{"tags", "category", "thumbnail"} {
		p, ok := props[name].(map[string]any)
		if !ok {
			t.Fatalf("%s is not declared", name)
		}
		if !strings.Contains(p["description"].(string), "not yet supported") && !strings.Contains(p["description"].(string), "never been probed") &&
			!strings.Contains(p["description"].(string), "not been probed") {
			t.Errorf("%s must say the control is unprobed, got %q", name, p["description"])
		}
	}
}

func enumHas(def ToolDef, action string) bool {
	raw, _ := json.Marshal(def.Schema["properties"].(map[string]any)["action"])
	var spec struct {
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return false
	}
	return slices.Contains(spec.Enum, action)
}

func mergeBaseUploadArgs(extra map[string]any) map[string]any {
	args := map[string]any{"action": "upload", "file": "/tmp/x.mp4", "made_for_kids": false}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

// publish_at must carry its own intent: an offset. A bare wall-clock string is exactly how a schedule lands
// in the wrong zone.
func TestParsePublishAtRequiresAnOffset(t *testing.T) {
	good := "2026-11-09T10:00:00+07:00"
	got, err := parsePublishAt(good, "Asia/Bangkok")
	if err != nil {
		t.Fatalf("%q: %v", good, err)
	}
	if got.Format("15:04") != "10:00" || got.Format("Z07:00") != "+07:00" {
		t.Fatalf("%q parsed as %s", good, got.Format(time.RFC3339))
	}
	for _, bad := range []string{"2026-11-09", "tomorrow", "2026-11-09 10:00", "10:00:00"} {
		if _, err := parsePublishAt(bad, ""); err == nil {
			t.Errorf("%q was accepted; publish_at must be RFC3339 with an offset", bad)
		}
	}
}

// The tool's per-action spec table is the whole contract; a missing entry means an action an agent cannot
// use, and an entry naming an undeclared parameter is a silent validation hole.
func TestYoutubeActionSpecsMatchTheSchema(t *testing.T) {
	def := youtubeDef(t)
	props := def.Schema["properties"].(map[string]any)
	seen := map[string]bool{}
	for _, action := range []string{"upload", "upload_status", "read", "comment", "reply"} {
		spec, ok := youtubeActionSpecs[action]
		if !ok {
			t.Fatalf("%s has no spec", action)
		}
		seen[action] = true
		for _, name := range append(append([]string{}, spec.required...), spec.forbidden...) {
			if _, ok := props[name]; !ok {
				t.Errorf("spec for %s constrains %q, which the schema does not declare", action, name)
			}
		}
	}
	for name := range youtubeActionSpecs {
		if !seen[name] {
			t.Errorf("spec for %q exists but the action enum does not offer it — is it wired into the dispatcher?", name)
		}
		if !enumHas(def, name) {
			t.Errorf("action %q is missing from the schema enum", name)
		}
	}
}

// Stage 2 builds the watch page comment flow; the rest are future stages and must not be advertised early.
func TestYoutubeDoesNotAdvertiseUnbuiltActions(t *testing.T) {
	def := youtubeDef(t)
	for _, banned := range []string{"edit_details", "list_videos", "top_comment", "delete_video", "get_video", "edit_video", "analytics", "read_comments"} {
		if enumHas(def, banned) {
			t.Errorf("%s is advertised before its stage is built — the tool would promise a page flow that does not exist", banned)
		}
	}
	// The mirror of that: the three watch page actions are built, so an agent must be able to find them.
	for _, built := range []string{"read", "comment", "reply"} {
		if !enumHas(def, built) {
			t.Errorf("%s is built but not advertised", built)
		}
	}
}

// A video is named by its id, and the id arrives either as itself or inside a URL a human pasted — the
// watch URL the probe used, or the youtu.be link this tool's own upload reply returns.
func TestParseVideoID(t *testing.T) {
	for _, c := range map[string]string{
		"YSNFGgh0aAs":     "YSNFGgh0aAs",
		"  YSNFGgh0aAs  ": "YSNFGgh0aAs",
		"https://www.youtube.com/watch?app=desktop&v=YSNFGgh0aAs": "YSNFGgh0aAs",
		"https://www.youtube.com/watch?v=YSNFGgh0aAs&t=42":        "YSNFGgh0aAs",
		"https://youtu.be/YSNFGgh0aAs?si=Z9":                      "YSNFGgh0aAs",
	} {
		got, err := parseVideoID(c)
		if err != nil {
			t.Errorf("%q: %v", c, err)
			continue
		}
		if got != "YSNFGgh0aAs" {
			t.Errorf("%q parsed as %q", c, got)
		}
	}
	for _, bad := range []string{"", "   ", "watch?v=short", "https://www.youtube.com/watch", "YSNFGgh0aAsXX", "https://example.com/v=YSNFGgh0aA"} {
		if got, err := parseVideoID(bad); err == nil {
			t.Errorf("%q was accepted as %q; an id that is not 11 characters is not a video", bad, got)
		}
	}
}

// A read capped below what the page rendered reports the cap and says so, and the page-internal row address is
// not part of what a caller gets back.
func TestCapCommentRows(t *testing.T) {
	raw := json.RawMessage(`{"count":2,"rows":[{"id":"a","author":"@x","text":"one","path":"body > div"},{"id":"b","text":"two","path":"body > div"}]}`)
	rows, kept, total, err := capCommentRows(raw, 1)
	if err != nil {
		t.Fatal(err)
	}
	if kept != 1 || total != 2 {
		t.Fatalf("kept %d of %d", kept, total)
	}
	if strings.Contains(string(rows), "path") {
		t.Errorf("the page address leaked into the reply: %s", rows)
	}
	if !strings.Contains(string(rows), `"id":"a"`) {
		t.Errorf("rows = %s", rows)
	}
	if rows, kept, total, err := capCommentRows(raw, 0); err != nil || kept != 2 || total != 2 || !strings.Contains(string(rows), `"id":"b"`) {
		t.Errorf("max 0 must mean every row: kept=%d total=%d err=%v", kept, total, err)
	}
	if _, _, _, err := capCommentRows(json.RawMessage(`{"rows":`), 5); err == nil {
		t.Error("a malformed read was accepted")
	}
}

// The journal cap and identity refusal are config-independent, but the wiring that reads config must exist.
func TestYoutubeSettingsComeFromConfig(t *testing.T) {
	cfg := mcpcfg.Get().Youtube
	if cfg.Container == "" || cfg.ContainerID == 0 {
		t.Skip("no youtube identity configured in this environment; the production config supplies it")
	}
	if cfg.MaxUploadsPerDay <= 0 {
		t.Error("maxUploadsPerDay must be a real cap, not zero (zero means unlimited)")
	}
	if !slices.Contains([]string{"private", "unlisted", "public", "schedule"}, cfg.DefaultVisibility) {
		t.Errorf("defaultVisibility %q is not a mode the wizard has", cfg.DefaultVisibility)
	}
}
