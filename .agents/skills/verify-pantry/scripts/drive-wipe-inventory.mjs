// drive-wipe-inventory.mjs — proves wiping inventory keeps product lookup.
//
// User story: a stocked product is wiped from inventory only after the user
// types WIPE INVENTORY. The barcode still resolves afterward.
// Run through scripts/pantry-verify.sh:
//   scripts/pantry-verify.sh drive scripts/drive-wipe-inventory.mjs wipe-inventory
import {
  openBrowser, createKnownProduct, scanBarcode, captureProof, readInventory, assert,
  API_URL,
} from './harness.mjs';

const BARCODE = '910000000027';
const PRODUCT = 'Verify Wipe Beans';

const { browser, page } = await openBrowser();
let failed = false;
try {
  await createKnownProduct(page, {
    barcode: BARCODE, name: PRODUCT, category: 'Pantry', unitOfMeasure: 'can',
  });

  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });
  const card = await scanBarcode(page, BARCODE);
  await card.getByRole('button', { name: 'Approve', exact: true }).click();
  await card.waitFor({ state: 'detached', timeout: 15_000 });

  await page.getByRole('link', { name: 'Inventory' }).click();
  await page.getByRole('heading', { name: PRODUCT }).waitFor({ state: 'visible', timeout: 10_000 });
  const banner = page.getByRole('alert', { name: 'Opening inventory' });
  await banner.waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('button', { name: 'This scan is complete' }).click();
  await banner.waitFor({ state: 'hidden', timeout: 10_000 });

  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Settings' }).click();
  await page.getByRole('heading', { name: 'Settings' }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByLabel('Months of supply').waitFor({ state: 'visible' });

  await page.getByRole('button', { name: 'Wipe inventory' }).click();
  const dialog = page.getByRole('dialog', { name: 'Wipe inventory' });
  await dialog.waitFor({ state: 'visible' });
  const confirm = dialog.getByRole('button', { name: 'Confirm wipe' });
  await assertDisabled(confirm);

  const phrase = dialog.getByRole('textbox', { name: 'Type WIPE INVENTORY to confirm' });
  await phrase.fill('wipe inventory');
  await assertDisabled(confirm);
  await captureProof(page, 'wipe-inventory-blocked', {
    feature: 'wipe-inventory',
    step: 'near-miss phrase leaves confirm disabled',
    confirmation: 'wipe inventory',
  });

  await phrase.fill('WIPE INVENTORY');
  await confirm.click();
  await page.getByRole('link', { name: 'Inventory' }).click();
  await page.getByText('Your inventory is empty.').waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('alert', { name: 'Opening inventory' }).waitFor({ state: 'visible', timeout: 10_000 });

  const inventory = await readInventory(page);
  assert(inventory.length === 0, `inventory empty after wipe (got ${inventory.length})`);

  const lookup = await page.request.get(`${API_URL}/api/products/lookup?barcode=${BARCODE}`);
  if (!lookup.ok()) throw new Error(`lookup failed: ${lookup.status()}`);
  const lookupBody = await lookup.json();
  assert(lookupBody.product?.name === PRODUCT, `lookup still returns ${PRODUCT}`);

  await captureProof(page, 'wipe-inventory', {
    feature: 'wipe-inventory',
    barcode: BARCODE,
    product: PRODUCT,
    inventoryCount: inventory.length,
    lookupProduct: lookupBody.product.name,
    lookupSource: lookupBody.source,
  });
  console.log('PASS: wipe-inventory — stock cleared and barcode lookup still resolves');
} catch (err) {
  failed = true;
  console.error('FAIL: wipe-inventory —', err.message);
  try { await captureProof(page, 'wipe-inventory-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);

async function assertDisabled(locator) {
  if (await locator.isEnabled()) {
    throw new Error('ASSERT FAILED: confirm wipe was enabled before the exact phrase');
  }
}
