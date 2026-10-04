# Stock out oldest-first

Stock-out lets a user consume a unit of a product. With the queue in stock-out
mode, scanning a stocked product and approving the scan removes one instance;
when the user makes no explicit instance choice, the earliest-expiring instance
is consumed first, so the pantry uses up soon-to-expire stock before fresher
stock.

## Sub-features

- `stock-out-mode` scanning the stock-out control barcode puts new scans in
  `stock_out` direction (the Stock out tab only changes the view, not the mode).
- `stock-out-scan` scanning a stocked product in stock-out mode creates a pending
  stock-out card.
- `stock-out-oldest` approving with no instance selected removes the
  earliest-expiring instance.
- `stock-out-inventory` the remaining, later-expiring instance stays in
  inventory.

## How to get to it (user POV)

- Open `Scan Queue` (`/`), scan the configured stock-out control barcode
  (default `STOCK_OUT`) to switch the shared scanner mode, then scan a product
  that already has inventory. The `Stock out` tab changes the view you look at,
  but the scan's direction comes from the scanner mode, not the tab.

## Driving it with Playwright

Preconditions:

- App is healthy at `http://127.0.0.1:5173`.
- The product is seeded (`createKnownProduct`) and has at least two stocked
  instances with different expiration dates. Stock two in first via the
  [stock-in](./stock-in.md) recipe, giving each a distinct `Expiration date`
  (for example `2030-01-10` and `2030-12-20`).

- **Stock two instances.** For each of two expiration dates: scan in stock-in
  mode, open `Add expiration`, set `getByLabel('Expiration date')`, and approve.
  Inventory `instanceCount` for the product is `2`.
- **Switch to stock out.** Scan the `STOCK_OUT` control barcode via
  `setScannerMode(page, 'stock_out')` and wait for the `Mode: stock_out` banner.
  New scans now carry `stock_out`. The Stock in/Stock out Tabs only change the
  VIEW, not the scanner mode, so clicking the tab alone leaves scans in
  `stock_in`.
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

`scripts/drive-stock-out.mjs` is this recipe, verified end to end.

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
