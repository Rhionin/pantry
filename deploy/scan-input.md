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

For Raspberry Pi deployment with a USB barcode scanner:

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

3. Reload udev and trigger:
   ```bash
   sudo udevadm control --reload
   sudo udevadm trigger
   ```

4. Verify `/dev/pantry-scanner` exists:
   ```bash
   ls -l /dev/pantry-scanner
   ```

5. Get the numeric gid of the "pantry" group:
   ```bash
   getent group pantry | cut -d: -f3
   ```

6. Add the `SCANNER_GID` to your `.env` file and run:
   ```bash
   docker compose up -d
   ```

7. Check health for scanner status:
   ```bash
   curl http://localhost:8080/health
   ```
