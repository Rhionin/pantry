// drive-brand-deals.mjs — proves brand preference and sale offers on the shopping list.
//
// Two store brands of the same product share one replenishment line. Noting a
// sale on the other brand offers it beside Export to cart. Export stays
// available the whole time. Saving "always this brand" keeps that brand on the
// line and drops the sale offer.
//
//   scripts/pantry-verify.sh drive scripts/drive-brand-deals.mjs brand-deals
import {
  openBrowser, createKnownProduct, scanBarcode, setScanExpiration, captureProof, assert, API_URL,
} from './harness.mjs';

const GV = {
  barcode: '910000000021',
  name: 'Great Value Cut Green Beans',
};
const KR = {
  barcode: '910000000022',
  name: 'Kroger Cut Green Beans',
};
const UNIT = 'can';
const TARGET = 4;

async function stockIn(page, barcode) {
  const card = await scanBarcode(page, barcode);
  const name = barcode === GV.barcode ? GV.name : KR.name;
  await card.getByRole('heading', { name }).waitFor({ state: 'visible' });
  await setScanExpiration(page, card, '2032-06-01');
  await card.getByRole('button', { name: 'Approve', exact: true }).click();
  await card.waitFor({ state: 'detached', timeout: 15_000 });
}

async function setTarget(page, productName) {
  // A full navigation remounts the target panel. Selecting a second product
  // on the same page reuses that panel and keeps the previous confirmation.
  await page.goto('/inventory');
  const row = page
    .getByRole('article')
    .filter({ has: page.getByRole('heading', { name: productName, exact: true }) });
  await row.getByText(`1 ${UNIT}`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await row.getByRole('button', { name: 'View instances' }).click();
  await page.getByRole('button', { name: 'Get suggestion' }).click();
  await page.getByLabel('Manual target quantity').fill(String(TARGET));
  await page.getByRole('button', { name: 'Save manual target' }).click();
  await page.getByText(`Target quantity set to ${TARGET}.`).waitFor({ state: 'visible', timeout: 10_000 });
}

const { browser, page } = await openBrowser();
let failed = false;
try {
  await createKnownProduct(page, {
    barcode: GV.barcode, name: GV.name, category: 'Canned', unitOfMeasure: UNIT,
  });
  await createKnownProduct(page, {
    barcode: KR.barcode, name: KR.name, category: 'Canned', unitOfMeasure: UNIT,
  });

  await page.goto('/');
  await page.getByText('Mode: stock_in').waitFor({ state: 'visible', timeout: 10_000 });
  await stockIn(page, GV.barcode);
  await stockIn(page, KR.barcode);
  await setTarget(page, GV.name);
  await setTarget(page, KR.name);

  await page.getByRole('link', { name: 'Shopping List' }).click();
  const line = page.getByRole('row').filter({ hasText: GV.name });
  await line.getByText(`2 ${UNIT}`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await line.getByText('Derived', { exact: true }).waitFor({ state: 'visible' });
  await page.getByRole('button', { name: 'Export to cart' }).waitFor({ state: 'visible' });
  assert(
    await page.getByText('Derived', { exact: true }).count() === 1,
    'store brands share one derived line',
  );

  await page.getByLabel('Brand on sale').selectOption({ label: KR.name });
  await page.getByLabel('Sale price in cents').fill('79');
  await page.getByRole('button', { name: 'Note sale' }).click();
  await page.getByText(/Kroger Cut Green Beans is on sale at \$0\.79/).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText(/This list buys Great Value Cut Green Beans/).waitFor({ state: 'visible' });
  await page.getByRole('button', { name: 'Export to cart' }).click();
  await page.getByText('Nothing was sent. Connect a store to add these items to a cart.').waitFor({ state: 'visible', timeout: 10_000 });

  await page.getByRole('button', { name: 'Take the deal on Kroger Cut Green Beans' }).click();
  await page.getByRole('button', { name: 'Keep Great Value Cut Green Beans' }).waitFor({ state: 'visible' });
  await captureProof(page, 'sale-offer', { acceptedDeal: KR.name, line: GV.name });

  await page.getByLabel('Preferred brand for cut green beans').selectOption({ label: KR.name });
  await page.getByText(/Kroger Cut Green Beans is on sale at/).waitFor({ state: 'hidden', timeout: 10_000 });
  const preferenceSaved = page.waitForResponse((res) => (
    res.url().includes('/api/shopping-list/preferences') && res.request().method() === 'PUT' && res.ok()
  ));
  await page.locator('label').filter({ hasText: 'Always buy this brand of cut green beans' }).click();
  await preferenceSaved;

  const list = await page.request.get(`${API_URL}/api/shopping-list`);
  const entries = await list.json();
  const considerations = await (await page.request.get(`${API_URL}/api/shopping-list/considerations`)).json();
  const kroger = entries.find((entry) => entry.quantity === 2);
  assert(entries.length === 1, `one shared line, got ${entries.length}`);
  assert(kroger !== undefined, 'shared line is present');
  const note = considerations.considerations?.[0];
  assert(note?.ignorePrice === true, 'price is not the point for the saved brand');
  assert(note?.offer === null, 'a sale is not offered once the preferred brand is locked');
  assert(note?.preferredItemId === kroger.itemId, 'the line is the preferred brand');

  await captureProof(page, 'preferred-brand', {
    line: KR.name,
    quantity: 2,
    ignorePrice: true,
    offer: null,
  });
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
