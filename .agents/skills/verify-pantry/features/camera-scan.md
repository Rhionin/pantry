# Scan a barcode with the device camera

The scan queue can capture a barcode from the phone or desktop camera and drop
it into the same stock-in / stock-out queue a hardware scanner uses. The camera
stays off until the user chooses Scan with camera. When the camera cannot start
(permission, no device, or a page that is not HTTPS), the same control offers a
typed barcode so the item can still be queued.

## Sub-features

- `camera-opt-in` the camera is off on load; Scan with camera starts it.
- `camera-enqueue` a decoded barcode is posted as a scan in the current mode.
- `camera-fallback` a blocked camera offers Type a barcode, which posts the same way.
- `camera-mode` a control barcode read by the camera switches stock in / stock out
  and does not create a product scan.
- `camera-disconnected` a disconnected hardware scanner points at the camera
  without changing the connection banner.

## How to get to it (user POV)

- Open `Scan Queue` (`/`). The `Scan with camera` button sits under the hardware
  scanner connection banner.
- Choose `Scan with camera`. Allow the camera prompt on a secure (HTTPS or
  localhost) page. Point the lens at a barcode, or type one if the camera cannot
  start.
- Review and approve the card the same way as a hardware scan.

## Driving it with Playwright

Preconditions:

- App is healthy (`doctor` passes) at `http://127.0.0.1:5173`.
- A product is seeded and bound to the barcode via `createKnownProduct`, because
  external lookup is disabled.
- The queue is in `stock_in` mode (`Mode: stock_in`).

Steps:

- Open `/` and confirm `Scan with camera` is visible. The hardware banner may
  read `Scanner disconnected` in this environment; that sentence also mentions
  the camera.
- Click `Scan with camera`.
- If `Camera scanning unavailable` appears, fill `Type a barcode` with the seeded
  code and click `Add scan`.
- If `Point the camera at a barcode.` appears, the preview started. This
  environment has no printed code in frame, so enqueue through `Barcode scanner
  input` instead. A real decoded frame uses that same capture handler; component
  tests cover the decode callback.
- Wait for the article `Scan <barcode>`, then `Approve` it.
- Confirm the card detaches, the inventory API reports one unit, and the
  Inventory route shows `1 box`.

`scripts/drive-camera-scan.mjs` is this recipe.

## Gotchas

- Headless Chromium cannot decode a real barcode image from a camera. Do not
  treat a green or empty preview as a failed scan. The e2e suite does not drive
  a lens for the same reason; frame decoding is covered by component tests.
- `getUserMedia` only works in a secure context. Localhost counts. A phone
  opening the Pi over plain `http://` will see the HTTPS explanation and the
  typed fallback.
- The same code is ignored for two seconds so a code held in frame is not queued
  twice.
- `Approve` needs `{ exact: true }` so it does not match the batch approve
  control.
