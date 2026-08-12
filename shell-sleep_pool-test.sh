#!/usr/bin/env bash
# Integration test: shell → pool round-trip via live MCP HTTP
# Reads elapsedMs from config.json, calls shell with sleep=elapsedMs+20s,
# extracts the returned pool_id, polls until the job completes, and asserts
# the final replay contains "DONE".
set -euo pipefail

MCP="http://127.0.0.1:3001/mcp"

# ── read elapsedMs from config.json (grep + sed, no jq/python dependency) ────
ELAPSED_MS=$(grep -E '"elapsedMs"[[:space:]]*:' config.json | sed -E 's/.*"elapsedMs"[[:space:]]*:[[:space:]]*([0-9]+).*/\1/')
[[ "$ELAPSED_MS" =~ ^[0-9]+$ ]] || { echo "FAIL: invalid elapsedMs: $ELAPSED_MS" >&2; exit 1; }
SLEEP_S=$(( (ELAPSED_MS + 20000) / 1000 ))

echo "[pool-it] elapsedMs=${ELAPSED_MS}ms  sleep=${SLEEP_S}s"

# ── helpers ──────────────────────────────────────────────────────────────────
req_id=1
rpc() { printf '{"jsonrpc":"2.0","id":%d,"method":"%s","params":%s}' "$((req_id++))" "$1" "$2"; }
die()  { echo "FAIL: $*" >&2; exit 1; }

# ── 1. initialize ────────────────────────────────────────────────────────────
curl -sf -m 10 -X POST "$MCP" \
  -H 'Content-Type: application/json' \
  -d "$(rpc initialize '{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"pool-it","version":"1"}}')" \
  >/dev/null

# ── 2. call shell (triggers pooling) ────────────────────────────────────────
resp=$(curl -sf -m 30 -X POST "$MCP" \
  -H 'Content-Type: application/json' \
  -d "$(rpc 'tools/call' "{\"name\":\"shell\",\"arguments\":{\"command\":\"sleep ${SLEEP_S} && echo DONE\"}}")")

# Extract pool_id from the running payload text: parse outer JSON → inner JSON → pool_id
pool_id=$(printf '%s' "$resp" | grep -oE '"pool_id"[[:space:]]*:[[:space:]]*"[^"]+"' | sed -E 's/.*"pool_id"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')
[[ -n "$pool_id" ]] || die "pool_id empty — shell did not pool (elapsedMs=$ELAPSED_MS, sleep=${SLEEP_S}s)"
echo "[pool-it] pool_id=$pool_id"

# ── 3. poll until done ───────────────────────────────────────────────────────
POLL_INTERVAL=5
HARD_TIMEOUT=$(( SLEEP_S + 60 ))
elapsed=0

while (( elapsed < HARD_TIMEOUT )); do
  resp=$(curl -sf -m 30 -X POST "$MCP" \
    -H 'Content-Type: application/json' \
    -d "$(rpc 'tools/call' "{\"name\":\"pool\",\"arguments\":{\"action\":\"poll\",\"pool_id\":\"${pool_id}\"}}")")

  state=$(printf '%s' "$resp" | grep -oE '"status"[[:space:]]*:[[:space:]]*"[^"]+"' | sed -E 's/.*"status"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')
  echo "[pool-it] poll @${elapsed}s  status=$state"

  case "$state" in
    done)
      stdout=$(printf '%s' "$resp" | grep -oE '"stdout"[[:space:]]*:[[:space:]]*"[^"]*"' | sed -E 's/.*"stdout"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/')
      [[ "$stdout" == *"DONE"* ]] || die "stdout missing DONE: $stdout"
      echo "[pool-it] PASS  pool_id=$pool_id  exitCode=$(printf '%s' "$resp" | grep -oE '"exitCode"[[:space:]]*:[[:space:]]*[0-9-]+' | sed 's/.*:[[:space:]]*//')  stdout=${stdout}"
      exit 0
      ;;
    cancelled) die "job cancelled" ;;
    unknown)   die "unknown pool_id — expired or server restarted" ;;
  esac

  sleep "$POLL_INTERVAL"
  elapsed=$(( elapsed + POLL_INTERVAL ))
done

die "timed out after ${HARD_TIMEOUT}s"
