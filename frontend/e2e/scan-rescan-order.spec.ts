import { expect, test, type Page } from '@playwright/test'
import { createKnownProduct, resetToStockIn, scanBarcode } from './helpers'

const first = { barcode: '210000000401', name: 'Rescan Oats' }
const second = { barcode: '210000000402', name: 'Rescan Milk' }

const articleLabels = (page: Page) =>
  page.locator('.scan-page-queue article').evaluateAll((nodes) =>
    nodes.map((node) => node.getAttribute('aria-label')))

test('rescanning an open card flashes it and moves it to the top, including after reload', async ({ page, request }) => {
  test.setTimeout(90_000)
  for (const item of [first, second]) {
    await createKnownProduct(request, {
      barcode: item.barcode,
      name: item.name,
      category: 'Grocery',
      unitOfMeasure: 'bag',
    })
  }

  await page.goto('/')
  await resetToStockIn(page)
  await scanBarcode(page, first.barcode)
  await scanBarcode(page, second.barcode)
  await expect.poll(() => articleLabels(page)).toEqual([
    `Scan ${second.barcode}`,
    `Scan ${first.barcode}`,
  ])

  const rescanned = page.getByRole('article', { name: `Scan ${first.barcode}`, exact: true })
  await scanBarcode(page, first.barcode)
  await expect.poll(async () => {
    const labels = await articleLabels(page)
    const className = (await rescanned.getAttribute('class')) ?? ''
    const flashing = /scan-entry-card--(?:just-captured|changed-animated)/.test(className)
    return labels?.join(',') === `Scan ${first.barcode},Scan ${second.barcode}` && flashing
  }).toBe(true)
  await expect(rescanned.getByLabelText('Unit count')).toHaveValue('2')

  await page.reload()
  await resetToStockIn(page)
  await expect.poll(() => articleLabels(page)).toEqual([
    `Scan ${first.barcode}`,
    `Scan ${second.barcode}`,
  ])
})
