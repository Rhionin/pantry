#!/usr/bin/env bash
# Apply deploy/setup.sh from a signed deploy trigger, then return.
#
# pantry-update.sh sources this file inside its existing lock. The trigger
# file is data: this script never evals it, and it never passes ref, a path,
# or a shell metacharacter to git. A sha is used only after it is 40 hex
# digits and an ancestor of origin/master fetched from GitHub (the tip counts).
# The request is not a command, and the pantry container is not given a root
# socket — the host path unit starts the updater that is already root.
#
# PANTRY_AUTO_SETUP=off in the deploy .env skips this and leaves the image
# pull as the only automatic step. Package installs and Docker upgrades stay
# out of this path; the one manual `sudo ./setup.sh` remains how those happen.

# Production paths are fixed here so a polluted environment (or a trigger
# line) cannot point git at another remote or another script. Tests opt in
# with PANTRY_SETUP_TEST=1 before sourcing this file.
if [[ "${PANTRY_SETUP_TEST:-}" != 1 ]]; then
  PANTRY_DIR=/opt/pantry
  PANTRY_SETUP_CHECKOUT=/opt/pantry/src
  PANTRY_SETUP_REMOTE=https://github.com/Rhionin/pantry.git
  PANTRY_SETUP_STATE_DIR=/opt/pantry/setup-state
  PANTRY_SETUP_STATUS_FILE=/opt/pantry/setup-state/status.json
  PANTRY_TRIGGER_FILE=/opt/pantry/deploy-trigger/request
  unset PANTRY_SETUP_SCRIPT
else
  : "${PANTRY_DIR:=/opt/pantry}"
  : "${PANTRY_SETUP_CHECKOUT:=${PANTRY_DIR}/src}"
  : "${PANTRY_SETUP_REMOTE:=https://github.com/Rhionin/pantry.git}"
  : "${PANTRY_SETUP_STATE_DIR:=${PANTRY_DIR}/setup-state}"
  : "${PANTRY_SETUP_STATUS_FILE:=${PANTRY_SETUP_STATE_DIR}/status.json}"
  : "${PANTRY_TRIGGER_FILE:=${PANTRY_DIR}/deploy-trigger/request}"
fi

# setup_remote_ok accepts only the GitHub repository. Tests may use an
# absolute local path so the ancestor check runs without a network.
setup_remote_ok() {
  local remote="$1"
  [[ -n "$remote" ]] || return 1
  [[ "$remote" != *$'\n'* && "$remote" != *$'\r'* ]] || return 1
  [[ "$remote" != -* ]] || return 1
  if [[ "$remote" == "https://github.com/Rhionin/pantry.git" ]]; then
    return 0
  fi
  if [[ "${PANTRY_SETUP_TEST:-}" == 1 && "$remote" == /* && "$remote" != *..* ]]; then
    return 0
  fi
  return 1
}

# auto_setup_enabled is false only for the documented opt-out. The value is
# not executed; anything other than "off" keeps the apply.
auto_setup_enabled() {
  local file="${PANTRY_DIR}/.env" line flag
  [[ -f "$file" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" == PANTRY_AUTO_SETUP=* ]] || continue
    flag=${line#PANTRY_AUTO_SETUP=}
    flag=${flag%$'\r'}
    flag=$(printf '%s' "$flag" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')
    [[ "$flag" != "off" ]]
    return
  done < "$file"
  return 0
}

# trigger_setup_sha prints a full lowercase commit from the trigger file.
# A short sha, a ref, or any other text prints nothing and is not executed.
trigger_setup_sha() {
  local file="${PANTRY_TRIGGER_FILE}" line sha
  [[ -f "$file" ]] || return 0
  IFS= read -r line < "$file" || return 0
  if [[ "$line" =~ ^sha=([0-9a-fA-F]{40})([[:space:]]|$) ]]; then
    sha=$(printf '%s' "${BASH_REMATCH[1]}" | tr '[:upper:]' '[:lower:]')
    printf '%s' "$sha"
    return 0
  fi
  return 0
}

# sha_is_on_origin_master is 0 when sha is origin/master or an ancestor of it.
# Callers must already have fetched. An unknown object or a branch-only
# commit is rejected so a trigger cannot check out an arbitrary tree.
sha_is_on_origin_master() {
  local checkout="$1" sha="$2"
  [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  git_in "$checkout" cat-file -e "${sha}^{commit}" 2>/dev/null || return 1
  git_in "$checkout" merge-base --is-ancestor "$sha" origin/master
}

setup_now() {
  if [[ "${PANTRY_SETUP_TEST:-}" == 1 && -n "${PANTRY_SETUP_NOW:-}" ]]; then
    printf '%s' "$PANTRY_SETUP_NOW"
    return 0
  fi
  date -u +%Y-%m-%dT%H:%M:%SZ
}

read_state_sha() {
  local file="$1" sha
  [[ -f "$file" ]] || return 0
  sha=$(tr -d '[:space:]' < "$file")
  if [[ "$sha" =~ ^[0-9a-f]{40}$ ]]; then
    printf '%s' "$sha"
  fi
  return 0
}

read_known_good() {
  read_state_sha "${PANTRY_SETUP_STATE_DIR}/last-known-good"
}

# write_state_sha records a validated commit. The value is never taken from
# the trigger file without sha_is_on_origin_master.
write_state_sha() {
  local name="$1" sha="$2"
  case "$name" in
    last-applied|last-known-good) ;;
    *) return 1 ;;
  esac
  [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  mkdir -p "$PANTRY_SETUP_STATE_DIR"
  chmod 755 "$PANTRY_SETUP_STATE_DIR"
  printf '%s\n' "$sha" > "${PANTRY_SETUP_STATE_DIR}/${name}"
}

# write_setup_status publishes the commit that is actually in effect.
# status is applied, rolled-back, or failed. Agents read this file through
# GET /api/build; it has no paths and no secrets.
write_setup_status() {
  local commit="$1" status="$2" applied_at dir tmp
  applied_at=$(setup_now)
  case "$status" in
    applied|rolled-back|failed) ;;
    *) return 1 ;;
  esac
  if [[ -n "$commit" && ! "$commit" =~ ^[0-9a-f]{40}$ ]]; then
    return 1
  fi
  if [[ ! "$applied_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
    return 1
  fi
  dir=$(dirname "$PANTRY_SETUP_STATUS_FILE")
  mkdir -p "$dir"
  chmod 755 "$dir"
  tmp=$(mktemp "${dir}/.status.XXXXXX")
  if [[ -n "$commit" ]]; then
    printf '{"setupCommit":"%s","setupAppliedAt":"%s","setupStatus":"%s"}\n' \
      "$commit" "$applied_at" "$status" > "$tmp"
  else
    printf '{"setupAppliedAt":"%s","setupStatus":"%s"}\n' \
      "$applied_at" "$status" > "$tmp"
  fi
  chmod 644 "$tmp"
  mv "$tmp" "$PANTRY_SETUP_STATUS_FILE"
}

git_in() {
  local checkout="$1"
  shift
  git -C "$checkout" -c "safe.directory=${checkout}" "$@"
}

# ensure_setup_checkout clones the dedicated root-owned tree once.
# An existing checkout is left in place; fetch resets its remote URL.
ensure_setup_checkout() {
  if ! setup_remote_ok "$PANTRY_SETUP_REMOTE"; then
    echo "pantry setup: refusing remote; automatic setup fetches https://github.com/Rhionin/pantry.git only" >&2
    return 1
  fi
  if [[ "$PANTRY_SETUP_CHECKOUT" != /* || "$PANTRY_SETUP_CHECKOUT" == "/" || "$PANTRY_SETUP_CHECKOUT" == "$PANTRY_DIR" ]]; then
    echo "pantry setup: refusing checkout path" >&2
    return 1
  fi
  if [[ -d "${PANTRY_SETUP_CHECKOUT}/.git" ]]; then
    return 0
  fi
  if [[ -e "$PANTRY_SETUP_CHECKOUT" ]]; then
    echo "pantry setup: ${PANTRY_SETUP_CHECKOUT} exists and is not a git checkout; not replacing it" >&2
    return 1
  fi
  if ! command -v git >/dev/null 2>&1; then
    echo "pantry setup: git is not installed. Automatic setup does not install packages." >&2
    return 1
  fi
  mkdir -p "$(dirname "$PANTRY_SETUP_CHECKOUT")"
  GIT_TERMINAL_PROMPT=0 GIT_ASKPASS=true \
    git -c "safe.directory=${PANTRY_SETUP_CHECKOUT}" \
    clone --origin origin "$PANTRY_SETUP_REMOTE" "$PANTRY_SETUP_CHECKOUT"
}

# fetch_origin_master replaces origin with the configured GitHub URL, then
# fetches only master. The trigger's ref is never a git ref.
fetch_origin_master() {
  if ! setup_remote_ok "$PANTRY_SETUP_REMOTE"; then
    echo "pantry setup: refusing remote; automatic setup fetches https://github.com/Rhionin/pantry.git only" >&2
    return 1
  fi
  GIT_TERMINAL_PROMPT=0 GIT_ASKPASS=true \
    git_in "$PANTRY_SETUP_CHECKOUT" remote set-url origin "$PANTRY_SETUP_REMOTE"
  GIT_TERMINAL_PROMPT=0 GIT_ASKPASS=true \
    git_in "$PANTRY_SETUP_CHECKOUT" fetch origin '+refs/heads/master:refs/remotes/origin/master'
}

checkout_detach() {
  local sha="$1"
  [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  # --force keeps a dirty tree from stopping on a prompt.
  GIT_TERMINAL_PROMPT=0 GIT_ASKPASS=true \
    git_in "$PANTRY_SETUP_CHECKOUT" checkout --detach --force "$sha"
}

setup_script_path() {
  if [[ "${PANTRY_SETUP_TEST:-}" == 1 && -n "${PANTRY_SETUP_SCRIPT:-}" ]]; then
    printf '%s' "$PANTRY_SETUP_SCRIPT"
    return 0
  fi
  printf '%s' "${PANTRY_SETUP_CHECKOUT}/deploy/setup.sh"
}

# run_setup_apply runs the checked-out setup.sh with stdin closed.
# PANTRY_SETUP_SKIP_PACKAGES keeps apt and the Docker installer out of this path.
run_setup_apply() {
  local script
  script=$(setup_script_path)
  if [[ ! -f "$script" ]]; then
    echo "pantry setup: setup.sh is missing from the checkout" >&2
    return 1
  fi
  PANTRY_SETUP_SKIP_PACKAGES=1 \
    PANTRY_DIR="$PANTRY_DIR" \
    DEBIAN_FRONTEND=noninteractive \
    GIT_TERMINAL_PROMPT=0 \
    bash "$script" apply </dev/null
}

# rollback_setup checks out last-known-good and runs setup.sh again.
# The log lines are intentional: a failed setup has to be obvious in the journal.
rollback_setup() {
  local good now
  now=$(setup_now)
  good=$(read_known_good || true)
  if [[ -z "$good" ]]; then
    echo "pantry setup: FAILED and there is no last-known-good setup to restore" >&2
    write_setup_status "" "failed" || true
    return 0
  fi
  echo "pantry setup: FAILED setup; ROLLING BACK to last-known-good ${good}" >&2
  if ! sha_is_on_origin_master "$PANTRY_SETUP_CHECKOUT" "$good"; then
    echo "pantry setup: FAILED rollback; ${good} is no longer on origin/master" >&2
    write_setup_status "" "failed" || true
    return 0
  fi
  if ! checkout_detach "$good"; then
    echo "pantry setup: FAILED to check out last-known-good ${good}" >&2
    write_setup_status "" "failed" || true
    return 0
  fi
  if ! run_setup_apply; then
    echo "pantry setup: FAILED to re-apply last-known-good ${good}" >&2
    write_setup_status "$good" "failed" || true
    return 0
  fi
  write_state_sha last-applied "$good" || true
  write_setup_status "$good" "rolled-back" || true
  echo "pantry setup: restored last-known-good ${good} at ${now}" >&2
  return 0
}

# apply_signed_setup checks out the trigger sha when it is on origin/master
# and runs setup.sh apply. Failure re-applies last-known-good. The image
# pull still runs afterwards; this function does not stop it.
apply_signed_setup() {
  local sha
  if ! auto_setup_enabled; then
    echo "pantry setup: PANTRY_AUTO_SETUP=off; leaving setup files unchanged"
    return 0
  fi
  sha=$(trigger_setup_sha || true)
  if [[ -z "$sha" ]]; then
    echo "pantry setup: no full commit in the trigger; leaving setup files unchanged"
    return 0
  fi
  if ! ensure_setup_checkout; then
    echo "pantry setup: FAILED to prepare the checkout; not applying ${sha}" >&2
    write_setup_status "" "failed" || true
    return 0
  fi
  if ! fetch_origin_master; then
    echo "pantry setup: FAILED to fetch origin/master; not applying ${sha}" >&2
    write_setup_status "" "failed" || true
    return 0
  fi
  if ! sha_is_on_origin_master "$PANTRY_SETUP_CHECKOUT" "$sha"; then
    echo "pantry setup: REFUSING ${sha}; it is not on origin/master" >&2
    return 0
  fi
  if ! checkout_detach "$sha"; then
    echo "pantry setup: FAILED to check out ${sha}" >&2
    rollback_setup
    return 0
  fi
  echo "pantry setup: applying ${sha}"
  if run_setup_apply; then
    write_state_sha last-applied "$sha" || true
    write_state_sha last-known-good "$sha" || true
    write_setup_status "$sha" "applied" || true
    echo "pantry setup: applied ${sha}"
    return 0
  fi
  echo "pantry setup: FAILED applying ${sha}" >&2
  rollback_setup
  return 0
}
