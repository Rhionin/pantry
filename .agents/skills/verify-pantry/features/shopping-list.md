# Shopping list

The shopping list tells a user what to buy. Before the opening scan is marked
complete, only rows the user added by hand appear. After that, the list replaces
what was used until a steady rate qualifies. A user can add an item, see its
source and note, mark it purchased, or remove it. Marking a derived row purchased
does not change inventory on its own.

## Sub-features

- `shopping-opening` the Inventory page shows `Opening inventory` until `This
  scan is complete`. That moment is the only start date.
- `shopping-derived` after the start, using a unit adds a `Derived` entry whose
  note is `replacing N you used`.
- `shopping-manual-add` a user adds a `Manual` entry by choosing a pantry item
  and quantity in the `Add an item` form.
- `shopping-purchase` `Mark <product> purchased` removes the entry from the list
  (works on both materialized derived entries and manual entries).
- `shopping-remove` `Remove <product>` removes an entry.
- `shopping-store-setup` client id, client secret, redirect URI, and modality are
  edited from the header menu. They are not on the shopping page.

## How to get to it (user POV)

- Open `Shopping List` (`/shopping`) to view entries, add an item, mark
  purchased, or remove. With a store connected, `Add to <store> cart` and
  `Start a new cart` stay on the page.
- Change the store connection from the header menu: `Menu`, then `Manage
  Kroger connection`. That dialog edits credentials (`Save credentials`,
  `Clear saved credentials`) and, only while Kroger is connected, offers
  `Disconnect Kroger`. The form does not replace the shopping page. The same
  menu reaches `Diagnostics`. The build id is in the menu's `Build` section,
  not in the action list.
- Mark the opening scan complete from `Inventory` (`/inventory`). The banner is
  the only place that start date is set.

## Driving it with Playwright

Preconditions:

- App is healthy at `http://127.0.0.1:5173`.
- A product is seeded and has some inventory (stock in at least two units via the
  [stock-in](./stock-in.md) recipe) so a shortfall can be derived after
  consumption.

- **Finish the opening scan.** On `Inventory`, click `getByRole('button', {
  name: 'This scan is complete' })`. The `Opening inventory` alert disappears.
- **Use one unit.** Stock out one unit (see [stock-out](./stock-out.md)).
- **See the derived entry.** Open `Shopping List`. In the `Shopping list entries`
  table, the product's row shows `1 <unit>`, the note `replacing 1 you used`,
  and a `Derived` source badge. The row has a real id and exposes
  `Mark <product> purchased` / `Remove <product>`.
- **Mark purchased.** Click `getByRole('button', { name: 'Mark <product>
  purchased' })` on the derived row. The row leaves the list; an emptied list
  shows `Your shopping list is empty.` (A `Manual` entry added through the `Add
  an item` form — `Pantry item` select + `Quantity` + `Add to shopping list` —
  behaves the same way.)
- **Confirm inventory is unchanged by purchase.** `readInventory(page)` shows the
  same `instanceCount` as before the purchase (purchasing marks intent, it does
  not stock in).
- **Proof.** `captureProof(page, 'shopping-list', { source, quantity })` with the
  entries table (or the empty state after purchase) visible.

`scripts/drive-shopping-list.mjs` is this recipe, verified end to end. It marks
the materialized derived row purchased directly and confirms inventory stays at
`1 bag`.

Credential fields (`Client ID`, `Client secret`, `Redirect URI`, `Modality`,
`Save credentials`) are absent until `Menu` → `Manage Kroger connection`.
`scripts/drive-shopping-connected.mjs` proves that phone layout with Kroger
already connected, then saves a modality change from the dialog. `Disconnect
Kroger` is in that same dialog, only while connected, not on the shopping page.
When Kroger is not connected yet, the same menu item opens the dialog so
credentials can still be edited; the form does not replace the shopping page.

## Gotchas

- Before `This scan is complete`, the shopping list shows only rows added by
  hand. Opening scans are not use and do not start a rate.
- A `Derived` row carries a real id and renders `Mark <product> purchased` /
  `Remove <product>`. The buttons carry the product name in their
  accessible label, so target the exact `Mark <product> purchased` string.
- Marking a shortfall purchased dismisses it; the purchased gap stays hidden
  until the on-hand quantity changes, so the row does not immediately reappear.
- Marking a derived shortfall purchased does not add inventory. Do not assert an
  inventory increase from the purchase step; assert it only after a subsequent
  stock-in.
- The `Pantry item` select lists inventory products by name; a product with no
  inventory will not be selectable for a manual add.
- Store credential fields are not rendered on the shopping page. Open
  `getByRole('button', { name: 'Menu' })`, then
  `getByRole('menuitem', { name: 'Manage Kroger connection' })`. A saved secret
  still shows `A client secret is saved and is not shown.` and `Clear saved
  credentials` inside that dialog. While connected, the dialog also shows
  `Disconnect Kroger`.
