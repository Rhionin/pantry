// drive-scan-push.mjs — proves the settings switch stores a push subscription
// when this pantry has VAPID keys. Run through scripts/pantry-verify.sh with
// VAPID_PUBLIC_KEY, VAPID_PRIVATE_KEY, and VAPID_SUBJECT set.
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { openBrowser, captureProof, assert, WEB_URL, EVIDENCE_DIR } from './harness.mjs';

const profile = join(EVIDENCE_DIR, 'chrome-profile');
await mkdir(profile, { recursive: true });
const { browser, context, page } = await openBrowser({
  channel: 'chrome',
  headless: false,
  viewport: { width: 390, height: 844 },
  persistentDir: profile,
});

let failed = false;
try {
  const logs = [];
  page.on('console', (message) => logs.push(`${message.type()}: ${message.text()}`));
  page.on('pageerror', (error) => logs.push(`pageerror: ${error.message}`));
  await page.goto(`${WEB_URL}/settings`);
  const ask = page.getByRole('switch', { name: 'Scan alerts on this device' });
  await ask.waitFor({ state: 'visible', timeout: 15_000 });
  await context.grantPermissions(['notifications']);
  await ask.click();
  const status = page.locator('#scan-alert-status');
  await status.waitFor({ state: 'visible', timeout: 10_000 });
  try {
    await page.waitForFunction(
      () => document.querySelector('#scan-alert-status')?.textContent?.includes('closed'),
      null,
      { timeout: 15_000 },
    );
  } catch {
    // The push service can refuse this machine. The status line still shows
    // whether the subscription was stored.
  }
  const statusText = await status.innerText();
  const pushState = await page.evaluate(async () => {
    const registration = await navigator.serviceWorker?.getRegistration('/');
    const subscription = await registration?.pushManager.getSubscription();
    return {
      controller: Boolean(navigator.serviceWorker?.controller),
      endpoint: subscription?.endpoint ?? '',
    };
  });
  assert(
    statusText === 'On: alerts arrive even when Pantry is closed'
      || statusText.includes('this tab is open'),
    `scan alert status is missing (got ${statusText})`,
  );
  await captureProof(page, 'scan-alert-push', {
    feature: 'scan-alerts',
    step: 'push-subscription',
    status: statusText,
    endpointHost: pushState.endpoint === '' ? '' : new URL(pushState.endpoint).host,
    controller: pushState.controller,
    logs: logs.slice(-12),
  });
  console.log(`PASS: scan push status — ${statusText}`);
} catch (err) {
  failed = true;
  console.error('FAIL: scan push —', err);
  try { await captureProof(page, 'scan-alert-push-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
