#!/usr/bin/env bash
# The signed updater applies setup.sh only for a commit that is on
# origin/master. These checks use a local git remote so they do not talk
# to GitHub, and a stub setup.sh so they do not need root or Docker.

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
UPDATER="$ROOT/deploy/systemd/pantry-setup.sh"

bash -n "$UPDATER"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

have() {
  local haystack="$1" needle="$2" label="$3"
  if ! printf '%s\n' "$haystack" | grep -q -F -- "$needle"; then
    fail "$label (missing $needle) in: $haystack"
  fi
}

lack() {
  local haystack="$1" needle="$2" label="$3"
  if printf '%s\n' "$haystack" | grep -q -F -- "$needle"; then
    fail "$label (unexpected $needle) in: $haystack"
  fi
}

gitc() {
  git -c user.name=PantryTest -c user.email=pantry@example.com "$@"
}

WORK=""
SHA_A=""
SHA_B=""
SHA_C=""
LAST_OUT=""
STUB_LOG=""

cleanup() {
  if [[ -n "$WORK" && -d "$WORK" ]]; then
    rm -rf "$WORK"
  fi
}
trap cleanup EXIT

export PANTRY_SETUP_TEST=1
export GIT_TERMINAL_PROMPT=0
export PANTRY_DIR=/tmp/pantry-setup-sync-unused
export PANTRY_SETUP_CHECKOUT=/tmp/pantry-setup-sync-unused/src
export PANTRY_SETUP_REMOTE=https://github.com/Rhionin/pantry.git
export PANTRY_SETUP_STATE_DIR=/tmp/pantry-setup-sync-unused/setup-state
export PANTRY_SETUP_STATUS_FILE=/tmp/pantry-setup-sync-unused/setup-state/status.json
export PANTRY_TRIGGER_FILE=/tmp/pantry-setup-sync-unused/request
export PANTRY_SETUP_NOW=2026-10-09T14:00:00Z
# shellcheck disable=SC1090,SC1091
source "$UPDATER"

# The real check curls the Pi. Fail only the sha a test names.
setup_health_ok() {
  local head
  if [[ "${PANTRY_SETUP_HEALTH_FAIL:-}" == 1 ]]; then
    head=$(git -C "${PANTRY_SETUP_CHECKOUT}" rev-parse HEAD 2>/dev/null || true)
    if [[ -n "${PANTRY_SETUP_HEALTH_FAIL_SHA:-}" && "$head" == "$PANTRY_SETUP_HEALTH_FAIL_SHA" ]]; then
      return 1
    fi
    if [[ -z "${PANTRY_SETUP_HEALTH_FAIL_SHA:-}" ]]; then
      return 1
    fi
  fi
  return 0
}

# new_fixture builds master commits A then C, and a side commit B that is
# not an ancestor of master. The updater's remote is that local bare repo.
new_fixture() {
  local src origin
  cleanup
  WORK=$(mktemp -d)
  src="$WORK/src-seed"
  origin="$WORK/origin.git"
  mkdir -p "$src"
  gitc init -b master "$src" >/dev/null 2>&1
  printf 'a\n' > "$src/a"
  gitc -C "$src" add a
  gitc -C "$src" commit -m 'A' >/dev/null
  SHA_A=$(gitc -C "$src" rev-parse HEAD)
  gitc -C "$src" checkout -q -b feature
  printf 'b\n' > "$src/b"
  gitc -C "$src" add b
  gitc -C "$src" commit -m 'B' >/dev/null
  SHA_B=$(gitc -C "$src" rev-parse HEAD)
  gitc -C "$src" checkout -q master
  printf 'c\n' > "$src/a"
  gitc -C "$src" add a
  gitc -C "$src" commit -m 'C' >/dev/null
  SHA_C=$(gitc -C "$src" rev-parse HEAD)
  gitc clone --bare "$src" "$origin" >/dev/null 2>&1

  export PANTRY_DIR="$WORK/opt"
  export PANTRY_SETUP_CHECKOUT="$WORK/checkout"
  export PANTRY_SETUP_REMOTE="$origin"
  export PANTRY_SETUP_STATE_DIR="$WORK/opt/setup-state"
  export PANTRY_SETUP_STATUS_FILE="$WORK/opt/setup-state/status.json"
  export PANTRY_TRIGGER_FILE="$WORK/opt/deploy-trigger/request"
  export PANTRY_SETUP_SCRIPT="$WORK/stub-setup.sh"
  export PANTRY_SETUP_NOW=2026-10-09T14:00:00Z
  export STUB_LOG="$WORK/stub.log"
  unset FAIL_SHA
  unset PANTRY_SETUP_HEALTH_FAIL
  unset PANTRY_SETUP_HEALTH_FAIL_SHA
  mkdir -p "$PANTRY_DIR/deploy-trigger" "$PANTRY_SETUP_STATE_DIR"
  : > "$STUB_LOG"

  cat > "$PANTRY_SETUP_SCRIPT" << 'EOF'
#!/usr/bin/env bash
set -euo pipefail
line=""
if IFS= read -r line; then
  printf 'stdin:%s\n' "$line" >> "${STUB_LOG:?}"
  exit 1
fi
head=$(git -C "${PANTRY_SETUP_CHECKOUT:?}" rev-parse HEAD)
printf 'eof head=%s packages=%s args=%s\n' "$head" "${PANTRY_SETUP_SKIP_PACKAGES:-}" "$*" >> "${STUB_LOG:?}"
if [[ -n "${FAIL_SHA:-}" && "$head" == "$FAIL_SHA" ]]; then
  exit 1
fi
exit 0
EOF
  chmod +x "$PANTRY_SETUP_SCRIPT"
}

run_apply() {
  LAST_OUT=$(printf 'PROMPT_DATA\n' | apply_signed_setup 2>&1)
}

write_trigger() {
  printf '%s\n' "$1" > "$PANTRY_TRIGGER_FILE"
}

# A full master sha is applied with stdin closed. The trigger's ref is text.
new_fixture
write_trigger "sha=${SHA_C} ref=\$(touch ${WORK}/pwned)"
printf 'DEPLOY_HOOK_SECRET=kept-hook-secret\nBASIC_AUTH_PASSWORD=correct horse battery staple\nPANTRY_IMAGE_TAG=pinned-by-operator\n' > "$PANTRY_DIR/.env"
printf 'auth-marker\n' > "$PANTRY_DIR/auth.caddy"
cp "$PANTRY_DIR/.env" "$WORK/env.before"
cp "$PANTRY_DIR/auth.caddy" "$WORK/auth.before"
run_apply
have "$LAST_OUT" "pantry setup: applied ${SHA_C}" "tip of master is applied"
lack "$LAST_OUT" "PROMPT_DATA" "apply does not echo stdin"
grep -q "^eof head=${SHA_C} packages=1 args=apply$" "$STUB_LOG" || fail "setup.sh was not run with packages skipped and stdin closed: $(cat "$STUB_LOG")"
lack "$(cat "$STUB_LOG")" "PROMPT_DATA" "setup.sh saw the prompt bytes"
[[ ! -e "$WORK/pwned" ]] || fail "trigger ref was executed"
cmp "$PANTRY_DIR/.env" "$WORK/env.before" || fail "updater rewrote .env"
cmp "$PANTRY_DIR/auth.caddy" "$WORK/auth.before" || fail "updater rewrote auth.caddy"
printf '{"setupCommit":"%s","setupAppliedAt":"2026-10-09T14:00:00Z","setupStatus":"applied"}\n' "$SHA_C" > "$WORK/expect-status"
cmp "$PANTRY_SETUP_STATUS_FILE" "$WORK/expect-status" || fail "status file was $(cat "$PANTRY_SETUP_STATUS_FILE")"
[[ "$(tr -d '[:space:]' < "$PANTRY_SETUP_STATE_DIR/last-known-good")" == "$SHA_C" ]] || fail "last-known-good was not the applied commit"
[[ "$(git -C "$PANTRY_SETUP_CHECKOUT" rev-parse HEAD)" == "$SHA_C" ]] || fail "checkout is not the trigger commit"

# The same trigger on the next minute must not run setup.sh again.
stub_lines=$(wc -l < "$STUB_LOG")
run_apply
have "$LAST_OUT" "already applied" "a repeated sha does not re-apply"
lack "$LAST_OUT" "applying" "a repeated sha does not start setup"
[[ "$(wc -l < "$STUB_LOG")" -eq "$stub_lines" ]] || fail "setup.sh ran again for an already-applied sha: $(cat "$STUB_LOG")"
cmp "$PANTRY_SETUP_STATUS_FILE" "$WORK/expect-status" || fail "repeated sha rewrote status: $(cat "$PANTRY_SETUP_STATUS_FILE")"
[[ ! -f "$PANTRY_SETUP_STATE_DIR/in-progress" ]] || fail "successful apply left the in-progress marker"

# A downgrade behind last-known-good is refused. The container can write the trigger.
write_trigger "sha=${SHA_A} ref=master"
: > "$STUB_LOG"
run_apply
have "$LAST_OUT" "REFUSING ${SHA_A}" "ancestor of last-known-good is refused"
have "$LAST_OUT" "ancestor of last-known-good" "downgrade names the reason"
lack "$LAST_OUT" "applying" "downgrade was applied"
[[ ! -s "$STUB_LOG" ]] || fail "setup.sh ran for a downgrade: $(cat "$STUB_LOG")"
[[ "$(git -C "$PANTRY_SETUP_CHECKOUT" rev-parse HEAD)" == "$SHA_C" ]] || fail "downgrade moved the checkout"
grep -q '"setupStatus":"refused"' "$PANTRY_SETUP_STATUS_FILE" || fail "downgrade did not record refused: $(cat "$PANTRY_SETUP_STATUS_FILE")"
grep -q "$SHA_A" "$PANTRY_SETUP_STATUS_FILE" || fail "refused status omitted the sha"

# Equality with last-known-good is not a re-apply when last-applied was cleared.
rm -f "$PANTRY_SETUP_STATE_DIR/last-applied"
write_trigger "sha=${SHA_C} ref=master"
: > "$STUB_LOG"
run_apply
have "$LAST_OUT" "is last-known-good" "known-good sha is not applied again"
[[ ! -s "$STUB_LOG" ]] || fail "setup.sh ran for last-known-good: $(cat "$STUB_LOG")"

# An ancestor of origin/master is accepted when nothing is known-good yet.
new_fixture
write_trigger "sha=${SHA_A} ref=master"
run_apply
have "$LAST_OUT" "pantry setup: applied ${SHA_A}" "ancestor of master is applied"
[[ "$(git -C "$PANTRY_SETUP_CHECKOUT" rev-parse HEAD)" == "$SHA_A" ]] || fail "checkout did not move to the ancestor"

# A descendant of last-known-good is the forward step the hook is allowed to take.
new_fixture
printf '%s\n' "$SHA_A" > "$PANTRY_SETUP_STATE_DIR/last-known-good"
write_trigger "sha=${SHA_C} ref=master"
run_apply
have "$LAST_OUT" "pantry setup: applied ${SHA_C}" "descendant of last-known-good is applied"
[[ "$(tr -d '[:space:]' < "$PANTRY_SETUP_STATE_DIR/last-known-good")" == "$SHA_C" ]] || fail "forward apply did not advance last-known-good"

# A branch-only commit is refused, including after its objects are present.
new_fixture
write_trigger "sha=${SHA_B} ref=feature"
run_apply
have "$LAST_OUT" "REFUSING ${SHA_B}" "branch-only sha is refused"
lack "$LAST_OUT" "applying" "refused sha was applied"
[[ ! -s "$STUB_LOG" ]] || fail "setup.sh ran for a branch-only sha: $(cat "$STUB_LOG")"
grep -q '"setupStatus":"refused"' "$PANTRY_SETUP_STATUS_FILE" || fail "branch-only sha did not record refused"
ensure_setup_checkout
gitc -C "$PANTRY_SETUP_CHECKOUT" fetch origin 'refs/heads/feature:refs/heads/feature' >/dev/null 2>&1
write_trigger "sha=${SHA_B} ref=feature"
run_apply
have "$LAST_OUT" "REFUSING ${SHA_B}" "branch-only sha is refused when the object exists"
[[ ! -s "$STUB_LOG" ]] || fail "setup.sh ran for a fetched branch-only sha"

# An unknown commit, a short sha, and a shell snippet are not commands.
new_fixture
write_trigger "sha=0000000000000000000000000000000000000000 ref=master"
run_apply
have "$LAST_OUT" "REFUSING 0000000000000000000000000000000000000000" "unknown sha is refused"
[[ ! -s "$STUB_LOG" ]] || fail "setup.sh ran for an unknown sha"

write_trigger "sha=abc1234 ref=master"
run_apply
have "$LAST_OUT" "no full commit" "short sha is ignored"
[[ ! -s "$STUB_LOG" ]] || fail "setup.sh ran for a short sha"

write_trigger "sha=${SHA_C};touch ${WORK}/pwned ref=master"
run_apply
[[ ! -e "$WORK/pwned" ]] || fail "sha suffix was executed"
have "$LAST_OUT" "no full commit" "sha with a suffix is ignored"

printf 'sha=%s\ntouch %s\n' "$SHA_C" "$WORK/pwned" > "$PANTRY_TRIGGER_FILE"
run_apply
[[ ! -e "$WORK/pwned" ]] || fail "second trigger line was executed"
have "$LAST_OUT" "applied ${SHA_C}" "first line still names the commit to apply"

# Opt-out leaves secrets and does not clone.
new_fixture
printf 'PANTRY_AUTO_SETUP=off\nDEPLOY_HOOK_SECRET=kept-hook-secret\n' > "$PANTRY_DIR/.env"
printf 'auth-marker\n' > "$PANTRY_DIR/auth.caddy"
cp "$PANTRY_DIR/.env" "$WORK/env.before"
cp "$PANTRY_DIR/auth.caddy" "$WORK/auth.before"
write_trigger "sha=${SHA_C} ref=master"
run_apply
have "$LAST_OUT" "PANTRY_AUTO_SETUP=off" "opt-out is logged"
[[ ! -d "$PANTRY_SETUP_CHECKOUT/.git" ]] || fail "opt-out cloned a checkout"
[[ ! -s "$STUB_LOG" ]] || fail "opt-out ran setup.sh"
cmp "$PANTRY_DIR/.env" "$WORK/env.before" || fail "opt-out rewrote .env"
cmp "$PANTRY_DIR/auth.caddy" "$WORK/auth.before" || fail "opt-out rewrote auth.caddy"

# A failed apply rolls back to last-known-good. With none recorded, it stops.
new_fixture
printf '%s\n' "$SHA_A" > "$PANTRY_SETUP_STATE_DIR/last-known-good"
export FAIL_SHA="$SHA_C"
write_trigger "sha=${SHA_C} ref=master"
run_apply
have "$LAST_OUT" "FAILED applying ${SHA_C}" "failed apply is loud"
have "$LAST_OUT" "ROLLING BACK to last-known-good ${SHA_A}" "rollback names the good commit"
have "$LAST_OUT" "restored last-known-good ${SHA_A}" "rollback finished"
printf '{"setupCommit":"%s","setupAppliedAt":"2026-10-09T14:00:00Z","setupStatus":"rolled-back"}\n' "$SHA_A" > "$WORK/expect-status"
cmp "$PANTRY_SETUP_STATUS_FILE" "$WORK/expect-status" || fail "rollback status was $(cat "$PANTRY_SETUP_STATUS_FILE")"
[[ "$(git -C "$PANTRY_SETUP_CHECKOUT" rev-parse HEAD)" == "$SHA_A" ]] || fail "rollback left the failed commit checked out"
grep -q "eof head=${SHA_A} packages=1 args=apply" "$STUB_LOG" || fail "rollback did not re-apply known good without a prompt: $(cat "$STUB_LOG")"
lack "$(cat "$STUB_LOG")" "PROMPT_DATA" "rollback setup.sh saw prompt bytes"

new_fixture
export FAIL_SHA="$SHA_C"
write_trigger "sha=${SHA_C} ref=master"
run_apply
have "$LAST_OUT" "no last-known-good" "missing known good is loud"
printf '{"setupAppliedAt":"2026-10-09T14:00:00Z","setupStatus":"failed"}\n' > "$WORK/expect-status"
cmp "$PANTRY_SETUP_STATUS_FILE" "$WORK/expect-status" || fail "failed status was $(cat "$PANTRY_SETUP_STATUS_FILE")"
[[ ! -e "$WORK/pwned" ]] || fail "failed apply executed a side file"

# A tampered origin URL is replaced with the configured remote before fetch.
new_fixture
write_trigger "sha=${SHA_C} ref=master"
run_apply
gitc -C "$PANTRY_SETUP_CHECKOUT" remote set-url origin https://evil.example/pantry.git
: > "$STUB_LOG"
rm -f "$PANTRY_SETUP_STATE_DIR/last-applied" "$PANTRY_SETUP_STATE_DIR/last-known-good"
run_apply
have "$LAST_OUT" "applied ${SHA_C}" "fetch still uses the configured remote"
[[ "$(git -C "$PANTRY_SETUP_CHECKOUT" remote get-url origin)" == "$PANTRY_SETUP_REMOTE" ]] || fail "origin URL was left pointing at the tampered remote"

# Anything other than the GitHub repository is refused outside the test seam,
# and a non-github URL is refused even in the test seam.
(
  unset PANTRY_SETUP_TEST
  unset PANTRY_SETUP_SCRIPT
  # shellcheck disable=SC1090,SC1091
  source "$UPDATER"
  [[ "$PANTRY_SETUP_REMOTE" == "https://github.com/Rhionin/pantry.git" ]] || exit 1
  [[ "$PANTRY_SETUP_CHECKOUT" == "/opt/pantry/src" ]] || exit 1
  [[ "$PANTRY_TRIGGER_FILE" == "/opt/pantry/deploy-trigger/request" ]] || exit 1
  [[ -z "${PANTRY_SETUP_SCRIPT:-}" ]] || exit 1
  setup_remote_ok "https://github.com/Rhionin/pantry.git"
  if setup_remote_ok "/tmp/not-github.git"; then
    exit 1
  fi
  if setup_remote_ok "--upload-pack=touch"; then
    exit 1
  fi
) || fail "production updater must fetch only https://github.com/Rhionin/pantry.git"

new_fixture
export PANTRY_SETUP_REMOTE="https://evil.example/pantry.git"
write_trigger "sha=${SHA_C} ref=master"
run_apply
have "$LAST_OUT" "refusing remote" "non-github remote is refused"
have "$LAST_OUT" "not applying ${SHA_C}" "refused remote does not apply"
[[ ! -d "$PANTRY_SETUP_CHECKOUT/.git" ]] || fail "refused remote still cloned"
[[ ! -s "$STUB_LOG" ]] || fail "refused remote ran setup.sh"
grep -q '"setupStatus":"failed"' "$PANTRY_SETUP_STATUS_FILE" || fail "refused remote did not record failure"

# A dirty checkout cannot block the next forward apply on a prompt.
new_fixture
write_trigger "sha=${SHA_A} ref=master"
run_apply
printf 'dirty\n' > "$PANTRY_SETUP_CHECKOUT/a"
write_trigger "sha=${SHA_C} ref=master"
run_apply
have "$LAST_OUT" "applied ${SHA_C}" "dirty checkout still applies"
[[ "$(git -C "$PANTRY_SETUP_CHECKOUT" rev-parse HEAD)" == "$SHA_C" ]] || fail "force checkout did not move off the dirty tree"

# Health failure rolls back and does not record the bad commit as known-good.
new_fixture
printf '%s\n' "$SHA_A" > "$PANTRY_SETUP_STATE_DIR/last-known-good"
export PANTRY_SETUP_HEALTH_FAIL=1
export PANTRY_SETUP_HEALTH_FAIL_SHA="$SHA_C"
write_trigger "sha=${SHA_C} ref=master"
run_apply
have "$LAST_OUT" "FAILED health check after ${SHA_C}" "unhealthy apply is loud"
have "$LAST_OUT" "ROLLING BACK to last-known-good ${SHA_A}" "unhealthy apply rolls back"
have "$LAST_OUT" "restored last-known-good ${SHA_A}" "unhealthy apply finishes the rollback"
[[ "$(tr -d '[:space:]' < "$PANTRY_SETUP_STATE_DIR/last-known-good")" == "$SHA_A" ]] || fail "health failure advanced last-known-good"
printf '{"setupCommit":"%s","setupAppliedAt":"2026-10-09T14:00:00Z","setupStatus":"rolled-back"}\n' "$SHA_A" > "$WORK/expect-status"
cmp "$PANTRY_SETUP_STATUS_FILE" "$WORK/expect-status" || fail "health rollback status was $(cat "$PANTRY_SETUP_STATUS_FILE")"
[[ ! -f "$PANTRY_SETUP_STATE_DIR/in-progress" ]] || fail "health rollback left the in-progress marker"
unset PANTRY_SETUP_HEALTH_FAIL PANTRY_SETUP_HEALTH_FAIL_SHA

# A killed apply leaves the marker. The next run records failure and rolls back.
new_fixture
write_trigger "sha=${SHA_A} ref=master"
run_apply
printf '%s\n' "$SHA_C" > "$PANTRY_SETUP_STATE_DIR/in-progress"
write_trigger "ref=master"
: > "$STUB_LOG"
run_apply
have "$LAST_OUT" "previous apply was interrupted" "a leftover marker is an interrupted apply"
have "$LAST_OUT" "ROLLING BACK to last-known-good ${SHA_A}" "interrupted apply rolls back"
[[ ! -f "$PANTRY_SETUP_STATE_DIR/in-progress" ]] || fail "interrupted apply left the marker"
[[ "$(git -C "$PANTRY_SETUP_CHECKOUT" rev-parse HEAD)" == "$SHA_A" ]] || fail "interrupted apply left a different checkout"

# A symlink or an oversized trigger is not read and is not copied into the log.
new_fixture
printf 'DEPLOY_HOOK_SECRET=kept-hook-secret\n' > "$PANTRY_DIR/.env"
rm -f "$PANTRY_TRIGGER_FILE"
ln -s "$PANTRY_DIR/.env" "$PANTRY_TRIGGER_FILE"
run_apply
have "$LAST_OUT" "REFUSING trigger" "symlink trigger is refused"
lack "$LAST_OUT" "kept-hook-secret" "symlink target was logged"
[[ ! -s "$STUB_LOG" ]] || fail "setup.sh ran for a symlink trigger"
rm -f "$PANTRY_TRIGGER_FILE"
dd if=/dev/zero bs=2048 count=1 2>/dev/null | tr '\0' 'a' > "$PANTRY_TRIGGER_FILE"
run_apply
have "$LAST_OUT" "1024" "oversized trigger is refused"
lack "$LAST_OUT" "aaaa" "oversized trigger body was logged"
[[ ! -s "$STUB_LOG" ]] || fail "setup.sh ran for an oversized trigger"
rm -f "$PANTRY_TRIGGER_FILE"
mkdir "$PANTRY_TRIGGER_FILE"
run_apply
have "$LAST_OUT" "REFUSING trigger" "directory trigger is refused"

# Installing deploy files replaces the running updater inode instead of rewriting it.
inode_dest=$(mktemp -d)
mkdir -p "$inode_dest/systemd"
printf 'old-updater\n' > "$inode_dest/systemd/pantry-update.sh"
inode_before=$(stat -c '%i' "$inode_dest/systemd/pantry-update.sh")
PANTRY_SETUP_SOURCE_ONLY=1 PANTRY_DIR="$inode_dest" \
  bash -c 'source "$1"; copy_deploy_files' _ "$ROOT/deploy/setup.sh" >/dev/null
inode_after=$(stat -c '%i' "$inode_dest/systemd/pantry-update.sh")
[[ "$inode_before" != "$inode_after" ]] || fail "install rewrote pantry-update.sh in place"
cmp "$ROOT/deploy/systemd/pantry-update.sh" "$inode_dest/systemd/pantry-update.sh" || fail "installed pantry-update.sh does not match the source"
cmp "$ROOT/deploy/systemd/pantry-setup.sh" "$inode_dest/systemd/pantry-setup.sh" || fail "installed pantry-setup.sh does not match the source"
grep -q 'main "$@"' "$inode_dest/systemd/pantry-update.sh" || fail "installed updater is not wrapped in main"
grep -q 'main "$@"' "$inode_dest/systemd/pantry-setup.sh" || fail "installed setup helper is not wrapped in main"
rm -rf "$inode_dest"

# A manual setup records last-known-good. The automatic path does not.
seed_dir=$(mktemp -d)
PANTRY_SETUP_SOURCE_ONLY=1 PANTRY_DIR="$seed_dir" \
  bash -c 'source "$1"; seed_known_good_setup' _ "$ROOT/deploy/setup.sh" >/dev/null
expected_sha=$(git -C "$ROOT" rev-parse HEAD)
[[ "$(tr -d '[:space:]' < "$seed_dir/setup-state/last-known-good")" == "$expected_sha" ]] || fail "manual setup did not seed last-known-good"
[[ "$(tr -d '[:space:]' < "$seed_dir/setup-state/last-applied")" == "$expected_sha" ]] || fail "manual setup did not seed last-applied"
PANTRY_SETUP_SOURCE_ONLY=1 PANTRY_SETUP_SKIP_PACKAGES=1 PANTRY_DIR="$seed_dir" \
  bash -c 'source "$1"; rm -f "$PANTRY_DIR/setup-state/last-known-good"; seed_known_good_setup' _ "$ROOT/deploy/setup.sh"
[[ ! -f "$seed_dir/setup-state/last-known-good" ]] || fail "automatic setup seeded last-known-good"
rm -rf "$seed_dir"

cleanup
trap - EXIT
echo "setup_sync_test ok"
