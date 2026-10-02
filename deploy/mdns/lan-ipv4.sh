#!/usr/bin/env bash
# Shared LAN address lookup for setup and the mDNS publisher.
# Sourced, not executed. Callers own shell options.

# private_ipv4 returns 0 when $1 is a dotted IPv4 address in 10/8,
# 172.16/12, or 192.168/16. Leading zeros are rejected so a value cannot
# be read as octal and cannot be interpolated into a DNS config.
private_ipv4() {
  local ip="$1" oct o1 o2 o3 o4
  [[ "$ip" =~ ^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})$ ]] || return 1
  o1="${BASH_REMATCH[1]}"
  o2="${BASH_REMATCH[2]}"
  o3="${BASH_REMATCH[3]}"
  o4="${BASH_REMATCH[4]}"
  for oct in "$o1" "$o2" "$o3" "$o4"; do
    if [[ "$oct" =~ ^0[0-9]+$ ]]; then
      return 1
    fi
    if (( 10#$oct > 255 )); then
      return 1
    fi
  done
  if (( 10#$o1 == 10 )); then
    return 0
  fi
  if (( 10#$o1 == 192 && 10#$o2 == 168 )); then
    return 0
  fi
  if (( 10#$o1 == 172 && 10#$o2 >= 16 && 10#$o2 <= 31 )); then
    return 0
  fi
  return 1
}

# lan_ipv4 prints the private address home Wi-Fi clients should use.
# PANTRY_LAN_IPV4, then the same key in ${PANTRY_DIR}/.env, wins over
# detection so a Pi with a Docker bridge and a LAN NIC can be pinned.
# Detection prefers the source address of the default route, which is the
# NIC that reaches the rest of the house, and only then hostname -I.
lan_ipv4() {
  local configured="" line ip candidate
  if [[ -n "${PANTRY_LAN_IPV4:-}" ]]; then
    configured="${PANTRY_LAN_IPV4}"
  elif [[ -n "${PANTRY_DIR:-}" && -f "${PANTRY_DIR}/.env" ]]; then
    configured=$({ grep '^PANTRY_LAN_IPV4=' "${PANTRY_DIR}/.env" || true; } | head -1 | cut -d= -f2- | tr -d '[:space:]')
  fi
  if [[ -n "$configured" ]]; then
    if private_ipv4 "$configured"; then
      printf '%s\n' "$configured"
      return 0
    fi
    printf 'PANTRY_LAN_IPV4=%s is not a private IPv4 address\n' "$configured" >&2
    return 1
  fi
  if command -v ip >/dev/null 2>&1; then
    line=$(ip -4 route get 1.1.1.1 2>/dev/null || true)
    if [[ "$line" =~ src[[:space:]]([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+) ]]; then
      ip="${BASH_REMATCH[1]}"
      if private_ipv4 "$ip"; then
        printf '%s\n' "$ip"
        return 0
      fi
    fi
  fi
  if command -v hostname >/dev/null 2>&1; then
    while read -r candidate; do
      if private_ipv4 "$candidate"; then
        printf '%s\n' "$candidate"
        return 0
      fi
    done < <(hostname -I 2>/dev/null | tr ' ' '\n')
  fi
  return 1
}
