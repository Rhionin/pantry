# Background Scan Listener

The `scanlistener` package provides a headless scanner input path that reads
barcode scans from the server process's standard input or from an evdev device,
depending on the `SCAN_INPUT` configuration.

## Status

This feature supersedes the original stdin-based implementation described in
the `background-scan-listener` spec. The changes are:

- **Narrowed, not removed:** Requirements 1.1 through 1.3 now apply only when
  `SCAN_INPUT=stdin`. The device path (`SCAN_INPUT=device`) implements a
  reconnect loop instead of the never-resume rule.
- **Preserved:** Requirements 2 through 6 remain in force for both sources.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SCAN_INPUT` | `device` | Source: `device` (evdev) or `stdin` (local dev) |
| `SCANNER_DEVICE` | `/dev/pantry-scanner` | Evdev device path |
| `STOCK_IN_CONTROL_BARCODE` | `STOCK_IN` | Mode-switching barcode |
| `STOCK_OUT_CONTROL_BARCODE` | `STOCK_OUT` | Mode-switching barcode |
| `HEADLESS_USER_ID` | `user-1` | User ID for headless scans |

## Local Development

```bash
SCAN_INPUT=stdin go run ./cmd/server
```

Type barcodes into the terminal with Enter.

## Appliance Deployment

See `deploy/` for udev rules, Docker Compose, and deployment documentation.
