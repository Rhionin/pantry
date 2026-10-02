#!/usr/bin/env bash
# Drop packets to Pantry's published port when the source is not loopback,
# a private LAN, or Tailscale's CGNAT range.
#
# Docker publishes that port with a userland proxy. A packet can be accepted
# in the host INPUT chain, or DNATed and forwarded. ufw rules on INPUT do not
# cover the forwarded path, and a DOCKER-USER rule does not cover the proxy.
# This installs both. It does not touch ports 80 or 443.
#
# Applied by `sudo ./setup.sh` when compose publishes the Pantry port or the
# public proxy is on. LAN clients in the ranges below keep working.
# Opt out by setting PANTRY_LAN_FIREWALL=off in /opt/pantry/.env and re-running
# setup. Remove until the next setup with:
#   sudo ./setup.sh firewall-off

set -euo pipefail

PANTRY_DIR="${PANTRY_DIR:-/opt/pantry}"
ENV_FILE="$PANTRY_DIR/.env"
HOST_PORT=8080
CONTAINER_PORT=8080

if [[ -f "$ENV_FILE" ]]; then
  port="$({ grep '^HOST_PORT=' "$ENV_FILE" || true; } | head -1 | cut -d= -f2- | tr -d '[:space:]')"
  if [[ "$port" =~ ^[0-9]+$ ]]; then
    HOST_PORT="$port"
  fi
fi

remove=false
if [[ "${1:-}" == "--remove" ]]; then
  remove=true
fi

allow_v4=(
  127.0.0.0/8
  10.0.0.0/8
  172.16.0.0/12
  192.168.0.0/16
  100.64.0.0/10
)
allow_v6=(
  ::1
  fc00::/7
  fe80::/10
)

delete_jump() {
  local bin="$1" chain="$2" dport="$3"
  if ! command -v "$bin" >/dev/null 2>&1; then
    return 0
  fi
  if "$bin" -C "$chain" -p tcp --dport "$dport" -j PANTRY-LAN 2>/dev/null; then
    "$bin" -D "$chain" -p tcp --dport "$dport" -j PANTRY-LAN
  fi
}

if [[ "$remove" == true ]]; then
  delete_jump iptables INPUT "$HOST_PORT"
  delete_jump iptables DOCKER-USER "$CONTAINER_PORT"
  if command -v iptables >/dev/null 2>&1 && iptables -nL PANTRY-LAN >/dev/null 2>&1; then
    iptables -F PANTRY-LAN
    iptables -X PANTRY-LAN
  fi
  delete_jump ip6tables INPUT "$HOST_PORT"
  delete_jump ip6tables DOCKER-USER "$CONTAINER_PORT"
  if command -v ip6tables >/dev/null 2>&1 && ip6tables -nL PANTRY-LAN >/dev/null 2>&1; then
    ip6tables -F PANTRY-LAN
    ip6tables -X PANTRY-LAN
  fi
  echo "Removed Pantry LAN-only rules for tcp/$HOST_PORT"
  exit 0
fi

if ! command -v iptables >/dev/null 2>&1; then
  echo "iptables is not installed" >&2
  exit 1
fi

rebuild_chain() {
  local bin="$1"
  shift
  local sources=("$@")
  if "$bin" -nL PANTRY-LAN >/dev/null 2>&1; then
    "$bin" -F PANTRY-LAN
  else
    "$bin" -N PANTRY-LAN
  fi
  local src
  for src in "${sources[@]}"; do
    "$bin" -A PANTRY-LAN -s "$src" -j RETURN
  done
  "$bin" -A PANTRY-LAN -j DROP
}

ensure_jump() {
  local bin="$1" chain="$2" dport="$3"
  if ! "$bin" -nL "$chain" >/dev/null 2>&1; then
    echo "$bin chain $chain is not present; skipped tcp/$dport" >&2
    return 0
  fi
  if ! "$bin" -C "$chain" -p tcp --dport "$dport" -j PANTRY-LAN 2>/dev/null; then
    "$bin" -I "$chain" 1 -p tcp --dport "$dport" -j PANTRY-LAN
  fi
}

rebuild_chain iptables "${allow_v4[@]}"
ensure_jump iptables INPUT "$HOST_PORT"
ensure_jump iptables DOCKER-USER "$CONTAINER_PORT"

if command -v ip6tables >/dev/null 2>&1; then
  rebuild_chain ip6tables "${allow_v6[@]}"
  ensure_jump ip6tables INPUT "$HOST_PORT"
  ensure_jump ip6tables DOCKER-USER "$CONTAINER_PORT"
fi

echo "Pantry tcp/$HOST_PORT accepts loopback, private LAN, and Tailscale sources only"
