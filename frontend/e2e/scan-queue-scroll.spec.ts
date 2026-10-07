import { expect, test, type Page } from '@playwright/test'
import { createKnownProduct, resetToStockIn, scanBarcode } from './helpers'

const items = [
  { barcode: '210000000101', name: 'Scroll Oats' },
  { barcode: '210000000102', name: 'Scroll Milk' },
  { barcode: '210000000103', name: 'Scroll Pasta' },
  { barcode: '210000000104', name: 'Scroll Bread' },
  { barcode: '210000000105', name: 'Scroll Rice' },
  { barcode: '210000000106', name: 'Scroll Beans' },
]

const articleLabels = (page: Page) =>
  page.locator('.scan-page-queue article').evaluateAll((nodes) =>
    nodes.map((node) => node.getAttribute('aria-label')))

test('desktop scan queue scrolls to the bottom and keeps newest-first order after reload', async ({ page, request }) => {
  test.setTimeout(90_000)
  await page.setViewportSize({ width: 1280, height: 720 })
  for (const item of items) {
    await createKnownProduct(request, {
      barcode: item.barcode,
      name: item.name,
      category: 'Grocery',
      unitOfMeasure: 'bag',
    })
  }

  await page.goto('/')
  await resetToStockIn(page)
  for (const item of items) {
    await scanBarcode(page, item.barcode)
  }

  const newestFirst = [...items].reverse().map((item) => `Scan ${item.barcode}`)
  await expect.poll(() => articleLabels(page)).toEqual(newestFirst)
  const newest = items[items.length - 1]
  await expect(page.getByRole('article', { name: `Scan ${newest.barcode}` }).getByText(`Barcode: ${newest.barcode}`)).toBeVisible()

  await page.reload()
  await expect.poll(() => articleLabels(page)).toEqual(newestFirst)

  const queue = page.locator('.scan-page-queue')
  const metrics = await queue.evaluate((element) => ({
    scrollHeight: element.scrollHeight,
    clientHeight: element.clientHeight,
  }))
  expect(metrics.scrollHeight).toBeGreaterThan(metrics.clientHeight + 1)

  const clipped = await page.locator('.scan-session-card--expanded').evaluate((element) =>
    element.scrollHeight > element.clientHeight + 2)
  expect(clipped).toBe(false)

  await queue.evaluate((element) => {
    element.scrollTop = element.scrollHeight
  })
  await expect(page.getByRole('article', { name: `Scan ${items[0].barcode}` })).toBeInViewport()

  await page.setViewportSize({ width: 390, height: 700 })
  await expect.poll(() => queue.evaluate((element) => element.scrollHeight > element.clientHeight + 1)).toBe(true)
  await queue.evaluate((element) => {
    element.scrollTop = element.scrollHeight
  })
  await expect(page.getByRole('article', { name: `Scan ${items[0].barcode}` })).toBeInViewport()
})
