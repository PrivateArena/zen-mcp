#!/usr/bin/env bash
set -euo pipefail

BIN="zen-mcp"
PKG="./cmd/zen"
ARGS=()
BUILD_TAGS="fts5"

# Allow passing runtime args after --
if [[ "${1:-}" == "--" ]]; then
  shift
  ARGS=("$@")
fi

run_binary() {
  echo "🚀 Starting ${BIN} ${ARGS[*]}"
  exec "./${BIN}" "${ARGS[@]}"
}

echo "🔨 Building ${BIN}..."
go build -tags "${BUILD_TAGS}" -o "${BIN}" "${PKG}"
echo "✅ Build complete"

if [[ "${WATCH:-1}" == "0" ]]; then
  run_binary
fi

LAST_BUILD=0
LAST_HASH=""

get_src_hash() {
  find . -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) \
    ! -path './tmp/*' ! -path './frontend/*' ! -path './vendor/*' \
    ! -path './assets/*' ! -path './build/*' \
    -print0 2>/dev/null | sort -z | xargs -0 stat --format='%Y %n' 2>/dev/null | md5sum | cut -d' ' -f1
}

echo "👀 Watching for changes (Ctrl+C to stop)..."

while true; do
  HASH="$(get_src_hash)"
  if [[ "${HASH}" != "${LAST_HASH}" ]]; then
    LAST_HASH="${HASH}"
    echo ""
    echo "🔨 Rebuilding ${BIN}..."
    if ! go build -tags "${BUILD_TAGS}" -o "${BIN}" "${PKG}"; then
      echo "❌ Build failed, will retry on next change"
      sleep 2
      continue
    fi
    echo "✅ Build complete"
  fi
  sleep 2
done &

WATCHER_PID=$!

cleanup() {
  kill "${WATCHER_PID}" 2>/dev/null || true
  exit 0
}
trap cleanup INT TERM EXIT

run_binary
