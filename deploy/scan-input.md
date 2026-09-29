# Headless Scanner Input Configuration

## Configuration Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SCAN_INPUT` | `device` | Source for barcode scans: `device` (evdev) or `stdin` (local development) |
| `SCANNER_DEVICE` | `/dev/input/pantry-scanner` | Path to the evdev device symlink (when `SCAN_INPUT=device`). Point at a concrete `/dev/input/eventN` to bypass the symlink while debugging |

The container starts and serves the web UI whether or not a scanner is attached, and the scanner may be connected or disconnected at any time. There is **no requirement** to install the udev rule or attach the scanner before starting the container. A scanner that appears is picked up within 30 seconds by the application's reconnect loop, which retries with backoff from 250 ms up to a 30 s cap. No systemd unit gates container start on the device.

## Local Development

For local development on a machine without a scanner:

```bash
SCAN_INPUT=stdin go run ./cmd/server
```

Type barcodes directly into the terminal and press Enter. The scanner status
will show `connected: false` since there's no device, but scans will still
work through stdin.

## Appliance Deployment

The symlink is created under `/dev/input/` because the container bind-mounts that directory rather than a single node. A symlink outside `/dev/input` would neither exist nor resolve inside the container.

### USB Barcode Scanners

1. Find your scanner's vendor/product ID:
   ```bash
   lsusb | grep -i scanner
   ```
   Output looks like: `Bus 001 Device 005: ID XXXX:YYYY Symbol Technologies, Inc. Scanner`

2. Create the udev rule at `/etc/udev/rules.d/99-pantry-scanner.rules`. Match on
   `ENV{ID_VENDOR_ID}`/`ENV{ID_MODEL_ID}` (set directly on the event device by
   udev's `input_id` builtin) rather than `ATTRS{idVendor}`/`ATTRS{idProduct}`,
   which must walk the parent chain and are order-sensitive:
   ```
   SUBSYSTEM=="input", KERNEL=="event*", ENV{ID_VENDOR_ID}=="XXXX", ENV{ID_MODEL_ID}=="YYYY", \
     SYMLINK+="input/pantry-scanner", GROUP="65532", MODE="0640"
   ```
   Replace `XXXX` and `YYYY` with your scanner's actual vendor and product IDs.

3. Reload udev and trigger:
   ```bash
   sudo udevadm control --reload
   sudo udevadm trigger --subsystem-match=input --action=add
   ```

4. Verify `/dev/input/pantry-scanner` exists:
   ```bash
   ls -l /dev/input/pantry-scanner
   ```

### Bluetooth Barcode Scanners

Bluetooth scanners appear as input devices and won't show in `lsusb`. Instead:

1. Find your scanner's device name:
   ```bash
   # List all input event devices
   ls -l /dev/input/event*

   # Get details about a specific event device (replace eventX with your device)
   udevadm info -a -p $(udevadm info -q path -n /dev/input/eventX) | grep -E "(vendor|product|name)"

   # Or look for your scanner in the input device list
   cat /proc/bus/input/devices | grep -A5 -B5 -i scanner
   ```

2. Create the udev rule at `/etc/udev/rules.d/99-pantry-scanner.rules`. Bluetooth
   devices are matched by `ATTRS{name}`, since that attribute lives on the input
   device itself:
   ```
   SUBSYSTEM=="input", KERNEL=="event*", ATTRS{name}=="*Scanner*", \
     SYMLINK+="input/pantry-scanner", GROUP="65532", MODE="0640"
   ```
   Replace `*Scanner*` with the actual name pattern from your device.

3. Reload udev and trigger:
   ```bash
   sudo udevadm control --reload
   sudo udevadm trigger --subsystem-match=input --action=add
   ```

4. Verify `/dev/input/pantry-scanner` exists:
   ```bash
   ls -l /dev/input/pantry-scanner
   ```

### Common Steps for Both Types

5. **Group ID:**

   The container process runs as uid 65532 (from the distroless image). The udev rule grants access to GID 65532. No configuration needed in `.env` — the default works. Per-device scoping comes from the node's ownership and mode (`0640` GID 65532), not from the container seeing only one node.

6. Run docker compose (in any order relative to attaching the scanner):
   ```bash
   cd /opt/pantry
   sudo docker compose up -d
   ```

7. Check health for scanner status:
   ```bash
   curl http://localhost:8080/health
   ```
   Look for the `scanner` object:
   - `connected: true` and `grabbed: true` means the scanner is working
   - `connected: false` with no error and no scanner attached is a supported state — the container still runs and the UI still works
   - `connected: false` with `lastError` containing "permission denied" (`EACCES`) means the udev rule's group doesn't match the container's GID (65532)
