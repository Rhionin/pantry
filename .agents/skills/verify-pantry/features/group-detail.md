# Group detail

The group detail page is where a product group is renamed, given a restocking
rule, given a keep-on-hand target, and where products are added, removed, or
moved. The yellow Pick a rule badge opens the same rule sheet used from
inventory. Products that are not already in a group can be searched and added
here.

## Sub-features

- `group-detail-rule` tapping Pick a rule, or the confirmed rule badge, opens
  the bottom rule sheet. Saving a rule replaces the yellow badge with the rule
  name.
- `group-detail-add` search ungrouped products, select one or more, and add
  them to this group. A supply-setting conflict offers the account window, 24
  ounces, or 3 months.

## How to get to it (user POV)

- Open Product groups from the header menu, then open a group. The route is
  `/groups/:id`.

## Driving it with Playwright

Preconditions:

- App is healthy (`doctor` passes) at `http://127.0.0.1:5173`.
- Two products are members of a group and a third product is stocked but not in
  any group. Seed them with `createKnownProduct`, commit stock-in scans, then
  `POST /api/groups`.

- **Open the rule sheet.** Go to `/groups/:id`. Click `Pick a rule`. The
  `Gatorade powder` dialog shows the Rule field, a Next trip line, and
  `Save rule`.
- **Save a rule.** Choose `Always my favorite`, choose the product under
  `Always buy`, and click `Save rule`. The badge becomes `Always my favorite`
  and `GET /api/groups/:id` reports `ruleConfirmed: true`.
- **Add a product.** Search `glacier`, check `Gatorade Glacier Freeze`, and
  click `Add to this group`. The product appears in the member list and leaves
  the ungrouped list.

## Gotchas

- The rule sheet is a bottom drawer. A full-page screenshot can miss it; capture
  the viewport while the dialog is open.
- `Use the account window` on the page clears the group's target. The add
  conflict uses `Keep the account window`, `Keep 24 ounces`, and `Keep 3 months`
  so the two actions stay distinct.
