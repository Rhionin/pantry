#!/usr/bin/env bash
# Point the public Pantry hostname at this Pi for DNS clients on the LAN.
#
# Off unless PANTRY_SPLIT_DNS=on. Public DNS is not modified, and Caddy's
# certificate and shared password stay in front of anyone who still reaches
# the hostname through the router. This only helps devices that actually
# query the Pi. A Gryphon router often answers DNS itself, in which case
# home Wi-Fi should use the LAN URL instead of waiting on this file.

set -euo pipefail

PANTRY_DIR="${PANTRY_DIR:-/opt/pantry}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/../mdns/lan-ipv4.sh"

ENV_FILE="${PANTRY_DIR}/.env"
DROP_DIR="${PANTRY_DNSMASQ_DIR:-/etc/dnsmasq.d}"
DROP_FILE="${DROP_DIR}/pantry-split-horizon.conf"
GENERATED="${PANTRY_DIR}/dns/pantry-split-horizon.conf"

env_value() {
  { grep "^${1}=" "$ENV_FILE" || true; } | head -1 | cut -d= -f2- | tr -d '[:space:]'
}

remove_drop_in() {
  if [[ -f "$DROP_FILE" ]]; then
    rm -f "$DROP_FILE"
    echo "split-horizon: removed ${DROP_FILE}"
    restart_dnsmasq || true
  fi
  if [[ -f "$GENERATED" ]]; then
    rm -f "$GENERATED"
  fi
}

restart_dnsmasq() {
  if [[ "${PANTRY_DNSMASQ_RESTART:-}" != 1 && "$DROP_DIR" != /etc/dnsmasq.d ]]; then
    return 0
  fi
  if ! command -v systemctl >/dev/null 2>&1; then
    echo "split-horizon: systemctl is not available to restart dnsmasq" >&2
    return 1
  fi
  if ! systemctl cat dnsmasq.service >/dev/null 2>&1; then
    echo "split-horizon: dnsmasq.service is not installed" >&2
    return 1
  fi
  systemctl enable dnsmasq.service
  systemctl restart dnsmasq.service
}

flag=$(env_value PANTRY_SPLIT_DNS)
flag=$(printf '%s' "$flag" | tr '[:upper:]' '[:lower:]')
case "$flag" in
  on|true|yes|1) ;;
  *)
    remove_drop_in
    exit 0
    ;;
esac

host=$(env_value PUBLIC_HOST)
if [[ ! "$host" =~ ^[A-Za-z0-9.-]+$ ]] || [[ "$host" != *.* ]] || [[ "$host" == .* ]] || [[ "$host" == *. ]]; then
  echo "split-horizon: PUBLIC_HOST must be a hostname such as pantry.rhionin.com before DNS can answer it" >&2
  exit 1
fi

ip_addr=$(lan_ipv4) || {
  echo "split-horizon: no private LAN address. Set PANTRY_LAN_IPV4 in ${ENV_FILE}" >&2
  exit 1
}

mkdir -p "${PANTRY_DIR}/dns"
tmp=$(mktemp "${PANTRY_DIR}/dns/pantry-split-horizon.XXXXXX")
cat > "$tmp" << EOF
# Written by pantry setup. Home Wi-Fi clients that use this Pi for DNS
# receive the LAN address for the public name, so they reach Caddy here
# instead of hairpinning through the router. Other names are forwarded.
# Listens on the LAN address only. Do not forward port 53 from the router.
listen-address=${ip_addr}
bind-interfaces
except-interface=lo
no-dhcp-interface=${ip_addr}
address=/${host}/${ip_addr}
server=1.1.1.1
server=9.9.9.9
domain-needed
bogus-priv
EOF

if [[ -f "$GENERATED" ]] && cmp -s "$tmp" "$GENERATED"; then
  rm -f "$tmp"
else
  mv "$tmp" "$GENERATED"
fi
chmod 644 "$GENERATED"

echo "split-horizon: ${host} -> ${ip_addr}"

if [[ ! -d "$DROP_DIR" ]]; then
  echo "split-horizon: ${DROP_DIR} is missing. Install dnsmasq and re-run sudo ./setup.sh. The config is ${GENERATED}" >&2
  exit 0
fi

if [[ -f "$DROP_FILE" ]] && cmp -s "$GENERATED" "$DROP_FILE"; then
  echo "split-horizon: ${DROP_FILE} already matches"
else
  cp "$GENERATED" "$DROP_FILE"
  echo "split-horizon: installed ${DROP_FILE}"
fi

if ! command -v dnsmasq >/dev/null 2>&1; then
  echo "split-horizon: dnsmasq is not installed, so clients are not using this yet" >&2
  exit 0
fi

restart_dnsmasq
