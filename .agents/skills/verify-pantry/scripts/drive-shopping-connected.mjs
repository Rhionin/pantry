// drive-shopping-connected.mjs — phone shopping page with Kroger already connected.
//
// Credential fields stay off the page. Daily cart actions stay. Editing client
// id, secret, redirect URI, and modality, and disconnecting, are the header menu.
// A real Kroger OAuth redirect cannot finish here, so the connected row is
// seeded in the server's SQLite database after credentials are saved through
// the API. The page then loads that state itself.
//
//   scripts/pantry-verify.sh drive scripts/drive-shopping-connected.mjs shopping-connected
import { execFileSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import {
  openBrowser, captureProof, assert, API_URL,
} from './harness.mjs';

const DB_PATH = process.env.PANTRY_DB_PATH ?? '';

function seedConnected(providerId) {
  assert(DB_PATH !== '', 'PANTRY_DB_PATH is required to seed a connected store');
  const sql = `
    INSERT INTO provider_connections (id, user_id, provider_id, state, access_token, refresh_token)
    VALUES ('user-1:${providerId}', 'user-1', '${providerId}', 'connected', 'verify-access', 'verify-refresh')
    ON CONFLICT(user_id, provider_id) DO UPDATE SET state = 'connected';
  `;
  execFileSync('python3', ['-c', `
import sqlite3, sys
conn = sqlite3.connect(sys.argv[1], timeout=10)
conn.execute(sys.argv[2])
conn.commit()
`, DB_PATH, sql], { stdio: 'inherit' });
}

const { browser, page } = await openBrowser({ viewport: { width: 390, height: 844 } });
let failed = false;
try {
  const saved = await page.request.put(`${API_URL}/api/providers/kroger/credentials`, {
    data: {
      clientId: 'pantry-verify-client',
      clientSecret: 'pantry-verify-secret',
      redirectUri: 'https://pantry.example/api/providers/kroger/callback',
      modality: 'PICKUP',
    },
  });
  if (!saved.ok()) throw new Error(`save credentials failed: ${saved.status()} ${await saved.text()}`);
  seedConnected('kroger');

  await page.goto('/shopping');
  await page.getByRole('heading', { name: 'Shopping list' }).waitFor({ state: 'visible' });
  await page.getByText('Connected', { exact: true }).waitFor({ state: 'visible' });

  assert(await page.getByLabel('Client ID').count() === 0, 'Client ID is on the shopping page');
  assert(await page.getByLabel('Client secret').count() === 0, 'Client secret is on the shopping page');
  assert(await page.getByLabel('Redirect URI').count() === 0, 'Redirect URI is on the shopping page');
  assert(await page.getByLabel('Modality').count() === 0, 'Modality is on the shopping page');
  assert(await page.getByRole('button', { name: 'Save credentials' }).count() === 0, 'Save credentials is on the shopping page');
  assert(await page.getByText('A client secret is saved and is not shown.').count() === 0, 'saved-secret note is on the shopping page');
  assert(await page.getByRole('button', { name: 'Add to Kroger cart' }).isVisible(), 'Add to Kroger cart is missing');
  assert(await page.getByRole('button', { name: 'Disconnect' }).count() === 0, 'Disconnect is still on the shopping page');
  assert(await page.getByRole('button', { name: 'Start a new cart' }).isVisible(), 'Start a new cart is missing');
  assert(await page.getByRole('button', { name: 'Add to shopping list' }).isVisible(), 'Add to shopping list is missing');
  assert(await page.getByText('Your shopping list is empty.').isVisible(), 'empty shopping list is missing');

  await captureProof(page, 'shopping-connected-phone', {
    connectionState: 'connected',
    credentialsOnPage: false,
    disconnectOnPage: false,
  });
  mkdirSync('/opt/cursor/artifacts', { recursive: true });
  await page.screenshot({ path: '/opt/cursor/artifacts/phone_shopping_no_disconnect.png' });

  assert(await page.getByRole('button', { name: 'Setup', exact: true }).count() === 0, 'Setup stayed on the Kroger row');
  await page.getByRole('button', { name: 'Menu' }).click();
  const disconnectItem = page.getByRole('menuitem', { name: 'Disconnect Kroger' });
  await disconnectItem.waitFor({ state: 'visible' });
  await page.waitForFunction(() => {
    const node = document.querySelector('.mantine-Menu-dropdown');
    return node !== null && getComputedStyle(node).opacity === '1';
  });
  await captureProof(page, 'shopping-menu-disconnect', { menuItem: 'Disconnect Kroger' });
  await page.screenshot({ path: '/opt/cursor/artifacts/phone_menu_disconnect.png' });
  await page.getByRole('menuitem', { name: 'Edit Kroger credentials' }).click();
  await page.getByRole('dialog', { name: 'Kroger credentials' }).waitFor({ state: 'visible' });
  const clientId = page.getByLabel('Client ID');
  await clientId.waitFor({ state: 'visible' });
  assert((await clientId.inputValue()) === 'pantry-verify-client', 'saved client id was not shown');
  assert(await page.getByLabel('Client secret').isVisible(), 'Client secret is missing from setup');
  assert(await page.getByLabel('Redirect URI').isVisible(), 'Redirect URI is missing from setup');
  assert(await page.getByLabel('Modality').isVisible(), 'Modality is missing from setup');
  assert(await page.getByText('A client secret is saved and is not shown.').isVisible(), 'saved-secret note is missing');
  assert(await page.getByRole('button', { name: 'Clear saved credentials' }).isVisible(), 'Clear saved credentials is missing');

  await page.getByLabel('Modality').selectOption('DELIVERY');
  const savedResponse = page.waitForResponse((res) => (
    res.url().includes('/api/providers/kroger/credentials')
    && res.request().method() === 'PUT'
    && res.ok()
  ));
  await page.getByRole('button', { name: 'Save credentials' }).click();
  await savedResponse;
  const listed = await page.request.get(`${API_URL}/api/providers`);
  if (!listed.ok()) throw new Error(`providers read failed: ${listed.status()}`);
  const providers = await listed.json();
  const kroger = providers.find((row) => row.id === 'kroger');
  assert(kroger?.connectionState === 'connected', `connectionState ${kroger?.connectionState}`);
  assert(kroger?.credentials?.modality === 'DELIVERY', `modality ${kroger?.credentials?.modality}`);
  assert(kroger?.credentials?.secretSet === true, 'saved secret was cleared');

  await captureProof(page, 'shopping-credentials-menu', { modality: 'DELIVERY' });

  await page.keyboard.press('Escape');
  await page.getByLabel('Client ID').waitFor({ state: 'hidden' });
  assert(await page.getByText('Connected', { exact: true }).isVisible(), 'Connected badge left the page');
  assert(await page.getByLabel('Client ID').count() === 0, 'Client ID stayed on the page after closing setup');

  await page.getByRole('button', { name: 'Menu' }).click();
  const disconnected = page.waitForResponse((res) => (
    res.url().includes('/api/providers/kroger/connection')
    && res.request().method() === 'DELETE'
    && res.ok()
  ));
  await page.getByRole('menuitem', { name: 'Disconnect Kroger' }).click();
  await disconnected;
  await page.getByText('Disconnected', { exact: true }).waitFor({ state: 'visible' });
  await page.getByRole('button', { name: 'Connect' }).waitFor({ state: 'visible' });
  assert(await page.getByRole('button', { name: 'Add to Kroger cart' }).isVisible(), 'Add to Kroger cart left after disconnect');
  assert(await page.getByRole('button', { name: 'Disconnect' }).count() === 0, 'Disconnect returned to the shopping page');
  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Edit Kroger credentials' }).waitFor({ state: 'visible' });
  assert(await page.getByRole('menuitem', { name: 'Disconnect Kroger' }).count() === 0, 'Disconnect stayed in the menu after it ran');
} catch (error) {
  failed = true;
  console.error(error);
} finally {
  await browser.close();
}

if (failed) process.exit(1);
