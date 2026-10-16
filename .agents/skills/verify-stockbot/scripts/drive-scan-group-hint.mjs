// drive-scan-group-hint.mjs — a known barcode that looks like a group, and an
// unknown barcode resolved through the What is it? sheet.
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import {
  openBrowser, createKnownProduct, scanBarcode, captureProof, assert, API_URL, EVIDENCE_DIR,
} from './harness.mjs';

const DEL_MONTE = '049000006340';
const UNKNOWN = '999000000111';
const PHONE = { width: 390, height: 844 };

const { browser, page } = await openBrowser({ viewport: PHONE });
let failed = false;
try {
  const kroger = await createKnownProduct(page, {
    barcode: '011110000001',
    name: 'Kroger Cut Green Beans',
    category: 'Canned goods',
    unitOfMeasure: 'can',
  });
  await createKnownProduct(page, {
    barcode: DEL_MONTE,
    name: 'Del Monte Cut Green Beans',
    category: 'Canned goods',
    unitOfMeasure: 'can',
  });
  const milk = await createKnownProduct(page, {
    barcode: '011110000002',
    name: 'Whole Milk',
    category: 'Dairy',
    unitOfMeasure: 'carton',
  });
  const created = await page.request.post(`${API_URL}/api/groups`, {
    data: { name: 'Cut green beans', productIds: [kroger.id] },
  });
  if (!created.ok()) throw new Error(`create group failed: ${created.status()} ${await created.text()}`);
  const group = await created.json();

  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });

  const hintCard = await scanBarcode(page, DEL_MONTE);
  await hintCard.getByText('Looks like Cut green beans').waitFor({ state: 'visible', timeout: 10_000 });
  await phoneShot(page, 'pr6-scan-hint');

  await hintCard.getByRole('button', { name: 'Add to group' }).click();
  await hintCard.getByText('Looks like Cut green beans').waitFor({ state: 'detached', timeout: 10_000 });

  await page.goto(`/groups/${group.id}`);
  await page.getByRole('heading', { name: 'Cut green beans' }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('Del Monte Cut Green Beans', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('Kroger Cut Green Beans', { exact: true }).waitFor({ state: 'visible' });

  await page.goto('/');
  const unknownCard = await scanBarcode(page, UNKNOWN);
  await unknownCard.getByText('Flagged', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await unknownCard.getByRole('button', { name: 'What is it?' }).click();
  const search = page.getByLabel('Search products');
  await search.waitFor({ state: 'visible', timeout: 10_000 });
  await search.fill('Whole Milk');
  await page.getByRole('option', { name: 'Whole Milk — Dairy' }).click();
  await phoneShot(page, 'pr6-what-is-it');
  await page.getByRole('button', { name: 'Use selected product' }).click();
  await unknownCard.getByRole('heading', { name: 'Whole Milk' }).waitFor({ state: 'visible', timeout: 15_000 });
  await unknownCard.getByText('Flagged', { exact: true }).waitFor({ state: 'detached', timeout: 10_000 });

  const detail = await page.request.get(`${API_URL}/api/products/${milk.id}`);
  if (!detail.ok()) throw new Error(`product read failed: ${detail.status()}`);
  const product = await detail.json();
  assert(Array.isArray(product.barcodes) && product.barcodes.includes(UNKNOWN), `barcode list grew (got ${JSON.stringify(product.barcodes)})`);

  await captureProof(page, 'pr6-resolved', {
    feature: 'scan-group-hint',
    groupId: group.id,
    attachedBarcode: UNKNOWN,
    barcodes: product.barcodes,
  });
  console.log('PASS: scan-group-hint — Del Monte joined the group and the unknown barcode attached to Whole Milk');
} catch (err) {
  failed = true;
  console.error('FAIL: scan-group-hint —', err.message);
  try { await captureProof(page, 'pr6-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);

async function phoneShot(page, name) {
  await mkdir(EVIDENCE_DIR, { recursive: true });
  await page.screenshot({ path: join(EVIDENCE_DIR, `${name}.png`) });
  const aria = await page.locator('body').ariaSnapshot();
  await writeFile(join(EVIDENCE_DIR, `${name}.aria.txt`), aria, 'utf8');
}
