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
  without changing the connection banner. The banner hides while the preview
  is open so the queue can stay on screen.

## How to get to it (user POV)

- Open `Scan Queue` (`/`). The `Scan with camera` button sits under the hardware
  scanner connection banner.
- Choose `Scan with camera`. Allow the camera prompt on a secure (HTTPS or
  localhost) page. The preview stays short so the queue remains on screen. If
  the picture has not started, tap `Tap to start scanning`. Otherwise the frame
  says it scans automatically. A captured code flashes the preview, outlines
  the barcode when the detector reports its location, and highlights the new
  queue card. Type a barcode if the camera cannot start.
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
- If `Tap to start scanning` appears, click it.
- If `Hold a barcode in the frame. It scans automatically.` appears, the preview
  started. A verification run can arm an injected detector; a real phone just
  holds the barcode in frame.
  This environment has no printed code, so if nothing is queued, enqueue through
  `Barcode scanner input` instead. A decoded frame uses that same capture handler.
- Wait for the article `Scan <barcode>`, then `Approve` it.
- Confirm the card detaches, the inventory API reports one unit, and the
  Inventory route shows `1 box`.

`scripts/drive-camera-scan.mjs` is this recipe.

## Gotchas

- Headless Chromium cannot decode a real barcode image from a camera. The
  camera driver injects a detector and a fake lens so the phone layout, capture
  line, and barcode outline can be checked. A run without that injection still
  falls back to typing. Do not treat a green or empty preview as a failed scan.
- `getUserMedia` only works in a secure context. Localhost counts. A phone
  opening the Pi over plain `http://` will see the HTTPS explanation and the
  typed fallback.
- The same code is ignored for two seconds so a code held in frame is not queued
  twice.
- `Approve` needs `{ exact: true }` so it does not match the batch approve
  control.
