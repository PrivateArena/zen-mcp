#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# run.sh — zen-mcp dev loop
# =============================================================================
# What it does:
#   1. Builds the server binary (`go build -tags fts5 .`).
#   2. Launches the server in the background.
#   3. Watches the source tree for changes.
#   4. On any change to a WATCHED file, debounces, rebuilds, and restarts
#      the server so your edits take effect immediately.
#
# Excluded files/folders never trigger a rebuild — mirroring .air.toml's
# include/ext + exclude_dir/file/regex semantics, so edits to vendored deps,
# test files, generated dirs, etc. don't cause wasteful recompiles.
#
# Usage:
#   ./run.sh                # build + run + watch
#   ./run.sh -- --stdio     # forward extra args to the server binary
#   WATCH=0 ./run.sh        # build once and run in the foreground, no watch
#
# Configuration (all optional, env-overridable):
#   BIN=zen-mcp                    binary name to produce/run
#   PKG=.                          Go package to build (entry point dir)
#   BUILD_TAGS=fts5                Go build tags
#   RUN_WATCH_INCLUDE_EXT=...      space/comma list of extensions to watch
#   RUN_WATCH_EXCLUDE_DIR=...      space/comma list of dirs (rel to root) to skip
#   RUN_WATCH_EXCLUDE_FILE=...     space/comma list of file basenames to skip
#   RUN_WATCH_EXCLUDE_REGEX=...    space/comma list of ERE patterns to skip
#   RUN_WATCH_DELAY_MS=4000        debounce (ms) before rebuilding after a change
#   RUN_WATCH_POLL_MS=1000         how often (ms) the source tree is re-hashed
#
# Example — ignore nothing but add "json" and drop "yaml":
#   RUN_WATCH_INCLUDE_EXT="go mod sum json" RUN_WATCH_EXCLUDE_FILE="yaml yml" ./run.sh
# =============================================================================

# --- binary / build settings ---
BIN="${BIN:-zen-mcp}"
PKG="${PKG:-.}"
BUILD_TAGS="${BUILD_TAGS:-fts5}"

# Runtime args to forward to the server, passed after `--`, e.g.: ./run.sh -- --stdio
ARGS=()
if [[ "${1:-}" == "--" ]]; then
  shift
  ARGS=("$@")
fi

# =============================================================================
# Watch configuration (mirrors .air.toml; env-overridable)
# =============================================================================
# Extensions that trigger a rebuild.
INCLUDE_EXT=(go mod sum yaml yml toml)
# Directories (relative to repo root) that are never watched.
EXCLUDE_DIR=(assets tmp npm vendor build frontend/node_modules internal/cfg/data .git)
# File basenames that are never watched, regardless of extension.
EXCLUDE_FILE=(yaml)
# ERE regexes (matched against each relative file path) that are never watched.
EXCLUDE_REGEX=(_test\.go)
# Debounce in ms: once a change is seen, wait this long before rebuilding so
# editor save bursts don't trigger multiple rebuilds in a row.
DELAY_MS="${RUN_WATCH_DELAY_MS:-4000}"
# Poll interval in ms: how often the source tree is re-hashed.
POLL_MS="${RUN_WATCH_POLL_MS:-1000}"

# Env overrides (space- or comma-separated).
if [[ -n "${RUN_WATCH_INCLUDE_EXT:-}" ]]; then IFS=' ,' read -r -a INCLUDE_EXT <<< "$RUN_WATCH_INCLUDE_EXT"; fi
if [[ -n "${RUN_WATCH_EXCLUDE_DIR:-}" ]]; then IFS=' ,' read -r -a EXCLUDE_DIR <<< "$RUN_WATCH_EXCLUDE_DIR"; fi
if [[ -n "${RUN_WATCH_EXCLUDE_FILE:-}" ]]; then IFS=' ,' read -r -a EXCLUDE_FILE <<< "$RUN_WATCH_EXCLUDE_FILE"; fi
if [[ -n "${RUN_WATCH_EXCLUDE_REGEX:-}" ]]; then IFS=' ,' read -r -a EXCLUDE_REGEX <<< "$RUN_WATCH_EXCLUDE_REGEX"; fi

# =============================================================================
# Build the find matchers from the include/exclude config above.
# NAME_ARGS   -> `-name '*.go' -o -name '*.mod' ...`  (files that are watched)
# EXCLUDE_ARGS -> `! -path './vendor/*' ! -name 'yaml' ...` (never watched)
# =============================================================================
NAME_ARGS=()
for ext in "${INCLUDE_EXT[@]}"; do NAME_ARGS+=( -o -name "*.$ext" ); done
if [[ "${#NAME_ARGS[@]}" -eq 0 ]]; then NAME_ARGS=( -o -name '*' ); fi   # nothing listed = watch all
NAME_ARGS=( "${NAME_ARGS[@]:1}" )   # drop the leading `-o` from the loop

EXCLUDE_ARGS=()
for d in "${EXCLUDE_DIR[@]}"; do EXCLUDE_ARGS+=( ! -path "./$d/*" ); done
for f in "${EXCLUDE_FILE[@]}"; do EXCLUDE_ARGS+=( ! -name "$f" ); done
for re in "${EXCLUDE_REGEX[@]}"; do EXCLUDE_ARGS+=( ! -regex ".*$re.*" ); done

# =============================================================================
# src_hash — snapshot of the watched tree.
# Only files matching INCLUDE_EXT and NOT excluded reach this hash, so edits
# to excluded files/folders never change it and never trigger a rebuild.
# (Hashes on mtime + path, order-independent via sort.)
# =============================================================================
src_hash() {
  find . -type f \( "${NAME_ARGS[@]}" \) "${EXCLUDE_ARGS[@]}" \
    -print0 2>/dev/null | sort -z | xargs -0 stat --format='%Y %n' 2>/dev/null | md5sum | cut -d' ' -f1
}

# =============================================================================
# restart — stop the current server (if any) and launch a fresh one.
# Uses SIGTERM first; zen-mcp's shutdown handler exits cleanly on it.
# =============================================================================
restart() {
  if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "⏹  Stopping ${BIN} (pid ${SERVER_PID})"
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  echo "🚀 Starting ${BIN} ${ARGS[*]}"
  "./${BIN}" "${ARGS[@]}" &
  SERVER_PID=$!
}

# =============================================================================
# build_and_run — rebuild the binary, then restart the server.
# On a failed build the previous binary is left running (like air's
# stop_on_error=false) so you keep working while fixing the error.
# =============================================================================
build_and_run() {
  echo "🔨 Building ${BIN}..."
  if ! go build -tags "${BUILD_TAGS}" -o "${BIN}" "${PKG}"; then
    echo "❌ Build failed — keeping the last good binary running"
    return
  fi
  echo "✅ Build complete"
  restart
}

# =============================================================================
# Modes
# =============================================================================
# WATCH=0: build once and run in the foreground (no watch loop, no restart).
if [[ "${WATCH:-1}" == "0" ]]; then
  go build -tags "${BUILD_TAGS}" -o "${BIN}" "${PKG}"
  exec "./${BIN}" "${ARGS[@]}"
fi

# The watch loop re-hashes relative to the project root, so refuse to run
# anywhere else.
if [[ ! -f config.json ]]; then
  echo "❌ config.json not found — run this from the project root" >&2
  exit 1
fi

echo "👀 Watching for changes (Ctrl+C to stop) — poll ${POLL_MS}ms, debounce ${DELAY_MS}ms"
echo "   include_ext: ${INCLUDE_EXT[*]} | exclude_dir: ${EXCLUDE_DIR[*]}"

# =============================================================================
# Watch loop
# =============================================================================
SERVER_PID=""
cleanup() {
  # On Ctrl+C / SIGTERM, stop the server child before exiting.
  if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
  fi
  exit 0
}
trap cleanup INT TERM EXIT

# Initial build + launch, then record the baseline hash.
build_and_run
LAST_HASH="$(src_hash)"

while true; do
  HASH="$(src_hash)"
  if [[ "$HASH" != "$LAST_HASH" ]]; then
    echo ""
    echo "🔍 Change detected"
    # Debounce: let a burst of edits settle, then re-check before rebuilding.
    sleep "$(( (DELAY_MS + 999) / 1000 ))"
    HASH="$(src_hash)"
    if [[ "$HASH" != "$LAST_HASH" ]]; then
      LAST_HASH="$HASH"
      build_and_run
    fi
  fi
  sleep "$(( (POLL_MS + 999) / 1000 ))"
done
