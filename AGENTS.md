# AGENTS

## Session IDs

The MCP server runs in **stateless mode** (`internal/server/routes.go:162`). It never accepts, generates, or returns MCP session IDs. Session ID negotiation is disabled because it wastes tokens on every initialize handshake. Every HTTP request is treated as a fresh session.

## Auto-restart

The server auto-restarts via `.air.toml` (if present). Manual restart is not needed during development.
