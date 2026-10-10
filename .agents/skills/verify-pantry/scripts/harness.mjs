// harness.mjs — shared Playwright driving helpers for verify-pantry drivers.
//
// A driver script imports these, opens a browser against PANTRY_WEB_URL, drives
// one feature the way a user would (HID scanner input, tab clicks, buttons),
// and writes proof (ARIA snapshot + screenshot + a JSON side-effect check)
// into PANTRY_EVIDENCE_DIR. It talks to the SAME UI a person uses; the only
// direct API calls are seeding a known product (a legitimate user precondition,
// since a real deployment gets products from Open Food Facts which is disabled
// here) and reading back stored state for side-effect proof.
//
// Chromium comes from the frontend's installed @playwright/test. Run drivers
// through scripts/pantry-verify.sh so the servers exist and are torn down.
// @playwright/test ships as CommonJS; import the default and destructure.
import { createRequire } from 'node:module';
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const scriptDir = dirname(fileURLToPath(import.meta.url));
const { chromium } = require(join(scriptDir, '../../../../frontend/node_modules/@playwright/test'));

export const WEB_URL = process.env.PANTRY_WEB_URL ?? 'http://127.0.0.1:5173';
export const API_URL = process.env.PANTRY_API_URL ?? 'http://127.0.0.1:18080';
export const EVIDENCE_DIR = process.env.PANTRY_EVIDENCE_DIR ?? process.cwd();
export const USER_ID = 'user-1';

export async function openBrowser({ args, viewport, permissions, timezoneId, headless, channel, colorScheme, deviceScaleFactor } = {}) {
  // headless Playwright Chromium reports notification permission as denied, so
  // a drive that must see the permission ask can pass channel: 'chrome'.
  const browser = await chromium.launch({
    ...(headless === undefined ? {} : { headless }),
    ...(channel ? { channel } : {}),
    ...(args ? { args } : {}),
  });
  const context = await browser.newContext({
    baseURL: WEB_URL,
    locale: 'en-US',
    ...(timezoneId ? { timezoneId } : {}),
    ...(viewport ? { viewport } : {}),
    ...(colorScheme ? { colorScheme } : {}),
    ...(deviceScaleFactor ? { deviceScaleFactor } : {}),
    ...(permissions ? { permissions } : {}),
  });
  const page = await context.newPage();
  return { browser, context, page };
}

// Seed a known product and bind it to a barcode, exactly as e2e/helpers.ts does.
// This is a user precondition, not the behavior under test: external Open Food
// Facts lookup is disabled in verification, so without a seeded override the
// barcode would flag as unknown.
export async function createKnownProduct(page, { barcode, name, category, unitOfMeasure }) {
  const create = await page.request.post(`${API_URL}/api/products`, {
    data: { name, category, unitOfMeasure },
  });
  if (!create.ok()) throw new Error(`create product failed: ${create.status()}`);

  const listed = await page.request.get(`${API_URL}/api/products`);
  const products = await listed.json();
  const product = products.find((p) => p.name === name);
  if (!product) throw new Error(`created product not returned: ${name}`);

  const override = await page.request.post(`${API_URL}/api/products/overrides`, {
    data: { barcode, productId: product.id },
  });
  if (!override.ok()) throw new Error(`override failed: ${override.status()}`);
  return product;
}

// Open the stock-in card's collapsed expiration control, fill a date, and wait
// until the blur PATCH lands. The confirmation keeps the date field hidden
// until "Add expiration" is used, then folds a saved date into "Expires …".
export async function setScanExpiration(page, card, expirationDate) {
  const addExpiration = card.getByRole('button', { name: 'Add expiration' });
  if (await addExpiration.isVisible()) {
    await addExpiration.click();
  }
  const expiry = card.getByLabel('Expiration date');
  await expiry.fill(expirationDate);
  const patchResponse = page.waitForResponse((response) => {
    const path = new URL(response.url()).pathname;
    return /\/api\/scans\/[^/]+$/.test(path)
      && response.request().method() === 'PATCH'
      && response.ok();
  });
  await expiry.blur();
  await patchResponse;
  await card.getByText(/^Expires /).waitFor({ state: 'visible', timeout: 10_000 });
}

// Type a barcode into the HID scanner input and press Enter, the way a physical
// scanner does. Returns the scan card locator for that barcode.
export async function scanBarcode(page, barcode) {
  const input = page.getByRole('textbox', { name: 'Barcode scanner input' });
  await input.fill(barcode);
  await input.press('Enter');
  const card = page.getByRole('article', { name: `Scan ${barcode}` });
  await card.waitFor({ state: 'visible', timeout: 15_000 });
  return card;
}

// Switch the scanner mode the way a user does: scan a reserved control barcode.
// A control barcode does NOT create a scan card - ScanQueuePage.captureBarcode
// classifies it, calls setScannerMode, and returns early server-side - so we
// wait for the on-screen `Mode: <mode>` banner instead of a card. The default
// control strings are 'STOCK_IN' / 'STOCK_OUT' (GET /api/scanner/config). This
// mirrors e2e/helpers.ts::setScannerMode.
export async function setScannerMode(page, mode) {
  const controlBarcode = mode === 'stock_out' ? 'STOCK_OUT' : 'STOCK_IN';
  const input = page.getByRole('textbox', { name: 'Barcode scanner input' });
  await input.fill(controlBarcode);
  await input.press('Enter');
  await page.getByText(`Mode: ${mode}`).waitFor({ state: 'visible', timeout: 10_000 });
}

// Capture proof: an ARIA snapshot of the page and a full-page screenshot, plus
// any structured side-effect object the caller wants recorded. All three land
// in EVIDENCE_DIR/<name>.* so they survive script-level cleanup.
export async function captureProof(page, name, sideEffect) {
  await mkdir(EVIDENCE_DIR, { recursive: true });
  const aria = await page.locator('body').ariaSnapshot();
  await writeFile(join(EVIDENCE_DIR, `${name}.aria.txt`), aria, 'utf8');
  await page.screenshot({ path: join(EVIDENCE_DIR, `${name}.png`), fullPage: true });
  if (sideEffect !== undefined) {
    await writeFile(
      join(EVIDENCE_DIR, `${name}.sideeffect.json`),
      JSON.stringify(sideEffect, null, 2),
      'utf8',
    );
  }
  return aria;
}

// Read inventory back through the API to prove a mutation's side effect from a
// second, read-only view (the user also sees this on the Inventory route).
export async function readInventory(page) {
  const res = await page.request.get(`${API_URL}/api/inventory`);
  if (!res.ok()) throw new Error(`inventory read failed: ${res.status()}`);
  return res.json();
}

export function assert(condition, message) {
  if (!condition) throw new Error(`ASSERT FAILED: ${message}`);
}
