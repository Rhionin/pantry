// drive-diagnostics.mjs — opens Diagnostics and checks the page-load snapshot.
//
// Run through scripts/pantry-verify.sh:
//   scripts/pantry-verify.sh drive scripts/drive-diagnostics.mjs diagnostics
import { openBrowser, captureProof, assert, API_URL } from './harness.mjs';

const { browser, page } = await openBrowser();
let failed = false;
try {
  const report = page.waitForResponse(
    (res) => res.url().includes('/api/telemetry/client') && res.request().method() === 'POST',
    { timeout: 15_000 },
  );
  await page.goto('/');
  await report;
  await page.getByRole('heading', { name: 'Scan queue' }).waitFor({ state: 'visible', timeout: 15_000 });

  const snapRes = await page.request.get(`${API_URL}/api/telemetry`);
  assert(snapRes.ok(), `telemetry status ${snapRes.status()}`);
  const snap = await snapRes.json();
  assert(snap.pageLoad && snap.pageLoad.samples >= 1, 'pageLoad has a sample');
  assert(typeof snap.pageLoad.note === 'string' && snap.pageLoad.note.length > 0, 'pageLoad.note');
  assert(!JSON.stringify(snap).includes('barcode'), 'snapshot omits barcodes');

  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Diagnostics' }).click();
  await page.getByRole('heading', { name: 'Diagnostics' }).waitFor({ state: 'visible' });
  await page.getByText(/Time to first byte:/).waitFor({ state: 'visible' });
  await page.getByText(snap.pageLoad.note).waitFor({ state: 'visible' });

  await captureProof(page, 'diagnostics', {
    feature: 'diagnostics',
    dominant: snap.pageLoad.dominant,
    note: snap.pageLoad.note,
    samples: snap.pageLoad.samples,
    latest: snap.pageLoad.latest,
  });
  console.log('PASS: diagnostics —', snap.pageLoad.note);
} catch (err) {
  failed = true;
  console.error('FAIL: diagnostics —', err.message);
  try { await captureProof(page, 'diagnostics-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
