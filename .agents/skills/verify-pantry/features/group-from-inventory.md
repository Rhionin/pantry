# Group from inventory

Inventory is one list of groups and ungrouped products. An ungrouped product
can join an existing group, or start a new one, from its menu. A product that
is already in a group is a row inside that group and can move to another
group. Creating a group opens that group so it can be filled in immediately.

## Sub-features

- `inventory-add-to-group` on an ungrouped product, the menu's Add to a group…
  opens a sheet of existing groups. Choosing one adds the product.
- `inventory-move-group` a product shown inside its group can move to another
  group. The sheet says which group it is already in and omits that group.
- `create-group-opens` Start a group with this, or New group on the add sheet,
  opens the new group's page.

## How to get to it (user POV)

- Inventory (`/inventory`). Ungrouped products have a menu with Add to a group…
  and Start a group with this. Groups expand in place; each member has Move to
  another group.
- The header has no Product groups page. `/groups` opens Inventory with the
  Groups filter.

## Driving it with Playwright

Preconditions:

- App is healthy (`doctor` passes) at `http://127.0.0.1:5173`.
- Two stocked products are each in their own group, and a third stocked product
  is in no group. Seed them with `createKnownProduct`, commit stock-in scans,
  then `POST /api/groups`.

- **Add to a group.** On Inventory, open `Actions for Rice crackers` and choose
  `Add to a group…`. The sheet is titled `Add Rice crackers to…` and lists the
  existing groups. Choose one. The product leaves its own row and shows inside
  that group.
- **Move.** Expand that group, open `Actions for Rice crackers`, and choose
  `Move to another group`. The sheet says `Already in` the current group and
  omits that group. Choose the other group.
- **Create and open.** On an ungrouped product, choose `Start a group with
  this`. The address is `/groups/:id` and the heading is the product name, with
  `Add a product` available.

## Gotchas

- Grouped products are one inventory row named for the group, not one row per
  product. Add to a group is only on an ungrouped product.
- A product can belong to one group. Moving sends `fromGroupId`. Adding without
  it is refused when the product is already grouped.
- The add sheet is a bottom drawer. Capture the viewport, not a full-page shot
  that misses the open sheet.
