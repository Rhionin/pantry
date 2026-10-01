# Pantry verification map

This directory is the maintained source for verifying Pantry's user-facing
behavior. Read this index before driving the app, then use the matching feature
file as the recipe. The parent [SKILL.md](../SKILL.md) covers launch, doctor,
evidence, and cleanup mechanics.

## Baseline preconditions

- Launch through `scripts/pantry-verify.sh drive <driver> <evidence-subdir>`
  from the repo root. It starts the Go API on `http://127.0.0.1:18080` and the
  Vite web UI on `http://127.0.0.1:5173`, each with a fresh SQLite DB, then tears
  them down on exit. Background processes do not persist across shell calls in
  this sandbox, so launch and drive live in one command.
- External Open Food Facts lookup is disabled
  (`DISABLE_EXTERNAL_PRODUCT_LOOKUP=true`). Seed products through the API with
  `createKnownProduct` before scanning a known barcode; an unseeded barcode is
  treated as unknown and flags.
- Run `scripts/pantry-verify.sh doctor` first when anything looks off. Require
  `health: {"status":"ok"}` and the expected scanner config before driving.
- Never drive an instance this run did not start. Fixed ports with
  `--strictPort` make a leftover process fail fast instead of double-driving.

## Driving conventions

- The web UI is the primary surface. Its three routes are `Scan Queue` (`/`),
  `Inventory` (`/inventory`), and `Shopping List` (`/shopping`), reachable from
  the header links.
- Prefer ARIA roles and accessible names over CSS selectors or DOM position.
  Scanning is the core action: fill the `Barcode scanner input` textbox and press
  Enter, exactly as an HID scanner types then terminates.
- Treat every command and quoted name as literal. Product names, barcodes, and
  button labels below must be used unchanged.
- Wait for a state signal (a card appearing, a card detaching, a row's text), not
  a fixed sleep. Scan events arrive over an SSE stream and the queue reloads
  after mutations.
- Use a distinct barcode and product name per feature so runs do not collide in a
  shared or reused DB.

## Proof and skip reporting

- Capture the user action and the resulting state, not only the final screen. A
  driver asserts the card appeared, then that it left the queue, then the
  inventory or shopping change.
- Verify the side effect through a second, read-only view: `readInventory` (the
  same data the Inventory route shows) or the shopping-list API, alongside the
  visible UI text.
- `captureProof(page, name, sideEffect)` writes `<name>.aria.txt`, `<name>.png`,
  and `<name>.sideeffect.json` under `evidence/<subdir>/`. Every artifact should
  identify Pantry and the feature under test.
- Report an unreachable path with the attempted action and the unmet
  precondition. Do not report a feature verified through the API when its user
  entry point is the UI.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph of user-visible
behavior, then uses exactly these four H2 sections in order:

1. `Sub-features` lists short IDs, one line each.
2. `How to get to it (user POV)` lists every user entry point.
3. `Driving it with Playwright` starts with `Preconditions:` and pairs each user
   action with an exact command and observable result.
4. `Gotchas` lists traps that can waste or invalidate a run.

## Features

- [Stock in a scan](./stock-in.md) — scan a known barcode in stock-in mode,
  approve it, and confirm the unit lands in inventory. Proven end to end by
  `scripts/drive-stock-in.mjs`.
- [Resolve a flagged scan](./flagged-resolution.md) — an unknown barcode flags;
  create a product for it and turn it into an approvable pending scan. Proven end
  to end by `scripts/drive-flagged-resolution.mjs`.
- [Stock out oldest-first](./stock-out.md) — switch to stock-out mode, scan a
  stocked product, and confirm the earliest-expiring instance is consumed. Proven
  end to end by `scripts/drive-stock-out.mjs`.
- [Shopping list](./shopping-list.md) — derive a restock gap from a target
  quantity, or add an item manually, then mark it purchased. Proven end to end by
  `scripts/drive-shopping-list.mjs`.
- [Wipe inventory](./wipe-inventory.md) — type `WIPE INVENTORY` to clear stock
  without deleting the product lookup cache. Proven end to end by
  `scripts/drive-wipe-inventory.mjs`.
