# Scan sessions

Stock in and Stock out group pending and flagged scans into session cards.
Scans stay in one card while each consecutive pair is under 5 minutes apart;
a gap of 5 minutes or more starts another card. The newest session starts
open and shows the same scan rows as before, including the unit-count
stepper. Older sessions start closed. Each open session has its own Select
all, and Confirm commits only the scans that are selected.

## Sub-features

- `session-gap` scans less than 5 minutes apart share a card; a 5-minute gap
  starts a new card. One card may cover more than 5 minutes in total.
- `session-expand` the newest card is open; older cards show the time range
  and scan count until opened.
- `session-select-all` Select all on an open card selects or clears only the
  eligible scans in that card.
- `session-confirm` the toolbar button reads Confirm N and commits the
  selected ids with `commit: true`.

## How to get to it (user POV)

- Open `Scan Queue` (`/`). Stock in is the default view.
- Scan several products. Scans from one trip appear in the open card. A later
  trip, after at least 5 minutes, appears as a closed card underneath.

## Driving it with Playwright

Preconditions:

- App is healthy (`doctor` passes) at `http://127.0.0.1:5173`.
- Products are seeded with `createKnownProduct` because external lookup is off.
- The queue banner reads `Mode: stock_in`.
- `POST /api/scans` stamps `scannedAt` with the current time, so a driver that
  needs a 5-minute gap updates `scan_entries.scanned_at` in `PANTRY_DB_PATH`
  and reloads. That is fixture setup, not the user action.

- **Seed and scan.** Create one product per barcode, then `scanBarcode` each
  one. Each row is an article `Scan <barcode>` inside the open session.
- **Read the open card.** The session toggle is a button with `aria-expanded`
  `true`. It shows the earliest–latest time and `N scans · stock in`. Each
  pending row still has `Unit count` and Increase/Decrease unit count.
- **Read a closed card.** A button with `aria-expanded` `false` shows the time
  range and scan count. Its rows are not in the page until the button is clicked.
- **Select the open card.** `getByRole('checkbox', { name: /Select all eligible
  scans in/ })` inside that session checks only that card's pending rows.
- **Confirm.** `getByRole('button', { name: 'Confirm N selected scans' })`
  posts `{ scanEntryIds, commit: true }`. Per-card confirm is
  `getByRole('button', { name: 'Confirm', exact: true })`.

`scripts/drive-scan-sessions.mjs` is this recipe.

## Gotchas

- Sessions are computed in the browser from `scannedAt`. Nothing stores a
  batch id.
- Stock in and Stock out are grouped separately. A scan with no direction stays
  on Stock out.
- The toolbar Select all (`Select all eligible scans for batch approval`) still
  covers every eligible scan in the current tab, including closed cards.
- Use `{ exact: true }` on the per-card `Confirm` button so it does not match
  `Confirm N selected scans`.
