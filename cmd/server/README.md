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
| `DISABLE_EXTERNAL_PRODUCT_LOOKUP` | `false` | Disable external API calls. Also keeps product contribution local |
| `PANTRY_RETAILER_API_KEY` | empty | Store price API key. Empty keeps live prices off. A key alone does not fetch prices until a retailer adapter is connected. Sales you note in the app still apply. |
| `PANTRY_RETAILER_API_URL` | empty | Reserved base URL for that adapter |
| `PRODUCT_OPENER_USER_ID` | empty | Optional Product Opener account. Both this and the password must be set before an opted-in product can be sent |
| `PRODUCT_OPENER_PASSWORD` | empty | Password for that account. Never invent one |
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

Public HTTPS, for a hostname you own, is an optional step on top of that LAN install. See `deploy/README.md` (Public Internet access). It does not add a login.

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

## Telemetry

`/health` answers "is the process up, and is the scanner connected?" The
diagnostic snapshot is `GET /api/telemetry`. It stays in memory on this
machine: nothing is exported, and the document does not include barcodes,
product names, or user ids. Query strings are stripped from browser reports.

```bash
curl -s http://localhost:8080/health
curl -s http://localhost:8080/api/telemetry
```

In development the Vite server proxies `/api`, so the same path works against
the dev UI's origin. The browser posts its own timings and errors to
`POST /api/telemetry/client`; you read them back from the snapshot.

Lines written to the server log start with `telemetry` when a request is slow
or failing, when a scan is published with no browser connected (after one has
connected before), when a slow browser is dropped, or when the page reports
an error. Grep for that prefix.

How to read a bad morning:

| What you saw | Where to look |
| --- | --- |
| The page never loads | If `curl /health` fails the same way, the process is not accepting connections. If `/health` is ok, check `http.recentProblems`, `http.serverErrors`, and `http.latency` for the route that hung (`GET /`, `GET /api/products`, `GET /api/scans`, `GET /api/inventory`). `streams.openLatency` is how long `GET /api/events` took to send headers. |
| A scan is missing until refresh | `scanSync.publishedWithNoSubscribers` (and `scanSync.byEvent.scan`) counts events saved while `subscribers` was 0. Refresh works because `GET /api/scans` reads the database; the live stream does not replay. `streams.active` and `scanSync.subscribers` are the browsers listening right now. |
| A scan disappears, or the page misses one while it stays open | `scanSync.droppedSlowClients` and `writeErrors` mean a browser fell behind and was disconnected. `client.sseGaps` and `client.sseErrors` are the page reporting a hole in event ids or a dropped stream. `client.recent` lists the latest page loads, route changes, API failures, and those stream errors. `at` on a client report is when the server received it. |

Latency fields are milliseconds. `p50Ms` / `p95Ms` / `p99Ms` are upper bounds of fixed buckets (1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000), not exact percentiles. `delivery` is publish-to-write. `ingestToDelivery` is the scan's `scannedAt` to that write. The `GET /api/telemetry` request itself is recorded after the snapshot is sent, so it shows up on the next fetch. UI asset requests share the `GET /` route because one handler serves the page.
