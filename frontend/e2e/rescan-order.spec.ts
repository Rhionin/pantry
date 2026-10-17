import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import { createKnownProduct, resetToStockIn, scanBarcode } from './helpers'

const articleLabels = (page: Page) =>
  page.locator('.scan-page-queue article').evaluateAll((nodes) =>
    nodes.map((node) => node.getAttribute('aria-label')))

const cancelBarcode = async (request: APIRequestContext, barcode: string) => {
  const response = await request.get('/api/scans', { params: { userId: 'user-1', status: 'pending' } })
  expect(response.ok()).toBe(true)
  const entries = await response.json() as { id: string; barcode: string }[]
  for (const entry of entries) {
    if (entry.barcode !== barcode) continue
    const removed = await request.patch(`/api/scans/${entry.id}`, { data: { status: 'cancelled' } })
    expect(removed.ok()).toBe(true)
  }
}

test('rescanning an open item flashes its card and moves it to the top', async ({ page, request }) => {
  const oats = { barcode: '810000000011', name: 'Reorder Oats' }
  const milk = { barcode: '810000000012', name: 'Reorder Milk' }
  try {
    for (const item of [oats, milk]) {
      await createKnownProduct(request, {
        barcode: item.barcode,
        name: item.name,
        category: 'Grocery',
        unitOfMeasure: 'bag',
      })
    }

    await page.goto('/')
    await resetToStockIn(page)
    await scanBarcode(page, oats.barcode)
    await scanBarcode(page, milk.barcode)

    const oatsName = `Scan ${oats.barcode}`
    const milkName = `Scan ${milk.barcode}`
    await expect.poll(async () => {
      const labels = await articleLabels(page)
      return labels.indexOf(milkName) !== -1 && labels.indexOf(milkName) < labels.indexOf(oatsName)
    }).toBe(true)

    const oatsCard = page.getByRole('article', { name: oatsName, exact: true })
    await scanBarcode(page, oats.barcode)

    // The card is already on screen, so the scan helper returns before the
    // merge lands. Order and the flash class update together when it does.
    await expect.poll(async () => {
      const labels = await articleLabels(page)
      const className = (await oatsCard.getAttribute('class')) ?? ''
      return {
        atTop: labels[0] === oatsName,
        beforeMilk: labels.indexOf(oatsName) < labels.indexOf(milkName),
        flashed: className.includes('scan-entry-card--changed-animated')
          || className.includes('scan-entry-card--changed-static'),
      }
    }).toEqual({ atTop: true, beforeMilk: true, flashed: true })
    await expect(oatsCard.getByRole('textbox', { name: 'Unit count', exact: true })).toHaveValue('2')

    await page.reload()
    await expect.poll(async () => {
      const labels = await articleLabels(page)
      return labels[0] === oatsName && labels.indexOf(oatsName) < labels.indexOf(milkName)
    }).toBe(true)
    await expect(page.getByRole('article', { name: oatsName, exact: true }).getByRole('textbox', { name: 'Unit count', exact: true })).toHaveValue('2')
  } finally {
    await cancelBarcode(request, oats.barcode)
    await cancelBarcode(request, milk.barcode)
  }
})
