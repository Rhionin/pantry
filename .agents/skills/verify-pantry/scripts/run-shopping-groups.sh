#!/usr/bin/env bash
# One-shot proof for the group shopping plan.
# Seed runs once at startup, so products are loaded, the seed flag is cleared,
# and the server is started again before the browser drive.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
SKILL_DIR="$REPO_ROOT/.agents/skills/verify-pantry"
FRONTEND_DIR="$REPO_ROOT/frontend"
RUN_DIR="$SKILL_DIR/evidence/shopping-groups"
SERVER_BIN="$SKILL_DIR/.bin/pantry-server"
API_PORT="${API_PORT:-18080}"
WEB_PORT="${WEB_PORT:-5173}"
API_URL="http://127.0.0.1:${API_PORT}"
WEB_URL="http://127.0.0.1:${WEB_PORT}"
DB_PATH="$RUN_DIR/pantry.db"
API_PID=""
WEB_PID=""

export PATH="${HOME}/sdk/go/bin:${PATH}"
export GOTOOLCHAIN=local

log() { printf '[shopping-groups] %s\n' "$*" >&2; }

cleanup() {
  if [[ -n "$WEB_PID" ]] && kill -0 "$WEB_PID" 2>/dev/null; then
    kill "$WEB_PID" 2>/dev/null || true
    pkill -P "$WEB_PID" 2>/dev/null || true
  fi
  if [[ -n "$API_PID" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
  fi
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

wait_for_url() {
  local url="$1" label="$2" timeout="${3:-60}" i=0
  while (( i < timeout )); do
    if curl -sf -o /dev/null "$url"; then
      log "$label is ready"
      return 0
    fi
    sleep 1
    (( i++ ))
  done
  log "ERROR: $label did not become ready at $url"
  return 1
}

start_api() {
  if [[ -n "$API_PID" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  DB_PATH="$DB_PATH" ADDR="127.0.0.1:${API_PORT}" DISABLE_EXTERNAL_PRODUCT_LOOKUP=true \
    "$SERVER_BIN" >>"$RUN_DIR/api.log" 2>&1 &
  API_PID=$!
  wait_for_url "$API_URL/health" "go server" 30
}

mkdir -p "$RUN_DIR"
rm -f "$DB_PATH" "$RUN_DIR"/pantry-pre-groups-*.db "$RUN_DIR"/api.log
log "building server"
( cd "$REPO_ROOT" && go build -o "$SERVER_BIN" ./cmd/server )
start_api
PANTRY_API_URL="$API_URL" node "$SKILL_DIR/scripts/prepare-shopping-groups.mjs"
log "stopping server so suggestions can be seeded from the products just added"
kill "$API_PID"
wait "$API_PID" 2>/dev/null || true
API_PID=""
python3 - "$DB_PATH" <<'PY'
import sqlite3, sys
con = sqlite3.connect(sys.argv[1])
con.execute("DELETE FROM app_settings WHERE key = 'group_suggestions_seeded_v1'")
con.commit()
PY
start_api
if ! grep -q "wrote shopping backup:" "$RUN_DIR/api.log"; then
  log "ERROR: shopping backup was not logged"
  exit 1
fi
log "starting vite"
( cd "$FRONTEND_DIR" && VITE_API_PROXY_TARGET="$API_URL" \
    npm run dev -- --host 127.0.0.1 --port "$WEB_PORT" --strictPort ) \
  >"$RUN_DIR/vite.log" 2>&1 &
WEB_PID=$!
wait_for_url "$WEB_URL" "vite" 60
PANTRY_WEB_URL="$WEB_URL" PANTRY_API_URL="$API_URL" PANTRY_EVIDENCE_DIR="$RUN_DIR" \
  PANTRY_DB_PATH="$DB_PATH" \
  node "$SKILL_DIR/scripts/drive-shopping-groups.mjs"
exit $?
