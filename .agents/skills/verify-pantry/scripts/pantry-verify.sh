#!/usr/bin/env bash
# pantry-verify.sh — the one command that launches Pantry, runs a Playwright
# driver against the real web UI, and tears everything down, all inside a single
# shell invocation.
#
# WHY one script instead of a long-lived server: in this sandbox, background
# processes and /tmp do NOT survive between separate shell calls. Only the
# workspace (the repo) and $HOME (Go build cache, node_modules) persist. So a
# "launch, then drive in a later call" model does not work here. Every drive
# starts its own isolated Pantry, drives it, and tears it down before the shell
# exits. Evidence is written under the repo so it survives.
#
# Usage:
#   pantry-verify.sh doctor
#       Read-only preflight: prints tool versions, confirms the Go server and
#       Vite dev server can start and answer, then tears them down. Proves the
#       checkout is worth driving. Writes nothing outside the evidence dir.
#
#   pantry-verify.sh drive <driver.mjs> [evidence-subdir]
#       Starts Pantry, runs the Playwright driver script, tears down. The driver
#       receives PANTRY_WEB_URL, PANTRY_API_URL, and PANTRY_EVIDENCE_DIR in its
#       environment. Evidence goes to
#       .agents/skills/verify-pantry/evidence/<evidence-subdir> (default: last).
#
# Env overrides (rarely needed; defaults are isolated):
#   API_PORT   default 18080     Go server port (loopback only)
#   WEB_PORT   default 5173      Vite dev server port (loopback only)
#   DB_PATH    default a fresh file under the evidence run dir
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
SKILL_DIR="$REPO_ROOT/.agents/skills/verify-pantry"
FRONTEND_DIR="$REPO_ROOT/frontend"

API_PORT="${API_PORT:-18080}"
WEB_PORT="${WEB_PORT:-5173}"
API_URL="http://127.0.0.1:${API_PORT}"
WEB_URL="http://127.0.0.1:${WEB_PORT}"

SERVER_BIN="$REPO_ROOT/.agents/skills/verify-pantry/.bin/pantry-server"

log() { printf '[pantry-verify] %s\n' "$*" >&2; }

# --- Process tracking. Cleanup kills only PIDs we started, never by name. ---
API_PID=""
WEB_PID=""

cleanup() {
  # Tear down instances this run started. Never touches the evidence dir.
  if [[ -n "$WEB_PID" ]] && kill -0 "$WEB_PID" 2>/dev/null; then
    log "stopping vite (pid $WEB_PID)"
    kill "$WEB_PID" 2>/dev/null
    # Vite spawns esbuild children; kill the process group too.
    pkill -P "$WEB_PID" 2>/dev/null
  fi
  if [[ -n "$API_PID" ]] && kill -0 "$API_PID" 2>/dev/null; then
    log "stopping go server (pid $API_PID)"
    kill "$API_PID" 2>/dev/null
  fi
  wait 2>/dev/null
}
trap cleanup EXIT INT TERM

wait_for_url() {
  # wait_for_url <url> <label> <timeout-seconds>
  local url="$1" label="$2" timeout="${3:-60}" i=0
  while (( i < timeout )); do
    if curl -sf -o /dev/null "$url"; then
      log "$label is ready ($url)"
      return 0
    fi
    sleep 1; (( i++ ))
  done
  log "ERROR: $label did not become ready at $url within ${timeout}s"
  return 1
}

build_server() {
  mkdir -p "$(dirname "$SERVER_BIN")"
  log "building go server -> $SERVER_BIN"
  ( cd "$REPO_ROOT" && go build -o "$SERVER_BIN" ./cmd/server ) || return 1
}

start_api() {
  local db_path="$1"
  log "starting go server on $API_URL (db=$db_path, external lookup disabled)"
  DB_PATH="$db_path" ADDR="127.0.0.1:${API_PORT}" DISABLE_EXTERNAL_PRODUCT_LOOKUP=true \
    "$SERVER_BIN" >"$RUN_DIR/api.log" 2>&1 &
  API_PID=$!
  wait_for_url "$API_URL/health" "go server" 30
}

start_web() {
  log "starting vite dev server on $WEB_URL (proxy -> $API_URL)"
  ( cd "$FRONTEND_DIR" && VITE_API_PROXY_TARGET="$API_URL" \
      npm run dev -- --host 127.0.0.1 --port "$WEB_PORT" --strictPort ) \
    >"$RUN_DIR/vite.log" 2>&1 &
  WEB_PID=$!
  wait_for_url "$WEB_URL" "vite dev server" 60
}

cmd_doctor() {
  RUN_DIR="$SKILL_DIR/evidence/doctor"
  mkdir -p "$RUN_DIR"
  local report="$RUN_DIR/doctor.txt"
  {
    echo "pantry doctor $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "repo: $REPO_ROOT"
    echo "go:   $(go version 2>&1)"
    echo "node: $(node --version 2>&1)"
    echo "npm:  $(npm --version 2>&1)"
    echo "node_modules present: $([[ -d "$FRONTEND_DIR/node_modules" ]] && echo yes || echo no)"
  } | tee "$report"

  build_server || { echo "FAIL: go build" | tee -a "$report"; return 1; }
  local db_path="$RUN_DIR/doctor.db"; rm -f "$db_path"
  start_api "$db_path" || { echo "FAIL: go server did not answer /health" | tee -a "$report"; return 1; }

  local health config
  health="$(curl -sf "$API_URL/health")"
  config="$(curl -sf "$API_URL/api/scanner/config")"
  {
    echo "health:  $health"
    echo "config:  $config"
  } | tee -a "$report"

  if [[ "$health" == *'"status":"ok"'* ]]; then
    echo "PASS: pantry is worth driving" | tee -a "$report"
    return 0
  fi
  echo "FAIL: unexpected health payload" | tee -a "$report"
  return 1
}

cmd_drive() {
  local driver="$1" subdir="${2:-last}"
  if [[ ! -f "$driver" ]]; then
    log "ERROR: driver script not found: $driver"; return 1
  fi
  RUN_DIR="$SKILL_DIR/evidence/$subdir"
  mkdir -p "$RUN_DIR"

  build_server || return 1
  local db_path="$RUN_DIR/pantry.db"; rm -f "$db_path"
  start_api "$db_path" || return 1
  start_web || return 1

  log "driving with $driver (evidence -> $RUN_DIR)"
  PANTRY_WEB_URL="$WEB_URL" PANTRY_API_URL="$API_URL" PANTRY_EVIDENCE_DIR="$RUN_DIR" \
    node "$driver"
  local rc=$?
  log "driver exit code: $rc"
  return $rc
}

main() {
  local sub="${1:-}"
  case "$sub" in
    doctor) cmd_doctor ;;
    drive)  shift; cmd_drive "$@" ;;
    *)
      echo "usage: pantry-verify.sh doctor | drive <driver.mjs> [evidence-subdir]" >&2
      return 2
      ;;
  esac
}

main "$@"
