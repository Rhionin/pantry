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

  # Copy all files except .env
  log_info "Copying deployment files to /opt/pantry..."
  for item in docker-compose.yml .env.example udev systemd; do
    if [[ -e "$SCRIPT_DIR/$item" ]]; then
      cp -r "$SCRIPT_DIR/$item" /opt/pantry/
    fi
  done
  cp "$SCRIPT_DIR/setup.sh" /opt/pantry/setup.sh
  chmod +x /opt/pantry/setup.sh
  log_success "Deployment files copied"

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
  local image_tag="" host_port="" scanner_device=""
  if [[ -f /opt/pantry/.env ]]; then
    image_tag=$(grep "^PANTRY_IMAGE_TAG=" /opt/pantry/.env | cut -d= -f2 || echo "latest")
    host_port=$(grep "^HOST_PORT=" /opt/pantry/.env | cut -d= -f2 || echo "8080")
    scanner_device=$(grep "^SCANNER_DEVICE=" /opt/pantry/.env | cut -d= -f2 || echo "/dev/input/pantry-scanner")
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

  echo ""
  if [[ $failed -eq 0 ]]; then
    log_success "All checks passed"
  else
    log_error "$failed check(s) failed"
    exit 1
  fi
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
  rule             Regenerate and install udev rule for current scanner
  status           Diagnose the deployment chain and report issues
  logs             Follow container logs (Ctrl+C to stop)
  freeze           Mask pantry-update.timer to prevent automatic updates during iteration
  thaw             Unmask pantry-update.timer to resume automatic updates
  help             Show this message (default if no command given)

EXAMPLES:
  # Initial setup
  sudo ./setup.sh install

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
