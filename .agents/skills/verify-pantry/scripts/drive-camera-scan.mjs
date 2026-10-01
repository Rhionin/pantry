// drive-camera-scan.mjs — proves a browser barcode reaches the scan queue.
//
// A headless browser has no barcode in front of a lens, so this driver does not
// claim to decode a printed code. It opens the opt-in camera control, then
// enqueues a barcode through whichever path the environment actually offers:
// the manual field shown when the camera cannot start, or the existing scanner
// input once a live preview is up. Either path uses the same capture handler
// as a decoded frame. Run through scripts/pantry-verify.sh:
//   scripts/pantry-verify.sh drive scripts/drive-camera-scan.mjs camera-scan
import {
  openBrowser, createKnownProduct, scanBarcode, captureProof, readInventory, assert,
} from './harness.mjs';

const BARCODE = '910000000017';
const PRODUCT = 'Verify Camera Oats';

const { browser, page } = await openBrowser();
let failed = false;
try {
  await createKnownProduct(page, {
    barcode: BARCODE, name: PRODUCT, category: 'Grocery', unitOfMeasure: 'box',
  });

  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('button', { name: 'Scan with camera' }).waitFor({ state: 'visible' });

  const disconnected = page.getByText('Scanner disconnected');
  if (await disconnected.isVisible()) {
    await page.getByText(/scan with this device's camera instead/i).waitFor({ state: 'visible' });
  }

  await page.getByRole('button', { name: 'Scan with camera' }).click();

  const unavailable = page.getByText('Camera scanning unavailable');
  const ready = page.getByText('Point the camera at a barcode.');
  await Promise.race([
    unavailable.waitFor({ state: 'visible', timeout: 15_000 }),
    ready.waitFor({ state: 'visible', timeout: 15_000 }),
  ]);

  let capturePath;
  if (await unavailable.isVisible()) {
    const typed = page.getByRole('textbox', { name: 'Type a barcode' });
    await typed.fill(BARCODE);
    await page.getByRole('button', { name: 'Add scan' }).click();
    capturePath = 'manual-fallback';
  } else {
    // The preview is live, but this environment has no barcode in frame.
    // The hardware-scanner field still feeds the same queue.
    capturePath = 'live-preview';
    await scanBarcode(page, BARCODE);
  }

  const card = page.getByRole('article', { name: `Scan ${BARCODE}` });
  await card.waitFor({ state: 'visible', timeout: 15_000 });
  await card.getByText(`Barcode: ${BARCODE}`).waitFor({ state: 'visible' });
  await card.getByRole('heading', { name: PRODUCT }).waitFor({ state: 'visible' });

  await card.getByRole('button', { name: 'Approve', exact: true }).click();
  await card.waitFor({ state: 'detached', timeout: 15_000 });

  const inventory = await readInventory(page);
  const row = inventory.find((item) => item.item.product.name === PRODUCT);
  assert(row !== undefined, `${PRODUCT} present in inventory`);
  assert(row.instanceCount === 1, `${PRODUCT} instanceCount is 1 (got ${row?.instanceCount})`);

  await page.getByRole('link', { name: 'Inventory' }).click();
  const invCard = page
    .getByRole('article')
    .filter({ has: page.getByRole('heading', { name: PRODUCT }) });
  await invCard.getByText('1 box', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });

  await captureProof(page, 'camera-scan', {
    feature: 'camera-scan',
    barcode: BARCODE,
    product: PRODUCT,
    capturePath,
    inventoryInstanceCount: row.instanceCount,
    unitOfMeasure: row.item.product.unitOfMeasure,
    note: 'Headless Chromium cannot decode a printed barcode from a camera frame. capturePath records whether the run used the camera fallback field or a live preview plus the scanner input. Both call the same queue capture handler as a decoded frame.',
  });
  console.log(`PASS: camera-scan — ${capturePath} enqueued ${BARCODE} and 1 box is in inventory`);
} catch (err) {
  failed = true;
  console.error('FAIL: camera-scan —', err.message);
  try { await captureProof(page, 'camera-scan-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
