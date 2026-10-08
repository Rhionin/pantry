// drive-shopping-groups.mjs — accept a seeded group, then show the shopping
// plan buying that group as one line while look-alikes that are still only
// suggestions stay separate.
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { openBrowser, captureProof, assert, API_URL, EVIDENCE_DIR } from './harness.mjs';

const PHONE = { width: 390, height: 844 };
const { browser, page } = await openBrowser({ viewport: PHONE });
let failed = false;
try {
  const suggestions = await page.request.get(`${API_URL}/api/group-suggestions`);
  if (!suggestions.ok()) throw new Error(`suggestions failed: ${suggestions.status()}`);
  const cards = await suggestions.json();
  const seeded = cards.find((card) => card.kind === 'from_old_plan');
  const corn = cards.find((card) => /corn/i.test(card.title));
  assert(seeded !== undefined, 'a from_old_plan card is open');
  assert(corn !== undefined && corn.kind === 'looks_alike', `corn is still a look-alike suggestion (${corn?.kind})`);

  await page.goto('/groups/suggestions');
  await page.getByRole('heading', { name: 'Cut green beans' }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('from the old shopping plan').waitFor({ state: 'visible' });
  await page.getByText('Del Monte Cut Green Beans').waitFor({ state: 'visible' });
  await phoneShot(page, 'pr8-groups-inbox');

  await page.getByRole('button', { name: 'Group', exact: true }).click();
  await page.getByText('Choose what this group should keep on hand').waitFor({ state: 'visible' });
  await page.getByLabel('Ounces').fill('48');
  await page.getByRole('button', { name: 'Use ounces' }).click();
  await page.getByRole('heading', { name: 'Kernel corn' }).waitFor({ state: 'visible', timeout: 10_000 });

  await page.goto('/inventory');
  const beans = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Cut green beans' }) });
  await beans.getByRole('button', { name: 'Edit rule' }).click();
  await page.getByRole('button', { name: 'Save rule' }).click();
  await beans.getByText('Always my favorite').waitFor({ state: 'visible', timeout: 10_000 });

  await page.goto('/shopping');
  await page.getByRole('button', { name: 'Build the list' }).click();
  const beanRow = page.getByRole('row').filter({ hasText: 'Cut green beans' });
  await beanRow.getByText('Always my favorite').waitFor({ state: 'visible', timeout: 10_000 });
  await beanRow.getByText('This is the one with the star.').waitFor({ state: 'visible' });
  await beanRow.getByRole('paragraph').filter({ hasText: 'Great Value Cut Green Beans' }).waitFor({ state: 'visible' });
  await beanRow.getByLabel('This trip, buy').waitFor({ state: 'visible' });
  await expectAbsent(page.getByLabel('Preferred brand for cut green beans'));
  await beanRow.scrollIntoViewIfNeeded();
  await phoneShot(page, 'pr8-shopping-group-line');

  const cornRows = page.getByRole('row').filter({ hasText: /Kernel Corn/ });
  await cornRows.first().waitFor({ state: 'visible' });
  const cornCount = await cornRows.count();
  assert(cornCount === 2, `ungrouped corn is two shopping lines (got ${cornCount})`);
  await cornRows.nth(1).scrollIntoViewIfNeeded();
  await phoneShot(page, 'pr8-ungrouped-split');

  const list = await page.request.get(`${API_URL}/api/shopping-list`);
  const lines = await list.json();
  const inventory = await (await page.request.get(`${API_URL}/api/inventory`)).json();
  const nameOf = new Map(inventory.map((row) => [row.item.id, row.item.product.name]));
  const grouped = lines.filter((line) => line.group && /green beans/i.test(line.group.name));
  const looseCorn = lines.filter((line) => /Kernel Corn/.test(nameOf.get(line.itemId) ?? '') && !line.group);
  assert(grouped.length === 1, `one beans group line (got ${grouped.length})`);
  assert(looseCorn.length === 2, `two corn lines (got ${looseCorn.length})`);

  await captureProof(page, 'pr8-done', {
    feature: 'shopping-groups',
    groupLines: grouped.length,
    cornLines: looseCorn.length,
  });
  console.log('PASS: shopping-groups — one group line, two ungrouped corn lines');
} catch (err) {
  failed = true;
  console.error('FAIL: shopping-groups —', err.message);
  try { await captureProof(page, 'pr8-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);

async function phoneShot(page, name) {
  await mkdir(EVIDENCE_DIR, { recursive: true });
  await page.screenshot({ path: join(EVIDENCE_DIR, `${name}.png`), fullPage: true });
  const aria = await page.locator('body').ariaSnapshot();
  await writeFile(join(EVIDENCE_DIR, `${name}.aria.txt`), aria, 'utf8');
}

async function expectAbsent(locator) {
  const count = await locator.count();
  assert(count === 0, `expected nothing, found ${count}`);
}
