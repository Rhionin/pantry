#!/usr/bin/env bash
# Behavioral checks for the single setup command. Docker, systemd, and
# iptables are stubs so this can run without root or a Pi.

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SETUP="$ROOT/deploy/setup.sh"

bash -n "$SETUP"
bash -n "$ROOT/deploy/firewall/pantry-lan-only.sh"
bash -n "$ROOT/deploy/systemd/pantry-update.sh"
bash -n "$ROOT/deploy/mdns/pantry-mdns.sh"
bash -n "$ROOT/deploy/mdns/lan-ipv4.sh"
bash -n "$ROOT/deploy/dns/pantry-split-dns.sh"
bash -n "$ROOT/deploy/publish-mode.sh"
bash -n "$ROOT/deploy/check-publish-modes.sh"
bash -n "$ROOT/deploy/check-deploy-hook.sh"
bash -n "$ROOT/deploy/auth-migrate.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

LAST_RC=0
LAST_OUT=""
LAST_DOCKER=""
LAST_IPTABLES=""
LAST_SYSTEMCTL=""
LAST_AVAHI_SERVICE=""
LAST_DNSMASQ=""
LAST_MDNS_UNIT=""
LAST_CHOWN=""
LAST_CURL=""

make_bin() {
  local bin="$1"
  mkdir -p "$bin"

  cat > "$bin/docker" << 'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${DOCKER_LOG:?}"
if [[ "$1" == "inspect" ]]; then
  printf '%s\n' true
  exit 0
fi
if [[ "$*" == *hash-password* ]]; then
  printf '%s\n' '$2a$14$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz012'
  exit 0
fi
exit 0
EOF

  # enable --now runs the unit's ExecStart. The stub does that itself so the
  # firewall script is what the test observes.
  cat > "$bin/systemctl" << 'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${SYSTEMCTL_LOG:?}"
if [[ "$*" == *enable* && "$*" == *pantry-lan-only.service* && -x "${PANTRY_DIR}/firewall/pantry-lan-only.sh" ]]; then
  "${PANTRY_DIR}/firewall/pantry-lan-only.sh"
fi
exit 0
EOF

  cat > "$bin/iptables" << 'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${IPTABLES_LOG:?}"
exit 0
EOF

  # Records the public-entry session probe. -o writes the body the way
  # curl does, and -w '%{http_code}' is printed on stdout.
  cat > "$bin/curl" << 'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${CURL_LOG:?}"
out=""
prev=""
write_code=false
for arg in "$@"; do
  if [[ "$prev" == "-o" ]]; then
    out=$arg
  fi
  if [[ "$arg" == *'%{http_code}'* ]]; then
    write_code=true
  fi
  prev=$arg
done
if [[ "$*" == *"/api/session"* ]]; then
  if [[ -n "$out" ]]; then
    printf '%s' "${CURL_SESSION_BODY:-}" > "$out"
  fi
  if [[ "$write_code" == true ]]; then
    printf '%s' "${CURL_SESSION_CODE:-200}"
  fi
fi
exit 0
EOF

  cat > "$bin/chown" << 'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${CHOWN_LOG:?}"
exit 0
EOF

  cat > "$bin/usermod" << 'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat > "$bin/udevadm" << 'EOF'
#!/usr/bin/env bash
exit 0
EOF

  # Present so a machine that also has Avahi still resolves this stub first.
  # publish_lan_name only runs when a test sets PANTRY_SKIP_MDNS=0.
  cat > "$bin/avahi-publish-address" << 'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${AVAHI_LOG:?}"
exit 0
EOF

  chmod +x "$bin/docker" "$bin/systemctl" "$bin/iptables" "$bin/curl" "$bin/chown" "$bin/usermod" "$bin/udevadm" "$bin/avahi-publish-address"
}

run_setup() {
  local pantry="$1"
  shift
  local work bin out err
  work=$(mktemp -d)
  bin="$work/bin"
  out="$work/out"
  err="$work/err"
  make_bin "$bin"
  : > "$work/docker.log"
  : > "$work/systemctl.log"
  : > "$work/iptables.log"
  : > "$work/avahi.log"
  : > "$work/chown.log"
  : > "$work/curl.log"
  mkdir -p "$work/systemd" "$work/avahi" "$work/dnsmasq"

  set +e
  DOCKER_LOG="$work/docker.log" \
    SYSTEMCTL_LOG="$work/systemctl.log" \
    IPTABLES_LOG="$work/iptables.log" \
    AVAHI_LOG="$work/avahi.log" \
    CHOWN_LOG="$work/chown.log" \
    CURL_LOG="$work/curl.log" \
    CURL_SESSION_CODE="${CURL_SESSION_CODE:-200}" \
    CURL_SESSION_BODY="${CURL_SESSION_BODY:-}" \
    PANTRY_DIR="$pantry" \
    PANTRY_SYSTEMD_UNIT_DIR="$work/systemd" \
    PANTRY_SETUP_SKIP_ROOT=1 \
    PANTRY_SKIP_MDNS="${PANTRY_SKIP_MDNS:-1}" \
    PANTRY_AVAHI_SERVICES_DIR="${PANTRY_AVAHI_SERVICES_DIR:-$work/avahi}" \
    PANTRY_DNSMASQ_DIR="${PANTRY_DNSMASQ_DIR:-$work/dnsmasq}" \
    PATH="$bin:$PATH" \
    bash "$SETUP" "$@" >"$out" 2>"$err"
  LAST_RC=$?
  set -e

  LAST_OUT=$(cat "$out" "$err")
  LAST_DOCKER=$(cat "$work/docker.log")
  LAST_IPTABLES=$(cat "$work/iptables.log")
  LAST_SYSTEMCTL=$(cat "$work/systemctl.log")
  LAST_AVAHI_SERVICE=""
  if [[ -f "$work/avahi/pantry-http.service" ]]; then
    LAST_AVAHI_SERVICE=$(cat "$work/avahi/pantry-http.service")
  fi
  LAST_DNSMASQ=""
  if [[ -f "$work/dnsmasq/pantry-split-horizon.conf" ]]; then
    LAST_DNSMASQ=$(cat "$work/dnsmasq/pantry-split-horizon.conf")
  fi
  LAST_MDNS_UNIT=""
  if [[ -f "$work/systemd/pantry-mdns.service" ]]; then
    LAST_MDNS_UNIT=$(cat "$work/systemd/pantry-mdns.service")
  fi
  LAST_CHOWN=$(cat "$work/chown.log")
  LAST_CURL=$(cat "$work/curl.log")
  rm -rf "$work"
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

write_env() {
  local pantry="$1"
  shift
  mkdir -p "$pantry"
  printf '%s\n' "$@" > "$pantry/.env"
}

# LAN-only: containers come up without the public profile, and the published
# port is firewalled because compose publishes it.
lan=$(mktemp -d)
write_env "$lan" \
  "PANTRY_IMAGE_TAG=latest" \
  "HOST_PORT=9090" \
  "PUBLIC_HOST=" \
  "ACME_EMAIL=" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "SCANNER_DEVICE=/dev/input/pantry-scanner"
run_setup "$lan"
[[ "$LAST_RC" -eq 0 ]] || fail "LAN setup exited $LAST_RC: $LAST_OUT"
have "$LAST_DOCKER" "up -d" "LAN compose up"
lack "$LAST_DOCKER" "--profile public" "LAN must not start the public proxy"
secret1=$(grep '^DEPLOY_HOOK_SECRET=' "$lan/.env" | cut -d= -f2-)
[[ "$secret1" =~ ^[0-9a-f]{64}$ ]] || fail "setup did not generate a 64-hex deploy hook secret (got $secret1)"
[[ -d "$lan/deploy-trigger" ]] || fail "setup did not create the deploy trigger directory"
run_setup "$lan"
[[ "$LAST_RC" -eq 0 ]] || fail "second LAN setup exited $LAST_RC: $LAST_OUT"
secret2=$(grep '^DEPLOY_HOOK_SECRET=' "$lan/.env" | cut -d= -f2-)
[[ "$secret1" == "$secret2" ]] || fail "setup rotated DEPLOY_HOOK_SECRET"
printed=$(PANTRY_DIR="$lan" PANTRY_SETUP_SKIP_ROOT=1 bash "$SETUP" deploy-secret)
[[ "$printed" == "$secret1" ]] || fail "deploy-secret printed [$printed], want [$secret1]"
lack "$LAST_DOCKER" "docker-compose.tunnel.yml" "LAN must not select the tunnel file"
lack "$LAST_DOCKER" "hash-password" "LAN must not hash a password"
lack "$LAST_DOCKER" "pull pantry" "LAN must not pull before the public site exists"
[[ -d "$lan/auth" ]] || fail "LAN setup did not create the auth directory"
[[ ! -e "$lan/auth/household" ]] || fail "LAN setup invented a password hash"
lack "$LAST_CURL" "/api/session" "LAN setup must not probe public sign-in"
have "$LAST_CHOWN" "65532:65532 $lan/auth" "LAN setup still gives the auth directory to uid 65532"
grep -q '^PANTRY_SESSION_SECRET=$' "$lan/.env" || fail "LAN setup filled a session secret"
lack "$LAST_DOCKER" "restart pantry-caddy" "LAN must not restart Caddy"
have "$LAST_IPTABLES" "--dport 9090" "firewall uses HOST_PORT"
have "$LAST_OUT" "Pantry setup complete" "LAN success"
have "$LAST_OUT" "Non-LAN clients are blocked on port 9090" "LAN firewall summary"
have "$LAST_OUT" "LAN: http://" "LAN URL is printed"
lack "$LAST_OUT" "Home Wi-Fi cannot open" "LAN-only must not warn about hairpin"
lack "$LAST_SYSTEMCTL" "pantry-mdns.service" "default test run does not install mDNS"
grep -q '0\.0\.0\.0:.*:8080' "$lan/docker-compose.yml" || fail "compose must still publish the LAN port"

# Existing auth.caddy is kept even when BASIC_AUTH_PASSWORD is empty or different.
# The public profile stays on. The password line in .env is not rewritten.
pub=$(mktemp -d)
write_env "$pub" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080"
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$pub/auth.caddy"
cp "$pub/auth.caddy" "$pub/auth.caddy.before"
cp "$pub/.env" "$pub/.env.before"
run_setup "$pub"
[[ "$LAST_RC" -eq 0 ]] || fail "existing auth setup exited $LAST_RC: $LAST_OUT"
cmp "$pub/auth.caddy" "$pub/auth.caddy.before" || fail "auth.caddy was regenerated"
grep -q '^BASIC_AUTH_PASSWORD=$' "$pub/.env" || fail "empty BASIC_AUTH_PASSWORD was rewritten"
have "$LAST_DOCKER" "--profile public" "public profile"
lack "$LAST_DOCKER" "docker-compose.tunnel.yml" "certificate mode must not select the tunnel file"
have "$LAST_DOCKER" "restart pantry-caddy" "Caddyfile restart"
lack "$LAST_DOCKER" "hash-password" "must not hash when auth.caddy exists"
lack "$LAST_DOCKER" "stop caddy" "must not disable public HTTPS"
have "$LAST_OUT" "Keeping existing" "kept auth.caddy"
cmp "$pub/auth.caddy" "$pub/auth/household" || fail "household hash was not copied from auth.caddy"
pub_mode=$(stat -c '%a' "$pub/auth/household")
[[ "$pub_mode" == "600" || "$pub_mode" == "640" ]] || fail "household file mode is $pub_mode, want 600 or 640"
have "$LAST_CHOWN" "65532:65532 $pub/auth" "setup chowns the auth directory to uid 65532"
have "$LAST_CHOWN" "65532:65532 $pub/auth/household" "setup chowns the household file to uid 65532"
have "$LAST_CURL" "X-Pantry-Entry: public" "public setup probes sign-in with the public entry header"
have "$LAST_CURL" "/api/session" "public setup probes /api/session"
pub_secret=$(grep '^PANTRY_SESSION_SECRET=' "$pub/.env" | cut -d= -f2-)
[[ ${#pub_secret} -eq 64 ]] || fail "session secret was not saved (len ${#pub_secret})"
have "$LAST_DOCKER" "pull pantry" "public setup pulls the image before dropping basic auth"
have "$LAST_IPTABLES" "--dport 8080" "public setup still firewalls the LAN port"
have "$LAST_OUT" "Home Wi-Fi cannot open https://pantry.example.com" "hairpin warning names the public host"
have "$LAST_OUT" "shared password" "public summary still mentions the password"
lack "$LAST_DOCKER" "stop caddy" "hairpin warning must not stop the public proxy"

# A second run stays idempotent.
run_setup "$pub"
[[ "$LAST_RC" -eq 0 ]] || fail "second setup exited $LAST_RC: $LAST_OUT"
cmp "$pub/auth.caddy" "$pub/auth.caddy.before" || fail "second run regenerated auth.caddy"
cmp "$pub/auth.caddy" "$pub/auth/household" || fail "second run changed the household hash"
have "$LAST_CHOWN" "65532:65532 $pub/auth/household" "second run chowns an existing household file"
pub_secret_again=$(grep '^PANTRY_SESSION_SECRET=' "$pub/.env" | cut -d= -f2-)
[[ "$pub_secret_again" == "$pub_secret" ]] || fail "second run rotated the session secret"
lack "$LAST_DOCKER" "hash-password" "second run hashed a password"

# A different password in .env does not replace an existing hash.
write_env "$pub" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=correct horse battery staple" \
  "HOST_PORT=8080"
run_setup "$pub"
[[ "$LAST_RC" -eq 0 ]] || fail "password-present re-run exited $LAST_RC: $LAST_OUT"
cmp "$pub/auth.caddy" "$pub/auth.caddy.before" || fail "existing auth.caddy changed when BASIC_AUTH_PASSWORD was set"
grep -q '^BASIC_AUTH_PASSWORD=correct horse battery staple$' "$pub/.env" || fail "BASIC_AUTH_PASSWORD was wiped"
lack "$LAST_DOCKER" "hash-password" "hashed despite existing auth.caddy"

# First public setup writes auth.caddy from the password already in .env.
first=$(mktemp -d)
write_env "$first" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=correct-horse-battery" \
  "HOST_PORT=8080"
run_setup "$first"
[[ "$LAST_RC" -eq 0 ]] || fail "first public setup exited $LAST_RC: $LAST_OUT"
[[ -f "$first/auth.caddy" ]] || fail "auth.caddy was not created"
grep -q 'basic_auth bcrypt Pantry' "$first/auth.caddy" || fail "auth.caddy missing basic_auth block"
grep -q 'abcdefghijklmnopqrstuu' "$first/auth.caddy" || fail "auth.caddy missing the hashed password"
grep -q '^BASIC_AUTH_PASSWORD=correct-horse-battery$' "$first/.env" || fail "password was not kept in .env"
cmp "$first/auth.caddy" "$first/auth/household" || fail "first public setup did not copy the new hash"
first_mode=$(stat -c '%a' "$first/auth/household")
[[ "$first_mode" == "600" || "$first_mode" == "640" ]] || fail "new household file mode is $first_mode"
[[ $(grep '^PANTRY_SESSION_SECRET=' "$first/.env" | cut -d= -f2- | wc -c) -eq 65 ]] || fail "first public setup did not save a session secret"
have "$LAST_DOCKER" "hash-password" "first public setup should hash"
have "$LAST_DOCKER" "--profile public" "first public profile"

# A too-short password fails without inventing auth.caddy or clearing .env.
bad=$(mktemp -d)
write_env "$bad" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=short" \
  "HOST_PORT=8080"
run_setup "$bad"
[[ "$LAST_RC" -ne 0 ]] || fail "short password should fail"
[[ ! -e "$bad/auth.caddy" ]] || fail "short password created auth.caddy"
grep -q '^BASIC_AUTH_PASSWORD=short$' "$bad/.env" || fail "short password was wiped"

# No password and no auth.caddy: LAN still converges, public proxy stays off.
incomplete=$(mktemp -d)
write_env "$incomplete" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080"
run_setup "$incomplete"
[[ "$LAST_RC" -eq 0 ]] || fail "incomplete public setup exited $LAST_RC: $LAST_OUT"
lack "$LAST_DOCKER" "--profile public" "incomplete public config must not start the proxy"
[[ ! -e "$incomplete/auth.caddy" ]] || fail "empty password created auth.caddy"
have "$LAST_OUT" "BASIC_AUTH_PASSWORD is empty" "incomplete public warning"

# Opt-out removes the rule instead of installing it.
opt=$(mktemp -d)
write_env "$opt" \
  "PUBLIC_HOST=" \
  "HOST_PORT=8080" \
  "PANTRY_LAN_FIREWALL=off" \
  "BASIC_AUTH_PASSWORD="
run_setup "$opt"
[[ "$LAST_RC" -eq 0 ]] || fail "opt-out setup exited $LAST_RC: $LAST_OUT"
have "$LAST_OUT" "PANTRY_LAN_FIREWALL=off" "opt-out notice"
lack "$LAST_OUT" "Non-LAN clients are blocked" "opt-out must not install the firewall"
lack "$LAST_SYSTEMCTL" "enable --now pantry-lan-only.service" "opt-out must not enable the firewall unit"
have "$LAST_IPTABLES" "--dport 8080" "opt-out removes rules for HOST_PORT"

# No command, and the old subcommands, all run the same setup.
for args in "" "apply" "install" "publish" "firewall"; do
  # shellcheck disable=SC2086
  run_setup "$lan" $args
  [[ "$LAST_RC" -eq 0 ]] || fail "command [$args] exited $LAST_RC: $LAST_OUT"
  have "$LAST_OUT" "Pantry setup complete" "command [$args]"
done

if [[ -e /opt/pantry ]]; then
  fail "setup wrote /opt/pantry; tests must stay on PANTRY_DIR"
fi

# Home Wi-Fi name: publish pantry.local and keep the public proxy on.
mdns=$(mktemp -d)
write_env "$mdns" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080" \
  "PANTRY_LAN_IPV4=192.168.1.203"
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$mdns/auth.caddy"
PANTRY_SKIP_MDNS=0 run_setup "$mdns"
[[ "$LAST_RC" -eq 0 ]] || fail "mdns setup exited $LAST_RC: $LAST_OUT"
have "$LAST_OUT" "LAN: http://192.168.1.203:8080" "pinned LAN URL"
have "$LAST_OUT" "LAN name: http://pantry.local:8080" "mDNS name"
have "$LAST_OUT" "Published http://pantry.local:8080" "mDNS publish success"
have "$LAST_SYSTEMCTL" "enable pantry-mdns.service" "mDNS unit enabled"
have "$LAST_SYSTEMCTL" "restart pantry-mdns.service" "mDNS unit restarted"
have "$LAST_MDNS_UNIT" "/opt/pantry/mdns/pantry-mdns.sh" "mDNS unit runs the publisher"
have "$LAST_AVAHI_SERVICE" "<port>8080</port>" "Avahi advertisement uses HOST_PORT"
have "$LAST_DOCKER" "--profile public" "mDNS setup keeps the public proxy"
lack "$LAST_DOCKER" "stop caddy" "mDNS setup must not stop Caddy"
grep -q 'basic_auth bcrypt Pantry' "$mdns/auth.caddy" || fail "mDNS setup rewrote auth.caddy"

# Split horizon maps only the public name at the LAN address.
split=$(mktemp -d)
write_env "$split" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080" \
  "PANTRY_LAN_IPV4=192.168.1.203" \
  "PANTRY_SPLIT_DNS=on"
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$split/auth.caddy"
run_setup "$split"
[[ "$LAST_RC" -eq 0 ]] || fail "split-dns setup exited $LAST_RC: $LAST_OUT"
have "$LAST_OUT" "split-horizon: pantry.example.com -> 192.168.1.203" "split-horizon mapping"
have "$LAST_DNSMASQ" "address=/pantry.example.com/192.168.1.203" "dnsmasq answer"
have "$LAST_DNSMASQ" "listen-address=192.168.1.203" "dnsmasq binds the LAN address"
have "$LAST_DNSMASQ" "no-dhcp-interface=192.168.1.203" "dnsmasq does not serve DHCP"
lack "$LAST_DNSMASQ" "0.0.0.0" "dnsmasq must not listen on every interface"
have "$LAST_DOCKER" "--profile public" "split-horizon keeps the public proxy"
grep -q 'ORIGINAL-HASH' "$split/auth.caddy" || fail "split-horizon rewrote auth.caddy"
# Turning it off removes the answer so the Pi stops overriding the name.
write_env "$split" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080" \
  "PANTRY_LAN_IPV4=192.168.1.203" \
  "PANTRY_SPLIT_DNS=off"
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$split/auth.caddy"
run_setup "$split"
[[ "$LAST_RC" -eq 0 ]] || fail "split-dns off exited $LAST_RC: $LAST_OUT"
[[ ! -e "$split/dns/pantry-split-horizon.conf" ]] || fail "split-horizon config remained after off"
lack "$LAST_DNSMASQ" "address=/pantry.example.com/192.168.1.203" "off run must not reinstall dnsmasq"
have "$LAST_DOCKER" "--profile public" "turning split-horizon off keeps the public proxy"

# A public address must not be published as the LAN workaround.
badip=$(mktemp -d)
write_env "$badip" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080" \
  "PANTRY_LAN_IPV4=8.8.8.8" \
  "PANTRY_SPLIT_DNS=on"
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$badip/auth.caddy"
run_setup "$badip"
[[ "$LAST_RC" -eq 0 ]] || fail "bad LAN IP must not fail the rest of setup, exited $LAST_RC: $LAST_OUT"
have "$LAST_OUT" "Split-horizon DNS was not applied" "bad LAN IP skips split-horizon"
lack "$LAST_DNSMASQ" "8.8.8.8" "public address was written into dnsmasq"
have "$LAST_DOCKER" "--profile public" "bad LAN IP still starts the public proxy"
have "$LAST_OUT" "Home Wi-Fi cannot open https://pantry.example.com" "hairpin warning remains without a detected LAN IP"

# shellcheck disable=SC1091
source "$ROOT/deploy/mdns/lan-ipv4.sh"
unset PANTRY_DIR
got=$(PANTRY_LAN_IPV4=10.1.2.3 lan_ipv4)
[[ "$got" == "10.1.2.3" ]] || fail "pinned 10/8 address, got $got"
got=$(PANTRY_LAN_IPV4=172.16.5.5 lan_ipv4)
[[ "$got" == "172.16.5.5" ]] || fail "pinned 172.16/12 address, got $got"
if PANTRY_LAN_IPV4=8.8.8.8 lan_ipv4 >/dev/null 2>&1; then
  fail "public address accepted as a LAN address"
fi
if PANTRY_LAN_IPV4=192.168.001.203 lan_ipv4 >/dev/null 2>&1; then
  fail "leading-zero address accepted"
fi
if PANTRY_LAN_IPV4=192.168.1.256 lan_ipv4 >/dev/null 2>&1; then
  fail "octet 256 accepted"
fi

mdns_bin=$(mktemp -d)
cat > "$mdns_bin/avahi-publish-address" << 'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" > "${AVAHI_LOG:?}"
exit 0
EOF
chmod +x "$mdns_bin/avahi-publish-address"
mdns_log=$(mktemp)
AVAHI_LOG="$mdns_log" PANTRY_LAN_IPV4=192.168.1.203 PATH="$mdns_bin:$PATH" \
  bash "$ROOT/deploy/mdns/pantry-mdns.sh"
have "$(cat "$mdns_log")" "-R pantry.local 192.168.1.203" "publisher arguments"
set +e
AVAHI_LOG="$mdns_log" PANTRY_LAN_IPV4=1.2.3.4 PATH="$mdns_bin:$PATH" \
  bash "$ROOT/deploy/mdns/pantry-mdns.sh" >/dev/null 2>&1
mdns_rc=$?
set -e
[[ "$mdns_rc" -ne 0 ]] || fail "publisher accepted a public address"

# A token selects the tunnel even when ACME_EMAIL is empty and --tunnel is omitted.
# The certificate profile stays off, and the token is not printed.
tun=$(mktemp -d)
write_env "$tun" \
  "PUBLIC_HOST=pantry.rhionin.com" \
  "ACME_EMAIL=" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080" \
  "PUBLISH_MODE=" \
  "CLOUDFLARE_TUNNEL_TOKEN=test-tunnel-token"
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$tun/auth.caddy"
cp "$tun/auth.caddy" "$tun/auth.caddy.before"
run_setup "$tun"
[[ "$LAST_RC" -eq 0 ]] || fail "tunnel setup exited $LAST_RC: $LAST_OUT"
have "$LAST_DOCKER" "docker-compose.tunnel.yml" "tunnel compose file"
have "$LAST_DOCKER" "--profile tunnel" "tunnel profile"
lack "$LAST_DOCKER" "--profile public" "tunnel must not publish the certificate profile"
have "$LAST_DOCKER" "restart pantry-caddy" "tunnel restarts Caddy"
have "$LAST_OUT" "http://caddy:80" "service URL is printed"
have "$LAST_OUT" "Nothing on this Pi is listening on ports 80 or 443" "tunnel closes host 80/443"
lack "$LAST_OUT" "Home Wi-Fi cannot open" "tunnel must not warn about hairpin"
lack "$LAST_OUT" "test-tunnel-token" "token must not be logged"
lack "$LAST_DOCKER" "test-tunnel-token" "token must not be a docker argument"
cmp "$tun/auth.caddy" "$tun/auth.caddy.before" || fail "tunnel setup regenerated auth.caddy"
lack "$LAST_DOCKER" "hash-password" "tunnel must not hash when auth.caddy exists"
# publish --tunnel is the same switch once the token is already in .env.
run_setup "$tun" publish --tunnel
[[ "$LAST_RC" -eq 0 ]] || fail "publish --tunnel exited $LAST_RC: $LAST_OUT"
have "$LAST_DOCKER" "--profile tunnel" "publish --tunnel"
cmp "$tun/auth.caddy" "$tun/auth.caddy.before" || fail "publish --tunnel regenerated auth.caddy"

# PUBLISH_MODE=acme does not override a token that is still set.
write_env "$tun" \
  "PUBLIC_HOST=pantry.rhionin.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080" \
  "PUBLISH_MODE=acme" \
  "CLOUDFLARE_TUNNEL_TOKEN=test-tunnel-token"
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$tun/auth.caddy"
run_setup "$tun"
[[ "$LAST_RC" -eq 0 ]] || fail "token overrides acme mode, exited $LAST_RC: $LAST_OUT"
have "$LAST_DOCKER" "--profile tunnel" "token wins over PUBLISH_MODE=acme"
lack "$LAST_DOCKER" "--profile public" "token must not open the certificate profile"

# Tunnel without a token fails before containers start, and does not fall through to ACME.
notoken=$(mktemp -d)
write_env "$notoken" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080" \
  "PUBLISH_MODE=tunnel" \
  "CLOUDFLARE_TUNNEL_TOKEN="
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$notoken/auth.caddy"
run_setup "$notoken" publish --tunnel
[[ "$LAST_RC" -ne 0 ]] || fail "tunnel without a token should fail"
lack "$LAST_DOCKER" "up -d" "missing token must not start containers"
have "$LAST_OUT" "CLOUDFLARE_TUNNEL_TOKEN" "missing token names the variable"
lack "$LAST_DOCKER" "--profile public" "missing token must not start the certificate proxy"

# Token set, password not ready: LAN still comes up, tunnel stays off.
unready=$(mktemp -d)
write_env "$unready" \
  "PUBLIC_HOST=pantry.rhionin.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080" \
  "CLOUDFLARE_TUNNEL_TOKEN=test-tunnel-token"
run_setup "$unready"
[[ "$LAST_RC" -eq 0 ]] || fail "unready tunnel exited $LAST_RC: $LAST_OUT"
lack "$LAST_DOCKER" "--profile tunnel" "unready tunnel must not start cloudflared"
lack "$LAST_DOCKER" "--profile public" "unready tunnel must not start the certificate proxy"
have "$LAST_OUT" "The tunnel was not started" "unready tunnel explains why"
[[ ! -e "$unready/auth.caddy" ]] || fail "empty password created auth.caddy"

# Split-horizon DNS would point the public name at the Pi. Tunnel mode removes it.
tunsplit=$(mktemp -d)
write_env "$tunsplit" \
  "PUBLIC_HOST=pantry.rhionin.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080" \
  "PANTRY_LAN_IPV4=192.168.1.203" \
  "PANTRY_SPLIT_DNS=on" \
  "CLOUDFLARE_TUNNEL_TOKEN=test-tunnel-token"
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$tunsplit/auth.caddy"
run_setup "$tunsplit"
[[ "$LAST_RC" -eq 0 ]] || fail "tunnel split-dns exited $LAST_RC: $LAST_OUT"
have "$LAST_OUT" "PANTRY_SPLIT_DNS is on" "tunnel warns about split-horizon"
have "$LAST_OUT" "Split-horizon DNS was removed" "tunnel removes split-horizon"
lack "$LAST_DNSMASQ" "address=/pantry.rhionin.com/192.168.1.203" "tunnel must not install dnsmasq"
have "$LAST_DOCKER" "--profile tunnel" "split-horizon removal keeps the tunnel"

# publish_mode_from_env
# shellcheck disable=SC1091
source "$ROOT/deploy/publish-mode.sh"
mode_dir=$(mktemp -d)
write_env "$mode_dir" "PUBLIC_HOST=" "CLOUDFLARE_TUNNEL_TOKEN=" "PUBLISH_MODE="
PANTRY_DIR="$mode_dir"
got=$(publish_mode_from_env)
[[ "$got" == none ]] || fail "empty env mode, got $got"
write_env "$mode_dir" "PUBLIC_HOST=pantry.example.com" "CLOUDFLARE_TUNNEL_TOKEN=" "PUBLISH_MODE="
got=$(publish_mode_from_env)
[[ "$got" == acme ]] || fail "host without token should be acme, got $got"
write_env "$mode_dir" "PUBLIC_HOST=pantry.example.com" "CLOUDFLARE_TUNNEL_TOKEN=tok" "PUBLISH_MODE=acme"
got=$(publish_mode_from_env)
[[ "$got" == tunnel ]] || fail "token should win, got $got"
write_env "$mode_dir" "PUBLIC_HOST=pantry.example.com" "CLOUDFLARE_TUNNEL_TOKEN=" "PUBLISH_MODE=tunnel"
set +e
got=$(publish_mode_from_env)
mode_rc=$?
set -e
[[ "$mode_rc" -eq 2 ]] || fail "tunnel mode without token rc=$mode_rc"
write_env "$mode_dir" "PUBLIC_HOST=" "CLOUDFLARE_TUNNEL_TOKEN=" "PUBLISH_MODE=sideways"
set +e
got=$(publish_mode_from_env)
mode_rc=$?
set -e
[[ "$mode_rc" -eq 1 ]] || fail "bad PUBLISH_MODE rc=$mode_rc"
write_env "$mode_dir" "PUBLIC_HOST=pantry.example.com" "CLOUDFLARE_TUNNEL_TOKEN=tok" "PUBLISH_MODE="
got=$(publish_mode_from_env tunnel)
[[ "$got" == tunnel ]] || fail "--tunnel request, got $got"

# A secret the operator set is left alone.
custom=$(mktemp -d)
write_env "$custom" \
  "DEPLOY_HOOK_SECRET=not-generated" \
  "HOST_PORT=8080" \
  "PUBLIC_HOST=" \
  "BASIC_AUTH_PASSWORD="
run_setup "$custom"
[[ "$LAST_RC" -eq 0 ]] || fail "custom secret setup exited $LAST_RC: $LAST_OUT"
grep -q '^DEPLOY_HOOK_SECRET=not-generated$' "$custom/.env" || fail "setup replaced a custom DEPLOY_HOOK_SECRET"

# --with-updates enables the timer and the path unit that the hook trips.
updates=$(mktemp -d)
write_env "$updates" \
  "HOST_PORT=8080" \
  "PUBLIC_HOST=" \
  "BASIC_AUTH_PASSWORD="
run_setup "$updates" --with-updates
[[ "$LAST_RC" -eq 0 ]] || fail "--with-updates exited $LAST_RC: $LAST_OUT"
have "$LAST_SYSTEMCTL" "enable --now pantry-update.timer" "timer enabled"
have "$LAST_SYSTEMCTL" "enable --now pantry-update.path" "path unit enabled"

# An unreadable password file is 503 from /api/session. Setup must say so
# instead of reporting success.
unavailable=$(mktemp -d)
write_env "$unavailable" \
  "PUBLIC_HOST=pantry.example.com" \
  "ACME_EMAIL=you@example.com" \
  "BASIC_AUTH_USER=pantry" \
  "BASIC_AUTH_PASSWORD=" \
  "HOST_PORT=8080"
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$unavailable/auth.caddy"
CURL_SESSION_CODE=503 \
  CURL_SESSION_BODY='{"error":"Sign-in is unavailable right now."}' \
  run_setup "$unavailable"
[[ "$LAST_RC" -ne 0 ]] || fail "unavailable sign-in should fail setup"
have "$LAST_OUT" "Sign-in is unavailable" "unavailable sign-in is a loud error"
have "$LAST_OUT" "uid 65532" "unavailable sign-in names the container user"
have "$LAST_CURL" "X-Pantry-Entry: public" "unavailable probe used the public entry header"

# pantry-update.sh copies the hash, via the same function, before compose up.
updater="$ROOT/deploy/systemd/pantry-update.sh"
sync_line=$(grep -n 'sync_household_credential' "$updater" | head -1 | cut -d: -f1)
up_line=$(grep -n 'up -d' "$updater" | head -1 | cut -d: -f1)
[[ -n "$sync_line" && -n "$up_line" && "$sync_line" -lt "$up_line" ]] || fail "pantry-update.sh must copy the household hash before compose up"

# A file left root-owned by a partial run is chowned again, and tightened to 600.
# A failed chown stops the copy unless the no-sudo test seam is set.
owned=$(mktemp -d)
mkdir -p "$owned/auth"
printf '%s\n' 'basic_auth bcrypt Pantry {' '	pantry ORIGINAL-HASH' '}' > "$owned/auth.caddy"
printf '%s\n' 'stale' > "$owned/auth/household"
chmod 644 "$owned/auth/household"
own_bin=$(mktemp -d)
own_log=$(mktemp)
cat > "$own_bin/chown" << 'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${CHOWN_LOG:?}"
exit 0
EOF
chmod +x "$own_bin/chown"
# shellcheck disable=SC1091
CHOWN_LOG="$own_log" PANTRY_DIR="$owned" PATH="$own_bin:$PATH" \
  bash -c 'source "$1"; sync_household_credential' _ "$ROOT/deploy/auth-migrate.sh"
cmp "$owned/auth.caddy" "$owned/auth/household" || fail "direct copy did not replace a stale household file"
owned_mode=$(stat -c '%a' "$owned/auth/household")
[[ "$owned_mode" == "600" || "$owned_mode" == "640" ]] || fail "direct copy left mode $owned_mode"
have "$(cat "$own_log")" "65532:65532 $owned/auth/household" "direct copy chowns an existing household file"
have "$(cat "$own_log")" "65532:65532 $owned/auth" "direct copy chowns the auth directory"

fail_bin=$(mktemp -d)
cat > "$fail_bin/chown" << 'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "$fail_bin/chown"
set +e
PANTRY_DIR="$owned" PATH="$fail_bin:$PATH" \
  bash -c 'source "$1"; sync_household_credential' _ "$ROOT/deploy/auth-migrate.sh" >/tmp/pantry-chown-fail.out 2>&1
fail_rc=$?
set -e
[[ "$fail_rc" -ne 0 ]] || fail "chown failure should stop the household copy"
have "$(cat /tmp/pantry-chown-fail.out)" "Could not give" "chown failure names the path"
set +e
PANTRY_SETUP_SKIP_ROOT=1 PANTRY_DIR="$owned" PATH="$fail_bin:$PATH" \
  bash -c 'source "$1"; sync_household_credential' _ "$ROOT/deploy/auth-migrate.sh" >/tmp/pantry-chown-skip.out 2>&1
skip_rc=$?
set -e
[[ "$skip_rc" -eq 0 ]] || fail "PANTRY_SETUP_SKIP_ROOT should warn instead of failing chown, rc=$skip_rc"
have "$(cat /tmp/pantry-chown-skip.out)" "[warn] Could not give" "skip-root chown failure is a warning"
unset CURL_SESSION_CODE CURL_SESSION_BODY

rm -rf "$lan" "$pub" "$first" "$bad" "$incomplete" "$opt" "$mdns" "$split" "$badip" "$mdns_bin" "$tun" "$notoken" "$unready" "$tunsplit" "$mode_dir" "$custom" "$updates" "$unavailable" "$owned" "$own_bin" "$fail_bin"
rm -f /tmp/pantry-chown-fail.out /tmp/pantry-chown-skip.out
echo "setup_apply_test ok"
