// drive-stock-ledger.mjs — the household history of every stock move.
//
// User story: after stocking products in and using one, History lists every
// move newest first, grouped by day, and the same row menu can correct or
// undo a move. Run through scripts/pantry-verify.sh:
//   scripts/pantry-verify.sh drive scripts/drive-stock-ledger.mjs stock-ledger
import { copyFile, mkdir } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import {
  openBrowser, createKnownProduct, scanBarcode, setScannerMode, captureProof, readInventory, assert,
  API_URL, EVIDENCE_DIR,
} from './harness.mjs';

const OATS_BARCODE = '910000000200';
const BEANS_BARCODE = '910000000201';
const ARTIFACTS = '/opt/cursor/artifacts';

const phone = { width: 390, height: 844 };
const desktop = { width: 1280, height: 800 };

const { browser, page } = await openBrowser({
  viewport: phone,
  deviceScaleFactor: 2,
  timezoneId: 'America/Chicago',
});
let failed = false;

try {
  const opened = await page.request.post(`${API_URL}/api/onboarding/complete`);
  if (!opened.ok()) throw new Error(`finish opening failed: ${opened.status()}`);

  const oats = await createKnownProduct(page, {
    barcode: OATS_BARCODE, name: 'Rolled Oats', category: 'Cereal', unitOfMeasure: 'cartons',
  });
  const beans = await createKnownProduct(page, {
    barcode: BEANS_BARCODE, name: 'Black Beans', category: 'Canned', unitOfMeasure: 'cans',
  });
  const grouped = await page.request.post(`${API_URL}/api/groups`, {
    data: { name: 'Beans', productIds: [beans.id] },
  });
  if (!grouped.ok()) throw new Error(`group failed: ${grouped.status()} ${await grouped.text()}`);

  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 15_000 });
  await stockIn(page, BEANS_BARCODE, 'Black Beans', 4);
  await setScannerMode(page, 'stock_out');
  await stockOut(page, BEANS_BARCODE, 'Black Beans');
  await setScannerMode(page, 'stock_in');
  await stockIn(page, OATS_BARCODE, 'Rolled Oats', 2);

  const shelf = await readInventory(page);
  const beanRow = shelf.find((row) => row.item.product.name === 'Black Beans');
  const oatRow = shelf.find((row) => row.item.product.name === 'Rolled Oats');
  assert(beanRow?.instanceCount === 3, `beans on hand is 3 (got ${beanRow?.instanceCount})`);
  assert(oatRow?.instanceCount === 2, `oats on hand is 2 (got ${oatRow?.instanceCount})`);

  // The scans are all today. Move the beans stock-in to yesterday so the
  // ledger shows two days. The moves themselves came from scanning.
  shiftToYesterday(process.env.PANTRY_DB_PATH, beans.id);

  await page.getByRole('navigation', { name: 'Sections' }).getByRole('link', { name: 'History' }).click();
  await page.getByRole('heading', { name: 'History', level: 1 }).waitFor({ state: 'visible' });
  await page.getByRole('region', { name: 'Today' }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('region', { name: 'Yesterday' }).waitFor({ state: 'visible' });
  const today = page.getByRole('region', { name: 'Today' });
  await today.getByText('Rolled Oats').waitFor({ state: 'visible' });
  await today.getByText('Black Beans').waitFor({ state: 'visible' });
  await page.getByRole('region', { name: 'Yesterday' }).getByText('Black Beans').waitFor({ state: 'visible' });
  await page.getByText('Beans', { exact: true }).waitFor({ state: 'visible' });

  const ledger = await readLedger(page);
  assert(ledger.moves.length === 3, `ledger has 3 moves (got ${ledger.moves.length})`);
  assert(ledger.moves[0].productName === 'Rolled Oats', 'newest move is the oats stock in');
  assert(ledger.moves[1].direction === 'out', 'the beans use is next');
  assert(ledger.moves[2].direction === 'in' && ledger.moves[2].productName === 'Black Beans', 'yesterday is the beans stock in');

  await captureProof(page, 'ledger-phone', {
    viewport: 'phone',
    moves: ledger.moves.map((move) => ({
      product: move.productName,
      direction: move.direction,
      quantity: move.quantity,
      at: move.at,
    })),
  });
  await saveArtifact('ledger-phone.png', 'history-phone.png');

  await assertNavFits(page, 320);
  await page.setViewportSize(desktop);
  await page.getByRole('heading', { name: 'History', level: 1 }).waitFor({ state: 'visible' });
  await captureProof(page, 'ledger-desktop', { viewport: 'desktop', moves: ledger.moves.length });
  await saveArtifact('ledger-desktop.png', 'history-desktop.png');

  await page.getByRole('button', { name: 'Used' }).click();
  await page.getByText('Rolled Oats').waitFor({ state: 'hidden' });
  await page.getByRole('region', { name: 'Today' }).getByText('Black Beans').waitFor({ state: 'visible' });
  const used = await readLedger(page, { direction: 'out' });
  assert(used.moves.length === 1 && used.moves[0].direction === 'out', 'used filter is the one stock out');

  await page.getByRole('button', { name: 'All' }).click();
  await page.getByRole('textbox', { name: 'Search products or groups' }).fill('Beans');
  await page.getByText('Rolled Oats').waitFor({ state: 'hidden' });
  await page.getByText('Black Beans').first().waitFor({ state: 'visible' });
  const byGroup = await readLedger(page, { q: 'Beans' });
  assert(byGroup.moves.every((move) => move.productName === 'Black Beans' || move.groupName === 'Beans'), 'search keeps the beans');
  assert(byGroup.moves.length === 2, `beans search has 2 moves (got ${byGroup.moves.length})`);

  await page.getByRole('textbox', { name: 'Search products or groups' }).fill('');
  await page.getByText('Rolled Oats').waitFor({ state: 'visible' });

  const use = ledger.moves.find((move) => move.direction === 'out');
  await page.getByRole('button', { name: /Actions for Black Beans, 1 used/ }).click();
  await page.getByRole('menuitem', { name: 'Change quantity' }).click();
  const dialog = page.getByRole('dialog', { name: 'Change quantity' });
  await dialog.getByLabel('How many units were actually used?').fill('2');
  await dialog.getByRole('button', { name: 'Save quantity' }).click();
  await dialog.waitFor({ state: 'hidden' });
  await page.getByText('−2').waitFor({ state: 'visible', timeout: 10_000 });
  const afterEdit = await readInventory(page);
  assert(afterEdit.find((row) => row.item.id === beanRow.item.id)?.instanceCount === 2, 'raising the use takes another can off the shelf');

  await page.getByRole('button', { name: /Actions for Black Beans, 2 used/ }).click();
  await page.getByRole('menuitem', { name: 'Undo this use' }).click();
  const undo = page.getByRole('dialog', { name: 'Undo this use?' });
  await undo.getByText('2 units go back on the shelf').waitFor({ state: 'visible' });
  await undo.getByRole('button', { name: 'Undo this use' }).click();
  await undo.waitFor({ state: 'hidden' });
  await page.getByText('−2').waitFor({ state: 'hidden', timeout: 10_000 });
  const afterUndo = await readInventory(page);
  assert(afterUndo.find((row) => row.item.id === beanRow.item.id)?.instanceCount === 4, 'undo puts both cans back');
  const remaining = await readLedger(page);
  assert(remaining.moves.length === 2, 'the undone use leaves the ledger');
  assert(remaining.moves.every((move) => move.id !== use.id), 'the undone move is gone');

  console.log('PASS: stock ledger — grouped history, filters, quantity, and undo');
} catch (err) {
  failed = true;
  console.error('FAIL: stock ledger —', err);
  try { await captureProof(page, 'ledger-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);

async function stockIn(page, barcode, name, units) {
  const card = await scanBarcode(page, barcode);
  await card.getByRole('heading', { name }).waitFor({ state: 'visible' });
  const input = card.getByRole('textbox', { name: 'Unit count', exact: true });
  const patch = page.waitForResponse((response) => (
    response.request().method() === 'PATCH' && response.ok() && /\/api\/scans\/[^/]+$/.test(new URL(response.url()).pathname)
  ));
  await input.fill(String(units));
  await input.blur();
  await patch;
  await card.getByRole('button', { name: 'Confirm', exact: true }).click();
  await card.waitFor({ state: 'detached', timeout: 15_000 });
}

async function stockOut(page, barcode, name) {
  const card = await scanBarcode(page, barcode);
  await card.getByRole('heading', { name }).waitFor({ state: 'visible' });
  await card.getByRole('button', { name: 'Confirm', exact: true }).click();
  await card.waitFor({ state: 'detached', timeout: 15_000 });
}

async function readLedger(page, query) {
  const params = new URLSearchParams();
  if (query?.direction) params.set('direction', query.direction);
  if (query?.q) params.set('q', query.q);
  const suffix = params.size > 0 ? `?${params.toString()}` : '';
  const res = await page.request.get(`${API_URL}/api/history${suffix}`);
  if (!res.ok()) throw new Error(`history read failed: ${res.status()} ${await res.text()}`);
  return res.json();
}

function shiftToYesterday(dbPath, productId) {
  if (!dbPath) throw new Error('PANTRY_DB_PATH is not set');
  execFileSync('python3', ['-c', `
import sqlite3, sys
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo
db, product_id = sys.argv[1], sys.argv[2]
chicago = ZoneInfo("America/Chicago")
now = datetime.now(chicago)
yesterday = (now - timedelta(days=1)).replace(hour=15, minute=4, second=0, microsecond=0)
stamp = yesterday.astimezone(ZoneInfo("UTC")).strftime("%Y-%m-%dT%H:%M:%SZ")
conn = sqlite3.connect(db)
row = conn.execute(
    "SELECT id FROM stock_moves WHERE product_id = ? AND direction = 'in' ORDER BY at ASC LIMIT 1",
    (product_id,),
).fetchone()
if row is None:
    raise SystemExit("no stock in to shift")
conn.execute("UPDATE stock_moves SET at = ? WHERE id = ?", (stamp, row[0]))
conn.execute("UPDATE item_instances SET stock_in_at = ? WHERE stock_move_id = ?", (stamp, row[0]))
conn.commit()
print("shifted stock in to", stamp)
`, dbPath, productId], { stdio: 'inherit' });
}

async function assertNavFits(page, width) {
  await page.setViewportSize({ width, height: 700 });
  const nav = page.getByRole('navigation', { name: 'Sections' });
  for (const name of ['Scan Queue', 'Inventory', 'History', 'Shopping List']) {
    const link = nav.getByRole('link', { name });
    const box = await link.boundingBox();
    assert(box !== null && box.width > 0, `${name} has a box at ${width}px`);
  }
  const history = await nav.getByRole('link', { name: 'History' }).boundingBox();
  const shopping = await nav.getByRole('link', { name: 'Shopping List' }).boundingBox();
  assert(history.x >= 0 && shopping.x + shopping.width <= width + 1, `tabs fit in ${width}px without running off the screen`);
}

async function saveArtifact(fromName, toName) {
  await mkdir(ARTIFACTS, { recursive: true });
  await copyFile(join(EVIDENCE_DIR, fromName), join(ARTIFACTS, toName));
}
