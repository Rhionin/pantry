import { expect, type APIRequestContext, type Locator, type Page } from '@playwright/test'

interface Product {
  id: string
  name: string
}

interface ProductInput {
  barcode: string
  name: string
  category: string
  unitOfMeasure: string
}

export const createKnownProduct = async (
  request: APIRequestContext,
  input: ProductInput,
): Promise<void> => {
  const createResponse = await request.post('/api/products', {
    data: {
      name: input.name,
      category: input.category,
      unitOfMeasure: input.unitOfMeasure,
    },
  })
  expect(createResponse.ok()).toBe(true)

  const productsResponse = await request.get('/api/products')
  expect(productsResponse.ok()).toBe(true)
  const products = await productsResponse.json() as Product[]
  const product = products.find((candidate) => candidate.name === input.name)
  if (product === undefined) {
    throw new Error(`Created product was not returned: ${input.name}`)
  }

  const overrideResponse = await request.post('/api/products/overrides', {
    data: { barcode: input.barcode, productId: product.id },
  })
  expect(overrideResponse.ok()).toBe(true)
}

export const scanBarcode = async (page: Page, barcode: string): Promise<Locator> => {
  const scannerInput = page.getByRole('textbox', { name: 'Barcode scanner input' })
  await scannerInput.fill(barcode)
  await scannerInput.press('Enter')

  const scanCard = page.getByRole('article', { name: `Scan ${barcode}` })
  await expect(scanCard).toBeVisible()
  return scanCard
}

export const commitSelectedScan = async (
  page: Page,
  scanCard: Locator,
  direction: 'stock_in' | 'stock_out',
  expirationDate?: string,
): Promise<void> => {
  await scanCard.getByRole('checkbox', { name: 'Select for batch review' }).check()
  await page.getByLabel('Direction').selectOption(direction)
  if (expirationDate !== undefined) {
    await page.getByLabel('Expiration date').fill(expirationDate)
  }
  await page.getByRole('button', { name: 'Commit 1 selected' }).click()
  await expect(scanCard).toHaveCount(0)
}

export const inventoryRow = (page: Page, productName: string): Locator =>
  page.getByRole('article').filter({ has: page.getByRole('heading', { name: productName }) })
