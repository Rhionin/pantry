import { expect, test } from '@playwright/test'
import { commitSelectedScan, scanBarcode } from './helpers'

const barcode = '210000000002'
const productName = 'E2E Resolved Beans'

test('unknown barcode can be resolved to a new product and committed', async ({ page }) => {
  await page.goto('/')

  const scanCard = await scanBarcode(page, barcode)
  await expect(scanCard.getByText('Flagged', { exact: true })).toBeVisible()
  await scanCard.getByLabel('Product name').fill(productName)
  await scanCard.getByLabel('Category').fill('Canned goods')
  await scanCard.getByLabel('Unit of measure').fill('can')
  await scanCard.getByRole('button', { name: 'Create and use product' }).click()

  await expect(scanCard.getByText('Flagged', { exact: true })).toHaveCount(0)
  await expect(scanCard.getByRole('heading', { name: productName })).toBeVisible()
  await expect(scanCard.getByRole('checkbox', { name: 'Select for batch review' })).toBeVisible()

  await commitSelectedScan(page, scanCard, 'stock_in', '2031-03-20')
})
