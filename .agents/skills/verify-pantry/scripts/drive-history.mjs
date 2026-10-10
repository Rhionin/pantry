// drive-history.mjs — pace, the recent trail, and correcting a mistaken move.
//
// User story: after stocking a known product in and using some of it, Inventory
// opens a history page that leads with days between uses and lists each move.
// A kebab on a move changes its quantity or undoes it, and the shelf and the
// pace follow. A group history lists every member's moves.
//
// Run: scripts/pantry-verify.sh drive scripts/drive-history.mjs history
import { copyFile, mkdir } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import {
  openBrowser, createKnownProduct, scanBarcode, setScannerMode, captureProof, readInventory, assert,
  WEB_URL, API_URL, EVIDENCE_DIR,
} from './harness.mjs';

const BLACK_BARCODE = '910000000171';
const KIDNEY_BARCODE = '910000000172';
const ARTIFACTS = '/opt/cursor/artifacts';

const phone = {
  viewport: { width: 390, height: 844 },
  deviceScaleFactor: 2,
  timezoneId: 'America/Chicago',
};

const { browser, page } = await openBrowser(phone);
let failed = false;

try {
  const opened = await page.request.post(`${API_URL}/api/onboarding/complete`);
  if (!opened.ok()) throw new Error(`finish opening failed: ${opened.status()}`);

  const black = await createKnownProduct(page, {
    barcode: BLACK_BARCODE, name: 'Black Beans', category: 'Canned', unitOfMeasure: 'cans',
  });
  const kidney = await createKnownProduct(page, {
    barcode: KIDNEY_BARCODE, name: 'Kidney Beans', category: 'Canned', unitOfMeasure: 'cans',
  });
  await sizeProduct(page, black.id, 'Black Beans');
  await sizeProduct(page, kidney.id, 'Kidney Beans');

  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 15_000 });
  await stockIn(page, BLACK_BARCODE, 'Black Beans', 10);
  await setScannerMode(page, 'stock_out');
  for (let i = 0; i < 8; i++) {
    await stockOut(page, BLACK_BARCODE, 'Black Beans');
  }
  await setScannerMode(page, 'stock_in');
  await stockIn(page, KIDNEY_BARCODE, 'Kidney Beans', 2);

  const before = await readInventory(page);
  const blackRow = before.find((row) => row.item.product.name === 'Black Beans');
  const kidneyRow = before.find((row) => row.item.product.name === 'Kidney Beans');
  assert(blackRow?.instanceCount === 2, `black beans on hand before history is 2 (got ${blackRow?.instanceCount})`);
  assert(kidneyRow?.instanceCount === 2, `kidney beans on hand is 2 (got ${kidneyRow?.instanceCount})`);

  // The scans just happened, so every use is today and the pace is not known yet.
  // Spread the recorded times across 60 days so the page can show a real pace.
  // The moves themselves still came from scanning and confirming.
  shiftBlackBeanDates(process.env.PANTRY_DB_PATH, black.id);

  await page.getByRole('link', { name: 'Inventory' }).click();
  const blackCard = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Black Beans', exact: true }) });
  await blackCard.getByRole('button', { name: 'History' }).waitFor({ state: 'visible', timeout: 10_000 });
  await captureProof(page, 'before-inventory-light', { scheme: 'light', surface: 'inventory' });
  await saveArtifact('before-inventory-light.png', 'before_inventory_light.png');

  await blackCard.getByRole('button', { name: 'History' }).click();
  await page.getByText('days between uses').waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('Steady compared with the previous 30 days').waitFor({ state: 'visible' });
  await page.getByText('Stocked in 10').waitFor({ state: 'visible' });
  const pace = await readHistory(page, blackRow.item.id);
  assert(pace.pace.known === true, 'pace is known');
  assert(pace.onHand === 2, `history on hand is 2 (got ${pace.onHand})`);
  assert(pace.pace.trend === 'steady', `trend is steady (got ${pace.pace.trend})`);
  await captureProof(page, 'after-history-light', {
    scheme: 'light',
    onHand: pace.onHand,
    daysBetweenUses: pace.pace.daysBetweenUses,
    daysLeft: pace.pace.daysLeft,
    trend: pace.pace.trend,
    moves: pace.moves.length,
  });
  await saveArtifact('after-history-light.png', 'after_history_light.png');

  const dark = await browser.newContext({
    ...phoneContext(),
    colorScheme: 'dark',
  });
  const darkPage = await dark.newPage();
  await darkPage.goto('/inventory');
  const darkCard = darkPage.getByRole('article').filter({ has: darkPage.getByRole('heading', { name: 'Black Beans', exact: true }) });
  await darkCard.getByRole('button', { name: 'History' }).waitFor({ state: 'visible', timeout: 10_000 });
  await captureProof(darkPage, 'before-inventory-dark', { scheme: 'dark', surface: 'inventory' });
  await saveArtifact('before-inventory-dark.png', 'before_inventory_dark.png');
  await darkCard.getByRole('button', { name: 'History' }).click();
  await darkPage.getByText('days between uses').waitFor({ state: 'visible', timeout: 10_000 });
  await captureProof(darkPage, 'after-history-dark', { scheme: 'dark', surface: 'history' });
  await saveArtifact('after-history-dark.png', 'after_history_dark.png');
  await dark.close();

  await page.getByRole('button', { name: /Actions for Used 1, Today/ }).click();
  await page.getByRole('menuitem', { name: 'Change quantity' }).click();
  const quantity = page.getByRole('dialog', { name: 'Change quantity' }).getByLabel('How many units were actually used?');
  await quantity.fill('2');
  await page.getByRole('button', { name: 'Save quantity' }).click();
  await page.getByText('Used 2').waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('1 on hand').waitFor({ state: 'visible' });
  const edited = await readHistory(page, blackRow.item.id);
  assert(edited.onHand === 1, `on hand after raising a use to 2 is 1 (got ${edited.onHand})`);
  const shelfAfterEdit = await readInventory(page);
  assert(shelfAfterEdit.find((row) => row.item.id === blackRow.item.id)?.instanceCount === 1, 'inventory count follows the edited use');
  await captureProof(page, 'after-quantity-light', { onHand: edited.onHand, daysBetweenUses: edited.pace.daysBetweenUses });
  await saveArtifact('after-quantity-light.png', 'after_quantity_corrected_light.png');

  await page.getByRole('button', { name: /Actions for Used 2, Today/ }).click();
  await page.getByRole('menuitem', { name: 'Undo this use' }).click();
  const dialog = page.getByRole('dialog', { name: 'Undo this use?' });
  await dialog.getByText('2 units go back on the shelf').waitFor({ state: 'visible' });
  await captureProof(page, 'undo-confirm-light', { confirm: 'undo this use' });
  await saveArtifact('undo-confirm-light.png', 'undo_confirm_light.png');
  await dialog.getByRole('button', { name: 'Keep it' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert((await readHistory(page, blackRow.item.id)).onHand === 1, 'keeping the move leaves the shelf alone');

  await page.getByRole('button', { name: /Actions for Used 2, Today/ }).click();
  await page.getByRole('menuitem', { name: 'Undo this use' }).click();
  await page.getByRole('dialog', { name: 'Undo this use?' }).getByRole('button', { name: 'Undo this use' }).click();
  await page.getByText('3 on hand').waitFor({ state: 'visible', timeout: 10_000 });
  const undone = await readHistory(page, blackRow.item.id);
  assert(undone.onHand === 3, `on hand after undo is 3 (got ${undone.onHand})`);
  assert(undone.pace.known === true, 'pace is still known after one use is removed');
  assert(undone.pace.daysBetweenUses !== edited.pace.daysBetweenUses, 'undoing a use recalculates the pace');
  const shelfAfterUndo = await readInventory(page);
  assert(shelfAfterUndo.find((row) => row.item.id === blackRow.item.id)?.instanceCount === 3, 'inventory count follows the undo');
  await captureProof(page, 'after-undo-light', {
    onHand: undone.onHand,
    daysBetweenUses: undone.pace.daysBetweenUses,
    previousDaysBetweenUses: edited.pace.daysBetweenUses,
  });
  await saveArtifact('after-undo-light.png', 'after_undo_light.png');

  await page.getByRole('navigation', { name: 'Sections' }).getByRole('link', { name: 'Inventory' }).click();
  await page.getByRole('button', { name: 'Select' }).click();
  await page.getByRole('checkbox', { name: 'Select Black Beans' }).check();
  await page.getByRole('checkbox', { name: 'Select Kidney Beans' }).check();
  await page.getByLabel('New group').fill('Beans');
  await page.getByRole('button', { name: 'Group these' }).click();
  const groupCard = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Beans', exact: true }) });
  await groupCard.getByRole('link', { name: 'History' }).click();
  await page.getByRole('heading', { name: 'Beans' }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('Black Beans').first().waitFor({ state: 'visible' });
  await page.getByText('Kidney Beans').first().waitFor({ state: 'visible' });
  await page.getByText('days between uses').waitFor({ state: 'visible' });
  await captureProof(page, 'group-history-light', { surface: 'group history' });
  await saveArtifact('group-history-light.png', 'group_history_light.png');

  console.log('PASS: history — pace, quantity correction, undo, and group trail');
} catch (err) {
  failed = true;
  console.error('FAIL: history —', err);
  try { await captureProof(page, 'history-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);

function phoneContext() {
  return {
    baseURL: WEB_URL,
    locale: 'en-US',
    timezoneId: phone.timezoneId,
    viewport: phone.viewport,
    deviceScaleFactor: phone.deviceScaleFactor,
  };
}

async function sizeProduct(page, id, name) {
  const update = await page.request.put(`${API_URL}/api/products/${id}`, {
    data: { name, category: 'Canned', unitOfMeasure: 'cans', netAmount: 15, netUnit: 'oz' },
  });
  if (!update.ok()) throw new Error(`size ${name} failed: ${update.status()} ${await update.text()}`);
}

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

async function readHistory(page, itemId) {
  const res = await page.request.get(`${API_URL}/api/items/${itemId}/history`);
  if (!res.ok()) throw new Error(`history read failed: ${res.status()}`);
  return res.json();
}

function shiftBlackBeanDates(dbPath, productId) {
  if (!dbPath) throw new Error('PANTRY_DB_PATH is not set');
  execFileSync('python3', ['-c', `
import sqlite3, sys
from datetime import datetime, timedelta, timezone
db, product_id = sys.argv[1], sys.argv[2]
now = datetime.now(timezone.utc).replace(microsecond=0)
conn = sqlite3.connect(db)
moves = conn.execute(
    "SELECT id, direction FROM stock_moves WHERE product_id = ? ORDER BY at ASC",
    (product_id,),
).fetchall()
ins = [row[0] for row in moves if row[1] == "in"]
outs = [row[0] for row in moves if row[1] == "out"]
if len(ins) != 1 or len(outs) != 8:
    raise SystemExit(f"expected 1 stock in and 8 uses, got {moves}")
def stamp(days):
    return (now - timedelta(days=days)).strftime("%Y-%m-%dT%H:%M:%SZ")
stocked = stamp(60)
conn.execute("UPDATE stock_moves SET at = ? WHERE id = ?", (stocked, ins[0]))
conn.execute("UPDATE item_instances SET stock_in_at = ? WHERE stock_move_id = ?", (stocked, ins[0]))
conn.execute("UPDATE stock_in_events SET at = ? WHERE stock_move_id = ?", (stocked, ins[0]))
for move_id, days in zip(outs, (55, 48, 42, 36, 28, 18, 9, 0)):
    at = stamp(days)
    conn.execute("UPDATE stock_moves SET at = ? WHERE id = ?", (at, move_id))
    conn.execute("UPDATE item_instances SET removed_at = ? WHERE removed_by_move_id = ?", (at, move_id))
    conn.execute("UPDATE consumption_events SET consumed_at = ? WHERE stock_move_id = ?", (at, move_id))
conn.commit()
print("shifted black bean history")
`, dbPath, productId], { stdio: 'inherit' });
}

async function saveArtifact(fromName, toName) {
  await mkdir(ARTIFACTS, { recursive: true });
  await copyFile(join(EVIDENCE_DIR, fromName), join(ARTIFACTS, toName));
}
