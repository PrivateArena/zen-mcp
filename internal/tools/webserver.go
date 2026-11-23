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
// this file is that the schema is authoritative: the action enum, the parameter set, the
// per-parameter enums, and (from `x-actions`, webserverActionContract) each action's required
// parameters, its transport and its compare-and-swap rule are all READ from it rather than restated
// here, so an endpoint change shows up as a mismatch — a panic at package start
// (webserverGuardProblems from init) and a test failure (webserver_test.go) — instead of as a tool
// that silently accepts a parameter it ignores. What is left hand-typed in webserverActionSpecs is
// only what the generated file cannot carry: the local policy (the present-but-empty key, the
// `confirm` literals) and the admin endpoint suffix.
//
// Three surfaces, and which one an action uses is the security property, not a detail (§5.1):
//
//	admin  — one site's loopback admin API (default mount /_zen/, published per site by
//	         api/catalog's admin_prefix), reached through webserver.Client (port from the
//	         status document, never from an argument)
//	status — the status document read straight off disk; no socket at all, because a `plan` that
//	         went through a running server would describe that server rather than the config (§5.1)
//	fleet  — publish/stop/start/rm/rollback: an intent FILE plus a verdict wait, so no network
//	         listener anywhere accepts a fleet mutation (§5.2)
//
// And because zen-mcp has no auth and no permission model of its own (plan 2 §5 constraint 2,
// §5.2), the gates that matter are enforced HERE, before anything is filed: the CAS hash, for the
// verbs whose contract carries one (see webserverActionContract.CAS), and the `confirm == "yes"`
// on destructive actions. The engine does not check confirmations, so this is the only place a
// "remove that site" can be refused.

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
// zen-webserver/testdata/webserver.schema.json. Do not hand-edit it: regenerate it (plan 2 §5.3)
// and check the copy against webserverSchemaVersion, which records which engine build it came from.
//
//go:embed webserver.schema.json
var webserverSchemaJSON []byte

// webserverSchemaVersion is the engine's own stamp on the contract embedded above, and it is
// recorded here for one reason: a COPIED file has no build failure attached to it. zen-mcp cannot
// import the engine (plan 2 §5.3), so the only link between webserver.schema.json and the build
// that produced it is this string — the digest of the engine's published request schemas plus its
// action table (pkg/admin_api/mcpschema.go:265, computed by pkg/admin_api/schemas.go SchemaVersion).
// TestWebserverSchemaVersionIsStamped compares the two, and a stale copy therefore fails `go test`
// instead of quietly shipping a contract the engine no longer publishes.
//
// To refresh after a copy: regenerate in zen-webserver
// (`go run . -fleet www -mode gen-mcp-schema -out testdata/webserver.schema.json`), copy
// zen-webserver/testdata/webserver.schema.json over internal/tools/webserver.schema.json, then set
// this constant to the `x-schema-version` the new file publishes. The test names all three steps.
const webserverSchemaVersion = "sha256:95b02436993f"

// webserverSchemaVersionKey is the annotation the engine stamps that version into
// (pkg/admin_api/mcpschema.go:265 "x-schema-version"), and webserverActionsKey the per-action
// contract below (same file, :266 "x-actions", emitted by actionsForSchema at :286-308).
const (
	webserverSchemaVersionKey = "x-schema-version"
	webserverActionsKey       = "x-actions"
)

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

// webserverActionContract is one entry of the schema's `x-actions` array: the per-action contract
// the FLAT properties map cannot carry, because every property is shared by every action. This is
// the engine's own data (pkg/admin_api/mcpschema.go Actions() → actionsForSchema), so the required
// set, the transport and the CAS rule are read from here rather than typed a second time — which is
// the drift that file names out loud (same file, :259-264: a consumer "can generate its guard table
// from it instead of typing one, which is where zen-mcp's start/stop rules drifted from this
// engine").
type webserverActionContract struct {
	Action    string `json:"action"`
	Summary   string `json:"summary"`
	Transport string `json:"transport"`
	Perm      string `json:"perm"`
	Mutates   bool   `json:"mutates"`
	Endpoint  string `json:"endpoint,omitempty"`
	// Method is the HTTP verb, published separately from Endpoint exactly so that nobody has to parse
	// it off the front of a route string to find the path (pkg/admin_api/mcpschema.go).
	Method   string   `json:"method,omitempty"`
	Required []string `json:"required"`
	Optional []string `json:"optional"`
	// CAS is what the reconciler does with expect_sha256, in the engine's own words
	// (pkg/admin_api/mcpschema.go:66-81 and the CASMode values at :85-96, whose ground truth is
	// pkg/fleet/consume.go). The ABSENCE of the key is itself an answer, not an unknown: the
	// action writes no intent file, so there is no hash to compare (same file, :95). It is
	// therefore treated as webserverCASNone everywhere below.
	CAS string `json:"cas,omitempty"`
}

// The three compare-and-swap modes, spelled exactly as the engine publishes them.
const (
	// webserverCASRequired — the hash is always compared, so an empty value can never match and a
	// real hash is mandatory (rm, rollback; pkg/fleet/consume.go:279 applyRollback, :375
	// applyRemove).
	webserverCASRequired = "required"
	// webserverCASCreates — publish's rule: an empty expect_sha256 is a VALID answer meaning
	// "there must be no incumbent config", so the key cannot be modelled as a required string
	// (pkg/fleet/consume.go:215-228). This is what webserverActionSpec.presentKeys is for.
	webserverCASCreates = "create-or-conflict"
	// webserverCASNone — the verb reads no hash at all: stop/start are os.Rename operations
	// (pkg/fleet/consume.go:308-349 applyPark never touches in.ExpectSHA256), so demanding a hash
	// here would be a rule this file invented.
	webserverCASNone = "none"
)

// webserverActionContracts is x-actions keyed by action name. Fatal if the array is missing or
// malformed: without it the guard table has nothing to be reconciled against, and a silently
// absent contract is exactly the stale-copy failure mode webserverSchemaVersion exists to catch.
var webserverActionContracts = mustWebserverActionContracts()

func mustWebserverActionContracts() map[string]webserverActionContract {
	raw, ok := webserverSchema[webserverActionsKey]
	if !ok {
		panic("tools: webserver.schema.json declares no " + webserverActionsKey + "; this copy predates the engine publishing its per-action contract (pkg/admin_api/mcpschema.go:266), so the guard table cannot be checked against it — refresh the file (see webserverSchemaVersion)")
	}
	// Re-encode rather than walk the map[string]any by hand: one shape to read, and a wrong type
	// is a decode error instead of a silent zero value.
	blob, err := json.Marshal(raw)
	if err != nil {
		panic("tools: webserver.schema.json " + webserverActionsKey + " cannot be re-encoded: " + err.Error())
	}
	var items []webserverActionContract
	if err := json.Unmarshal(blob, &items); err != nil {
		panic("tools: webserver.schema.json " + webserverActionsKey + " is not a list of action contracts: " + err.Error())
	}
	if len(items) == 0 {
		panic("tools: webserver.schema.json " + webserverActionsKey + " is empty; the engine declares 19 actions")
	}
	out := make(map[string]webserverActionContract, len(items))
	for _, item := range items {
		switch {
		case item.Action == "":
			panic("tools: webserver.schema.json has an " + webserverActionsKey + " entry with no action name")
		case !webserverSchemaHasAction(item.Action):
			panic("tools: webserver.schema.json declares " + webserverActionsKey + " action " + item.Action + ", which its own action enum does not")
		}
		if _, dup := out[item.Action]; dup {
			panic("tools: webserver.schema.json declares action " + item.Action + " twice in " + webserverActionsKey)
		}
		out[item.Action] = item
	}
	return out
}

// webserverContractFor is the per-action lookup the validator and the fleet verb run on.
func webserverContractFor(action string) (webserverActionContract, bool) {
	c, ok := webserverActionContracts[action]
	return c, ok
}

// webserverActionKind is which surface one action drives. The kinds exist so that "the fleet verbs
// touch no socket" (§5.2) is a property a test can assert over a group of actions instead of a
// comment an auditor has to trust. Which kind an action belongs to is not a matter of opinion here:
// the engine publishes the transport, and webserverGuardProblems below refuses to start when the
// two disagree.
type webserverActionKind string

// The transports x-actions publishes (pkg/admin_api/mcpschema.go TransportKind()). Three fixed
// tokens, named here because webserverGuardProblems compares them exactly: a fleet verb that read
// "intent" instead of "intent-file" would put a mutation on the wrong side of §5.2. They are
// deliberately NOT the human-readable transport strings the generated docs show — those carry a
// route name, and switching on a route would make a rename change behaviour.
const (
	webserverTransportIntentFile = "intent-file"
	webserverTransportStatusFile = "status-file"
	webserverTransportAdmin      = "admin"
)

const (
	webserverAdminAction  webserverActionKind = "admin"  // one site's loopback admin API
	webserverStatusAction webserverActionKind = "status" // the status document, file read only
	webserverFleetAction  webserverActionKind = "fleet"  // intent file + verdict wait, file only
)

// webserverTransportKind names the surface one published transport drives. An unrecognised value is
// an error rather than a guess: the kinds are the security grouping, and a new transport that
// silently landed in one of them would put a mutation on the wrong side of §5.2.
func webserverTransportKind(transport string) (webserverActionKind, error) {
	switch transport {
	case webserverTransportIntentFile:
		return webserverFleetAction, nil
	case webserverTransportStatusFile:
		return webserverStatusAction, nil
	case webserverTransportAdmin:
		return webserverAdminAction, nil
	}
	return "", fmt.Errorf("transport %q is none of %q, %q, %q", transport, webserverTransportAdmin, webserverTransportIntentFile, webserverTransportStatusFile)
}

// webserverActionSpec is the LOCAL half of the per-action contract, in the shape youtube.go
// established: the surface the action may use, the parameters whose only acceptable value is a
// literal, and the admin endpoint suffix. Validation runs before any dispatch, so a malformed
// request neither opens a connection nor files an intent.
//
// There is deliberately no `required` field. Which parameters the engine refuses to run without is
// generated data, not policy: it arrives in x-actions (`required`, pkg/admin_api/mcpschema.go:299,
// from ActionSpec.Params[].Required) and validateWebserverArgs reads it from there. A hand-typed
// copy of it is how `stop`/`start` ended up demanding a hash the engine never reads.
type webserverActionSpec struct {
	// kind is the surface this action may drive. It is cross-checked against the published
	// `transport` at startup (webserverGuardProblems), so it cannot quietly disagree with the engine.
	kind webserverActionKind
	// presentKeys must arrive at all, "" allowed. It exists for exactly one action today: a
	// publish's expect_sha256, whose empty value is a real answer ("there must be no incumbent
	// config", pkg/fleet/intent.go:65-68, enforced by pkg/fleet/consume.go:215-228) — so the rule
	// is "the key must be present", which a required string cannot express. The startup check
	// insists this set matches x-actions `cas: "create-or-conflict"`, no wider.
	presentKeys []string
	// exact names a param whose only accepted value is the literal, checked case-sensitively:
	// "y", "YES" and "true" are not the confirmation this gate is asking for. This is local policy
	// — the engine checks no confirm on `delete` or on an rm intent (pkg/admin_api/mcpschema.go:179
	// and :191 spell both out as CLIENT-SIDE GUARD) — and the startup check refuses to run if any
	// action that declares a `confirm` parameter in x-actions loses its literal here.
	exact map[string]string
	// endpoint is the admin suffix for the admin kind, recorded so a test can prove no fleet
	// action has one. It is checked against the same action's published `endpoint`, which carries
	// the "api/" segment this field omits, so it cannot point somewhere the engine does not serve.
	endpoint string
}

var webserverActionSpecs = map[string]webserverActionSpec{
	// --- status document reads: never an HTTP round trip (plan 2 §5.1) ---
	"sites":   {kind: webserverStatusAction},
	"status":  {kind: webserverStatusAction},
	"plan":    {kind: webserverStatusAction},
	"intents": {kind: webserverStatusAction},

	// --- one site's loopback admin API (plan 2 §5.4: doctor is the verify gate) ---
	// Only the surface and the endpoint are named below: `site`, `slug` and the rest are required
	// by the engine's x-actions, not by anything here.
	"doctor":        {kind: webserverAdminAction, endpoint: "doctor"},
	"catalog":       {kind: webserverAdminAction, endpoint: "catalog"},
	"list":          {kind: webserverAdminAction, endpoint: "list"},
	"get":           {kind: webserverAdminAction, endpoint: "get"},
	"save":          {kind: webserverAdminAction, endpoint: "save"},
	"revision_list": {kind: webserverAdminAction, endpoint: "revision/list"},
	"media_upload":  {kind: webserverAdminAction, endpoint: "media/upload"},
	"rename_slug":   {kind: webserverAdminAction, endpoint: "rename_slug"},
	// A URL move that is not a rename. The engine refuses an off-site destination, and says why in
	// the error, so nothing here needs to second-guess the value: the one thing worth stating is
	// that this tool is not the guard (plan 2 A-12's open-redirect rule lives server-side, where
	// every writer of the table can be held to it).
	"redirect": {kind: webserverAdminAction, endpoint: "redirect"},
	// An article delete is irreversible and the engine admits it without asking, so the literal is
	// required here — same reasoning as `rm` below.
	"delete": {kind: webserverAdminAction, endpoint: "delete", exact: map[string]string{"confirm": "yes"}},

	// --- fleet mutations: intent files only, never an endpoint (§5.2) ---
	// Which of these carries a hash is the engine's answer, published as `cas`: publish's empty
	// expect_sha256 is a valid create-only answer (webserverCASCreates → presentKeys), rm/rollback
	// always compare one (webserverCASRequired, and their x-actions `required` names the key), and
	// stop/start read none at all (webserverCASNone, consume.go:308-349), so they declare nothing.
	"publish": {
		kind:        webserverFleetAction,
		presentKeys: []string{"expect_sha256"},
	},
	"stop":  {kind: webserverFleetAction},
	"start": {kind: webserverFleetAction},
	// The engine does not check `confirm` on an rm either (plan 2 §5.2's whole point about
	// zen-mcp's missing permission model; pkg/admin_api/mcpschema.go:191 says an intent has no
	// confirm field at all), so this refusal is the ONLY gate between "rm" and a config leaving the
	// fleet. It is enforced here, in the layer that can enforce it.
	"rm":       {kind: webserverFleetAction, exact: map[string]string{"confirm": "yes"}},
	"rollback": {kind: webserverFleetAction},
}

// webserverGuardProblems reconciles the two halves of the guard table and returns one line per
// disagreement, naming the action and both values. It returns problems rather than panicking so
// webserver_test.go can prove it catches a wrong table without editing a global; init below is what
// makes a real mismatch fatal before the first tool call.
//
// What it insists on, and why each half exists at all:
//
//   - the action sets agree — a new engine action with no local entry would be dispatched blind,
//     and a local entry the engine no longer declares is dead code that reads as coverage;
//   - `transport` implies `kind` — the kinds are the security grouping behind §5.2 (a fleet verb
//     must have no endpoint), so the pairing is checked, not assumed;
//   - the endpoint suffix this file types is the one the transport names — otherwise a reply could
//     report an endpoint the call never hit;
//   - the hash rules — `cas: "required"` must produce a real required hash, `cas:
//     "create-or-conflict"` must be present-keys-only, and no verb the engine reads no hash for may
//     gain a local hash rule of its own. That last clause is the defect this reconciliation exists
//     for: `stop`/`start` carried an invented expect_sha256 demand here for exactly as long as it
//     took to make a valid `start` fail;
//   - every action whose x-actions declares a `confirm` parameter keeps its local literal gate —
//     the engine checks no confirmation, so dropping one here drops the only check there is.
func webserverGuardProblems(specs map[string]webserverActionSpec, contracts map[string]webserverActionContract) []string {
	var problems []string
	for _, action := range sortedWebserverNames(specs, contracts) {
		spec, local := specs[action]
		contract, generated := contracts[action]
		switch {
		case !generated:
			problems = append(problems, fmt.Sprintf("action %q has a local contract (%+v) but no %s entry; it would be dispatched without the engine's parameter rules", action, spec, webserverActionsKey))
			continue
		case !local:
			problems = append(problems, fmt.Sprintf("action %q is published by the engine (%s) but has no entry in webserverActionSpecs; this build does not know which surface it may touch", action, contract.Transport))
			continue
		}

		wantKind, kindErr := webserverTransportKind(contract.Transport)
		if kindErr != nil {
			problems = append(problems, fmt.Sprintf("action %q: %v, so its kind %q cannot be confirmed", action, kindErr, string(spec.kind)))
		} else if wantKind != spec.kind {
			problems = append(problems, fmt.Sprintf("action %q: transport %q is the %s surface but this build routes it as %q", action, contract.Transport, wantKind, spec.kind))
		}

		if spec.kind == webserverAdminAction {
			suffix, hasPath := strings.CutPrefix(contract.Endpoint, "api/")
			switch {
			case !hasPath:
				problems = append(problems, fmt.Sprintf("action %q: kind %q needs an endpoint under api/, and the engine published %q (transport %q)", action, spec.kind, contract.Endpoint, contract.Transport))
			case spec.endpoint == "":
				problems = append(problems, fmt.Sprintf("action %q: the engine publishes endpoint %q, which webserverActionSpecs leaves empty", action, suffix))
			case spec.endpoint != suffix:
				problems = append(problems, fmt.Sprintf("action %q: this build calls %q, the engine publishes %q", action, spec.endpoint, suffix))
			}
		} else if spec.endpoint != "" {
			// A fleet verb with an endpoint is §5.2 broken — and it is exactly the shape a careless
			// regeneration produces, by copying the row above.
			problems = append(problems, fmt.Sprintf("action %q: kind %q must file an intent, but it records endpoint %q", action, spec.kind, spec.endpoint))
		}

		switch rule := webserverContractHashRule(contract); rule {
		case webserverCASRequired:
			if !webserverHas(contract.Required, "expect_sha256") {
				problems = append(problems, fmt.Sprintf("action %q: cas %q compares the hash always, so the engine must require it; required is [%s]", action, rule, strings.Join(contract.Required, ", ")))
			}
			if webserverHas(spec.presentKeys, "expect_sha256") {
				problems = append(problems, fmt.Sprintf("action %q: cas %q already requires expect_sha256; presentKeys %q would give the rule two owners", action, rule, strings.Join(spec.presentKeys, ", ")))
			}
		case webserverCASCreates:
			if !webserverHas(spec.presentKeys, "expect_sha256") {
				problems = append(problems, fmt.Sprintf("action %q: cas %q makes an empty expect_sha256 a real answer, so presentKeys must name it; presentKeys is [%s]", action, rule, strings.Join(spec.presentKeys, ", ")))
			}
			if webserverHas(contract.Required, "expect_sha256") {
				problems = append(problems, fmt.Sprintf("action %q: cas %q cannot hold while x-actions %q also requires expect_sha256", action, rule, strings.Join(contract.Required, ", ")))
			}
		default:
			// webserverCASNone — the rename verbs — and an absent `cas`, which means this action
			// writes no intent file at all (pkg/admin_api/mcpschema.go:95). Either way the engine
			// compares nothing, so neither side may carry a hash rule for it.
			if webserverHas(contract.Required, "expect_sha256") || webserverHas(contract.Optional, "expect_sha256") {
				problems = append(problems, fmt.Sprintf("action %q: hash rule %q compares nothing, yet x-actions lists expect_sha256 in [%s]", action, rule, strings.Join(append(append([]string{}, contract.Required...), contract.Optional...), ", ")))
			}
			if webserverHas(spec.presentKeys, "expect_sha256") {
				problems = append(problems, fmt.Sprintf("action %q: hash rule %q compares nothing, yet this build demands expect_sha256 in presentKeys", action, rule))
			}
		}

		for _, name := range spec.presentKeys {
			if !webserverHas(contract.Required, name) && !webserverHas(contract.Optional, name) {
				problems = append(problems, fmt.Sprintf("action %q: presentKeys demands %q, which neither x-actions required [%s] nor optional [%s] declares", action, name, strings.Join(contract.Required, ", "), strings.Join(contract.Optional, ", ")))
			}
		}

		if webserverHas(contract.Optional, "confirm") {
			if got, ok := spec.exact["confirm"]; !ok {
				problems = append(problems, fmt.Sprintf("action %q: the engine declares a confirm parameter and checks no confirmation, so this build must require the literal; exact is %v", action, spec.exact))
			} else if got != "yes" {
				problems = append(problems, fmt.Sprintf("action %q: the confirm literal is %q, not the \"yes\" this gate exists to insist on", action, got))
			}
		}
		for name, want := range spec.exact {
			if want != "yes" {
				problems = append(problems, fmt.Sprintf("action %q: exact literal for %s is %q; this build only ever gates on \"yes\"", action, name, want))
			}
		}
	}
	return problems
}

// webserverContractHashRule is a contract's CAS mode with the absent key read the way the engine
// means it: no intent file, therefore no hash, never "unknown" (pkg/admin_api/mcpschema.go:95).
func webserverContractHashRule(contract webserverActionContract) string {
	if contract.CAS == "" {
		return webserverCASNone
	}
	return contract.CAS
}

// webserverHas reports whether one parameter name appears in a list this file got from the schema
// (an action's required or optional set) or typed for it (presentKeys).
func webserverHas(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// sortedWebserverNames is every action named by either half of the table, in one order, so a
// disagreement prints the same way on every run.
func sortedWebserverNames(specs map[string]webserverActionSpec, contracts map[string]webserverActionContract) []string {
	set := map[string]bool{}
	for action := range specs {
		set[action] = true
	}
	for action := range contracts {
		set[action] = true
	}
	out := make([]string, 0, len(set))
	for action := range set {
		out = append(out, action)
	}
	sort.Strings(out)
	return out
}

// init is where the two halves of the guard table are reconciled, before a single call can be
// served. It has to be a start-up panic and not only a test: a table that has drifted is a table
// whose hash and confirm gates are no longer the ones the plan says are enforced, and a running MCP
// server must not serve that.
func init() {
	if problems := webserverGuardProblems(webserverActionSpecs, webserverActionContracts); len(problems) > 0 {
		panic("tools: the webserver guard table disagrees with " + webserverActionsKey + " in webserver.schema.json (see webserverSchemaVersion for how to refresh):\n\t" + strings.Join(problems, "\n\t"))
	}
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

// webserverAdminPrefix is the FALLBACK admin path, spelled the way Client.Call spells it (internal/
// webserver/client.go:41 apiPrefix, joined at :206-211 as `base + /_zen/api + /suffix`). It is the
// only place this literal is written in this file, and it is a fallback precisely because the
// engine does not keep the mount to itself: an admin surface is mounted at `settings.admin_path`
// (server_snapshot.go:36-39, whose default is the MOUNT `/_zen/`, not `/_zen/api`), and api/catalog
// publishes it per site as `admin_prefix` (pkg/admin_api/catalog.go:71). Endpoint paths are
// `<mount>api/<suffix>` — the engine's own doctor walks sn.adminPrefix + "api/catalog"
// (doctor.go:180) — which is why webserverAdminAPIPath below adds the `api` segment rather than
// treating the published value as the whole path. Only the one call the client cannot express needs
// any of this: see webserverMediaUpload.
const webserverAdminPrefix = "/_zen/api"

// webserverAdminAPIPath turns an admin mount into the path prefix one endpoint hangs off, in the
// shape Client.Call uses: `/<mount>…/api/`. It accepts either spelling — the mount api/catalog
// publishes ("/_zen/", and a site's own settings.admin_path) or the mount-plus-`api` form of the
// fallback constant — so one join rule covers both without this file having to know which it got.
func webserverAdminAPIPath(published string) string {
	path := strings.Trim(strings.TrimSpace(published), "/")
	if path == "" {
		path = strings.Trim(webserverAdminPrefix, "/")
	}
	segs := strings.Split(path, "/")
	if segs[len(segs)-1] != "api" {
		segs = append(segs, "api")
	}
	return "/" + strings.Join(segs, "/") + "/"
}

// webserverCatalogAdminMount reads one site's admin mount out of an api/catalog reply
// (`admin_prefix`, pkg/admin_api/catalog.go:71), or "" when the reply does not carry it.
func webserverCatalogAdminMount(raw []byte) string {
	var cat struct {
		AdminPrefix string `json:"admin_prefix"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &cat) != nil {
		return ""
	}
	return strings.TrimSpace(cat.AdminPrefix)
}

// webserverAdminPathFor answers "where does THIS site mount its admin API" from the only source
// that knows: api/catalog, fetched through the same client, so the loopback and status-document rules
// apply to it exactly as they do to the call it is preparing for. A catalog that cannot be read is
// not an error here — the request this one is staging will report the real problem — so it falls
// back to webserverAdminPrefix, said out loud in the log because a mount the site moved is exactly
// the kind of thing that should leave a trace.
//
// The bootstrap caveat is honest and unavoidable: the catalog call that discovers the mount is itself
// spelled with Client.Call's default (internal/webserver/client.go:41), because discovering a mount
// cannot require knowing it. A site that moved admin_path and cannot answer api/catalog is therefore
// not reachable from here — and it fails as a 404/unknown_admin_endpoint on the upload, never as a
// write to some other path.
func webserverAdminPathFor(ctx context.Context, client *webserver.Client, siteID string) string {
	raw, err := client.Catalog(ctx, siteID)
	if err == nil {
		if mount := webserverCatalogAdminMount(raw); mount != "" {
			return webserverAdminAPIPath(mount)
		}
		logfilter.Debugf("[webserver] site %s published no admin_prefix in api/catalog; using %s", siteID, webserverAdminPrefix)
		return webserverAdminAPIPath(webserverAdminPrefix)
	}
	logfilter.Debugf("[webserver] cannot read api/catalog for site %s (%v); using the default admin mount %s", siteID, err, webserverAdminPrefix)
	return webserverAdminAPIPath(webserverAdminPrefix)
}

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
	// Which parameters must arrive is the ENGINE's list, read off x-actions (`required`, generated
	// from ActionSpec.Params[].Required at pkg/admin_api/mcpschema.go:299) rather than typed here:
	// a hand-ported copy is how `stop`/`start` came to demand a hash applyPark never reads
	// (pkg/fleet/consume.go:308-349). What stays local is only what the generated file cannot say:
	// present-but-empty keys and the confirm literals, both below.
	contract, ok := webserverContractFor(action)
	if !ok {
		return fmt.Errorf("webserver %s: %s declares no contract for it, so this build cannot know which parameters it needs (known actions: %s)",
			action, webserverActionsKey, strings.Join(webserverActions(), ", "))
	}
	for _, name := range contract.Required {
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

// ---- admin actions (one site's loopback admin API) --------------------------------------------

// webserverAdminCall runs one content/read action. The URL is never assembled from an argument:
// Client.Resolve turns `site` into a loopback base from the status document, and Call appends the
// endpoint this build knows about (plan 2 §5.1, A-18). Only one of these calls assembles a URL
// itself, and even that one takes its mount from api/catalog — see webserverMediaUpload.
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
		raw, err = webserverMediaUpload(ctx, client, site, inv.Spec.endpoint, inv.Args)
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
// parses (HandleMediaUpload reads FormFile("file"), pkg/admin_api/handlers.go:276); Client.Call speaks
// JSON bodies only, so this is the one admin call this tool assembles itself. It still resolves the
// base through Client.Resolve, so the port comes from the status document and a non-loopback host is
// refused exactly as it is everywhere else (plan 2 §5.1) — the multipart is the difference in
// encoding, not in trust. The MOUNT is the site's own, read from api/catalog's admin_prefix
// (webserverAdminPathFor) rather than assumed, because settings.admin_path is per-site
// (pkg/config/config.go:66) and a wrong guess here is a 404 on a 32 MiB POST; the literal is only
// the documented fallback for a catalog that cannot be read.
func webserverMediaUpload(ctx context.Context, client *webserver.Client, siteID, endpoint string, args map[string]any) (json.RawMessage, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+webserverAdminPathFor(ctx, client, siteID)+endpoint, &body)
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

// webserverAppendNote stacks a caveat onto a fleet reply instead of replacing one: a call can owe
// the caller two separate truths at once — that expect_sha256 was never going to be compared, and
// that nobody answered by the deadline — and the first is the one a caller would otherwise lose.
func webserverAppendNote(reply *webserverFleetReply, note string) {
	if note == "" {
		return
	}
	if reply.Note == "" {
		reply.Note = note
		return
	}
	reply.Note += " · " + note
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
	// The engine's own rule for what happens to that value, read off x-actions: `required` (rm,
	// rollback) compares it always, `create-or-conflict` (publish) accepts an empty one as "no
	// incumbent", and `none` (stop, start) does not read a hash at all — those two are os.Rename
	// operations (pkg/fleet/consume.go:308-349 applyPark, which never touches in.ExpectSHA256).
	// Absent means the same as `none` here, and is not treated as unknown (pkg/admin_api/mcpschema.go:95).
	contract, known := webserverContractFor(inv.Action)
	if !known {
		// Unreachable through HandleWebserver, which refuses an action with no contract; kept as a
		// refusal because filing an intent without knowing its CAS rule is the blind dispatch this
		// tool is written to make impossible.
		return toolresponse.WrapErrorWithContext(ctx, "webserver",
			fmt.Errorf("webserver %s: %s declares no contract for it, so this build cannot know what the reconciler compares", inv.Action, webserverActionsKey), inv.Start)
	}
	hashRule := webserverContractHashRule(contract)
	comparesHash := hashRule == webserverCASRequired || hashRule == webserverCASCreates
	// noCompareNote is the caveat a rename owes its caller: see the CAS block below.
	var noCompareNote string

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

	// The CAS, checked locally before anything is filed — and only for the verbs whose published
	// contract carries a hash at all. The engine enforces the same rule (applyPublish
	// consume.go:215-228, applyRollback :279, applyRemove :375), so this is an early refusal of a
	// conflict the reconciler would reject anyway, which is what keeps a doomed full YAML out of the
	// intent log (A-3). For stop/start the comparison is skipped rather than loosened: demanding a
	// hash there was this file inventing a rule, and it made a valid `start` fail. What guards a
	// rename instead is the status document above plus the engine's own already_stopped / no_config
	// / conflict refusals (consume.go:322-345).
	if comparesHash {
		if live, liveErr := webserverLiveConfigPath(target.FleetDir, resolved, site, resolves); liveErr == nil {
			if sum, herr := webserverHashFile(live); herr != nil {
				return toolresponse.WrapErrorWithContext(ctx, "webserver", fmt.Errorf("webserver %s: cannot hash the live config %s: %w", inv.Action, live, herr), inv.Start)
			} else if sum != expect {
				return toolresponse.WrapErrorWithContext(ctx, "webserver",
					fmt.Errorf("webserver %s: cas_conflict — the live config %s hashes %s but expect_sha256 said %s; re-read it and re-submit. Nothing was filed", inv.Action, live, sum, expect), inv.Start)
			}
		} else if hashRule != webserverCASCreates || expect != "" {
			return toolresponse.WrapErrorWithContext(ctx, "webserver", liveErr, inv.Start)
		}
	} else if expect != "" {
		// Not an error: the engine ignores the field for a rename, and refusing here would break a
		// caller the OLD client contract forced to send one. But pretending a compare happened when
		// none can is worse than saying so, so the reply says it.
		noCompareNote = fmt.Sprintf("expect_sha256 was not compared: %s parks or unparks the config with a rename, and the engine reads no hash for it (pkg/fleet/consume.go:308-349)", inv.Action)
	}

	reply := webserverFleetReply{Action: inv.Action, Site: site, ConfigPath: configPath}
	webserverAppendNote(&reply, noCompareNote)

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
		webserverAppendNote(&reply, "the reconciler had not answered by the deadline; the intent file is still there and may be applied on a later pass — check `webserver status` before re-filing, and re-file with the same request_id never (a new attempt needs a new id)")
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
		webserverAppendNote(&reply, "the verdict says applied and the site is not in the document yet; the next pass will list it")
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
