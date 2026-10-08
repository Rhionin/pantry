#!/usr/bin/env bash
# How this Pi publishes the public hostname.
#
# Prints acme, tunnel, or none. Returns 2 when tunnel mode was chosen but
# CLOUDFLARE_TUNNEL_TOKEN is empty, and 1 when PUBLISH_MODE is not acme or
# tunnel. A non-empty token selects tunnel, so a later setup cannot open
# ports 80 and 443 while the token is still in .env. With the token empty
# and PUBLISH_MODE unset, the result is the certificate path.

# Service URL saved on the tunnel's public hostname in Zero Trust.
# Read by setup.sh after this file is sourced.
# shellcheck disable=SC2034
PANTRY_TUNNEL_ORIGIN=http://caddy:80

publish_env_value() {
  { grep "^${1}=" "${PANTRY_DIR}/.env" 2>/dev/null || true; } | head -1 | cut -d= -f2- | tr -d '[:space:]'
}

# publish_mode_from_env REQUESTED
# REQUESTED is "tunnel" when setup was invoked with --tunnel.
publish_mode_from_env() {
  local requested="${1:-}" token mode host
  token=$(publish_env_value CLOUDFLARE_TUNNEL_TOKEN)
  mode=$(publish_env_value PUBLISH_MODE)
  mode=$(printf '%s' "$mode" | tr '[:upper:]' '[:lower:]')
  if [[ -n "$mode" && "$mode" != acme && "$mode" != tunnel ]]; then
    printf 'PUBLISH_MODE must be acme or tunnel (got: %s)\n' "$mode" >&2
    return 1
  fi
  if [[ "$requested" == tunnel || "$mode" == tunnel || -n "$token" ]]; then
    if [[ -z "$token" ]]; then
      return 2
    fi
    printf 'tunnel'
    return 0
  fi
  host=$(publish_env_value PUBLIC_HOST)
  if [[ -n "$host" ]]; then
    printf 'acme'
    return 0
  fi
  printf 'none'
}

# drop_other_publish_containers MODE removes the proxy that belongs to the
# other mode. Both modes use the container name pantry-caddy, and Compose
# will not replace a container labeled for the other service.
drop_other_publish_containers() {
  local mode="$1" svc
  svc=$(docker inspect -f '{{index .Config.Labels "com.docker.compose.service"}}' pantry-caddy 2>/dev/null || true)
  svc=${svc//$'\r'/}
  svc=${svc//$'\n'/}
  if [[ "$mode" == tunnel && "$svc" == caddy ]]; then
    echo "[info] Removing the port-forward proxy so nothing listens on ports 80 or 443"
    docker rm -f pantry-caddy >/dev/null
  elif [[ "$mode" != tunnel && "$svc" == caddy-tunnel ]]; then
    echo "[info] Removing the tunnel proxy so Caddy can listen on ports 80 and 443"
    docker rm -f pantry-caddy >/dev/null
  fi
  if [[ "$mode" != tunnel ]]; then
    docker rm -f pantry-cloudflared >/dev/null 2>&1 || true
  fi
}

# drop_stale_cloudflared removes pantry-cloudflared when a later
# `compose up` would not place it at 10.77.77.2.
#
# Compose recreates cf-tunnel when the IPAM config changes, so a network
# left by a failed start does not have to be deleted here. It does not
# recreate a cloudflared container whose service spec is unchanged. A
# container left Created ("Address already in use") is reconnected with
# a dynamic address instead of the pin. A running connector already on
# 10.77.77.2 is left alone. If inspect cannot show an address, a running
# container is left alone rather than interrupting a live tunnel.
drop_stale_cloudflared() {
  local running ips
  running=$(docker inspect -f '{{.State.Running}}' pantry-cloudflared 2>/dev/null || true)
  running=${running//$'\r'/}
  running=${running//$'\n'/}
  if [[ -z "$running" ]]; then
    return 0
  fi
  if [[ "$running" == true ]]; then
    ips=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{"\n"}}{{end}}' pantry-cloudflared 2>/dev/null || true)
    if printf '%s\n' "$ips" | grep -qx '10.77.77.2'; then
      return 0
    fi
    if ! printf '%s\n' "$ips" | grep -q '^[0-9]'; then
      return 0
    fi
    echo "[info] Removing pantry-cloudflared so it can take 10.77.77.2"
    docker rm -f pantry-cloudflared >/dev/null
    return 0
  fi
  echo "[info] Removing pantry-cloudflared left from a failed start so it can take 10.77.77.2"
  docker rm -f pantry-cloudflared >/dev/null
}
