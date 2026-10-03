# Shopping list

The shopping list tells a user what to buy. Entries are either derived
automatically from a product's target quantity versus its current stock, or added
manually. A user can add an item, see its source, mark it purchased, or remove
it. Marking a derived shortfall purchased does not change inventory on its own;
inventory changes only when the bought item is later stocked in.

## Sub-features

- `shopping-target` setting a target quantity on an inventory item (via `Get
  suggestion` then `Save manual target`) enables automatic restock derivation.
- `shopping-derived` when stock drops below the target, a `Derived` entry appears
  with the shortfall quantity.
- `shopping-manual-add` a user adds a `Manual` entry by choosing a pantry item
  and quantity in the `Add an item` form.
- `shopping-purchase` `Mark <product> purchased` removes the entry from the list
  (works on both materialized derived entries and manual entries).
- `shopping-remove` `Remove <product>` removes an entry.
- `shopping-store-setup` client id, client secret, redirect URI, and modality are
  edited from the store setup menu. They are not on the shopping page.

## How to get to it (user POV)

- Open `Shopping List` (`/shopping`) to view entries, add an item, mark
  purchased, or remove. With a store connected, `Add to <store> cart`,
  `Disconnect`, and `Start a new cart` stay on the page.
- Change store credentials from the same page: `Kroger setup`, then `Edit
  credentials`. Saving and `Clear saved credentials` stay in that dialog.
- Set a target quantity from `Inventory` (`/inventory`): open a product's `View
  instances`, then use its `Target quantity` panel.

## Driving it with Playwright

Preconditions:

- App is healthy at `http://127.0.0.1:5173`.
- A product is seeded and has some inventory (stock in at least two units via the
  [stock-in](./stock-in.md) recipe) so a shortfall can be derived after
  consumption.

- **Set a target quantity.** On `Inventory`, open the product and its instances,
  then in the target panel click `getByRole('button', { name: 'Get suggestion'
  })`. With little history the panel offers manual entry: fill
  `getByLabel('Manual target quantity')` with the target (for example `2`) and
  click `getByRole('button', { name: 'Save manual target' })`. Expect
  `getByText('Target quantity set to 2.')`.
- **Create a shortfall.** Stock out one unit (see [stock-out](./stock-out.md)) so
  current stock falls below the target.
- **See the derived entry.** Open `Shopping List`. In the `Shopping list entries`
  table, the product's row shows the shortfall quantity (for example `1 <unit>`)
  and a `Derived` source badge. The backend materializes derived entries
  (`shopping.SyncDerivedItems`), so this row carries a real id and exposes the
  `Mark <product> purchased` / `Remove <product>` actions.
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
`Save credentials`) are absent until `Kroger setup` → `Edit credentials`.
`scripts/drive-shopping-connected.mjs` proves that phone layout with Kroger
already connected, then saves a modality change from the dialog.

## Gotchas

- A derived entry only appears after a target quantity is set AND current stock is
  below it. Setting the target alone, with stock at or above target, derives
  nothing.
- The API-backed list the page shows materializes derived entries
  (`shopping.SyncDerivedItems`), so a `Derived` row carries a real id and DOES
  render `Mark <product> purchased` / `Remove <product>` buttons — you can mark
  the derived shortfall purchased directly. The empty-id/no-action-buttons case
  applies only to the frontend-only pure fallback (`deriveShoppingListEntries`),
  not the list the page renders. The buttons carry the product name in their
  accessible label, so target the exact `Mark <product> purchased` string.
- Marking a shortfall purchased dismisses it; the purchased gap stays hidden
  until the on-hand quantity changes, so the row does not immediately reappear.
- Marking a derived shortfall purchased does not add inventory. Do not assert an
  inventory increase from the purchase step; assert it only after a subsequent
  stock-in.
- The `Pantry item` select lists inventory products by name; a product with no
  inventory will not be selectable for a manual add.
- Store credential fields are not rendered on the shopping page. Open
  `getByRole('button', { name: 'Kroger setup' })`, then
  `getByRole('menuitem', { name: 'Edit credentials' })`. A saved secret still
  shows `A client secret is saved and is not shown.` and `Clear saved
  credentials` inside that dialog.
