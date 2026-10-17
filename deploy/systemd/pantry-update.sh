#!/usr/bin/env bash
# Pull and recreate the Stockbot stack.
#
# When PUBLIC_HOST is set and auth.caddy exists, the public HTTPS proxy
# (compose profile "public") is part of the same update. A plain
# `docker compose up` omits profiled services, which would leave certificate
# renewal and proxy config out of automatic updates. A LAN-only install
# leaves PUBLIC_HOST empty, so Caddy is never started. Without auth.caddy,
# starting the profile would publish the hostname with no password file.
# The hash is copied into auth/household before containers are recreated,
# and an existing PANTRY_SESSION_SECRET is kept.
#
# A non-empty CLOUDFLARE_TUNNEL_TOKEN selects the tunnel profile instead.
# That profile does not publish ports 80 or 443. The certificate profile
# is stopped so a token cannot leave both paths running.
#
# The timer and the deploy-hook path unit both start this script. A lock
# keeps those from pulling at the same time. If a hook lands while a pull
# is already running, the running copy sees the new trigger file and pulls
# again instead of dropping it.
#
# A pull that does not change the image id of a running service stops there.
# The timer fires every minute, and recreating a healthy stack each time
# would drop the scanner session for nothing.
#
# When an image does change, the pantry revision label is read before
# anything is recreated. An image without org.opencontainers.image.revision
# is not started. The new containers are kept only if GET /health is ok and
# GET /api/build's commit equals that label. While /health reports
# "migrating", the wait continues up to PANTRY_UPDATE_MIGRATION_TIMEOUT
# (default 10 minutes). The short timeout (PANTRY_UPDATE_HEALTH_TIMEOUT,
# default 90 seconds) uses /proc/uptime, because the Pi has no RTC. Both
# URLs are read on 127.0.0.1:$HOST_PORT. /health on the public hostname asks
# for the household password, so that URL is not the LAN check. When the
# public proxy is part of this update, /api/build is also requested through
# Caddy.
#
# .update-pending is written before containers are recreated. If this process
# stops after that and before a verdict, the next run checks health again
# against the revision label of the digest named in that marker, then checks
# every container. It rolls back if either check fails. A newer image pulled
# in the meantime is not the commit that check expects. If the pending digest
# is no longer on this machine, the resume check is skipped with a warning.
#
# Before containers are recreated, each service's status and RestartCount
# are recorded. Afterwards only a service whose image changed can fail the
# deploy. `docker compose up` failing is not itself a rollback: if every
# changed service is healthy, the new images stay. An unchanged grocy-sync
# or cloudflared that is down logs a journal warning and does not roll
# back. The service is listed in .update-sidecar-warn. The next run checks
# that list even when the images did not change, warns again while it is
# down, and removes it once it is running. Status and RestartCount are
# read again after PANTRY_UPDATE_SETTLE_SECONDS (default 12, never more
# than 15). A restart in that window fails the deploy only for a changed
# image. An unchanged cloudflared that restarts on its own does not.
#
# On timeout, a compose failure, or a commit mismatch, each service that
# was running is re-pinned to its previous digest and started with --pull
# never. That rollback is health-checked. If the previous image starts but
# does not become healthy, the log says BOTH IMAGES DOWN. If no previous
# digest was recorded, the new image is left in place and that banner is
# not printed. The local tag compose uses
# (ghcr.io/rhionin/pantry:$PANTRY_IMAGE_TAG, and the same for any other
# service whose digest changed) is moved back to the previous digest. A
# later manual compose up or setup.sh would otherwise start the
# rejected image with no health check. .env is not rewritten.
# PANTRY_IMAGE_TAG and the setup last-known-good sha (#152) stay whatever
# they already were. A failure pinned on one changed service rejects that
# digest. A pantry digest that already passed /health and /api/build is
# not rejected when a sidecar is the one that failed. Every changed digest
# is rejected only when pantry itself is unverified, or when no failing
# changed service can be named. An unchanged service that is merely down
# is not that named failure.
# Lines are "service<TAB>digest" in .update-rejected-digest. The next run
# retags each rejected service back to its previous digest. The first skip
# of a digest is a journal warning; later minutes log one skip line and do
# not warn again. A warning mark is dropped when that digest is no longer
# rejected. Other services are still deployed. Comparison happens
# after pull, so the restored local tag is not treated as the remote image.
#
# Tunnel mode checks caddy-tunnel before recreate, then again after.
# Both requests are `curl --resolve $PUBLIC_HOST:80:10.77.77.3
# http://$PUBLIC_HOST/api/build`. PUBLIC_HOST is the hostname compose
# passes in, and the only site Caddyfile.tunnel serves. The address
# 10.77.77.3 is caddy-tunnel on the cf-tunnel network. The second check
# must return the new pantry commit. The probe is skipped only when the
# pre-deploy request was also unreachable (curl 6, 7, 28, or 52). A refusal
# or a hang after a reachable old stack fails the deploy. Curl 56 on the
# old stack is not "unreachable": a healthy new listener still passes.
# Certificate mode makes the same before-and-after request over HTTPS.
# An unchanged proxy that passed before and fails after, with a new pantry
# image, is a pantry failure: roll back and reject pantry. An unchanged
# proxy that was already failing only warns. A changed proxy that fails
# rejects that proxy, not a pantry image that already passed /health.
# The service name is caddy-tunnel in tunnel mode and caddy otherwise.
# Cloudflare still uses http://caddy:80 inside that network.
#
# Re-pinning the image does not undo a migration. The migration runner is
# not transactional (#249). This script does not copy pantry-pre-*.db back
# over the database. Logs from the rejected pantry container are saved in
# the pantry directory before the rollback removes that container. The
# banner names a pantry-pre-*.db path only when those logs say this deploy
# wrote the snapshot. An "already exists" line is not a new snapshot.
#
# Service images come from `docker compose config`. python3 is not required.

set -euo pipefail

UPDATE_STATE=""
# PANTRY_VERIFIED is 1 after the new pantry image has passed /health and
# /api/build. FAILED_SERVICES lists services that failed after that, one
# name per line. A verified pantry digest is rejected only if it is in
# that list.
PANTRY_VERIFIED=0
FAILED_SERVICES=""
# TUNNEL_REACHABLE is 1 when the pre-deploy probe was not a connect failure.
# PROXY_REACHABLE is 1 when that probe actually succeeded (curl 0). An
# unchanged proxy that passed and then fails is charged to pantry.
TUNNEL_REACHABLE=0
PROXY_REACHABLE=0
TUNNEL_PROBE_RC=0
cleanup_update_state() {
  if [[ -n "${UPDATE_STATE:-}" ]]; then
    rm -rf "$UPDATE_STATE"
  fi
}
trap cleanup_update_state EXIT

PANTRY_DIR=${PANTRY_DIR:-/opt/pantry}
cd "$PANTRY_DIR"
# shellcheck disable=SC1091
source "$PANTRY_DIR/publish-mode.sh"

compose=(docker compose --project-directory "$PANTRY_DIR" -f "$PANTRY_DIR/docker-compose.yml")
mode_rc=0
mode=$(publish_mode_from_env) || mode_rc=$?
if [[ "$mode_rc" -eq 2 ]]; then
  echo "Tunnel mode needs CLOUDFLARE_TUNNEL_TOKEN in /opt/pantry/.env. The public proxy was left unchanged. Run: sudo ./setup.sh publish --tunnel" >&2
  exit 1
elif [[ "$mode_rc" -ne 0 ]]; then
  echo "PUBLISH_MODE must be acme or tunnel. The public proxy was left unchanged." >&2
  exit 1
fi

if [[ "$mode" == tunnel ]]; then
  if [[ ! -f "$PANTRY_DIR/auth.caddy" ]]; then
    echo "CLOUDFLARE_TUNNEL_TOKEN is set but ${PANTRY_DIR}/auth.caddy is missing; leaving the tunnel stopped. Run: sudo ./setup.sh publish --tunnel" >&2
  else
    compose+=(-f "$PANTRY_DIR/docker-compose.tunnel.yml" --profile tunnel)
  fi
elif [[ "$mode" == acme ]]; then
  # -f /opt/pantry/auth.caddy is the production gate. A fixture sets
  # PANTRY_DIR somewhere else and is checked only then.
  if [[ -f /opt/pantry/auth.caddy || ( "$PANTRY_DIR" != /opt/pantry && -f "$PANTRY_DIR/auth.caddy" ) ]]; then
    compose+=(--profile public)
  else
    echo "PUBLIC_HOST is set but ${PANTRY_DIR}/auth.caddy is missing; leaving the public proxy stopped. Run: sudo ./setup.sh" >&2
  fi
fi
while IFS= read -r arg; do
  [[ -n "$arg" ]] && compose+=("$arg")
done < <(grocy_compose_suffix)

# uid 65532 is the distroless nonroot user that writes the trigger file.
mkdir -p "$PANTRY_DIR/deploy-trigger"
if ! chown 65532:65532 "$PANTRY_DIR/deploy-trigger"; then
  # The no-sudo test seam. A real timer runs this file as root and can chown.
  if [[ "${PANTRY_SETUP_SKIP_ROOT:-}" == 1 ]]; then
    echo "[warn] Could not give ${PANTRY_DIR}/deploy-trigger to uid 65532" >&2
  else
    echo "Could not give ${PANTRY_DIR}/deploy-trigger to uid 65532" >&2
    exit 1
  fi
fi
chmod 755 "$PANTRY_DIR/deploy-trigger"

exec 9>"$PANTRY_DIR/deploy-trigger/.lock"
if ! flock -n 9; then
  echo "pantry update: another update is running; waiting so this trigger is not dropped"
  flock 9
fi
echo "pantry update: lock acquired"

# shellcheck disable=SC1091
source "$PANTRY_DIR/auth-migrate.sh"
if ! sync_household_credential; then
  echo "Could not copy the household password hash; leaving containers unchanged." >&2
  exit 1
fi
if [[ -f "$PANTRY_DIR/auth.caddy" ]]; then
  if ! ensure_session_secret; then
    echo "Could not save the session secret; leaving containers unchanged." >&2
    exit 1
  fi
  if [[ ! -s "$PANTRY_DIR/auth/household" ]]; then
    echo "auth.caddy is present but the login hash was not copied; leaving containers unchanged." >&2
    exit 1
  fi
fi
# shellcheck disable=SC1091
source "$PANTRY_DIR/vapid-env.sh"
if ! ensure_vapid_keys; then
  echo "Could not generate VAPID keys; scan alerts stay on an open tab until sudo ./setup.sh succeeds." >&2
fi

trigger_digest() {
  if [[ -f "$PANTRY_DIR/deploy-trigger/request" ]]; then
    sha256sum "$PANTRY_DIR/deploy-trigger/request" | awk '{print $1}'
  fi
}

# compose_service_images prints "service<TAB>image" for the compose command
# this update will actually run (profiles included). `docker compose config`
# is YAML; python3 is not required.
compose_service_images() {
  local config svc image
  config=$("${compose[@]}" config) || return 1
  while IFS=$'\t' read -r svc image; do
    [[ -z "$svc" ]] && continue
    image=${image#\"}
    image=${image%\"}
    image=${image#\'}
    image=${image%\'}
    printf '%s\t%s\n' "$svc" "$image"
  done < <(printf '%s\n' "$config" | awk '
    /^services:/ { in_s = 1; next }
    in_s && /^[^ #]/ { in_s = 0 }
    in_s && /^  [^ ].*:[[:space:]]*$/ {
      svc = $1
      sub(/:$/, "", svc)
      next
    }
    in_s && /^    image:[[:space:]]/ {
      img = $0
      sub(/^    image:[[:space:]]+/, "", img)
      if (svc != "") print svc "\t" img
    }
  ')
}

image_repo_name() {
  local ref="$1" tail
  if [[ "$ref" == *@sha256:* ]]; then
    printf '%s' "${ref%%@*}"
    return 0
  fi
  tail=${ref##*/}
  if [[ "$tail" == *:* ]]; then
    printf '%s' "${ref%:*}"
    return 0
  fi
  printf '%s' "$ref"
}

# image_repo_digest IMAGE_ID REF prints the registry digest of IMAGE_ID,
# preferring one for REF's repository. Empty when the image has none.
image_repo_digest() {
  local image_id="$1" ref="$2" repo digests digest match
  [[ -n "$image_id" ]] || return 0
  repo=$(image_repo_name "$ref")
  digests=$(docker image inspect -f '{{range .RepoDigests}}{{println .}}{{end}}' "$image_id" 2>/dev/null || true)
  match=""
  while IFS= read -r digest; do
    digest=${digest//$'\r'/}
    [[ -z "$digest" || "$digest" == "<no value>" ]] && continue
    if [[ "$digest" == "$repo@"* ]]; then
      match=$digest
      break
    fi
    if [[ -z "$match" ]]; then
      match=$digest
    fi
  done <<< "$digests"
  printf '%s' "$match"
}

one_line() {
  local value="$1"
  value=${value//$'\r'/}
  value=${value//$'\n'/}
  printf '%s' "$value"
}

# running_image_id SERVICE prints the image id of the running container,
# or of a stopped one when nothing is running, so a later failure can
# re-pin the image that was here before the pull.
running_image_id() {
  local svc="$1" cids cid line fallback
  cids=$("${compose[@]}" ps -aq "$svc" 2>/dev/null || true)
  fallback=""
  while IFS= read -r cid; do
    cid=${cid//$'\r'/}
    [[ -z "$cid" ]] && continue
    line=$(docker inspect -f '{{.State.Running}} {{.Image}}' "$cid" 2>/dev/null || true)
    line=$(one_line "$line")
    if [[ "$line" == true\ * ]]; then
      printf '%s' "${line#true }"
      return 0
    fi
    if [[ -z "$fallback" && "$line" == false\ * ]]; then
      fallback=${line#false }
    fi
  done <<< "$cids"
  printf '%s' "$fallback"
}

record_running_images() {
  local state="$1" svc image running_id digest
  compose_service_images > "$state/refs.tsv" || return 1
  if [[ ! -s "$state/refs.tsv" ]]; then
    echo "pantry update: compose returned no services" >&2
    return 1
  fi
  : > "$state/running.tsv"
  while IFS=$'\t' read -r svc image; do
    [[ -z "$svc" ]] && continue
    running_id=$(running_image_id "$svc")
    digest=""
    if [[ -n "$running_id" ]]; then
      digest=$(image_repo_digest "$running_id" "$image")
    fi
    printf '%s\t%s\t%s\n' "$svc" "$running_id" "$digest" >> "$state/running.tsv"
    if [[ -n "$digest" ]]; then
      echo "pantry update: running ${svc} image ${digest}"
    elif [[ -n "$running_id" ]]; then
      echo "pantry update: running ${svc} image ${running_id}"
    else
      echo "pantry update: ${svc} is not running"
    fi
  done < "$state/refs.tsv"
}

images_unchanged() {
  local state="$1" svc running_id digest ref pulled_id any=0
  while IFS=$'\t' read -r svc running_id digest; do
    [[ -z "$svc" ]] && continue
    any=1
    [[ -n "$running_id" ]] || return 1
    ref=$(awk -F '\t' -v s="$svc" '$1 == s { print $2; exit }' "$state/refs.tsv")
    [[ -n "$ref" ]] || return 1
    pulled_id=$(docker image inspect -f '{{.Id}}' "$ref" 2>/dev/null || true)
    pulled_id=$(one_line "$pulled_id")
    [[ -n "$pulled_id" && "$pulled_id" == "$running_id" ]] || return 1
  done < "$state/running.tsv"
  [[ "$any" -eq 1 ]]
}

service_ref() {
  awk -F '\t' -v s="$1" '$1 == s { print $2; exit }' "$2/refs.tsv"
}

running_field() {
  awk -F '\t' -v s="$1" -v col="$2" '$1 == s { print $col; exit }' "$3/running.tsv"
}

service_pulled_id() {
  local ref id
  ref=$(service_ref "$1" "$2")
  [[ -n "$ref" ]] || return 1
  id=$(docker image inspect -f '{{.Id}}' "$ref" 2>/dev/null || true)
  id=$(one_line "$id")
  [[ -n "$id" ]] || return 1
  printf '%s' "$id"
}

service_changed() {
  local running_id pulled
  running_id=$(running_field "$2" 2 "$1")
  pulled=$(service_pulled_id "$2" "$1" || true)
  [[ -n "$pulled" && "$pulled" != "$running_id" ]]
}

service_desired_digest() {
  local ref digest id
  ref=$(service_ref "$2" "$1")
  [[ -n "$ref" ]] || return 0
  id=$(service_pulled_id "$2" "$1" || true)
  if [[ -n "$id" ]]; then
    digest=$(image_repo_digest "$id" "$ref")
  fi
  if [[ -z "${digest:-}" ]]; then
    digest=$id
  fi
  printf '%s' "$digest"
}

image_revision() {
  local revision
  revision=$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$1" 2>/dev/null || true)
  revision=$(one_line "$revision")
  if [[ -z "$revision" || "$revision" == "<no value>" ]]; then
    return 1
  fi
  printf '%s' "$revision"
}

# digest_was_rejected SERVICE DIGEST is true when that pair was recorded.
# A legacy line with only the digest still matches.
digest_was_rejected() {
  local svc="$1" digest="$2" file="$PANTRY_DIR/.update-rejected-digest" line got_svc got_digest
  [[ -n "$digest" && -f "$file" ]] || return 1
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    if [[ "$line" != *$'\t'* ]]; then
      [[ "$line" == "$digest" ]] && return 0
      continue
    fi
    got_svc=${line%%$'\t'*}
    got_digest=${line#*$'\t'}
    [[ "$got_svc" == "$svc" && "$got_digest" == "$digest" ]] && return 0
  done < "$file"
  return 1
}

remember_rejected_digest() {
  local svc="$1" digest="$2" file="$PANTRY_DIR/.update-rejected-digest"
  if [[ -z "$svc" || -z "$digest" ]]; then
    return 0
  fi
  if digest_was_rejected "$svc" "$digest"; then
    return 0
  fi
  if [[ ! -f "$file" ]]; then
    if ! printf '%s\n' '# Image digests rolled back by pantry-update.sh, one service per line. Not a setup last-known-good sha.' > "$file"; then
      echo "PANTRY UPDATE FAILED: could not write ${file}; rejected ${svc} digest ${digest} was not recorded" >&2
      return 1
    fi
  fi
  if ! printf '%s\t%s\n' "$svc" "$digest" >> "$file"; then
    echo "PANTRY UPDATE FAILED: could not record rejected ${svc} digest ${digest} in ${file}" >&2
    return 1
  fi
  echo "pantry update: recorded rejected ${svc} digest ${digest}" >&2
}

note_failed() {
  local svc="$1" line
  [[ -n "$svc" ]] || return 0
  while IFS= read -r line; do
    [[ "$line" == "$svc" ]] && return 0
  done <<< "$FAILED_SERVICES"
  FAILED_SERVICES+="${svc}"$'\n'
}

# changed_failure_named is true when a changed service was recorded as down
# or failing. An unchanged service is not recorded there.
changed_failure_named() {
  local line
  while IFS= read -r line; do
    [[ -n "$line" ]] && return 0
  done <<< "$FAILED_SERVICES"
  return 1
}

# should_reject_service is true for every changed service when pantry did
# not pass health and build. After it has, only services that actually
# failed are rejected.
should_reject_service() {
  local svc="$1" line
  if [[ "$PANTRY_VERIFIED" -eq 0 ]]; then
    return 0
  fi
  while IFS= read -r line; do
    [[ "$line" == "$svc" ]] && return 0
  done <<< "$FAILED_SERVICES"
  return 1
}

# remember_changed_digest records one changed service. Returns 0 when the
# write worked, and 2 when this service was not selected.
remember_changed_digest() {
  local state="$1" svc="$2" desired
  service_changed "$state" "$svc" || return 2
  if ! should_reject_service "$svc"; then
    return 2
  fi
  desired=$(service_desired_digest "$state" "$svc")
  remember_rejected_digest "$svc" "$desired"
}

record_changed_rejections() {
  local state="$1" svc running_id previous rc recorded=0 failed=0
  while IFS=$'\t' read -r svc running_id previous; do
    [[ -z "$svc" ]] && continue
    rc=0
    remember_changed_digest "$state" "$svc" || rc=$?
    if [[ "$rc" -eq 0 ]]; then
      recorded=1
    elif [[ "$rc" -ne 2 ]]; then
      failed=1
    fi
  done < "$state/running.tsv"
  if [[ "$recorded" -eq 0 ]]; then
    echo "<4>pantry update: warning: could not pin the failure on one service; rejecting every changed digest" >&2
    PANTRY_VERIFIED=0
    FAILED_SERVICES=""
    while IFS=$'\t' read -r svc running_id previous; do
      [[ -z "$svc" ]] && continue
      rc=0
      remember_changed_digest "$state" "$svc" || rc=$?
      if [[ "$rc" -eq 0 ]]; then
        recorded=1
      elif [[ "$rc" -ne 2 ]]; then
        failed=1
      fi
    done < "$state/running.tsv"
  fi
  if [[ "$recorded" -eq 0 ]]; then
    echo "PANTRY UPDATE FAILED: no changed digest was recorded, so the next run may try this release again" >&2
    return 1
  fi
  [[ "$failed" -eq 0 ]]
}

# prune_skip_warnings drops warning marks whose digest is no longer rejected,
# so deleting a line in .update-rejected-digest warns again the next time
# that digest is skipped.
prune_skip_warnings() {
  local mark="$PANTRY_DIR/.update-skip-warned" tmp svc digest
  [[ -f "$mark" ]] || return 0
  tmp=$(mktemp) || return 0
  while IFS=$'\t' read -r svc digest || [[ -n "${svc:-}" ]]; do
    [[ -z "${svc:-}" || -z "${digest:-}" ]] && continue
    if digest_was_rejected "$svc" "$digest"; then
      printf '%s\t%s\n' "$svc" "$digest"
    fi
  done < "$mark" > "$tmp"
  mv "$tmp" "$mark"
}

# note_skipped_digest warns the first time a digest is skipped. Later
# minutes keep the one-line skip. <4> is a syslog warning for journald.
note_skipped_digest() {
  local svc="$1" digest="$2" mark="$PANTRY_DIR/.update-skip-warned" line
  if [[ -f "$mark" ]]; then
    while IFS= read -r line; do
      if [[ "$line" == "${svc}"$'\t'"${digest}" ]]; then
        echo "pantry update: skipping rejected ${digest}" >&2
        return 0
      fi
    done < "$mark"
  fi
  echo "<4>pantry update: warning: skipping rejected ${digest}; delete that line in .update-rejected-digest to retry it" >&2
  if ! printf '%s\t%s\n' "$svc" "$digest" >> "$mark"; then
    echo "pantry update: could not record that ${digest} was already warned" >&2
  fi
}

# park_rejected_services points each rejected digest's local tag back at
# the previous image. Services that are not rejected stay on the pulled
# digest so a healthy pantry image can still be deployed.
park_rejected_services() {
  local state="$1" svc running_id previous desired ref pin
  prune_skip_warnings
  while IFS=$'\t' read -r svc running_id previous; do
    [[ -z "$svc" ]] && continue
    service_changed "$state" "$svc" || continue
    desired=$(service_desired_digest "$state" "$svc")
    if ! digest_was_rejected "$svc" "$desired"; then
      continue
    fi
    note_skipped_digest "$svc" "$desired"
    ref=$(service_ref "$svc" "$state")
    [[ -z "$ref" || "$ref" == *@sha256:* ]] && continue
    pin=$previous
    [[ -z "$pin" ]] && pin=$running_id
    [[ -z "$pin" ]] && continue
    if ! docker tag "$pin" "$ref"; then
      echo "pantry update: could not retag ${ref} to ${pin}" >&2
      return 1
    fi
    echo "pantry update: retagged ${ref} to ${pin}" >&2
  done < "$state/running.tsv"
}

log_rollback_banner() {
  cat >&2 <<'EOF'
================================================================
PANTRY UPDATE FAILED: rolling back to the previous image.
This rollback only restores container images. It does not undo a database migration.
The migration runner is not transactional: a migration that already ran stays applied, and starting the previous image cannot undo it.
Automatic pre-migration snapshots, when one was taken, are files named pantry-pre-*.db in /data (the pantry-data volume).
Restoring one of those over /data/pantry.db is a manual step. This script does not restore the database.
================================================================
EOF
}

json_string_field() {
  local body="$1" field="$2" pattern
  pattern='"'"${field}"'"[[:space:]]*:[[:space:]]*"([^"]*)"'
  if [[ "$body" =~ $pattern ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
  fi
}

health_is_ok() {
  [[ "$1" =~ \"status\"[[:space:]]*:[[:space:]]*\"ok\" ]]
}

health_is_migrating() {
  [[ "$1" =~ \"status\"[[:space:]]*:[[:space:]]*\"migrating\" ]]
}

# mono_seconds is seconds since boot. SECONDS follows the wall clock, and a
# Pi without an RTC can step that clock when NTP starts.
mono_seconds() {
  if [[ -r /proc/uptime ]]; then
    awk '{printf "%d\n", $1}' /proc/uptime
    return 0
  fi
  printf '%s\n' "${SECONDS:-0}"
}

container_state() {
  local cid line
  cid=$("${compose[@]}" ps -aq "$1" 2>/dev/null | head -n 1 || true)
  cid=$(one_line "$cid")
  [[ -n "$cid" ]] || return 1
  line=$(docker inspect -f '{{.State.Status}} {{.RestartCount}}' "$cid" 2>/dev/null || true)
  line=$(one_line "$line")
  [[ -n "$line" ]] || return 1
  printf '%s' "$line"
}

# pantry_container_failed BASELINE is true when pantry has exited, is
# restarting, or its RestartCount has risen.
pantry_container_failed() {
  local baseline="$1" line status restarts
  line=$(container_state pantry || true)
  if [[ -z "$line" ]]; then
    echo "pantry update: pantry container is missing" >&2
    return 0
  fi
  status=${line%% *}
  restarts=${line##* }
  if [[ "$status" == exited || "$status" == restarting || "$status" == dead ]]; then
    echo "pantry update: pantry container is ${status}" >&2
    return 0
  fi
  if [[ "$restarts" =~ ^[0-9]+$ && "$baseline" =~ ^[0-9]+$ && "$restarts" -gt "$baseline" ]]; then
    echo "pantry update: pantry container restarted (${baseline} -> ${restarts})" >&2
    return 0
  fi
  return 1
}

# wait_for_expected_build STATE [REVISION]
# A second argument, even an empty one, is the commit to expect. Empty means
# /health ok is enough (the previous image had no revision label). With one
# argument the commit is the pantry image's current label.
wait_for_expected_build() {
  local state="$1" supplied=0 revision="" ref host_port timeout interval migr_cap
  local start now short_deadline hard_deadline health build commit baseline line
  if [[ "$#" -ge 2 ]]; then
    supplied=1
    revision=$2
  fi
  ref=$(service_ref pantry "$state")
  if [[ -z "$ref" ]]; then
    echo "pantry update: compose project has no pantry service" >&2
    return 1
  fi
  if [[ "$supplied" -eq 0 ]]; then
    revision=$(image_revision "$ref" || true)
    if [[ -z "$revision" ]]; then
      echo "pantry update: ${ref} has no org.opencontainers.image.revision label" >&2
      return 1
    fi
  fi
  host_port=$(publish_env_value HOST_PORT)
  host_port=${host_port:-8080}
  timeout=${PANTRY_UPDATE_HEALTH_TIMEOUT:-90}
  interval=${PANTRY_UPDATE_HEALTH_INTERVAL:-2}
  migr_cap=${PANTRY_UPDATE_MIGRATION_TIMEOUT:-600}
  if [[ -n "$revision" ]]; then
    echo "pantry update: waiting for /health and /api/build commit ${revision} on 127.0.0.1:${host_port}"
  else
    echo "pantry update: waiting for /health on 127.0.0.1:${host_port}"
  fi
  start=$(mono_seconds)
  short_deadline=$((start + timeout))
  hard_deadline=$((start + migr_cap))
  line=$(container_state pantry || true)
  baseline=${line##* }
  [[ "$baseline" =~ ^[0-9]+$ ]] || baseline=0
  health=""
  build=""
  while true; do
    if pantry_container_failed "$baseline"; then
      return 1
    fi
    # curl -f hides the body of a 503, which is how /health reports migrating.
    health=$(curl -sS --max-time 5 "http://127.0.0.1:${host_port}/health" 2>/dev/null || true)
    if health_is_migrating "$health"; then
      now=$(mono_seconds)
      if [[ "$now" -ge "$hard_deadline" ]]; then
        echo "pantry update: still migrating after ${migr_cap}s" >&2
        return 1
      fi
      echo "pantry update: migrations still running"
      sleep "$interval"
      continue
    fi
    build=$(curl -sS --max-time 5 "http://127.0.0.1:${host_port}/api/build" 2>/dev/null || true)
    commit=$(json_string_field "$build" commit)
    if health_is_ok "$health"; then
      if [[ -z "$revision" || ( -n "$commit" && "$commit" == "$revision" ) ]]; then
        if [[ "${ROLLBACK_HEALTH:-}" == 1 ]]; then
          if [[ -n "$commit" ]]; then
            echo "pantry update: previous image healthy, commit ${commit}"
          else
            echo "pantry update: previous image healthy"
          fi
        elif [[ -n "$commit" ]]; then
          echo "pantry update: healthy, running commit ${commit}"
        else
          echo "pantry update: healthy"
        fi
        return 0
      fi
    fi
    now=$(mono_seconds)
    if [[ "$timeout" -le 0 || "$now" -ge "$short_deadline" ]]; then
      break
    fi
    sleep "$interval"
  done
  echo "pantry update: health check timed out after ${timeout}s (wanted commit ${revision:-<any>})" >&2
  echo "pantry update: last /health: ${health:-<none>}" >&2
  echo "pantry update: last /api/build: ${build:-<none>}" >&2
  return 1
}

# restore_local_tags moves each compose ref whose pulled digest differs
# from the image recorded before pull back onto that previous digest.
# Pull points the local tag at the remote image. Doing this after the
# remote digest has been inspected keeps the next run honest: a new
# remote digest still looks new, and the rejected digest still does not
# look like the image that is running.
restore_local_tags() {
  local state="$1" svc running_id good pin ref remote remote_id
  while IFS=$'\t' read -r svc running_id good; do
    [[ -z "$svc" ]] && continue
    ref=$(awk -F '\t' -v s="$svc" '$1 == s { print $2; exit }' "$state/refs.tsv")
    [[ -z "$ref" || "$ref" == *@sha256:* ]] && continue
    pin=$good
    [[ -z "$pin" ]] && pin=$running_id
    [[ -z "$pin" ]] && continue
    remote=$(image_repo_digest "$ref" "$ref")
    remote_id=$(docker image inspect -f '{{.Id}}' "$ref" 2>/dev/null || true)
    remote_id=$(one_line "$remote_id")
    if [[ -n "$good" && -n "$remote" && "$remote" == "$good" ]]; then
      continue
    fi
    if [[ -n "$running_id" && -n "$remote_id" && "$remote_id" == "$running_id" ]]; then
      continue
    fi
    if ! docker tag "$pin" "$ref"; then
      echo "pantry update: could not retag ${ref} to ${pin}" >&2
      return 1
    fi
    echo "pantry update: retagged ${ref} to ${pin}" >&2
  done < "$state/running.tsv"
}

free_kib() {
  df -Pk "$1" 2>/dev/null | awk 'NR==2 { print $4 }'
}

ensure_free_space() {
  local min path free root
  min=${PANTRY_UPDATE_MIN_FREE_KIB:-65536}
  for path in "$PANTRY_DIR" /tmp; do
    free=$(free_kib "$path" || true)
    if [[ -z "$free" || ! "$free" =~ ^[0-9]+$ ]]; then
      echo "PANTRY UPDATE FAILED: could not read free space on ${path}" >&2
      return 1
    fi
    if [[ "$free" -lt "$min" ]]; then
      echo "PANTRY UPDATE FAILED: ${path} has ${free} KiB free; need at least ${min} before deploying" >&2
      return 1
    fi
  done
  root=$(docker info -f '{{.DockerRootDir}}' 2>/dev/null || true)
  root=$(one_line "$root")
  if [[ -n "$root" && -d "$root" ]]; then
    free=$(free_kib "$root" || true)
    if [[ -n "$free" && "$free" =~ ^[0-9]+$ && "$free" -lt "$min" ]]; then
      echo "PANTRY UPDATE FAILED: docker root ${root} has ${free} KiB free; need at least ${min} before deploying" >&2
      return 1
    fi
  fi
}

save_rejected_logs() {
  local cid dest
  cid=$("${compose[@]}" ps -aq pantry 2>/dev/null | head -n 1 || true)
  cid=$(one_line "$cid")
  dest="$PANTRY_DIR/update-rejected-pantry.log"
  if [[ -z "$cid" ]]; then
    echo "pantry update: no pantry container to capture logs from" >&2
    return 0
  fi
  if docker logs --tail 300 "$cid" >"$dest" 2>&1; then
    echo "pantry update: saved rejected container logs to ${dest}" >&2
  else
    echo "PANTRY UPDATE FAILED: could not save container logs to ${dest}" >&2
  fi
}

# newest_snapshot_path prints a pantry-pre-*.db path only when the rejected
# container log says this deploy wrote it. "already exists" names an older
# file and is ignored. The last write in the log wins.
newest_snapshot_path() {
  local log="$PANTRY_DIR/update-rejected-pantry.log" line path newest=""
  [[ -f "$log" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" == *"already exists"* ]] && continue
    [[ "$line" == *"wrote"* && "$line" == *"backup"* ]] || continue
    path=$(printf '%s\n' "$line" | grep -oE '/data/pantry-pre-[^[:space:]]+\.db' | tail -n 1 || true)
    [[ -n "$path" ]] && newest=$path
  done < "$log"
  printf '%s' "$newest"
}

log_both_down() {
  cat >&2 <<'EOF'
================================================================
BOTH IMAGES DOWN
The new image failed and the previous image did not become healthy.
The site may be unreachable. This script does not restore the database.
================================================================
EOF
}

write_pin_override() {
  local pins="$1" override="$2" svc pin rest any=0
  if ! printf 'services:\n' > "$override"; then
    return 1
  fi
  while IFS=$'\t' read -r svc pin rest; do
    [[ -z "$svc" || "$svc" == \#* ]] && continue
    [[ -z "$pin" ]] && continue
    any=1
    printf '  %s:\n    image: %s\n' "$svc" "$pin" >> "$override"
    echo "pantry update: re-pinning ${svc} to ${pin}" >&2
  done < "$pins"
  [[ "$any" -eq 1 ]]
}

pins_from_running() {
  local state="$1" dest="$2" svc running_id digest pin
  : > "$dest"
  while IFS=$'\t' read -r svc running_id digest; do
    [[ -z "$svc" ]] && continue
    pin=$digest
    [[ -z "$pin" ]] && pin=$running_id
    [[ -z "$pin" ]] && continue
    printf '%s\t%s\n' "$svc" "$pin" >> "$dest"
  done < "$state/running.tsv"
}

# rollback_to_pins STATE PINS
# Returns 0 when the previous image is healthy, 2 when it started but is not
# healthy, and 1 when it did not start. Callers record a rejection only for
# 0 and 2.
rollback_to_pins() {
  local state="$1" pins="$2" override snap old_pin old_rev
  override=$(mktemp) || {
    echo "PANTRY UPDATE FAILED: could not create a rollback override file" >&2
    log_both_down
    return 1
  }
  if ! write_pin_override "$pins" "$override"; then
    cat >&2 <<'EOF'
================================================================
PANTRY UPDATE FAILED: no previous image digest was recorded, so the new image was left in place.
This script does not undo a database migration. The migration runner is not transactional.
Automatic pre-migration snapshots, when one was taken, are files named pantry-pre-*.db in /data (the pantry-data volume).
Restoring one of those over /data/pantry.db is a manual step. This script does not restore the database.
================================================================
EOF
    rm -f "$override"
    return 1
  fi
  save_rejected_logs
  log_rollback_banner
  snap=$(newest_snapshot_path || true)
  if [[ -n "$snap" ]]; then
    echo "Pre-migration snapshot written by this deploy: ${snap}" >&2
  else
    echo "pantry update: no new snapshot found" >&2
  fi
  if ! "${compose[@]}" -f "$override" up -d --pull never; then
    echo "PANTRY UPDATE FAILED: could not start the previous image." >&2
    log_both_down
    rm -f "$override"
    return 1
  fi
  rm -f "$override"
  old_pin=$(awk -F '\t' '$1 == "pantry" { print $2; exit }' "$pins")
  old_rev=""
  if [[ -n "$old_pin" ]]; then
    old_rev=$(image_revision "$old_pin" || true)
  fi
  ROLLBACK_HEALTH=1
  if ! wait_for_expected_build "$state" "$old_rev"; then
    ROLLBACK_HEALTH=""
    log_both_down
    return 2
  fi
  ROLLBACK_HEALTH=""
  return 0
}

write_pending() {
  local state="$1" tmp svc running_id previous desired pin
  tmp=$(mktemp "${PANTRY_DIR}/.update-pending.XXXXXX") || {
    echo "PANTRY UPDATE FAILED: could not create the pending-update marker" >&2
    return 1
  }
  while IFS=$'\t' read -r svc running_id previous; do
    [[ -z "$svc" ]] && continue
    desired=$(service_desired_digest "$state" "$svc")
    pin=$previous
    [[ -z "$pin" ]] && pin=$running_id
    printf '%s\t%s\t%s\n' "$svc" "$pin" "$desired"
  done < "$state/running.tsv" > "$tmp"
  if ! mv "$tmp" "$PANTRY_DIR/.update-pending"; then
    echo "PANTRY UPDATE FAILED: could not write ${PANTRY_DIR}/.update-pending" >&2
    rm -f "$tmp"
    return 1
  fi
}

# deploy_was_started is true when a service's running digest is the digest
# .update-pending said we were deploying, and that digest is not the previous one.
deploy_was_started() {
  local state="$1" svc pin desired current
  [[ -f "$PANTRY_DIR/.update-pending" ]] || return 1
  while IFS=$'\t' read -r svc pin desired; do
    [[ -z "$svc" || "$svc" == \#* ]] && continue
    [[ -n "$desired" && "$desired" != "$pin" ]] || continue
    current=$(running_field "$svc" 3 "$state")
    if [[ "$current" == "$desired" ]]; then
      return 0
    fi
  done < "$PANTRY_DIR/.update-pending"
  return 1
}

record_pending_rejections() {
  local svc pin desired failed=0 recorded=0 spill=0
  while IFS=$'\t' read -r svc pin desired; do
    [[ -z "$svc" || "$svc" == \#* ]] && continue
    [[ -n "$desired" && "$desired" != "$pin" ]] || continue
    if [[ "$spill" -eq 0 ]] && ! should_reject_service "$svc"; then
      continue
    fi
    if remember_rejected_digest "$svc" "$desired"; then
      recorded=1
    else
      failed=1
    fi
  done < "$PANTRY_DIR/.update-pending"
  if [[ "$recorded" -eq 0 && "$spill" -eq 0 ]]; then
    echo "<4>pantry update: warning: could not pin the failure on one service; rejecting every changed digest" >&2
    PANTRY_VERIFIED=0
    FAILED_SERVICES=""
    spill=1
    while IFS=$'\t' read -r svc pin desired; do
      [[ -z "$svc" || "$svc" == \#* ]] && continue
      [[ -n "$desired" && "$desired" != "$pin" ]] || continue
      if remember_rejected_digest "$svc" "$desired"; then
        recorded=1
      else
        failed=1
      fi
    done < "$PANTRY_DIR/.update-pending"
  fi
  if [[ "$recorded" -eq 0 ]]; then
    echo "PANTRY UPDATE FAILED: no changed digest was recorded, so the next run may try this release again" >&2
    return 1
  fi
  [[ "$failed" -eq 0 ]]
}

retag_pins() {
  local file="$1" state="$2" svc pin desired ref
  while IFS=$'\t' read -r svc pin desired; do
    [[ -z "$svc" || "$svc" == \#* ]] && continue
    ref=$(service_ref "$svc" "$state")
    [[ -z "$ref" || "$ref" == *@sha256:* || -z "$pin" ]] && continue
    if ! docker tag "$pin" "$ref"; then
      echo "pantry update: could not retag ${ref} to ${pin}" >&2
      return 1
    fi
    echo "pantry update: retagged ${ref} to ${pin}" >&2
  done < "$file"
}

# record_container_baseline saves status and RestartCount before recreate.
record_container_baseline() {
  local state="$1" svc image line status restarts
  : > "$state/pre-deploy.tsv"
  while IFS=$'\t' read -r svc image; do
    [[ -z "$svc" ]] && continue
    line=$(container_state "$svc" || true)
    status=${line%% *}
    restarts=${line##* }
    if [[ ! "$restarts" =~ ^[0-9]+$ ]]; then
      restarts=0
    fi
    [[ -n "$status" ]] || status=missing
    printf '%s\t%s\t%s\n' "$svc" "$status" "$restarts" >> "$state/pre-deploy.tsv"
  done < "$state/refs.tsv"
}

pre_status() {
  awk -F '\t' -v s="$1" '$1 == s { print $2; exit }' "$2/pre-deploy.tsv"
}

# warn_unchanged logs a journal warning for a service whose image did not
# change. A down non-proxy service is remembered so the next run repeats
# the warning until the container is running again.
warn_unchanged() {
  local svc="$1" detail="$2" mark="$PANTRY_DIR/.update-sidecar-warn" line remember="${3:-}"
  echo "<4>pantry update: warning: ${svc} ${detail}; not rolling back" >&2
  [[ "$remember" == 1 ]] || return 0
  [[ "$svc" == caddy || "$svc" == caddy-tunnel ]] && return 0
  if [[ -f "$mark" ]]; then
    while IFS= read -r line; do
      [[ "$line" == "$svc" ]] && return 0
    done < "$mark"
  fi
  printf '%s\n' "$svc" >> "$mark"
}

# recheck_sidecar_warnings repeats warnings for services still down, and
# drops the ones that are running. It runs even when no image changed.
recheck_sidecar_warnings() {
  local mark="$PANTRY_DIR/.update-sidecar-warn" tmp svc line status
  [[ -f "$mark" ]] || return 0
  tmp=$(mktemp) || return 0
  while IFS= read -r svc || [[ -n "${svc:-}" ]]; do
    [[ -z "${svc:-}" || "$svc" == \#* ]] && continue
    line=$(container_state "$svc" || true)
    status=${line%% *}
    if [[ "$status" == running ]]; then
      echo "pantry update: ${svc} is running again"
      continue
    fi
    echo "<4>pantry update: warning: ${svc} is ${status:-unhealthy} and its image did not change; not rolling back" >&2
    printf '%s\n' "$svc" >> "$tmp"
  done < "$mark"
  mv "$tmp" "$mark"
  [[ -s "$mark" ]] || rm -f "$mark"
}

# confirm_recreated checks changed services, then checks them again after a
# short settle. An unchanged service that is down or restarting is a warning.
# One sleep, capped at 15s.
confirm_recreated() {
  local state="$1" svc image line status restarts base failed=0 settle before
  : > "$state/restart-base.tsv"
  while IFS=$'\t' read -r svc image; do
    [[ -z "$svc" ]] && continue
    line=$(container_state "$svc" || true)
    status=${line%% *}
    restarts=${line##* }
    if [[ ! "$restarts" =~ ^[0-9]+$ ]]; then
      restarts=0
    fi
    if ! service_changed "$state" "$svc"; then
      if [[ "$status" != running ]]; then
        before=$(pre_status "$svc" "$state")
        if [[ "$before" == running ]]; then
          warn_unchanged "$svc" "is ${status:-unhealthy} and its image did not change" 1
        else
          warn_unchanged "$svc" "was already ${status:-unhealthy} and its image did not change" 1
        fi
      elif [[ -n "$status" ]]; then
        printf '%s\t%s\n' "$svc" "$restarts" >> "$state/restart-base.tsv"
      fi
      continue
    fi
    if [[ "$status" != running ]]; then
      echo "pantry update: ${svc} is not running (${status:-missing})" >&2
      note_failed "$svc"
      failed=1
      continue
    fi
    printf '%s\t%s\n' "$svc" "$restarts" >> "$state/restart-base.tsv"
  done < "$state/refs.tsv"
  [[ "$failed" -eq 0 ]] || return 1

  settle=${PANTRY_UPDATE_SETTLE_SECONDS:-12}
  if [[ ! "$settle" =~ ^[0-9]+$ ]]; then
    settle=12
  fi
  if [[ "$settle" -gt 15 ]]; then
    settle=15
  fi
  if [[ "$settle" -gt 0 ]]; then
    echo "pantry update: watching containers for ${settle}s" >&2
    sleep "$settle"
  fi

  while IFS=$'\t' read -r svc image; do
    [[ -z "$svc" ]] && continue
    line=$(container_state "$svc" || true)
    status=${line%% *}
    restarts=${line##* }
    if [[ ! "$restarts" =~ ^[0-9]+$ ]]; then
      restarts=0
    fi
    if ! service_changed "$state" "$svc"; then
      if [[ "$status" != running ]]; then
        continue
      fi
      base=$(awk -F '\t' -v s="$svc" '$1 == s { print $2; exit }' "$state/restart-base.tsv")
      if [[ "$base" =~ ^[0-9]+$ && "$restarts" -gt "$base" ]]; then
        warn_unchanged "$svc" "restarted during settle (${base} -> ${restarts}) but its image did not change"
      fi
      continue
    fi
    if [[ "$status" != running ]]; then
      echo "pantry update: ${svc} is not running (${status:-missing})" >&2
      note_failed "$svc"
      failed=1
      continue
    fi
    base=$(awk -F '\t' -v s="$svc" '$1 == s { print $2; exit }' "$state/restart-base.tsv")
    if [[ "$base" =~ ^[0-9]+$ && "$restarts" -gt "$base" ]]; then
      echo "pantry update: ${svc} restarted (${base} -> ${restarts})" >&2
      note_failed "$svc"
      failed=1
    fi
  done < "$state/refs.tsv"
  [[ "$failed" -eq 0 ]]
}

note_stopped_services() {
  local state="$1" svc image line status
  while IFS=$'\t' read -r svc image; do
    [[ -z "$svc" ]] && continue
    line=$(container_state "$svc" || true)
    status=${line%% *}
    if [[ "$status" == running ]]; then
      continue
    fi
    if ! service_changed "$state" "$svc"; then
      warn_unchanged "$svc" "is ${status:-unhealthy} and its image did not change" 1
      continue
    fi
    note_failed "$svc"
  done < "$state/refs.tsv"
}

publish_auth_present() {
  if [[ -f /opt/pantry/auth.caddy ]]; then
    return 0
  fi
  [[ "$PANTRY_DIR" != /opt/pantry && -f "$PANTRY_DIR/auth.caddy" ]]
}

# proxy_compose_service is the compose service that publishes the hostname.
# Tunnel mode's service is caddy-tunnel. The certificate path's is caddy.
proxy_compose_service() {
  if [[ "$mode" == tunnel ]]; then
    printf 'caddy-tunnel'
    return 0
  fi
  printf 'caddy'
}

# tunnel_connect_failed is true for curl exits that never reached Caddy:
# resolve failure, connection refused, timeout, and empty reply.
tunnel_connect_failed() {
  case "$1" in
    6|7|28|52) return 0 ;;
    *) return 1 ;;
  esac
}

# tunnel_probe_curl requests http://$PUBLIC_HOST/api/build with that name
# resolved to caddy-tunnel's pinned address. The body is written to DEST
# in this shell, and TUNNEL_PROBE_RC is curl's exit code. A command
# substitution would drop that exit code. PUBLIC_HOST is the value compose
# interpolates into Caddyfile.tunnel.
tunnel_probe_curl() {
  local dest="$1" host ip="10.77.77.3"
  TUNNEL_PROBE_RC=0
  : > "$dest"
  host=$(publish_env_value PUBLIC_HOST)
  if [[ -z "$host" ]]; then
    TUNNEL_PROBE_RC=6
    return 0
  fi
  curl -sS --max-time 5 --resolve "${host}:80:${ip}" "http://${host}/api/build" >"$dest" 2>"${dest}.err" || TUNNEL_PROBE_RC=$?
  if [[ -s "${dest}.err" ]]; then
    cat "${dest}.err" >&2
  fi
}

# remember_tunnel_reachability probes the stack that is still running,
# before recreate. A later refusal or hang is ignored only when this
# probe could not connect either. Curl 0 also marks the proxy reachable.
remember_tunnel_reachability() {
  local state="$1"
  TUNNEL_REACHABLE=0
  [[ "$mode" == tunnel ]] || return 0
  publish_auth_present || return 0
  [[ -n "$(publish_env_value PUBLIC_HOST)" ]] || return 0
  echo "pantry update: checking whether caddy-tunnel was reachable before recreate"
  tunnel_probe_curl "$state/tunnel-pre.body"
  if ! tunnel_connect_failed "$TUNNEL_PROBE_RC"; then
    TUNNEL_REACHABLE=1
  fi
  if [[ "$TUNNEL_PROBE_RC" -eq 0 ]]; then
    PROXY_REACHABLE=1
  fi
}

# remember_acme_reachability is the certificate-mode form of the same
# pre-deploy probe. HTTPS to PUBLIC_HOST, resolved to this host.
remember_acme_reachability() {
  local host rc=0
  [[ "$mode" == acme ]] || return 0
  publish_auth_present || return 0
  host=$(publish_env_value PUBLIC_HOST)
  [[ -n "$host" ]] || return 0
  echo "pantry update: checking whether Caddy was reachable before recreate"
  curl -sf --max-time 10 --resolve "${host}:443:127.0.0.1" "https://${host}/api/build" >/dev/null 2>/dev/null || rc=$?
  if [[ "$rc" -eq 0 ]]; then
    PROXY_REACHABLE=1
  fi
}

# probe_tunnel_listener checks the new stack through caddy-tunnel.
# The Host header is PUBLIC_HOST, because that is the only site in
# Caddyfile.tunnel. The commit has to match the pantry image label.
# Connect failures are skipped only when the pre-deploy probe failed too.
probe_tunnel_listener() {
  local state="$1" host ip="10.77.77.3" body commit revision ref
  host=$(publish_env_value PUBLIC_HOST)
  [[ -n "$host" ]] || return 0
  echo "pantry update: checking http://${host}/api/build through caddy-tunnel at ${ip}"
  tunnel_probe_curl "$state/tunnel-post.body"
  body=$(<"$state/tunnel-post.body")
  if tunnel_connect_failed "$TUNNEL_PROBE_RC"; then
    if [[ "$TUNNEL_REACHABLE" -ne 1 ]]; then
      echo "pantry update: caddy-tunnel internal listener at ${ip}:80 is not reachable from this host; skipped. The pre-deploy probe could not reach it either. Cloudflare uses http://caddy:80 on the cf-tunnel network, which is not published on the Pi." >&2
      return 0
    fi
    echo "pantry update: caddy-tunnel probe of http://${host}/api/build failed (curl ${TUNNEL_PROBE_RC})" >&2
    return 1
  fi
  if [[ "$TUNNEL_PROBE_RC" -ne 0 ]]; then
    echo "pantry update: caddy-tunnel probe of http://${host}/api/build failed (curl ${TUNNEL_PROBE_RC})" >&2
    return 1
  fi
  commit=$(json_string_field "$body" commit)
  ref=$(service_ref pantry "$state")
  revision=""
  if [[ -n "$ref" ]]; then
    revision=$(image_revision "$ref" || true)
  fi
  if [[ -z "$revision" || "$commit" != "$revision" ]]; then
    echo "pantry update: caddy-tunnel probe of http://${host}/api/build failed (wanted commit ${revision:-<missing>}, got ${commit:-<none>})" >&2
    return 1
  fi
}

probe_caddy() {
  local state="$1" host
  host=$(publish_env_value PUBLIC_HOST)
  [[ -n "$host" ]] || return 0
  publish_auth_present || return 0
  if [[ "$mode" == tunnel ]]; then
    probe_tunnel_listener "$state"
    return
  fi
  [[ "$mode" == acme ]] || return 0
  echo "pantry update: checking https://${host}/api/build through Caddy"
  curl -sf --max-time 10 --resolve "${host}:443:127.0.0.1" "https://${host}/api/build" >/dev/null
}

clear_state() {
  if [[ -n "${UPDATE_STATE:-}" ]]; then
    rm -rf "$UPDATE_STATE"
    UPDATE_STATE=""
  fi
}

finish_rollback() {
  local state="$1" pins="$2" rc=0
  rollback_to_pins "$state" "$pins" || rc=$?
  if [[ "$rc" -eq 0 || "$rc" -eq 2 ]]; then
    # Record while the local tag still names the image that failed. Retagging
    # first would make that digest look like the one already running.
    if [[ "$pins" == "$PANTRY_DIR/.update-pending" ]]; then
      if ! record_pending_rejections; then
        echo "PANTRY UPDATE FAILED: could not record a rejected digest; the next run may try that image again" >&2
      fi
      if ! retag_pins "$pins" "$state"; then
        echo "pantry update: the previous container is running, but the local tag still names the rejected image. A manual docker compose up would start it." >&2
      fi
    else
      if ! record_changed_rejections "$state"; then
        echo "PANTRY UPDATE FAILED: could not record a rejected digest; the next run may try that image again" >&2
      fi
      if ! restore_local_tags "$state"; then
        echo "pantry update: the previous container is running, but the local tag still names the rejected image. A manual docker compose up would start it." >&2
      fi
    fi
    rm -f "$PANTRY_DIR/.update-pending"
  else
    echo "pantry update: previous image did not start; the next run will try this image again" >&2
  fi
}

# resume_pending_deploy checks the digest named in .update-pending.
# Returns 0 when the script should continue, and exits on a failed resume.
resume_pending_deploy() {
  local state="$1" desired rev
  if ! deploy_was_started "$state"; then
    echo "pantry update: pending marker was left before the new image started"
    rm -f "$PANTRY_DIR/.update-pending"
    return 0
  fi
  desired=$(awk -F '\t' '$1 == "pantry" { print $3; exit }' "$PANTRY_DIR/.update-pending")
  if [[ -z "$desired" ]]; then
    echo "pantry update: pending marker has no pantry digest; skipping the resume check" >&2
    rm -f "$PANTRY_DIR/.update-pending"
    return 0
  fi
  if ! docker image inspect "$desired" >/dev/null 2>&1; then
    echo "pantry update: pending digest ${desired} is not on this machine; skipping the resume check" >&2
    rm -f "$PANTRY_DIR/.update-pending"
    return 0
  fi
  rev=$(image_revision "$desired" || true)
  record_container_baseline "$state"
  echo "pantry update: previous deploy was not confirmed; checking health"
  PANTRY_VERIFIED=0
  FAILED_SERVICES=""
  if [[ -z "$rev" ]]; then
    echo "pantry update: pending digest ${desired} has no revision label" >&2
    echo "pantry update: pending deploy failed the health check" >&2
    finish_rollback "$state" "$PANTRY_DIR/.update-pending"
    clear_state
    exit 1
  fi
  if wait_for_expected_build "$state" "$rev"; then
    PANTRY_VERIFIED=1
    if confirm_recreated "$state"; then
      rm -f "$PANTRY_DIR/.update-pending"
      echo "pantry update: pending deploy is healthy"
      PANTRY_VERIFIED=0
      FAILED_SERVICES=""
      return 0
    fi
  fi
  echo "pantry update: pending deploy failed the health check" >&2
  finish_rollback "$state" "$PANTRY_DIR/.update-pending"
  clear_state
  exit 1
}

apply_compose() {
  local state up_ok ref pins desired containers_ok caddy_ok proxy
  PANTRY_VERIFIED=0
  FAILED_SERVICES=""
  TUNNEL_REACHABLE=0
  PROXY_REACHABLE=0
  TUNNEL_PROBE_RC=0
  UPDATE_STATE=$(mktemp -d) || {
    echo "PANTRY UPDATE FAILED: could not create a temp directory" >&2
    return 1
  }
  state=$UPDATE_STATE
  if ! record_running_images "$state"; then
    clear_state
    return 1
  fi

  "${compose[@]}" pull

  if [[ -f "$PANTRY_DIR/.update-pending" ]]; then
    resume_pending_deploy "$state"
  fi

  if images_unchanged "$state"; then
    recheck_sidecar_warnings
    echo "pantry update: images unchanged"
    clear_state
    return 0
  fi

  if ! park_rejected_services "$state"; then
    echo "pantry update: could not point a rejected image tag back at the previous digest." >&2
  fi
  if images_unchanged "$state"; then
    recheck_sidecar_warnings
    clear_state
    return 0
  fi

  ref=$(service_ref pantry "$state")
  if ! image_revision "$ref" >/dev/null; then
    echo "pantry update: ${ref} has no org.opencontainers.image.revision label; not starting it" >&2
    if service_changed "$state" pantry; then
      desired=$(service_desired_digest "$state" pantry)
      if ! remember_rejected_digest pantry "$desired"; then
        echo "PANTRY UPDATE FAILED: could not record the rejected digest ${desired}" >&2
      fi
    fi
    if ! restore_local_tags "$state"; then
      echo "pantry update: could not point the local image tag back at the previous digest." >&2
    fi
    clear_state
    exit 1
  fi

  if ! ensure_free_space; then
    clear_state
    exit 1
  fi
  if ! write_pending "$state"; then
    clear_state
    exit 1
  fi

  # After a successful pull, drop a container left from the other mode so
  # the proxy name can be recreated on this path. A tunnel token with no
  # password file leaves whatever is already running alone.
  if [[ "$mode" != tunnel || -f "$PANTRY_DIR/auth.caddy" ]]; then
    drop_other_publish_containers "$mode"
  fi
  if [[ "$mode" == tunnel && -f "$PANTRY_DIR/auth.caddy" ]]; then
    drop_stale_cloudflared
  fi
  record_container_baseline "$state"
  remember_tunnel_reachability "$state"
  remember_acme_reachability
  up_ok=0
  if "${compose[@]}" up -d; then
    up_ok=1
  fi
  PANTRY_VERIFIED=0
  FAILED_SERVICES=""
  containers_ok=0
  caddy_ok=0
  if [[ "$up_ok" -eq 1 ]]; then
    if wait_for_expected_build "$state"; then
      PANTRY_VERIFIED=1
      if confirm_recreated "$state"; then
        containers_ok=1
      fi
    fi
  else
    echo "pantry update: docker compose up failed" >&2
    # One poll. The health timeout must not add another 90s after compose
    # has already said the recreate failed. The exit code is not the
    # verdict: changed services that are healthy stay up.
    if PANTRY_UPDATE_HEALTH_TIMEOUT=0 wait_for_expected_build "$state"; then
      PANTRY_VERIFIED=1
    fi
    note_stopped_services "$state"
    if [[ "$PANTRY_VERIFIED" -eq 1 ]] && ! changed_failure_named; then
      containers_ok=1
    fi
  fi
  if [[ "$PANTRY_VERIFIED" -eq 1 && "$containers_ok" -eq 1 ]]; then
    if probe_caddy "$state"; then
      caddy_ok=1
    else
      proxy=$(proxy_compose_service)
      if service_changed "$state" "$proxy"; then
        note_failed "$proxy"
      elif [[ "$PROXY_REACHABLE" -eq 1 ]] && service_changed "$state" pantry; then
        echo "<4>pantry update: warning: ${proxy} was reachable before recreate and failed after; rejecting pantry" >&2
        note_failed pantry
      else
        warn_unchanged "$proxy" "probe failed and its image did not change"
        caddy_ok=1
      fi
    fi
  fi
  if [[ "$PANTRY_VERIFIED" -eq 1 && "$containers_ok" -eq 1 && "$caddy_ok" -eq 1 ]]; then
    if [[ "$up_ok" -eq 0 ]]; then
      echo "<4>pantry update: warning: docker compose up failed, but every changed service is healthy; not rolling back" >&2
    fi
    rm -f "$PANTRY_DIR/.update-pending"
    clear_state
    return 0
  fi
  pins=$(mktemp) || {
    echo "PANTRY UPDATE FAILED: could not create a rollback pin file" >&2
    log_both_down
    clear_state
    exit 1
  }
  pins_from_running "$state" "$pins"
  finish_rollback "$state" "$pins"
  rm -f "$pins"
  clear_state
  exit 1
}

round=0
while true; do
  round=$((round + 1))
  if [[ "$round" -gt 3 ]]; then
    echo "pantry update: stopping after 3 pulls; the timer will retry"
    exit 0
  fi
  before=$(trigger_digest || true)
  if [[ -n "$before" ]]; then
    echo "pantry update: deploy trigger $(tr '\n' ' ' < "$PANTRY_DIR/deploy-trigger/request")"
  else
    echo "pantry update: scheduled pull"
  fi
  apply_compose
  after=$(trigger_digest || true)
  if [[ "$after" == "$before" ]]; then
    echo "pantry update: finished"
    break
  fi
  echo "pantry update: trigger changed during the pull; pulling again"
done
