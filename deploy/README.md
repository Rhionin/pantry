# Pantry Deployment Guide

This guide covers deploying Pantry on a Raspberry Pi using Docker Compose.

## Quick Start

New to this? Follow these steps in order on your Raspberry Pi and you'll have Pantry running with automatic updates. Each step says what it does. For options, backups, and troubleshooting, use the reference sections further down.

1. **What you need.** A Raspberry Pi running 64-bit Raspberry Pi OS, and the ability to open a terminal. (See [Supported Platform](#supported-platform) below for exact hardware and OS details.)

2. **Install Docker.** Docker is the engine that runs Pantry. Run:

   ```bash
   curl -fsSL https://get.docker.com | sh
   sudo usermod -aG docker $USER
   ```

   Then **log out and log back in** so the second command takes effect.

3. **Get the Pantry files onto the Pi.** The deployment files live in this repository's `deploy/` folder: `docker-compose.yml`, `.env.example`, and the `systemd/` folder. Copy that whole folder into `/opt/pantry` on your Pi (for example, clone the repo with `git` and copy the `deploy/` contents, or copy them over with a USB drive or `scp`). Use exactly `/opt/pantry`, because the automatic-update files expect that path.

4. **Create the config file.** This holds your settings; the defaults work fine to get started, and you can edit it later (see the [Configuration](#configuration) table). Run:

   ```bash
   cd /opt/pantry
   sudo cp .env.example .env
   ```

5. **Install the scanner udev rule.** Copy the provided rule file to `/etc/udev/rules.d/` and edit it with your scanner's identifiers (see [Headless Scanner Input](#headless-scanner-input) for details). This also enables automatic start/stop of the container when the scanner connects or disconnects.

6. **Install the pantry.service unit.** This systemd unit owns the container's lifecycle and starts it automatically when the scanner is detected:

   ```bash
   sudo cp /opt/pantry/systemd/pantry.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```
   
   Do **not** run `systemctl enable` on pantry.service — it's activated purely by udev device events, not at boot.

7. **Start Pantry.** With the scanner connected, run:

   ```bash
   sudo udevadm trigger --action=add
   ```
   
   This fires the udev add event and starts the container automatically via pantry.service. You can also power-cycle the scanner to trigger the same behavior.

8. **Check it works.** Run this on the Pi. It should print `{"status":"ok"}`:

   ```bash
   curl http://localhost:8080/health
   ```

   You can also open `http://<pi-ip-address>:8080` in a web browser on any device on the same network.

7. **Turn on automatic updates.** This copies in two small helper files and switches on a timer that checks for and installs new versions of Pantry for you:

   ```bash
   sudo cp /opt/pantry/systemd/pantry-update.service /etc/systemd/system/
   sudo cp /opt/pantry/systemd/pantry-update.timer /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now pantry-update.timer
   ```

   Once this is on, every new version installs automatically with no approval step, and the updater runs as root because it controls Docker. See [⚠️ Important Considerations](#️-important-considerations) below for the full picture. Don't turn this on if you've pinned Pantry to a specific version (see [Pinning to Specific Versions](#pinning-to-specific-versions)), because the updater would keep looking for updates it should not apply.

That's it. Pantry will now keep itself up to date.

The rest of this guide is reference material: configuration options, backups, the barcode scanner, rolling back, and troubleshooting.

## Supported Platform

**Target:** 64-bit Raspberry Pi OS on arm64 hardware

Pantry is packaged as a `linux/arm64` container image and requires 64-bit Raspberry Pi OS running on compatible hardware (Raspberry Pi 3B+, 4, 5, or newer models with arm64 architecture).

## First-Time Setup

### 1. Install Docker and Docker Compose

Install Docker and the Compose plugin on your Raspberry Pi:

```bash
# Update system packages
sudo apt update && sudo apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com | sh

# Add your user to the docker group (requires logout/login to take effect)
sudo usermod -aG docker $USER

# Install Docker Compose plugin (if not already included)
sudo apt install docker-compose-plugin

# Verify installation
docker --version
docker compose version
```

Log out and back in for the group membership to take effect.

### 2. Set Up Deployment Files

Copy the deployment files to your Pi:

```bash
# Create deployment directory
sudo mkdir -p /opt/pantry

# Copy the entire deploy/ directory contents from this repository to /opt/pantry
# This includes: docker-compose.yml, .env.example, systemd/, and udev/

# Navigate to deployment directory
cd /opt/pantry

# Copy and customize environment file
sudo cp .env.example .env
sudo nano .env  # Edit configuration as needed
```

### 4. **Install the scanner udev rule.** Copy the provided rule file to `/etc/udev/rules.d/` and edit it with your scanner's identifiers (see [Headless Scanner Input](#headless-scanner-input) for details). This also enables automatic start/stop of the container when the scanner connects or disconnects.

5. **Install the pantry.service unit.** This systemd unit owns the container's lifecycle and starts it automatically when the scanner is detected:

   ```bash
   sudo cp /opt/pantry/systemd/pantry.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```
   
   Do **not** run `systemctl enable` on pantry.service — it's activated purely by udev device events, not at boot.

6. **Start Pantry.** With the scanner connected, run:

   ```bash
   sudo udevadm trigger --action=add
   ```
   
   This fires the udev add event and starts the container automatically via pantry.service. You can also power-cycle the scanner to trigger the same behavior.

7. **Verify Deployment**

Check that Pantry is running:

```bash
# Health check
curl http://localhost:8080/health

# Expected response: {"status":"ok"}

# Access the web UI
# Open http://<pi-ip-address>:8080 in a browser on your network
```

## Manual Update Procedure

To update to the latest version:

```bash
cd /opt/pantry
sudo docker compose pull
sudo docker compose up -d
```

Docker Compose will automatically:
- Download the new image if available
- Recreate the container only if the image changed
- Preserve your data volume across updates
- Apply your `.env` configuration to the new container

## Configuration

All configuration is handled through environment variables in `/opt/pantry/.env`. Copy from `.env.example` and modify as needed:

| Variable | Default | Description |
|----------|---------|-------------|
| `PANTRY_IMAGE_TAG` | `latest` | Container image tag to deploy. Use `latest` for newest build, `master` for master branch, or full commit SHA to pin version |
| `HOST_PORT` | `8080` | Host port to expose Pantry service. Container always uses port 8080 internally |
| `PRODUCT_CACHE_TTL` | `720h` | How long to cache product lookups (720h = 30 days) |
| `PRODUCT_MISS_TTL` | `168h` | How long to cache "not found" results (168h = 7 days) |
| `DISABLE_EXTERNAL_PRODUCT_LOOKUP` | `false` | Set to `true` to disable external API calls for product information |
| `STOCK_IN_CONTROL_BARCODE` | `STOCK_IN` | Barcode to scan for switching scanner to "stock in" mode |
| `STOCK_OUT_CONTROL_BARCODE` | `STOCK_OUT` | Barcode to scan for switching scanner to "stock out" mode |
| `HEADLESS_USER_ID` | `user-1` | User ID for headless scan operations (when no user is logged in) |

**Note:** The variables `ADDR` and `DB_PATH` are pinned by the Docker Compose file and should not be overridden. To change the host port, use `HOST_PORT` instead of modifying `ADDR`.

After changing configuration:

```bash
cd /opt/pantry
sudo docker compose up -d
```

## Headless Scanner Input

Pantry now reads barcode scans directly from the scanner's evdev device (`/dev/input/eventN`) instead of standard input, so it works headlessly without a keyboard, monitor, or attached terminal session.

### Prerequisites

1. **Install the scanner udev rule with systemd integration:**
   Copy the provided rule file to `/etc/udev/rules.d/`:
   ```bash
   sudo cp /opt/pantry/udev/99-pantry-scanner.rules /etc/udev/rules.d/
   ```
   
   **For USB scanners:** Edit the rule to replace `XXXX` and `YYYY` with your scanner's actual vendor and product IDs, and uncomment both the `SUBSYSTEM` line and the `TAG+="systemd", ENV{SYSTEMD_WANTS}="pantry.service"` line.
   
   **For Bluetooth scanners:** Modify the rule to match by device name instead and uncomment the `TAG+="systemd", ENV{SYSTEMD_WANTS}="pantry.service"` line:
   ```bash
   SUBSYSTEM=="input", ATTRS{name}=="*Scanner*", \
     KERNEL=="event*", SYMLINK+="pantry-scanner", GROUP="65532", MODE="0640", \
     TAG+="systemd", ENV{SYSTEMD_WANTS}="pantry.service"
   ```
   Replace `*Scanner*` with the actual name pattern from your device (found in step 1).

2. **Reload udev and trigger:**
   ```bash
   sudo udevadm control --reload
   sudo udevadm trigger --action=add    # use 'add', not 'change' or 'reload'
   ```
   
   The `--action=add` flag is essential — a plain `udevadm trigger` defaults to `change` and won't fire `SYSTEMD_WANTS`.

3. **Verify `/dev/pantry-scanner` exists:**
   ```bash
   ls -l /dev/pantry-scanner
   ```
   
   The symlink should exist with GID 65532.

2. **Ensure `SCANNER_DEVICE` is set** (defaults to `/dev/pantry-scanner`) in `.env`.

3. **Important:** The udev rule must be installed and `/dev/pantry-scanner` must exist **before** running `docker compose up`. Docker refuses to start a container whose declared device path does not exist.

4. **Install the pantry.service unit.** This systemd unit owns the container's lifecycle and starts it automatically when the scanner is detected:

   ```bash
   sudo cp /opt/pantry/systemd/pantry.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```
   
   Do **not** run `systemctl enable` on pantry.service — it's activated purely by udev device events, not at boot.

5. **Start the container.** With the scanner connected, run:

   ```bash
   sudo udevadm trigger --action=add
   ```
   
   This fires the udev add event and starts the container automatically via pantry.service. You can also power-cycle the scanner to trigger the same behavior.

5. **Verify scanner status:**
   ```bash
   curl http://localhost:8080/health
   ```
   
   Look for the `scanner` object in the response:
   - `connected: true` and `grabbed: true` means the scanner is working
   - `connected: false` with `lastError` containing "permission denied" means the udev rule's group doesn't match the container's GID (65532)

### Local Development

For local development without a scanner, use stdin mode:
```bash
SCAN_INPUT=stdin go run ./cmd/server
```

Type barcodes directly into the terminal and press Enter. The scanner status will show `connected: false` since there's no device, but scans will still work.

### Troubleshooting

The `GET /health` endpoint's `scanner` object can help diagnose issues:

| Field | Expected Value | Issue if different |
|-------|----------------|-------------------|
| `connected` | `true` | Device not found, permission denied, or unplugged |
| `grabbed` | `true` | Another process is using the device |
| `unmappedKeys` | Low / zero | Scanner is emitting keycodes outside the US-layout map |

**Common issues:**
- `connected: false` with "permission denied" in `lastError`: The udev rule's `GROUP` doesn't match the container's GID (65532)
- High `unmappedKeys`: Your scanner uses a non-US layout; consider modifying `internal/scanlistener/keymap.go`

### Automatic Updates

`pantry-update.service` only updates the container image, never the deployment files. This means:
- The udev rule must be installed once manually on the Pi
- Docker Compose and `.env` files are never automatically modified

## Data Management

### Database Location

Pantry stores its SQLite database in a Docker named volume called `pantry-data`. To inspect the volume:

```bash
sudo docker volume inspect pantry-data
```

### Backup and Restore

**Backup the database:**

```bash
# Create backup
sudo docker cp pantry:/data/pantry.db /home/pi/pantry-backup.db

# Or backup while container is stopped
sudo docker compose down
sudo docker cp pantry:/data/pantry.db /home/pi/pantry-backup.db
sudo docker compose up -d
```

**Restore from backup:**

```bash
# Stop the container
sudo docker compose down

# Restore database
sudo docker cp /home/pi/pantry-backup.db pantry:/data/pantry.db

# Restart
sudo docker compose up -d
```

### Host Directory Access (Alternative)

If you prefer a host directory instead of a named volume, modify `docker-compose.yml`:

```yaml
volumes:
  - /opt/pantry/data:/data
```

Then ensure proper permissions:

```bash
sudo mkdir -p /opt/pantry/data
sudo chown 65532:65532 /opt/pantry/data
```

## Version Management

### Pinning to Specific Versions

To pin to a specific version, set `PANTRY_IMAGE_TAG` in `.env` to a full commit SHA:

```bash
# Example: pin to specific commit
PANTRY_IMAGE_TAG=a1b2c3d4e5f6789012345678901234567890abcd
```

Available tags:
- `latest` - Most recent build from master branch
- `master` - Alias for latest master branch build  
- `<commit-sha>` - Specific commit (full SHA)

### Rolling Back

If you need to rollback to a previous version:

1. Find the commit SHA of the working version from the [repository](https://github.com/Rhionin/pantry)
2. Update `.env`:
   ```bash
   PANTRY_IMAGE_TAG=<previous-commit-sha>
   ```
3. Apply the rollback:
   ```bash
   cd /opt/pantry
   sudo docker compose up -d
   ```

## Automatic Updates (Optional)

For automatic updates, you can install systemd units that periodically check for and apply new releases.

### ⚠️ Important Considerations

**Before enabling automatic updates, understand:**
- The service runs as root because it drives the Docker daemon
- Enabling automatic updates means **every push to master deploys unattended** with no approval step
- Container recreation will drop any attached barcode scanner session
- If you pin to a specific commit SHA, leave automatic updates disabled (they would run forever finding nothing)

### Installation

1. Confirm Docker path:
   ```bash
   command -v docker
   # Should output: /usr/bin/docker
   ```

2. Install the systemd units:
   ```bash
   sudo cp /opt/pantry/systemd/pantry-update.service /etc/systemd/system/
   sudo cp /opt/pantry/systemd/pantry-update.timer /etc/systemd/system/
   sudo systemctl daemon-reload
   ```

3. Enable and start automatic updates:
   ```bash
   sudo systemctl enable --now pantry-update.timer
   ```

### Management

**Check status:**
```bash
# View next scheduled update
sudo systemctl list-timers pantry-update.timer

# Check recent update attempts
sudo journalctl -u pantry-update

# View timer status
sudo systemctl status pantry-update.timer
```

**Disable automatic updates:**
```bash
sudo systemctl disable --now pantry-update.timer
```

**Manual trigger:**
```bash
sudo systemctl start pantry-update.service
```

### Update Interval

The default interval is 5 minutes after boot, then every 5 minutes. To customize:

```bash
sudo systemctl edit pantry-update.timer
```

Add:
```ini
[Timer]
OnUnitActiveSec=
OnUnitActiveSec=30min
```

Save and reload:
```bash
sudo systemctl daemon-reload
```

**Note:** Shorter intervals mean more registry requests but no faster delivery than the build workflow's runtime.

### Rollback After Automatic Update

If an automatic update causes issues:

1. Disable automatic updates:
   ```bash
   sudo systemctl disable --now pantry-update.timer
   ```

2. Pin to last known good version in `.env`:
   ```bash
   PANTRY_IMAGE_TAG=<last-good-commit-sha>
   ```

3. Apply rollback:
   ```bash
   cd /opt/pantry
   sudo docker compose up -d
   ```

4. Verify health:
   ```bash
   curl http://localhost:8080/health
   ```

## Authentication (Private Repository)

Currently, `ghcr.io/rhionin/pantry` is public and requires no authentication.

If the repository becomes private, you'll need to authenticate:

```bash
# Create a GitHub personal access token with 'read:packages' permission
# Then login on the Pi:
sudo docker login ghcr.io
# Username: your-github-username  
# Password: your-personal-access-token
```

The credentials are stored in root's Docker config, which is the identity the update service runs as.

## Troubleshooting

### Container Won't Start

Check logs:
```bash
sudo docker compose logs pantry
```

Common issues:
- Database permissions: Ensure the `/data` volume is writable by uid 65532
- Port conflict: Another service using port 8080
- Image pull failure: Check network connectivity and authentication

**If `/dev/pantry-scanner` exists but the container never starts:**
The container lifecycle is now driven by systemd's `pantry.service`, which is activated by udev device events. Check:

```bash
# Verify the device unit exists and is active
systemctl status dev-pantry\x2dscanner.device

# Verify pantry.service is active
systemctl status pantry.service

# Check the service logs
journalctl -u pantry.service
```

If the scanner is already connected but nothing happened, fire an add event:
```bash
sudo udevadm trigger --action=add
```

### Scanner Not Working

1. **Check scanner status via `/health`:**
   ```bash
   curl http://localhost:8080/health
   ```
   
   Look for the `scanner` object:
   - `connected: false` with `lastError` containing "permission denied" means the udev rule's group doesn't match the container's GID (65532)
   - `connected: false` without an error means the device doesn't exist - verify the udev rule was installed and run `sudo udevadm trigger`

2. **Verify `/dev/pantry-scanner` exists and the device unit is active:**
   ```bash
   ls -l /dev/pantry-scanner
   systemctl status dev-pantry\x2dscanner.device
   systemctl status pantry.service
   ```
   
   Both should show `active`. If the scanner exists but `pantry.service` is inactive, run `sudo udevadm trigger --action=add`.

3. **Check Docker device mapping:**
   ```bash
   sudo docker inspect pantry | grep -A 5 Devices
   ```
   
   The `Devices` array should include `/dev/pantry-scanner`.

4. **Verify the udev rule matches your device:**
   ```bash
   lsusb
   ```
   
   Ensure the `idVendor` and `idProduct` in `/etc/udev/rules.d/99-pantry-scanner.rules` match your scanner's `ID` from `lsusb`.

### Updates Failing

Check update service logs:
```bash
sudo journalctl -u pantry-update -f
```

Common issues:
- Network connectivity during `docker compose pull`
- Docker daemon not running
- Incorrect file paths in systemd unit

### Permission Issues

If using host bind mounts instead of named volumes:
```bash
sudo chown -R 65532:65532 /opt/pantry/data
```

### Configuration Not Applied

Ensure `.env` file is in the same directory as `docker-compose.yml`:
```bash
ls -la /opt/pantry/.env
cd /opt/pantry
sudo docker compose up -d
```

## Support

For issues and support:
- Check the [Pantry repository](https://github.com/Rhionin/pantry) for documentation and issues
- Review container logs: `sudo docker compose logs pantry`
- Check system resources: `free -h && df -h`
- Verify network connectivity to GitHub Container Registry

## Development vs Production

This deployment guide covers the production container setup. For local development:

- See `frontend/README.md` for the development workflow
- Development uses separate Vite dev server + Go API server
- This deployment serves the compiled frontend embedded in the Go binary

The production container serves both the frontend and API on a single port, while development uses two separate processes on different ports.