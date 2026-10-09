// drive-scan-alerts.mjs — proves scan alerts through the web UI.
//
// Settings shows Scan alerts on, and a note when the browser has blocked
// notifications. With permission granted, an unrecognized barcode notifies
// immediately and a recognized stock-in scan notifies once the page clock has
// moved 5 quiet minutes. Run through scripts/pantry-verify.sh.
import { copyFile, mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import {
  openBrowser, createKnownProduct, scanBarcode, captureProof, assert, WEB_URL, EVIDENCE_DIR,
} from './harness.mjs';

const UNKNOWN = '910000000088';
const KNOWN = '910000000089';
const PRODUCT = 'Verify Alert Oats';
const BLOCKED = "Scan alerts are blocked. Re-allow notifications for this site in the browser's site settings.";
const viewport = { width: 412, height: 915 };

const publishShot = async (name) => {
  const artifacts = '/opt/cursor/artifacts';
  await mkdir(artifacts, { recursive: true });
  await copyFile(join(EVIDENCE_DIR, `${name}.png`), join(artifacts, `${name}.png`));
};

const openSettings = async (page) => {
  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Settings' }).click();
  await page.getByRole('menu').waitFor({ state: 'hidden', timeout: 5_000 });
  await page.getByRole('switch', { name: 'Scan alerts' }).waitFor({ state: 'visible', timeout: 15_000 });
};

let failed = false;
try {
  const denied = await openBrowser({ headless: true, viewport });
  try {
    await denied.page.goto(WEB_URL);
    await openSettings(denied.page);
    const toggle = denied.page.getByRole('switch', { name: 'Scan alerts' });
    assert(await toggle.isChecked(), 'scan alerts default on when the browser has blocked them');
    await denied.page.getByText(BLOCKED).waitFor({ state: 'visible' });
    const permission = await denied.page.evaluate(() => (
      typeof Notification === 'undefined' ? 'unsupported' : Notification.permission
    ));
    assert(permission === 'denied', `denied screenshot expects permission denied (got ${permission})`);
    await captureProof(denied.page, 'scan-alerts-denied', {
      feature: 'scan-alerts',
      step: 'permission-denied',
      permission,
    });
    await publishShot('scan-alerts-denied');
  } finally {
    await denied.browser.close();
  }

  const { browser, page } = await openBrowser({
    // Headless Chromium keeps notification permission at denied, so the
    // switch-on state and the live alerts use installed Chrome.
    channel: 'chrome',
    headless: false,
    viewport,
    permissions: ['notifications'],
  });
  try {
    await page.clock.install();
    await page.goto(WEB_URL);
    await openSettings(page);
    const toggle = page.getByRole('switch', { name: 'Scan alerts' });
    assert(await toggle.isChecked(), 'scan alerts default on');
    const description = await toggle.getAttribute('aria-describedby');
    assert(description !== null && description !== '', 'scan alerts describes when they fire');
    const described = await page.locator(`[id="${description}"]`).innerText();
    assert(
      described.includes('Failed scans and unrecognized products alert right away'),
      `scan alerts description names immediate alerts (got ${described})`,
    );
    assert(await page.getByText(BLOCKED).count() === 0, 'a granted browser does not show the blocked note');
    assert(
      await page.getByRole('button', { name: 'Notify me about scans' }).count() === 0,
      'the opt-in button is gone',
    );
    const permission = await page.evaluate(() => Notification.permission);
    assert(permission === 'granted', `switch-on screenshot expects permission granted (got ${permission})`);
    await captureProof(page, 'scan-alerts-switch-on', {
      feature: 'scan-alerts',
      step: 'switch-on',
      permission,
    });
    await publishShot('scan-alerts-switch-on');

    await page.getByRole('link', { name: 'Scan Queue' }).click();
    await page.getByRole('textbox', { name: 'Barcode scanner input' }).waitFor({ state: 'visible' });
    await page.evaluate(() => {
      const native = window.Notification;
      const calls = [];
      window.__pantryAlerts = calls;
      function Wrapped(title, options) {
        const record = {
          title,
          body: options && options.body,
          tag: options && options.tag,
        };
        try {
          const notification = new native(title, options);
          record.shown = true;
          calls.push(record);
          return notification;
        } catch (error) {
          record.shown = false;
          record.error = String(error);
          calls.push(record);
          throw error;
        }
      }
      Wrapped.requestPermission = native.requestPermission.bind(native);
      Object.defineProperty(Wrapped, 'permission', { get: () => native.permission });
      window.Notification = Wrapped;
    });

    const unknownCard = await scanBarcode(page, UNKNOWN);
    await unknownCard.getByText('Flagged').waitFor({ state: 'visible' });
    await page.waitForFunction(
      () => window.__pantryAlerts.some((alert) => alert.title === 'Unrecognized product'),
      null,
      { timeout: 10_000 },
    );

    await createKnownProduct(page, {
      barcode: KNOWN, name: PRODUCT, category: 'Grocery', unitOfMeasure: 'box',
    });
    const knownCard = await scanBarcode(page, KNOWN);
    await knownCard.getByRole('heading', { name: PRODUCT }).waitFor({ state: 'visible' });
    const beforeQuiet = await alerts(page);
    assert(
      !beforeQuiet.some((alert) => alert.title === 'Stock in batch is ready'),
      `the batch alert is not immediate (got ${JSON.stringify(beforeQuiet)})`,
    );
    await page.clock.fastForward(4 * 60 * 1000);
    const duringQuiet = await alerts(page);
    assert(
      !duringQuiet.some((alert) => alert.title === 'Stock in batch is ready'),
      `the batch alert waits past 4 quiet minutes (got ${JSON.stringify(duringQuiet)})`,
    );
    await page.clock.fastForward(60 * 1000);
    await page.waitForFunction(
      () => window.__pantryAlerts.some((alert) => alert.title === 'Stock in batch is ready'),
      null,
      { timeout: 10_000 },
    );
    const sent = await alerts(page);
    const batch = sent.find((alert) => alert.title === 'Stock in batch is ready');
    assert(batch !== undefined, 'stock in batch alert was sent');
    assert(batch.body.includes('1 scan is waiting'), `batch body names one waiting scan (got ${batch.body})`);
    const unrecognized = sent.find((alert) => alert.title === 'Unrecognized product');
    assert(unrecognized !== undefined, 'unrecognized product alert was sent');
    assert(
      unrecognized.body.includes(UNKNOWN),
      `unrecognized body names the barcode (got ${unrecognized.body})`,
    );
    console.log('PASS: scan alerts — settings switch, denied note, unrecognized product, quiet batch');
  } finally {
    await browser.close();
  }
} catch (err) {
  failed = true;
  console.error('FAIL: scan alerts —', err);
} finally {
  process.exit(failed ? 1 : 0);
}

async function alerts(page) {
  return page.evaluate(() => window.__pantryAlerts);
}
