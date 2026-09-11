# Pantry Server

## Environment variables

| Env var | Default | Purpose |
|---|---|---|
| `DB_PATH` | `pantry.db` | Path to the SQLite database file. |
| `ADDR` | `:8080` | HTTP listen address. |
| `PRODUCT_CACHE_TTL` | `720h` (30 days) | How long a cached external product lookup is considered fresh. |
| `DISABLE_EXTERNAL_PRODUCT_LOOKUP` | unset | Set to `true` to disable Open Food Facts lookups. |
| `STOCK_IN_CONTROL_BARCODE` | `STOCK_IN` | Barcode that switches the headless scan listener to stock-in mode. |
| `STOCK_OUT_CONTROL_BARCODE` | `STOCK_OUT` | Barcode that switches the headless scan listener to stock-out mode. Must differ from `STOCK_IN_CONTROL_BARCODE`, or the listener won't start. |
| `HEADLESS_USER_ID` | `user-1` | User ID attached to scan entries created by the headless scan listener. |

## Headless barcode scanning

In addition to the browser-based scanner input (`BarcodeInputField` in the frontend), the server reads barcode scans directly from its own standard input. This lets you stock items in or out using a USB or Bluetooth HID barcode scanner without a browser open.

**This requires the terminal window running the server to have keyboard focus.** An HID barcode scanner behaves exactly like a keyboard: it types into whatever window currently has focus. If that's a different application, or no window at all, the scans won't reach the server. There's no OS-level input capture involved — just reading whatever gets typed into the process's stdin, so keeping that terminal focused while scanning is what makes this work.

Because HID scanners have no direction button, mode is set by scanning one of two reserved "control" barcodes (printed labels), configured via `STOCK_IN_CONTROL_BARCODE` / `STOCK_OUT_CONTROL_BARCODE`. Scanning a control barcode switches the mode for all subsequent product scans until another control barcode is scanned. The mode resets to stock-in on every server restart.

Every scan captured this way goes through the same review queue as browser-created scans, so lookup failures and confirmations are handled identically regardless of how the scan was entered.
