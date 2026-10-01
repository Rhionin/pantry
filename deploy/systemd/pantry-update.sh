#!/usr/bin/env bash
# Pull and recreate the Pantry stack.
#
# When PUBLIC_HOST is set, the public HTTPS proxy (compose profile "public")
# is part of the same update. A plain `docker compose up` omits profiled
# services, which would leave certificate renewal and proxy config out of
# automatic updates. A LAN-only install leaves PUBLIC_HOST empty, so Caddy
# is never started.

set -euo pipefail

cd /opt/pantry

compose=(docker compose --project-directory /opt/pantry -f /opt/pantry/docker-compose.yml)
if [[ -f .env ]]; then
  public_host="$({ grep '^PUBLIC_HOST=' .env || true; } | head -1 | cut -d= -f2- | tr -d '[:space:]')"
  if [[ -n "$public_host" ]]; then
    compose+=(--profile public)
  fi
fi

"${compose[@]}" pull
"${compose[@]}" up -d
