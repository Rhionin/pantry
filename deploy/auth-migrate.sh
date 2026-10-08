#!/usr/bin/env bash
# Copy the existing Caddy bcrypt hash into the file Pantry reads, and keep
# one session secret. Sourced by setup.sh and pantry-update.sh.
# auth.caddy is never rewritten. A secret that is already set is never rotated,
# so a redeploy does not sign phones out or require a new password.

# pantry_runtime_uid is the distroless nonroot account. The runtime image is
# gcr.io/distroless/static-debian12:nonroot and Dockerfile sets
# USER nonroot:nonroot. That account is uid 65532. The pantry process drops
# every capability, so a root-owned 0700 directory is unreadable and the
# public site fails closed.
pantry_runtime_uid=65532

# give_auth_to_pantry matches prepare_deploy_trigger_dir: a failed chown stops
# setup, except the no-sudo test seam, which warns and continues.
give_auth_to_pantry() {
  local path="$1"
  if chown "${pantry_runtime_uid}:${pantry_runtime_uid}" "$path"; then
    return 0
  fi
  if [[ "${PANTRY_SETUP_SKIP_ROOT:-}" == 1 ]]; then
    echo "[warn] Could not give ${path} to uid ${pantry_runtime_uid}" >&2
    return 0
  fi
  echo "Could not give ${path} to uid ${pantry_runtime_uid}" >&2
  return 1
}

sync_household_credential() {
  local dir dest tmp
  if [[ -z "${PANTRY_DIR:-}" ]]; then
    echo "PANTRY_DIR is not set" >&2
    return 1
  fi
  dir="${PANTRY_DIR}/auth"
  dest="${dir}/household"
  mkdir -p "$dir" || return 1
  chmod 700 "$dir" || return 1
  # Every run, including one that finds a root-owned file from a partial setup.
  give_auth_to_pantry "$dir" || return 1
  if [[ -f "${PANTRY_DIR}/auth.caddy" ]]; then
    tmp=$(mktemp "${dir}/household.XXXXXX") || return 1
    if ! cp "${PANTRY_DIR}/auth.caddy" "$tmp"; then
      rm -f "$tmp"
      return 1
    fi
    chmod 600 "$tmp" || { rm -f "$tmp"; return 1; }
    mv "$tmp" "$dest" || { rm -f "$tmp"; return 1; }
  fi
  if [[ -f "$dest" ]]; then
    chmod 600 "$dest" || return 1
    give_auth_to_pantry "$dest" || return 1
  fi
}

ensure_session_secret() {
  local env current secret tmp
  if [[ -z "${PANTRY_DIR:-}" ]]; then
    echo "PANTRY_DIR is not set" >&2
    return 1
  fi
  env="${PANTRY_DIR}/.env"
  if [[ ! -f "$env" ]]; then
    echo "No .env to store the session secret" >&2
    return 1
  fi
  current=$({ grep '^PANTRY_SESSION_SECRET=' "$env" || true; } | head -1 | cut -d= -f2- | tr -d '[:space:]')
  if [[ -n "$current" ]]; then
    return 0
  fi
  secret=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
  if [[ ${#secret} -ne 64 ]]; then
    echo "Could not generate a session secret" >&2
    return 1
  fi
  if grep -q '^PANTRY_SESSION_SECRET=' "$env"; then
    tmp=$(mktemp "${env}.XXXXXX") || return 1
    if ! sed "s|^PANTRY_SESSION_SECRET=.*|PANTRY_SESSION_SECRET=${secret}|" "$env" > "$tmp"; then
      rm -f "$tmp"
      return 1
    fi
    chmod 600 "$tmp" || { rm -f "$tmp"; return 1; }
    mv "$tmp" "$env" || { rm -f "$tmp"; return 1; }
  else
    printf '\nPANTRY_SESSION_SECRET=%s\n' "$secret" >> "$env" || return 1
  fi
  chmod 600 "$env"
}
