// drive-shopping-list.mjs — proves the shopping-list flow end to end through the
// web UI.
//
// User story: the first scans are a snapshot. After the user marks that scan
// complete, using a unit puts a Derived row on the shopping list that replaces
// what was used. Marking it purchased dismisses the row and leaves on-hand
// inventory unchanged. Run through scripts/pantry-verify.sh:
//   scripts/pantry-verify.sh drive scripts/drive-shopping-list.mjs shopping-list
import {
  openBrowser, createKnownProduct, scanBarcode, setScanExpiration, setScannerMode, captureProof, readInventory, assert,
} from './harness.mjs';

const BARCODE = '910000000004';
const PRODUCT = 'Verify Shopping Rice';
const UNIT = 'bag';
async function stockInUnit(page, expiry) {
  const card = await scanBarcode(page, BARCODE);
  await card.getByRole('heading', { name: PRODUCT }).waitFor({ state: 'visible' });
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

  // Precondition: stock two units so a shortfall can open after consumption.
  await stockInUnit(page, '2032-04-10');
  await stockInUnit(page, '2032-08-20');

  // The opening scans are already here. Marking them complete is the start date.
  await page.getByRole('link', { name: 'Inventory' }).click();
  const riceRow = page
    .getByRole('article')
    .filter({ has: page.getByRole('heading', { name: PRODUCT }) });
  await riceRow.getByText(`2 ${UNIT}`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('button', { name: 'This scan is complete' }).click();
  await page.getByRole('alert', { name: 'Opening inventory' }).waitFor({ state: 'hidden', timeout: 10_000 });

  // Using one bag after the snapshot is consumption, not another snapshot.
  await page.getByRole('link', { name: 'Scan Queue' }).click();
  await setScannerMode(page, 'stock_out');
  const outCard = await scanBarcode(page, BARCODE);
  await outCard
    .getByRole('radio', { name: 'Use oldest available automatically' })
    .waitFor({ state: 'visible', timeout: 15_000 });
  await outCard.getByRole('button', { name: 'Approve', exact: true }).click();
  await outCard.waitFor({ state: 'detached', timeout: 15_000 });

  // The list replaces the one bag that was used.
  await page.getByRole('link', { name: 'Shopping List' }).click();
  const derivedRow = page
    .getByRole('row')
    .filter({ hasText: PRODUCT })
    .filter({ has: page.getByText('Derived', { exact: true }) });
  await derivedRow.getByText(`1 ${UNIT}`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await derivedRow.getByText('replacing 1 you used').waitFor({ state: 'visible' });
  await derivedRow.getByText('Derived', { exact: true }).waitFor({ state: 'visible' });

  // Inventory count before the purchase, to prove the purchase does not change it.
  let inventory = await readInventory(page);
  let row = inventory.find((i) => i.item.product.name === PRODUCT);
  assert(row !== undefined, `${PRODUCT} present in inventory before purchase`);
  assert(row.instanceCount === 1, `${PRODUCT} instanceCount is 1 before purchase (got ${row?.instanceCount})`);

  // Purchase: mark the derived shortfall purchased. The row leaves the list.
  await derivedRow.getByRole('button', { name: `Mark ${PRODUCT} purchased` }).click();
  await derivedRow.waitFor({ state: 'detached', timeout: 15_000 });
  // The purchased gap stays hidden until the quantity changes; list is now empty.
  await page.getByText('Your shopping list is empty.').waitFor({ state: 'visible', timeout: 10_000 });

  // Capture proof on the Shopping List page so the ARIA snapshot shows the
  // resulting (emptied) list state.
  await captureProof(page, 'shopping-list', {
    feature: 'shopping-list',
    barcode: BARCODE,
    product: PRODUCT,
    derivedQuantity: `1 ${UNIT}`,
    source: 'Derived',
    inventoryInstanceCountAfterPurchase: 1,
  });

  // Side-effect proof: a purchase records intent only — inventory is unchanged.
  inventory = await readInventory(page);
  row = inventory.find((i) => i.item.product.name === PRODUCT);
  assert(row !== undefined, `${PRODUCT} still present after purchase`);
  assert(
    row.instanceCount === 1,
    `${PRODUCT} instanceCount still 1 after purchase (got ${row?.instanceCount})`,
  );
  // ...and the user-visible Inventory route agrees.
  await page.getByRole('link', { name: 'Inventory' }).click();
  await page
    .getByRole('article')
    .filter({ has: page.getByRole('heading', { name: PRODUCT }) })
    .getByText(`1 ${UNIT}`, { exact: true })
    .waitFor({ state: 'visible', timeout: 10_000 });
  console.log('PASS: shopping-list — 1 bag shortfall derived, marked purchased, inventory unchanged (1 bag)');
} catch (err) {
  failed = true;
  console.error('FAIL: shopping-list —', err.message);
  try { await captureProof(page, 'shopping-list-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
