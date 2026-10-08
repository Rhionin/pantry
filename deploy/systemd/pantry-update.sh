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
#
# A non-empty CLOUDFLARE_TUNNEL_TOKEN selects the tunnel profile instead.
# That profile does not publish ports 80 or 443. The certificate profile
# is stopped so a token cannot leave both paths running.

set -euo pipefail

cd /opt/pantry
# Read by publish-mode.sh after it is sourced.
# shellcheck disable=SC2034
PANTRY_DIR=/opt/pantry
# shellcheck disable=SC1091
source /opt/pantry/publish-mode.sh

compose=(docker compose --project-directory /opt/pantry -f /opt/pantry/docker-compose.yml)
mode_rc=0
mode=$(publish_mode_from_env) || mode_rc=$?
if [[ "$mode_rc" -eq 2 ]]; then
  echo "Tunnel mode needs CLOUDFLARE_TUNNEL_TOKEN in /opt/pantry/.env. The public proxy was left unchanged. Run: sudo ./setup.sh publish --tunnel" >&2
  exit 1
elif [[ "$mode_rc" -ne 0 ]]; then
  echo "PUBLISH_MODE must be acme or tunnel. The public proxy was left unchanged." >&2
  exit 1
fi

if [[ "$mode" == tunnel ]]; then
  if [[ ! -f /opt/pantry/auth.caddy ]]; then
    echo "CLOUDFLARE_TUNNEL_TOKEN is set but /opt/pantry/auth.caddy is missing; leaving the tunnel stopped. Run: sudo ./setup.sh publish --tunnel" >&2
  else
    compose+=(-f /opt/pantry/docker-compose.tunnel.yml --profile tunnel)
  fi
elif [[ "$mode" == acme ]]; then
  if [[ -f /opt/pantry/auth.caddy ]]; then
    compose+=(--profile public)
  else
    echo "PUBLIC_HOST is set but /opt/pantry/auth.caddy is missing; leaving the public proxy stopped. Run: sudo ./setup.sh" >&2
  fi
fi

"${compose[@]}" pull
# After a successful pull, drop a container left from the other mode so
# the name pantry-caddy can be recreated on this path. A tunnel token with
# no password file leaves whatever is already running alone.
if [[ "$mode" != tunnel || -f /opt/pantry/auth.caddy ]]; then
  drop_other_publish_containers "$mode"
fi
"${compose[@]}" up -d
