# Stock out oldest-first

Stock-out lets a user consume a unit of a product. With the queue in stock-out
mode, scanning a stocked product and approving the scan removes one instance;
when the user makes no explicit instance choice, the earliest-expiring instance
is consumed first, so the pantry uses up soon-to-expire stock before fresher
stock.

## Sub-features

- `stock-out-mode` switching to the Stock out tab (or scanning the stock-out
  control barcode) puts new scans in `stock_out` direction.
- `stock-out-scan` scanning a stocked product in stock-out mode creates a pending
  stock-out card.
- `stock-out-oldest` approving with no instance selected removes the
  earliest-expiring instance.
- `stock-out-inventory` the remaining, later-expiring instance stays in
  inventory.

## How to get to it (user POV)

- Open `Scan Queue` (`/`), select the `Stock out` tab, and scan a product that
  already has inventory.
- Or scan the configured stock-out control barcode (default `STOCK_OUT`) to
  switch the shared scanner mode, then scan the product.

## Driving it with Playwright

Preconditions:

- App is healthy at `http://127.0.0.1:5173`.
- The product is seeded (`createKnownProduct`) and has at least two stocked
  instances with different expiration dates. Stock two in first via the
  [stock-in](./stock-in.md) recipe, giving each a distinct `Expiration date`
  (for example `2030-01-10` and `2030-12-20`).

- **Stock two instances.** For each of two expiration dates: scan in stock-in
  mode, set the card's `getByLabel('Expiration date')`, and approve. Inventory
  `instanceCount` for the product is `2`.
- **Switch to stock out.** `page.getByRole('tab', { name: 'Stock out'
  }).click()` (or scan the `STOCK_OUT` control barcode). New scans now carry
  `stock_out`.
- **Scan for stock out.** `scanBarcode(page, barcode)`; the card appears under
  the Stock out tab. Leave the instance selector unset to exercise oldest-first.
- **Approve.** `card.getByRole('button', { name: 'Approve', exact: true
  }).click()`; the card detaches.
- **Confirm oldest consumed.** Open `Inventory`, open the product's `View
  instances`, and assert the later date remains (`Expires Dec 20, 2030`) while
  the earlier date is gone (`Expires Jan 10, 2030` has count 0). Cross-check
  `readInventory` shows `instanceCount` `1`.
- **Proof.** `captureProof(page, 'stock-out', { remainingInstanceCount })` with
  the instance list visible, so the surviving expiry date is in the ARIA
  snapshot.

## Gotchas

- Stock-out needs existing inventory. Scanning a product with zero instances in
  stock-out mode has nothing to consume and shows a `No inventory item is
  available for this product` notice on the card.
- Oldest-first only applies when no specific instance is chosen. If a
  `StockOutInstanceSelector` value is set, that instance is consumed instead;
  leave it unset to verify the default.
- Expiry dates render in medium format in the UTC time zone (`Expires Jan 10,
  2030`); assert the rendered string, not the raw ISO input.
- The `Stock out` tab is a `tab` role, and its catchall view also holds flagged
  and direction-null scans, so filter by the specific `Scan <barcode>` article.
