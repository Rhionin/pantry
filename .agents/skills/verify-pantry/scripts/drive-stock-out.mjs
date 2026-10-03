// drive-stock-out.mjs — proves the stock-out oldest-first flow end to end
// through the web UI.
//
// User story: a product has two stocked instances with different expiry dates;
// the user switches the scanner to stock-out mode (by scanning the STOCK_OUT
// control barcode), scans the product, and approves without choosing a specific
// instance; the earliest-expiring instance is consumed first, leaving the
// later-expiring one in inventory. Run through scripts/pantry-verify.sh:
//   scripts/pantry-verify.sh drive scripts/drive-stock-out.mjs stock-out
import {
  openBrowser, createKnownProduct, scanBarcode, setScanExpiration, setScannerMode, captureProof, readInventory, assert,
} from './harness.mjs';

const BARCODE = '910000000003';
const PRODUCT = 'Verify Stock-Out Pasta';
const UNIT = 'box';
// Two instances, distinct expiries. The earlier date must be consumed first.
const EARLY_EXPIRY = '2030-01-10'; // rendered "Expires Jan 10, 2030"
const LATE_EXPIRY = '2030-12-20'; // rendered "Expires Dec 20, 2030"

// Stock in one instance with a specific expiry: scan in stock_in mode, open
// Add expiration, commit the date, then approve.
async function stockInInstance(page, expiry) {
  const card = await scanBarcode(page, BARCODE);
  await card.getByText(`Barcode: ${BARCODE}`).waitFor({ state: 'visible' });
  await setScanExpiration(page, card, expiry);
  await card.getByRole('button', { name: 'Approve', exact: true }).click();
  await card.waitFor({ state: 'detached', timeout: 15_000 });
}

const { browser, page } = await openBrowser();
let failed = false;
try {
  await createKnownProduct(page, {
    barcode: BARCODE, name: PRODUCT, category: 'Dry goods', unitOfMeasure: UNIT,
  });

  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });

  // Precondition: stock two instances with different expiry dates.
  await stockInInstance(page, EARLY_EXPIRY);
  await stockInInstance(page, LATE_EXPIRY);

  let inventory = await readInventory(page);
  let row = inventory.find((i) => i.item.product.name === PRODUCT);
  assert(row !== undefined, `${PRODUCT} present in inventory`);
  assert(row.instanceCount === 2, `${PRODUCT} instanceCount is 2 before stock-out (got ${row?.instanceCount})`);

  // Switch the scanner to stock_out mode the way a user does (control barcode).
  await setScannerMode(page, 'stock_out');

  // Scan the product; it appears as a stock-out card.
  const outCard = await scanBarcode(page, BARCODE);
  // Wait for the instance selector to resolve the inventory item (loads async)
  // before approving; approving too early commits with no item and errors
  // `no item found`. Leave it at the default (empty) to exercise oldest-first.
  await outCard
    .getByRole('radio', { name: 'Use oldest available automatically' })
    .waitFor({ state: 'visible', timeout: 15_000 });
  await outCard.getByRole('button', { name: 'Approve', exact: true }).click();
  await outCard.waitFor({ state: 'detached', timeout: 15_000 });

  // Confirm the oldest was consumed via the user-visible Inventory route.
  await page.getByRole('link', { name: 'Inventory' }).click();
  const invCard = page
    .getByRole('article')
    .filter({ has: page.getByRole('heading', { name: PRODUCT }) });
  await invCard.getByText(`1 ${UNIT}`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await invCard.getByRole('button', { name: 'View instances' }).click();
  // The later-expiring instance survives; the earlier one is gone.
  await page.getByText('Expires Dec 20, 2030', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  assert(
    (await page.getByText('Expires Jan 10, 2030', { exact: true }).count()) === 0,
    'earliest-expiring instance (Jan 10, 2030) was consumed',
  );

  // Cross-check the API view.
  inventory = await readInventory(page);
  row = inventory.find((i) => i.item.product.name === PRODUCT);
  assert(row !== undefined, `${PRODUCT} still present after stock-out`);
  assert(row.instanceCount === 1, `${PRODUCT} instanceCount is 1 after stock-out (got ${row?.instanceCount})`);

  await captureProof(page, 'stock-out', {
    feature: 'stock-out',
    barcode: BARCODE,
    product: PRODUCT,
    remainingInstanceCount: row.instanceCount,
    survivingExpiry: 'Expires Dec 20, 2030',
    consumedExpiry: 'Expires Jan 10, 2030',
  });
  console.log('PASS: stock-out — oldest instance (Jan 10, 2030) consumed, Dec 20, 2030 survives (1 box left)');
} catch (err) {
  failed = true;
  console.error('FAIL: stock-out —', err.message);
  try { await captureProof(page, 'stock-out-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
