// drive-shopping-buttons.mjs — checks Build / Send line up, then the built plan.
//
// Desktop: the two buttons share a width and sit on the title's center line.
// Phone: the same pair is equal width, and Send stays disabled until the list is built.
//
//   scripts/pantry-verify.sh drive scripts/drive-shopping-buttons.mjs shopping-buttons
import { mkdirSync } from 'node:fs';
import {
  openBrowser, createKnownProduct, scanBarcode, setScanExpiration, setScannerMode, captureProof, assert,
} from './harness.mjs';

const BARCODE = '910000000031';
const PRODUCT = 'Verify Button Beans';
const UNIT = 'can';
const PHONE = { width: 390, height: 844 };
const DESKTOP = { width: 1280, height: 720 };

async function boxes(page, actionName) {
  const fill = page.getByRole('button', { name: actionName });
  const send = page.getByRole('button', { name: 'Send to Kroger' });
  const title = page.getByRole('heading', { name: 'Shopping plan', exact: true });
  await fill.waitFor({ state: 'visible' });
  await send.waitFor({ state: 'visible' });
  const [fillBox, sendBox, titleBox] = await Promise.all([
    fill.boundingBox(),
    send.boundingBox(),
    title.boundingBox(),
  ]);
  assert(fillBox !== null && sendBox !== null && titleBox !== null, 'button row is missing a box');
  return { fill, send, title, fillBox, sendBox, titleBox };
}

function assertPair(label, fillBox, sendBox) {
  assert(Math.abs(fillBox.width - sendBox.width) < 2, `${label} widths ${fillBox.width} and ${sendBox.width}`);
  assert(Math.abs(fillBox.y - sendBox.y) < 2, `${label} tops ${fillBox.y} and ${sendBox.y}`);
  assert(Math.abs(fillBox.height - sendBox.height) < 2, `${label} heights ${fillBox.height} and ${sendBox.height}`);
  assert(sendBox.x >= fillBox.x + fillBox.width - 1, `${label} Send sits left of Fill`);
}

async function stockInUnit(page, expiry) {
  const card = await scanBarcode(page, BARCODE);
  await card.getByRole('heading', { name: PRODUCT }).waitFor({ state: 'visible' });
  await setScanExpiration(page, card, expiry);
  await card.getByRole('button', { name: 'Approve', exact: true }).click();
  await card.waitFor({ state: 'detached', timeout: 15_000 });
}

const { browser, page } = await openBrowser({ viewport: DESKTOP });
let failed = false;
try {
  mkdirSync('/opt/cursor/artifacts', { recursive: true });

  await page.goto('/shopping');
  await page.getByRole('heading', { name: 'Shopping plan' }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('Adds these items to your Kroger cart.').waitFor({ state: 'visible' });
  let row = await boxes(page, 'Build the list');
  assert(await row.send.isDisabled(), 'Send is enabled on an empty desktop plan');
  assertPair('desktop', row.fillBox, row.sendBox);
  const titleMid = row.titleBox.y + row.titleBox.height / 2;
  const buttonMid = row.fillBox.y + row.fillBox.height / 2;
  assert(Math.abs(titleMid - buttonMid) < 4, `desktop title mid ${titleMid} button mid ${buttonMid}`);
  await page.getByText('Build the list before sending it to Kroger.').waitFor({ state: 'visible' });
  await captureProof(page, 'shopping-plan-desktop-empty', {
    fillWidth: row.fillBox.width,
    sendWidth: row.sendBox.width,
    alignedWithTitle: true,
    action: 'Build the list',
  });
  await page.screenshot({ path: '/opt/cursor/artifacts/shopping-plan-desktop-empty.png' });

  await page.setViewportSize(PHONE);
  row = await boxes(page, 'Build the list');
  assert(await row.send.isDisabled(), 'Send is enabled on an empty phone plan');
  assertPair('phone empty', row.fillBox, row.sendBox);
  const toolbar = await page.locator('.shopping-toolbar').boundingBox();
  assert(toolbar !== null, 'toolbar is missing');
  assert(Math.abs(row.fillBox.x - toolbar.x) < 2, 'phone buttons do not share the title row left edge');
  assert(Math.abs((row.sendBox.x + row.sendBox.width) - (toolbar.x + toolbar.width)) < 2, 'phone buttons do not share the title row right edge');
  await page.getByText('Build the list before sending it to Kroger.').waitFor({ state: 'visible' });
  await captureProof(page, 'shopping-plan-phone-empty', { send: 'disabled', action: 'Build the list' });
  await page.screenshot({ path: '/opt/cursor/artifacts/shopping-plan-phone-empty.png' });

  await page.setViewportSize(DESKTOP);
  await createKnownProduct(page, {
    barcode: BARCODE, name: PRODUCT, category: 'Canned', unitOfMeasure: UNIT,
  });
  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });
  await stockInUnit(page, '2032-06-01');
  await stockInUnit(page, '2032-08-01');
  await page.getByRole('link', { name: 'Inventory' }).click();
  await page.getByRole('button', { name: 'This scan is complete' }).click();
  await page.getByRole('alert', { name: 'Opening inventory' }).waitFor({ state: 'hidden', timeout: 10_000 });
  await page.getByRole('link', { name: 'Scan Queue' }).click();
  await setScannerMode(page, 'stock_out');
  const outCard = await scanBarcode(page, BARCODE);
  await outCard.getByRole('radio', { name: 'Use oldest available automatically' }).waitFor({ state: 'visible', timeout: 15_000 });
  await outCard.getByRole('button', { name: 'Approve', exact: true }).click();
  await outCard.waitFor({ state: 'detached', timeout: 15_000 });

  await page.getByRole('link', { name: 'Shopping List' }).click();
  await page.getByRole('button', { name: 'Build the list' }).click();
  const derived = page.getByRole('row').filter({ hasText: PRODUCT }).filter({ has: page.getByText('Derived', { exact: true }) });
  await derived.getByText(`1 ${UNIT}`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByRole('button', { name: 'Update the list' }).waitFor({ state: 'visible' });
  await page.getByText('Build the list before sending it to Kroger.').waitFor({ state: 'hidden' });

  row = await boxes(page, 'Update the list');
  assert(await row.send.isEnabled(), 'Send stays disabled after the list is built');
  assertPair('desktop built', row.fillBox, row.sendBox);
  const builtTitleMid = row.titleBox.y + row.titleBox.height / 2;
  const builtButtonMid = row.fillBox.y + row.fillBox.height / 2;
  assert(Math.abs(builtTitleMid - builtButtonMid) < 4, `built title mid ${builtTitleMid} button mid ${builtButtonMid}`);
  await captureProof(page, 'shopping-plan-desktop-built', { send: 'enabled', product: PRODUCT, action: 'Update the list' });
  await page.screenshot({ path: '/opt/cursor/artifacts/shopping-plan-desktop-built.png' });

  await page.setViewportSize(PHONE);
  row = await boxes(page, 'Update the list');
  assert(await row.send.isEnabled(), 'Send stays disabled on the phone after the list is built');
  assertPair('phone built', row.fillBox, row.sendBox);
  await captureProof(page, 'shopping-plan-phone-built', { send: 'enabled', product: PRODUCT, action: 'Update the list' });
  await page.screenshot({ path: '/opt/cursor/artifacts/shopping-plan-phone-built.png' });
  console.log('PASS: shopping plan — Build, then Update, with Send aligned on desktop and phone');
} catch (error) {
  failed = true;
  console.error(error);
  try { await captureProof(page, 'shopping-buttons-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}

if (failed) process.exit(1);
