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
- `shopping-purchase` `Mark <product> purchased` removes the entry from the list.
- `shopping-remove` `Remove <product>` removes a manual entry.

## How to get to it (user POV)

- Open `Shopping List` (`/shopping`) to view entries, add an item, mark
  purchased, or remove.
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
  and a `Derived` source badge.
- **Add a manual item (alternative).** In `Add an item`, choose the product in
  the `Pantry item` select, set `Quantity`, and click `getByRole('button', {
  name: 'Add to shopping list' })`. A row with a `Manual` badge appears.
- **Mark purchased.** Click `getByRole('button', { name: 'Mark <product>
  purchased' })`. The row leaves the list; an emptied list shows `Your shopping
  list is empty.`
- **Confirm inventory is unchanged by purchase.** `readInventory(page)` shows the
  same `instanceCount` as before the purchase (purchasing marks intent, it does
  not stock in).
- **Proof.** `captureProof(page, 'shopping-list', { source, quantity })` with the
  entries table (or the empty state after purchase) visible.

## Gotchas

- A derived entry only appears after a target quantity is set AND current stock is
  below it. Setting the target alone, with stock at or above target, derives
  nothing.
- Derived entries have an empty id and no action buttons until they are
  materialized; the `Mark ... purchased` and `Remove ...` buttons show the
  product name in their accessible label, so target the exact `Mark <product>
  purchased` string.
- Marking a derived shortfall purchased does not add inventory. Do not assert an
  inventory increase from the purchase step; assert it only after a subsequent
  stock-in.
- The `Pantry item` select lists inventory products by name; a product with no
  inventory will not be selectable for a manual add.
