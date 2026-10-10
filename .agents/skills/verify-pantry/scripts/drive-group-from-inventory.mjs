// Add an inventory product to a group, move it, and open a group just created.
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import {
  openBrowser, createKnownProduct, assert, API_URL, EVIDENCE_DIR, USER_ID,
} from './harness.mjs';

const PHONE = { width: 390, height: 844 };
const DESKTOP = { width: 1280, height: 900 };
const { browser, context, page } = await openBrowser({ viewport: PHONE });
let failed = false;
try {
  const barley = await createKnownProduct(page, {
    barcode: '166000000011',
    name: 'Lemon barley water',
    category: 'Drinks',
    unitOfMeasure: 'bottle',
  });
  const sesame = await createKnownProduct(page, {
    barcode: '166000000022',
    name: 'Sesame crunch',
    category: 'Snacks',
    unitOfMeasure: 'bag',
  });
  const crackers = await createKnownProduct(page, {
    barcode: '166000000033',
    name: 'Rice crackers',
    category: 'Snacks',
    unitOfMeasure: 'box',
  });
  await createKnownProduct(page, {
    barcode: '166000000044',
    name: 'Rice cakes',
    category: 'Snacks',
    unitOfMeasure: 'bag',
  });
  await stock(page, '166000000011');
  await stock(page, '166000000022');
  await stock(page, '166000000033');
  await stock(page, '166000000044');

  const sesameGroup = await page.request.post(`${API_URL}/api/groups`, {
    data: { name: 'Sesame snacks', productIds: [sesame.id] },
  });
  if (!sesameGroup.ok()) throw new Error(await sesameGroup.text());
  const sesameBody = await sesameGroup.json();
  const barleyGroup = await page.request.post(`${API_URL}/api/groups`, {
    data: { name: 'Barley drinks', productIds: [barley.id] },
  });
  if (!barleyGroup.ok()) throw new Error(await barleyGroup.text());

  await page.goto('/inventory');
  const crackers = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Rice crackers' }) });
  await crackers.waitFor({ state: 'visible', timeout: 15_000 });
  const actions = crackers.getByRole('button', { name: 'Actions for Rice crackers' });
  await actions.waitFor({ state: 'visible' });
  await actions.scrollIntoViewIfNeeded();
  await shot(page, 'after-inventory-add-button-390');

  await page.setViewportSize(DESKTOP);
  await crackers.getByRole('button', { name: 'Actions for Rice crackers' }).waitFor({ state: 'visible' });
  await shot(page, 'after-inventory-add-button-1280');
  await page.setViewportSize(PHONE);

  await crackers.getByRole('button', { name: 'Actions for Rice crackers' }).click();
  await page.getByRole('menuitem', { name: 'Add to a group…' }).click();
  const addDialog = page.getByRole('dialog', { name: 'Add Rice crackers to…' });
  await addDialog.getByRole('button', { name: /Sesame snacks/ }).waitFor({ state: 'visible' });
  await addDialog.getByRole('button', { name: /Barley drinks/ }).waitFor({ state: 'visible' });
  await shot(page, 'after-add-to-group-dialog-390');
  await addDialog.getByRole('button', { name: /Sesame snacks/ }).click();
  await addDialog.waitFor({ state: 'hidden' });
  await page.getByRole('heading', { name: 'Rice crackers' }).waitFor({ state: 'detached' });

  const sesameCard = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Sesame snacks' }) });
  await sesameCard.getByRole('button', { name: 'Expand Sesame snacks' }).click();
  const memberActions = sesameCard.getByRole('button', { name: 'Actions for Rice crackers' });
  await memberActions.waitFor({ state: 'visible' });
  await memberActions.scrollIntoViewIfNeeded();
  await shot(page, 'after-inventory-product-in-group-390');

  await memberActions.click();
  await page.getByRole('menuitem', { name: 'Move to another group' }).click();
  const moveDialog = page.getByRole('dialog', { name: 'Move Rice crackers to…' });
  await moveDialog.getByText('Already in Sesame snacks.').waitFor({ state: 'visible' });
  await shot(page, 'after-move-to-group-dialog-390');
  await moveDialog.getByRole('button', { name: /Barley drinks/ }).click();
  await moveDialog.waitFor({ state: 'hidden' });
  await sesameCard.getByText('Rice crackers').waitFor({ state: 'detached' });

  const cakes = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Rice cakes' }) });
  await cakes.getByRole('button', { name: 'Actions for Rice cakes' }).click();
  await shot(page, 'after-new-group-modal-390');
  await page.getByRole('menuitem', { name: 'Start a group with this' }).click();
  await page.getByRole('heading', { name: 'Rice cakes' }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('button', { name: 'Add a product' }).waitFor({ state: 'visible' });
  await shot(page, 'after-created-group-opens-390');

  await page.setViewportSize(DESKTOP);
  await page.getByRole('heading', { name: 'Rice cakes' }).waitFor({ state: 'visible' });
  await shot(page, 'after-created-group-opens-1280');

  assert(page.url().includes('/groups/'), `opened the new group (${page.url()})`);
  assert(!page.url().endsWith('/groups'), 'left the groups list');
  const listed = await page.request.get(`${API_URL}/api/groups`);
  const groups = await listed.json();
  const barleyView = groups.find((item) => item.name === 'Barley drinks');
  const sesameView = groups.find((item) => item.name === 'Sesame snacks');
  assert(barleyView.members.some((member) => member.productId === crackers.id), 'rice crackers moved to barley drinks');
  assert(!sesameView.members.some((member) => member.productId === crackers.id), 'rice crackers left sesame snacks');
  assert(sesameBody.id !== '', 'sesame group was created');
  console.log('PASS: group-from-inventory');
} catch (err) {
  failed = true;
  console.error('FAIL: group-from-inventory —', err);
  try { await shot(page, 'after-failure'); } catch { /* best effort */ }
} finally {
  await context.close();
  await browser.close();
}
process.exit(failed ? 1 : 0);

async function stock(page, barcode) {
  const created = await page.request.post(`${API_URL}/api/scans`, {
    data: { barcode, direction: 'stock_in', userId: USER_ID, unitCount: 1 },
  });
  if (!created.ok()) throw new Error(`scan failed: ${created.status()} ${await created.text()}`);
  const entry = await created.json();
  const committed = await page.request.post(`${API_URL}/api/scans/${entry.id}/commit`, { data: {} });
  if (!committed.ok()) throw new Error(`commit failed: ${committed.status()} ${await committed.text()}`);
}

async function shot(page, name) {
  await mkdir(EVIDENCE_DIR, { recursive: true });
  await page.screenshot({ path: join(EVIDENCE_DIR, `${name}.png`) });
}
