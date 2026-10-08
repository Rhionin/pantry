// drive-flagged-resolution.mjs — proves the flagged-resolution flow end to end
// through the web UI.
//
// User story: an unknown barcode (no product seeded) is scanned in stock-in
// mode; it appears as a flagged card that cannot be approved; the user creates a
// product for it on the spot via the card's `Create new product` fieldset; the
// flag clears, the card shows the product name and a batch-approval checkbox,
// and — because it was scanned in stock_in mode it carries a direction — the
// resolved scan is approvable and lands one unit in inventory. Run through
// scripts/pantry-verify.sh:
//   scripts/pantry-verify.sh drive scripts/drive-flagged-resolution.mjs flagged-resolution
import {
  openBrowser, scanBarcode, captureProof, readInventory, assert,
} from './harness.mjs';

// A DISTINCT, unseeded barcode: do NOT createKnownProduct for it, so the scan
// flags. The product is created in-UI as part of the flow under test.
const BARCODE = '910000000002';
const PRODUCT = 'Verify Flagged Beans';
const CATEGORY = 'Canned goods';
const UNIT = 'can';

const { browser, page } = await openBrowser();
let failed = false;
try {
  await page.goto('/');
  // Queue defaults to stock_in mode; a scan here will carry a stock_in
  // direction, so the resolved entry becomes approvable.
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });

  // Scan an unknown barcode — it flags instead of becoming a normal pending scan.
  const card = await scanBarcode(page, BARCODE);
  await card.getByText('Flagged', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });

  // Resolve it from the What is it? sheet: create a product and use it.
  await card.getByRole('button', { name: 'What is it?' }).click();
  await page.getByLabel('Product name').fill(PRODUCT);
  await page.getByLabel('Category').fill(CATEGORY);
  await page.getByLabel('Package').fill(UNIT);
  await page.getByRole('button', { name: 'Create and use product' }).click();

  // Resolution proof: the Flagged badge disappears, the product heading shows,
  // and the pending batch-approval checkbox renders.
  await card.getByText('Flagged', { exact: true }).waitFor({ state: 'detached', timeout: 15_000 });
  await card.getByRole('heading', { name: PRODUCT }).waitFor({ state: 'visible' });
  await card
    .getByRole('checkbox', { name: 'Select scan for batch approval' })
    .waitFor({ state: 'visible', timeout: 10_000 });

  // The resolved scan carries a stock_in direction, so Confirm renders. Confirm
  // it and confirm the unit lands in inventory, like the stock-in flow.
  await card.getByRole('button', { name: 'Confirm', exact: true }).click();
  await card.waitFor({ state: 'detached', timeout: 15_000 });

  // Side-effect proof (API view): the resolved+approved unit is in inventory.
  const inventory = await readInventory(page);
  const row = inventory.find((i) => i.item.product.name === PRODUCT);
  assert(row !== undefined, `${PRODUCT} present in inventory`);
  assert(row.instanceCount === 1, `${PRODUCT} instanceCount is 1 (got ${row?.instanceCount})`);

  // ...and the user-visible Inventory route shows the same product.
  await page.getByRole('link', { name: 'Inventory' }).click();
  const invCard = page
    .getByRole('article')
    .filter({ has: page.getByRole('heading', { name: PRODUCT }) });
  await invCard.getByText(`1 ${UNIT}`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });

  await captureProof(page, 'flagged-resolution', {
    feature: 'flagged-resolution',
    barcode: BARCODE,
    product: PRODUCT,
    resolvedStatus: 'approved',
    inventoryInstanceCount: row.instanceCount,
    unitOfMeasure: row.item.product.unitOfMeasure,
  });
  console.log('PASS: flagged-resolution — unknown barcode resolved, approved, and 1 can is in inventory');
} catch (err) {
  failed = true;
  console.error('FAIL: flagged-resolution —', err.message);
  try { await captureProof(page, 'flagged-resolution-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
