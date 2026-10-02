#!/usr/bin/env bash
# Behavioral checks for the single setup command. Docker, systemd, and
# iptables are stubs so this can run without root or a Pi.

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SETUP="$ROOT/deploy/setup.sh"

bash -n "$SETUP"
bash -n "$ROOT/deploy/firewall/pantry-lan-only.sh"
bash -n "$ROOT/deploy/systemd/pantry-update.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

LAST_RC=0
LAST_OUT=""
LAST_DOCKER=""
LAST_IPTABLES=""
LAST_SYSTEMCTL=""

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

  cat > "$bin/curl" << 'EOF'
#!/usr/bin/env bash
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

  chmod +x "$bin/docker" "$bin/systemctl" "$bin/iptables" "$bin/curl" "$bin/usermod" "$bin/udevadm"
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
  mkdir -p "$work/systemd"

  set +e
  DOCKER_LOG="$work/docker.log" \
    SYSTEMCTL_LOG="$work/systemctl.log" \
    IPTABLES_LOG="$work/iptables.log" \
    PANTRY_DIR="$pantry" \
    PANTRY_SYSTEMD_UNIT_DIR="$work/systemd" \
    PANTRY_SETUP_SKIP_ROOT=1 \
    PATH="$bin:$PATH" \
    bash "$SETUP" "$@" >"$out" 2>"$err"
  LAST_RC=$?
  set -e

  LAST_OUT=$(cat "$out" "$err")
  LAST_DOCKER=$(cat "$work/docker.log")
  LAST_IPTABLES=$(cat "$work/iptables.log")
  LAST_SYSTEMCTL=$(cat "$work/systemctl.log")
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
lack "$LAST_DOCKER" "hash-password" "LAN must not hash a password"
lack "$LAST_DOCKER" "restart pantry-caddy" "LAN must not restart Caddy"
have "$LAST_IPTABLES" "--dport 9090" "firewall uses HOST_PORT"
have "$LAST_OUT" "Pantry setup complete" "LAN success"
have "$LAST_OUT" "Non-LAN clients are blocked on port 9090" "LAN firewall summary"
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
have "$LAST_DOCKER" "restart pantry-caddy" "Caddyfile restart"
lack "$LAST_DOCKER" "hash-password" "must not hash when auth.caddy exists"
lack "$LAST_DOCKER" "stop caddy" "must not disable public HTTPS"
have "$LAST_OUT" "Keeping existing" "kept auth.caddy"
have "$LAST_IPTABLES" "--dport 8080" "public setup still firewalls the LAN port"

# A second run stays idempotent.
run_setup "$pub"
[[ "$LAST_RC" -eq 0 ]] || fail "second setup exited $LAST_RC: $LAST_OUT"
cmp "$pub/auth.caddy" "$pub/auth.caddy.before" || fail "second run regenerated auth.caddy"
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

rm -rf "$lan" "$pub" "$first" "$bad" "$incomplete" "$opt"
echo "setup_apply_test ok"
