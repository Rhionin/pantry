#!/usr/bin/env bash
# Start the Pantry API and Vite dev server. Safe to run again if they are already up.
set -euo pipefail

export PATH="/usr/local/go/bin:/usr/local/bin:${PATH}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
log_dir="/tmp/pantry"
mkdir -p "${log_dir}"

api_up() {
  curl -sf -o /dev/null --max-time 2 http://127.0.0.1:8080/health
}

ui_up() {
  curl -sf -o /dev/null --max-time 2 http://127.0.0.1:5173/
}

if ! api_up && ! tmux has-session -t pantry-api 2>/dev/null; then
  tmux new-session -d -s pantry-api \
    "export PATH='${PATH}'; cd '${root}'; exec go run ./cmd/server >>'${log_dir}/api.log' 2>&1"
fi

if ! ui_up && ! tmux has-session -t pantry-ui 2>/dev/null; then
  tmux new-session -d -s pantry-ui \
    "export PATH='${PATH}'; cd '${root}/frontend'; exec npm run dev -- --host 127.0.0.1 --port 5173 >>'${log_dir}/ui.log' 2>&1"
fi

for _ in $(seq 1 90); do
  if api_up && ui_up; then
    echo "pantry api and ui are ready"
    exit 0
  fi
  sleep 2
done

echo "pantry services did not become ready" >&2
echo "--- api log ---" >&2
tail -n 80 "${log_dir}/api.log" >&2 || true
echo "--- ui log ---" >&2
tail -n 80 "${log_dir}/ui.log" >&2 || true
exit 1
