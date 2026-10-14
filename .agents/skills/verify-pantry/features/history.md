# Consumption history

A product or a group has a history page that leads with how many days usually
pass between uses, then lists recent stock-ins and uses. Each move can have its
quantity changed, or be undone after a confirmation. The shelf count and the
pace follow the correction.

## Sub-features

- `history-pace` the page leads with days between uses, how many are on hand,
  about how many days that lasts, and whether use is steady, faster, or slower
  than the previous 30 days.
- `history-trail` each row names the move, the date and time, whether it was a
  scan or by hand, and the quantity change.
- `history-correct` the row menu changes the quantity or undoes the move. Undo
  asks first. Inventory and the pace update.
- `history-group` a group's history uses the same pace and lists each member
  product on its moves.
- `history-ledger` the History tab lists every stock in and stock out, newest
  first, grouped by day. Each row shows the product photo and name, whether it
  was stocked in or used, the amount, and the time. All, Stocked in, and Used
  filter the direction. Search matches a product or a group. The row menu
  changes the quantity or undoes the move, the same actions as one product.
  A pulse chart above the days draws one line for stocked-in above a zero line
  and one line for used below it, for the last 7, 30, or 90 days. Overlapping
  amounts add. The opening stock-in is the start of the chart when it falls
  inside the range, so later moves set the scale. The chart shows unit ticks,
  date ticks, and the range totals. A tap or hover reads that day's stocked-in
  and used counts. It uses the same filters and search.

## How to get to it (user POV)

- On Inventory, a product row's `History` button opens `/inventory/:itemId/history`.
- A group row's `History` link opens `/groups/:groupId/history`.
- Inside an expanded group, a member's menu has `View history`.
- Group detail has a `History` link beside `Inventory`.
- The header `History` tab opens `/history`, the combined ledger. Proven by
  `scripts/drive-stock-ledger.mjs`.

## Driving it with Playwright

Preconditions:

- App is healthy at `http://127.0.0.1:5173`.
- Opening is finished (`POST /api/onboarding/complete`) so later scans count as use.
- Products are seeded with `createKnownProduct`, then stocked in and used by
  scanning and confirming. Proven by `scripts/drive-history.mjs`.

- **Open a product history.** From Inventory, choose the product's `History`
  button. The heading is the product name. With enough spread-out uses the page
  shows `days between uses`.
- **Change a quantity.** Open `Actions for …`, choose `Change quantity`, set the
  number, and choose `Save quantity`. The on-hand count and the pace change, and
  `GET /api/inventory` reports the same count.
- **Undo.** Open the same menu, choose `Undo this use`, then `Keep it` leaves
  the shelf unchanged. Choosing `Undo this use` in the dialog puts the units
  back and the pace changes again.
- **Open a group history.** Select the products, name a group, choose
  `Group these`, then the group's `History` link. Member names appear on the
  moves.
- **Open the household ledger.** Choose `History` in the header. Moves are
  grouped under `Today` and `Yesterday`, newest first, with the product name,
  `Stocked in` or `Used`, and the amount. `Stocked in` and `Used` filter the
  list. `Search products or groups` matches a product or a group. The row menu
  uses `Change quantity` and `Undo this stock in` or `Undo this use`.
- **Read the pulse chart.** Above the days, `30 days` is pressed. One line
  rises for stocked in and one line falls for used. Overlapping moves add
  together. Unit ticks, date ticks, and the range totals sit with the chart.
  When the opening stock-in is inside the range, the chart starts there and
  says so. `7 days` and `90 days` change the window. A tap or hover shows that
  day's date, stocked-in count, and used count, and marks the day in the list.
  The chart follows the direction filter and the search.

## Gotchas

- During the opening snapshot, stock movement does not write consumption, so the
  pace stays unknown until opening is finished.
- A brand-new cluster of uses on the same day does not show a pace. The screen
  says `Pace not known yet` until two uses are at least a week apart.
- `Unit count` on a scan card also matches the stepper buttons. The number field
  is the textbox named `Unit count`.
- The history page has its own `Inventory` link, so the header one is
  `navigation` named `Sections`.
