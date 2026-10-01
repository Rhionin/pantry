# Wipe inventory

Wipe inventory empties stock for the current user while leaving the product
lookup cache in place. The control sits at the bottom of the Inventory page and
will not submit until the user types `WIPE INVENTORY` exactly.

## Sub-features

- `wipe-friction` the confirm button stays disabled until the typed phrase is
  exactly `WIPE INVENTORY`.
- `wipe-stock` confirming removes every inventory row and its shopping-list
  entries.
- `wipe-lookup-kept` the product and its barcode still resolve after the wipe.

## How to get to it (user POV)

- Open `Inventory` (`/inventory`). The `Wipe inventory` button is at the bottom
  of the page.
- The dialog is titled `Wipe inventory`. Confirm is the `Confirm wipe` button.

## Driving it with Playwright

Preconditions:

- App is healthy (`doctor` passes) at `http://127.0.0.1:5173`.
- A product is seeded and bound to the barcode via `createKnownProduct`, because
  external lookup is disabled.
- That product has been stocked in, so the Inventory route shows one unit.

- **Open the dialog.** `page.getByRole('button', { name: 'Wipe inventory' }).click()`.
  A dialog named `Wipe inventory` appears, and `Confirm wipe` is disabled.
- **Reject a near miss.** Fill `Type WIPE INVENTORY to confirm` with
  `wipe inventory`. `Confirm wipe` stays disabled.
- **Confirm.** Fill the same textbox with `WIPE INVENTORY`, then click
  `Confirm wipe`. The page shows `Your inventory is empty.`
- **Check the side effect.** `readInventory` returns an empty list, and
  `GET /api/products/lookup?barcode=<barcode>` still returns the seeded product.

## Gotchas

- The phrase is case-sensitive and is not trimmed. `wipe inventory` does not
  enable confirm.
- The wipe also removes shopping-list rows that pointed at the deleted items.
  Pending scans stay in the queue.
- The button is an outline control at the bottom of the page, not next to
  per-item actions.
