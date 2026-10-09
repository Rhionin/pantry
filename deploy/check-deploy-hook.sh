#!/usr/bin/env bash
# Static contract for the deploy hook: the signature bytes, the public
# Caddy exception, the host path unit, and the workflow skip.

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

have() {
  local file="$1" needle="$2" label="$3"
  if ! grep -q -F -- "$needle" "$file"; then
    fail "$label (missing $needle in $file)"
  fi
}

# Same vector as internal/deployhook/hook_test.go. The workflow signs with
# this openssl invocation, so a drift here fails before a push.
secret=test-secret
ts=1700000000
body='{"sha":"abc1234","ref":"master"}'
sig=$(printf '%s.%s' "$ts" "$body" | openssl dgst -sha256 -hmac "$secret" -hex | awk '{print $NF}')
have internal/deployhook/hook_test.go "$sig" "Go test vector matches openssl"
have .github/workflows/ci.yml "printf '%s.%s'" "workflow signs timestamp, a dot, then the body"
have .github/workflows/ci.yml 'X-Pantry-Deploy-Timestamp' "workflow sends the timestamp header"
have .github/workflows/ci.yml 'X-Pantry-Deploy-Signature' "workflow sends the signature header"
have .github/workflows/ci.yml 'https://pantry.rhionin.com/api/deploy-hook' "workflow posts to the public hook"
have .github/workflows/ci.yml 'DEPLOY_HOOK_SECRET is not set; skipping the deploy hook' "workflow skips when the repo secret is unset"
have .github/workflows/ci.yml "github.ref == 'refs/heads/master'" "workflow notifies only after a master push"
have .github/workflows/ci.yml "inputs.publish_image != 'skip'" "reusable Release run does not publish the image"
have .github/workflows/release.yml 'publish_image: skip' "Release does not publish :latest and :master"

have deploy/Caddyfile '@deploy path /api/deploy-hook' "certificate Caddyfile exempts the hook"
have deploy/Caddyfile.tunnel '@deploy path /api/deploy-hook' "tunnel Caddyfile exempts the hook"
# The exception has to be above the catch-all household handle. route_body
# equality is checked by check-publish-modes.sh; here, confirm the hook is
# not inside that handle. `handle {` is the catch-all; `handle @name {` is not.
caddy_before=$(awk '/handle \{/{exit} {print}' deploy/Caddyfile)
printf '%s\n' "$caddy_before" | grep -q -F '@deploy path /api/deploy-hook' || fail "deploy hook is behind the household login"

have deploy/systemd/pantry-update.timer 'OnUnitActiveSec=1min' "fallback poll is one minute"
if grep -q 'OnUnitActiveSec=5min' deploy/systemd/pantry-update.timer; then
  fail "timer still polls every 5 minutes"
fi
have deploy/systemd/pantry-update.path 'PathChanged=/opt/pantry/deploy-trigger/request' "path unit watches the trigger file"
have deploy/systemd/pantry-update.path 'Unit=pantry-update.service' "path unit starts the existing update service"
have deploy/systemd/pantry-update.sh 'flock' "update script serializes overlapping triggers"
have deploy/systemd/pantry-update.sh 'trigger changed during the pull' "update script pulls again when a trigger lands mid-run"
have deploy/systemd/pantry-update.sh 'apply_signed_setup' "update script applies setup from the trigger"
have deploy/systemd/pantry-setup.sh 'merge-base --is-ancestor' "setup accepts only an ancestor of origin/master"
have deploy/systemd/pantry-setup.sh 'https://github.com/Rhionin/pantry.git' "setup fetches GitHub, not the request"
have deploy/systemd/pantry-setup.sh 'apply </dev/null' "setup.sh runs with stdin closed"
have deploy/systemd/pantry-setup.sh 'PANTRY_SETUP_SKIP_PACKAGES=1' "automatic setup does not install packages"
have deploy/systemd/pantry-setup.sh 'PANTRY_AUTO_SETUP' "auto setup can be turned off"
have deploy/docker-compose.yml './setup-state:/etc/pantry/setup-state:ro' "setup status mount is read-only"
have deploy/setup.sh 'PANTRY_SETUP_SKIP_PACKAGES' "setup.sh honors the no-install path"
have internal/server/handler_build.go 'setupCommit' "public build info reports the applied setup commit"

setup_line=$(grep -n 'apply_signed_setup' deploy/systemd/pantry-update.sh | tail -1 | cut -d: -f1)
compose_line=$(grep -n 'apply_compose' deploy/systemd/pantry-update.sh | tail -1 | cut -d: -f1)
lock_line=$(grep -n 'flock -n 9' deploy/systemd/pantry-update.sh | head -1 | cut -d: -f1)
[[ -n "$setup_line" && -n "$compose_line" && "$setup_line" -lt "$compose_line" ]] || fail "setup must run before the image pull"
[[ -n "$lock_line" && "$lock_line" -lt "$setup_line" ]] || fail "setup must run inside the update lock"

if grep -q '"os/exec"' internal/deployhook/hook.go || grep -q 'exec.Command' internal/deployhook/hook.go; then
  fail "deploy hook must only write the trigger file"
fi
if grep -q 'eval ' deploy/systemd/pantry-setup.sh; then
  fail "setup updater must not eval the trigger"
fi
if grep -Eq 'src:|/opt/pantry/src' deploy/docker-compose.yml deploy/docker-compose.tunnel.yml; then
  fail "compose must not mount the setup checkout into the container"
fi

# The compose value is a literal ${...} interpolation, not a shell expansion.
# shellcheck disable=SC2016
have deploy/docker-compose.yml 'DEPLOY_HOOK_SECRET: ${DEPLOY_HOOK_SECRET:-}' "compose passes the hook secret"
have deploy/docker-compose.yml './deploy-trigger:/deploy-trigger' "compose mounts the trigger directory"
have deploy/setup.sh 'openssl rand -hex 32' "setup generates the hook secret"
have deploy/setup.sh 'deploy-secret' "setup can print the hook secret"
have deploy/setup.sh 'pantry-update.path' "setup installs the path unit"
have deploy/.env.example 'DEPLOY_HOOK_SECRET=' ".env.example leaves the hook secret empty"
have deploy/README.md 'sudo ./setup.sh deploy-secret' "README prints the Pi command"
have deploy/README.md 'DEPLOY_HOOK_SECRET' "README names the repo secret"

echo "check-deploy-hook ok"
