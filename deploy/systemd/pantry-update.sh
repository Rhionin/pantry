#!/usr/bin/env bash
# Pull and recreate the Pantry stack.
#
# When PUBLIC_HOST is set and auth.caddy exists, the public HTTPS proxy
# (compose profile "public") is part of the same update. A plain
# `docker compose up` omits profiled services, which would leave certificate
# renewal and proxy config out of automatic updates. A LAN-only install
# leaves PUBLIC_HOST empty, so Caddy is never started. Without auth.caddy,
# starting the profile would make Docker create a directory at that path
# and the proxy would not be password-protected.

set -euo pipefail

cd /opt/pantry

compose=(docker compose --project-directory /opt/pantry -f /opt/pantry/docker-compose.yml)
if [[ -f .env ]]; then
  public_host="$({ grep '^PUBLIC_HOST=' .env || true; } | head -1 | cut -d= -f2- | tr -d '[:space:]')"
  if [[ -n "$public_host" && -f /opt/pantry/auth.caddy ]]; then
    compose+=(--profile public)
  elif [[ -n "$public_host" ]]; then
    echo "PUBLIC_HOST is set but /opt/pantry/auth.caddy is missing; leaving the public proxy stopped. Run: sudo ./setup.sh" >&2
  fi
fi

"${compose[@]}" pull
"${compose[@]}" up -d
