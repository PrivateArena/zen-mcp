# AGENTS

## Session IDs

The MCP server runs in **stateless mode** (`internal/server/routes.go:162`). It never accepts, generates, or returns MCP session IDs. Session ID negotiation is disabled because it wastes tokens on every initialize handshake. Every HTTP request is treated as a fresh session.

## CLI wrappers

Every tool in `AllDefs` (`internal/tools/types.go`) also exists as a generated shell wrapper in `cli/` (`zfetch`, `zrequest`, ...) plus a `~/.local/bin` symlink. The generator is `internal/terminal/exportcli.go`.

After adding, renaming, or changing the schema of a tool, regenerate the wrappers — otherwise the `z*` command for it does not exist. The tool takes no positional arguments and its own script prints help when called bare, so state the flags explicitly:

- over MCP/CLI: `zexport_cli --clean false` (`--short=false` for long-only flags, `--clean=true` to remove wrappers)
- in the terminal REPL: `export-cli [--short|--long] [--clean]`

Both paths run `terminal.RunExportCLI`, which is also what `tools.Deps.ExportCLI` injects, so an agent can refresh the CLI itself. It writes `./cli` of the **running server's** working directory and rebuilds every wrapper, so it is safe to re-run. Regenerate after changing the generated preamble too — the wrapper scripts are checked-in artifacts of the generator.

Each wrapper decides which project it is calling for in this order:
`ZENMCP_WORKSPACE_ROOT` → the nearest `.zenmcp/config.json` walking up from `pwd` → the workspace the server was last pointed at (`zworkspace`, or an `initialize` rootUri) → `pwd`. `--dry-run` prints the resolved `workspace` and `workspace_src`; `ZENMCP_DEBUG_WORKSPACE=1` says so on stderr for real calls.

## Two-tier config

`config.json` in the zen-mcp folder is the machine-wide tier; `<workspace>/.zenmcp/config.json` is a project overlay deep-merged on top of it — any nested key, siblings inherited, a nested `null` resetting that value. `mcpcfg.For(workspace)` is the accessor, cached per workspace by content hash; `mcpcfg.Get()` is the global tier alone. A tool gains project scoping by reading `For(workspace)` with the `workspace` string its handler already receives (see `internal/tools/youtube.go`), and `internal/toolstate` layers `enabled_tools` the same way.

The per-workspace `mcpserver` cache outlives the config it was built from, so `RouteDeps.RefreshWorkspace` (`main.go workspaceRegistries`) re-applies a workspace's tool states whenever a request hits an already-cached server. Each cached workspace owns its own `toolregistry` — sharing one let the last-registered project decide `enabled_tools` for all of them.

## Auto-restart

The server auto-restarts via `run.sh`. Manual restart is not needed during development.

[FORBIDDEN] DO NOT KILL THE SERVER, JUST EDIT CODE AND IT WILL RESTART. DO NOT ASSUME THAT THE SERVER FAIL TO RESTART/COMPILE, IF THE CODE IS CORRECT IT ALWAYS RECOMPILE WITH FRESH BINARY.
