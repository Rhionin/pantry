// drive-scan-alerts.mjs — proves scan alerts through the web UI.
//
// The permission control is visible before any prompt. An unrecognized barcode
// notifies immediately. A recognized stock-in scan notifies once the page clock
// has moved 5 quiet minutes. Run through scripts/pantry-verify.sh.
import {
  openBrowser, createKnownProduct, scanBarcode, captureProof, assert, WEB_URL,
} from './harness.mjs';

const UNKNOWN = '910000000088';
const KNOWN = '910000000089';
const PRODUCT = 'Verify Alert Oats';

const { browser, context, page } = await openBrowser({
  // Installed Chrome starts notification permission at "default", so the ask
  // is on screen. Playwright's headless Chromium reports it as denied.
  channel: 'chrome',
  headless: false,
  viewport: { width: 390, height: 844 },
});

let failed = false;
try {
  await page.clock.install();
  await page.goto(WEB_URL);
  const ask = page.getByRole('button', { name: 'Notify me about scans' });
  await ask.waitFor({ state: 'visible', timeout: 15_000 });
  await ask.focus();
  assert(
    (await ask.getAttribute('aria-describedby')) === 'scan-alert-explainer',
    'the scan alert button describes what the alerts do',
  );
  await captureProof(page, 'scan-alert-cta', {
    feature: 'scan-alerts',
    step: 'permission-cta',
    permission: await page.evaluate(() => Notification.permission),
  });

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

  await context.grantPermissions(['notifications']);
  await ask.click();
  await ask.waitFor({ state: 'hidden', timeout: 10_000 });
  assert(
    await page.evaluate(() => Notification.permission) === 'granted',
    'permission is granted only after Notify me about scans',
  );

  const unknownCard = await scanBarcode(page, UNKNOWN);
  await unknownCard.getByText('Flagged').waitFor({ state: 'visible' });
  await page.waitForFunction(
    () => window.__pantryAlerts.some((alert) => alert.title === 'Unrecognized product'),
    null,
    { timeout: 10_000 },
  );
  await captureProof(page, 'scan-alert-unrecognized', {
    feature: 'scan-alerts',
    step: 'unrecognized',
    alerts: await alerts(page),
  });

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
  await captureProof(page, 'scan-alert-batch', {
    feature: 'scan-alerts',
    step: 'batch-settled',
    alerts: sent,
  });
  console.log('PASS: scan alerts — permission ask, unrecognized product, quiet batch');
} catch (err) {
  failed = true;
  console.error('FAIL: scan alerts —', err);
  try { await captureProof(page, 'scan-alert-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);

async function alerts(page) {
  return page.evaluate(() => window.__pantryAlerts);
}
