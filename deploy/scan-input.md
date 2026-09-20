# Headless Scanner Input Configuration

## Configuration Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SCAN_INPUT` | `device` | Source for barcode scans: `device` (evdev) or `stdin` (local development) |
| `SCANNER_DEVICE` | `/dev/pantry-scanner` | Path to the evdev device file (when `SCAN_INPUT=device`) |

## Local Development

For local development on a machine without a scanner:

```bash
SCAN_INPUT=stdin go run ./cmd/server
```

Type barcodes directly into the terminal and press Enter. The scanner status
will show `connected: false` since there's no device, but scans will still
work through stdin.

## Appliance Deployment

### USB Barcode Scanners

1. Find your scanner's vendor/product ID:
   ```bash
   lsusb | grep -i scanner
   ```
   Output looks like: `Bus 001 Device 005: ID XXXX:YYYY Symbol Technologies, Inc. Scanner`

2. Create the udev rule at `/etc/udev/rules.d/99-pantry-scanner.rules`:
   ```bash
   SUBSYSTEM=="input", ATTRS{idVendor}=="XXXX", ATTRS{idProduct}=="YYYY", \
     KERNEL=="event*", SYMLINK+="pantry-scanner", GROUP="pantry", MODE="0640"
   ```
   Replace `XXXX` and `YYYY` with your scanner's actual vendor and product IDs.

3. Reload udev and trigger:
   ```bash
   sudo udevadm control --reload
   sudo udevadm trigger
   ```

4. Verify `/dev/pantry-scanner` exists:
   ```bash
   ls -l /dev/pantry-scanner
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

2. Create the udev rule at `/etc/udev/rules.d/99-pantry-scanner.rules`:
   ```bash
   SUBSYSTEM=="input", ATTRS{name}=="*Scanner*", \
     KERNEL=="event*", SYMLINK+="pantry-scanner", GROUP="pantry", MODE="0640"
   ```
   Replace `*Scanner*` with the actual name pattern from your device.

3. Reload udev and trigger:
   ```bash
   sudo udevadm control --reload
   sudo udevadm trigger
   ```

4. Verify `/dev/pantry-scanner` exists:
   ```bash
   ls -l /dev/pantry-scanner
   ```

### Common Steps for Both Types

5. **Set the scanner group ID:**

   The container process runs as uid 65532 (from the distroless image). The udev rule grants access to this GID.

   **Default (recommended):** Don't set `SCANNER_GID` in `.env` - it defaults to 65532:
   ```bash
   # .env file - SCANNER_GID not needed, defaults to 65532
   ```

   **Custom group (advanced):** If you prefer a named group, create it:
   ```bash
   sudo groupadd --gid 65532 pantry
   ```
   Then add to `.env`:
   ```bash
   SCANNER_GID=65532
   ```

6. Run docker compose:
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
   - `connected: false` with `lastError` containing "permission denied" means the udev rule's group doesn't match `SCANNER_GID`