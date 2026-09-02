import { expect, test } from '@playwright/test'
import { commitSelectedScan, createKnownProduct, inventoryRow, scanBarcode } from './helpers'

const barcode = '210000000003'
const productName = 'E2E Oldest-First Pasta'

test('stock-out without a selection removes the earliest-expiring instance', async ({ page, request }) => {
  await createKnownProduct(request, {
    barcode,
    name: productName,
    category: 'Dry goods',
    unitOfMeasure: 'box',
  })
  await page.goto('/')

  await commitSelectedScan(page, await scanBarcode(page, barcode), 'stock_in', '2030-01-10')
  await commitSelectedScan(page, await scanBarcode(page, barcode), 'stock_in', '2030-12-20')
  await commitSelectedScan(page, await scanBarcode(page, barcode), 'stock_out')
  await page.getByRole('link', { name: 'Inventory' }).click()

  const pastaRow = inventoryRow(page, productName)
  await expect(pastaRow.getByText('1 box', { exact: true })).toBeVisible()
  await pastaRow.getByRole('button', { name: 'View instances' }).click()
  await expect(page.getByText('Expires Dec 20, 2030', { exact: true })).toBeVisible()
  await expect(page.getByText('Expires Jan 10, 2030', { exact: true })).toHaveCount(0)
})
