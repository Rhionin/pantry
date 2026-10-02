#!/usr/bin/env bash
# Publish pantry.local as this machine's LAN address.
#
# Phones on home Wi-Fi look up a public hostname in ordinary DNS and then
# try the router's WAN address. When the router does not NAT-hairpin, that
# connection hangs and never reaches Caddy. mDNS stays on the LAN, so
# http://pantry.local:<port> reaches the passwordless Pantry port directly.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lan-ipv4.sh"

NAME="${PANTRY_MDNS_NAME:-pantry.local}"

if [[ "$NAME" != "pantry.local" ]]; then
  echo "pantry.local is the only mDNS name this publisher registers" >&2
  exit 1
fi

ip_addr=$(lan_ipv4) || {
  echo "No private LAN address to publish as pantry.local. Set PANTRY_LAN_IPV4 in /opt/pantry/.env and re-run sudo ./setup.sh" >&2
  exit 1
}

publish=avahi-publish-address
if [[ -n "${PANTRY_AVAHI_PUBLISH:-}" && "${PANTRY_AVAHI_PUBLISH:-}" != "missing" ]]; then
  publish="${PANTRY_AVAHI_PUBLISH}"
fi
if [[ "${PANTRY_AVAHI_PUBLISH:-}" == "missing" ]] || ! command -v "$publish" >/dev/null 2>&1; then
  echo "avahi-publish-address is not installed, so pantry.local was not published" >&2
  exit 1
fi

# -R skips the reverse record so this alias does not replace the Pi's own
# hostname.local entry. avahi-publish-address runs until it is stopped.
exec "$publish" -R "$NAME" "$ip_addr"
