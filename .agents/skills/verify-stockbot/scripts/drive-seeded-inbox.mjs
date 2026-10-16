// drive-seeded-inbox.mjs — the inbox shows the card seeded from the old plan,
// and the shopping plan still combines those store brands on one line.
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
  assert(seeded !== undefined, 'a from_old_plan card is open');
  const names = seeded.members.map((member) => member.name);
  assert(names.includes('Del Monte Cut Green Beans'), `Del Monte is on the card (${names.join(', ')})`);
  assert(names.includes('Great Value Cut Green Beans') && names.includes('Kroger Cut Green Beans'), `store brands are on the card (${names.join(', ')})`);

  await page.goto('/groups/suggestions');
  await page.getByRole('heading', { name: 'Cut green beans' }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('from the old shopping plan').waitFor({ state: 'visible' });
  await page.getByText('Del Monte Cut Green Beans').waitFor({ state: 'visible' });
  await phoneShot(page, 'pr7-seeded-inbox');

  await page.goto('/shopping');
  await page.getByRole('button', { name: 'Build the list' }).click();
  await page.getByLabel('Preferred brand for cut green beans').waitFor({ state: 'visible', timeout: 10_000 });
  const rows = page.getByRole('row');
  const rowText = await rows.allTextContents();
  const beanRows = rowText.filter((text) => /green beans/i.test(text));
  assert(beanRows.length === 1, `store brands stay one shopping line (got ${beanRows.length}: ${beanRows.join(' | ')})`);
  await phoneShot(page, 'pr7-shopping-still-combined');

  await captureProof(page, 'pr7-done', { feature: 'seeded-inbox', beanRows: beanRows.length, members: names });
  console.log('PASS: seeded-inbox — one old-plan card, shopping still one combined line');
} catch (err) {
  failed = true;
  console.error('FAIL: seeded-inbox —', err.message);
  try { await captureProof(page, 'pr7-failure'); } catch { /* best effort */ }
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
