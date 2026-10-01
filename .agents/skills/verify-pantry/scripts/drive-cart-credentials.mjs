// drive-cart-credentials.mjs — proves Kroger credentials are entered in the UI.
//
// The secret is saved on the server and is not returned by GET /api/providers.
// Clearing the saved row returns Kroger to unconfigured. Add to Kroger cart
// stays disabled until the store is connected.
//
//   scripts/pantry-verify.sh drive scripts/drive-cart-credentials.mjs cart-credentials
import { openBrowser, captureProof, assert, API_URL } from './harness.mjs';

const SECRET = 'ui-secret-not-in-get';

const { browser, page } = await openBrowser();
let failed = false;
try {
  await page.goto('/shopping');
  await page.getByRole('heading', { name: 'Shopping list' }).waitFor({ state: 'visible', timeout: 10_000 });
  const form = page.getByRole('form', { name: 'Kroger credentials' });
  await form.waitFor({ state: 'visible' });
  await page.getByText('unconfigured', { exact: true }).waitFor({ state: 'visible' });
  await page.getByText('Kroger is unconfigured.').waitFor({ state: 'visible' });

  await form.getByLabel('Client ID').fill('ui-client');
  await form.getByLabel('Client secret').fill(SECRET);
  await form.getByLabel('Redirect URI').fill(`${API_URL}/api/providers/kroger/callback`);
  await form.getByRole('button', { name: 'Save credentials' }).click();

  await page.getByText('A client secret is saved and is not shown.').waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('Disconnected', { exact: true }).waitFor({ state: 'visible' });
  await page.getByRole('button', { name: 'Connect' }).waitFor({ state: 'visible' });
  const secretField = form.getByLabel('Client secret');
  assert(await secretField.inputValue() === '', 'the secret field is empty after save');
  const addToCart = page.getByRole('button', { name: 'Add to Kroger cart' });
  await addToCart.waitFor({ state: 'visible' });
  assert(await addToCart.isDisabled(), 'Add to Kroger cart stays disabled until Kroger is connected');

  const saved = await page.request.get(`${API_URL}/api/providers`);
  const savedBody = await saved.text();
  assert(saved.ok(), 'GET /api/providers succeeded');
  assert(!savedBody.includes(SECRET), 'GET /api/providers does not contain the client secret');
  assert(savedBody.includes('"secretSet":true'), 'the response says a secret is saved');
  assert(!savedBody.includes('clientSecret'), 'the response has no clientSecret field');

  await captureProof(page, 'credentials-saved', {
    secretInGet: false,
    secretSet: true,
    addToCartDisabled: true,
  });

  await page.getByRole('button', { name: 'Clear saved credentials' }).click();
  await page.getByText('unconfigured', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('Kroger is unconfigured.').waitFor({ state: 'visible' });
  assert(await form.getByLabel('Client ID').inputValue() === '', 'clearing removes the client id from the form');

  const cleared = await page.request.get(`${API_URL}/api/providers`);
  const clearedBody = await cleared.text();
  assert(!clearedBody.includes(SECRET), 'the secret is gone from GET after clear');
  assert(clearedBody.includes('"source":"none"'), 'credentials source is none after clear');
  assert(clearedBody.includes('"secretSet":false'), 'no secret remains after clear');

  await captureProof(page, 'credentials-cleared', { source: 'none', secretSet: false });
} catch (error) {
  failed = true;
  console.error(error);
  try {
    await captureProof(page, 'failure', { error: String(error) });
  } catch (captureError) {
    console.error(captureError);
  }
} finally {
  await browser.close();
}

if (failed) process.exit(1);
