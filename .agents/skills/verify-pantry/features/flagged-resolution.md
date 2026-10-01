# Resolve a flagged scan

When a scanned barcode matches no known product (external lookup is off, or the
barcode is genuinely unknown), the scan is flagged instead of becoming
approvable. Resolution lets a user attach an existing product or create a new one
on the spot, turning the flagged scan into a normal pending scan.

## Sub-features

- `flag-unknown` scanning an unseeded barcode creates a card with a `Flagged`
  badge and no approval checkbox.
- `flag-create` the card's `Create new product` fieldset creates a product and
  binds it to the barcode.
- `flag-contribute` sharing is off until `Let me contribute products I type in`
  is switched on, and a product is still not sent unless `Contribute this
  product` is checked. With no Product Opener account, the product stays local.
- `flag-search` an existing product can be attached via the `Search products`
  combobox and `Use selected product`.
- `flag-becomes-pending` after resolution the card loses `Flagged`, shows the
  product name, and becomes an approvable pending scan.

## How to get to it (user POV)

- Open `Scan Queue` (`/`) and scan a barcode that no product override covers. The
  flagged card renders in place, under the Stock out tab when its direction is
  still null.

## Driving it with Playwright

Preconditions:

- App is healthy at `http://127.0.0.1:5173`.
- The chosen barcode has NO product seeded for it (do not call
  `createKnownProduct` for this barcode).

- **Scan an unknown barcode.** `scanBarcode(page, barcode)`. The card appears and
  `card.getByText('Flagged', { exact: true })` is visible.
- **Fill the new-product form.** In the card's `Create new product` fieldset,
  `card.getByLabel('Product name').fill(name)`,
  `card.getByLabel('Category').fill(category)`, and
  `card.getByLabel('Unit of measure').fill(unitOfMeasure)`.
- **Create and use.** `card.getByRole('button', { name: 'Create and use product'
  }).click()`. The `Flagged` badge disappears (`toHaveCount(0)` / wait for
  detached), the card shows a heading with the product name, and a `Select scan
  for batch approval` checkbox appears.
- **Confirm it is now approvable.** The card now renders like a pending stock
  card; approve it with `card.getByRole('button', { name: 'Approve', exact: true
  }).click()` and confirm inventory as in [stock-in](./stock-in.md), or stop once
  the flag is cleared if you are only verifying resolution.
- **Proof.** `captureProof(page, 'flagged-resolution', { barcode, product })`
  after the flag clears, so the ARIA snapshot shows the product name in place of
  the `Flagged` badge.

`scripts/drive-flagged-resolution.mjs` is this recipe, verified end to end. It
scans the unknown barcode in stock_in mode, so the resolved entry carries a
direction and is approvable; the driver approves it and confirms `1 can` lands
in inventory.

## Gotchas

- The distinguishing signal is the `Flagged` badge, not the absence of a card. A
  flagged card still renders; do not confuse it with a missing scan.
- A newly created product must have a non-empty `Product name`; the `Create and
  use product` button is disabled until then.
- Resolution registers a barcode override, so re-scanning the same barcode later
  in the same DB will no longer flag. Use a fresh barcode per run.
- The combobox `Search products` opens on focus and filters as you type; when
  attaching an existing product, wait for the option before `Use selected
  product`, which is disabled until a product is selected.
- Leave `Let me contribute products I type in` off to resolve a flagged scan
  without sharing. The create button does not contribute unless that switch is
  on and `Contribute this product` is also checked.
