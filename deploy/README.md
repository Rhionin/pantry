# Pantry Deployment Guide

This guide covers deploying Pantry on a Raspberry Pi using Docker Compose.

## Quick Start

New to this? Follow these steps in order on your Raspberry Pi and you'll have Pantry running. The setup script does the heavy lifting; each step says what it does. For options, backups, and troubleshooting, use the reference sections further down.

1. **What you need.** A Raspberry Pi running 64-bit Raspberry Pi OS, and the ability to open a terminal. (See [Supported Platform](#supported-platform) below for exact hardware and OS details.)

2. **Get the Pantry files onto the Pi.** The deployment files live in this repository's `deploy/` folder: `docker-compose.yml`, `Caddyfile`, `.env.example`, `setup.sh`, and the `systemd/` and `udev/` folders. Copy that whole folder onto your Pi (for example, clone the repo with `git`, or copy with a USB drive or `scp`).

3. **Run the setup script.** From the `deploy/` folder you copied over:

   ```bash
   sudo ./setup.sh install
   ```

   This installs Docker if needed, copies the deployment files into `/opt/pantry`, creates your configuration, installs the scanner udev rule, starts Pantry, and waits until it reports healthy. It is safe to re-run: it never overwrites configuration values you already set.

4. **Check it works.** The installer already polled `/health` for you, but you can confirm any time. This should print `{"status":"ok"}`:

   ```bash
   curl http://localhost:8080/health
   ```

   You can also open `http://<pi-ip-address>:8080` in a web browser on any device on the same network.

5. **See the full picture.** Run the diagnostic any time to see the whole chain from udev rule to scan ingestion:

   ```bash
   sudo ./setup.sh status
   ```

6. **Optional: open it to the public internet.** The steps above stay on your home network. To serve the same UI at a hostname you own, such as `https://pantry.rhionin.com`, follow [Public Internet access](#public-internet-access). The public site asks for one shared password.

**The scanner is optional at every step.** Pantry starts and serves the web UI whether or not a barcode scanner is attached, and you can connect or disconnect the scanner at any time — see [Headless Scanner Input](#headless-scanner-input).

The rest of this guide is reference material: configuration options, backups, the barcode scanner, rolling back, and troubleshooting.

## Supported Platform

**Target:** 64-bit Raspberry Pi OS on arm64 hardware

Pantry is packaged as a `linux/arm64` container image and requires 64-bit Raspberry Pi OS running on compatible hardware (Raspberry Pi 3B+, 4, 5, or newer models with arm64 architecture).

## First-Time Setup

The recommended path is `sudo ./setup.sh install`, which performs every step below in order and verifies the result. The steps are documented here so you understand what the script does and can run them by hand if you prefer.

### 1. Install Docker and Docker Compose

Install Docker and the Compose plugin on your Raspberry Pi:

```bash
# Update system packages
sudo apt update && sudo apt upgrade -y

# Install Docker (this is what setup.sh runs)
curl -fsSL https://get.docker.com | sh

# Add your user to the docker group (requires logout/login to take effect)
sudo usermod -aG docker $USER

# Verify installation
docker --version
docker compose version
```

Log out and back in for the group membership to take effect.

### 2. Set up deployment files and configuration

`setup.sh install` copies the whole `deploy/` tree to `/opt/pantry` (including `systemd/` and `udev/`) and reconciles `.env`. To do it by hand:

```bash
# Create deployment directory
sudo mkdir -p /opt/pantry

# Copy the entire deploy/ directory contents to /opt/pantry
# (docker-compose.yml, Caddyfile, .env.example, setup.sh, systemd/, udev/)

# Create your config from the example; the defaults work to get started
cd /opt/pantry
sudo cp .env.example .env
sudo nano .env  # edit if needed — see the Configuration table below
```

The script's `.env` handling is idempotent: it creates `.env` from `.env.example` only when it is absent, and on later runs appends only keys that are missing, never touching a value you already set. A pinned `PANTRY_IMAGE_TAG` therefore survives re-running `install`.

### 3. Install the scanner udev rule

```bash
sudo ./setup.sh rule
```

This detects your scanner and writes `/etc/udev/rules.d/99-pantry-scanner.rules` with a symlink at `/dev/input/pantry-scanner`. See [Headless Scanner Input](#headless-scanner-input) for how to do it manually or for a Bluetooth scanner. This step is optional — Pantry runs without it — but scanning won't work until a matching rule exists.

### 4. Start Pantry

```bash
cd /opt/pantry
sudo docker compose up -d
```

The container starts whether or not the scanner is attached. There is **no ordering requirement**: you can install the udev rule and attach the scanner before or after starting the container, in any order.

### 5. Verify deployment

```bash
# Health check — should return {"status":"ok"}
curl http://localhost:8080/health

# Full chain diagnostic
sudo ./setup.sh status

# Access the web UI at http://<pi-ip-address>:8080 from any device on your network
```

## Public Internet access

This puts the Pantry UI on a hostname you already own, with HTTPS, using the same Docker Compose stack. The recommended name is a subdomain (`pantry.rhionin.com`) so the bare domain can stay unused.

The public site asks for one shared password before it serves anything, including the API and the live scan stream. That stops scanners and other bots that do not have the password. It is not separate accounts, and anyone who has the password can change the pantry. The home-network address `http://<pi-ip>:8080` does not ask for the password, so do not forward port 8080 on the router.

The path below is Caddy in the `public` Compose profile, a Let's Encrypt certificate, and an A record at Squarespace. It needs a public IPv4 address and the ability to forward TCP ports 80 and 443. If your ISP uses CGNAT, skip to [When port forwarding cannot work](#when-port-forwarding-cannot-work).

### 1. Confirm the Pi is reachable from the internet

On the Pi:

```bash
curl -4 https://ifconfig.me
```

That prints the address the internet sees. On your router, open the WAN / internet status page and read the WAN IP.

- If those two addresses match, and the WAN address is not in `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, or `100.64.0.0/10`, port forwarding can work.
- If they differ, or the router WAN address is in one of those ranges, the ISP is using CGNAT. Forwarding ports will not make the Pi reachable. Use [Cloudflare Tunnel](#when-port-forwarding-cannot-work) instead.

Reserve a DHCP lease for the Pi (or set a static LAN address) so the forward does not follow the Pi to a new address later.

### 2. Forward ports on the router

Create two forwards to the Pi's LAN address:

| WAN (external) | Pi (internal) |
|----------------|---------------|
| TCP 80 | TCP 80 |
| TCP 443 | TCP 443 |

UDP 443 is optional. It enables HTTP/3. The site works with only the two TCP forwards.

Do not forward port 8080. Leave UPnP off, and delete any other forward you are not using.

The published Pantry port is IPv4-only (`0.0.0.0`). That keeps a global IPv6 address on the Pi from answering on 8080. It does not stop a router forward: a forwarded packet is still addressed to the Pi's LAN IPv4 address, and Docker's userland proxy accepts it. `ufw` does not close that port either. Docker accepts the connection in its own proxy and, separately, DNATs it around the `INPUT` chain `ufw` edits.

If `sudo ufw status` says `active`, allow the proxy ports. Leave a firewall that is currently inactive turned off. Enabling it without an SSH allow rule can lock you out of the Pi.

```bash
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 443/udp
```

To also drop non-LAN clients that reach the Pantry port (loopback, private LAN, and Tailscale `100.64.0.0/10` stay allowed; ports 80 and 443 are not changed):

```bash
sudo ./setup.sh firewall
```

`sudo ./setup.sh firewall-off` removes that rule. It is optional. The router rule above is the one that matters.

### 3. Point the Squarespace domain at that address

`rhionin.com` stays registered at Squarespace. You are adding one DNS record, not moving the domain.

1. Open the [Squarespace domains dashboard](https://account.squarespace.com/domains) and sign in.
2. Click **rhionin.com**.
3. Click **DNS**, then **DNS Settings**.
4. Under **Custom records**, click **Add record**.
5. Set:
   - **Type:** A
   - **Host:** `pantry`  
     Squarespace appends `.rhionin.com`. Do not type the full name, and do not type `https://`.
   - **Data** (IP address): the IPv4 from `curl -4 https://ifconfig.me`
6. Save the record.

Leave the Squarespace default records for `@` and `www` alone. A new host named `pantry` does not conflict with them. Use host `@` only if you want `https://rhionin.com` itself; in that case delete the Squarespace default A and CNAME records for `@` first, because a custom record cannot override those presets.

Squarespace does not offer a TTL field; their DNS is commonly cached for about four hours. Check that the record has landed before asking for a certificate:

```bash
dig +short pantry.rhionin.com A
```

The answer must be the same address `curl -4 https://ifconfig.me` prints. If `dig` is not installed, `getent hosts pantry.rhionin.com` is enough. This often updates within an hour and can take up to a day.

When your home IP changes, edit this same A record. The certificate is for the hostname, so HTTPS starts working again as soon as DNS matches the new address. Squarespace has no dynamic-DNS service; updating the record is a manual step.

### 4. Start the proxy

On the Pi, edit `/opt/pantry/.env` (create the LAN install first with `sudo ./setup.sh install` if you have not):

```bash
PUBLIC_HOST=pantry.rhionin.com
ACME_EMAIL=you@example.com
BASIC_AUTH_USER=pantry
BASIC_AUTH_PASSWORD=replace-with-a-long-passphrase
```

`PUBLIC_HOST` is the hostname only. `ACME_EMAIL` is where Let's Encrypt sends expiry notices. Replace `BASIC_AUTH_PASSWORD` with a passphrase of 12 to 72 characters, and do not wrap it in quotes. `publish` hashes it into `/opt/pantry/auth.caddy` (mode `0600`) and restricts `.env` to its owner. The password itself stays in `.env` so you can change it later; it is not written into the image or the repository.

```bash
sudo ./setup.sh publish
```

That refreshes `docker-compose.yml` and `Caddyfile` into `/opt/pantry`, then starts [Caddy](https://caddyserver.com/) on ports 80 and 443. Caddy requests a Let's Encrypt certificate for `PUBLIC_HOST` and renews it on its own. Starting the proxy accepts the Let's Encrypt subscriber agreement.

Certificates are stored in the Docker volume `caddy-data`. Do not delete that volume to "retry" a failure: Let's Encrypt rate-limits repeat issuances (on the order of five duplicate certificates per hostname per week).

### 5. Prove it is public

From a phone on cellular data, not the home Wi-Fi:

```bash
curl -fsS -u 'pantry:replace-with-a-long-passphrase' https://pantry.rhionin.com/health
```

Without the password, that command returns `401`. With it, expect `{"status":"ok",...}`. Then open `https://pantry.rhionin.com` in the phone's browser, enter the same username and password when asked, and confirm the pantry UI loads. The scan queue and inventory pages keep a live connection to `/api/events`; new scans should show up without a refresh.

To change the password, edit `BASIC_AUTH_PASSWORD` and run `sudo ./setup.sh publish` again. Browsers that saved the old password will ask again.

`sudo ./setup.sh unpublish` stops only the proxy. The LAN site keeps running. Clear `PUBLIC_HOST` as well if an automatic-update timer is enabled, or the next update will start the proxy again.

### When something fails

Logs from the proxy:

```bash
cd /opt/pantry
sudo docker compose --profile public logs --tail=80 caddy
```

| What you see | What to fix |
|--------------|-------------|
| `NXDOMAIN`, or `dig` returns no address | The Squarespace A record is missing or still cached. Wait, then check `dig +short pantry.rhionin.com A` again. |
| Certificate error mentioning timeout, connection refused, or `404` from another site | Port 80 is not reaching this Pi. Re-check the router forward and that no other program is bound to port 80. |
| Browser warning, certificate name mismatch | `PUBLIC_HOST` and the Squarespace host are not the same name. They must match exactly. |
| `https://` works at home but not on cellular | The phone is still using the LAN address, or the forward is wrong. Test on cellular. |
| Browser or curl gets `401` | The shared password is missing or does not match `.env`. A request with no password is supposed to be rejected. Re-run `sudo ./setup.sh publish` after changing `BASIC_AUTH_PASSWORD`. |
| UI loads, but the scan queue never updates live | `/api/events` is being buffered. `deploy/Caddyfile` must keep `flush_interval -1` on that path. Re-run `sudo ./setup.sh publish` after pulling a fresh `Caddyfile`. |

### When port forwarding cannot work

Two free options, neither of which is wired into this repo. Pick one; do not run them in front of Caddy at the same time.

**Cloudflare Tunnel** (fits `pantry.rhionin.com` when you cannot forward ports). Create a free Cloudflare account, add `rhionin.com`, and let Cloudflare show you two nameservers. In Squarespace: **Domains → rhionin.com → DNS → Nameservers → use custom nameservers**, and paste those two. That moves DNS for the whole domain to Cloudflare; the registration stays at Squarespace. Then install `cloudflared` on the Pi. Do not point the tunnel at port 8080: that port has no password. Put Cloudflare Access (free for a small number of users) in front of the hostname, or publish through Caddy on localhost and tunnel to that. The tunnel login writes a credential on the Pi. Leave it there; do not commit it. No ports to forward, and a changing home IP does not matter.

**Tailscale Funnel** (fits a stable URL when you do not need `rhionin.com`). The free personal tier can expose the Pi as a `*.ts.net` name without opening ports. Putting a Squarespace name on Funnel is more work than the Caddy path; use Funnel when a Tailscale hostname is enough. Do not funnel port 8080; that port has no password. Funnel the Caddy port, or put Tailscale in front of the LAN address and skip Funnel.

### What stays exposed if you forward ports

Forwarding 80 and 443 to a computer in the house is a different risk from a VPN or a tunnel. The Pi is on your LAN. Anyone who gets past the shared password is on a process that can read the pantry database, change inventory, and, if you connected Kroger, use the refresh token stored in that database. A bug or a guessed password is not confined to a cloud VM. A tunnel or Tailscale does not put a listening port on the home router; the router path does.

The shared password is one secret for the whole household. Caddy applies it to every path on the public hostname, including `/api/events`, static files, and `/health`. There is no lockout and no rate limit in this Caddy build, so the password needs to be long (12 to 72 characters, and longer is better). The LAN address `http://<pi-ip>:8080` never asks for it. That is deliberate. Do not publish that port.

Kroger client secrets and refresh tokens are stored in `pantry.db` as plain text. The HTTP API does not return them. A copy of the database (a backup, or the Docker volume) does. Treat `pantry.db` like a password file. The app has no login of its own: if Caddy is stopped or mis-mounted and something else forwards port 8080, every route is open, including wiping inventory (the body must contain `WIPE INVENTORY`) and saving a Kroger client secret.

Automatic updates run as root and pull `latest` when you enable the timer. A bad image then starts on the same Pi that is reachable from the internet. Leave the timer off unless you want that.

Prefer a tunnel or Tailscale when you do not need a public listener. If you keep the port forward: only TCP 80 and 443, DHCP reservation, UPnP off, no forward of 8080, and `sudo ./setup.sh firewall` so a mistaken forward of the Pantry port still has to come from the LAN.

### Keeping HTTPS across automatic updates

`pantry-update.sh` includes the `public` profile only when `PUBLIC_HOST` is set, so a timer pull renews the proxy instead of forgetting it. `sudo ./setup.sh publish` copies the updated unit into `/etc/systemd/system/` if that unit is already installed. If you enabled the timer before this change and have not run `publish` yet:

```bash
sudo cp /opt/pantry/systemd/pantry-update.service /etc/systemd/system/
sudo systemctl daemon-reload
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

Recreating the container briefly drops the scanner session, but the listener reconnects on its own within 30 seconds — no manual step is needed.

## Configuration

All configuration is handled through environment variables in `/opt/pantry/.env`. Copy from `.env.example` and modify as needed:

| Variable | Default | Description |
|----------|---------|-------------|
| `PANTRY_IMAGE_TAG` | `latest` | Container image tag to deploy. Use `latest` for newest build, `master` for master branch, or full commit SHA to pin version |
| `HOST_PORT` | `8080` | IPv4 port for the LAN site. The container still listens on 8080. Published on `0.0.0.0` only, not IPv6. Do not forward this port on the router |
| `PUBLIC_HOST` | empty | Hostname for the public HTTPS proxy, such as `pantry.rhionin.com`. Empty keeps the install LAN-only. No `https://` |
| `ACME_EMAIL` | empty | Email Let's Encrypt uses for certificate expiry notices. Required when `PUBLIC_HOST` is set. Not a Pantry login |
| `BASIC_AUTH_USER` | `pantry` | Username the browser asks for on the public site |
| `BASIC_AUTH_PASSWORD` | empty | Shared password for the public site, 12 to 72 characters. Required before `publish` will start. The hash is written to `auth.caddy`; this value stays in `.env` |
| `PRODUCT_CACHE_TTL` | `720h` | How long to cache product lookups (720h = 30 days) |
| `PRODUCT_MISS_TTL` | `168h` | How long to cache "not found" results (168h = 7 days) |
| `DISABLE_EXTERNAL_PRODUCT_LOOKUP` | `false` | Set to `true` to disable external API calls for product information. This also keeps product contribution local |
| `PANTRY_RETAILER_API_KEY` | empty | Store price API key. Leave empty until a retailer adapter and credentials exist. Noted sales in the app still apply |
| `PANTRY_RETAILER_API_URL` | empty | Reserved base URL for that store price adapter |
| `PRODUCT_OPENER_USER_ID` | empty | Optional. Account used only after someone opts in to share a product. Leave empty to keep contributions on this pantry |
| `PRODUCT_OPENER_PASSWORD` | empty | Optional password for that account. Each open database has its own users |
| `STOCK_IN_CONTROL_BARCODE` | `STOCK_IN` | Barcode to scan for switching scanner to "stock in" mode |
| `STOCK_OUT_CONTROL_BARCODE` | `STOCK_OUT` | Barcode to scan for switching scanner to "stock out" mode |
| `HEADLESS_USER_ID` | `user-1` | User ID for headless scan operations (when no user is logged in) |
| `SCANNER_DEVICE` | `/dev/input/pantry-scanner` | Path to the scanner's stable symlink created by the udev rule. Point at a concrete `/dev/input/eventN` to bypass the symlink while debugging |
| `SCANNER_GID` | `65532` | Numeric group ID the container process joins so it can read the `0640` scanner node |

**Note:** The variables `ADDR` and `DB_PATH` are pinned by the Docker Compose file and should not be overridden. To change the host port, use `HOST_PORT` instead of modifying `ADDR`.

After changing configuration:

```bash
cd /opt/pantry
sudo docker compose up -d
```

Setting `PUBLIC_HOST` does not publish the site by itself. Run `sudo ./setup.sh publish` so the `public` profile starts. A plain `docker compose up` leaves that profile off.

## Headless Scanner Input

Pantry reads barcode scans directly from the scanner's evdev device (`/dev/input/eventN`) instead of standard input, so it works headlessly without a keyboard, monitor, or attached terminal session.

**The container starts whether or not the scanner is attached, and the scanner may be connected or disconnected at any time.** There is no requirement to install the udev rule or attach the scanner before starting the container.

### How hot-plug works

- The container bind-mounts the `/dev/input` **directory** rather than a single device node. Nodes that udev creates later — when you plug the scanner in — become visible inside the running container automatically, with no recreation.
- The application retries opening the device with exponential backoff, from 250 ms up to a 30 s cap. A scanner that appears (at boot, on plug-in, or on Bluetooth wake) is picked up **within 30 seconds** with no operator action.
- The web UI is served the entire time, independent of scanner state. Disconnecting the scanner does not stop, restart, or recreate the container.

### Prerequisites

1. **Find your scanner's device information:**

   **USB scanners:**
   ```bash
   lsusb | grep -i scanner
   ```
   Output looks like: `Bus 001 Device 005: ID XXXX:YYYY Symbol Technologies, Inc. Scanner`. The `XXXX` is the vendor ID and `YYYY` is the product ID.

   **Bluetooth scanners:**
   Bluetooth scanners appear as input devices and won't show in `lsusb`. Instead:
   ```bash
   # List all input event devices
   ls -l /dev/input/event*

   # Get details about a specific event device (replace eventX with your device)
   udevadm info -a -p $(udevadm info -q path -n /dev/input/eventX) | grep -E "(vendor|product|name)"

   # Or look for your scanner in the input device list
   cat /proc/bus/input/devices | grep -A5 -B5 -i scanner
   ```

2. **Install the udev rule.** The primary method is the setup script, which detects the device and writes the rule for you:
   ```bash
   sudo ./setup.sh rule
   ```

   **Manual fallback.** Copy the example rule and edit it:
   ```bash
   sudo cp /opt/pantry/udev/99-pantry-scanner.rules /etc/udev/rules.d/
   ```

   **For USB scanners**, uncomment the USB example and set the vendor/product IDs. The rule matches on `ENV{ID_VENDOR_ID}` and `ENV{ID_MODEL_ID}` (set directly on the event device by udev's `input_id` builtin) rather than `ATTRS{idVendor}`/`ATTRS{idProduct}`, which must walk the parent chain and are order-sensitive:
   ```
   SUBSYSTEM=="input", KERNEL=="event*", ENV{ID_VENDOR_ID}=="XXXX", ENV{ID_MODEL_ID}=="YYYY", \
     SYMLINK+="input/pantry-scanner", GROUP="65532", MODE="0640"
   ```

   **For Bluetooth scanners**, match by device name instead:
   ```
   SUBSYSTEM=="input", KERNEL=="event*", ATTRS{name}=="*Scanner*", \
     SYMLINK+="input/pantry-scanner", GROUP="65532", MODE="0640"
   ```
   Replace `*Scanner*` with the actual name pattern from your device (found in step 1).

   Note the symlink is `input/pantry-scanner`, which resolves to `/dev/input/pantry-scanner`. It **must** live under `/dev/input` because the container bind-mounts only that directory; a symlink elsewhere would neither exist nor resolve inside the container.

3. **Reload udev and trigger:**
   ```bash
   sudo udevadm control --reload
   sudo udevadm trigger --subsystem-match=input --action=add
   ```

4. **Verify `/dev/input/pantry-scanner` exists:**
   ```bash
   ls -l /dev/input/pantry-scanner
   ```
   The symlink should exist with GID 65532 on its target node.

5. **Ensure `SCANNER_DEVICE` is set** (defaults to `/dev/input/pantry-scanner`) in `.env`.

6. **Verify scanner status:**
   ```bash
   curl http://localhost:8080/health
   ```
   Look for the `scanner` object in the response:
   - `connected: true` and `grabbed: true` means the scanner is working
   - `connected: false` is a fully supported state — the container still runs and the UI still works; the scanner is simply absent or not yet readable

### Security: per-device scoping

Per-device scoping now comes from the scanner node's **ownership and mode**, not from the container seeing only one node. The container bind-mounts all of `/dev/input`, but the udev rule grants only the scanner's node `GROUP="65532"` mode `0640`. Every other input node stays `root:input` mode `0600`, so the container process (uid 65532, group 65532) can open the scanner and nothing else. This is why Pantry does not become a keylogger for every attached keyboard, and it is why the service user is deliberately **not** added to the `input` group — doing so would grant read access to every input device on the box.

### Local Development

For local development without a scanner, use stdin mode:
```bash
SCAN_INPUT=stdin go run ./cmd/server
```

Type barcodes directly into the terminal and press Enter. The scanner status will show `connected: false` since there's no device, but scans will still work.

### Troubleshooting

First, distinguish the two very different situations that both show `connected: false`:

- **Scanner absent** — no scanner is plugged in. This is a fully **supported** state: the container is running, `/health` returns `status: ok`, and the web UI works. Nothing is wrong. Plug the scanner in and it is picked up within 30 seconds.
- **Scanner present but unreadable** — the scanner is attached but Pantry cannot read it. This is the case worth investigating. The `lastError` field tells you which of three causes applies, in the order worth checking:

| `lastError` | Meaning | Fix |
|---|---|---|
| `ENOENT` / no such file | The udev rule did not match, so no symlink was created | Fix the rule's match keys (`ENV{ID_VENDOR_ID}`/`ENV{ID_MODEL_ID}` for USB, `ATTRS{name}` for Bluetooth) and re-run `sudo ./setup.sh rule` |
| `EACCES` / permission denied | The node's group is not 65532 | Fix `GROUP` in the rule or `SCANNER_GID` in `.env`, then reload udev |
| `EPERM` / operation not permitted | The device cgroup denied the open | `device_cgroup_rules` is not taking effect — do **not** fall back to `privileged: true` or reintroduce a `devices:` entry; see `ACCEPTANCE.md` |

The `GET /health` `scanner` object also reports:

| Field | Expected Value | Issue if different |
|-------|----------------|-------------------|
| `grabbed` | `true` | Another process is using the device (the exclusive grab failed — capture still works) |
| `unmappedKeys` | Low / zero | Scanner is emitting keycodes outside the US-layout map; consider modifying `internal/scanlistener/keymap.go` |

The image is **distroless**, so `docker exec pantry sh` does not work. Use `sudo ./setup.sh logs`, `sudo docker compose logs pantry`, `GET /health`, and `sudo ./setup.sh status` for all runtime introspection.

### Automatic Updates and the scanner

`pantry-update.service` only updates the container image, never the deployment files. When `PUBLIC_HOST` is set, the update script also refreshes the public HTTPS proxy; see [Keeping HTTPS across automatic updates](#keeping-https-across-automatic-updates). This means:
- The udev rule is installed once (via `sudo ./setup.sh rule`) and is not touched by updates
- Docker Compose and `.env` files are never automatically modified

When an update recreates the container, the scanner session is dropped, but the listener reconnects on its own within 30 seconds. While iterating, run `sudo ./setup.sh freeze` to mask the update timer so an update doesn't change the target mid-experiment; run `sudo ./setup.sh thaw` to restore it.

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

A pinned tag is preserved across re-runs of `sudo ./setup.sh install` and is never silently reset to `latest`.

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

For automatic updates, you can install systemd units that periodically check for and apply new releases. `setup.sh install` does **not** enable them by default — pass `--with-updates` or enable them manually when you are done iterating.

### ⚠️ Important Considerations

**Before enabling automatic updates, understand:**
- The service runs as root because it drives the Docker daemon
- Enabling automatic updates means **every push to master deploys unattended** with no approval step
- Container recreation drops the attached scanner session, but the listener reconnects on its own within 30 seconds
- If you pin to a specific commit SHA, leave automatic updates disabled (they would run forever finding nothing)

### Installation

1. Confirm Docker path:
   ```bash
   command -v docker
   # Should output: /usr/bin/docker
   ```

2. Install the systemd units. The service runs `/opt/pantry/systemd/pantry-update.sh`, which `setup.sh install` copies into place and marks executable. When `PUBLIC_HOST` is set, that script keeps the HTTPS proxy in the update.
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

**Freeze / thaw during iteration:**
```bash
sudo ./setup.sh freeze   # mask the timer so updates don't change the target
sudo ./setup.sh thaw     # unmask it when you're done
```

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

### Public website doesn't load

See [Public Internet access](#public-internet-access). The usual causes are the Squarespace A record not pointing at this Pi yet, or router ports 80 and 443 not forwarded. From `/opt/pantry`:

```bash
sudo docker compose --profile public logs --tail=80 caddy
```

### Container Won't Start

Start with the full-chain diagnostic:
```bash
sudo ./setup.sh status
```

Check logs:
```bash
sudo docker compose logs pantry
```

Common issues:
- Database permissions: Ensure the `/data` volume is writable by uid 65532
- Port conflict: Another service using port 8080 (change `HOST_PORT` in `.env`)
- Image pull failure: Check network connectivity and authentication

Note the image is distroless, so `docker exec pantry sh` does not work — use logs and `/health` instead.

### Scanner Not Working

1. **Run the diagnostic first — it names the broken link:**
   ```bash
   sudo ./setup.sh status
   ```

2. **Upgrading from a pre-hot-plug version? Check `SCANNER_DEVICE`.**
   ```bash
   grep '^SCANNER_DEVICE=' /opt/pantry/.env
   ```
   An older `.env` pinned `SCANNER_DEVICE=/dev/pantry-scanner`, and the container can no longer open that path — the symlink now lives at `/dev/input/pantry-scanner`. Running `sudo ./setup.sh install` migrates this obsolete default automatically; after it runs, the value should be `/dev/input/pantry-scanner`. (A custom path you set on purpose is left untouched.) Apply it with `cd /opt/pantry && sudo docker compose up -d`. Symptom of the stale value: logs show `failed to open device /dev/pantry-scanner: no such file or directory` and `status` reports the old path.

3. **Check scanner status via `/health`:**
   ```bash
   curl http://localhost:8080/health
   ```
   Then use the `lastError` table in [Headless Scanner Input → Troubleshooting](#troubleshooting) to tell `ENOENT` (rule didn't match), `EACCES` (wrong group), and `EPERM` (cgroup denied) apart — they have different fixes. Remember that `connected: false` with **no** error and no scanner attached is normal and supported.

4. **Verify `/dev/input/pantry-scanner` exists:**
   ```bash
   ls -l /dev/input/pantry-scanner
   ```
   If it is missing while the scanner is attached — and especially if the scanner's keystrokes are appearing in the Pi's terminal — the udev rule is not matching. The scanner echoing to the console is the tell: the rule that would grant the node to GID 65532 and create the symlink never fired. Regenerate it with `sudo ./setup.sh rule` (which now matches USB scanners on vendor/product IDs) and confirm with `ls -l /dev/input/pantry-scanner`.

5. **Check the container's mounts** (there is no longer a `Devices` array — the scanner is a directory bind mount now):
   ```bash
   sudo docker inspect -f '{{json .Mounts}}' pantry
   ```
   The `Mounts` array should include the `/dev/input` bind mount.

6. **Verify the udev rule matches your device:**
   ```bash
   lsusb                              # USB
   cat /proc/bus/input/devices        # any input device
   ```
   Ensure the `ENV{ID_VENDOR_ID}`/`ENV{ID_MODEL_ID}` (USB) or `ATTRS{name}` (Bluetooth) in `/etc/udev/rules.d/99-pantry-scanner.rules` match your scanner, then re-run `sudo ./setup.sh rule`.

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
