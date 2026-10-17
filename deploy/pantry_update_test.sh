#!/usr/bin/env bash
# Behavioral checks for pantry-update.sh. Docker and curl are stubs so this
# can run without root, a Pi, or a registry.

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
UPDATE="$ROOT/deploy/systemd/pantry-update.sh"

bash -n "$UPDATE"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

LAST_RC=0
LAST_OUT=""
LAST_DOCKER=""
LAST_CURL=""
LAST_SEQUENCE=""

make_bin() {
  local bin="$1"
  mkdir -p "$bin"

  cat > "$bin/docker" << 'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${DOCKER_LOG:?}"
if [[ -n "${SEQUENCE_LOG:-}" ]]; then
  printf 'docker %s\n' "$*" >> "$SEQUENCE_LOG"
fi

tsv_col() {
  local key="$1" col="$2" svc ref run_id run_dig pull_id pull_dig revision run_revision
  while IFS=$'\t' read -r svc ref run_id run_dig pull_id pull_dig revision run_revision || [[ -n "$svc" ]]; do
    [[ -z "$svc" || "$svc" == \#* ]] && continue
    if [[ "$key" == "$svc" || "$key" == "cid-$svc" || "$key" == "$ref" || "$key" == "$run_id" || "$key" == "$pull_id" || "$key" == "$run_dig" || "$key" == "$pull_dig" ]]; then
      case "$col" in
        ref) printf '%s' "$ref" ;;
        run_id) printf '%s' "$run_id" ;;
        run_dig) printf '%s' "$run_dig" ;;
        pull_id) printf '%s' "$pull_id" ;;
        pull_dig) printf '%s' "$pull_dig" ;;
        revision)
          if [[ -n "$run_revision" && ( "$key" == "$run_id" || "$key" == "$run_dig" || "$key" == "cid-$svc" ) ]]; then
            printf '%s' "$run_revision"
          else
            printf '%s' "$revision"
          fi
          ;;
        run_revision) printf '%s' "$run_revision" ;;
        *) return 1 ;;
      esac
      return 0
    fi
  done < "${STUB_IMAGES:?}"
  return 1
}

# Local tags survive across runs. Pull moves each ref onto the remote
# image. docker tag records a switch back to an older digest.
set_tag() {
  local ref="$1" id="$2" dig="$3" tmp tref tid tdig
  [[ -n "${STUB_TAG_STATE:-}" ]] || return 0
  tmp=$(mktemp)
  if [[ -f "$STUB_TAG_STATE" ]]; then
    while IFS=$'\t' read -r tref tid tdig || [[ -n "$tref" ]]; do
      [[ -z "$tref" || "$tref" == "$ref" ]] && continue
      printf '%s\t%s\t%s\n' "$tref" "$tid" "$tdig" >> "$tmp"
    done < "$STUB_TAG_STATE"
  fi
  printf '%s\t%s\t%s\n' "$ref" "$id" "$dig" >> "$tmp"
  mv "$tmp" "$STUB_TAG_STATE"
}

tag_field() {
  local ref="$1" col="$2" tref tid tdig
  [[ -n "${STUB_TAG_STATE:-}" && -f "$STUB_TAG_STATE" ]] || return 1
  while IFS=$'\t' read -r tref tid tdig || [[ -n "$tref" ]]; do
    [[ "$tref" == "$ref" ]] || continue
    case "$col" in
      id) printf '%s' "$tid" ;;
      dig) printf '%s' "$tdig" ;;
      *) return 1 ;;
    esac
    return 0
  done < "$STUB_TAG_STATE"
  return 1
}

apply_pull_tags() {
  local svc ref run_id run_dig pull_id pull_dig revision
  while IFS=$'\t' read -r svc ref run_id run_dig pull_id pull_dig revision || [[ -n "$svc" ]]; do
    [[ -z "$svc" || "$svc" == \#* ]] && continue
    set_tag "$ref" "$pull_id" "$pull_dig"
  done < "${STUB_IMAGES:?}"
}

format_arg() {
  local prev="" arg
  for arg in "$@"; do
    if [[ "$prev" == -f || "$prev" == --format ]]; then
      printf '%s' "$arg"
      return 0
    fi
    prev=$arg
  done
}

if [[ "$1" == rm ]]; then
  exit 0
fi

if [[ "$1" == logs ]]; then
  printf '%s\n' "${STUB_CONTAINER_LOGS:-wrote backup: /data/pantry-pre-targets-20261017.db}"
  exit 0
fi

if [[ "$1" == info ]]; then
  exit 0
fi

if [[ "$1" == inspect ]]; then
  format=$(format_arg "$@")
  target=${*: -1}
  if [[ "$format" == *com.docker.compose.service* ]]; then
    exit 1
  fi
  if [[ "$format" == *State.Status* ]]; then
    name=${target#cid-}
    status=${STUB_CONTAINER_STATUS:-running}
    restarts=${STUB_RESTART_COUNT:-0}
    if [[ -n "${STUB_STATUS_TSV:-}" && -f "$STUB_STATUS_TSV" ]]; then
      while IFS=$'\t' read -r svc st rc || [[ -n "${svc:-}" ]]; do
        [[ -z "${svc:-}" || "$svc" == \#* ]] && continue
        [[ "$svc" == "$name" ]] || continue
        status=$st
        restarts=${rc:-0}
        break
      done < "$STUB_STATUS_TSV"
    fi
    if [[ -n "${STUB_BUMP_SERVICE:-}" && "$name" == "$STUB_BUMP_SERVICE" && -n "${STUB_INSPECT_STATE:-}" ]]; then
      n=0
      if [[ -f "$STUB_INSPECT_STATE" ]]; then
        n=$(awk -F '\t' -v s="$name" '$1 == s { print $2; exit }' "$STUB_INSPECT_STATE")
        n=${n:-0}
      fi
      n=$((n + 1))
      tmp=$(mktemp)
      if [[ -f "$STUB_INSPECT_STATE" ]]; then
        awk -F '\t' -v s="$name" '$1 != s { print }' "$STUB_INSPECT_STATE" > "$tmp"
      fi
      printf '%s\t%s\n' "$name" "$n" >> "$tmp"
      mv "$tmp" "$STUB_INSPECT_STATE"
      bump_on=${STUB_BUMP_ON_CALL:-2}
      if [[ "$n" -ge "$bump_on" ]]; then
        restarts=${STUB_BUMP_RESTARTS:-1}
        if [[ -n "${STUB_BUMP_STATUS:-}" ]]; then
          status=$STUB_BUMP_STATUS
        fi
      fi
    fi
    printf '%s %s\n' "$status" "$restarts"
    exit 0
  fi
  if [[ "$format" == *State.Running* ]]; then
    name=${target#cid-}
    id=$(tsv_col "$name" run_id || true)
    if [[ -n "$id" ]]; then
      printf 'true %s\n' "$id"
      exit 0
    fi
    exit 1
  fi
  exit 1
fi

if [[ "$1" == tag ]]; then
  src=$2
  dest=$3
  found_id=""
  found_dig=""
  while IFS=$'\t' read -r svc ref run_id run_dig pull_id pull_dig revision || [[ -n "${svc:-}" ]]; do
    [[ -z "${svc:-}" || "$svc" == \#* ]] && continue
    if [[ "$src" == "$run_dig" || "$src" == "$run_id" ]]; then
      found_id=$run_id
      found_dig=$run_dig
      break
    fi
    if [[ "$src" == "$pull_dig" || "$src" == "$pull_id" ]]; then
      found_id=$pull_id
      found_dig=$pull_dig
      break
    fi
  done < "${STUB_IMAGES:?}"
  if [[ -z "$found_id" ]]; then
    if [[ "$src" == sha256:* || "$src" == *@sha256:* ]]; then
      found_id=$src
      found_dig=$src
    else
      echo "stub docker tag: unknown source ${src}" >&2
      exit 1
    fi
  fi
  set_tag "$dest" "$found_id" "$found_dig"
  exit 0
fi

if [[ "$1" == image && "$2" == inspect ]]; then
  format=$(format_arg "$@")
  target=${*: -1}
  if [[ -n "${STUB_ABSENT_REFS:-}" ]]; then
    IFS=',' read -ra absent_refs <<< "$STUB_ABSENT_REFS"
    for absent in "${absent_refs[@]}"; do
      if [[ -n "$absent" && "$target" == "$absent" ]]; then
        echo "stub docker image inspect: absent ${target}" >&2
        exit 1
      fi
    done
  fi
  tagged_id=""
  tagged_dig=""
  if [[ "$target" != sha256:* ]]; then
    tagged_id=$(tag_field "$target" id || true)
    tagged_dig=$(tag_field "$target" dig || true)
  fi
  if [[ "$format" == *RepoDigests* ]]; then
    if [[ -n "$tagged_dig" ]]; then
      printf '%s\n' "$tagged_dig"
      exit 0
    fi
    run_id=$(tsv_col "$target" run_id || true)
    if [[ -n "$run_id" && "$run_id" == "$target" ]]; then
      tsv_col "$target" run_dig || true
      printf '\n'
      exit 0
    fi
    tsv_col "$target" pull_dig || true
    printf '\n'
    exit 0
  fi
  if [[ "$format" == *org.opencontainers.image.revision* ]]; then
    if [[ -n "$tagged_id" ]]; then
      rev=$(tsv_col "$tagged_id" revision || true)
      printf '%s\n' "$rev"
      exit 0
    fi
    tsv_col "$target" revision || true
    printf '\n'
    exit 0
  fi
  if [[ "$format" == *'.Id'* ]]; then
    if [[ -n "$tagged_id" ]]; then
      printf '%s\n' "$tagged_id"
      exit 0
    fi
    tsv_col "$target" pull_id || true
    printf '\n'
    exit 0
  fi
  exit 0
fi

if [[ "$1" == compose ]]; then
  joined="$*"
  if [[ "$joined" == *" config"* ]]; then
    printf 'services:\n'
    while IFS=$'\t' read -r svc ref run_id run_dig pull_id pull_dig revision run_revision || [[ -n "${svc:-}" ]]; do
      [[ -z "${svc:-}" || "$svc" == \#* ]] && continue
      printf '  %s:\n    image: %s\n' "$svc" "$ref"
    done < "${STUB_IMAGES:?}"
    exit 0
  fi
  if [[ "$joined" == *" pull" || "$joined" == *" pull "* ]]; then
    apply_pull_tags
    exit 0
  fi
  if [[ "$joined" == *" ps "* ]]; then
    svc=${*: -1}
    id=$(tsv_col "$svc" run_id || true)
    if [[ -n "$id" ]]; then
      printf 'cid-%s\n' "$svc"
    fi
    exit 0
  fi
  if [[ "$joined" == *"--pull never"* ]]; then
    prev=""
    for arg in "$@"; do
      if [[ "$prev" == -f && -f "$arg" && "$arg" != *docker-compose.yml ]]; then
        printf 'override-file\n' >> "${DOCKER_LOG:?}"
        cat "$arg" >> "${DOCKER_LOG:?}"
      fi
      prev=$arg
    done
    exit "${STUB_ROLLBACK_RC:-0}"
  fi
  if [[ "$joined" == *" up "* || "$joined" == *" up" ]]; then
    exit "${STUB_UP_RC:-0}"
  fi
  exit 0
fi

exit 0
EOF

  cat > "$bin/curl" << 'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${CURL_LOG:?}"
if [[ -n "${SEQUENCE_LOG:-}" ]]; then
  printf 'curl %s\n' "$*" >> "$SEQUENCE_LOG"
fi
url=""
for arg in "$@"; do
  case "$arg" in
    http://*|https://*) url=$arg ;;
  esac
done
rollback_phase=0
if [[ -n "${DOCKER_LOG:-}" && -f "$DOCKER_LOG" ]] && grep -q -- '--pull never' "$DOCKER_LOG"; then
  rollback_phase=1
fi
if [[ "$url" == *"/health"* ]]; then
  if [[ "$rollback_phase" -eq 1 ]]; then
    [[ "${STUB_ROLLBACK_HEALTH_RC:-0}" -eq 0 ]] || exit "$STUB_ROLLBACK_HEALTH_RC"
    printf '%s' "${STUB_ROLLBACK_HEALTH_BODY-{\"status\":\"ok\"}}"
    exit 0
  fi
  if [[ -n "${STUB_HEALTH_QUEUE:-}" && -s "$STUB_HEALTH_QUEUE" ]]; then
    body=$(head -n 1 "$STUB_HEALTH_QUEUE")
    tail -n +2 "$STUB_HEALTH_QUEUE" > "${STUB_HEALTH_QUEUE}.next"
    mv "${STUB_HEALTH_QUEUE}.next" "$STUB_HEALTH_QUEUE"
    printf '%s' "$body"
    exit 0
  fi
  [[ "${STUB_HEALTH_RC:-0}" -eq 0 ]] || exit "$STUB_HEALTH_RC"
  printf '%s' "${STUB_HEALTH_BODY-}"
  exit 0
fi
if [[ "$url" == *"/api/build"* ]]; then
  if [[ "$rollback_phase" -eq 1 ]]; then
    [[ "${STUB_ROLLBACK_BUILD_RC:-0}" -eq 0 ]] || exit "$STUB_ROLLBACK_BUILD_RC"
    if [[ -n "${STUB_ROLLBACK_BUILD_BODY:-}" ]]; then
      printf '%s' "$STUB_ROLLBACK_BUILD_BODY"
    else
      rev=$(awk -F '\t' '$1 == "pantry" { print ($8 != "" ? $8 : $7); exit }' "${STUB_IMAGES:?}")
      printf '{"commit":"%s"}' "$rev"
    fi
    exit 0
  fi
  if [[ -n "${STUB_BUILD_QUEUE:-}" && -s "$STUB_BUILD_QUEUE" ]]; then
    body=$(head -n 1 "$STUB_BUILD_QUEUE")
    tail -n +2 "$STUB_BUILD_QUEUE" > "${STUB_BUILD_QUEUE}.next"
    mv "${STUB_BUILD_QUEUE}.next" "$STUB_BUILD_QUEUE"
    printf '%s' "$body"
    exit 0
  fi
  [[ "${STUB_BUILD_RC:-0}" -eq 0 ]] || exit "$STUB_BUILD_RC"
  printf '%s' "${STUB_BUILD_BODY-}"
  exit 0
fi
exit 0
EOF

  cat > "$bin/chown" << 'EOF'
#!/usr/bin/env bash
exit 0
EOF

  chmod +x "$bin/docker" "$bin/curl" "$bin/chown"
}

prepare_pantry() {
  local pantry="$1"
  mkdir -p "$pantry"
  cp "$ROOT/deploy/publish-mode.sh" "$ROOT/deploy/auth-migrate.sh" "$ROOT/deploy/vapid-env.sh" "$pantry/"
  cat > "$pantry/.env" << 'EOF'
HOST_PORT=8080
PUBLIC_HOST=
PANTRY_IMAGE_TAG=latest
VAPID_PUBLIC_KEY=test-public
VAPID_PRIVATE_KEY=test-private
VAPID_SUBJECT=mailto:pantry@localhost
EOF
  cp "$pantry/.env" "$pantry/.env.orig"
}

# write_images DEST PANTRY_PULLED_ID PANTRY_PULLED_DIGEST CADDY_PULLED_ID CADDY_PULLED_DIGEST
write_images() {
  local dest="$1"
  cat > "$dest" << EOF
pantry	ghcr.io/rhionin/pantry:latest	sha256:runpantry	ghcr.io/rhionin/pantry@sha256:oldpantry	$2	$3	0123456789abcdef0123456789abcdef01234567
caddy	caddy:2.11.7-alpine	sha256:runcaddy	caddy@sha256:oldcaddy	$4	$5	
EOF
}

run_update() {
  local pantry="$1"
  local work bin out err
  work=$(mktemp -d)
  bin="$work/bin"
  out="$work/out"
  err="$work/err"
  make_bin "$bin"
  : > "$work/docker.log"
  : > "$work/curl.log"
  : > "$work/sequence.log"

  set +e
  DOCKER_LOG="$work/docker.log" \
    CURL_LOG="$work/curl.log" \
    SEQUENCE_LOG="$work/sequence.log" \
    STUB_IMAGES="$pantry/images.tsv" \
    STUB_TAG_STATE="$pantry/tag-state.tsv" \
    STUB_HEALTH_BODY="${STUB_HEALTH_BODY-}" \
    STUB_HEALTH_RC="${STUB_HEALTH_RC:-0}" \
    STUB_BUILD_BODY="${STUB_BUILD_BODY-}" \
    STUB_BUILD_RC="${STUB_BUILD_RC:-0}" \
    STUB_HEALTH_QUEUE="${STUB_HEALTH_QUEUE:-}" \
    STUB_BUILD_QUEUE="${STUB_BUILD_QUEUE:-}" \
    STUB_UP_RC="${STUB_UP_RC:-0}" \
    STUB_ROLLBACK_RC="${STUB_ROLLBACK_RC:-0}" \
    STUB_ROLLBACK_HEALTH_RC="${STUB_ROLLBACK_HEALTH_RC:-0}" \
    STUB_ROLLBACK_HEALTH_BODY="${STUB_ROLLBACK_HEALTH_BODY:-}" \
    STUB_ROLLBACK_BUILD_BODY="${STUB_ROLLBACK_BUILD_BODY:-}" \
    STUB_CONTAINER_STATUS="${STUB_CONTAINER_STATUS:-running}" \
    STUB_RESTART_COUNT="${STUB_RESTART_COUNT:-0}" \
    STUB_CONTAINER_LOGS="${STUB_CONTAINER_LOGS:-}" \
    STUB_STATUS_TSV="${STUB_STATUS_TSV:-}" \
    STUB_BUMP_SERVICE="${STUB_BUMP_SERVICE:-}" \
    STUB_BUMP_ON_CALL="${STUB_BUMP_ON_CALL:-2}" \
    STUB_BUMP_RESTARTS="${STUB_BUMP_RESTARTS:-1}" \
    STUB_BUMP_STATUS="${STUB_BUMP_STATUS:-}" \
    STUB_INSPECT_STATE="$work/inspect-counts.tsv" \
    STUB_ABSENT_REFS="${STUB_ABSENT_REFS:-}" \
    PANTRY_DIR="$pantry" \
    PANTRY_SETUP_SKIP_ROOT=1 \
    PANTRY_TEST_PROJECT_PREFIX="${PANTRY_TEST_PROJECT_PREFIX:-}" \
    PANTRY_UPDATE_HEALTH_TIMEOUT="${PANTRY_UPDATE_HEALTH_TIMEOUT:-0}" \
    PANTRY_UPDATE_HEALTH_INTERVAL="${PANTRY_UPDATE_HEALTH_INTERVAL:-1}" \
    PANTRY_UPDATE_MIGRATION_TIMEOUT="${PANTRY_UPDATE_MIGRATION_TIMEOUT:-600}" \
    PANTRY_UPDATE_SETTLE_SECONDS="${PANTRY_UPDATE_SETTLE_SECONDS:-0}" \
    PANTRY_UPDATE_MIN_FREE_KIB="${PANTRY_UPDATE_MIN_FREE_KIB:-65536}" \
    PATH="$bin:$PATH" \
    bash "$UPDATE" >"$out" 2>"$err"
  LAST_RC=$?
  set -e

  LAST_OUT=$(cat "$out" "$err")
  LAST_DOCKER=$(cat "$work/docker.log")
  LAST_CURL=$(cat "$work/curl.log")
  LAST_SEQUENCE=$(cat "$work/sequence.log")
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

line_of() {
  local haystack="$1" needle="$2"
  printf '%s\n' "$haystack" | grep -n -F -m1 -- "$needle" | cut -d: -f1
}

env_unchanged() {
  local pantry="$1"
  cmp -s "$pantry/.env" "$pantry/.env.orig" || fail "update rewrote .env"
}

NEW_COMMIT=0123456789abcdef0123456789abcdef01234567
OLD_COMMIT=ffffffffffffffffffffffffffffffffffffffff
MID_COMMIT=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa

# Healthy new image: digest recorded before pull, health and commit match,
# previous digest is not re-pinned, .env is left alone.
healthy=$(mktemp -d)
prepare_pantry "$healthy"
write_images "$healthy/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:newcaddy caddy@sha256:newcaddy
STUB_HEALTH_RC=0
STUB_BUILD_RC=0
STUB_HEALTH_BODY='{"status":"ok","scanner":{"connected":false}}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\",\"version\":\"vtest\"}"
run_update "$healthy"
[[ "$LAST_RC" -eq 0 ]] || fail "healthy update exited $LAST_RC: $LAST_OUT"
have "$LAST_OUT" "pantry update: running pantry image ghcr.io/rhionin/pantry@sha256:oldpantry" "healthy run records the pantry digest"
have "$LAST_OUT" "pantry update: running caddy image caddy@sha256:oldcaddy" "healthy run records the caddy digest"
have "$LAST_OUT" "pantry update: healthy, running commit ${NEW_COMMIT}" "healthy run keeps the matching commit"
have "$LAST_OUT" "pantry update: finished" "healthy run finishes"
lack "$LAST_OUT" "PANTRY UPDATE FAILED" "healthy run does not roll back"
lack "$LAST_DOCKER" "--pull never" "healthy run does not re-pin"
have "$LAST_CURL" "http://127.0.0.1:8080/health" "healthy run polls the host port health endpoint"
have "$LAST_CURL" "http://127.0.0.1:8080/api/build" "healthy run polls /api/build"
lack "$LAST_CURL" "stockbot.rhionin.com" "healthy run does not use the public hostname"
lack "$LAST_DOCKER" "rm -f pantry-caddy" "a fixture pantry dir does not remove pantry-caddy"
lack "$LAST_DOCKER" "rm -f pantry-cloudflared" "a fixture pantry dir does not remove pantry-cloudflared"
env_unchanged "$healthy"
[[ ! -e "$healthy/.update-rejected-digest" ]] || fail "healthy run recorded a rejected digest"
record_line=$(line_of "$LAST_DOCKER" "sha256:runpantry")
pull_line=$(line_of "$LAST_DOCKER" " pull")
up_line=$(line_of "$LAST_SEQUENCE" " up -d")
curl_line=$(line_of "$LAST_SEQUENCE" "curl ")
[[ -n "$record_line" && -n "$pull_line" && "$record_line" -lt "$pull_line" ]] || fail "healthy run inspected the running image after pull (record=$record_line pull=$pull_line)"
[[ -n "$up_line" && -n "$curl_line" && "$up_line" -lt "$curl_line" ]] || fail "healthy run polled health before up -d"
unset STUB_HEALTH_BODY STUB_BUILD_BODY STUB_HEALTH_RC STUB_BUILD_RC

# Nothing changed on pull: no recreate and no health poll.
noop=$(mktemp -d)
prepare_pantry "$noop"
write_images "$noop/images.tsv" \
  sha256:runpantry ghcr.io/rhionin/pantry@sha256:oldpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
STUB_HEALTH_RC=0
STUB_BUILD_RC=0
STUB_HEALTH_BODY='{"status":"ok"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\"}"
run_update "$noop"
[[ "$LAST_RC" -eq 0 ]] || fail "unchanged update exited $LAST_RC: $LAST_OUT"
have "$LAST_OUT" "pantry update: images unchanged" "unchanged pull is a no-op"
have "$LAST_OUT" "pantry update: finished" "unchanged pull finishes"
have "$LAST_DOCKER" " pull" "unchanged pull still checks the registry"
lack "$LAST_DOCKER" " up " "unchanged pull does not recreate containers"
[[ -z "$LAST_CURL" ]] || fail "unchanged pull polled health: $LAST_CURL"
lack "$LAST_OUT" "PANTRY UPDATE FAILED" "unchanged pull does not roll back"
env_unchanged "$noop"
[[ ! -e "$noop/.update-rejected-digest" ]] || fail "unchanged pull recorded a rejected digest"
unset STUB_HEALTH_BODY STUB_BUILD_BODY

# Health is ok but /api/build is a different commit: roll back, exit non-zero,
# and do not retry that digest on the next run.
mismatch=$(mktemp -d)
prepare_pantry "$mismatch"
write_images "$mismatch/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:newcaddy caddy@sha256:newcaddy
STUB_HEALTH_RC=0
STUB_BUILD_RC=0
STUB_HEALTH_BODY='{"status":"ok","scanner":{"connected":false}}'
STUB_BUILD_BODY="{\"commit\":\"${OLD_COMMIT}\",\"version\":\"vtest\"}"
run_update "$mismatch"
[[ "$LAST_RC" -ne 0 ]] || fail "mismatched sha should exit non-zero: $LAST_OUT"
have "$LAST_OUT" "PANTRY UPDATE FAILED: rolling back to the previous image." "mismatch rolls back loudly"
have "$LAST_OUT" "does not undo a database migration" "mismatch says an image rollback cannot undo a migration"
have "$LAST_OUT" "not transactional" "mismatch names the migration runner"
have "$LAST_OUT" "pantry-pre-*.db" "mismatch names the snapshot files"
have "$LAST_OUT" "/data" "mismatch names the snapshot directory"
have "$LAST_OUT" "does not restore the database" "mismatch does not imply a database restore"
have "$LAST_OUT" "pantry update: re-pinning pantry to ghcr.io/rhionin/pantry@sha256:oldpantry" "mismatch re-pins the pantry digest"
have "$LAST_OUT" "pantry update: re-pinning caddy to caddy@sha256:oldcaddy" "mismatch re-pins the caddy digest"
have "$LAST_DOCKER" "--pull never" "mismatch starts the previous image without pulling"
have "$LAST_DOCKER" "image: ghcr.io/rhionin/pantry@sha256:oldpantry" "mismatch override pins the pantry digest"
have "$LAST_DOCKER" "image: caddy@sha256:oldcaddy" "mismatch override pins the caddy digest"
lack "$LAST_OUT" "pantry update: finished" "mismatch does not report success"
have "$LAST_DOCKER" "tag ghcr.io/rhionin/pantry@sha256:oldpantry ghcr.io/rhionin/pantry:latest" "rollback retags the pantry ref to the good digest"
have "$LAST_DOCKER" "tag caddy@sha256:oldcaddy caddy:2.11.7-alpine" "rollback retags a changed sidecar ref"
have "$(cat "$mismatch/tag-state.tsv")" "ghcr.io/rhionin/pantry:latest	sha256:runpantry	ghcr.io/rhionin/pantry@sha256:oldpantry" "local pantry tag resolves to the good digest"
have "$(cat "$mismatch/tag-state.tsv")" "caddy:2.11.7-alpine	sha256:runcaddy	caddy@sha256:oldcaddy" "local caddy tag resolves to the good digest"
env_unchanged "$mismatch"
have "$(cat "$mismatch/.update-rejected-digest")" "ghcr.io/rhionin/pantry@sha256:newpantry" "mismatch remembers the rejected digest"
have "$(cat "$mismatch/.update-rejected-digest")" "Not a setup last-known-good sha" "rejected file is not the setup sha"
lack "$(cat "$mismatch/.update-rejected-digest")" "PANTRY_IMAGE_TAG" "rejected file does not pin the tag"
forward_up=$(line_of "$LAST_SEQUENCE" " up -d")
rollback_up=$(line_of "$LAST_SEQUENCE" "--pull never")
probed=$(line_of "$LAST_SEQUENCE" "curl ")
[[ -n "$forward_up" && -n "$probed" && "$forward_up" -lt "$probed" ]] || fail "mismatch polled health before the new image was started"
[[ -n "$probed" && -n "$rollback_up" && "$probed" -lt "$rollback_up" ]] || fail "mismatch re-pinned before the health check failed"
checked_again=$(printf '%s\n' "$LAST_SEQUENCE" | awk -v n="$rollback_up" 'NR > n && /curl / { print NR; exit }')
[[ -n "$checked_again" ]] || fail "rollback was not health-checked"

# The next minute pulls the same bad remote digest, does not start it, and
# leaves the local tag on the good digest. The restored tag must not make
# this look like "images unchanged". A digest that was already rejected
# exits 0 with one line instead of failing every minute.
run_update "$mismatch"
[[ "$LAST_RC" -eq 0 ]] || fail "rejected digest should exit 0: $LAST_OUT"
have "$LAST_OUT" "skipping rejected ghcr.io/rhionin/pantry@sha256:newpantry" "second run skips the rejected digest"
lack "$LAST_OUT" "PANTRY UPDATE FAILED" "second run does not repeat the failure banner"
lack "$LAST_OUT" "was already rolled back" "second run does not use the old failure banner"
have "$LAST_DOCKER" " pull" "second run still pulls the remote digest"
lack "$LAST_OUT" "images unchanged" "retagged local tag is not treated as the remote image"
lack "$LAST_DOCKER" " up " "second run does not recreate containers"
lack "$LAST_DOCKER" "--pull never" "second run does not roll back again"
[[ -z "$LAST_CURL" ]] || fail "second run polled health: $LAST_CURL"
have "$(cat "$mismatch/tag-state.tsv")" "ghcr.io/rhionin/pantry:latest	sha256:runpantry	ghcr.io/rhionin/pantry@sha256:oldpantry" "second run puts the local tag back on the good digest"
env_unchanged "$mismatch"

# A newer remote digest is not stuck behind the retag.
write_images "$mismatch/images.tsv" \
  sha256:newerpantry ghcr.io/rhionin/pantry@sha256:newerpantry \
  sha256:newercaddy caddy@sha256:newercaddy
STUB_HEALTH_BODY='{"status":"ok"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\",\"version\":\"vtest\"}"
run_update "$mismatch"
[[ "$LAST_RC" -eq 0 ]] || fail "new digest after a rollback exited $LAST_RC: $LAST_OUT"
have "$LAST_OUT" "pantry update: healthy, running commit ${NEW_COMMIT}" "new digest is deployed"
have "$LAST_DOCKER" " up -d" "new digest recreates the stack"
lack "$LAST_OUT" "images unchanged" "new digest is not skipped as unchanged"
lack "$LAST_OUT" "was already rolled back" "new digest is not the rejected one"
have "$(cat "$mismatch/tag-state.tsv")" "ghcr.io/rhionin/pantry:latest	sha256:newerpantry	ghcr.io/rhionin/pantry@sha256:newerpantry" "new digest keeps the local tag"
env_unchanged "$mismatch"
unset STUB_HEALTH_BODY STUB_BUILD_BODY STUB_HEALTH_RC STUB_BUILD_RC

# The new image never answers /health.
unhealthy=$(mktemp -d)
prepare_pantry "$unhealthy"
write_images "$unhealthy/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
STUB_HEALTH_RC=1
STUB_BUILD_RC=1
STUB_HEALTH_BODY=""
STUB_BUILD_BODY=""
run_update "$unhealthy"
[[ "$LAST_RC" -ne 0 ]] || fail "unhealthy image should exit non-zero: $LAST_OUT"
have "$LAST_OUT" "PANTRY UPDATE FAILED: rolling back to the previous image." "unhealthy image rolls back loudly"
have "$LAST_OUT" "does not undo a database migration" "unhealthy rollback says a migration stays applied"
have "$LAST_OUT" "not transactional" "unhealthy rollback names the migration runner"
have "$LAST_OUT" "pantry-pre-*.db" "unhealthy rollback names the snapshot files"
have "$LAST_OUT" "does not restore the database" "unhealthy rollback does not restore the database"
have "$LAST_OUT" "pantry update: re-pinning pantry to ghcr.io/rhionin/pantry@sha256:oldpantry" "unhealthy rollback re-pins pantry"
have "$LAST_DOCKER" "tag ghcr.io/rhionin/pantry@sha256:oldpantry ghcr.io/rhionin/pantry:latest" "unhealthy rollback retags pantry"
lack "$LAST_DOCKER" "tag caddy@" "unchanged sidecar tag is left alone"
have "$LAST_DOCKER" "--pull never" "unhealthy rollback starts the previous image"
have "$LAST_DOCKER" "image: ghcr.io/rhionin/pantry@sha256:oldpantry" "unhealthy override pins the pantry digest"
lack "$LAST_OUT" "pantry update: finished" "unhealthy rollback does not report success"
env_unchanged "$unhealthy"
unset STUB_HEALTH_BODY STUB_BUILD_BODY STUB_HEALTH_RC STUB_BUILD_RC

# A caddy-only change that fails to start must not reject the pantry digest
# that was already running.
caddy_only=$(mktemp -d)
prepare_pantry "$caddy_only"
write_images "$caddy_only/images.tsv" \
  sha256:runpantry ghcr.io/rhionin/pantry@sha256:oldpantry \
  sha256:newcaddy caddy@sha256:newcaddy
STUB_UP_RC=1
run_update "$caddy_only"
[[ "$LAST_RC" -ne 0 ]] || fail "caddy-only up failure should exit non-zero: $LAST_OUT"
have "$(cat "$caddy_only/.update-rejected-digest")" "caddy@sha256:newcaddy" "caddy-only failure rejects the caddy digest"
lack "$(cat "$caddy_only/.update-rejected-digest")" "oldpantry" "caddy-only failure does not reject the running pantry digest"
lack "$(cat "$caddy_only/.update-rejected-digest")" "newpantry" "caddy-only failure does not invent a pantry digest"
unset STUB_UP_RC

# Rollback that does not start records nothing and says both images are down.
rollback_fail=$(mktemp -d)
prepare_pantry "$rollback_fail"
write_images "$rollback_fail/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:newcaddy caddy@sha256:newcaddy
STUB_HEALTH_RC=1
STUB_BUILD_RC=1
STUB_ROLLBACK_RC=1
run_update "$rollback_fail"
[[ "$LAST_RC" -ne 0 ]] || fail "failed rollback should exit non-zero: $LAST_OUT"
have "$LAST_OUT" "BOTH IMAGES DOWN" "failed rollback names both images"
[[ ! -e "$rollback_fail/.update-rejected-digest" ]] || fail "failed rollback recorded a digest: $(cat "$rollback_fail/.update-rejected-digest")"
unset STUB_HEALTH_RC STUB_BUILD_RC STUB_ROLLBACK_RC

# A pantry image with no revision label is never started.
unlabeled=$(mktemp -d)
prepare_pantry "$unlabeled"
write_images "$unlabeled/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
awk -F '\t' 'BEGIN { OFS = "\t" } $1 == "pantry" { $7 = ""; $8 = "" } { print }' \
  "$unlabeled/images.tsv" > "$unlabeled/images.tsv.next"
mv "$unlabeled/images.tsv.next" "$unlabeled/images.tsv"
run_update "$unlabeled"
[[ "$LAST_RC" -ne 0 ]] || fail "missing label should exit non-zero: $LAST_OUT"
lack "$LAST_SEQUENCE" " up -d" "missing label must not run compose up"
lack "$LAST_SEQUENCE" "--pull never" "missing label must not start a rollback"
have "$LAST_OUT" "not starting it" "missing label says the image was not started"
have "$(cat "$unlabeled/.update-rejected-digest")" "ghcr.io/rhionin/pantry@sha256:newpantry" "missing label rejects the new pantry digest"

# Health becomes ok on the third poll. Timeout 0 would stop after the first.
polled=$(mktemp -d)
prepare_pantry "$polled"
write_images "$polled/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:newcaddy caddy@sha256:newcaddy
printf '%s\n' '{"status":"starting"}' '{"status":"starting"}' > "$polled/health.queue"
STUB_HEALTH_QUEUE="$polled/health.queue"
STUB_HEALTH_BODY='{"status":"ok"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\"}"
PANTRY_UPDATE_HEALTH_TIMEOUT=30
PANTRY_UPDATE_HEALTH_INTERVAL=0
PANTRY_TEST_PROJECT_PREFIX=fixture
run_update "$polled"
[[ "$LAST_RC" -eq 0 ]] || fail "health that recovers should exit 0: $LAST_OUT"
have "$LAST_OUT" "pantry update: healthy, running commit ${NEW_COMMIT}" "recovered health is kept"
lack "$LAST_OUT" "PANTRY UPDATE FAILED" "recovered health does not roll back"
health_polls=$(printf '%s\n' "$LAST_CURL" | grep -c '/health' || true)
[[ "$health_polls" -ge 3 ]] || fail "health loop ran ${health_polls} polls, want at least 3"
have "$LAST_DOCKER" "rm -f fixture-cloudflared" "test project prefix derives the cloudflared name"
lack "$LAST_DOCKER" "rm -f pantry-cloudflared" "test project prefix does not remove pantry-cloudflared"
unset STUB_HEALTH_QUEUE STUB_HEALTH_BODY STUB_BUILD_BODY PANTRY_UPDATE_HEALTH_TIMEOUT PANTRY_UPDATE_HEALTH_INTERVAL PANTRY_TEST_PROJECT_PREFIX

# /health reports migrating twice, then ok. Timeout 0 must not give up on the first 503.
migrating=$(mktemp -d)
prepare_pantry "$migrating"
write_images "$migrating/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
printf '%s\n' '{"status":"migrating"}' '{"status":"migrating"}' > "$migrating/health.queue"
STUB_HEALTH_QUEUE="$migrating/health.queue"
STUB_HEALTH_BODY='{"status":"ok"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\"}"
PANTRY_UPDATE_HEALTH_TIMEOUT=0
PANTRY_UPDATE_HEALTH_INTERVAL=0
run_update "$migrating"
[[ "$LAST_RC" -eq 0 ]] || fail "migrating health should exit 0: $LAST_OUT"
have "$LAST_OUT" "migrations still running" "migrating status keeps the wait going"
have "$LAST_OUT" "pantry update: healthy, running commit ${NEW_COMMIT}" "migrating wait then succeeds"
lack "$LAST_DOCKER" "--pull never" "migrating wait does not roll back"
unset STUB_HEALTH_QUEUE STUB_HEALTH_BODY STUB_BUILD_BODY PANTRY_UPDATE_HEALTH_TIMEOUT PANTRY_UPDATE_HEALTH_INTERVAL

# A pending marker means the previous run died after up. Images look unchanged,
# but the health check still runs and a failure rolls back to the saved pin.
pending=$(mktemp -d)
prepare_pantry "$pending"
write_images "$pending/images.tsv" \
  sha256:runpantry ghcr.io/rhionin/pantry@sha256:oldpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
printf '%s\n' $'pantry\tghcr.io/rhionin/pantry@sha256:previous\tghcr.io/rhionin/pantry@sha256:oldpantry' > "$pending/.update-pending"
STUB_HEALTH_RC=1
STUB_BUILD_RC=1
run_update "$pending"
[[ "$LAST_RC" -ne 0 ]] || fail "pending unhealthy deploy should exit non-zero: $LAST_OUT"
lack "$LAST_OUT" "images unchanged" "pending marker is not treated as a no-op"
have "$LAST_DOCKER" "image: ghcr.io/rhionin/pantry@sha256:previous" "pending failure re-pins the saved digest"
have "$LAST_OUT" "previous deploy was not confirmed" "pending marker is resumed"
unset STUB_HEALTH_RC STUB_BUILD_RC

# An exited container fails the forward wait without sitting out the timeout.
exited=$(mktemp -d)
prepare_pantry "$exited"
write_images "$exited/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
STUB_CONTAINER_STATUS=exited
STUB_HEALTH_BODY='{"status":"ok"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\"}"
PANTRY_UPDATE_HEALTH_TIMEOUT=30
PANTRY_UPDATE_HEALTH_INTERVAL=0
run_update "$exited"
[[ "$LAST_RC" -ne 0 ]] || fail "exited container should exit non-zero: $LAST_OUT"
have "$LAST_OUT" "pantry container is exited" "exited container fails fast"
have "$LAST_DOCKER" "--pull never" "exited container rolls back"
unset STUB_CONTAINER_STATUS STUB_HEALTH_BODY STUB_BUILD_BODY PANTRY_UPDATE_HEALTH_TIMEOUT PANTRY_UPDATE_HEALTH_INTERVAL

# The public proxy is checked through Caddy after the LAN health check.
proxied=$(mktemp -d)
prepare_pantry "$proxied"
printf '%s\n' 'basic_auth bcrypt Pantry {' '}' > "$proxied/auth.caddy"
cat > "$proxied/.env" << 'EOF'
HOST_PORT=8080
PUBLIC_HOST=stockbot.example
PANTRY_IMAGE_TAG=latest
PANTRY_SESSION_SECRET=test-secret
VAPID_PUBLIC_KEY=test-public
VAPID_PRIVATE_KEY=test-private
VAPID_SUBJECT=mailto:pantry@localhost
EOF
cp "$proxied/.env" "$proxied/.env.orig"
write_images "$proxied/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:newcaddy caddy@sha256:newcaddy
STUB_HEALTH_BODY='{"status":"ok"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\"}"
run_update "$proxied"
[[ "$LAST_RC" -eq 0 ]] || fail "caddy probe should exit 0: $LAST_OUT"
have "$LAST_CURL" "--resolve stockbot.example:443:127.0.0.1" "caddy probe uses --resolve"
have "$LAST_CURL" "https://stockbot.example/api/build" "caddy probe requests /api/build"
env_unchanged "$proxied"
unset STUB_HEALTH_BODY STUB_BUILD_BODY

# Not enough free space skips the deploy before any container is recreated.
full_disk=$(mktemp -d)
prepare_pantry "$full_disk"
write_images "$full_disk/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:newcaddy caddy@sha256:newcaddy
PANTRY_UPDATE_MIN_FREE_KIB=999999999999
run_update "$full_disk"
[[ "$LAST_RC" -ne 0 ]] || fail "low disk should exit non-zero: $LAST_OUT"
have "$LAST_OUT" "KiB free" "low disk names the free space"
lack "$LAST_SEQUENCE" " up -d" "low disk does not start containers"
unset PANTRY_UPDATE_MIN_FREE_KIB

# Pantry and grocy-sync both change. grocy-sync exits after a healthy pantry
# check, so only the sidecar digest is rejected.
grocy=$(mktemp -d)
prepare_pantry "$grocy"
write_images "$grocy/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
printf '%s\n' $'grocy-sync\tghcr.io/rhionin/pantry-grocy-sync:latest\tsha256:rungrocy\tghcr.io/rhionin/pantry-grocy-sync@sha256:oldgrocy\tsha256:newgrocy\tghcr.io/rhionin/pantry-grocy-sync@sha256:newgrocy\t' >> "$grocy/images.tsv"
printf '%s\n' $'grocy-sync\texited\t0' > "$grocy/status.tsv"
STUB_STATUS_TSV="$grocy/status.tsv"
STUB_HEALTH_BODY='{"status":"ok"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\"}"
run_update "$grocy"
[[ "$LAST_RC" -ne 0 ]] || fail "grocy-sync exit should fail the deploy: $LAST_OUT"
have "$LAST_OUT" "grocy-sync is not running" "grocy-sync exit is reported"
have "$LAST_OUT" "pantry update: healthy, running commit ${NEW_COMMIT}" "pantry passed health before the sidecar check"
have "$(cat "$grocy/.update-rejected-digest")" $'grocy-sync\tghcr.io/rhionin/pantry-grocy-sync@sha256:newgrocy' "grocy-sync exit rejects the grocy digest"
lack "$(cat "$grocy/.update-rejected-digest")" "newpantry" "grocy-sync exit does not reject the healthy pantry digest"
unset STUB_STATUS_TSV STUB_HEALTH_BODY STUB_BUILD_BODY

# compose up fails because grocy-sync is not running. Pantry's health check
# still passed, so its digest is not rejected either.
grocy_up=$(mktemp -d)
prepare_pantry "$grocy_up"
write_images "$grocy_up/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
printf '%s\n' $'grocy-sync\tghcr.io/rhionin/pantry-grocy-sync:latest\tsha256:rungrocy\tghcr.io/rhionin/pantry-grocy-sync@sha256:oldgrocy\tsha256:newgrocy\tghcr.io/rhionin/pantry-grocy-sync@sha256:newgrocy\t' >> "$grocy_up/images.tsv"
printf '%s\n' $'grocy-sync\texited\t0' > "$grocy_up/status.tsv"
STUB_STATUS_TSV="$grocy_up/status.tsv"
STUB_UP_RC=1
STUB_HEALTH_BODY='{"status":"ok"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\"}"
run_update "$grocy_up"
[[ "$LAST_RC" -ne 0 ]] || fail "grocy-sync up failure should fail the deploy: $LAST_OUT"
have "$LAST_OUT" "docker compose up failed" "grocy-sync up failure is reported"
have "$(cat "$grocy_up/.update-rejected-digest")" "newgrocy" "grocy-sync up failure rejects the grocy digest"
lack "$(cat "$grocy_up/.update-rejected-digest")" "newpantry" "grocy-sync up failure does not reject the healthy pantry digest"
unset STUB_STATUS_TSV STUB_UP_RC STUB_HEALTH_BODY STUB_BUILD_BODY

# A sidecar that is running at first and then crash-loops is caught on the
# settle re-check. Pantry already passed health, so only caddy is rejected.
crash=$(mktemp -d)
prepare_pantry "$crash"
write_images "$crash/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:newcaddy caddy@sha256:newcaddy
STUB_BUMP_SERVICE=caddy
STUB_BUMP_ON_CALL=2
STUB_BUMP_RESTARTS=1
STUB_HEALTH_BODY='{"status":"ok"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\"}"
run_update "$crash"
[[ "$LAST_RC" -ne 0 ]] || fail "crash-loop should fail the deploy: $LAST_OUT"
have "$LAST_OUT" "caddy restarted (0 -> 1)" "settle re-check notices the restart"
have "$(cat "$crash/.update-rejected-digest")" $'caddy\tcaddy@sha256:newcaddy' "crash-loop rejects the caddy digest"
lack "$(cat "$crash/.update-rejected-digest")" "newpantry" "crash-loop does not reject the healthy pantry digest"
unset STUB_BUMP_SERVICE STUB_BUMP_ON_CALL STUB_BUMP_RESTARTS STUB_HEALTH_BODY STUB_BUILD_BODY

# A newer release is pulled before the pending deploy is resumed. Health is
# compared to the pending digest's label, not the newest image's label.
resumed=$(mktemp -d)
prepare_pantry "$resumed"
cat > "$resumed/images.tsv" << EOF
pantry	ghcr.io/rhionin/pantry:latest	sha256:runmid	ghcr.io/rhionin/pantry@sha256:midpantry	sha256:newpantry	ghcr.io/rhionin/pantry@sha256:newpantry	${NEW_COMMIT}	${MID_COMMIT}
caddy	caddy:2.11.7-alpine	sha256:runcaddy	caddy@sha256:oldcaddy	sha256:runcaddy	caddy@sha256:oldcaddy	
EOF
printf '%s\n' $'pantry\tghcr.io/rhionin/pantry@sha256:oldpantry\tghcr.io/rhionin/pantry@sha256:midpantry' > "$resumed/.update-pending"
printf '%s\n' "{\"commit\":\"${MID_COMMIT}\"}" "{\"commit\":\"${NEW_COMMIT}\"}" > "$resumed/build.queue"
STUB_BUILD_QUEUE="$resumed/build.queue"
STUB_HEALTH_BODY='{"status":"ok"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\"}"
run_update "$resumed"
[[ "$LAST_RC" -eq 0 ]] || fail "resume against the pending digest exited $LAST_RC: $LAST_OUT"
have "$LAST_OUT" "previous deploy was not confirmed" "resume notices the pending marker"
have "$LAST_OUT" "waiting for /health and /api/build commit ${MID_COMMIT}" "resume waits for the pending digest's label"
have "$LAST_OUT" "pending deploy is healthy" "resume keeps the pending digest when its build matches"
have "$LAST_OUT" "waiting for /health and /api/build commit ${NEW_COMMIT}" "the newer release is deployed after the resume"
lack "$LAST_OUT" "re-pinning pantry to ghcr.io/rhionin/pantry@sha256:oldpantry" "resume does not roll back to the pre-pending image"
lack "$LAST_DOCKER" "--pull never" "resume does not start a rollback"
first_wait=$(printf '%s\n' "$LAST_OUT" | grep -m1 'waiting for /health')
have "$first_wait" "$MID_COMMIT" "the first health check is the pending digest"
lack "$first_wait" "$NEW_COMMIT" "the first health check is not the newer release"
unset STUB_BUILD_QUEUE STUB_HEALTH_BODY STUB_BUILD_BODY

# The pending digest is gone locally. Skip the resume instead of rolling back.
gone=$(mktemp -d)
prepare_pantry "$gone"
cat > "$gone/images.tsv" << EOF
pantry	ghcr.io/rhionin/pantry:latest	sha256:runmid	ghcr.io/rhionin/pantry@sha256:midpantry	sha256:runmid	ghcr.io/rhionin/pantry@sha256:midpantry	${MID_COMMIT}
caddy	caddy:2.11.7-alpine	sha256:runcaddy	caddy@sha256:oldcaddy	sha256:runcaddy	caddy@sha256:oldcaddy	
EOF
printf '%s\n' $'pantry\tghcr.io/rhionin/pantry@sha256:oldpantry\tghcr.io/rhionin/pantry@sha256:midpantry' > "$gone/.update-pending"
STUB_ABSENT_REFS='ghcr.io/rhionin/pantry@sha256:midpantry'
run_update "$gone"
[[ "$LAST_RC" -eq 0 ]] || fail "missing pending digest exited $LAST_RC: $LAST_OUT"
have "$LAST_OUT" "not on this machine; skipping the resume check" "a missing pending digest is skipped"
have "$LAST_OUT" "images unchanged" "a missing pending digest leaves an unchanged stack alone"
lack "$LAST_OUT" "PANTRY UPDATE FAILED" "a missing pending digest does not fail the run"
lack "$LAST_OUT" "rolling back" "a missing pending digest does not roll back"
lack "$LAST_DOCKER" "--pull never" "a missing pending digest does not start a rollback"
unset STUB_ABSENT_REFS

# /health degraded, even with the expected build commit, is not healthy.
degraded=$(mktemp -d)
prepare_pantry "$degraded"
write_images "$degraded/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
STUB_HEALTH_BODY='{"status":"degraded"}'
STUB_BUILD_BODY="{\"commit\":\"${NEW_COMMIT}\"}"
run_update "$degraded"
[[ "$LAST_RC" -ne 0 ]] || fail "degraded health should exit non-zero: $LAST_OUT"
have "$LAST_OUT" "PANTRY UPDATE FAILED: rolling back to the previous image." "degraded health rolls back"
have "$(cat "$degraded/.update-rejected-digest")" "ghcr.io/rhionin/pantry@sha256:newpantry" "degraded health rejects the new pantry digest"
lack "$LAST_OUT" "healthy, running commit" "degraded health is not treated as ok"
unset STUB_HEALTH_BODY STUB_BUILD_BODY

# The banner names a snapshot this deploy wrote, not an older "already exists" path.
snap=$(mktemp -d)
prepare_pantry "$snap"
write_images "$snap/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
STUB_HEALTH_RC=1
STUB_BUILD_RC=1
STUB_CONTAINER_LOGS=$'shopping backup already exists: /data/pantry-pre-groups-20200101T000000Z.db\nwrote shopping backup: /data/pantry-pre-groups-20261017T150405Z.db'
run_update "$snap"
[[ "$LAST_RC" -ne 0 ]] || fail "snapshot rollback should exit non-zero: $LAST_OUT"
have "$LAST_OUT" "Pre-migration snapshot written by this deploy: /data/pantry-pre-groups-20261017T150405Z.db" "banner names the snapshot this deploy wrote"
lack "$LAST_OUT" "pantry-pre-groups-20200101T000000Z.db" "banner does not name the already-exists snapshot"
lack "$LAST_OUT" "no new snapshot found" "a written snapshot is not reported missing"

# Only an "already exists" line: say so, and do not print that older path.
snap_old=$(mktemp -d)
prepare_pantry "$snap_old"
write_images "$snap_old/images.tsv" \
  sha256:newpantry ghcr.io/rhionin/pantry@sha256:newpantry \
  sha256:runcaddy caddy@sha256:oldcaddy
STUB_CONTAINER_LOGS='shopping backup already exists: /data/pantry-pre-groups-20200101T000000Z.db'
run_update "$snap_old"
[[ "$LAST_RC" -ne 0 ]] || fail "old snapshot rollback should exit non-zero: $LAST_OUT"
have "$LAST_OUT" "no new snapshot found" "banner says when this deploy wrote no snapshot"
lack "$LAST_OUT" "pantry-pre-groups-20200101T000000Z.db" "banner does not name an already-exists snapshot"
unset STUB_HEALTH_RC STUB_BUILD_RC STUB_CONTAINER_LOGS

rm -rf "$healthy" "$noop" "$mismatch" "$unhealthy" "$caddy_only" "$rollback_fail" "$unlabeled" "$polled" "$migrating" "$pending" "$exited" "$proxied" "$full_disk" "$grocy" "$grocy_up" "$crash" "$resumed" "$gone" "$degraded" "$snap" "$snap_old"
echo "pantry_update_test ok"
