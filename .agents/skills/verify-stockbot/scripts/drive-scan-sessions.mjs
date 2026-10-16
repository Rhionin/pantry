// drive-scan-sessions.mjs — proves Stock in session cards.
//
// A grocery trip (four scans, each step under 5 minutes) stays in the open
// card. An earlier two-scan trip, separated by more than 5 minutes, starts
// closed. Select all on the open card checks only that card.
//
// POST /api/scans always stamps scannedAt with now, so after the real scans
// land this driver writes earlier timestamps into the verification database
// and reloads. The queue then groups those stored times.
import { copyFile, mkdir } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import {
  openBrowser, createKnownProduct, scanBarcode, captureProof, assert, API_URL,
} from './harness.mjs';

const GROCERY = [
  { barcode: '910000000111', name: 'Oats', at: '2026-10-07T20:02:00Z' },
  { barcode: '910000000112', name: 'Milk', at: '2026-10-07T20:06:00Z' },
  { barcode: '910000000113', name: 'Pasta', at: '2026-10-07T20:10:00Z' },
  { barcode: '910000000114', name: 'Bread', at: '2026-10-07T20:14:00Z' },
];
const EARLIER = [
  { barcode: '910000000115', name: 'Yogurt', at: '2026-10-07T19:30:00Z' },
  { barcode: '910000000116', name: 'Butter', at: '2026-10-07T19:32:00Z' },
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
for row in conn.execute("SELECT barcode, scanned_at FROM scan_entries ORDER BY scanned_at"):
    print(row[0], row[1])
`;
  const args = [dbPath];
  for (const row of rows) args.push(row.barcode, row.at);
  execFileSync('python3', ['-c', script, ...args], { stdio: 'inherit' });
};

const { browser, page } = await openBrowser({
  viewport: { width: 390, height: 900 },
  timezoneId: 'America/New_York',
});
let failed = false;
try {
  const dbPath = process.env.PANTRY_DB_PATH;
  assert(typeof dbPath === 'string' && dbPath !== '', 'PANTRY_DB_PATH is set');

  for (const item of [...GROCERY, ...EARLIER]) {
    await createKnownProduct(page, {
      barcode: item.barcode,
      name: item.name,
      category: 'Grocery',
      unitOfMeasure: 'bag',
    });
  }

  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });
  for (const item of [...GROCERY, ...EARLIER]) {
    await scanBarcode(page, item.barcode);
  }

  backdate(dbPath, [...GROCERY, ...EARLIER]);
  const listed = await page.request.get(`${API_URL}/api/scans?userId=user-1&status=pending`);
  if (!listed.ok()) throw new Error(`list scans failed: ${listed.status()}`);
  const pending = await listed.json();
  const oats = pending.find((entry) => entry.barcode === GROCERY[0].barcode);
  const yogurt = pending.find((entry) => entry.barcode === EARLIER[0].barcode);
  assert(oats?.scannedAt?.startsWith('2026-10-07T20:02:00'), `oats scannedAt backdated (got ${oats?.scannedAt})`);
  assert(yogurt?.scannedAt?.startsWith('2026-10-07T19:30:00'), `yogurt scannedAt backdated (got ${yogurt?.scannedAt})`);

  await page.reload();
  await page.getByRole('heading', { name: 'Oats' }).waitFor({ state: 'visible', timeout: 10_000 });
  const openSession = page.getByRole('region', { name: /Scan session/ }).filter({
    has: page.getByRole('article', { name: `Scan ${GROCERY[0].barcode}` }),
  });
  const openToggle = openSession.getByRole('button', { expanded: true });
  await openToggle.waitFor({ state: 'visible' });
  const openName = await openToggle.innerText();
  assert(openName.includes('4 scans'), `open session counts 4 scans (got ${openName})`);
  assert(openName.includes('4:02') && openName.includes('4:14'), `open session spans 4:02–4:14 (got ${openName})`);
  assert(openName.toLowerCase().includes('stock in'), `open session names stock in (got ${openName})`);
  for (const item of GROCERY) {
    await openSession.getByRole('heading', { name: item.name }).waitFor({ state: 'visible' });
    const row = openSession.getByRole('article', { name: `Scan ${item.barcode}` });
    await row.getByLabel('Unit count', { exact: true }).waitFor({ state: 'visible' });
    await row.getByRole('button', { name: 'Increase unit count' }).waitFor({ state: 'visible' });
  }
  for (const item of EARLIER) {
    assert(await page.getByRole('heading', { name: item.name }).count() === 0, `${item.name} stays inside the closed session`);
  }

  const closed = page.getByRole('button', { name: /2 scans/, expanded: false });
  await closed.waitFor({ state: 'visible' });
  const closedName = await closed.innerText();
  assert(closedName.includes('2 scans'), `closed session counts 2 scans (got ${closedName})`);
  assert(closedName.includes('3:30'), `closed session starts at 3:30 (got ${closedName})`);

  const selectAll = openSession.getByRole('checkbox', { name: /Select all eligible scans in/ });
  await selectAll.click();
  for (const item of GROCERY) {
    const box = openSession.getByRole('article', { name: `Scan ${item.barcode}` }).getByRole('checkbox');
    assert(await box.isChecked(), `${item.name} selected by this session's Select all`);
  }
  await closed.click();
  for (const item of EARLIER) {
    const row = page.getByRole('article', { name: `Scan ${item.barcode}` });
    await row.waitFor({ state: 'visible' });
    assert(!(await row.getByRole('checkbox').isChecked()), `${item.name} stays unselected`);
  }
  await page.getByRole('button', { name: /2 scans/, expanded: true }).click();
  await page.getByRole('heading', { name: 'Yogurt' }).waitFor({ state: 'hidden' });
  await selectAll.click();
  assert(!(await openSession.getByRole('article', { name: `Scan ${GROCERY[0].barcode}` }).getByRole('checkbox').isChecked()), 'Select all clears the open session');

  await page.getByRole('button', { name: 'Confirm 0 selected scans' }).waitFor({ state: 'visible' });

  const shoot = async (width, filename) => {
    await page.setViewportSize({ width, height: 900 });
    const toggle = page.getByRole('button', { name: /2 scans/, expanded: false });
    await toggle.scrollIntoViewIfNeeded();
    const box = await toggle.boundingBox();
    const height = Math.max(900, Math.ceil((box?.y ?? 0) + (box?.height ?? 0) + 32));
    await page.setViewportSize({ width, height });
    await page.screenshot({ path: join(process.env.PANTRY_EVIDENCE_DIR, filename) });
    await mkdir('/opt/cursor/artifacts', { recursive: true });
    await copyFile(join(process.env.PANTRY_EVIDENCE_DIR, filename), `/opt/cursor/artifacts/${filename}`);
  };
  await shoot(390, 'stock-in-sessions-mobile.png');
  await shoot(1280, 'stock-in-sessions-desktop.png');

  await captureProof(page, 'scan-sessions', {
    feature: 'scan-sessions',
    openSession: GROCERY.map((item) => item.name),
    closedSession: EARLIER.map((item) => item.name),
    selectAllScopedToOpenSession: true,
  });
  console.log('PASS: scan sessions — grocery card open, earlier card closed, select-all stays in the open card');
} catch (err) {
  failed = true;
  console.error('FAIL: scan sessions —', err.message);
  try { await captureProof(page, 'scan-sessions-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
