#!/usr/bin/env bash

set -euo pipefail

# Pantry Deployment Setup Script
# 
# This script configures a Raspberry Pi to run Pantry in a Docker container
# with hot-pluggable barcode scanner support. It is designed for iteration:
# run it multiple times as you make changes, and it will converge to the
# desired state without losing configuration.
#
# Usage:
#   sudo ./setup.sh install          # One-time setup: Docker, udev, container
#   sudo ./setup.sh install --with-updates  # ...and enable auto-update timer
#   sudo ./setup.sh publish          # HTTPS on PUBLIC_HOST (Let's Encrypt via Caddy)
#   sudo ./setup.sh unpublish        # Stop the public proxy; LAN pantry keeps running
#   sudo ./setup.sh firewall         # Drop non-LAN clients that reach the Pantry port
#   sudo ./setup.sh firewall-off     # Remove that restriction
#   sudo ./setup.sh rule             # Regenerate udev rule for a new scanner
#   sudo ./setup.sh status           # Diagnose the full chain from udev to health
#   sudo ./setup.sh logs             # Follow container logs
#   sudo ./setup.sh freeze           # Mask pantry-update.timer during iteration
#   sudo ./setup.sh thaw             # Unmask pantry-update.timer when done
#   sudo ./setup.sh help             # Show this message
#
# Default: help

# Color codes for output (TTY detection)
if [[ -t 1 ]]; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  BLUE='\033[0;36m'
  NC='\033[0m'
else
  RED=''
  GREEN=''
  YELLOW=''
  BLUE=''
  NC=''
fi

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Helpers
log_info() {
  if [[ -t 1 ]]; then
    echo -e "${BLUE}[info]${NC} $*"
  else
    echo "[info] $*"
  fi
}

log_success() {
  if [[ -t 1 ]]; then
    echo -e "${GREEN}[✓]${NC} $*"
  else
    echo "[✓] $*"
  fi
}

log_warn() {
  if [[ -t 1 ]]; then
    echo -e "${YELLOW}[warn]${NC} $*"
  else
    echo "[warn] $*"
  fi
}

log_error() {
  if [[ -t 1 ]]; then
    echo -e "${RED}[error]${NC} $*" >&2
  else
    echo "[error] $*" >&2
  fi
}

fatal() {
  log_error "$@"
  exit 1
}

# Verify running as root
require_root() {
  if [[ $EUID -ne 0 ]]; then
    fatal "This script must be run as root (use sudo)"
  fi
}

# Check if a command exists
command_exists() {
  command -v "$1" &> /dev/null
}

# env_value prints KEY's value from /opt/pantry/.env, without surrounding whitespace.
# Missing keys print nothing. Callers decide whether an empty value is an error.
env_value() {
  { grep "^${1}=" /opt/pantry/.env || true; } | head -1 | cut -d= -f2- | tr -d '[:space:]'
}

# env_value_keep_spaces is env_value for a passphrase. Internal spaces stay.
# Only a trailing CR and whitespace at the ends are removed.
env_value_keep_spaces() {
  local line
  line=$({ grep "^${1}=" /opt/pantry/.env || true; } | head -1)
  line=${line#*=}
  line=${line%$'\r'}
  line="${line#"${line%%[![:space:]]*}"}"
  line="${line%"${line##*[![:space:]]}"}"
  printf '%s' "$line"
}

# write_auth_caddy stores the bcrypt hash Caddy checks on the public site.
# The plaintext password never goes in this file. A literal bcrypt hash is
# required: doubling the dollar signs makes the password stop matching.
write_auth_caddy() {
  local user="$1" hash="$2" tmp
  if [[ -d /opt/pantry/auth.caddy ]]; then
    fatal "/opt/pantry/auth.caddy is a directory. Docker creates one when the file is missing. Remove it and re-run 'sudo ./setup.sh publish'"
  fi
  tmp=$(mktemp /opt/pantry/auth.caddy.XXXXXX)
  # Realm is a directive argument. Inside the block, a line is a username and hash.
  printf 'basic_auth bcrypt Pantry {\n\t%s %s\n}\n' "$user" "$hash" > "$tmp"
  chmod 600 "$tmp"
  mv "$tmp" /opt/pantry/auth.caddy
}

# copy_deploy_files refreshes /opt/pantry from this script's directory.
# .env is never copied, so a re-run cannot clobber operator settings.
# Copying a file onto itself (running the already-installed script) is skipped.
copy_deploy_files() {
  log_info "Copying deployment files to /opt/pantry..."
  mkdir -p /opt/pantry
  local item src dest
  for item in docker-compose.yml .env.example Caddyfile udev systemd firewall; do
    src="$SCRIPT_DIR/$item"
    dest="/opt/pantry/$item"
    if [[ ! -e "$src" ]]; then
      continue
    fi
    if [[ "$src" == "$dest" ]]; then
      continue
    fi
    cp -r "$src" /opt/pantry/
  done
  if [[ "$SCRIPT_DIR/setup.sh" != "/opt/pantry/setup.sh" ]]; then
    cp "$SCRIPT_DIR/setup.sh" /opt/pantry/setup.sh
  fi
  chmod +x /opt/pantry/setup.sh
  if [[ -f /opt/pantry/systemd/pantry-update.sh ]]; then
    chmod +x /opt/pantry/systemd/pantry-update.sh
  fi
  if [[ -f /opt/pantry/firewall/pantry-lan-only.sh ]]; then
    chmod +x /opt/pantry/firewall/pantry-lan-only.sh
  fi
  log_success "Deployment files copied"
}

# ============================================================================
# install: Full deployment setup
# ============================================================================
cmd_install() {
  require_root

  # Parse flags. --with-updates opts into automatic updates; by default the
  # update timer is left disabled so iteration is not disrupted mid-experiment.
  local with_updates=false
  shift || true  # drop the "install" subcommand
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --with-updates) with_updates=true ;;
      *) log_warn "Ignoring unknown install option: $1" ;;
    esac
    shift
  done

  log_info "Starting Pantry deployment setup..."

  # Verify Docker and compose plugin exist; install if missing
  if ! command_exists docker; then
    log_warn "Docker not found, installing..."
    if ! curl -fsSL https://get.docker.com | sh; then
      fatal "Failed to install Docker"
    fi
    log_success "Docker installed"
  else
    log_success "Docker already installed"
  fi

  if ! docker compose version &> /dev/null; then
    fatal "Docker Compose plugin not found after Docker installation"
  fi
  log_success "Docker Compose plugin available"

  # Add current user to docker group
  local current_user="${SUDO_USER:-$USER}"
  if [[ -z "$current_user" ]]; then
    log_warn "Could not determine current user, skipping docker group add"
  else
    if id -nG "$current_user" | grep -qw docker; then
      log_success "User $current_user already in docker group"
    else
      usermod -aG docker "$current_user"
      log_success "Added $current_user to docker group (you may need to log out and back in)"
    fi
  fi

  # Copy deploy tree
  if [[ ! -d /opt/pantry ]]; then
    mkdir -p /opt/pantry
    log_success "Created /opt/pantry"
  fi

  copy_deploy_files

  # Reconcile .env
  cmd_reconcile_env

  # Ensure /dev/input exists
  if [[ ! -d /dev/input ]]; then
    log_warn "/dev/input does not exist, creating it (scanner nodes will not appear inside container otherwise)"
    mkdir -p /dev/input
  else
    log_success "/dev/input exists"
  fi

  # Generate udev rule
  cmd_rule

  # Start container
  log_info "Starting Pantry container..."
  if ! cd /opt/pantry && docker compose up -d; then
    fatal "Failed to start Pantry container"
  fi
  log_success "Container started"

  # Poll for health
  log_info "Waiting for Pantry to become healthy..."
  local deadline=$((SECONDS + 60))
  local host_port
  host_port=$(grep "^HOST_PORT=" /opt/pantry/.env | cut -d= -f2)
  host_port=${host_port:-8080}

  while [[ $SECONDS -lt $deadline ]]; do
    if curl -sf "http://localhost:$host_port/health" > /dev/null 2>&1; then
      log_success "Pantry is healthy"
      break
    fi
    sleep 2
  done

  if [[ $SECONDS -ge $deadline ]]; then
    log_error "Pantry failed to become healthy within 60 seconds"
    log_error "Recent logs:"
    docker compose -f /opt/pantry/docker-compose.yml logs --tail=50 pantry || true
    fatal "Health check timeout"
  fi

  # Automatic updates: opt-in only. Left disabled by default so an update does
  # not recreate the container mid-iteration.
  if [[ "$with_updates" == true ]]; then
    log_info "Installing automatic-update systemd units (--with-updates)..."
    if [[ -f /opt/pantry/systemd/pantry-update.service ]] && [[ -f /opt/pantry/systemd/pantry-update.timer ]]; then
      cp /opt/pantry/systemd/pantry-update.service /etc/systemd/system/
      cp /opt/pantry/systemd/pantry-update.timer /etc/systemd/system/
      systemctl daemon-reload
      systemctl enable --now pantry-update.timer
      log_success "pantry-update.timer enabled (updates will run automatically)"
    else
      log_warn "systemd unit files not found under /opt/pantry/systemd; skipping update timer"
    fi
  else
    log_info "Automatic updates NOT enabled (pass --with-updates to enable)"
  fi

  log_success "Pantry deployment setup complete"
  log_info "Next: sudo ./setup.sh status"
}

# ============================================================================
# reconcile_env: Idempotent .env management
# ============================================================================
cmd_reconcile_env() {
  log_info "Reconciling /opt/pantry/.env..."

  # Create from example if absent
  if [[ ! -f /opt/pantry/.env ]]; then
    cp /opt/pantry/.env.example /opt/pantry/.env
    log_success "Created /opt/pantry/.env from .env.example"
    return
  fi

  # Migrate the obsolete default SCANNER_DEVICE path. A .env written by a
  # pre-hotplug version pinned SCANNER_DEVICE=/dev/pantry-scanner. That was
  # never an operator choice — it was the old default — and the container can
  # no longer open it, since the symlink now lives under /dev/input. Rewrite
  # only that exact obsolete default; a custom path an operator set on purpose
  # (anything other than the old default) is left untouched.
  if grep -q '^SCANNER_DEVICE=/dev/pantry-scanner$' /opt/pantry/.env; then
    sed -i 's|^SCANNER_DEVICE=/dev/pantry-scanner$|SCANNER_DEVICE=/dev/input/pantry-scanner|' /opt/pantry/.env
    log_warn "Migrated obsolete SCANNER_DEVICE=/dev/pantry-scanner -> /dev/input/pantry-scanner"
  fi

  # Append missing keys from .env.example
  local added=0
  while IFS='=' read -r key value; do
    # Skip comments and empty lines
    [[ "$key" =~ ^# ]] && continue
    [[ -z "$key" ]] && continue

    # Check if key exists in .env
    if ! grep -q "^${key}=" /opt/pantry/.env; then
      echo "${key}=${value}" >> /opt/pantry/.env
      log_info "Added missing key: $key"
      added=$((added + 1))
    fi
  done < /opt/pantry/.env.example

  if [[ $added -gt 0 ]]; then
    log_success "Added $added missing keys to .env"
  else
    log_success ".env is up to date"
  fi
}

# ============================================================================
# rule: Generate and install udev rule
# ============================================================================
cmd_rule() {
  require_root
  log_info "Generating udev rule for barcode scanner..."

  # Find candidates from /proc/bus/input/devices. Each device block has an
  # "I:" line (Bus/Vendor/Product) and an "N: Name=..." line. We capture both
  # so a USB scanner can be matched on vendor/product (robust) rather than on
  # its name string alone.
  local candidate_names=() candidate_vids=() candidate_pids=()
  if [[ -f /proc/bus/input/devices ]]; then
    local cur_vid="" cur_pid=""
    while IFS= read -r line; do
      if [[ "$line" =~ ^I:\ Bus= ]]; then
        # e.g. "I: Bus=0003 Vendor=05e0 Product=1200 Version=0100"
        cur_vid=$(printf '%s\n' "$line" | sed -n 's/.*Vendor=\([0-9a-fA-F]*\).*/\1/p')
        cur_pid=$(printf '%s\n' "$line" | sed -n 's/.*Product=\([0-9a-fA-F]*\).*/\1/p')
      elif [[ "$line" =~ ^N:\ Name= ]]; then
        local name="${line#*Name=\"}"
        name="${name%\"*}"
        # Filter for likely scanner devices (avoid generic HID keyboards).
        # nocasematch makes the =~ test case-insensitive so "Scanner",
        # "BARCODE", etc. all match; it is restored immediately after.
        shopt -s nocasematch
        if [[ "$name" =~ (scanner|barcode) ]]; then
          candidate_names+=("$name")
          candidate_vids+=("$cur_vid")
          candidate_pids+=("$cur_pid")
        fi
        shopt -u nocasematch
      fi
    done < /proc/bus/input/devices
  fi

  # Show candidates or use first
  if [[ ${#candidate_names[@]} -eq 0 ]]; then
    log_warn "No scanner candidates found in /proc/bus/input/devices"
    log_info "Devices available:"
    grep "^N: " /proc/bus/input/devices || true
    fatal "Please manually create the udev rule at /etc/udev/rules.d/99-pantry-scanner.rules"
  fi

  if [[ ${#candidate_names[@]} -gt 1 ]]; then
    log_warn "Multiple scanner candidates found. Using the first:"
    log_info "${candidate_names[0]}"
  else
    log_success "Found scanner: ${candidate_names[0]}"
  fi

  local scanner_name="${candidate_names[0]}"
  local scanner_vid="${candidate_vids[0]}"
  local scanner_pid="${candidate_pids[0]}"

  # Prefer matching on vendor/product IDs when available (robust, not
  # order-sensitive). udev's input_id builtin lowercases these into
  # ENV{ID_VENDOR_ID}/ENV{ID_MODEL_ID}. Fall back to the device name for
  # Bluetooth or when IDs are absent.
  local rule_content
  if [[ -n "$scanner_vid" && -n "$scanner_pid" ]]; then
    local vid_lc pid_lc
    vid_lc=$(printf '%s' "$scanner_vid" | tr '[:upper:]' '[:lower:]')
    pid_lc=$(printf '%s' "$scanner_pid" | tr '[:upper:]' '[:lower:]')
    log_info "Matching on vendor=$vid_lc product=$pid_lc"
    rule_content="# Pantry Scanner udev rule - auto-generated for \"$scanner_name\"
SUBSYSTEM==\"input\", KERNEL==\"event*\", ENV{ID_VENDOR_ID}==\"$vid_lc\", ENV{ID_MODEL_ID}==\"$pid_lc\", \\
  SYMLINK+=\"input/pantry-scanner\", GROUP=\"65532\", MODE=\"0640\"
"
  else
    log_info "No vendor/product IDs found; matching on device name"
    rule_content="# Pantry Scanner udev rule - auto-generated
SUBSYSTEM==\"input\", KERNEL==\"event*\", ATTRS{name}==\"$scanner_name\", \\
  SYMLINK+=\"input/pantry-scanner\", GROUP=\"65532\", MODE=\"0640\"
"
  fi

  local rule_file="/etc/udev/rules.d/99-pantry-scanner.rules"

  # Only write if the installed content differs from what we would generate.
  # Comparing the whole content (not just the match key) means an outdated
  # rule — e.g. one from a pre-hotplug version whose SYMLINK was still
  # "pantry-scanner" instead of "input/pantry-scanner" — is correctly
  # rewritten on re-run.
  if [[ ! -f "$rule_file" ]] || [[ "$(cat "$rule_file")" != "$rule_content" ]]; then
    printf '%s' "$rule_content" > "$rule_file"
    log_success "Wrote $rule_file"
  else
    log_success "$rule_file already up to date"
  fi

  # Reload and trigger udev
  udevadm control --reload
  udevadm trigger --subsystem-match=input --action=add
  log_success "udev rules reloaded and triggered"
}

# ============================================================================
# status: Diagnostic chain
# ============================================================================
cmd_status() {
  require_root
  log_info "Diagnosing Pantry deployment chain..."
  echo ""

  local failed=0

  # 1. Deployment files
  if [[ -d /opt/pantry ]]; then
    log_success "Deployment directory exists"
  else
    log_error "Deployment directory /opt/pantry missing"
    failed=$((failed + 1))
  fi

  # 2. .env file
  local image_tag="" host_port="" scanner_device="" public_host=""
  if [[ -f /opt/pantry/.env ]]; then
    image_tag=$(grep "^PANTRY_IMAGE_TAG=" /opt/pantry/.env | cut -d= -f2 || echo "latest")
    host_port=$(grep "^HOST_PORT=" /opt/pantry/.env | cut -d= -f2 || echo "8080")
    scanner_device=$(grep "^SCANNER_DEVICE=" /opt/pantry/.env | cut -d= -f2 || echo "/dev/input/pantry-scanner")
    public_host=$(env_value PUBLIC_HOST)
    log_success ".env present: PANTRY_IMAGE_TAG=$image_tag, HOST_PORT=$host_port, SCANNER_DEVICE=$scanner_device"
  else
    log_error ".env not found at /opt/pantry/.env"
    failed=$((failed + 1))
  fi

  # 3. /dev/input exists
  if [[ -d /dev/input ]]; then
    local fs_type
    fs_type=$(stat -f /dev/input -c "%T" 2>/dev/null || echo "unknown")
    if [[ "$fs_type" == "tmpfs" ]] || [[ "$fs_type" == "devtmpfs" ]]; then
      log_success "/dev/input exists on devtmpfs"
    else
      log_warn "/dev/input exists but is on $fs_type (expect devtmpfs)"
    fi
  else
    log_error "/dev/input does not exist"
    failed=$((failed + 1))
  fi

  # 4. udev rule exists
  if [[ -f /etc/udev/rules.d/99-pantry-scanner.rules ]]; then
    if grep -q "SYMLINK.*input/pantry-scanner" /etc/udev/rules.d/99-pantry-scanner.rules; then
      log_success "udev rule installed with symlink at /dev/input/pantry-scanner"
    else
      log_error "udev rule exists but does not set symlink to /dev/input/pantry-scanner"
      failed=$((failed + 1))
    fi
  else
    log_warn "udev rule not installed at /etc/udev/rules.d/99-pantry-scanner.rules"
  fi

  # 5. Scanner device resolves to a real node, and 6. that node is group
  #    65532 / mode 0640. A missing scanner is a SUPPORTED state, not a
  #    failure — the container still runs and /health still returns ok — so
  #    it is reported as PASS-with-note. What operators care about is the
  #    other case: a scanner PRESENT but unreadable.
  if [[ -n "${scanner_device:-}" ]]; then
    # readlink -f succeeds on a non-existent path (it just echoes it back),
    # so test for actual existence with -e, not readlink's exit status.
    if [[ -e "$scanner_device" ]]; then
      local resolved
      resolved=$(readlink -f "$scanner_device")
      log_success "Scanner device resolves: $scanner_device -> $resolved"

      # 6. Group and mode on the resolved node.
      local node_group node_mode
      node_group=$(stat -c "%g" "$resolved" 2>/dev/null || echo "?")
      node_mode=$(stat -c "%a" "$resolved" 2>/dev/null || echo "?")
      if [[ "$node_group" == "65532" && "$node_mode" == "640" ]]; then
        log_success "Scanner node is group 65532 mode 0640 (readable by the container)"
      else
        log_error "Scanner node is group $node_group mode $node_mode, expected group 65532 mode 0640 — scanner is PRESENT but the container cannot read it (EACCES). Fix GROUP in the udev rule or SCANNER_GID in .env, then: sudo ./setup.sh rule"
        failed=$((failed + 1))
      fi
    else
      log_warn "Scanner device $scanner_device does not exist — scanner absent (SUPPORTED: container runs and /health is ok). If a scanner IS attached, the udev rule did not match (ENOENT): fix its match keys and run 'sudo ./setup.sh rule'"
    fi
  fi

  # 6. Container running
  if docker compose -f /opt/pantry/docker-compose.yml ps 2>/dev/null | grep -q "pantry.*Up"; then
    log_success "Pantry container is running"
  else
    log_error "Pantry container is not running"
    failed=$((failed + 1))
  fi

  # 7. Health check
  if [[ -n "${host_port:-}" ]]; then
    if curl -sf "http://localhost:$host_port/health" | grep -q '"status":"ok"'; then
      log_success "GET /health returns ok"
    else
      log_error "GET /health failed or timed out"
      failed=$((failed + 1))
    fi
  fi

  # 8. Public proxy. Empty PUBLIC_HOST is the LAN-only default, not a failure.
  if [[ -n "$public_host" ]]; then
    if [[ ! -f /opt/pantry/auth.caddy ]]; then
      log_warn "PUBLIC_HOST=$public_host but /opt/pantry/auth.caddy is missing, so the proxy will not start. Run: sudo ./setup.sh publish"
    elif docker inspect -f '{{.State.Running}}' pantry-caddy 2>/dev/null | grep -qx true; then
      log_success "Public proxy is running for https://$public_host (shared password required)"
    else
      log_warn "PUBLIC_HOST=$public_host but the proxy container is not running. Start it with: sudo ./setup.sh publish"
    fi
  else
    log_info "Public internet access is not configured (PUBLIC_HOST is empty)"
  fi

  echo ""
  if [[ $failed -eq 0 ]]; then
    log_success "All checks passed"
  else
    log_error "$failed check(s) failed"
    exit 1
  fi
}

# ============================================================================
# publish: HTTPS reverse proxy for PUBLIC_HOST
# ============================================================================
cmd_publish() {
  require_root

  if [[ ! -f /opt/pantry/.env ]]; then
    fatal "No /opt/pantry/.env yet. Run 'sudo ./setup.sh install', set PUBLIC_HOST, ACME_EMAIL, and BASIC_AUTH_PASSWORD, then re-run 'sudo ./setup.sh publish'"
  fi

  copy_deploy_files
  cmd_reconcile_env

  if [[ -d /opt/pantry/Caddyfile ]]; then
    fatal "/opt/pantry/Caddyfile is a directory. Docker creates one when the file is missing on first start. Remove it and re-run 'sudo ./setup.sh publish'"
  fi
  if [[ ! -f /opt/pantry/Caddyfile ]]; then
    fatal "Caddyfile is missing from /opt/pantry"
  fi

  local public_host acme_email
  public_host=$(env_value PUBLIC_HOST)
  acme_email=$(env_value ACME_EMAIL)

  if [[ -z "$public_host" ]]; then
    fatal "Set PUBLIC_HOST in /opt/pantry/.env to a hostname such as pantry.rhionin.com (no https://), then re-run 'sudo ./setup.sh publish'"
  fi
  if [[ ! "$public_host" =~ ^[A-Za-z0-9.-]+$ ]] || [[ "$public_host" != *.* ]] || [[ "$public_host" == .* ]] || [[ "$public_host" == *. ]]; then
    fatal "PUBLIC_HOST must be a hostname such as pantry.rhionin.com, without a scheme or path (got: $public_host)"
  fi
  if [[ -z "$acme_email" ]]; then
    fatal "Set ACME_EMAIL in /opt/pantry/.env to an email address for Let's Encrypt expiry notices, then re-run 'sudo ./setup.sh publish'"
  fi
  if [[ ! "$acme_email" =~ ^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$ ]]; then
    fatal "ACME_EMAIL must be an email address (got: $acme_email)"
  fi

  local auth_user auth_password auth_hash
  auth_user=$(env_value BASIC_AUTH_USER)
  auth_password=$(env_value_keep_spaces BASIC_AUTH_PASSWORD)
  if [[ -z "$auth_user" ]]; then
    auth_user=pantry
  fi
  if [[ ! "$auth_user" =~ ^[A-Za-z][A-Za-z0-9._-]{0,63}$ ]]; then
    fatal "BASIC_AUTH_USER must be letters, digits, dots, underscores, or hyphens (got: $auth_user)"
  fi
  if [[ -z "$auth_password" ]]; then
    fatal "Set BASIC_AUTH_PASSWORD in /opt/pantry/.env to a shared password of 12 to 72 characters, then re-run 'sudo ./setup.sh publish'"
  fi
  if [[ ${#auth_password} -lt 12 || ${#auth_password} -gt 72 ]]; then
    fatal "BASIC_AUTH_PASSWORD must be 12 to 72 characters"
  fi

  # Hash on stdin so the password is not a docker argument. Caddy requires
  # the trailing newline as a separator and does not treat it as part of the password.
  log_info "Hashing the shared password"
  if ! auth_hash=$(printf '%s\n' "$auth_password" | docker run --rm -i caddy:2.11.4-alpine caddy hash-password); then
    fatal "Could not hash the shared password. Docker must be able to run caddy:2.11.4-alpine."
  fi
  auth_hash=${auth_hash//$'\r'/}
  auth_hash=${auth_hash//$'\n'/}
  if [[ ! "$auth_hash" =~ ^\$2[aby]\$ ]]; then
    fatal "Caddy did not return a bcrypt password hash. The public proxy was not started."
  fi
  unset auth_password
  write_auth_caddy "$auth_user" "$auth_hash"
  unset auth_hash
  chmod 600 /opt/pantry/.env /opt/pantry/auth.caddy
  log_success "Wrote /opt/pantry/auth.caddy and restricted .env to the owner"

  log_info "Starting the public HTTPS proxy for https://$public_host"
  if ! docker compose --project-directory /opt/pantry -f /opt/pantry/docker-compose.yml --profile public up -d; then
    fatal "Failed to start the public proxy"
  fi

  local deadline=$((SECONDS + 20))
  local running=false
  while [[ $SECONDS -lt $deadline ]]; do
    if docker inspect -f '{{.State.Running}}' pantry-caddy 2>/dev/null | grep -qx true; then
      running=true
      break
    fi
    sleep 1
  done
  if [[ "$running" != true ]]; then
    log_error "The proxy container exited. Recent logs:"
    docker compose --project-directory /opt/pantry -f /opt/pantry/docker-compose.yml --profile public logs --tail=80 caddy || true
    fatal "Public proxy did not stay running"
  fi

  # An already-installed update timer must include the public profile, or the
  # next unattended pull recreates Pantry without refreshing the proxy.
  if [[ -f /etc/systemd/system/pantry-update.service ]]; then
    cp /opt/pantry/systemd/pantry-update.service /etc/systemd/system/pantry-update.service
    systemctl daemon-reload
    log_success "Refreshed pantry-update.service so automatic updates keep the public proxy"
  fi

  local host_port
  host_port=$(env_value HOST_PORT)
  host_port=${host_port:-8080}

  log_success "Public proxy is running"
  log_info "Browsers will ask for user $auth_user and the shared password in /opt/pantry/.env."
  log_info "LAN access does not ask for that password: http://<pi-address>:$host_port"
  log_warn "Do not forward port $host_port on the router. It has no password."
  log_info "Optional: sudo ./setup.sh firewall   # reject non-LAN clients that still reach port $host_port"
  log_info "After DNS and router port forwards are in place, check from outside the house:"
  log_info "  curl -fsS -u '$auth_user:<password>' https://$public_host/health"
}

# ============================================================================
# unpublish: stop HTTPS proxy, leave the LAN service running
# ============================================================================
cmd_unpublish() {
  require_root
  if [[ ! -f /opt/pantry/docker-compose.yml ]]; then
    fatal "Nothing installed at /opt/pantry. Run 'sudo ./setup.sh install' first"
  fi
  log_info "Stopping the public HTTPS proxy. Pantry keeps running on the LAN."
  docker compose --project-directory /opt/pantry -f /opt/pantry/docker-compose.yml --profile public stop caddy || true
  docker compose --project-directory /opt/pantry -f /opt/pantry/docker-compose.yml --profile public rm -f caddy || true
  log_success "Public proxy stopped. The certificate volume was kept so a later publish can reuse it."
  log_info "Clear PUBLIC_HOST in /opt/pantry/.env if automatic updates should not start the proxy again."
}

# ============================================================================
# firewall: reject non-LAN clients on the published Pantry port
# ============================================================================
cmd_firewall() {
  require_root
  if [[ ! -f /opt/pantry/docker-compose.yml ]]; then
    fatal "Nothing installed at /opt/pantry. Run 'sudo ./setup.sh install' first"
  fi
  copy_deploy_files
  if [[ ! -f /opt/pantry/firewall/pantry-lan-only.sh ]]; then
    fatal "firewall/pantry-lan-only.sh is missing from /opt/pantry"
  fi
  chmod +x /opt/pantry/firewall/pantry-lan-only.sh
  if [[ -f /opt/pantry/systemd/pantry-lan-only.service ]]; then
    cp /opt/pantry/systemd/pantry-lan-only.service /etc/systemd/system/pantry-lan-only.service
    systemctl daemon-reload
    systemctl enable --now pantry-lan-only.service
  else
    /opt/pantry/firewall/pantry-lan-only.sh
  fi
  log_success "Published Pantry port accepts LAN, loopback, and Tailscale sources"
  log_info "Ports 80 and 443 are unchanged. Remove this with: sudo ./setup.sh firewall-off"
}

cmd_firewall_off() {
  require_root
  if [[ -f /opt/pantry/firewall/pantry-lan-only.sh ]]; then
    /opt/pantry/firewall/pantry-lan-only.sh --remove || true
  fi
  if [[ -f /etc/systemd/system/pantry-lan-only.service ]]; then
    systemctl disable --now pantry-lan-only.service || true
    rm -f /etc/systemd/system/pantry-lan-only.service
    systemctl daemon-reload
  fi
  log_success "Removed the LAN-only rule. The published port is reachable from any source again."
}

# ============================================================================
# logs: Follow container logs
# ============================================================================
cmd_logs() {
  require_root
  log_info "Following Pantry container logs (Ctrl+C to stop)..."
  docker compose -f /opt/pantry/docker-compose.yml logs --tail=100 -f pantry || true
}

# ============================================================================
# freeze: Mask the update timer
# ============================================================================
cmd_freeze() {
  require_root
  if [[ -f /etc/systemd/system/pantry-update.timer ]]; then
    if systemctl is-enabled pantry-update.timer &>/dev/null; then
      systemctl mask pantry-update.timer
      log_success "Masked pantry-update.timer (updates frozen during iteration)"
    else
      log_success "pantry-update.timer already masked or disabled"
    fi
  else
    log_warn "pantry-update.timer not installed"
  fi
}

# ============================================================================
# thaw: Unmask the update timer
# ============================================================================
cmd_thaw() {
  require_root
  if [[ -f /etc/systemd/system/pantry-update.timer ]]; then
    if systemctl is-enabled pantry-update.timer &>/dev/null; then
      log_success "pantry-update.timer already enabled"
    else
      systemctl unmask pantry-update.timer
      log_success "Unmasked pantry-update.timer (updates will resume)"
    fi
  else
    log_warn "pantry-update.timer not installed"
  fi
}

# ============================================================================
# help: Show usage
# ============================================================================
cmd_help() {
  cat << 'EOF'
Pantry Deployment Setup Script

USAGE:
  sudo ./setup.sh [COMMAND] [OPTIONS]

COMMANDS:
  install          One-time setup: Docker, udev, container, health check
                   Pass --with-updates to also enable the automatic-update timer
                   (disabled by default). The timer fires every 5 minutes, pulls
                   'latest', and recreates the container, so leave it off while
                   iterating.
  publish          Serve PUBLIC_HOST over HTTPS with Let's Encrypt (Caddy).
                   Requires PUBLIC_HOST, ACME_EMAIL, and BASIC_AUTH_PASSWORD
                   in /opt/pantry/.env. The public site asks for that shared
                   password. See deploy/README.md.
  unpublish        Stop the HTTPS proxy. Pantry keeps running on the LAN.
  firewall         Drop non-LAN clients that reach the published Pantry port.
                   LAN, loopback, and Tailscale (100.64.0.0/10) still work.
                   Does not change ports 80 or 443. See deploy/README.md.
  firewall-off     Remove that restriction.
  rule             Regenerate and install udev rule for current scanner
  status           Diagnose the deployment chain and report issues
  logs             Follow container logs (Ctrl+C to stop)
  freeze           Mask pantry-update.timer to prevent automatic updates during iteration
  thaw             Unmask pantry-update.timer to resume automatic updates
  help             Show this message (default if no command given)

EXAMPLES:
  # Initial setup
  sudo ./setup.sh install

  # After DNS and port forwards: HTTPS on the hostname in PUBLIC_HOST
  sudo ./setup.sh publish

  # Check status after setup or troubleshooting
  sudo ./setup.sh status

  # Iterate on configuration
  sudo ./setup.sh freeze          # Pause auto-updates
  # ... make changes to /opt/pantry/.env ...
  sudo docker compose -f /opt/pantry/docker-compose.yml up -d
  sudo ./setup.sh status
  sudo ./setup.sh thaw            # Resume auto-updates

NOTES:
  - The container is distroless, so 'docker exec pantry sh' does not work.
  - Use 'sudo ./setup.sh logs' and GET /health for runtime introspection.
  - Run install multiple times—it is idempotent. Existing .env values are preserved.
  - PANTRY_IMAGE_TAG and HOST_PORT are never overwritten if already set.
EOF
}

# ============================================================================
# Main dispatch
# ============================================================================
main() {
  local cmd="${1:-help}"

  case "$cmd" in
    install)  cmd_install "$@" ;;
    publish)  cmd_publish ;;
    unpublish) cmd_unpublish ;;
    firewall) cmd_firewall ;;
    firewall-off) cmd_firewall_off ;;
    rule)     cmd_rule ;;
    status)   cmd_status ;;
    logs)     cmd_logs ;;
    freeze)   cmd_freeze ;;
    thaw)     cmd_thaw ;;
    help|--help|-h) cmd_help ;;
    *)        fatal "Unknown command: $cmd (try 'help')" ;;
  esac
}

main "$@"
