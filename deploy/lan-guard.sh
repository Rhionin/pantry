#!/usr/bin/env bash
# Limit the published Pantry port to private networks.
#
# Docker publishes HOST_PORT in the DOCKER chain, which is evaluated before
# a host firewall such as ufw. Filtering belongs in DOCKER-USER.
#
# A normal port forward keeps the client's real source address, so a WAN
# client fails the private-network check. Some routers also rewrite that
# source to the LAN gateway. Traffic from the default gateway is therefore
# dropped too: household devices use their own addresses, not the gateway's.
#
# This does not replace "do not forward HOST_PORT". It also does not run
# when iptables or the DOCKER-USER chain is absent.

set -euo pipefail

port=8080
if [[ -f /opt/pantry/.env ]]; then
  configured=$({ grep '^HOST_PORT=' /opt/pantry/.env || true; } | head -1 | cut -d= -f2- | tr -d '[:space:]')
  if [[ -n "$configured" ]]; then
    port=$configured
  fi
fi

if [[ ! "$port" =~ ^[0-9]+$ ]] || (( port < 1 || port > 65535 )); then
  echo "lan-guard: HOST_PORT must be a TCP port number (got: $port)" >&2
  exit 1
fi
if [[ "$port" == "80" || "$port" == "443" ]]; then
  echo "lan-guard: refusing to filter port $port; that port belongs to the public proxy" >&2
  exit 1
fi

if ! command -v iptables >/dev/null 2>&1; then
  echo "lan-guard: iptables is not installed; port $port was not filtered" >&2
  exit 0
fi
if ! iptables -L DOCKER-USER -n >/dev/null 2>&1; then
  echo "lan-guard: DOCKER-USER is not present yet; port $port was not filtered" >&2
  exit 0
fi

delete_guard_rules() {
  local cmd="$1"
  local line args
  local guard=0
  while line=$("$cmd" -S DOCKER-USER 2>/dev/null | grep 'pantry-lan-guard' | head -1); do
    [[ -z "$line" ]] && break
    args=${line#-A DOCKER-USER }
    # shellcheck disable=SC2086
    "$cmd" -D DOCKER-USER $args
    guard=$((guard + 1))
    if (( guard > 40 )); then
      echo "lan-guard: stopped deleting $cmd rules after 40 attempts" >&2
      return 1
    fi
  done
}

# Inserted at position 1, so the last rule added is checked first.
insert_drop() {
  local cmd="$1"
  "$cmd" -I DOCKER-USER 1 -p tcp -m conntrack --ctorigdstport "$port" -m comment --comment pantry-lan-guard -j DROP
}

insert_return() {
  local cmd="$1"
  local source="$2"
  "$cmd" -I DOCKER-USER 1 -p tcp -s "$source" -m conntrack --ctorigdstport "$port" -m comment --comment pantry-lan-guard -j RETURN
}

apply_v4() {
  delete_guard_rules iptables
  insert_drop iptables
  local source
  for source in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 100.64.0.0/10 127.0.0.0/8; do
    insert_return iptables "$source"
  done
  if command -v ip >/dev/null 2>&1; then
    local gateway
    gateway=$(ip -4 route show default 2>/dev/null | awk '{print $3; exit}')
    if [[ "$gateway" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      iptables -I DOCKER-USER 1 -p tcp -s "$gateway" -m conntrack --ctorigdstport "$port" -m comment --comment pantry-lan-guard -j DROP
      echo "lan-guard: IPv4 port $port allows private networks and drops gateway $gateway"
      return
    fi
  fi
  echo "lan-guard: IPv4 port $port allows private networks"
}

apply_v6() {
  if ! command -v ip6tables >/dev/null 2>&1; then
    return 0
  fi
  if ! ip6tables -L DOCKER-USER -n >/dev/null 2>&1; then
    return 0
  fi
  delete_guard_rules ip6tables
  insert_drop ip6tables
  local source
  for source in fc00::/7 fe80::/10 ::1/128; do
    insert_return ip6tables "$source"
  done
}

apply_v4
apply_v6 || echo "lan-guard: IPv6 filter was not changed" >&2
