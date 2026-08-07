#!/usr/bin/env bash
set -euo pipefail

# run.sh — dev loop for the zen-mcp server.
# Rebuilds and RESTARTS the server whenever watched sources change.
# Excluded files/folders never trigger a rebuild (air.toml-style include/exclude).

BIN="${BIN:-zen-mcp}"
PKG="${PKG:-./cmd/zen}"
BUILD_TAGS="${BUILD_TAGS:-fts5}"

# Runtime args to forward to the server, passed after `--`, e.g.: ./run.sh -- --stdio
ARGS=()
if [[ "${1:-}" == "--" ]]; then
  shift
  ARGS=("$@")
fi

# =============================================================================
# Watch configuration (mirrors .air.toml semantics; all env-overridable)
# =============================================================================
# Extensions that trigger a rebuild (space- or comma-separated when overridden).
INCLUDE_EXT=(go mod sum yaml yml toml)
# Directories (relative to repo root) to never watch.
EXCLUDE_DIR=(assets tmp npm vendor build frontend/node_modules internal/cfg/data .git)
# File basenames to ignore regardless of extension.
EXCLUDE_FILE=(yaml)
# ERE regexes matched against each relative file path to ignore.
EXCLUDE_REGEX=(_test\.go)
# ms: debounce before rebuilding after the first change is detected.
DELAY_MS=4000
# ms: grace period between SIGTERM and SIGKILL when restarting the server.
KILL_DELAY_MS=1000
# ms: polling interval.
POLL_MS=1000

if [[ -n "${RUN_WATCH_INCLUDE_EXT:-}" ]]; then
  IFS=' ,' read -r -a INCLUDE_EXT <<< "$RUN_WATCH_INCLUDE_EXT"
fi
if [[ -n "${RUN_WATCH_EXCLUDE_DIR:-}" ]]; then
  IFS=' ,' read -r -a EXCLUDE_DIR <<< "$RUN_WATCH_EXCLUDE_DIR"
fi
if [[ -n "${RUN_WATCH_EXCLUDE_FILE:-}" ]]; then
  IFS=' ,' read -r -a EXCLUDE_FILE <<< "$RUN_WATCH_EXCLUDE_FILE"
fi
if [[ -n "${RUN_WATCH_EXCLUDE_REGEX:-}" ]]; then
  IFS=' ,' read -r -a EXCLUDE_REGEX <<< "$RUN_WATCH_EXCLUDE_REGEX"
fi
if [[ -n "${RUN_WATCH_DELAY_MS:-}" ]]; then DELAY_MS="$RUN_WATCH_DELAY_MS"; fi
if [[ -n "${RUN_WATCH_KILL_DELAY_MS:-}" ]]; then KILL_DELAY_MS="$RUN_WATCH_KILL_DELAY_MS"; fi
if [[ -n "${RUN_WATCH_POLL_MS:-}" ]]; then POLL_MS="$RUN_WATCH_POLL_MS"; fi

# =============================================================================
# Server ports (from config.json, env-overridable)
# =============================================================================
read_config_ports() {
  MCP_PORT="$(python3 -c 'import json,sys
try:
    d=json.load(open(sys.argv[1])); print(d.get("mcpPort",3001))
except Exception: print("3001")' config.json 2>/dev/null)"
  CLI_PORT="$(python3 -c 'import json,sys
try:
    d=json.load(open(sys.argv[1])); print(d.get("cliPort",2999))
except Exception: print("2999")' config.json 2>/dev/null)"
  MCP_PORT="${MCP_PORT:-$(grep -oP '"mcpPort"\s*:\s*\K[0-9]+' config.json 2>/dev/null | head -1)}"
  CLI_PORT="${CLI_PORT:-$(grep -oP '"cliPort"\s*:\s*\K[0-9]+' config.json 2>/dev/null | head -1)}"
  MCP_PORT="${MCP_PORT:-3001}"
  CLI_PORT="${CLI_PORT:-2999}"
  if [[ -n "${RUN_MCP_PORT:-}" ]]; then MCP_PORT="$RUN_MCP_PORT"; fi
  if [[ -n "${RUN_CLI_PORT:-}" ]]; then CLI_PORT="$RUN_CLI_PORT"; fi
}
read_config_ports

port_in_use() {
  local port="$1"
  ss -tln 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${port}$" && return 0
  lsof -iTCP:"$port" -sTCP:LISTEN -t >/dev/null 2>&1 && return 0
  (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null && { exec 3>&- 3<&-; return 0; }
  return 1
}

pid_on_port() {
  local port="$1"
  ss -tlnp 2>/dev/null | grep -E "[:.]${port}\b" | grep -oP 'pid=\K[0-9]+' | head -1
}

# =============================================================================
# Build / run helpers
# =============================================================================
build_binary() {
  echo "🔨 Building ${BIN}..."
  if go build -tags "${BUILD_TAGS}" -o "${BIN}" "${PKG}"; then
    echo "✅ Build complete"
    return 0
  fi
  echo "❌ Build failed — keeping the running server on the last good binary"
  return 1
}

start_binary() {
  echo "🚀 Starting ${BIN} ${ARGS[*]}"
  "./${BIN}" "${ARGS[@]}" &
  SERVER_PID=$!
}

stop_binary() {
  if [[ -z "${SERVER_PID:-}" ]] || ! kill -0 "$SERVER_PID" 2>/dev/null; then
    SERVER_PID=""
    return
  fi
  echo "⏹  Stopping ${BIN} (pid ${SERVER_PID})"
  kill "$SERVER_PID" 2>/dev/null || true
  WAITED=0
  while kill -0 "$SERVER_PID" 2>/dev/null; do
    if (( WAITED >= KILL_DELAY_MS )); then
      echo "⚠️  Force killing ${BIN}"
      kill -9 "$SERVER_PID" 2>/dev/null || true
      break
    fi
    sleep 0.1
    (( WAITED += 100 ))
  done
  wait "$SERVER_PID" 2>/dev/null || true
  SERVER_PID=""
}

# Start the server only if its ports are free; otherwise report the conflict
# (or stop the stale holder when RUN_KILL_STALE=1) instead of crash-looping.
try_start() {
  if port_in_use "$MCP_PORT" || port_in_use "$CLI_PORT"; then
    local p
    p="$(pid_on_port "$MCP_PORT")"
    if [[ -z "$p" ]]; then p="$(pid_on_port "$CLI_PORT")"; fi
    echo "⚠️  Port ${MCP_PORT}/${CLI_PORT} already in use by pid ${p:-unknown} — not starting a second server"
    if [[ "${RUN_KILL_STALE:-0}" == "1" ]]; then
      echo "⏹  RUN_KILL_STALE=1 → stopping stale pid ${p:-unknown}"
      [[ -n "$p" ]] && kill "$p" 2>/dev/null || true
      for _ in {1..20}; do
        port_in_use "$MCP_PORT" || port_in_use "$CLI_PORT" || break
        sleep 0.2
      done
      if port_in_use "$MCP_PORT" || port_in_use "$CLI_PORT"; then
        echo "⚠️  Ports still busy — cannot start"
        return 1
      fi
    else
      echo "   → stop it manually, or re-run with RUN_KILL_STALE=1 to let run.sh take over"
      return 1
    fi
  fi
  start_binary
}

# =============================================================================
# Watch hashing — only watched files contribute to the change hash.
# =============================================================================
# Build the find name matcher from INCLUDE_EXT.
NAME_ARGS=()
for ext in "${INCLUDE_EXT[@]}"; do
  NAME_ARGS+=( -o -name "*.$ext" )
done
if [[ "${#NAME_ARGS[@]}" -eq 0 ]]; then
  NAME_ARGS=( -o -name '*' )
fi
NAME_ARGS=( "${NAME_ARGS[@]:1}" )

# Build the exclusion matcher from EXCLUDE_DIR / EXCLUDE_FILE / EXCLUDE_REGEX.
EXCLUDE_ARGS=()
for dir in "${EXCLUDE_DIR[@]}"; do
  EXCLUDE_ARGS+=( ! -path "./$dir/*" )
done
for f in "${EXCLUDE_FILE[@]}"; do
  EXCLUDE_ARGS+=( ! -name "$f" )
done
for re in "${EXCLUDE_REGEX[@]}"; do
  EXCLUDE_ARGS+=( ! -regex ".*$re.*" )
done

get_src_hash() {
  find . -type f \( "${NAME_ARGS[@]}" \) \
    "${EXCLUDE_ARGS[@]}" \
    -print0 2>/dev/null \
    | sort -z \
    | xargs -0 stat --format='%Y %n' 2>/dev/null \
    | md5sum | cut -d' ' -f1
}

# Debounce, re-check, then rebuild and restart on success.
rebuild_and_restart() {
  sleep "$(( (DELAY_MS + 999) / 1000 ))"
  local h
  h="$(get_src_hash)"
  if [[ "$h" == "$LAST_HASH" ]]; then
    return
  fi
  LAST_HASH="$h"
  if build_binary; then
    CRASH_WINDOW=()
    stop_binary
    try_start
  fi
}

# =============================================================================
# Modes
# =============================================================================
if [[ "${WATCH:-1}" == "0" ]]; then
  build_binary
  echo "🚀 Starting ${BIN} ${ARGS[*]} (no watch)"
  exec "./${BIN}" "${ARGS[@]}"
fi

if [[ ! -f config.json ]]; then
  echo "❌ config.json not found — run this from the zen-mcp project root" >&2
  exit 1
fi

# =============================================================================
# Watch loop
# =============================================================================
echo "👀 Watching for changes (Ctrl+C to stop) — poll ${POLL_MS}ms, debounce ${DELAY_MS}ms"
echo "   server ports: mcp=${MCP_PORT} cli=${CLI_PORT}"
echo "   include_ext: ${INCLUDE_EXT[*]}"
echo "   exclude_dir: ${EXCLUDE_DIR[*]}"
echo "   exclude_file: ${EXCLUDE_FILE[*]}"
echo "   exclude_regex: ${EXCLUDE_REGEX[*]}"

SERVER_PID=""
CRASH_WINDOW=()
cleanup() {
  stop_binary
  exit 0
}
trap cleanup INT TERM EXIT

build_binary && try_start
LAST_HASH="$(get_src_hash)"

while true; do
  HASH="$(get_src_hash)"
  if [[ "$HASH" != "$LAST_HASH" ]]; then
    echo ""
    echo "🔍 Change detected"
    rebuild_and_restart
  fi
  if [[ -n "${SERVER_PID:-}" ]] && ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "⚠️  ${BIN} exited (was pid ${SERVER_PID})"
    SERVER_PID=""
    NOW="$(date +%s)"
    CRASH_WINDOW+=( "$NOW" )
    RECENT=0
    PRUNED=()
    for t in "${CRASH_WINDOW[@]}"; do
      if (( NOW - t <= 5 )); then
        RECENT=$(( RECENT + 1 ))
        PRUNED+=( "$t" )
      fi
    done
    CRASH_WINDOW=( "${PRUNED[@]}" )
    if (( RECENT >= 3 )); then
      echo "⚠️  Server exited ${RECENT}x within 5s — backing off 5s before retry"
      sleep 5
      continue
    fi
    try_start
  fi
  if [[ -z "${SERVER_PID:-}" ]] && ! port_in_use "$MCP_PORT" && ! port_in_use "$CLI_PORT"; then
    try_start
  fi
  sleep "$(( (POLL_MS + 999) / 1000 ))"
done
