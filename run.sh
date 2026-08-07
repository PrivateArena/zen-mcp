#!/usr/bin/env bash
set -euo pipefail

BIN="${BIN:-zen-mcp}"
PKG="${PKG:-.}"
BUILD_TAGS="${BUILD_TAGS:-fts5}"

ARGS=()
if [[ "${1:-}" == "--" ]]; then
  shift
  ARGS=("$@")
fi

INCLUDE_EXT=(go mod sum yaml yml toml)
EXCLUDE_DIR=(assets tmp npm vendor build frontend/node_modules internal/cfg/data .git)
EXCLUDE_FILE=(yaml)
EXCLUDE_REGEX=(_test\.go)
DELAY_MS="${RUN_WATCH_DELAY_MS:-4000}"
POLL_MS="${RUN_WATCH_POLL_MS:-1000}"

if [[ -n "${RUN_WATCH_INCLUDE_EXT:-}" ]]; then IFS=' ,' read -r -a INCLUDE_EXT <<< "$RUN_WATCH_INCLUDE_EXT"; fi
if [[ -n "${RUN_WATCH_EXCLUDE_DIR:-}" ]]; then IFS=' ,' read -r -a EXCLUDE_DIR <<< "$RUN_WATCH_EXCLUDE_DIR"; fi
if [[ -n "${RUN_WATCH_EXCLUDE_FILE:-}" ]]; then IFS=' ,' read -r -a EXCLUDE_FILE <<< "$RUN_WATCH_EXCLUDE_FILE"; fi
if [[ -n "${RUN_WATCH_EXCLUDE_REGEX:-}" ]]; then IFS=' ,' read -r -a EXCLUDE_REGEX <<< "$RUN_WATCH_EXCLUDE_REGEX"; fi

NAME_ARGS=()
for ext in "${INCLUDE_EXT[@]}"; do NAME_ARGS+=( -o -name "*.$ext" ); done
if [[ "${#NAME_ARGS[@]}" -eq 0 ]]; then NAME_ARGS=( -o -name '*' ); fi
NAME_ARGS=( "${NAME_ARGS[@]:1}" )

EXCLUDE_ARGS=()
for d in "${EXCLUDE_DIR[@]}"; do EXCLUDE_ARGS+=( ! -path "./$d/*" ); done
for f in "${EXCLUDE_FILE[@]}"; do EXCLUDE_ARGS+=( ! -name "$f" ); done
for re in "${EXCLUDE_REGEX[@]}"; do EXCLUDE_ARGS+=( ! -regex ".*$re.*" ); done

src_hash() {
  find . -type f \( "${NAME_ARGS[@]}" \) "${EXCLUDE_ARGS[@]}" \
    -print0 2>/dev/null | sort -z | xargs -0 stat --format='%Y %n' 2>/dev/null | md5sum | cut -d' ' -f1
}

# --- Preserve the script's original stdin/stdout/stderr on separate fds.
# THIS IS THE FIX: bash redirects an async command's (`cmd &`) stdin to
# /dev/null automatically when job control is off (true inside any script),
# no matter what fd 0 currently is. stdout/stderr are NOT affected by this
# rule, only stdin — which is why output looked fine in earlier attempts
# but the terminal commander never received input. We capture real stdin
# as fd 5 here, then explicitly attach the backgrounded server to it.
exec 3>&1 4>&2 5<&0

SERVER_PID=""

start_server() {
  echo "🚀 Starting ${BIN} ${ARGS[*]}" >&3
  # Prefer the controlling tty if one exists (handles the case where fd 5
  # itself came from a pipe rather than a real terminal); fall back to the
  # preserved original stdin (fd 5) otherwise.
  if [[ -r /dev/tty ]] && [[ -t 0 || -e /dev/tty ]]; then
    "./${BIN}" "${ARGS[@]}" </dev/tty >&3 2>&4 &
  else
    "./${BIN}" "${ARGS[@]}" <&5 >&3 2>&4 &
  fi
  SERVER_PID=$!
}

stop_server() {
  if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "⏹  Stopping ${BIN} (pid ${SERVER_PID})" >&3
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}

build_and_run() {
  echo "🔨 Building ${BIN}..." >&3
  if ! go build -tags "${BUILD_TAGS}" -o "${BIN}.new" "${PKG}"; then
    echo "❌ Build failed — keeping the last good binary running" >&4
    rm -f "${BIN}.new"
    return
  fi
  mv -f "${BIN}.new" "${BIN}"
  echo "✅ Build complete" >&3
  stop_server
  start_server
}

if [[ "${WATCH:-1}" == "0" ]]; then
  go build -tags "${BUILD_TAGS}" -o "${BIN}" "${PKG}"
  exec "./${BIN}" "${ARGS[@]}"
fi

if [[ ! -f config.json ]]; then
  echo "❌ config.json not found — run this from the project root" >&2
  exit 1
fi

printf '%s\n' \
  "👀 Watching for changes (Ctrl+C to stop) — poll ${POLL_MS}ms, debounce ${DELAY_MS}ms" \
  "   include_ext : ${INCLUDE_EXT[*]}" \
  "   exclude_dir : ${EXCLUDE_DIR[*]}" \
  "   exclude_file: ${EXCLUDE_FILE[*]}" \
  "   exclude_regex: ${EXCLUDE_REGEX[*]}" \
  >&3

cleanup() {
  stop_server
  exit 0
}
trap cleanup INT TERM EXIT

build_and_run
LAST_HASH="$(src_hash)"

while true; do
  HASH="$(src_hash)"
  if [[ "$HASH" != "$LAST_HASH" ]]; then
    echo "" >&3
    echo "🔍 Change detected" >&3
    sleep "$(( (DELAY_MS + 999) / 1000 ))"
    HASH="$(src_hash)"
    if [[ "$HASH" != "$LAST_HASH" ]]; then
      LAST_HASH="$HASH"
      build_and_run
    fi
  fi
  sleep "$(( (POLL_MS + 999) / 1000 ))"
done