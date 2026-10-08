#!/usr/bin/env bash
# Validate both publish modes: compose config, and caddy adapt.
# The certificate path is docker-compose.yml + Caddyfile.
# The tunnel path adds docker-compose.tunnel.yml + Caddyfile.tunnel.

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

have() {
  local haystack="$1" needle="$2" label="$3"
  if ! printf '%s\n' "$haystack" | grep -q -F -- "$needle"; then
    fail "$label (missing $needle)"
  fi
}

lack() {
  local haystack="$1" needle="$2" label="$3"
  if printf '%s\n' "$haystack" | grep -q -F -- "$needle"; then
    fail "$label (unexpected $needle)"
  fi
}

if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  compose() { docker compose "$@"; }
elif command -v docker-compose >/dev/null 2>&1; then
  compose() { docker-compose "$@"; }
else
  fail "docker compose is required to validate publish modes"
fi

if command -v caddy >/dev/null 2>&1; then
  caddy_adapt() {
    local cfg="$1" dir="$2"
    (
      cd "$dir"
      PUBLIC_HOST=pantry.rhionin.com ACME_EMAIL=cj@example.com \
        caddy adapt --config "$cfg" --pretty
    )
  }
elif command -v docker >/dev/null 2>&1; then
  caddy_adapt() {
    local cfg="$1" dir="$2"
    docker run --rm \
      -e PUBLIC_HOST=pantry.rhionin.com \
      -e ACME_EMAIL=cj@example.com \
      -v "$dir:/etc/caddy:ro" \
      caddy:2.11.4-alpine \
      caddy adapt --config "/etc/caddy/$(basename "$cfg")" --pretty
  }
else
  fail "caddy or docker is required to adapt the Caddyfiles"
fi

# Site blocks start at the same comment and must stay byte-for-byte identical
# so a public-path change cannot land in only one mode.
route_body() {
  local file="$1" text
  text=$(cat "$file")
  local marker=$'\n\t# handle is first-match.'
  if [[ "$text" != *"$marker"* ]]; then
    fail "$file is missing the shared route block"
  fi
  printf '%s\n' "${text#*"$marker"}"
}

acme_routes=$(route_body deploy/Caddyfile)
tunnel_routes=$(route_body deploy/Caddyfile.tunnel)
if [[ "$acme_routes" != "$tunnel_routes" ]]; then
  fail "Caddyfile and Caddyfile.tunnel route blocks differ"
fi

empty_env=$(mktemp)
public_cfg=$(compose --env-file "$empty_env" -f deploy/docker-compose.yml --profile public config)
tunnel_cfg=$(compose --env-file "$empty_env" -f deploy/docker-compose.yml -f deploy/docker-compose.tunnel.yml --profile tunnel config)
rm -f "$empty_env"

have "$public_cfg" 'published: "80"' "certificate mode publishes port 80"
have "$public_cfg" 'published: "443"' "certificate mode publishes port 443"
have "$public_cfg" 'caddy:2.11.4-alpine' "certificate mode pins Caddy"
lack "$public_cfg" 'cloudflare/cloudflared' "certificate mode must not start cloudflared"
lack "$public_cfg" '10.77.77.2' "certificate mode must not add the tunnel network"
lack "$public_cfg" 'Caddyfile.tunnel' "certificate mode must not mount the tunnel Caddyfile"

lack "$tunnel_cfg" 'published: "80"' "tunnel mode must not publish port 80"
lack "$tunnel_cfg" 'published: "443"' "tunnel mode must not publish port 443"
have "$tunnel_cfg" 'cloudflare/cloudflared:2026.10.0' "tunnel mode pins cloudflared"
have "$tunnel_cfg" 'caddy:2.11.4-alpine' "tunnel mode pins Caddy"
have "$tunnel_cfg" 'ipv4_address: 10.77.77.2' "cloudflared has the trusted address"
have "$tunnel_cfg" 'Caddyfile.tunnel' "tunnel mode mounts Caddyfile.tunnel"
have "$tunnel_cfg" 'target: /etc/caddy/Caddyfile' "tunnel Caddyfile is what Caddy loads"
have "$tunnel_cfg" 'container_name: pantry-cloudflared' "cloudflared container name"
lack "$tunnel_cfg" 'CLOUDFLARE_TUNNEL_TOKEN: null' "token env must be the empty string when unset, not null"
# Command is tunnel run. The image entrypoint supplies cloudflared --no-autoupdate.
have "$tunnel_cfg" $'- tunnel\n      - run' "cloudflared runs tunnel run"

adapt_dir=$(mktemp -d)
cp deploy/Caddyfile deploy/Caddyfile.tunnel "$adapt_dir/"
# shellcheck disable=SC2016 # bcrypt hashes start with $2
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry $2a$14$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz012' '}' > "$adapt_dir/auth.caddy"

acme_json=$(caddy_adapt "$adapt_dir/Caddyfile" "$adapt_dir")
tunnel_json=$(caddy_adapt "$adapt_dir/Caddyfile.tunnel" "$adapt_dir")
rm -rf "$adapt_dir"

have "$acme_json" 'pantry.rhionin.com' "certificate adapt expands PUBLIC_HOST"
have "$acme_json" '/api/telemetry' "certificate adapt keeps telemetry public"
have "$acme_json" '/brand/logo.png' "certificate adapt keeps the brand mark public"
have "$acme_json" 'cj@example.com' "certificate adapt keeps the ACME email"
lack "$acme_json" '10.77.77.2' "certificate adapt must not trust the tunnel address"

have "$tunnel_json" 'pantry.rhionin.com' "tunnel adapt expands PUBLIC_HOST"
have "$tunnel_json" '/api/telemetry' "tunnel adapt keeps telemetry public"
have "$tunnel_json" '/api/telemetry/client' "tunnel adapt keeps the client report public"
have "$tunnel_json" '/brand/logo.png' "tunnel adapt keeps the brand mark public"
have "$tunnel_json" '/terms' "tunnel adapt keeps terms public"
have "$tunnel_json" '/privacy' "tunnel adapt keeps privacy public"
have "$tunnel_json" '/api/events' "tunnel adapt keeps the event stream unbuffered"
have "$tunnel_json" '10.77.77.2/32' "tunnel adapt trusts only cloudflared"
have "$tunnel_json" 'CF-Connecting-IP' "tunnel adapt reads the visitor address"
lack "$tunnel_json" 'cj@example.com' "tunnel adapt must not configure an ACME account"
lack "$tunnel_json" '"module": "acme"' "tunnel adapt must not request a certificate"

echo "check-publish-modes ok"
