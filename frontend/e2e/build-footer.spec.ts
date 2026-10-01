import { expect, test, type Locator, type Page } from '@playwright/test'
import { commitSelectedScan, createKnownProduct, inventoryRow, resetToStockIn, scanBarcode } from './helpers'

interface Box {
  x: number
  y: number
  width: number
  height: number
}

const requireBox = async (locator: Locator): Promise<Box> => {
  const box = await locator.boundingBox()
  expect(box).not.toBeNull()
  return box!
}

// The stamp is in normal flow under the scrollport, so its box starts at or
// below the main region's bottom edge and stays inside the viewport.
const expectFooterClearsMain = async (page: Page, viewportHeight: number) => {
  const footer = page.getByRole('contentinfo')
  const note = footer.getByRole('note')
  await expect(note).toBeVisible()
  await expect(note).toContainText('build')

  const mainBox = await requireBox(page.getByRole('main'))
  const footerBox = await requireBox(footer)
  const noteBox = await requireBox(note)

  expect(footerBox.y).toBeGreaterThanOrEqual(mainBox.y + mainBox.height - 1)
  expect(noteBox.y).toBeGreaterThanOrEqual(mainBox.y + mainBox.height - 1)
  expect(footerBox.y + footerBox.height).toBeLessThanOrEqual(viewportHeight + 1)
  expect(noteBox.x).toBeGreaterThanOrEqual(-1)
  expect(noteBox.x + noteBox.width).toBeLessThanOrEqual(page.viewportSize()!.width + 1)
}

test('build footer stays below page content on every tab', async ({ page }) => {
  const viewport = { width: 390, height: 844 }
  await page.setViewportSize(viewport)

  for (const path of ['/', '/inventory', '/shopping']) {
    await page.goto(path)
    await expectFooterClearsMain(page, viewport.height)
  }
})

test('inventory cards scroll above the build footer', async ({ page, request }) => {
  const viewport = { width: 390, height: 700 }
  await page.setViewportSize(viewport)

  const items: Array<[string, string]> = [
    ['210000000101', 'Footer Check Beans'],
    ['210000000102', 'Footer Check Rice'],
    ['210000000103', 'Footer Check Oats'],
    ['210000000104', 'Footer Check Pasta'],
  ]
  for (const [barcode, name] of items) {
    await createKnownProduct(request, {
      barcode,
      name,
      category: 'Dry goods',
      unitOfMeasure: 'bag',
    })
  }

  await page.goto('/')
  await resetToStockIn(page)
  for (const [barcode] of items) {
    await commitSelectedScan(page, await scanBarcode(page, barcode))
  }

  await page.getByRole('link', { name: 'Inventory' }).click()
  await expectFooterClearsMain(page, viewport.height)

  const main = page.getByRole('main')
  await main.evaluate((element) => {
    element.scrollTop = element.scrollHeight
  })

  const footerBox = await requireBox(page.getByRole('contentinfo'))
  for (const [, name] of items) {
    const row = inventoryRow(page, name)
    await row.scrollIntoViewIfNeeded()
    const rowBox = await requireBox(row)
    expect(rowBox.y + rowBox.height).toBeLessThanOrEqual(footerBox.y + 1)
  }
})
