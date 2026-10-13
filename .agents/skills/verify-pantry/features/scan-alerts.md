# Scan alerts

While Pantry is open in a browser tab, the scan queue offers "Notify me about
scans". That button is the only way the page asks for notification permission.
After permission is granted, a failed lookup and an unrecognized product each
raise a browser notification immediately. A stock in or stock out batch of
recognized scans raises one notification 5 minutes after the last scan in that
batch. Clicking a notification focuses the scan queue on the tab that holds
those scans.

## Sub-features

- `alert-permission` the scan queue shows "Notify me about scans" until the
  person uses it. Loading the page does not open the browser permission prompt.
- `alert-immediate` a failed lookup and an unrecognized barcode notify at once.
- `alert-batch` recognized stock in and stock out scans share a 5-minute quiet
  period per direction. One notification names how many scans are still waiting.
- `alert-open` choosing the notification focuses Scan Queue. A stock in notice
  selects Stock in; an unrecognized barcode with no direction selects Stock out.

## How to get to it (user POV)

- Open `Scan Queue` (`/`). The control sits under the scanner connection line
  and above the Stock in and Stock out tabs.
- Use `Notify me about scans`, then allow notifications in the browser prompt.
- Leave the tab open. Scan from this page or from the scanner.

## Driving it with Playwright

Preconditions:

- App is healthy (`doctor` passes) at `http://127.0.0.1:5173`.
- Notification permission starts unset, so the button is visible.
- Products for a batch are seeded with `createKnownProduct`. An unseeded
  barcode is the unrecognized-product case. External lookup is off.
- The queue banner reads `Mode: stock_in` before the batch scan.

- **See the ask.** `getByRole('button', { name: 'Notify me about scans' })` is
  visible, and its description says failed scans and unrecognized products
  alert right away.
- **Allow alerts.** `context.grantPermissions(['notifications'])`, then click
  `Notify me about scans`. The button leaves the page. A status line says
  alerts work while Pantry is closed when this browser stored a push
  subscription, or that they stay in this tab when it could not.
- **Unrecognized product.** `scanBarcode` an unseeded barcode. The card shows
  `Flagged`. The notification title is `Unrecognized product`.
- **Quiet batch.** Scan a seeded product, then advance the page clock by 5
  minutes. The notification title is `Stock in batch is ready`.

`scripts/drive-scan-alerts.mjs` is this recipe. `scripts/drive-scan-push.mjs`
repeats the permission step with VAPID keys in the environment and a normal
Chrome profile. An incognito window cannot subscribe. When the subscription
is stored, the status says alerts arrive while Pantry is closed.

## Gotchas

- A browser that stores a push subscription gets the same three alerts from
  the server, including while Pantry is closed. The page notification is the
  fallback when that subscription is not available, and it still needs the tab.
- iPhone Safari has no Notification API for a normal tab, so the queue shows
  "This browser cannot show notifications from an open tab."
- The 5-minute wait is the same gap that splits session cards. A driver proves
  it by installing `page.clock` before navigation and fast-forwarding.
- Grant notification permission only after the button is on screen. Granting
  it before load hides the button, because permission is already `granted`.
- Choosing a stock in notification selects the Stock in tab, which also sets
  the shared scanner mode, the same as tapping that tab.
