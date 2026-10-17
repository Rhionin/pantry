// drive-visible-scan-alert.mjs — a visible Stockbot window gets an in-app
// toast from the service worker instead of a system notification.
//
// The push is delivered inside the registered worker with a real PushEvent.
// Run through scripts/stockbot-verify.sh.
import { openBrowser, captureProof, assert, WEB_URL } from './harness.mjs';

const payload = {
  title: 'Unrecognized product',
  body: 'Barcode 000111 needs a product. Open the scan queue to match it.',
  tag: 'pantry-scan-unrecognized-flag-1',
  url: '/?scanAlert=stock_in',
};

const { browser, context, page } = await openBrowser({
  headless: true,
  viewport: { width: 390, height: 844 },
});

let failed = false;
try {
  await page.goto(`${WEB_URL}/inventory`);
  await page.getByRole('link', { name: 'Inventory' }).waitFor({ state: 'visible', timeout: 15_000 });
  await page.evaluate(() => {
    localStorage.setItem('pantry-scan-alerts', 'on');
  });

  const workerPromise = context.waitForEvent('serviceworker');
  await page.evaluate(async () => {
    await navigator.serviceWorker.register('/sw.js', { scope: '/', updateViaCache: 'none' });
    await navigator.serviceWorker.ready;
  });
  const worker = context.serviceWorkers()[0] ?? await workerPromise;

  const probe = await worker.evaluate(() => ({
    pushEvent: typeof PushEvent,
    suppress: typeof shouldSuppressSystemNotification,
  }));
  assert(probe.suppress === 'function', `worker did not load the suppress decision (got ${probe.suppress})`);
  assert(probe.pushEvent === 'function', `PushEvent is not constructable (got ${probe.pushEvent})`);

  const decision = await worker.evaluate(() => ({
    visible: shouldSuppressSystemNotification([{ visibilityState: 'visible', focused: false }]),
    hidden: shouldSuppressSystemNotification([{ visibilityState: 'hidden', focused: false }]),
    focused: shouldSuppressSystemNotification([{ visibilityState: 'hidden', focused: true }]),
    none: shouldSuppressSystemNotification([]),
  }));
  assert(decision.visible === true && decision.focused === true, `visible or focused should suppress (got ${JSON.stringify(decision)})`);
  assert(decision.hidden === false && decision.none === false, `hidden or absent windows should notify (got ${JSON.stringify(decision)})`);

  await worker.evaluate((body) => {
    self.dispatchEvent(new PushEvent('push', { data: JSON.stringify(body) }));
  }, payload);

  const toast = page.locator('.mantine-Notification-root');
  await toast.getByText('Unrecognized product').waitFor({ state: 'visible', timeout: 10_000 });
  await page.waitForFunction(() => {
    const el = document.querySelector('.mantine-Notification-root');
    if (!el) return false;
    const style = getComputedStyle(el);
    const rect = el.getBoundingClientRect();
    const button = el.querySelector('.scan-alert-toast-open');
    const buttonRect = button?.getBoundingClientRect();
    return Number(style.opacity) > 0.95
      && rect.top >= 108
      && rect.bottom <= window.innerHeight
      && buttonRect !== undefined
      && buttonRect.top >= rect.top
      && buttonRect.bottom <= rect.bottom
      && buttonRect.bottom <= window.innerHeight;
  }, null, { timeout: 5_000 });
  const layout = await toast.evaluate((el) => {
    const rect = el.getBoundingClientRect();
    const button = el.querySelector('.scan-alert-toast-open');
    let container = el.parentElement;
    while (container && getComputedStyle(container).position !== 'fixed') {
      container = container.parentElement;
    }
    const link = document.querySelector('a[href="/inventory"]');
    const linkRect = link?.getBoundingClientRect();
    const hit = linkRect
      ? document.elementFromPoint(linkRect.x + 8, linkRect.y + linkRect.height / 2)
      : null;
    return {
      y: rect.y,
      height: rect.height,
      containerTop: container ? getComputedStyle(container).top : '',
      containerPointerEvents: container ? getComputedStyle(container).pointerEvents : '',
      headerHit: hit?.closest('a')?.textContent?.trim() ?? '',
      buttonBackground: button ? getComputedStyle(button).backgroundColor : '',
    };
  });
  assert(layout.headerHit === 'Inventory', `the toast covers the header (hit ${layout.headerHit})`);
  assert(layout.containerPointerEvents === 'none', `the toast container traps clicks (${layout.containerPointerEvents})`);
  assert(layout.buttonBackground === 'rgb(245, 158, 11)', `toast action color was ${layout.buttonBackground}`);
  const notes = await worker.evaluate(async () => {
    const list = await self.registration.getNotifications();
    return list.map((note) => note.title);
  });
  assert(notes.length === 0, `a visible window still raised a system notification: ${notes.join(', ')}`);
  await captureProof(page, 'visible-scan-alert-toast', {
    feature: 'scan-alerts',
    step: 'toast-while-visible',
    systemNotifications: notes,
    path: new URL(page.url()).pathname,
  });

  await toast.getByRole('button', { name: 'Open the scan queue' }).click();
  await page.getByRole('tab', { name: /^Stock in/ }).waitFor({ state: 'visible', timeout: 10_000 });
  const selected = await page.getByRole('tab', { name: /^Stock in/ }).getAttribute('aria-selected');
  assert(selected === 'true', `stock in tab was not selected (aria-selected=${selected})`);
  await toast.waitFor({ state: 'hidden', timeout: 5_000 }).catch(() => {});
  await captureProof(page, 'visible-scan-alert-opened', {
    feature: 'scan-alerts',
    step: 'toast-opened-queue',
    path: new URL(page.url()).pathname,
    stockInSelected: selected,
  });
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(`${WEB_URL}/inventory`);
  await page.getByRole('heading', { name: 'Inventory' }).waitFor({ state: 'visible', timeout: 15_000 });
  await worker.evaluate((body) => {
    self.dispatchEvent(new PushEvent('push', { data: JSON.stringify(body) }));
  }, { ...payload, title: 'Scan failed', body: 'Open the scan queue to try again.', tag: 'pantry-scan-failure-desktop' });
  await page.waitForFunction(() => {
    const el = document.querySelector('.mantine-Notification-root');
    if (!el) return false;
    const rect = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return Number(style.opacity) > 0.95 && rect.top >= 60 && rect.top < 100 && rect.bottom <= window.innerHeight;
  }, null, { timeout: 5_000 });
  const desktopHit = await page.evaluate(() => {
    const link = document.querySelector('a[href="/inventory"]');
    const linkRect = link?.getBoundingClientRect();
    const hit = linkRect
      ? document.elementFromPoint(linkRect.x + 8, linkRect.y + linkRect.height / 2)
      : null;
    return hit?.closest('a')?.textContent?.trim() ?? '';
  });
  assert(desktopHit === 'Inventory', `desktop toast covers the header (hit ${desktopHit})`);
  await captureProof(page, 'visible-scan-alert-desktop', {
    feature: 'scan-alerts',
    step: 'toast-below-wide-header',
  });

  console.log('PASS: visible scan alert — in-app toast, no system notification, queue opened');
} catch (err) {
  failed = true;
  console.error('FAIL: visible scan alert —', err);
  try { await captureProof(page, 'visible-scan-alert-failure'); } catch { /* best effort */ }
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
