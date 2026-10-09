#!/usr/bin/env bash
# Pull and recreate the Pantry stack.
#
# When PUBLIC_HOST is set and auth.caddy exists, the public HTTPS proxy
# (compose profile "public") is part of the same update. A plain
# `docker compose up` omits profiled services, which would leave certificate
# renewal and proxy config out of automatic updates. A LAN-only install
# leaves PUBLIC_HOST empty, so Caddy is never started. Without auth.caddy,
# starting the profile would publish the hostname with no password file.
# The hash is copied into auth/household before containers are recreated,
# and an existing PANTRY_SESSION_SECRET is kept.
#
# A non-empty CLOUDFLARE_TUNNEL_TOKEN selects the tunnel profile instead.
# That profile does not publish ports 80 or 443. The certificate profile
# is stopped so a token cannot leave both paths running.
#
# The timer and the deploy-hook path unit both start this script. A lock
# keeps those from pulling at the same time. If a hook lands while a pull
# is already running, the running copy sees the new trigger file and pulls
# again instead of dropping it.
#
# A trigger that names a full commit also applies setup from that commit
# before the pull, when the commit is on origin/master. The same lock covers
# that apply, so two hooks cannot run setup.sh at once. PANTRY_AUTO_SETUP=off
# skips the apply and keeps the image pull.

set -euo pipefail

cd /opt/pantry
# Read by publish-mode.sh after it is sourced.
# shellcheck disable=SC2034
PANTRY_DIR=/opt/pantry
# shellcheck disable=SC1091
source /opt/pantry/publish-mode.sh
# shellcheck disable=SC1091
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/pantry-setup.sh"

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

# uid 65532 is the distroless nonroot user that writes the trigger file.
mkdir -p /opt/pantry/deploy-trigger
chown 65532:65532 /opt/pantry/deploy-trigger
chmod 755 /opt/pantry/deploy-trigger

exec 9>/opt/pantry/deploy-trigger/.lock
if ! flock -n 9; then
  echo "pantry update: another update is running; waiting so this trigger is not dropped"
  flock 9
fi
echo "pantry update: lock acquired"

# shellcheck disable=SC1091
source /opt/pantry/auth-migrate.sh
if ! sync_household_credential; then
  echo "Could not copy the household password hash; leaving containers unchanged." >&2
  exit 1
fi
if [[ -f /opt/pantry/auth.caddy ]]; then
  if ! ensure_session_secret; then
    echo "Could not save the session secret; leaving containers unchanged." >&2
    exit 1
  fi
  if [[ ! -s /opt/pantry/auth/household ]]; then
    echo "auth.caddy is present but the login hash was not copied; leaving containers unchanged." >&2
    exit 1
  fi
fi

trigger_digest() {
  if [[ -f /opt/pantry/deploy-trigger/request ]]; then
    sha256sum /opt/pantry/deploy-trigger/request | awk '{print $1}'
  fi
}

apply_compose() {
  "${compose[@]}" pull
  # After a successful pull, drop a container left from the other mode so
  # the name pantry-caddy can be recreated on this path. A tunnel token with
  # no password file leaves whatever is already running alone.
  if [[ "$mode" != tunnel || -f /opt/pantry/auth.caddy ]]; then
    drop_other_publish_containers "$mode"
  fi
  if [[ "$mode" == tunnel && -f /opt/pantry/auth.caddy ]]; then
    drop_stale_cloudflared
  fi
  "${compose[@]}" up -d
}

round=0
while true; do
  round=$((round + 1))
  if [[ "$round" -gt 3 ]]; then
    echo "pantry update: stopping after 3 pulls; the timer will retry"
    exit 0
  fi
  before=$(trigger_digest || true)
  if [[ -n "$before" ]]; then
    echo "pantry update: deploy trigger $(tr '\n' ' ' < /opt/pantry/deploy-trigger/request)"
  else
    echo "pantry update: scheduled pull"
  fi
  # Setup runs under the lock acquired above, then the image pull. A setup
  # failure rolls back inside apply_signed_setup and still reaches this pull.
  apply_signed_setup
  apply_compose
  after=$(trigger_digest || true)
  if [[ "$after" == "$before" ]]; then
    echo "pantry update: finished"
    break
  fi
  echo "pantry update: trigger changed during the pull; pulling again"
done
