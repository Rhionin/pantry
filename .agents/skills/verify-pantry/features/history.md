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

## How to get to it (user POV)

- On Inventory, a product row's `History` button opens `/inventory/:itemId/history`.
- A group row's `History` link opens `/groups/:groupId/history`.
- Inside an expanded group, a member's menu has `View history`.
- Group detail has a `History` link beside `Inventory`.

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

## Gotchas

- During the opening snapshot, stock movement does not write consumption, so the
  pace stays unknown until opening is finished.
- A brand-new cluster of uses on the same day does not show a pace. The screen
  says `Pace not known yet` until two uses are at least a week apart.
- `Unit count` on a scan card also matches the stepper buttons. The number field
  is the textbox named `Unit count`.
- The history page has its own `Inventory` link, so the header one is
  `navigation` named `Sections`.
