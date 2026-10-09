# Scan alerts

Scan alerts are on by default. The first pointer, key, or scan in a browser
asks for notification permission once, and only when permission is still
undecided and the person has not switched alerts off. That ask is remembered
in this browser, so a later visit does not prompt again.

Settings (`/settings`) has a Scan alerts switch. On sends alerts. Off stops
them and leaves the browser permission as it is. Switching on while permission
is still undecided asks again. When the browser has blocked notifications, the
switch stays on and a note says to re-allow them in the browser's site
settings. A browser with no Notification API keeps the switch and says it
cannot show notifications from an open tab.

After permission is granted, a failed lookup and an unrecognized product each
raise a browser notification immediately. A stock in or stock out batch of
recognized scans raises one notification 5 minutes after the last scan in that
batch. Clicking a notification focuses the scan queue on the tab that holds
those scans.

## Sub-features

- `alert-permission` the first pointer, key, or scan asks once. Loading a page
  does not open the browser permission prompt. The Scan alerts switch in
  Settings is the place to turn alerts off, or to ask again by turning them
  back on while permission is undecided.
- `alert-immediate` a failed lookup and an unrecognized barcode notify at once.
- `alert-batch` recognized stock in and stock out scans share a 5-minute quiet
  period per direction. One notification names how many scans are still waiting.
- `alert-open` choosing the notification focuses Scan Queue. A stock in notice
  selects Stock in; an unrecognized barcode with no direction selects Stock out.

## How to get to it (user POV)

- Open the header menu and choose `Settings` (`/settings`). Scan alerts is on.
  Its description says failed scans and unrecognized products alert right away.
- Leave the tab open. Scan from the scan queue or from the scanner.
- Turn Scan alerts off to stop notifications without changing the browser
  permission. Turn it back on to send them again.

## Driving it with Playwright

Preconditions:

- App is healthy (`doctor` passes) at `http://127.0.0.1:5173`.
- Products for a batch are seeded with `createKnownProduct`. An unseeded
  barcode is the unrecognized-product case. External lookup is off.
- The queue banner reads `Mode: stock_in` before the batch scan.
- Headless Chromium reports notification permission as `denied`, which is how
  the blocked note is shown. Installed Chrome with `notifications` granted
  shows the switch on and no blocked note. Headless stays denied even after
  `grantPermissions`.

- **Switch on.** Open Settings. `getByRole('switch', { name: 'Scan alerts' })`
  is checked. Its description says failed scans and unrecognized products alert
  right away. The blocked note is absent when permission is granted.
- **Denied.** With permission denied, the same switch is checked and the page
  says `Scan alerts are blocked. Re-allow notifications for this site in the
  browser's site settings.`
- **Unrecognized product.** On a context with notifications granted,
  `scanBarcode` an unseeded barcode. The card shows `Flagged`. The notification
  title is `Unrecognized product`.
- **Quiet batch.** Scan a seeded product, then advance the page clock by 5
  minutes. The notification title is `Stock in batch is ready`.

`scripts/drive-scan-alerts.mjs` is this recipe.

## Gotchas

- Alerts are sent only while this tab's JavaScript is running. Closing the tab
  or suspending the browser stops them. There is no server push.
- A browser without `Notification` shows `This browser cannot show
  notifications from an open tab.` under Scan alerts. The switch stays usable
  so the preference is still stored.
- The automatic ask runs on the first `pointerdown`, `keydown`, or scan event.
  A click on the Scan alerts switch does not count: turning the switch off must
  not open the prompt, and turning it on asks on its own when permission is
  still undecided.
- The ask is stored under `pantry-scan-alerts-asked`. Alerts are on unless
  `pantry-scan-alerts` is `off`.
- The 5-minute wait is the same gap that splits session cards. A driver proves
  it by installing `page.clock` before navigation and fast-forwarding.
- Choosing a stock in notification selects the Stock in tab, which also sets
  the shared scanner mode, the same as tapping that tab.
