// drive-group-detail.mjs — pick a restocking rule and add an ungrouped product
// from the group detail page, the way CJ does on a phone.
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import {
  openBrowser, createKnownProduct, captureProof, assert, API_URL, EVIDENCE_DIR, USER_ID,
} from './harness.mjs';

const PHONE = { width: 390, height: 844 };
const { browser, page } = await openBrowser({ viewport: PHONE });
let failed = false;
try {
  const punch = await createKnownProduct(page, {
    barcode: '052000340075',
    name: 'Fruit punch thirst quencher powder',
    category: 'Drinks',
    unitOfMeasure: 'canister',
  });
  const thirst = await createKnownProduct(page, {
    barcode: '052000340242',
    name: 'Thirst Quencher Powder',
    category: 'Drinks',
    unitOfMeasure: 'canister',
  });
  const glacier = await createKnownProduct(page, {
    barcode: '052000341111',
    name: 'Gatorade Glacier Freeze',
    category: 'Drinks',
    unitOfMeasure: 'canister',
  });
  await stock(page, '052000340075', 2);
  await stock(page, '052000340242', 4);
  await stock(page, '052000341111', 1);

  const sized = await page.request.put(`${API_URL}/api/products/${glacier.id}`, {
    data: {
      name: 'Gatorade Glacier Freeze',
      category: 'Drinks',
      unitOfMeasure: 'canister',
      netAmount: 12,
      netUnit: 'oz',
    },
  });
  if (!sized.ok()) throw new Error(`size product failed: ${sized.status()} ${await sized.text()}`);
  const override = await page.request.put(`${API_URL}/api/products/${glacier.id}/supply-override`, {
    data: { quantity: 12 },
  });
  if (!override.ok()) throw new Error(`supply override failed: ${override.status()} ${await override.text()}`);

  const created = await page.request.post(`${API_URL}/api/groups`, {
    data: { name: 'Gatorade powder', productIds: [punch.id, thirst.id] },
  });
  if (!created.ok()) throw new Error(`create group failed: ${created.status()} ${await created.text()}`);
  const group = await created.json();
  const targeted = await page.request.put(`${API_URL}/api/groups/${group.id}/target`, {
    data: { windowMonths: 6 },
  });
  if (!targeted.ok()) throw new Error(`group target failed: ${targeted.status()} ${await targeted.text()}`);

  await page.goto(`/groups/${group.id}`);
  await page.getByRole('heading', { name: 'Gatorade powder' }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('button', { name: 'Pick a rule' }).waitFor({ state: 'visible' });
  await page.getByRole('checkbox', { name: 'Gatorade Glacier Freeze · 1 on hand' }).waitFor({ state: 'visible' });
  await page.getByText('Fruit punch thirst quencher powder').waitFor({ state: 'visible' });
  await phoneShot(page, 'group-detail-pick-a-rule');

  await page.getByRole('button', { name: 'Pick a rule' }).click();
  const sheet = page.getByRole('dialog', { name: 'Gatorade powder' });
  await sheet.getByRole('button', { name: 'Save rule' }).waitFor({ state: 'visible', timeout: 10_000 });
  await sheet.getByText(/Next trip:/).waitFor({ state: 'visible' });
  await phoneShot(page, 'group-detail-rule-sheet', { fullPage: false });
  await sheet.getByLabel('Rule').click();
  await page.getByRole('option', { name: 'Always my favorite' }).waitFor({ state: 'visible' });
  await phoneShot(page, 'group-detail-rule-options', { fullPage: false });

  await page.getByRole('option', { name: 'Always my favorite' }).click();
  await sheet.getByLabel('Always buy').click();
  await page.getByRole('option', { name: 'Fruit punch thirst quencher powder' }).click();
  await sheet.getByRole('button', { name: 'Save rule' }).click();
  await page.getByRole('button', { name: 'Edit rule: Always my favorite' }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('button', { name: 'Pick a rule' }).waitFor({ state: 'detached' });
  await phoneShot(page, 'group-detail-rule-saved');

  await page.getByLabel('Search products').fill('glacier');
  await page.getByRole('checkbox', { name: 'Gatorade Glacier Freeze · 1 on hand' }).check();
  await page.getByRole('button', { name: 'Add to this group' }).click();
  const prompt = page.getByText('These products have their own supply setting. Which should the whole group use?');
  await prompt.waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('button', { name: "Keep the group's 6 months" }).waitFor({ state: 'visible' });
  await page.getByRole('button', { name: 'Use 12 ounces (from Gatorade Glacier Freeze)' }).waitFor({ state: 'visible' });
  await page.getByRole('button', { name: 'Cancel' }).waitFor({ state: 'visible' });
  await prompt.scrollIntoViewIfNeeded();
  await phoneShot(page, 'group-detail-target-conflict', { fullPage: false });

  await page.getByRole('button', { name: 'Use 12 ounces (from Gatorade Glacier Freeze)' }).click();
  await page.getByText('Gatorade Glacier Freeze', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('checkbox', { name: /Gatorade Glacier Freeze/ }).waitFor({ state: 'detached' });
  await page.getByText('Gatorade Glacier Freeze', { exact: true }).scrollIntoViewIfNeeded();
  await phoneShot(page, 'group-detail-product-added', { fullPage: false });

  const saved = await page.request.get(`${API_URL}/api/groups/${group.id}`);
  if (!saved.ok()) throw new Error(`reload group failed: ${saved.status()}`);
  const view = await saved.json();
  const memberIds = view.members.map((member) => member.productId);
  assert(view.rule === 'favorite', `rule is favorite (got ${view.rule})`);
  assert(view.ruleConfirmed === true, 'rule is confirmed');
  assert(view.pinnedProductId === punch.id, 'fruit punch is pinned');
  assert(memberIds.includes(glacier.id), 'glacier freeze is a member');
  assert(memberIds.includes(punch.id) && memberIds.includes(thirst.id), 'original members stay');
  assert(view.quantity === 12, `group keeps 12 ounces (got ${view.quantity})`);
  assert(view.dimension === 'mass', `group dimension is mass (got ${view.dimension})`);
  assert(view.windowMonths === undefined, 'the 6 month target was replaced by the chosen quantity');

  await captureProof(page, 'group-detail', {
    feature: 'group-detail',
    rule: view.rule,
    ruleConfirmed: view.ruleConfirmed,
    members: view.members.map((member) => member.name),
  });
  console.log('PASS: group-detail — rule saved and glacier freeze added');
} catch (err) {
  failed = true;
  console.error('FAIL: group-detail —', err.message);
  try { await captureProof(page, 'group-detail-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);

async function stock(page, barcode, times) {
  for (let i = 0; i < times; i++) {
    const created = await page.request.post(`${API_URL}/api/scans`, {
      data: { barcode, direction: 'stock_in', userId: USER_ID, unitCount: 1 },
    });
    if (!created.ok()) throw new Error(`scan failed: ${created.status()} ${await created.text()}`);
    const entry = await created.json();
    const committed = await page.request.post(`${API_URL}/api/scans/${entry.id}/commit`, { data: {} });
    if (!committed.ok()) throw new Error(`commit failed: ${committed.status()} ${await committed.text()}`);
  }
}

async function phoneShot(page, name, { fullPage = true } = {}) {
  await mkdir(EVIDENCE_DIR, { recursive: true });
  await page.screenshot({ path: join(EVIDENCE_DIR, `${name}.png`), fullPage });
  const aria = await page.locator('body').ariaSnapshot();
  await writeFile(join(EVIDENCE_DIR, `${name}.aria.txt`), aria, 'utf8');
}
