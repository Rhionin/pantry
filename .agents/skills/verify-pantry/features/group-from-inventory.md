# Group from inventory

An ungrouped inventory product can join an existing product group from its
inventory card. A product that is already in a group says so and can move to
another group. Creating a group from Product groups opens that group so it can
be filled in immediately.

## Sub-features

- `inventory-add-to-group` on an ungrouped inventory card, Add to group lists
  existing groups and adds the product to the one chosen.
- `inventory-move-group` a product shown inside its group can move to another
  group. With no other groups, the dialog says it is already in this one.
- `create-group-opens` New group on Product groups opens the new group's page.

## How to get to it (user POV)

- Inventory (`/inventory`). Ungrouped products show Add to group. Products
  already in a group are the group card; Show products, then Move to.
- Product groups in the header menu (`/groups`), then New group.

## Driving it with Playwright

Preconditions:

- App is healthy (`doctor` passes) at `http://127.0.0.1:5173`.
- Two stocked products are each in their own group, and a third stocked product
  is in no group. Seed them with `createKnownProduct`, commit stock-in scans,
  then `POST /api/groups`.

- **Add to a group.** On Inventory, click `Add Rice crackers to a group`. The
  dialog lists the existing groups. Choose one and click `Add to group`. The
  product leaves its own card and shows under that group.
- **Move.** Show products on that group, click `Move Rice crackers to another
  group`. The dialog says `Already in` the current group and omits that group
  from the choices. Choose the other group and click `Move to this group`.
- **Create and open.** On Product groups, click `New group`, enter a name, and
  click `Create group`. The address is `/groups/:id` and the heading is the new
  name, with `Add a product` available.

## Gotchas

- Grouped products are one inventory card named for the group, not one card per
  product. Add to group is only on an ungrouped product card.
- A product can belong to one group. Moving sends `fromGroupId`. Adding without
  it is refused when the product is already grouped.
- The dialogs are `position: fixed`. Capture the viewport, not a full-page shot
  that misses the open dialog.
