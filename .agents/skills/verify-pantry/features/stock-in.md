# Stock in a scan

Stock-in lets a user scan a product's barcode while the queue is in stock-in
mode, review it as a pending scan, approve it, and have one unit of that product
appear in inventory. This is the core add-to-pantry flow.

## Sub-features

- `stock-in-scan` scanning a known barcode in stock-in mode creates a pending
  scan card stamped `stock_in`.
- `stock-in-card` the card is one row: the product name, a thumb-sized
  unit-count stepper (decrease, count, increase), Approve, and Remove. Under the
  name, a short `Add expiration` control opens the date field only when asked.
  A saved date reads `Exp <date>` (for example `Exp May 28, 2030`) and reopens
  the field with Clear and Cancel. An Open Food Facts product keeps its
  provenance label under the name. The barcode and scan time are not shown.
- `stock-in-approve` approving the card commits it and removes it from the queue.
- `stock-in-inventory` the approved unit appears on the Inventory route and in
  the inventory API.

## How to get to it (user POV)

- Open `Scan Queue` (the default route `/`) and scan into the `Barcode scanner
  input` field; the queue opens in `stock_in` mode.
- The batch path (`Select scan for batch approval` checkbox, then `Approve N
  selected` in the Approve scans panel) commits several pending scans at once.

## Driving it with Playwright

Preconditions:

- App is healthy (`doctor` passes) at `http://127.0.0.1:5173`.
- A product is seeded and bound to the barcode via `createKnownProduct`, because
  external lookup is disabled.
- The queue banner reads `Mode: stock_in`.

- **Seed the product.** Run `createKnownProduct(page, { barcode, name, category,
  unitOfMeasure })`. The product is returned and its barcode override is
  registered.
- **Open the queue.** `page.goto('/')` then wait for `getByText('Mode:
  stock_in')`. The stock-in mode banner is visible.
- **Scan.** `scanBarcode(page, barcode)` fills `getByRole('textbox', { name:
  'Barcode scanner input' })` and presses Enter. A `getByRole('article', { name:
  'Scan <barcode>' })` card appears under the Stock in tab.
- **Read the card.** The card is named `Scan <barcode>` and contains a heading
  with the product name. `getByRole('textbox', { name: 'Unit count', exact:
  true })` reads `1`.
- **Approve.** `card.getByRole('button', { name: 'Approve', exact: true
  }).click()`. The card detaches from the queue (`waitFor({ state: 'detached' })`).
- **Confirm inventory (API view).** `readInventory(page)` returns a row whose
  `item.product.name` matches and whose `instanceCount` is `1`.
- **Confirm inventory (UI view).** `getByRole('link', { name: 'Inventory'
  }).click()`, then find the article filtered by the product heading and assert
  its `1 <unitOfMeasure>` text (for example `1 carton`).
- **Proof.** `captureProof(page, 'stock-in', { instanceCount, unitOfMeasure })`
  writes the ARIA snapshot, screenshot, and side-effect JSON showing the stocked
  unit.

`scripts/drive-stock-in.mjs` is this recipe, verified end to end.

`scripts/drive-stock-in-row.mjs` proves the card at phone width (390px). The
stepper, Approve, and Remove share one 44px band after the name, and the date
field stays hidden until `Add expiration`. A saved date folds back into `Exp …`.
Two stepper taps and the date both reach inventory (3 units, each with that
date), and Remove cancels a scan without stocking it. It also asserts one
unclipped row at 360px. To show the provenance label, it marks a seeded product
as Open Food Facts in the SQLite file at `PANTRY_DB_PATH`.

## Gotchas

- Use `{ exact: true }` on the `Approve` button: `getByRole('button', { name:
  'Approve' })` also matches the `Approve N selected` batch button and the
  `Approve scans` heading region.
- The barcode field is visually hidden (`VisuallyHidden`) but focusable; drive it
  by role and accessible name, not by clicking a visible box.
- An unseeded barcode flags instead of creating a pending card. Seed first, or you
  are actually exercising the flagged-resolution feature.
- Scans arrive over `/api/events` (SSE) and the queue reloads after mutations.
  Wait for the card to appear or detach, not a fixed delay.
- The card's direction is stamped from the current scanner mode at scan time.
  A `stock_in` scan only appears under the Stock in tab.
- `getByLabel('Unit count')` also matches the `Decrease unit count` and
  `Increase unit count` buttons. Use `getByRole('textbox', { name: 'Unit count',
  exact: true })` for the field.
- A saved date is a button named by its text (`Exp May 28, 2030`). `Change
  expiration` is its description, not its name.
- At 320px the widest saved date (`Exp May 28, 2030`) is cut off with an
  ellipsis. The row driver records 320px but only asserts 360px and wider.
