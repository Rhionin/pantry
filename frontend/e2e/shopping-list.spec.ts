import { expect, test } from '@playwright/test'
import { commitSelectedScan, createKnownProduct, inventoryRow, scanBarcode } from './helpers'

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
  await commitSelectedScan(page, await scanBarcode(page, barcode), 'stock_in', '2032-04-10')
  await commitSelectedScan(page, await scanBarcode(page, barcode), 'stock_in', '2032-08-20')

  await page.getByRole('link', { name: 'Inventory' }).click()
  const riceRow = inventoryRow(page, productName)
  await expect(riceRow.getByText('2 bag', { exact: true })).toBeVisible()
  await riceRow.getByRole('button', { name: 'View instances' }).click()
  await page.getByRole('button', { name: 'Get suggestion' }).click()
  await page.getByLabel('Manual target quantity').fill('2')
  await page.getByRole('button', { name: 'Save manual target' }).click()
  await expect(page.getByText('Target quantity set to 2.')).toBeVisible()

  await page.getByRole('link', { name: 'Scan Queue' }).click()
  await commitSelectedScan(page, await scanBarcode(page, barcode), 'stock_out')
  await page.getByRole('link', { name: 'Shopping List' }).click()

  const shoppingRow = page.getByRole('row').filter({ hasText: productName })
  await expect(shoppingRow.getByText('1 bag', { exact: true })).toBeVisible()
  await expect(shoppingRow.getByText('Derived', { exact: true })).toBeVisible()
  await shoppingRow.getByRole('button', { name: `Mark ${productName} purchased` }).click()
  await expect(shoppingRow).toHaveCount(0)
  await expect(page.getByText('Your shopping list is empty.')).toBeVisible()

  await page.getByRole('link', { name: 'Inventory' }).click()
  await expect(inventoryRow(page, productName).getByText('1 bag', { exact: true })).toBeVisible()
})
