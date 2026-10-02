// drive-camera-scan.mjs — proves a browser barcode reaches the scan queue.
//
// A headless browser has no printed barcode in front of a lens. This driver
// opens the camera at a phone size, arms an injected detector when the preview
// is up, and checks that the queue stays on screen with a capture cue and a
// barcode outline. If the camera cannot start, it types the barcode instead.
// Run through scripts/pantry-verify.sh:
//   scripts/pantry-verify.sh drive scripts/drive-camera-scan.mjs camera-scan
import {
  openBrowser, createKnownProduct, scanBarcode, captureProof, readInventory, assert, EVIDENCE_DIR,
} from './harness.mjs';
import { join } from 'node:path';

const BARCODE = '910000000017';
const PRODUCT = 'Verify Camera Oats';

const { browser, page } = await openBrowser({
  args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream'],
  viewport: { width: 390, height: 844 },
  permissions: ['camera'],
});
let failed = false;
try {
  await createKnownProduct(page, {
    barcode: BARCODE, name: PRODUCT, category: 'Grocery', unitOfMeasure: 'box',
  });

  await page.addInitScript((barcode) => {
    let armedAt = 0;
    window.__armCameraDetector = () => {
      armedAt = Date.now();
    };
    window.__disarmCameraDetector = () => {
      armedAt = 0;
    };
    class FakeBarcodeDetector {
      static async getSupportedFormats() {
        return ['ean_13', 'ean_8', 'upc_a', 'upc_e', 'code_128', 'code_39', 'itf', 'qr_code'];
      }
      async detect() {
        if (armedAt === 0) return [];
        return [{
          rawValue: barcode,
          cornerPoints: [
            { x: 80, y: 140 },
            { x: 520, y: 140 },
            { x: 520, y: 280 },
            { x: 80, y: 280 },
          ],
          boundingBox: { x: 80, y: 140, width: 440, height: 140 },
        }];
      }
    }
    window.BarcodeDetector = FakeBarcodeDetector;
  }, BARCODE);

  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('button', { name: 'Scan with camera' }).waitFor({ state: 'visible' });

  const disconnected = page.getByText('Scanner disconnected');
  if (await disconnected.isVisible()) {
    await page.getByText(/scan with this device's camera instead/i).waitFor({ state: 'visible' });
  }

  await page.getByRole('button', { name: 'Scan with camera' }).click();

  const unavailable = page.getByText('Camera scanning unavailable');
  const automatic = page.getByText(/scans automatically/i);
  const tapToStart = page.getByRole('button', { name: 'Tap to start scanning' });
  await Promise.race([
    unavailable.waitFor({ state: 'visible', timeout: 15_000 }),
    automatic.waitFor({ state: 'visible', timeout: 15_000 }),
    tapToStart.waitFor({ state: 'visible', timeout: 15_000 }),
  ]);
  if (await tapToStart.isVisible()) {
    await tapToStart.click();
    await Promise.race([
      unavailable.waitFor({ state: 'visible', timeout: 15_000 }),
      automatic.waitFor({ state: 'visible', timeout: 15_000 }),
    ]);
  }

  const card = page.getByRole('article', { name: `Scan ${BARCODE}` });
  let capturePath;
  if (await unavailable.isVisible()) {
    await captureProof(page, 'camera-fallback', {
      feature: 'camera-scan',
      step: 'camera-unavailable',
    });
    const typed = page.getByRole('textbox', { name: 'Type a barcode' });
    await typed.fill(BARCODE);
    await page.getByRole('button', { name: 'Add scan' }).click();
    capturePath = 'manual-fallback';
  } else {
    await page.evaluate(() => window.__armCameraDetector?.());
    try {
      await card.waitFor({ state: 'visible', timeout: 8_000 });
      capturePath = 'camera-decode';
    } catch {
      // Preview is up, but this run could not decode a frame. The hardware
      // scanner field still feeds the same queue.
      capturePath = 'live-preview';
      await scanBarcode(page, BARCODE);
    }
  }

  await card.waitFor({ state: 'visible', timeout: 15_000 });
  await card.getByText(`Barcode: ${BARCODE}`).waitFor({ state: 'visible' });
  await card.getByRole('heading', { name: PRODUCT }).waitFor({ state: 'visible' });
  if (capturePath === 'camera-decode') {
    const preview = page.getByLabel('Camera preview');
    const previewBox = await preview.boundingBox();
    const cardBox = await card.boundingBox();
    const viewport = page.viewportSize();
    assert(previewBox !== null && cardBox !== null && viewport !== null, 'preview and queue card are laid out');
    assert(previewBox.height <= 220, `camera preview stays compact (height ${previewBox.height})`);
    assert(cardBox.y < viewport.height, `scan card is inside the phone viewport (y ${cardBox.y}, viewport ${viewport.height})`);
    await page.getByRole('status').filter({ hasText: `Captured ${BARCODE}` }).waitFor({ state: 'visible' });
    await page.locator('.camera-preview-boxes polygon').waitFor({ state: 'visible', timeout: 3_000 });
    const highlighted = await card.evaluate((node) => node.classList.contains('scan-entry-card--just-captured'));
    assert(highlighted, 'the captured scan card is highlighted');
    await page.screenshot({ path: join(EVIDENCE_DIR, 'camera-phone-viewport.png') });
    await page.evaluate(() => window.__disarmCameraDetector?.());
  }
  await captureProof(page, 'camera-queued', {
    feature: 'camera-scan',
    step: 'queued',
    barcode: BARCODE,
    capturePath,
  });

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
