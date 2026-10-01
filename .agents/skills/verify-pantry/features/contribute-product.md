# Contribute a typed-in product

A product someone types in because no open database knew the barcode can be
shared back, but only after two explicit choices: a household switch that
defaults off, and a per-product checkbox that also defaults off. Without a
Product Opener account, the product is saved here and nothing is sent.

## Sub-features

- `share-off` the switch `Let me contribute products I type in` is unchecked,
  and `Contribute this product` is not shown.
- `share-on-still-local` turning the switch on reveals an unchecked
  `Contribute this product` checkbox and says this Pantry is not signed in.
- `share-one-product` checking the box and creating the product records a local
  contribution and still resolves the flagged scan.

## How to get to it (user POV)

- Open `Scan Queue` (`/`) and scan a barcode that no product override covers.
  The flagged card's `Create new product` fieldset holds the switch.
- After the product is in inventory, `Edit product` on that item repeats the
  same choice for a later correction.

## Driving it with Playwright

Preconditions:

- App is healthy at `http://127.0.0.1:5173`.
- The chosen barcode has NO product seeded for it.
- No `PRODUCT_OPENER_USER_ID` / `PRODUCT_OPENER_PASSWORD` are set, and external
  lookup is disabled, so a contribution cannot leave the machine.

- **Scan an unknown barcode.** `scanBarcode(page, barcode)`. The card shows
  `Flagged`.
- **Confirm sharing is off.** The switch named `Let me contribute products I
  type in` is unchecked. Mantine hides the native input, so read it with
  `includeHidden: true`. `Contribute this product` is absent from the DOM.
- **Turn the household switch on.** Click the visible text `Let me contribute
  products I type in`. `Contribute this product` appears unchecked, with text
  that this Pantry is not signed in.
- **Opt in for this product.** Click the visible text `Contribute this
  product`, choose `Open Products Facts` in `Open database`, fill the product
  fields, and click `Create and use product`.
- **Confirm it stayed local and the scan resolved.** The card loses `Flagged`,
  shows the product name, and `GET /api/contributions` has one
  `not_configured` row for that barcode. `GET /api/settings/contribution`
  reports `enabled: true` and `configured: false`.
- **Edit the stocked product.** Approve the card, open Inventory, `View
  instances`, then `Edit product`. Wait until `Product name` shows the saved
  name, change it, check `Contribute this product`, and `Save product`. The
  inventory heading updates and `GET /api/contributions` has two
  `not_configured` rows for that barcode.
- **Proof.** `captureProof(page, 'contribute-product', { barcode, product,
  contributionStatus: 'not_configured' })` after the edit.

`scripts/drive-contribute-product.mjs` is this recipe.

## Gotchas

- Checking the product box while the switch is off is impossible: the box is
  not rendered. Sending `contribute: true` while the setting is off does not
  call upstream.
- The notice that nothing was sent is a toast. The durable proof is
  `GET /api/contributions`, not the toast, which can dismiss.
- A product that already has upstream provenance is not offered the checkbox
  on `Edit product`.
