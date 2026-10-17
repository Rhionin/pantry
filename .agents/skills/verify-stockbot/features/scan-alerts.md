# Scan alerts

Scan alerts are turned on from Settings, for this browser only. The switch
asks for notification permission, then stores a push subscription when this
pantry has a push key. After that, a failed lookup and an unrecognized product
each raise a browser notification immediately. A stock in or stock out batch
of recognized scans raises one notification 5 minutes after the last scan in
that batch. Clicking a notification focuses the scan queue on the tab that
holds those scans. Turning the switch off stops those alerts and removes this
browser's push subscription.

## Sub-features

- `alert-permission` Settings shows "Scan alerts on this device". The switch
  starts off. Loading Scan Queue does not open the browser permission prompt.
- `alert-immediate` a failed lookup and an unrecognized barcode notify at once
  while the switch is on.
- `alert-batch` recognized stock in and stock out scans share a 5-minute quiet
  period per direction. One notification names how many scans are still waiting.
- `alert-open` choosing the notification focuses Scan Queue. A stock in notice
  selects Stock in; an unrecognized barcode with no direction selects Stock out.

## How to get to it (user POV)

- Open the header menu and choose `Settings` (`/settings`). The switch sits
  under the default group rule.
- Turn on `Scan alerts on this device`, then allow notifications in the
  browser prompt.
- Return to `Scan Queue` (`/`). Leave the tab open. Scan from this page or
  from the scanner.

## Driving it with Playwright

Preconditions:

- App is healthy (`doctor` passes) at `http://127.0.0.1:5173`.
- Notification permission starts unset, so the switch is off and the status
  reads `Off`.
- Products for a batch are seeded with `createKnownProduct`. An unseeded
  barcode is the unrecognized-product case. External lookup is off.
- The queue banner reads `Mode: stock_in` before the batch scan.

- **See the switch.** On Settings,
  `getByRole('switch', { name: 'Scan alerts on this device' })` is off. Its
  description says this browser only. The status line reads `Off`.
- **Allow alerts.** `context.grantPermissions(['notifications'])`, then turn
  the switch on. The status line reads `On: alerts arrive even when Stockbot is
  closed` when this browser stored a push subscription. Otherwise it names why
  push failed and still says alerts arrive while this tab is open, with a
  `Retry` button.
- **Unrecognized product.** `scanBarcode` an unseeded barcode. The card shows
  `Flagged`. The notification title is `Unrecognized product`.
- **Quiet batch.** Scan a seeded product, then advance the page clock by 5
  minutes. The notification title is `Stock in batch is ready`.

`scripts/drive-scan-alerts.mjs` is this recipe. `scripts/drive-scan-push.mjs`
repeats the permission step with VAPID keys in the environment and a normal
Chrome profile. An incognito window cannot subscribe. When the subscription
is stored, the status says alerts arrive even when Stockbot is closed.

`scripts/drive-visible-scan-alert.mjs` delivers one push while Inventory is
open. The alert is a toast under the header, not a system notification.
**Open the scan queue** selects Stock in. A wide window keeps that toast
under the shorter header, and the header links stay clickable.

## Gotchas

- A browser that stores a push subscription gets the same three alerts from
  the server. While a Stockbot window is visible, the alert is an in-app toast.
  A hidden or closed window still gets the system notification. The page
  notification is the fallback when that subscription is not available, and it
  still needs the tab.
- A browser with no Notification API shows "This browser cannot show
  notifications." on Settings and the switch stays off.
- The 5-minute wait is the same gap that splits session cards. A driver proves
  it by installing `page.clock` before navigation and fast-forwarding.
- Leave notification permission unset until the switch is on screen. Granting
  it before Settings loads turns the switch on, because a granted permission
  is the old opt-in.
- Choosing a stock in notification selects the Stock in tab, which also sets
  the shared scanner mode, the same as tapping that tab.
