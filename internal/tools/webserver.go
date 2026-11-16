package tools

// The `webserver` tool: plan 2 §5's agent face on a zen-webserver fleet.
//
// One fat tool with an `action` enum, because that is the house answer to a wide domain surface
// (youtube.go, browser.go) and because the plan says so explicitly (§5.2). What makes this tool
// different from those two is where its input contract comes from: zen-mcp may never import the
// engine, so the schema is a GENERATED FILE ARTIFACT (webserver.schema.json, produced by
// `go run . -fleet www -mode gen-mcp-schema` in zen-webserver, plan 2 §5.3) that is embedded and
// served verbatim. A hand-ported copy would be a second list of endpoints that no build failure
// connects to the first — which is the exact cost the plan exists to remove. The consequence for
// this file is that the schema is authoritative: the action enum, the parameter set and the
// per-parameter enums are all READ from it rather than restated here, so an endpoint change shows
// up as a mismatch against this file's guard table (webserver_test.go) instead of as a tool that
// silently accepts a parameter it ignores.
//
// Three surfaces, and which one an action uses is the security property, not a detail (§5.1):
//
//	admin  — one site's loopback /_zen/api, reached through webserver.Client (port from the
//	         status document, never from an argument)
//	status — the status document read straight off disk; no socket at all, because a `plan` that
//	         went through a running server would describe that server rather than the config (§5.1)
//	fleet  — publish/stop/start/rm/rollback: an intent FILE plus a verdict wait, so no network
//	         listener anywhere accepts a fleet mutation (§5.2)
//
// And because zen-mcp has no auth and no permission model of its own (plan 2 §5 constraint 2,
// §5.2), the gates that matter are enforced HERE, before anything is filed: the CAS hash, and the
// `confirm == "yes"` on destructive actions. The engine does not check confirmations, so this is
// the only place a "remove that site" can be refused.

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/toolresponse"
	"zen-mcp/internal/webserver"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// webserverSchemaJSON is the engine-generated input contract, copied in from
// zen-webserver/testdata/webserver.schema.json. Do not hand-edit it: regenerate it (plan 2 §5.3).
//
//go:embed webserver.schema.json
var webserverSchemaJSON []byte

// webserverSchema is that file as a Go map, ready to serve in tools/list. Parsing happens once at
// package initialisation and a failure is fatal: a schema that does not parse is a broken
// checked-in artifact, and the package's own tests read it the same way, so the failure lands on
// `go test` rather than on a running server.
var webserverSchema = mustWebserverSchema()

func mustWebserverSchema() map[string]any {
	var schema map[string]any
	if err := json.Unmarshal(webserverSchemaJSON, &schema); err != nil {
		panic("tools: webserver.schema.json is not valid JSON: " + err.Error())
	}
	return schema
}

// webserverActionKind is which surface one action drives. The kinds exist so that "the fleet verbs
// touch no socket" (§5.2) is a property a test can assert over a group of actions instead of a
// comment an auditor has to trust.
type webserverActionKind string

const (
	webserverAdminAction  webserverActionKind = "admin"  // one site's loopback /_zen/api
	webserverStatusAction webserverActionKind = "status" // the status document, file read only
	webserverFleetAction  webserverActionKind = "fleet"  // intent file + verdict wait, file only
)

// webserverActionSpec is the per-action contract in the shape youtube.go established: required
// params, params whose only acceptable value is a literal, and the surface the action may use.
// Validation runs before any dispatch, so a malformed request neither opens a connection nor files
// an intent.
//
// The required sets mirror the engine's own ActionSpec `Required` flags (pkg/admin_api/mcpschema.go),
// because the FLAT schema cannot carry them: a property is shared by every action, so per-action
// requirements are runtime rules and this table is the only place they can live.
type webserverActionSpec struct {
	kind webserverActionKind
	// required must arrive non-empty.
	required []string
	// presentKeys must arrive at all, "" allowed. A publish's expect_sha256 is in this set: ""
	// means "there must be no incumbent config" (pkg/fleet/intent.go), which is a decision the
	// caller has to state rather than forget.
	presentKeys []string
	// exact names a param whose only accepted value is the literal, checked case-sensitively:
	// "y", "YES" and "true" are not the confirmation this gate is asking for.
	exact map[string]string
	// endpoint is the admin suffix for the admin kind, recorded so a test can prove no fleet
	// action has one.
	endpoint string
}

var webserverActionSpecs = map[string]webserverActionSpec{
	// --- status document reads: never an HTTP round trip (plan 2 §5.1) ---
	"sites":   {kind: webserverStatusAction},
	"status":  {kind: webserverStatusAction},
	"plan":    {kind: webserverStatusAction},
	"intents": {kind: webserverStatusAction},

	// --- one site's loopback admin API (plan 2 §5.4: doctor is the verify gate) ---
	"doctor":        {kind: webserverAdminAction, required: []string{"site"}, endpoint: "doctor"},
	"catalog":       {kind: webserverAdminAction, required: []string{"site"}, endpoint: "catalog"},
	"list":          {kind: webserverAdminAction, required: []string{"site"}, endpoint: "list"},
	"get":           {kind: webserverAdminAction, required: []string{"site", "slug"}, endpoint: "get"},
	"save":          {kind: webserverAdminAction, required: []string{"site", "slug"}, endpoint: "save"},
	"revision_list": {kind: webserverAdminAction, required: []string{"site", "slug"}, endpoint: "revision/list"},
	"media_upload":  {kind: webserverAdminAction, required: []string{"site", "path"}, endpoint: "media/upload"},
	"rename_slug":   {kind: webserverAdminAction, required: []string{"site", "old_slug", "new_slug"}, endpoint: "rename_slug"},
	// A URL move that is not a rename. The engine refuses an off-site destination, and says why in
	// the error, so nothing here needs to second-guess the value: the one thing worth stating is
	// that this tool is not the guard (plan 2 A-12's open-redirect rule lives server-side, where
	// every writer of the table can be held to it).
	"redirect": {kind: webserverAdminAction, required: []string{"site", "from", "to"}, endpoint: "redirect"},
	// An article delete is irreversible and the engine admits it without asking, so the literal is
	// required here — same reasoning as `rm` below.
	"delete": {kind: webserverAdminAction, required: []string{"site", "slug"}, endpoint: "delete", exact: map[string]string{"confirm": "yes"}},

	// --- fleet mutations: intent files only, never an endpoint (§5.2) ---
	"publish": {
		kind:        webserverFleetAction,
		required:    []string{"site", "config_file"},
		presentKeys: []string{"expect_sha256"},
	},
	"stop": {
		kind:     webserverFleetAction,
		required: []string{"site", "expect_sha256"},
	},
	"start": {
		kind:     webserverFleetAction,
		required: []string{"site", "expect_sha256"},
	},
	// The engine does not check `confirm` on an rm either (plan 2 §5.2's whole point about
	// zen-mcp's missing permission model), so this refusal is the ONLY gate between "rm" and a
	// config leaving the fleet. It is enforced here, in the layer that can enforce it.
	"rm": {
		kind:     webserverFleetAction,
		required: []string{"site", "expect_sha256"},
		exact:    map[string]string{"confirm": "yes"},
	},
	"rollback": {
		kind:     webserverFleetAction,
		required: []string{"site", "expect_sha256"},
	},
}

// webserverVerdict is what a fleet action reports. The four values are the plan's vocabulary:
// `inconsistent` (the engine answered, but not with `applied` — a failed swap or an expired intent
// both leave the fleet in a state the caller must look at) and `no_verdict` (nobody answered at
// all, which is a different problem and must never be collapsed into "rejected", plan 2 A-7/A-19).
const (
	webserverVerdictApplied      = "applied"
	webserverVerdictRejected     = "rejected"
	webserverVerdictInconsistent = "inconsistent"
	webserverVerdictNoVerdict    = "no_verdict"
)

// webserverMaxConfigBytes is the engine's staged-config ceiling (pkg/fleet maxIntentConfigBytes,
// 1 MiB) restated here so an over-large YAML is refused on read rather than after being staged,
// which is where the engine would reject it. One number governs both sides by agreement, and a
// site YAML is a few KB.
const webserverMaxConfigBytes = 1 << 20

// webserverMaxUploadBytes is the ceiling api/media/upload itself applies (r.ParseMultipartForm(32
// << 20)); reading more than that off disk would only produce a 400 after costing a whole file.
const webserverMaxUploadBytes = 32 << 20

// webserverAdminPrefix is the engine's admin mount point, spelled the way Client.Call spells it
// (plan 2 §5.1's `base + "/_zen/api" + suffix` idiom). It exists only for the one call the client
// cannot express — see webserverMediaUpload.
const webserverAdminPrefix = "/_zen/api"

// webserverClientFor builds the admin client for one fleet. A package-level variable for exactly
// the reason youtube.go makes its browser one: a test has to be able to watch whether an action
// opens a connection at all (TestNoFleetActionUsesHTTP), and no other seam shows the truth.
var webserverClientFor = func(statusPath string, timeout time.Duration) *webserver.Client {
	return webserver.New(statusPath, timeout)
}

func defWebserver(workspace string, deps Deps) ToolDef {
	description, _ := webserverSchema["description"].(string)
	if description == "" {
		description = "Inspect, maintain and publish sites on the zen-webserver fleet."
	}
	return ToolDef{
		Name:  "webserver",
		Title: "Zen Webserver Fleet & Content",
		// The action sentence is the house contract (youtube.go): an agent that free-texts this
		// parameter gets a refusal naming the set, never a guess.
		Description: description + " Action MUST be an action from the enumerated set; dispatch strictly by it, never free text.",
		// The generated schema IS the input contract (plan 2 §5.3); nothing here re-describes it.
		Schema: webserverSchema,
		// Fleet verbs wait for the reconciler's verdict (up to webserver.intentDeadlineMs, 30 s by
		// default), which the default 30 s tool context cannot outlast.
		LongRunning: true,
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleWebserver(ctx, workspace, deps, req), nil
		},
	}
}

// HandleWebserver validates against the generated schema, then dispatches. Every refusal before the
// switch is deliberate: an unknown action, an unknown key or a missing parameter must cost nothing
// — no port opened, no intent filed, no config body read.
func HandleWebserver(ctx context.Context, workspace string, deps Deps, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()

	action := argString(args, "action")
	if action == "" {
		return toolresponse.WrapErrorWithContext(ctx, "webserver", fmt.Errorf("action is required; this tool does: %s", strings.Join(webserverActions(), ", ")), start)
	}
	// Both checks, on purpose: the embedded enum is what the caller was told, and this switch is
	// what this build can actually do. webserver_test.go fails the two apart (plan 2 A-10/A-14: a
	// missing case is a runtime error, so it is made a test failure instead).
	if !webserverSchemaHasAction(action) {
		return toolresponse.WrapErrorWithContext(ctx, "webserver",
			fmt.Errorf("unknown action %q; this tool does: %s", action, strings.Join(webserverActions(), ", ")), start)
	}
	spec, ok := webserverActionSpecs[action]
	if !ok {
		return toolresponse.WrapErrorWithContext(ctx, "webserver",
			fmt.Errorf("action %q is in the schema but has no contract in this build: it would be dispatched blind", action), start)
	}
	if err := validateWebserverArgs(action, args, spec); err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "webserver", err, start)
	}

	target, err := webserverTargetFor(workspace)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "webserver", err, start)
	}
	return webserverDispatch(ctx, webserverInvocation{
		Action:    action,
		Args:      args,
		Spec:      spec,
		Target:    target,
		Start:     start,
		Workspace: workspace,
	})
}

// webserverInvocation is one validated call: the action, the arguments, and the fleet this server
// is configured to act on. A validated call carries no path or port of its own (§5.2), which is
// why every handler below reads target rather than args.
type webserverInvocation struct {
	Action    string
	Args      map[string]any
	Spec      webserverActionSpec
	Target    webserverTarget
	Start     time.Time
	Workspace string
}

// webserverDispatch is the switch the schema's enum is checked against. Every case is one action;
// the fallthrough is unreachable while webserver_test.go keeps this list and the enum together.
func webserverDispatch(ctx context.Context, inv webserverInvocation) *mcp.CallToolResult {
	switch inv.Action {
	case "sites":
		return webserverSites(ctx, inv)
	case "status":
		return webserverStatusDoc(ctx, inv)
	case "plan":
		return webserverPlan(ctx, inv)
	case "intents":
		return webserverIntents(ctx, inv)
	case "doctor":
		return webserverAdminCall(ctx, inv)
	case "catalog":
		return webserverAdminCall(ctx, inv)
	case "list":
		return webserverAdminCall(ctx, inv)
	case "get":
		return webserverAdminCall(ctx, inv)
	case "save":
		return webserverAdminCall(ctx, inv)
	case "delete":
		return webserverAdminCall(ctx, inv)
	case "media_upload":
		return webserverAdminCall(ctx, inv)
	case "redirect":
		return webserverAdminCall(ctx, inv)
	case "revision_list":
		return webserverAdminCall(ctx, inv)
	case "rename_slug":
		return webserverAdminCall(ctx, inv)
	case "publish":
		return webserverFleetVerb(ctx, inv)
	case "stop":
		return webserverFleetVerb(ctx, inv)
	case "start":
		return webserverFleetVerb(ctx, inv)
	case "rm":
		return webserverFleetVerb(ctx, inv)
	case "rollback":
		return webserverFleetVerb(ctx, inv)
	}
	return toolresponse.WrapErrorWithContext(ctx, "webserver",
		fmt.Errorf("action %q is declared but not wired", inv.Action), inv.Start)
}

// ---- schema-derived guards -------------------------------------------------------------------

// webserverActions returns the action enum in schema order.
func webserverActions() []string {
	raw, _ := json.Marshal(webserverSchema["properties"])
	var props struct {
		Action struct {
			Enum []string `json:"enum"`
		} `json:"action"`
	}
	if err := json.Unmarshal(raw, &props); err != nil {
		return nil
	}
	return props.Action.Enum
}

func webserverSchemaHasAction(action string) bool {
	for _, a := range webserverActions() {
		if a == action {
			return true
		}
	}
	return false
}

// webserverSchemaParams is the declared parameter set, `action` included. It is the allowlist the
// unknown-key guard runs against.
func webserverSchemaParams() map[string]any {
	props, _ := webserverSchema["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	return props
}

// validateWebserverArgs is the browser-free half of the contract, and here the socket- and
// file-free half: the unknown-key rule first, then the per-action requirements.
func validateWebserverArgs(action string, args map[string]any, spec webserverActionSpec) error {
	// The bridge does not enforce additionalProperties:false, so an argument that is not in the
	// schema would otherwise be silently ignored — the shape a caller could use to smuggle a
	// base_url, a status_file or a config_path past the rule that fleet paths come from config
	// alone (plan 2 §5.2). Refusing the key is what makes that rule enforceable.
	declared := webserverSchemaParams()
	for name := range args {
		if _, ok := declared[name]; !ok {
			return fmt.Errorf("webserver %s: %q is not a declared parameter; this tool takes its fleet directory and status file from config.json, never from an argument (known parameters: %s)",
				action, name, strings.Join(webserverParamNames(declared), ", "))
		}
	}
	for _, name := range spec.required {
		if !hasArg(args, name) {
			return fmt.Errorf("webserver %s: %s is required", action, name)
		}
	}
	for _, name := range spec.presentKeys {
		if _, ok := args[name]; !ok {
			return fmt.Errorf("webserver %s: %s is required (an empty value is a real answer: it means the config must not exist yet)", action, name)
		}
	}
	for name, want := range spec.exact {
		got := argString(args, name)
		if got != want {
			return fmt.Errorf("webserver %s: %s must be the literal %s — the engine does not check confirmations, so this tool is the only thing that can refuse this run", action, name, want)
		}
	}
	// Parameter enums come from the schema, so `confirm` and `sort` are enforced on the same
	// words the engine generated rather than on a second copy of them.
	for name, raw := range declared {
		prop, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		list, ok := prop["enum"].([]any)
		if !ok || len(list) == 0 {
			continue
		}
		v, ok := args[name].(string)
		if !ok || v == "" {
			continue
		}
		allowed := make([]string, 0, len(list))
		found := false
		for _, item := range list {
			s, _ := item.(string)
			allowed = append(allowed, s)
			if s == v {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("webserver %s: %s must be one of %s", action, name, strings.Join(allowed, ", "))
		}
	}
	return nil
}

func webserverParamNames(props map[string]any) []string {
	out := make([]string, 0, len(props))
	for name := range props {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ---- configuration --------------------------------------------------------------------------

// webserverTarget is the resolved, per-workspace view of the `webserver` config block. The fleet
// directory and the status path are config-only values: the schema has no property that could
// carry either, and the unknown-key guard is what proves it (plan 2 §5.2).
type webserverTarget struct {
	FleetDir       string
	StatusPath     string
	CallTimeout    time.Duration
	IntentDeadline time.Duration
}

func webserverTargetFor(workspace string) (webserverTarget, error) {
	var block mcpcfg.WebserverConfig
	if cfg := mcpcfg.For(workspace); cfg != nil {
		block = cfg.Webserver
	}
	target := webserverTarget{
		FleetDir:       strings.TrimSpace(block.FleetDir),
		StatusPath:     strings.TrimSpace(block.StatusFile),
		CallTimeout:    durationOr(block.CallTimeoutMs, mcpcfg.DefaultWebserverCallTimeoutMs),
		IntentDeadline: durationOr(block.IntentDeadlineMs, mcpcfg.DefaultWebserverIntentDeadlineMs),
	}
	if target.StatusPath == "" && target.FleetDir != "" {
		target.StatusPath = webserver.StatusPathFor(target.FleetDir)
	}
	if target.StatusPath == "" {
		return target, fmt.Errorf("no fleet is configured: set webserver.fleetDir (or webserver.statusFile) in config.json — this tool never takes a fleet directory or a status path from its caller")
	}
	return target, nil
}

// durationOr falls back to the config default when a tier was installed without going through
// mcpcfg.Load (tests, and any caller that stores a ZenConfig directly), so a zero here is never a
// zero-length timeout.
func durationOr(ms, fallbackMS int) time.Duration {
	if ms <= 0 {
		ms = fallbackMS
	}
	if ms <= 0 {
		ms = 10000
	}
	return time.Duration(ms) * time.Millisecond
}

// ---- status-document actions (file reads, never an HTTP round trip) ----------------------------

// webserverStatusRead reads the document and its age. A stale document is returned WITH a warning
// rather than refused, because these actions are how an agent diagnoses a dead reconciler — and
// refusing to read it would hide the evidence. The client-side gate stays strict (Client.Resolve
// refuses to build a URL from a stale document); this is the report, not the target.
func webserverStatusRead(target webserverTarget) (webserver.Status, time.Duration, string, error) {
	st, err := webserver.Read(target.StatusPath)
	if err != nil {
		return webserver.Status{}, 0, "", err
	}
	age, staleErr := st.StaleFor(time.Now())
	note := ""
	if staleErr != nil {
		note = staleErr.Error()
	}
	return st, age, note, nil
}

func webserverSites(ctx context.Context, inv webserverInvocation) *mcp.CallToolResult {
	st, age, note, err := webserverStatusRead(inv.Target)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "webserver", err, inv.Start)
	}
	sites := make([]map[string]any, 0, len(st.Sites))
	for _, s := range st.Sites {
		sites = append(sites, map[string]any{
			"id":                 s.ID,
			"site_id":            s.SiteID,
			"config_path":        s.ConfigPath,
			"port":               s.Port,
			"expose":             s.Expose,
			"state":              s.State,
			"applied_sha256":     s.AppliedSHA256,
			"applied_request_id": s.AppliedReqID,
			"generation":         s.Generation,
			"last_error":         s.LastErr,
			"next_attempt":       s.NextAttempt,
			"last_ok":            s.LastOK,
		})
	}
	return toolresponse.WrapSuccess(ctx, "webserver", map[string]any{
		"action":        inv.Action,
		"status_file":   inv.Target.StatusPath,
		"generated_at":  st.GeneratedAt,
		"age_seconds":   int64(age.Seconds()),
		"stale_warning": note,
		"boot_id":       st.BootID,
		"pass_seq":      st.PassSeq,
		"sites":         sites,
	}, inv.Start)
}

func webserverStatusDoc(ctx context.Context, inv webserverInvocation) *mcp.CallToolResult {
	st, age, note, err := webserverStatusRead(inv.Target)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "webserver", err, inv.Start)
	}
	return toolresponse.WrapSuccess(ctx, "webserver", map[string]any{
		"action":                inv.Action,
		"status_file":           inv.Target.StatusPath,
		"generated_at":          st.GeneratedAt,
		"age_seconds":           int64(age.Seconds()),
		"stale_warning":         note,
		"boot_id":               st.BootID,
		"pid":                   st.PID,
		"pass_seq":              st.PassSeq,
		"reconcile_interval_ms": st.IntervalMS,
		"sites":                 st.Sites,
		"desired":               st.Desired,
		"recent_requests":       st.RecentRequests,
		"pending":               st.Staged,
	}, inv.Start)
}

// webserverPlan diffs what the fleet directory holds against what the reconciler has recorded —
// off the status document and the directory listing, because a plan computed by a running server
// would describe that server rather than the config (plan 2 §5.1).
func webserverPlan(ctx context.Context, inv webserverInvocation) *mcp.CallToolResult {
	st, age, note, err := webserverStatusRead(inv.Target)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "webserver", err, inv.Start)
	}
	if inv.Target.FleetDir == "" {
		return toolresponse.WrapErrorWithContext(ctx, "webserver",
			fmt.Errorf("plan needs webserver.fleetDir: it lists the site YAMLs on disk"), inv.Start)
	}
	entries, err := os.ReadDir(inv.Target.FleetDir)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "webserver",
			fmt.Errorf("webserver: read the fleet directory %s: %w", inv.Target.FleetDir, err), inv.Start)
	}

	type planned struct {
		Config         string              `json:"config_path"`
		OnDisk         bool                `json:"on_disk"`
		StoredSHA256   string              `json:"on_disk_sha256,omitempty"`
		DesiredVerdict string              `json:"desired_verdict,omitempty"`
		DesiredSHA256  string              `json:"desired_sha256,omitempty"`
		Findings       []webserver.Finding `json:"findings,omitempty"`
		Drifted        bool                `json:"drifted,omitempty"`
		SiteID         string              `json:"site_id,omitempty"`
		State          string              `json:"state,omitempty"`
		AppliedSHA256  string              `json:"applied_sha256,omitempty"`
		Unmanaged      bool                `json:"unmanaged,omitempty"`
	}

	liveByPath := map[string]webserver.SiteState{}
	for _, s := range st.Sites {
		liveByPath[s.ConfigPath] = s
	}
	seen := map[string]bool{}
	var files []planned
	for _, e := range entries {
		if e.IsDir() || !webserverIsConfig(e.Name()) {
			continue
		}
		path := filepath.Join(inv.Target.FleetDir, e.Name())
		seen[path] = true
		entry := planned{Config: path, OnDisk: true}
		if sum, herr := webserverHashFile(path); herr == nil {
			entry.StoredSHA256 = sum
		} else {
			entry.Drifted = true
		}
		if d, ok := st.DesiredFor(path); ok {
			entry.DesiredVerdict = d.Verdict
			entry.DesiredSHA256 = d.SHA256
			entry.Findings = d.Findings
			// drifted is the plan's point: the file on disk is not what the reconciler last saw,
			// which is what A-14 (an agent editing YAML around the intent protocol) looks like.
			entry.Drifted = d.SHA256 != "" && entry.StoredSHA256 != d.SHA256
		} else {
			// Absent from `desired` entirely: not rejected, simply not managed (plan 2 A-14's
			// `source: unmanaged`), including a parked `.disabled` YAML.
			entry.Unmanaged = true
		}
		if s, ok := liveByPath[path]; ok {
			entry.SiteID = s.ID
			entry.State = s.State
			entry.AppliedSHA256 = s.AppliedSHA256
		}
		files = append(files, entry)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Config < files[j].Config })

	// The other half of the diff: something the scan saw that is no longer on disk.
	var missing []string
	for _, d := range st.Desired {
		if !seen[d.ConfigPath] {
			missing = append(missing, d.ConfigPath)
		}
	}
	sort.Strings(missing)

	return toolresponse.WrapSuccess(ctx, "webserver", map[string]any{
		"action":                   inv.Action,
		"fleet_dir":                inv.Target.FleetDir,
		"generated_at":             st.GeneratedAt,
		"age_seconds":              int64(age.Seconds()),
		"stale_warning":            note,
		"configs":                  files,
		"desired_absent_from_disk": missing,
	}, inv.Start)
}

// webserverIntents lists the intake directory: what is waiting, what the engine still holds, and
// what has been archived — all from the filesystem, since the intent files ARE the transport.
func webserverIntents(ctx context.Context, inv webserverInvocation) *mcp.CallToolResult {
	st, _, note, err := webserverStatusRead(inv.Target)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "webserver", err, inv.Start)
	}
	if inv.Target.FleetDir == "" {
		return toolresponse.WrapErrorWithContext(ctx, "webserver",
			fmt.Errorf("intents needs webserver.fleetDir: the intent files live under it"), inv.Start)
	}
	dir := webserver.RequestsPathFor(inv.Target.FleetDir)
	type listed struct {
		File        string `json:"file"`
		State       string `json:"state,omitempty"`
		Action      string `json:"action,omitempty"`
		SiteID      string `json:"site_id,omitempty"`
		RequestedAt string `json:"requested_at,omitempty"`
		DeadlineAt  string `json:"deadline_at,omitempty"`
		Expired     bool   `json:"expired,omitempty"`
		Verdict     string `json:"verdict,omitempty"`
		Problem     string `json:"problem,omitempty"`
	}
	list := func(sub, state string) []listed {
		entries, lerr := os.ReadDir(filepath.Join(dir, sub))
		if lerr != nil {
			return nil
		}
		out := make([]listed, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".json")
			item := listed{File: filepath.Join(dir, sub, e.Name()), State: state, Verdict: "unanswered"}
			intent, ierr := webserver.ReadIntent(item.File)
			if ierr != nil {
				// A file this cannot parse is still a file the reconciler will consume, so it is
				// listed with the problem rather than skipped.
				item.Problem = ierr.Error()
				out = append(out, item)
				continue
			}
			item.Action = intent.Action
			item.SiteID = intent.SiteID
			item.RequestedAt = intent.RequestedAt.Format(time.RFC3339)
			item.DeadlineAt = intent.DeadlineAt.Format(time.RFC3339)
			item.Expired = intent.Expired(time.Now())
			if res, ok := webserver.LatestRequest(st, name); ok {
				item.Verdict = res.Result
				if res.Error != "" {
					item.Problem = res.Error
				}
			}
			out = append(out, item)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
		return out
	}

	pending := list("", "pending")
	archived := append(list("done", "archived"), list("failed", "failed")...)
	noteStr := ""
	switch {
	case note != "":
		noteStr = note
	case len(pending) == 0 && len(archived) == 0:
		noteStr = "no intent files under " + dir + " (the fleet has received none, or the reconciler has consumed and archived every one)"
	}
	return toolresponse.WrapSuccess(ctx, "webserver", map[string]any{
		"action":         inv.Action,
		"requests_dir":   dir,
		"pending":        pending,
		"archived":       archived,
		"engine_pending": st.Staged,
		"note":           noteStr,
	}, inv.Start)
}

// webserverIsConfig reports whether one fleet-directory entry is a site YAML, including a parked
// one (`stop` renames to .disabled, so a plan that ignored those would hide a stopped site).
func webserverIsConfig(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"),
		strings.HasSuffix(lower, ".yaml.disabled"), strings.HasSuffix(lower, ".yml.disabled"):
		return !strings.HasPrefix(name, ".")
	default:
		return false
	}
}

// ---- admin actions (one site's loopback /_zen/api) --------------------------------------------

// webserverAdminCall runs one content/read action. The URL is never assembled from an argument:
// Client.Resolve turns `site` into a loopback base from the status document, and Call appends the
// endpoint this build knows about (plan 2 §5.1, A-18).
func webserverAdminCall(ctx context.Context, inv webserverInvocation) *mcp.CallToolResult {
	client := webserverClientFor(inv.Target.StatusPath, inv.Target.CallTimeout)
	site := argString(inv.Args, "site")
	var (
		raw json.RawMessage
		err error
	)
	switch inv.Action {
	case "doctor":
		raw, err = client.Doctor(ctx, site, webserverBoolArg(inv.Args, "deep"))
	case "catalog":
		raw, err = client.Catalog(ctx, site)
	case "list":
		raw, err = client.List(ctx, site, webserverListParams(inv.Args))
	case "get":
		raw, err = client.Get(ctx, site, argString(inv.Args, "slug"))
	case "save":
		raw, err = client.Call(ctx, site, "save", webserverSaveBody(inv.Args))
	case "delete":
		raw, err = client.Call(ctx, site, "delete", map[string]any{"slug": argString(inv.Args, "slug")})
	case "rename_slug":
		raw, err = client.Call(ctx, site, "rename_slug", map[string]any{
			"old_slug": argString(inv.Args, "old_slug"),
			"new_slug": argString(inv.Args, "new_slug"),
		})
	case "redirect":
		// `permanent` is only sent when the caller said so: the endpoint's own default is a
		// permanent move, and volunteering the key would turn "record a redirect" into "record a
		// temporary redirect" whenever the CLI stringified an absent flag.
		body := map[string]any{"from": argString(inv.Args, "from"), "to": argString(inv.Args, "to")}
		if hasArg(inv.Args, "permanent") {
			body["permanent"] = webserverBoolArg(inv.Args, "permanent")
		}
		raw, err = client.Call(ctx, site, "redirect", body)
	case "revision_list":
		q := url.Values{"slug": []string{argString(inv.Args, "slug")}}
		raw, err = client.Call(ctx, site, "revision/list?"+q.Encode(), nil)
	case "media_upload":
		raw, err = webserverMediaUpload(ctx, client, site, inv.Args)
	default:
		err = fmt.Errorf("action %q is not an admin action", inv.Action)
	}
	if err != nil {
		// The admin code is the actionable part (plan 2 A-15): `unknown_admin_endpoint` means do
		// not retry, and a caller that cannot see the code cannot tell that from a site being down.
		if code := webserver.CodeOf(err); code != "" {
			return toolresponse.WrapErrorWithContext(ctx, "webserver",
				fmt.Errorf("%w (admin code %s)", err, code), inv.Start)
		}
		return toolresponse.WrapErrorWithContext(ctx, "webserver", err, inv.Start)
	}
	return toolresponse.WrapSuccess(ctx, "webserver", map[string]any{
		"action":   inv.Action,
		"site":     site,
		"endpoint": inv.Spec.endpoint,
		"data":     raw,
	}, inv.Start)
}

// webserverListParams carries the caller's pagination through verbatim: `list` is the engine's
// query surface, and this tool has no opinion about which articles an agent wants.
func webserverListParams(args map[string]any) url.Values {
	q := url.Values{}
	for _, name := range []string{"category", "slug_prefix", "search", "created_after", "sort", "fields"} {
		if v := argString(args, name); v != "" {
			q.Set(name, v)
		}
	}
	// page and per_page are declared integer in the schema precisely because the CLI layer
	// stringifies flags (plan 2 §5.3), so both shapes have to be accepted here.
	for _, name := range []string{"page", "per_page"} {
		if n, ok := webserverIntArg(args[name]); ok && n > 0 {
			q.Set(name, strconv.Itoa(n))
		}
	}
	return q
}

// webserverSaveBody is the article this call writes: a FULL document, because api/save replaces the
// row it is given (pkg/admin_api/handlers.go HandleSave decodes a storage.Article and saves it).
// Only the keys the caller sent are transmitted, and the schema's description says the rest of the
// document is not preserved, so the reply carries no ambiguity about what was written.
func webserverSaveBody(args map[string]any) map[string]any {
	body := map[string]any{"slug": argString(args, "slug")}
	for _, name := range []string{"title", "content", "description", "category", "image_url", "tags", "attributes", "author"} {
		if v, ok := args[name]; ok {
			if s := argString(args, name); s != "" {
				body[name] = s
			} else if b, isBool := v.(bool); isBool {
				body[name] = b
			}
		}
	}
	return body
}

// webserverMediaUpload posts one file as multipart/form-data, which is the shape api/media/upload
// parses (HandleMediaUpload reads FormFile("file")); Client.Call speaks JSON bodies only, so this is
// the one admin call this tool assembles itself. It still resolves the base through Client.Resolve,
// so the port comes from the status document and a non-loopback host is refused exactly as it is
// everywhere else (plan 2 §5.1) — the multipart is the difference in encoding, not in trust.
func webserverMediaUpload(ctx context.Context, client *webserver.Client, siteID string, args map[string]any) (json.RawMessage, error) {
	path := argString(args, "path")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("webserver: open %s for upload: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("webserver: stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("webserver: %s is not a regular file", path)
	}
	if info.Size() > webserverMaxUploadBytes {
		return nil, fmt.Errorf("webserver: %s is %d bytes, over the %d the site will accept", path, info.Size(), webserverMaxUploadBytes)
	}
	base, err := client.Resolve(ctx, siteID)
	if err != nil {
		return nil, err
	}

	// The part carries the name the engine stores (`as`, defaulting to the file's own name) and a
	// content type read from that name, because SaveAndRegisterAsset derives the stored extension
	// from the part header.
	name := argString(args, "as")
	if name == "" {
		name = filepath.Base(path)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return nil, fmt.Errorf("webserver: build the upload form: %w", err)
	}
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		if setter, ok := part.(interface{ Set(h http.Header) }); ok {
			setter.Set(http.Header{"Content-Type": []string{ct}})
		}
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("webserver: read %s into the form: %w", path, err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("webserver: close the upload form: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+webserverAdminPrefix+"/media/upload", &body)
	if err != nil {
		return nil, fmt.Errorf("webserver: build the upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webserver: upload to site %s: %w", siteID, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("webserver: read the upload reply: %w", err)
	}
	if resp.StatusCode >= 400 {
		// Same decoding as the client's, so a failure is an APIError with a code whichever route
		// carried it.
		var ae webserver.APIError
		if json.Unmarshal(raw, &ae) != nil || (ae.Code == "" && ae.Message == "") {
			ae = webserver.APIError{Message: strings.TrimSpace(string(raw))}
		}
		ae.Status = resp.StatusCode
		if ae.Message == "" {
			ae.Message = http.StatusText(resp.StatusCode)
		}
		return nil, &ae
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("webserver: the site answered the upload with %d and an empty body", resp.StatusCode)
	}
	return json.RawMessage(raw), nil
}

// ---- fleet verbs (intent file + verdict wait, never a socket) ---------------------------------

// webserverFleetReply is what a fleet verb reports: the verdict, the request id, and on `applied`
// the site's applied hash and generation (§5.2's acceptance line). No config body is ever
// included — a full YAML in a tool response (or in the request log that produced it) is a config
// leak, which is why the config travels as a staged FILE and only its hash is echoed (A-3).
type webserverFleetReply struct {
	Action       string `json:"action"`
	Site         string `json:"site"`
	ConfigPath   string `json:"config_path,omitempty"`
	RequestID    string `json:"request_id"`
	IntentFile   string `json:"intent_file"`
	StagedConfig string `json:"staged_config,omitempty"`
	StagedSHA256 string `json:"staged_sha256,omitempty"`
	Verdict      string `json:"verdict"`
	// EngineResult is the ledger's own word (applied|failed|rejected|expired), kept beside the
	// tool's verdict so `inconsistent` never hides which of them the reconciler actually wrote.
	EngineResult  string              `json:"engine_result,omitempty"`
	Error         string              `json:"error,omitempty"`
	Findings      []webserver.Finding `json:"findings,omitempty"`
	State         string              `json:"state,omitempty"`
	AppliedSHA256 string              `json:"applied_sha256,omitempty"`
	Generation    int64               `json:"generation,omitempty"`
	Note          string              `json:"note,omitempty"`
}

func webserverFleetVerb(ctx context.Context, inv webserverInvocation) *mcp.CallToolResult {
	target := inv.Target
	if target.FleetDir == "" {
		// An intent is a FILE under <fleet>/.zen/requests, so a status file reached over a path
		// alone is not enough to mutate anything. There is no fallback and none is wanted (§5.2).
		return toolresponse.WrapErrorWithContext(ctx, "webserver",
			fmt.Errorf("%s needs webserver.fleetDir: a fleet mutation is an intent file written under it", inv.Action), inv.Start)
	}
	site := argString(inv.Args, "site")
	expect := argString(inv.Args, "expect_sha256")

	st, err := webserver.Read(target.StatusPath)
	if err != nil {
		return toolresponse.WrapErrorWithContext(ctx, "webserver", err, inv.Start)
	}
	// Status.Site reports a miss as an error; the fleet branches below care about the boolean,
	// and the message must quote THAT error rather than the (nil) read error above.
	resolved, resolveErr := st.Site(site)
	resolves := resolveErr == nil

	// The config path is the status document's, not a guess: it is the engine's own ground truth,
	// and the intent's containment check refuses a path outside the fleet directory anyway.
	configPath := resolved.ConfigPath
	if !resolves {
		if inv.Action != "publish" {
			return toolresponse.WrapErrorWithContext(ctx, "webserver",
				fmt.Errorf("webserver %s: site %q is not in the fleet status document, so there is nothing to %s (%v)", inv.Action, site, inv.Action, resolveErr), inv.Start)
		}
		if expect != "" {
			// A create whose incumbent cannot be named: the caller believes a config exists, and
			// the document says it does not. That is a CAS question, not a permission one.
			return toolresponse.WrapErrorWithContext(ctx, "webserver",
				fmt.Errorf("webserver publish: cas_conflict — site %q is not in the fleet status document, so its live hash cannot be checked against expect_sha256 %s; send expect_sha256 \"\" to state that the config must not exist yet", site, expect), inv.Start)
		}
	}

	// The CAS, checked locally before anything is filed. The engine enforces it too (pkg/fleet
	// applyPublish/applyRemove/applyRollback), but applyPark (stop/start) does NOT, so for those
	// this check is the only compare-and-swap in the system: it is what stops "stop the site I
	// think I'm looking at" from parking a config that was recreated since the caller read it.
	if live, liveErr := webserverLiveConfigPath(target.FleetDir, resolved, site, resolves); liveErr == nil {
		if sum, herr := webserverHashFile(live); herr != nil {
			return toolresponse.WrapErrorWithContext(ctx, "webserver", fmt.Errorf("webserver %s: cannot hash the live config %s: %w", inv.Action, live, herr), inv.Start)
		} else if sum != expect {
			return toolresponse.WrapErrorWithContext(ctx, "webserver",
				fmt.Errorf("webserver %s: cas_conflict — the live config %s hashes %s but expect_sha256 said %s; re-read it and re-submit. Nothing was filed", inv.Action, live, sum, expect), inv.Start)
		}
	} else if inv.Action != "publish" || expect != "" {
		return toolresponse.WrapErrorWithContext(ctx, "webserver", liveErr, inv.Start)
	}

	reply := webserverFleetReply{Action: inv.Action, Site: site, ConfigPath: configPath}

	// A publish carries its YAML as a staged file: the hash the caller computed is then the hash of
	// the bytes the reconciler will read, with no JSON escaping in between (and no config body in
	// the intent's request log, A-3).
	var staged []byte
	if inv.Action == "publish" {
		raw, rerr := webserverReadConfig(argString(inv.Args, "config_file"))
		if rerr != nil {
			return toolresponse.WrapErrorWithContext(ctx, "webserver", rerr, inv.Start)
		}
		staged = raw
	}

	siteID := resolved.ID
	if !resolves {
		// A create has no record to take the stem from, and the engine derives `<fleet>/<id>.yaml`
		// from SiteID when ConfigPath is empty — so the caller's id is passed through untouched and
		// a name it cannot hold as a file is refused there rather than sanitised here.
		siteID = site
	}

	requests := webserver.RequestsPathFor(target.FleetDir)
	id := webserver.NewRequestID()
	intent := webserver.Intent{
		RequestID:  id,
		Action:     inv.Action,
		SiteID:     siteID,
		ConfigPath: configPath,
		// The stem, not settings.site_id: the engine keys its site table and its snapshot
		// directories on the config filename (pkg/fleet idFromPath), so an id that sorts under a
		// different name would roll back the wrong history.
		ExpectSHA256: expect,
		SnapshotID:   argString(inv.Args, "snapshot_id"),
		RequestedAt:  time.Now(),
		DeadlineAt:   time.Now().Add(target.IntentDeadline),
	}
	if len(staged) > 0 {
		path, serr := webserver.StageConfig(requests, id, staged)
		if serr != nil {
			return toolresponse.WrapErrorWithContext(ctx, "webserver", fmt.Errorf("webserver publish: stage the config: %w", serr), inv.Start)
		}
		intent.ConfigFile = path
		reply.StagedConfig = path
		reply.StagedSHA256 = fmt.Sprintf("%x", sha256.Sum256(staged))
	}

	file, werr := webserver.WriteIntent(requests, intent)
	if werr != nil {
		return toolresponse.WrapErrorWithContext(ctx, "webserver", fmt.Errorf("webserver %s: file the intent: %w", inv.Action, werr), inv.Start)
	}
	reply.RequestID = id
	reply.IntentFile = filepath.Join(requests, id+".json")
	_ = file

	final, res, werr := webserver.Wait(ctx, target.StatusPath, id, siteID, intent.DeadlineAt)
	if werr != nil {
		if !webserver.IsNoVerdict(werr) {
			return toolresponse.WrapErrorWithContext(ctx, "webserver", werr, inv.Start)
		}
		// Nobody answered. The intent is still on disk and may yet be applied, which is a
		// different message from "refused" and the reason the two outcomes are kept apart
		// (plan 2 A-7/A-19).
		reply.Verdict = webserverVerdictNoVerdict
		reply.Error = werr.Error()
		reply.Note = "the reconciler had not answered by the deadline; the intent file is still there and may be applied on a later pass — check `webserver status` before re-filing, and re-file with the same request_id never (a new attempt needs a new id)"
		return toolresponse.WrapSuccess(ctx, "webserver", reply, inv.Start)
	}

	reply.EngineResult = res.Result
	reply.Error = res.Error
	switch res.Result {
	case "applied":
		reply.Verdict = webserverVerdictApplied
	case "rejected":
		reply.Verdict = webserverVerdictRejected
	default:
		// failed or expired: the ledger answered, and the answer is not a clean apply. Say so
		// rather than reporting a rejection the engine did not write.
		reply.Verdict = webserverVerdictInconsistent
	}
	if siteState, err := st.Site(siteID); err == nil {
		reply.ConfigPath = siteState.ConfigPath
	}
	if fresh, err := webserver.Read(target.StatusPath); err == nil {
		if siteState, serr := fresh.Site(siteID); serr == nil {
			reply.State = siteState.State
			reply.AppliedSHA256 = siteState.AppliedSHA256
			reply.Generation = siteState.Generation
		}
		// The lint findings are the actionable half of a rejection (plan 2 §5.2's acceptance line:
		// the findings come back READ FROM THE STATUS FILE), so they are returned verbatim.
		if reply.Verdict != webserverVerdictApplied {
			if d, ok := fresh.DesiredFor(reply.ConfigPath); ok && len(d.Findings) > 0 {
				reply.Findings = d.Findings
			}
		}
	}
	if len(final.Sites) == 0 && reply.Verdict == webserverVerdictApplied {
		reply.Note = "the verdict says applied and the site is not in the document yet; the next pass will list it"
	}
	return toolresponse.WrapSuccess(ctx, "webserver", reply, inv.Start)
}

// webserverLiveConfigPath is the file the CAS compares against: the status document's config path,
// or the parked sibling for a site that has been stopped.
func webserverLiveConfigPath(fleetDir string, resolved webserver.SiteState, site string, resolves bool) (string, error) {
	candidate := strings.TrimSpace(resolved.ConfigPath)
	if candidate == "" {
		return "", fmt.Errorf("webserver: the status document records no config path for site %q", site)
	}
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	if parked := candidate + ".disabled"; resolves {
		if _, err := os.Stat(parked); err == nil {
			return parked, nil
		}
	}
	return "", fmt.Errorf("webserver: no live config at %s for site %q (a publish with expect_sha256 \"\" creates one)", candidate, site)
}

// webserverReadConfig reads the YAML a publish will apply, under the engine's own size ceiling.
func webserverReadConfig(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("webserver publish: config_file is required (the path to the YAML to publish)")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("webserver: open config_file %s: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("webserver: stat config_file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("webserver: config_file %s is not a regular file", path)
	}
	if info.Size() > webserverMaxConfigBytes {
		return nil, fmt.Errorf("webserver: config_file %s is %d bytes, over the %d a site YAML may reach", path, info.Size(), webserverMaxConfigBytes)
	}
	raw, err := io.ReadAll(io.LimitReader(f, webserverMaxConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("webserver: read config_file %s: %w", path, err)
	}
	if len(raw) > webserverMaxConfigBytes {
		return nil, fmt.Errorf("webserver: config_file %s is over the %d limit", path, webserverMaxConfigBytes)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("webserver: config_file %s is empty; publishing nothing would remove a site's configuration", path)
	}
	return raw, nil
}

// webserverHashFile is the lowercase hex SHA-256 the CAS compares, streaming so a large file costs
// no buffer. It is the same digest pkg/fleet records as applied_sha256, which is what makes the
// comparison in the reply meaningful.
func webserverHashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ---- small argument readers -----------------------------------------------------------------

// webserverBoolArg reads a boolean flag in every shape a caller can send it: a real JSON bool from
// an MCP client, or a string from a CLI wrapper that has nothing else.
func webserverBoolArg(args map[string]any, name string) bool {
	switch v := args[name].(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		b, err := strconv.ParseBool(s)
		return err == nil && b
	default:
		if n, ok := webserverIntArg(v); ok {
			return n != 0
		}
		return false
	}
}

// webserverIntArg is bridgeIntArg with a log line on the way out: an integer the tool cannot read
// is a caller bug worth seeing in the server log rather than a parameter that silently defaults
// (plan 2 §5.3's reason for declaring page/per_page as integers at all).
func webserverIntArg(v any) (int, bool) {
	n, ok := bridgeIntArg(v)
	if !ok && v != nil {
		logfilter.Debugf("[webserver] ignored non-integer parameter %v", v)
	}
	return n, ok
}
