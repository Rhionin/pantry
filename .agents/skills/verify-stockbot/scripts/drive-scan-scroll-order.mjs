// Proves the scan queue scrolls on a desktop viewport, keeps newest-first
// order after a reload, and shows the barcode on an expanded scan row and
// an expanded inventory product.
import { copyFile, mkdir } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import {
  openBrowser, createKnownProduct, scanBarcode, captureProof, assert, API_URL,
} from './harness.mjs';

const TRIP = [
  { barcode: '920000000201', name: 'Oats' },
  { barcode: '920000000202', name: 'Milk' },
  { barcode: '920000000203', name: 'Pasta' },
  { barcode: '920000000204', name: 'Bread' },
  { barcode: '920000000205', name: 'Rice' },
  { barcode: '920000000206', name: 'Beans' },
];
const EARLIER = [
  { barcode: '920000000211', name: 'Yogurt' },
  { barcode: '920000000212', name: 'Butter' },
  { barcode: '920000000213', name: 'Cheese' },
  { barcode: '920000000214', name: 'Eggs' },
  { barcode: '920000000215', name: 'Apples' },
  { barcode: '920000000216', name: 'Soup' },
];

const backdate = (dbPath, rows) => {
  const script = `
import sqlite3, sys
db, pairs = sys.argv[1], sys.argv[2:]
conn = sqlite3.connect(db, timeout=15)
cur = conn.cursor()
for i in range(0, len(pairs), 2):
    barcode, scanned_at = pairs[i], pairs[i + 1]
    cur.execute("UPDATE scan_entries SET scanned_at = ? WHERE barcode = ?", (scanned_at, barcode))
    if cur.rowcount != 1:
        raise SystemExit(f"expected 1 row for {barcode}, updated {cur.rowcount}")
conn.commit()
`;
  const args = [dbPath];
  for (const row of rows) args.push(row.barcode, row.at);
  execFileSync('python3', ['-c', script, ...args], { stdio: 'inherit' });
};

const queueHeadings = (page) => page.locator('.scan-page-queue article h3').allTextContents();

const saveShot = async (page, name) => {
  const evidence = process.env.PANTRY_EVIDENCE_DIR;
  const path = join(evidence, `${name}.png`);
  await page.screenshot({ path });
  await mkdir('/opt/cursor/artifacts', { recursive: true });
  await copyFile(path, `/opt/cursor/artifacts/${name}.png`);
};

const assertQueueScrolls = async (page, label) => {
  const metrics = await page.locator('.scan-page-queue').evaluate((element) => ({
    scrollHeight: element.scrollHeight,
    clientHeight: element.clientHeight,
  }));
  const clipped = await page.locator('.scan-session-card--expanded').evaluate((element) =>
    element.scrollHeight > element.clientHeight + 2);
  assert(metrics.scrollHeight > metrics.clientHeight + 1, `${label} queue scrolls (scroll ${metrics.scrollHeight} client ${metrics.clientHeight})`);
  assert(!clipped, `${label} open session is not clipped`);
  await page.locator('.scan-page-queue').evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  const oldest = page.getByRole('button', { name: /2:30 PM/, expanded: false });
  await oldest.waitFor({ state: 'visible' });
  const box = await oldest.boundingBox();
  const viewport = page.viewportSize();
  assert(box !== null && viewport !== null, `${label} oldest session has a box`);
  assert(box.y + box.height <= viewport.height + 1, `${label} oldest session is in view after scrolling (y ${box.y})`);
};

const { browser, page } = await openBrowser({
  viewport: { width: 1280, height: 800 },
  timezoneId: 'America/New_York',
});
let failed = false;
try {
  const dbPath = process.env.PANTRY_DB_PATH;
  assert(typeof dbPath === 'string' && dbPath !== '', 'PANTRY_DB_PATH is set');

  for (const item of [...TRIP, ...EARLIER]) {
    await createKnownProduct(page, {
      barcode: item.barcode,
      name: item.name,
      category: 'Grocery',
      unitOfMeasure: 'bag',
    });
  }

  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });
  for (const item of [...TRIP, ...EARLIER]) {
    await scanBarcode(page, item.barcode);
  }

  const liveOrder = await queueHeadings(page);
  assert(liveOrder[0] === 'Soup', `live order starts with the newest scan (got ${liveOrder[0]})`);
  assert(liveOrder.at(-1) === 'Oats', `live order ends with the oldest scan (got ${liveOrder.at(-1)})`);

  const tripStart = Date.parse('2026-10-07T20:00:00Z');
  const dated = [
    ...TRIP.map((item, index) => ({
      ...item,
      at: new Date(tripStart + index * 2 * 60 * 1000).toISOString(),
    })),
    ...EARLIER.map((item, index) => ({
      ...item,
      at: new Date(tripStart - (index + 1) * 15 * 60 * 1000).toISOString(),
    })),
  ];
  backdate(dbPath, dated);
  await page.reload();
  await page.getByRole('heading', { name: 'Beans' }).waitFor({ state: 'visible', timeout: 10_000 });

  const reloaded = await queueHeadings(page);
  assert(
    JSON.stringify(reloaded) === JSON.stringify(['Beans', 'Rice', 'Bread', 'Pasta', 'Milk', 'Oats']),
    `reloaded open session is newest first (got ${reloaded.join(', ')})`,
  );
  const beans = page.getByRole('article', { name: `Scan ${TRIP[5].barcode}` });
  await beans.getByText(`Barcode: ${TRIP[5].barcode}`).waitFor({ state: 'visible' });
  assert(await page.getByRole('heading', { name: 'Yogurt' }).count() === 0, 'collapsed session hides its product');
  assert(await page.getByText(`Barcode: ${EARLIER[0].barcode}`).count() === 0, 'collapsed session hides its barcode');

  await saveShot(page, 'after-desktop-order');
  await assertQueueScrolls(page, 'desktop');
  await saveShot(page, 'after-desktop-scrolled');

  await page.setViewportSize({ width: 390, height: 800 });
  await page.getByRole('heading', { name: 'Beans' }).waitFor({ state: 'visible' });
  await assertQueueScrolls(page, 'mobile');
  await saveShot(page, 'after-mobile-scrolled');

  await page.setViewportSize({ width: 1280, height: 800 });
  await page.locator('.scan-page-queue').evaluate((element) => {
    element.scrollTop = 0;
  });
  await beans.getByRole('button', { name: 'Confirm', exact: true }).click();
  await beans.waitFor({ state: 'detached', timeout: 15_000 });

  await page.getByRole('link', { name: 'Inventory' }).click();
  const inventoryRow = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Beans' }) });
  await inventoryRow.waitFor({ state: 'visible', timeout: 10_000 });
  assert(await inventoryRow.getByText(/Barcode:/).count() === 0, 'collapsed inventory row hides the barcode');
  await inventoryRow.getByRole('button', { name: 'View instances' }).click();
  await inventoryRow.getByText(`Barcode: ${TRIP[5].barcode}`).waitFor({ state: 'visible', timeout: 10_000 });
  await saveShot(page, 'after-inventory-barcode');

  const listed = await page.request.get(`${API_URL}/api/products`);
  assert(listed.ok(), 'product list answers');
  await captureProof(page, 'scan-scroll-order', {
    liveNewest: 'Soup',
    reloadedOpenSession: reloaded,
    inventoryBarcode: TRIP[5].barcode,
  });
  console.log('PASS: queue scrolls, newest-first survives reload, barcodes show when expanded');
} catch (error) {
  failed = true;
  console.error(error);
  try {
    await saveShot(page, 'after-failure');
  } catch {
    // ignore screenshot failure
  }
} finally {
  await browser.close();
}
if (failed) process.exit(1);
