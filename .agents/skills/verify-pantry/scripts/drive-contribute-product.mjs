// drive-contribute-product.mjs — proves the opt-in contribute flow through the
// web UI. External lookup is disabled and no Product Opener account is
// configured, so the product must stay local.
//
//   scripts/pantry-verify.sh drive scripts/drive-contribute-product.mjs contribute-product
import {
  openBrowser, scanBarcode, captureProof, readInventory, assert, API_URL,
} from './harness.mjs';

const BARCODE = '910000000099';
const PRODUCT = 'Verify Contribute Sponge';
const EDITED = 'Verify Contribute Sponge Edited';
const CATEGORY = 'Cleaning';
const UNIT = 'each';

const { browser, page } = await openBrowser();
let failed = false;
try {
  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });

  const card = await scanBarcode(page, BARCODE);
  await card.getByText('Flagged', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });

  // Mantine hides the native switch/checkbox input and paints a track. The
  // accessible role is on that input; the visible control a person clicks is
  // its label.
  const shareSwitch = card.getByRole('switch', {
    name: 'Let me contribute products I type in',
    includeHidden: true,
  });
  await shareSwitch.waitFor({ state: 'attached', timeout: 10_000 });
  assert(!(await shareSwitch.isChecked()), 'sharing switch starts off');
  assert(
    (await card.getByRole('checkbox', { name: 'Contribute this product', includeHidden: true }).count()) === 0,
    'per-product checkbox is absent while sharing is off',
  );

  await card.getByText('Let me contribute products I type in', { exact: true }).click();
  const productOptIn = card.getByRole('checkbox', {
    name: 'Contribute this product',
    includeHidden: true,
  });
  await productOptIn.waitFor({ state: 'attached', timeout: 10_000 });
  assert(!(await productOptIn.isChecked()), 'per-product checkbox starts unchecked');
  await card.getByText(/not signed in to the open databases/).waitFor({ state: 'visible' });

  await card.getByText('Contribute this product', { exact: true }).click();
  await card.getByLabel('Open database').selectOption('openproductsfacts');
  await card.getByLabel('Product name').fill(PRODUCT);
  await card.getByLabel('Category').fill(CATEGORY);
  await card.getByLabel('Unit of measure').fill(UNIT);
  await card.getByRole('button', { name: 'Create and use product' }).click();

  await card.getByText('Flagged', { exact: true }).waitFor({ state: 'detached', timeout: 15_000 });
  await card.getByRole('heading', { name: PRODUCT }).waitFor({ state: 'visible' });

  const settings = await page.request.get(`${API_URL}/api/settings/contribution`);
  assert(settings.ok(), `settings read failed: ${settings.status()}`);
  const settingsBody = await settings.json();
  assert(settingsBody.enabled === true, 'household sharing is on');
  assert(settingsBody.configured === false, 'server is not signed in');

  const contributions = await page.request.get(`${API_URL}/api/contributions`);
  assert(contributions.ok(), `contributions read failed: ${contributions.status()}`);
  const rows = await contributions.json();
  const row = rows.find((entry) => entry.barcode === BARCODE);
  assert(row !== undefined, 'contribution row exists');
  assert(row.status === 'not_configured', `contribution status is not_configured (got ${row?.status})`);
  assert(row.database === 'openproductsfacts', 'contribution targets Open Products Facts');

  await captureProof(page, 'contribute-created', {
    feature: 'contribute-product',
    step: 'created',
    barcode: BARCODE,
    product: PRODUCT,
    contributionStatus: row.status,
    database: row.database,
    configured: settingsBody.configured,
  });

  // The scan was taken in stock-in mode, so the resolved card can be approved
  // and the product lands in inventory, where the same opt-in edits it.
  await card.getByRole('button', { name: 'Approve', exact: true }).click();
  await card.waitFor({ state: 'detached', timeout: 15_000 });
  await page.getByRole('link', { name: 'Inventory' }).click();
  const invCard = page.getByRole('article').filter({ has: page.getByRole('heading', { name: PRODUCT }) });
  await invCard.getByRole('button', { name: 'View instances' }).click();
  await page.getByRole('button', { name: 'Edit product' }).click();
  const nameInput = page.getByLabel('Product name');
  await page.waitForFunction((expected) => {
    const label = [...document.querySelectorAll('label')].find((node) =>
      (node.textContent ?? '').startsWith('Product name'),
    );
    const input = label ? document.getElementById(label.htmlFor) : null;
    return input instanceof HTMLInputElement && input.value === expected;
  }, PRODUCT);
  await nameInput.fill(EDITED);
  const editOptIn = page.getByRole('checkbox', { name: 'Contribute this product', includeHidden: true });
  await editOptIn.waitFor({ state: 'attached', timeout: 10_000 });
  assert(!(await editOptIn.isChecked()), 'edit contribution starts unchecked');
  await page.getByText('Contribute this product', { exact: true }).click();
  await page.getByRole('button', { name: 'Save product' }).click();
  await page.getByRole('heading', { name: EDITED, exact: true }).waitFor({ state: 'visible', timeout: 10_000 });

  const inventory = await readInventory(page);
  const stocked = inventory.find((item) => item.item.product.name === EDITED);
  assert(stocked !== undefined, `${EDITED} is in inventory`);

  const afterEdit = await page.request.get(`${API_URL}/api/contributions`);
  const editedRows = await afterEdit.json();
  const forBarcode = editedRows.filter((entry) => entry.barcode === BARCODE);
  assert(forBarcode.length === 2, `two local contributions (got ${forBarcode.length})`);
  assert(forBarcode.every((entry) => entry.status === 'not_configured'), 'edits stay local without an account');

  await captureProof(page, 'contribute-product', {
    feature: 'contribute-product',
    step: 'edited',
    barcode: BARCODE,
    product: EDITED,
    contributionCount: forBarcode.length,
    contributionStatus: 'not_configured',
  });
  console.log('PASS: contribute-product — create and edit stayed local');
} catch (err) {
  failed = true;
  console.error('FAIL: contribute-product —', err.message);
  try { await captureProof(page, 'contribute-product-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
