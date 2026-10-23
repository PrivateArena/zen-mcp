# AGENTS

## Session IDs

The MCP server runs in **stateless mode** (`internal/server/routes.go:162`). It never accepts, generates, or returns MCP session IDs. Session ID negotiation is disabled because it wastes tokens on every initialize handshake. Every HTTP request is treated as a fresh session.

## CLI wrappers

Every tool in `AllDefs` (`internal/tools/types.go`) also exists as a generated shell wrapper in `cli/` (`zfetch`, `zrequest`, ...) plus a `~/.local/bin` symlink. The generator is `internal/terminal/exportcli.go`.

After adding, renaming, or changing the schema of a tool, regenerate the wrappers — otherwise the `z*` command for it does not exist:

- over MCP/CLI: `zexport_cli` (`--short=false` for long-only flags, `--clean=true` to remove wrappers)
- in the terminal REPL: `export-cli [--short|--long] [--clean]`

Both paths run `terminal.RunExportCLI`, which is also what `tools.Deps.ExportCLI` injects, so an agent can refresh the CLI itself. It writes `./cli` of the **running server's** working directory and rebuilds every wrapper, so it is safe to re-run.

## Auto-restart

The server auto-restarts via `run.sh`. Manual restart is not needed during development.

[FORBIDDEN] DO NOT KILL THE SERVER, JUST EDIT CODE AND IT WILL RESTART. DO NOT ASSUME THAT THE SERVER FAIL TO RESTART/COMPILE, IF THE CODE IS CORRECT IT ALWAYS RECOMPILE WITH FRESH BINARY.
