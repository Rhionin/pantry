// drive-stock-in-row.mjs — proves the stock-in confirmation is one tight row
// on a phone and still saves quantity and expiration the same way.
//
// User story: on a 390px phone, a scanned product appears as a single row with
// the name, a thumb-sized stepper, Approve, and Remove. Expiration stays a
// short text control under the name until opened. The user bumps the count,
// sets a date, and approves; inventory gets that many units with that date.
// A product from Open Food Facts keeps its provenance label in the same row,
// and Remove takes a scan off the queue without stocking it.
//
// External lookup is disabled here, so the Open Food Facts product is seeded by
// marking a created product's external_source in the server's SQLite database.
//
//   scripts/pantry-verify.sh drive scripts/drive-stock-in-row.mjs stock-in-row
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import {
  openBrowser, createKnownProduct, scanBarcode, setScanExpiration, captureProof, readInventory,
  assert, API_URL, EVIDENCE_DIR,
} from './harness.mjs';

const DB_PATH = process.env.PANTRY_DB_PATH ?? '';
const BARCODE = '910000000031';
const PRODUCT = 'Organic Whole Milk';
const SOURCED_BARCODE = '910000000032';
const SOURCED_PRODUCT = 'Greek Yogurt';
// May renders widest of the twelve medium-style months, so it is the date
// most likely to be clipped in the name column.
const EXPIRES = '2030-05-28';
const EXPIRES_TEXT = 'Exp May 28, 2030';
const MAX_ROW_CARD_HEIGHT = 60;
const MAX_SOURCED_CARD_HEIGHT = 66;
const MIN_TARGET = 44;

const box = async (locator, label) => {
  const found = await locator.boundingBox();
  assert(found !== null, `${label} is laid out`);
  return found;
};

// A new or changed card flashes briefly; screenshot it at rest. Dropping the
// flash class cancels the animation, which rejects `finished`; that is at rest too.
const settle = (locator) => locator.evaluate((node) =>
  Promise.all(node.getAnimations({ subtree: true }).map((animation) => animation.finished.catch(() => undefined))));

function markFromOpenFoodFacts(productId) {
  assert(DB_PATH !== '', 'PANTRY_DB_PATH is required to seed an Open Food Facts product');
  execFileSync('python3', ['-c', `
import sqlite3, sys
conn = sqlite3.connect(sys.argv[1], timeout=10)
conn.execute("UPDATE products SET external_source = 'openfoodfacts' WHERE id = ?", (sys.argv[2],))
conn.commit()
`, DB_PATH, productId], { stdio: 'inherit' });
}

// One row means the stepper, Approve, and Remove share the stepper's 44px band
// and run left to right after the name column, with nothing overflowing.
async function assertOneRow(card, label, productName) {
  const cardBox = await box(card, 'card');
  const band = await box(card.getByRole('button', { name: 'Decrease unit count' }), 'decrease');
  const controls = [
    ['decrease', card.getByRole('button', { name: 'Decrease unit count' })],
    ['unit count', card.getByRole('textbox', { name: 'Unit count', exact: true })],
    ['increase', card.getByRole('button', { name: 'Increase unit count' })],
    ['approve', card.getByRole('button', { name: 'Approve', exact: true })],
    ['remove', card.getByRole('button', { name: 'Remove', exact: true })],
  ];
  const boxes = [];
  for (const [name, locator] of controls) {
    const part = await box(locator, name);
    assert(part.y >= band.y - 1 && part.y + part.height <= band.y + band.height + 1,
      `${label}: ${name} sits inside the row band (y ${part.y}..${part.y + part.height}, band ${band.y}..${band.y + band.height})`);
    boxes.push([name, part]);
  }
  for (let index = 1; index < boxes.length; index += 1) {
    const [previousName, previous] = boxes[index - 1];
    const [name, current] = boxes[index];
    assert(previous.x + previous.width <= current.x + 0.5, `${label}: ${previousName} is left of ${name}`);
  }
  const [, removeBox] = boxes[boxes.length - 1];
  assert(removeBox.x + removeBox.width <= cardBox.x + cardBox.width + 0.5, `${label}: remove stays inside the card`);

  const heading = await box(card.getByRole('heading', { name: productName }), 'product name');
  const nameColumn = await box(card.locator('.scan-entry-row-name'), 'name column');
  assert(nameColumn.x + nameColumn.width <= band.x + 0.5, `${label}: the name column is left of the stepper`);
  assert(heading.x >= nameColumn.x - 0.5 && heading.x + heading.width <= nameColumn.x + nameColumn.width + 0.5,
    `${label}: the product name stays in its column`);

  const overflow = await card.evaluate((node) => node.scrollWidth - node.clientWidth);
  assert(overflow <= 0, `${label}: the row does not overflow the card (${overflow}px)`);
  const expiry = card.locator('.scan-entry-row-expiry');
  const expiryText = await expiry.innerText();
  const clipped = await expiry.evaluate((node) => node.scrollWidth - node.clientWidth);
  assert(clipped <= 0, `${label}: "${expiryText}" is not clipped (${clipped}px over)`);
  return { cardBox, nameWidth: nameColumn.width, expiryText };
}

const { browser, page } = await openBrowser({
  viewport: { width: 390, height: 844 },
  deviceScaleFactor: 2,
  isMobile: true,
  hasTouch: true,
});
let failed = false;
try {
  await createKnownProduct(page, {
    barcode: BARCODE, name: PRODUCT, category: 'Dairy', unitOfMeasure: 'carton',
  });
  const sourced = await createKnownProduct(page, {
    barcode: SOURCED_BARCODE, name: SOURCED_PRODUCT, category: 'Dairy', unitOfMeasure: 'cup',
  });
  markFromOpenFoodFacts(sourced.id);
  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });

  const card = await scanBarcode(page, BARCODE);
  await card.getByRole('heading', { name: PRODUCT }).waitFor({ state: 'visible' });
  const addExpiration = card.getByRole('button', { name: 'Add expiration' });
  await addExpiration.waitFor({ state: 'visible' });
  assert(await card.getByLabel('Expiration date').count() === 0, 'the expiration field stays hidden until asked');
  assert(await card.getByLabel(/^Product data from/).count() === 0, 'a local product shows no provenance label');

  for (const name of ['Decrease unit count', 'Increase unit count']) {
    const target = await box(card.getByRole('button', { name }), name);
    assert(target.width >= MIN_TARGET && target.height >= MIN_TARGET,
      `${name} is at least ${MIN_TARGET}px (got ${target.width}x${target.height})`);
  }
  const closed = await assertOneRow(card, 'closed', PRODUCT);
  assert(closed.cardBox.height <= MAX_ROW_CARD_HEIGHT,
    `closed card is one row (height ${closed.cardBox.height})`);
  await settle(card);
  await card.screenshot({ path: join(EVIDENCE_DIR, 'stock-in-row-closed.png') });
  await page.screenshot({ path: join(EVIDENCE_DIR, 'stock-in-row-closed-page.png') });

  await addExpiration.click();
  const expiry = card.getByLabel('Expiration date');
  await expiry.waitFor({ state: 'visible' });
  await card.getByRole('button', { name: 'Approve', exact: true }).waitFor({ state: 'visible' });
  await card.getByRole('button', { name: 'Remove', exact: true }).waitFor({ state: 'visible' });
  const openCard = await box(card, 'open card');
  await card.screenshot({ path: join(EVIDENCE_DIR, 'stock-in-row-open.png') });
  await page.screenshot({ path: join(EVIDENCE_DIR, 'stock-in-row-open-page.png') });

  await setScanExpiration(page, card, EXPIRES);
  const summary = card.getByRole('button', { name: EXPIRES_TEXT });
  await summary.waitFor({ state: 'visible' });
  const set = await assertOneRow(card, 'expiration set', PRODUCT);
  assert(set.cardBox.height <= MAX_ROW_CARD_HEIGHT,
    `a saved date folds back into the row (height ${set.cardBox.height})`);
  await card.screenshot({ path: join(EVIDENCE_DIR, 'stock-in-row-set.png') });

  const increase = card.getByRole('button', { name: 'Increase unit count' });
  for (let step = 0; step < 2; step += 1) {
    const patched = page.waitForResponse((response) => /\/api\/scans\/[^/]+$/.test(new URL(response.url()).pathname)
      && response.request().method() === 'PATCH' && response.ok());
    await increase.click();
    await patched;
  }
  await page.waitForFunction(
    (barcode) => document.querySelector(`[aria-label="Scan ${barcode}"] input[aria-label="Unit count"]`)?.value === '3',
    BARCODE,
  );
  const scans = await (await page.request.get(`${API_URL}/api/scans?userId=user-1&status=pending`)).json();
  const pending = scans.find((scan) => scan.barcode === BARCODE);
  assert(pending?.unitCount === 3, `the stepper saved unit count 3 (got ${pending?.unitCount})`);

  const narrow = {};
  for (const width of [360, 320]) {
    await page.setViewportSize({ width, height: 800 });
    try {
      narrow[width] = { fits: true, ...(await assertOneRow(card, `${width}px`, PRODUCT)) };
    } catch (err) {
      narrow[width] = { fits: false, reason: err.message };
    }
  }
  assert(narrow[360].fits, `still one unclipped row at 360px: ${narrow[360].reason ?? ''}`);
  await page.setViewportSize({ width: 390, height: 844 });

  await card.getByRole('button', { name: 'Approve', exact: true }).click();
  await card.waitFor({ state: 'detached', timeout: 15_000 });

  const inventory = await readInventory(page);
  const row = inventory.find((item) => item.item.product.name === PRODUCT);
  assert(row !== undefined, `${PRODUCT} present in inventory`);
  assert(row.instanceCount === 3, `${PRODUCT} instanceCount is 3 (got ${row?.instanceCount})`);
  const instances = await (await page.request.get(`${API_URL}/api/inventory/${row.item.id}/instances`)).json();
  const expiryDates = instances.map((instance) => instance.expiresAt?.substring(0, 10));
  assert(expiryDates.length === 3 && expiryDates.every((date) => date === EXPIRES),
    `every stocked unit expires ${EXPIRES} (got ${expiryDates.join(', ')})`);

  const sourcedCard = await scanBarcode(page, SOURCED_BARCODE);
  await sourcedCard.getByRole('heading', { name: SOURCED_PRODUCT }).waitFor({ state: 'visible' });
  const provenance = sourcedCard.getByLabel('Product data from Open Food Facts');
  await provenance.waitFor({ state: 'visible' });
  const sourcedRow = await assertOneRow(sourcedCard, 'Open Food Facts product', SOURCED_PRODUCT);
  assert(sourcedRow.cardBox.height <= MAX_SOURCED_CARD_HEIGHT,
    `the provenance label keeps the card to one row (height ${sourcedRow.cardBox.height})`);
  const badge = await box(provenance, 'provenance label');
  const nameColumn = await box(sourcedCard.locator('.scan-entry-row-name'), 'name column');
  assert(badge.x + badge.width <= nameColumn.x + nameColumn.width + 0.5, 'the provenance label stays in the name column');
  await settle(sourcedCard);
  await sourcedCard.screenshot({ path: join(EVIDENCE_DIR, 'stock-in-row-provenance.png') });

  const removed = page.waitForResponse((response) => /\/api\/scans\/[^/]+$/.test(new URL(response.url()).pathname)
    && response.request().method() === 'PATCH' && response.ok());
  await sourcedCard.getByRole('button', { name: 'Remove', exact: true }).click();
  const removeBody = (await removed).request().postDataJSON();
  assert(removeBody?.status === 'cancelled', `Remove cancels the scan (sent ${JSON.stringify(removeBody)})`);
  await sourcedCard.waitFor({ state: 'detached', timeout: 15_000 });
  const stillPending = await (await page.request.get(`${API_URL}/api/scans?userId=user-1&status=pending`)).json();
  assert(!stillPending.some((scan) => scan.barcode === SOURCED_BARCODE), 'the removed scan is no longer pending');
  const afterRemove = await readInventory(page);
  assert(!afterRemove.some((item) => item.item.product.name === SOURCED_PRODUCT), 'a removed scan stocks nothing');

  await page.getByRole('link', { name: 'Inventory' }).click();
  const invCard = page.getByRole('article').filter({ has: page.getByRole('heading', { name: PRODUCT }) });
  await invCard.getByText('3 carton', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });

  await captureProof(page, 'stock-in-row', {
    feature: 'stock-in-card',
    viewport: '390x844',
    barcode: BARCODE,
    product: PRODUCT,
    closedCardHeight: closed.cardBox.height,
    openCardHeight: openCard.height,
    setCardHeight: set.cardBox.height,
    openFoodFactsCardHeight: sourcedRow.cardBox.height,
    productNameWidthAt390: closed.nameWidth,
    narrowWidths: narrow,
    savedUnitCount: pending.unitCount,
    inventoryInstanceCount: row.instanceCount,
    instanceExpiryDates: expiryDates,
    removedScanCancelled: true,
  });
  console.log(`PASS: stock-in-row — one ${closed.cardBox.height}px row at 390px; 3 cartons expiring ${EXPIRES} are in inventory`);
} catch (err) {
  failed = true;
  console.error('FAIL: stock-in-row —', err.message);
  try { await captureProof(page, 'stock-in-row-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
