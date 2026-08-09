package terminal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"zen-mcp/internal/tools"
)

// cliTool is the minimal tool metadata needed to render a wrapper script.
type cliTool struct {
	name        string
	description string
	schema      map[string]any
}

// cliParam is one schema property rendered into the wrapper help.
type cliParam struct {
	key      string
	required bool
	desc     string
	values   []string
}

// ExportCLI generates CLI wrapper scripts for all tools using full --param names.
func ExportCLI(w io.Writer, cliPort, mcpPort int) {
	ExportCLIWithShort(w, cliPort, mcpPort, false)
}

// ExportCLIWithShort generates CLI wrapper scripts. When short is true each
// param also gets its shortest unambiguous -alias (--message -> -m) so callers
// can save tokens; full --param names remain accepted either way.
func ExportCLIWithShort(w io.Writer, cliPort, mcpPort int, short bool) {
	host := "127.0.0.1"
	port := cliPort
	if port == 0 {
		port = mcpPort
	}
	url := fmt.Sprintf("http://%s:%d", host, port)
	cliDir := filepath.Join(".", "cli")
	absCliDir, err := filepath.Abs(cliDir)
	if err != nil {
		absCliDir = cliDir
	}
	binDir := localBinDir()

	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		fmt.Fprintf(w, "ERROR: could not create %s: %v\n", cliDir, err)
		return
	}

	toolList := collectTools()
	generated := map[string]bool{}
	for _, t := range toolList {
		script := buildWrapperScriptOpt(t, url, short)
		path := filepath.Join(cliDir, "zen-"+t.name)
		if err := writeAtomic(path, script); err != nil {
			fmt.Fprintf(w, "WARN: failed to write %s: %v\n", path, err)
			continue
		}
		_ = os.Chmod(path, 0o755)
		generated["zen-"+t.name] = true
	}

	// Remove stale zen-* wrappers no longer generated.
	removeStaleZen(cliDir, generated)

	// Symlink into ~/.local/bin so the tools are reachable on PATH. The link
	// target must be absolute, otherwise links break when binDir is remote.
	symlinked := ""
	if binDir != "" {
		if err := os.MkdirAll(binDir, 0o755); err == nil {
			for name := range generated {
				target := filepath.Join(absCliDir, name)
				link := filepath.Join(binDir, name)
				_ = os.Remove(link)
				if os.Symlink(target, link) == nil {
					symlinked = binDir
				}
			}
			removeStaleZen(binDir, generated)
		}
	}

	fmt.Fprintf(w, "Exported %d tools → %s\n", len(generated), cliDir)
	if symlinked != "" {
		fmt.Fprintf(w, "Symlinked to %s\n", symlinked)
		if !pathContains(symlinked) {
			fmt.Fprintf(w, "WARNING: %s is not in your PATH. Run: export PATH=\"$HOME/.local/bin:$PATH\"\n", symlinked)
		}
	}
}

// ExportCliClean removes generated CLI wrappers and their ~/.local/bin links.
func ExportCliClean(w io.Writer) {
	removed := 0
	cliDir := filepath.Join(".", "cli")
	entries, _ := os.ReadDir(cliDir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "zen-") {
			if os.Remove(filepath.Join(cliDir, entry.Name())) == nil {
				removed++
			}
		}
	}
	if binDir := localBinDir(); binDir != "" {
		if entries, err := os.ReadDir(binDir); err == nil {
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "zen-") {
					if os.Remove(filepath.Join(binDir, entry.Name())) == nil {
						removed++
					}
				}
			}
		}
	}
	fmt.Fprintf(w, "Cleaned %d zen-* artifacts from cli/ and %s\n", removed, localBinDirLabel())
}

// collectTools resolves the tool set from the registry, falling back to the
// static schema set (tools.AllDefs) if no HTTP request has registered tools yet.
func collectTools() []cliTool {
	if d := GetDeps(); d.Reg != nil {
		regs := d.Reg.ListTools()
		if len(regs) > 0 {
			out := make([]cliTool, 0, len(regs))
			for _, tr := range regs {
				if tr.Name == "" {
					continue
				}
				out = append(out, cliTool{name: tr.Name, description: tr.Description, schema: tr.Schema})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
			return out
		}
	}
	defs := tools.AllDefs("", tools.Deps{})
	out := make([]cliTool, 0, len(defs))
	for _, d := range defs {
		out = append(out, cliTool{name: d.Name, description: d.Description, schema: d.Schema})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// collectParams parses a JSON Schema object into ordered, deterministic params.
func collectParams(schema map[string]any) []cliParam {
	if schema == nil {
		return nil
	}
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return nil
	}
	requiredSet := map[string]bool{}
	for _, r := range strSlice(schema["required"]) {
		requiredSet[r] = true
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	params := make([]cliParam, 0, len(keys))
	for _, k := range keys {
		p := cliParam{key: k, required: requiredSet[k]}
		if ps, ok := props[k].(map[string]any); ok {
			if d, ok := ps["description"].(string); ok {
				p.desc = d
			}
			p.values = strSlice(ps["enum"])
		}
		params = append(params, p)
	}
	return params
}

// shortAliasMap computes, per tool, the shortest unambiguous alias for each
// param key (e.g. message -> m, or mes when method is also present). Keys that
// are a prefix of another key keep their full name (url vs url2). The single
// char "h" is reserved for --help and never used.
func shortAliasMap(params []cliParam) map[string]string {
	if len(params) == 0 {
		return nil
	}
	keys := make([]string, len(params))
	for i, p := range params {
		keys[i] = p.key
	}
	aliases := make(map[string]string, len(params))
	for _, p := range params {
		if p.key == "" {
			continue
		}
		aliases[p.key] = shortestAlias(p.key, keys)
	}
	return aliases
}

// shortestAlias returns the shortest prefix of key that is not a prefix of any
// other key in keys, falling back to the full key when no proper prefix is
// unique. "h" is skipped so --help never collides with a short alias.
func shortestAlias(key string, keys []string) string {
	for i := 1; i <= len(key); i++ {
		cand := key[:i]
		if cand == "h" {
			continue
		}
		unique := true
		for _, k := range keys {
			if k == key {
				continue
			}
			if strings.HasPrefix(k, cand) {
				unique = false
				break
			}
		}
		if unique {
			return cand
		}
	}
	return key
}

// flagLabel renders "--key" or "-a, --key" for help/header lines.
func flagLabel(p cliParam, aliases map[string]string) string {
	if a, ok := aliases[p.key]; ok {
		return "-" + a + ", --" + p.key
	}
	return "--" + p.key
}

// strSlice extracts string elements from []string, []any, or nil.
func strSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return append([]string(nil), s...)
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func paramSuffix(p cliParam) (req, vals string) {
	if p.required {
		req = " (required)"
	}
	if len(p.values) > 0 {
		vals = " [" + strings.Join(p.values, "|") + "]"
	}
	return req, vals
}

// q escapes a string for embedding inside a double-quoted bash echo line,
// mirroring the TS generator's injection-safe escaping.
func q(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	return s
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// buildHelpStatements renders the --help case body for the wrapper.
func buildHelpStatements(t cliTool, url string) []string {
	return buildHelpStatementsOpt(t, url, nil)
}

// buildHelpStatementsOpt renders the --help case body; when aliases are set
// params are shown as "-a, --key".
func buildHelpStatementsOpt(t cliTool, url string, aliases map[string]string) []string {
	desc := oneLine(t.description)
	if desc == "" {
		desc = "(no description)"
	}
	params := collectParams(t.schema)

	lines := []string{
		`echo "` + q(t.name+" — "+desc) + `"`,
		`echo ""`,
		`echo "SERVER:"`,
		`echo "  POST ` + q(url) + `/mcp"`,
		`echo ""`,
		`echo "USAGE:"`,
		`echo "  $0 --<param> <value>..."`,
		`echo "  $0 --json '{\"key\":\"val\"}'   # raw JSON escape hatch"`,
	}
	if len(aliases) > 0 {
		lines = append(lines, `echo "  $0 -<short> <value>...   # short aliases"`)
	}
	lines = append(lines,
		`echo ""`,
		`echo "PARAMETERS:"`,
	)
	for _, p := range params {
		req, vals := paramSuffix(p)
		lines = append(lines, `echo "  `+flagLabel(p, aliases)+req+`  `+q(p.desc)+vals+`"`)
	}
	var allValues []string
	for _, p := range params {
		if len(p.values) > 0 {
			allValues = append(allValues, p.values...)
		}
	}
	if len(allValues) > 0 {
		lines = append(lines, `echo ""`)
		lines = append(lines, `echo "ACTION VALUES: `+q(strings.Join(allValues, ", "))+`"`)
	}
	lines = append(lines, `echo ""`)
	lines = append(lines, `exit 0`)
	return lines
}

// buildWrapperScript renders a self-contained bash wrapper for one tool using
// full --param names.
func buildWrapperScript(t cliTool, url string) string {
	return buildWrapperScriptOpt(t, url, false)
}

// buildWrapperScriptOpt renders the wrapper; when short is true each param
// also gets its shortest unambiguous single-dash alias (--message -> -m) in
// the arg parser, header comment, and --help output.
func buildWrapperScriptOpt(t cliTool, url string, short bool) string {
	desc := oneLine(t.description)
	if desc == "" {
		desc = "(no description)"
	}
	params := collectParams(t.schema)
	var aliases map[string]string
	if short {
		aliases = shortAliasMap(params)
	}

	var b strings.Builder
	ln := func(line string) { b.WriteString(line); b.WriteString("\n") }

	// Header comment block.
	ln("#!/usr/bin/env bash")
	ln("# " + t.name + " — generated wrapper for MCP tool: " + t.name)
	ln("# " + desc)
	ln("# Parameters (from JSON Schema):")
	for _, p := range params {
		req, vals := paramSuffix(p)
		if p.desc != "" || req != "" || vals != "" {
			ln("#   " + flagLabel(p, aliases) + req + "  " + oneLine(p.desc) + vals)
		}
	}
	ln("# Server: " + url)
	ln("# Generated: " + time.Now().Format(time.RFC3339))
	ln("")

	// Preamble + arg parsing.
	ln("set -euo pipefail")
	ln(`SESSION_ID="${ZENMCP_SESSION_ID:-zen-cli-$$}"`)
	ln(`SHARED_WS=$(curl -sf "` + url + `/shared/workspace-root" 2>/dev/null | jq -r .value 2>/dev/null || true)`)
	ln(`WORKSPACE="${ZENMCP_WORKSPACE_ROOT:-${SHARED_WS:-$(pwd)}}"`)
	ln(`TOOL="` + t.name + `"`)
	ln(`RAW_JSON=""`)
	ln(`declare -A PARAMS`)
	ln("")
	ln(`while [[ $# -gt 0 ]]; do`)
	ln(`  case "$1" in`)
	ln(`    --json)  RAW_JSON="$2"; shift 2 ;;`)
	ln(`    --help|-h)`)
	for _, l := range buildHelpStatementsOpt(t, url, aliases) {
		ln("      " + l)
	}
	ln(`      ;;`)
	for _, p := range params {
		if a, ok := aliases[p.key]; ok {
			ln(`    -` + a + `) key="` + p.key + `"; PARAMS["$key"]="$2"; shift 2 ;;`)
		}
	}
	ln(`    --*)`)
	ln(`      key="${1#--}"; PARAMS["$key"]="$2"; shift 2 ;;`)
	ln(`    *) echo "Unknown arg: $1" >&2; exit 1 ;;`)
	ln(`  esac`)
	ln(`done`)
	ln("")

	// Injection-safe JSON build via jq.
	ln(`# Build arguments JSON safely via jq (no string concatenation — injection-safe)`)
	ln(`if [[ -n "$RAW_JSON" ]]; then`)
	ln(`  ARGS_JSON="$RAW_JSON"`)
	ln(`else`)
	ln(`  ARGS_JSON=$(`)
	ln(`    for k in "${!PARAMS[@]}"; do printf "%s\0%s\0" "$k" "${PARAMS[$k]}"; done \`)
	ln(`    | jq -Rsc 'split("\u0000") | . as $a | [range(0; length-1; 2)] | map({key: $a[.], value: $a[.+1]}) | from_entries'`)
	ln(`  )`)
	ln(`fi`)
	ln("")
	ln(`PAYLOAD=$(jq -n --arg tool "$TOOL" --argjson args "$ARGS_JSON" '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":$tool,"arguments":$args}}')`)
	ln("")

	// Execute with curl failure diagnostics.
	ln(`# Execute — distinguish common curl failure modes`)
	ln(`RESPONSE=$(curl -sf --max-time "${ZENMCP_TIMEOUT:-60}" \`)
	ln(`  -X POST "` + url + `/mcp" \`)
	ln(`  -H "Content-Type: application/json" \`)
	ln(`  -H "Accept: application/json, text/event-stream" \`)
	ln(`  -H "mcp-session-id: $SESSION_ID" \`)
	ln(`  -H "x-workspace-root: $WORKSPACE" \`)
	ln(`  -d "$PAYLOAD" 2>&1) || {`)
	ln(`  code=$?`)
	ln(`  case $code in`)
	ln(`    7)  echo "Error: connection refused — is zenmcp running at ` + url + `?" >&2 ;;`)
	ln(`    28) echo "Error: request timed out (${ZENMCP_TIMEOUT:-60}s) — server may be busy" >&2 ;;`)
	ln(`    22) echo "Error: HTTP error from server" >&2 ;;`)
	ln(`    *)  echo "Error: curl failed (exit code $code)" >&2 ;;`)
	ln(`  esac`)
	ln(`  exit 1`)
	ln(`}`)
	ln("")

	// Unwrap the MCP content envelope.
	ln(`# Unwrap MCP content envelope; fall back to raw JSON if shape differs`)
	ln(`echo "$RESPONSE" | jq -e 'if .result.content then .result.content[] | if .type == "text" then .text else . end elif .result then .result elif .error then error(.error.message) else . end' || echo "$RESPONSE"`)

	return b.String()
}

// writeAtomic writes dest via a temp file + rename so a partially generated
// wrapper is never observed, mirroring the TS writeAtomic helper.
func writeAtomic(dest, content string) error {
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// removeStaleZen deletes zen-* artifacts in dir that are not in keep.
func removeStaleZen(dir string, keep map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "zen-") {
			continue
		}
		if keep[name] {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// localBinDir returns ~/.local/bin, or "" when the home dir is unavailable.
func localBinDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "bin")
	}
	return ""
}

func localBinDirLabel() string {
	if d := localBinDir(); d != "" {
		return d
	}
	return "~/.local/bin (unavailable)"
}

// pathContains reports whether dir is present in PATH.
func pathContains(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == dir {
			return true
		}
	}
	return false
}
