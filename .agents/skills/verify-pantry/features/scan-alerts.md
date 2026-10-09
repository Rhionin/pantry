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
  `Notify me about scans`. The button leaves the page.
- **Unrecognized product.** `scanBarcode` an unseeded barcode. The card shows
  `Flagged`. The notification title is `Unrecognized product`.
- **Quiet batch.** Scan a seeded product, then advance the page clock by 5
  minutes. The notification title is `Stock in batch is ready`.

`scripts/drive-scan-alerts.mjs` is this recipe.

## Gotchas

- Alerts are sent only while this tab's JavaScript is running. Closing the tab
  or suspending the browser stops them. There is no server push.
- iPhone Safari has no Notification API for a normal tab, so the queue shows
  "This browser cannot show notifications from an open tab." A Home Screen icon
  does not change that: this app does not register a service worker.
- The 5-minute wait is the same gap that splits session cards. A driver proves
  it by installing `page.clock` before navigation and fast-forwarding.
- Grant notification permission only after the button is on screen. Granting
  it before load hides the button, because permission is already `granted`.
- Choosing a stock in notification selects the Stock in tab, which also sets
  the shared scanner mode, the same as tapping that tab.
