// drive-stock-in.mjs — proves the stock-in flow end to end through the web UI.
//
// User story: a known product's barcode is scanned while the queue is in
// stock-in mode; it appears as a pending card; the user approves it; the unit
// lands in inventory. Run through scripts/pantry-verify.sh:
//   scripts/pantry-verify.sh drive scripts/drive-stock-in.mjs stock-in
import {
  openBrowser, createKnownProduct, scanBarcode, captureProof, readInventory, assert,
} from './harness.mjs';

const BARCODE = '910000000001';
const PRODUCT = 'Verify Stock-In Milk';

const { browser, page } = await openBrowser();
let failed = false;
try {
  await createKnownProduct(page, {
    barcode: BARCODE, name: PRODUCT, category: 'Dairy', unitOfMeasure: 'carton',
  });

  await page.goto('/');
  // Queue defaults to stock_in mode; confirm from the on-screen mode banner.
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });

  const card = await scanBarcode(page, BARCODE);
  await card.getByText(`Barcode: ${BARCODE}`).waitFor({ state: 'visible' });
  await card.getByRole('heading', { name: PRODUCT }).waitFor({ state: 'visible' });

  // Approve the single pending scan via its per-card Approve button.
  await card.getByRole('button', { name: 'Approve', exact: true }).click();
  // The card leaves the queue once committed.
  await card.waitFor({ state: 'detached', timeout: 15_000 });

  // Side-effect proof: the unit is now in inventory. Confirm via the API view...
  const inventory = await readInventory(page);
  const row = inventory.find((i) => i.item.product.name === PRODUCT);
  assert(row !== undefined, `${PRODUCT} present in inventory`);
  assert(row.instanceCount === 1, `${PRODUCT} instanceCount is 1 (got ${row?.instanceCount})`);

  // ...and confirm the user-visible Inventory route shows the same product.
  await page.getByRole('link', { name: 'Inventory' }).click();
  const invCard = page
    .getByRole('article')
    .filter({ has: page.getByRole('heading', { name: PRODUCT }) });
  await invCard.getByText('1 carton', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });

  await captureProof(page, 'stock-in', {
    feature: 'stock-in',
    barcode: BARCODE,
    product: PRODUCT,
    inventoryInstanceCount: row.instanceCount,
    unitOfMeasure: row.item.product.unitOfMeasure,
  });
  console.log('PASS: stock-in — scan approved and 1 carton is in inventory');
} catch (err) {
  failed = true;
  console.error('FAIL: stock-in —', err.message);
  try { await captureProof(page, 'stock-in-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
