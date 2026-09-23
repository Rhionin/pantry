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
     KERNEL=="event*", SYMLINK+="pantry-scanner", GROUP="65532", MODE="0640", \
     TAG+="systemd", ENV{SYSTEMD_WANTS}="pantry.service"
   ```
   Replace `XXXX` and `YYYY` with your scanner's actual vendor and product IDs.

3. Reload udev and trigger:
   ```bash
   sudo udevadm control --reload
   sudo udevadm trigger --action=add    # use 'add', not 'change' or 'reload'
   ```

4. Verify `/dev/pantry-scanner` exists and the device unit is active:
   ```bash
   ls -l /dev/pantry-scanner
   systemctl status dev-pantry\x2dscanner.device
   systemctl status pantry.service
   ```

   If the scanner is already connected but the container hasn't started, the `--action=add` trigger above should have fired. You can also power-cycle the scanner.

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
     KERNEL=="event*", SYMLINK+="pantry-scanner", GROUP="65532", MODE="0640", \
     TAG+="systemd", ENV{SYSTEMD_WANTS}="pantry.service"
   ```
   Replace `*Scanner*` with the actual name pattern from your device.

3. Reload udev and trigger:
   ```bash
   sudo udevadm control --reload
   sudo udevadm trigger --action=add    # use 'add', not 'change' or 'reload'
   ```

4. Verify `/dev/pantry-scanner` exists and the device unit is active:
   ```bash
   ls -l /dev/pantry-scanner
   systemctl status dev-pantry\x2dscanner.device
   systemctl status pantry.service
   ```

   If the scanner is already connected but the container hasn't started, the `--action=add` trigger above should have fired. You can also power-cycle the scanner.

### Common Steps for Both Types

5. **Install the pantry.service unit.** This systemd unit owns the container's lifecycle and starts it automatically when the scanner is detected:

   ```bash
   sudo cp /opt/pantry/systemd/pantry.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```
   
   Do **not** run `systemctl enable` on pantry.service — it's activated purely by udev device events, not at boot.

6. **Check health for scanner status:**
   ```bash
   curl http://localhost:8080/health
   ```
   Look for the `scanner` object:
   - `connected: true` and `grabbed: true` means the scanner is working
   - `connected: false` with `lastError` containing "permission denied" means the udev rule's group doesn't match the container's GID (65532)

   If `/dev/pantry-scanner` exists but the container hasn't started, fire an add event:
   ```bash
   sudo udevadm trigger --action=add
   ```