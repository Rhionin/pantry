// drive-shopping-list.mjs — proves the shopping-list flow end to end through the
// web UI.
//
// User story: a product has a target quantity; when its stock drops below the
// target a `Derived` shortfall row appears on the shopping list. The backend
// materializes derived entries (shopping.SyncDerivedItems), so the derived row
// carries a real id and exposes `Mark <product> purchased`. Marking it purchased
// dismisses the row and — because a purchase records intent, not stock — leaves
// on-hand inventory unchanged. Run through scripts/pantry-verify.sh:
//   scripts/pantry-verify.sh drive scripts/drive-shopping-list.mjs shopping-list
import {
  openBrowser, createKnownProduct, scanBarcode, setScanExpiration, setScannerMode, captureProof, readInventory, assert,
} from './harness.mjs';

const BARCODE = '910000000004';
const PRODUCT = 'Verify Shopping Rice';
const UNIT = 'bag';
const TARGET = 2;

async function stockInUnit(page, expiry) {
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

  // Precondition: stock two units so a shortfall can open after consumption.
  await stockInUnit(page, '2032-04-10');
  await stockInUnit(page, '2032-08-20');

  // Set a target quantity of 2 via Get suggestion -> manual target.
  await page.getByRole('link', { name: 'Inventory' }).click();
  const riceRow = page
    .getByRole('article')
    .filter({ has: page.getByRole('heading', { name: PRODUCT }) });
  await riceRow.getByText(`2 ${UNIT}`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await riceRow.getByRole('button', { name: 'View instances' }).click();
  await page.getByRole('button', { name: 'Get suggestion' }).click();
  await page.getByLabel('Manual target quantity').fill(String(TARGET));
  await page.getByRole('button', { name: 'Save manual target' }).click();
  await page.getByText(`Target quantity set to ${TARGET}.`).waitFor({ state: 'visible', timeout: 10_000 });

  // Create a shortfall: stock out one unit so on hand (1) < target (2).
  await page.getByRole('link', { name: 'Scan Queue' }).click();
  await setScannerMode(page, 'stock_out');
  const outCard = await scanBarcode(page, BARCODE);
  await outCard
    .getByRole('radio', { name: 'Use oldest available automatically' })
    .waitFor({ state: 'visible', timeout: 15_000 });
  await outCard.getByRole('button', { name: 'Approve', exact: true }).click();
  await outCard.waitFor({ state: 'detached', timeout: 15_000 });

  // Derivation proof: the Shopping List shows a `Derived` shortfall row of
  // `1 bag` (target 2 minus 1 on hand). The backend materializes it, so it
  // carries a real id and exposes the Mark-purchased action.
  await page.getByRole('link', { name: 'Shopping List' }).click();
  const derivedRow = page
    .getByRole('row')
    .filter({ hasText: PRODUCT })
    .filter({ has: page.getByText('Derived', { exact: true }) });
  await derivedRow.getByText(`1 ${UNIT}`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
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
    target: TARGET,
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
