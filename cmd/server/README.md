# Pantry Server

A barcode inventory management server for home pantries, using Open Food Facts
as the product database.

## Quick Start

```bash
# Build and run
go run ./cmd/server

# Server listens on :8080 by default, database is pantry.db
# Visit http://localhost:8080 to access the web UI
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `ADDR` | `:8080` | HTTP server address |
| `DB_PATH` | `pantry.db` | SQLite database path |
| `PRODUCT_CACHE_TTL` | `720h` | Product cache TTL before refresh |
| `PRODUCT_MISS_TTL` | `168h` | Miss cache TTL before retry |
| `DISABLE_EXTERNAL_PRODUCT_LOOKUP` | `false` | Disable external API calls |
| `STOCK_IN_CONTROL_BARCODE` | `STOCK_IN` | Barcode to switch to stock-in mode |
| `STOCK_OUT_CONTROL_BARCODE` | `STOCK_OUT` | Barcode to switch to stock-out mode |
| `HEADLESS_USER_ID` | `user-1` | User ID for headless scan operations |

## Headless Scanner Input

### Local Development (with keyboard)

To test control barcodes and mode switching without a physical scanner:

```bash
SCAN_INPUT=stdin go run ./cmd/server
```

Type a barcode followed by Enter in the terminal. The scanner status at
`GET /health` will show `connected: false` since there's no device.

### Appliance Deployment (with USB scanner)

For Raspberry Pi deployment with a USB barcode scanner:

1. Configure the udev rule at `/etc/udev/rules.d/99-pantry-scanner.rules`
   (see `deploy/udev/99-pantry-scanner.rules` in the repository)

2. In your `.env` file, keep `SCAN_INPUT=device` (default) and set
   `SCANNER_DEVICE=/dev/pantry-scanner`

3. The container will automatically reconnect if the scanner is unplugged
   and replugged

### Health Endpoint

The `/health` endpoint includes scanner status when the listener is configured:

```json
{
  "status": "ok",
  "scanner": {
    "source": "device",
    "devicePath": "/dev/pantry-scanner",
    "connected": true,
    "grabbed": true,
    "mode": "stock_in",
    "unmappedKeys": 0,
    "lastScanAt": "2026-01-15T10:30:00Z",
    "lastError": ""
  }
}
```

The `source` field shows whether the listener is reading from `device` (evdev)
or `stdin`. The `connected` field indicates whether the device is currently
accessible (always true for stdin). The `mode` field shows the current
scan direction (`stock_in` or `stock_out`).
