#!/usr/bin/env bash
# Apply deploy/setup.sh from a signed deploy trigger, then return.
#
# pantry-update.sh sources this file inside its existing lock. The whole
# body is one function so bash parses it before any of it runs: setup.sh
# replaces this file with a new inode while an update is in progress.
# The trigger file is data: this script never evals it, and it never passes
# ref, a path, or a shell metacharacter to git. A sha is used only after it
# is 40 hex digits and an ancestor of origin/master fetched from GitHub.
# With no last-known-good yet, only the origin/master tip is applied. After
# that, only that commit or a descendant of it is applied. A sha that already
# failed or was already refused is skipped before another fetch. Rolling
# back to last-known-good is the only backward
# step. The request is not a command, and the pantry container is not given
# a root socket — the host path unit starts the updater that is already root.
#
# PANTRY_AUTO_SETUP=off in the deploy .env skips this and leaves the image
# pull as the only automatic step. Package installs and Docker upgrades stay
# out of this path; the one manual `sudo ./setup.sh` remains how those happen.

pantry_setup_main() {
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

  trigger_claim_path() {
    printf '%s' "${PANTRY_SETUP_STATE_DIR}/trigger-claim/request"
  }

  # stage_trigger moves the container-writable request into a root-owned
  # directory on the same filesystem. rename does not open the inode, so a
  # fifo or a symlink swapped in at the last moment is not followed and
  # cannot block the updater. Later reads use only the staged path.
  stage_trigger() {
    local src="${PANTRY_TRIGGER_FILE}" dest dir
    dir="${PANTRY_SETUP_STATE_DIR}/trigger-claim"
    dest=$(trigger_claim_path)
    [[ -n "$src" && -n "$PANTRY_SETUP_STATE_DIR" ]] || return 1
    mkdir -p "$dir"
    chmod 700 "$dir"
    if [[ -e "$src" || -L "$src" ]]; then
      rm -rf -- "$dest"
      if ! mv -f -- "$src" "$dest"; then
        return 1
      fi
    fi
    [[ -e "$dest" || -L "$dest" ]]
  }

  # trigger_staged_status prints missing, unsafe, or ok. It does not read
  # a non-regular file. The fd check is on the staged inode, which the
  # container cannot replace.
  trigger_staged_status() {
    local dest kind size fd
    if ! stage_trigger; then
      printf 'missing'
      return 0
    fi
    dest=$(trigger_claim_path)
    if [[ -L "$dest" || -p "$dest" || ! -f "$dest" ]]; then
      printf 'unsafe'
      return 0
    fi
    size=$(stat -c '%s' "$dest" 2>/dev/null || printf '%s' 999999)
    if [[ ! "$size" =~ ^[0-9]+$ ]] || (( 10#$size > 1024 )); then
      printf 'unsafe'
      return 0
    fi
    exec {fd}<"$dest" || { printf 'unsafe'; return 0; }
    kind=$(stat -L -c '%F' "/dev/fd/${fd}" 2>/dev/null || true)
    size=$(stat -L -c '%s' "/dev/fd/${fd}" 2>/dev/null || printf '%s' 999999)
    exec {fd}<&-
    if [[ "$kind" != "regular file" && "$kind" != "regular empty file" ]]; then
      printf 'unsafe'
      return 0
    fi
    if [[ ! "$size" =~ ^[0-9]+$ ]] || (( 10#$size > 1024 )); then
      printf 'unsafe'
      return 0
    fi
    printf 'ok'
  }

  # trigger_request_unsafe is true when the staged request must not be parsed.
  # Missing means there is nothing to apply.
  trigger_request_unsafe() {
    local status
    status=$(trigger_staged_status)
    if [[ "$status" == "unsafe" ]]; then
      rm -rf -- "$(trigger_claim_path)"
      return 0
    fi
    return 1
  }

  # trigger_setup_sha prints a full lowercase commit from the staged trigger.
  # A short sha, a ref, or any other text prints nothing and is not executed.
  trigger_setup_sha() {
    local dest line sha fd kind
    [[ "$(trigger_staged_status)" == "ok" ]] || return 0
    dest=$(trigger_claim_path)
    exec {fd}<"$dest" || return 0
    kind=$(stat -L -c '%F' "/dev/fd/${fd}" 2>/dev/null || true)
    if [[ "$kind" != "regular file" && "$kind" != "regular empty file" ]]; then
      exec {fd}<&-
      return 0
    fi
    IFS= read -r line <&"$fd" || true
    exec {fd}<&-
    if [[ "$line" =~ ^sha=([0-9a-fA-F]{40})([[:space:]]|$) ]]; then
      sha=$(printf '%s' "${BASH_REMATCH[1]}" | tr '[:upper:]' '[:lower:]')
      printf '%s' "$sha"
    fi
    return 0
  }

  # trigger_digest_value prints a hash of a staged regular trigger. A symlink
  # is not followed, and an oversized file or a fifo is not read.
  trigger_digest_value() {
    local dest sum
    [[ "$(trigger_staged_status)" == "ok" ]] || return 0
    dest=$(trigger_claim_path)
    sum=$(sha256sum -- "$dest" | awk '{print $1}')
    printf '%s' "$sum"
  }

  # sha_is_on_origin_master is 0 when sha is origin/master or an ancestor of it.
  # Callers must already have fetched. An unknown object or a branch-only
  # commit is rejected so a trigger cannot check out an arbitrary tree.
  # Forward-only from last-known-good is a separate check: rollback has to
  # accept the known-good commit itself, which is an ancestor of itself.
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
    [[ -f "$file" && ! -L "$file" ]] || return 0
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
  # the trigger file without the origin/master and last-known-good checks.
  write_state_sha() {
    local name="$1" sha="$2"
    case "$name" in
      last-applied|last-known-good|last-failed|last-refused) ;;
      *) return 1 ;;
    esac
    [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || return 1
    mkdir -p "$PANTRY_SETUP_STATE_DIR"
    chmod 755 "$PANTRY_SETUP_STATE_DIR"
    printf '%s\n' "$sha" > "${PANTRY_SETUP_STATE_DIR}/${name}"
  }

  # write_setup_status publishes the commit that is actually in effect.
  # status is applied, rolled-back, failed, or refused. Agents read this
  # file through GET /api/build; it has no paths and no secrets.
  write_setup_status() {
    local commit="$1" status="$2" applied_at dir tmp
    applied_at=$(setup_now)
    case "$status" in
      applied|rolled-back|failed|refused) ;;
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
  # fetches only master. set-url and fetch are one chain so a failed set-url
  # cannot fetch whatever remote was stored before. The trigger's ref is never
  # a git ref.
  fetch_origin_master() {
    if ! setup_remote_ok "$PANTRY_SETUP_REMOTE"; then
      echo "pantry setup: refusing remote; automatic setup fetches https://github.com/Rhionin/pantry.git only" >&2
      return 1
    fi
    GIT_TERMINAL_PROMPT=0 GIT_ASKPASS=true \
      git_in "$PANTRY_SETUP_CHECKOUT" remote set-url origin "$PANTRY_SETUP_REMOTE" &&
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
    PANTRY_SETUP_SOURCE_ONLY='' \
      PANTRY_SETUP_SKIP_PACKAGES=1 \
      PANTRY_DIR="$PANTRY_DIR" \
      DEBIAN_FRONTEND=noninteractive \
      GIT_TERMINAL_PROMPT=0 \
      bash "$script" apply </dev/null
  }

  # setup_listen_port is HOST_PORT from the deploy .env when it is a TCP port.
  # Anything else stays on 8080 so a trigger cannot aim the health check elsewhere.
  setup_listen_port() {
    local port="8080" line file="${PANTRY_DIR}/.env"
    if [[ -f "$file" ]]; then
      line=$(grep '^HOST_PORT=' "$file" | head -1 || true)
      line=${line#HOST_PORT=}
      line=${line//[[:space:]]/}
      line=${line//$'\r'/}
      if [[ "$line" =~ ^[0-9]+$ ]] && (( 10#$line >= 1 && 10#$line <= 65535 )); then
        port=$((10#$line))
      fi
    fi
    printf '%s' "$port"
  }

  # setup_health_ok waits briefly for local /health. That is the pantry
  # process on this host. It does not check the tunnel, Caddy, or the firewall.
  # last-known-good is recorded only after this passes, so a commit that does
  # not boot is not what the next failure restores.
  setup_health_ok() {
    local port deadline wait body
    port=$(setup_listen_port)
    wait="${PANTRY_SETUP_HEALTH_WAIT:-20}"
    [[ "$wait" =~ ^[0-9]+$ ]] || wait=20
    deadline=$((SECONDS + wait))
    while true; do
      body=$(curl -sf --max-time 2 "http://127.0.0.1:${port}/health" 2>/dev/null || true)
      if printf '%s' "$body" | grep -q '"status":"ok"'; then
        return 0
      fi
      if (( SECONDS >= deadline )); then
        echo "pantry setup: health check failed at http://127.0.0.1:${port}/health" >&2
        return 1
      fi
      sleep 1
    done
  }

  mark_in_progress() {
    local sha="$1"
    [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || return 1
    mkdir -p "$PANTRY_SETUP_STATE_DIR"
    chmod 755 "$PANTRY_SETUP_STATE_DIR"
    printf '%s\n' "$sha" > "${PANTRY_SETUP_STATE_DIR}/in-progress"
  }

  clear_in_progress() {
    rm -f "${PANTRY_SETUP_STATE_DIR}/in-progress"
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
    if ! setup_health_ok; then
      echo "pantry setup: FAILED health check after restoring ${good}" >&2
      write_setup_status "$good" "failed" || true
      return 0
    fi
    write_state_sha last-applied "$good" || true
    write_setup_status "$good" "rolled-back" || true
    echo "pantry setup: restored last-known-good ${good} at ${now}" >&2
    return 0
  }

  # recover_interrupted_setup runs when a previous apply was killed (the
  # systemd timeout sends SIGTERM, then SIGKILL). The marker is removed only
  # after this attempt finishes, so a kill during the rollback is tried again.
  recover_interrupted_setup() {
    local marker="${PANTRY_SETUP_STATE_DIR}/in-progress" prev applied lkg
    if [[ -L "$marker" ]]; then
      rm -f "$marker"
      return 0
    fi
    [[ -f "$marker" ]] || return 0
    prev=$(read_state_sha "$marker" || true)
    applied=$(read_state_sha "${PANTRY_SETUP_STATE_DIR}/last-applied" || true)
    lkg=$(read_known_good || true)
    if [[ -n "$prev" && "$prev" == "$applied" && "$prev" == "$lkg" ]]; then
      echo "pantry setup: previous apply of ${prev} finished; clearing the in-progress marker"
      rm -f "$marker"
      return 0
    fi
    echo "pantry setup: previous apply was interrupted; recording failed and rolling back" >&2
    if [[ -n "$prev" ]]; then
      write_setup_status "$prev" "failed" || true
    else
      write_setup_status "" "failed" || true
    fi
    if [[ -d "${PANTRY_SETUP_CHECKOUT}/.git" ]]; then
      rollback_setup
    fi
    rm -f "$marker"
  }

  # apply_signed_setup checks out the trigger sha when it is a forward commit
  # on origin/master and runs setup.sh apply. The same sha, and any ancestor
  # of last-known-good, returns without running setup again. Failure re-applies
  # last-known-good. The image pull still runs afterwards; this function does
  # not stop it.
  apply_signed_setup() {
    local sha applied lkg
    if ! auto_setup_enabled; then
      echo "pantry setup: PANTRY_AUTO_SETUP=off; leaving setup files unchanged"
      return 0
    fi
    recover_interrupted_setup
    if trigger_request_unsafe; then
      echo "pantry setup: REFUSING trigger; request must be a regular file of at most 1024 bytes" >&2
      return 0
    fi
    sha=$(trigger_setup_sha || true)
    if [[ -z "$sha" ]]; then
      echo "pantry setup: no full commit in the trigger; leaving setup files unchanged"
      return 0
    fi
    applied=$(read_state_sha "${PANTRY_SETUP_STATE_DIR}/last-applied" || true)
    if [[ -n "$applied" && "$sha" == "$applied" ]]; then
      echo "pantry setup: ${sha} already applied; not running setup again"
      return 0
    fi
    # A failed or refused sha stays in the trigger file. Skip it before
    # fetch so the minute timer does not clone, fetch, or roll back again.
    applied=$(read_state_sha "${PANTRY_SETUP_STATE_DIR}/last-failed" || true)
    if [[ -n "$applied" && "$sha" == "$applied" ]]; then
      echo "pantry setup: ${sha} already failed; not running setup again"
      return 0
    fi
    applied=$(read_state_sha "${PANTRY_SETUP_STATE_DIR}/last-refused" || true)
    if [[ -n "$applied" && "$sha" == "$applied" ]]; then
      echo "pantry setup: ${sha} already refused; not running setup again"
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
      write_setup_status "$sha" "refused" || true
      write_state_sha last-refused "$sha" || true
      return 0
    fi
    lkg=$(read_known_good || true)
    if [[ -z "$lkg" ]]; then
      # No baseline yet: an ancestor of master would let the container roll
      # the host back to the first commit. Only the tip is a forward step.
      applied=$(git_in "$PANTRY_SETUP_CHECKOUT" rev-parse origin/master 2>/dev/null || true)
      if [[ "$sha" != "$applied" ]]; then
        echo "pantry setup: REFUSING ${sha}; it is not the origin/master tip" >&2
        write_setup_status "$sha" "refused" || true
        write_state_sha last-refused "$sha" || true
        return 0
      fi
    elif git_in "$PANTRY_SETUP_CHECKOUT" merge-base --is-ancestor "$sha" "$lkg"; then
      if [[ "$sha" == "$lkg" ]]; then
        echo "pantry setup: ${sha} is last-known-good; not running setup again"
        return 0
      fi
      echo "pantry setup: REFUSING ${sha}; it is an ancestor of last-known-good" >&2
      write_setup_status "$sha" "refused" || true
      write_state_sha last-refused "$sha" || true
      return 0
    elif ! git_in "$PANTRY_SETUP_CHECKOUT" merge-base --is-ancestor "$lkg" "$sha"; then
      echo "pantry setup: REFUSING ${sha}; it is not a descendant of last-known-good" >&2
      write_setup_status "$sha" "refused" || true
      write_state_sha last-refused "$sha" || true
      return 0
    fi
    echo "pantry setup: applying ${sha}"
    mark_in_progress "$sha" || true
    if ! checkout_detach "$sha"; then
      echo "pantry setup: FAILED to check out ${sha}" >&2
      rollback_setup
      clear_in_progress
      write_state_sha last-failed "$sha" || true
      return 0
    fi
    if ! run_setup_apply; then
      echo "pantry setup: FAILED applying ${sha}" >&2
      rollback_setup
      clear_in_progress
      write_state_sha last-failed "$sha" || true
      return 0
    fi
    if ! setup_health_ok; then
      echo "pantry setup: FAILED health check after ${sha}" >&2
      rollback_setup
      clear_in_progress
      write_state_sha last-failed "$sha" || true
      return 0
    fi
    write_state_sha last-applied "$sha" || true
    write_state_sha last-known-good "$sha" || true
    write_setup_status "$sha" "applied" || true
    clear_in_progress
    echo "pantry setup: applied ${sha}"
    return 0
  }
}

# Not named main: pantry-update.sh defines main and sources this file.
pantry_setup_main "$@"
# Sourced by pantry-update.sh. exit only when this file is the process.
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  exit
fi
