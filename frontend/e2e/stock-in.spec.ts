import { expect, test } from '@playwright/test'
import { commitSelectedScan, createKnownProduct, inventoryRow, resetToStockIn, scanBarcode } from './helpers'

const barcode = '210000000001'
const productName = 'E2E Stock-In Milk'

test('HID scan can be reviewed and committed as stock-in', async ({ page, request }) => {
  await createKnownProduct(request, {
    barcode,
    name: productName,
    category: 'Dairy',
    unitOfMeasure: 'carton',
  })
  await page.goto('/')
  await resetToStockIn(page)
  await expect(page.getByText('Mode: stock_in')).toBeVisible()

  const scanCard = await scanBarcode(page, barcode)
  await expect(scanCard.getByRole('heading', { name: productName })).toBeVisible()
  await expect(scanCard.getByText(`Barcode: ${barcode}`)).toBeVisible()
  await expect(scanCard.getByText(`Barcode: ${barcode}`)).toHaveClass(/copyable-barcode/)
  await expect(scanCard.getByText(/^Scanned:/)).toBeVisible()

  await commitSelectedScan(page, scanCard, '2030-06-15')
  await page.getByRole('link', { name: 'Inventory' }).click()

  const milkRow = inventoryRow(page, productName)
  await expect(milkRow).toBeVisible()
  await expect(milkRow.getByText('1 carton', { exact: true })).toBeVisible()
})
