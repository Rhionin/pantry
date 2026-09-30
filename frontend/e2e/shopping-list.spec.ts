import { expect, test } from '@playwright/test'
import { commitSelectedScan, createKnownProduct, inventoryRow, resetToStockIn, scanBarcode, setScannerMode } from './helpers'

const barcode = '210000000004'
const productName = 'E2E Shopping Rice'

test('inventory gap is derived, purchased, and leaves inventory unchanged', async ({ page, request }) => {
  await createKnownProduct(request, {
    barcode,
    name: productName,
    category: 'Dry goods',
    unitOfMeasure: 'bag',
  })
  await page.goto('/')
  await resetToStockIn(page)
  await commitSelectedScan(page, await scanBarcode(page, barcode), '2032-04-10')
  await commitSelectedScan(page, await scanBarcode(page, barcode), '2032-08-20')

  await page.getByRole('link', { name: 'Inventory' }).click()
  const riceRow = inventoryRow(page, productName)
  await expect(riceRow.getByText('2 bag', { exact: true })).toBeVisible()
  await riceRow.getByRole('button', { name: 'View instances' }).click()
  await page.getByRole('button', { name: 'Get suggestion' }).click()
  await page.getByLabel('Manual target quantity').fill('2')
  await page.getByRole('button', { name: 'Save manual target' }).click()
  await expect(page.getByText('Target quantity set to 2.')).toBeVisible()

  // Stock out one unit to open a shortfall below the target of 2.
  await page.getByRole('link', { name: 'Scan Queue' }).click()
  await setScannerMode(page, 'stock_out')
  await commitSelectedScan(page, await scanBarcode(page, barcode))

  await page.getByRole('link', { name: 'Shopping List' }).click()

  // The Derived (auto) shortfall row proves the gap was computed from
  // inventory: target 2 minus 1 on hand = 1 bag short. The backend materializes
  // derived entries (SyncDerivedItems) so they carry a real id and expose the
  // Mark purchased / Remove actions.
  const derivedRow = page
    .getByRole('row')
    .filter({ hasText: productName })
    .filter({ has: page.getByText('Derived', { exact: true }) })
  await expect(derivedRow.getByText('1 bag', { exact: true })).toBeVisible()
  await expect(derivedRow.getByText('Derived', { exact: true })).toBeVisible()

  // Marking the shortfall purchased dismisses it (purchased gaps stay hidden
  // until the quantity changes) and must NOT change on-hand inventory.
  await derivedRow.getByRole('button', { name: `Mark ${productName} purchased` }).click()
  await expect(derivedRow).toHaveCount(0)
  await expect(page.getByText('Your shopping list is empty.')).toBeVisible()

  // A purchase records intent; on-hand inventory is unchanged (still 1 bag).
  await page.getByRole('link', { name: 'Inventory' }).click()
  await expect(inventoryRow(page, productName).getByText('1 bag', { exact: true })).toBeVisible()
})
