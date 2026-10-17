import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import { commitSelectedScan, createKnownProduct, inventoryRow, resetToStockIn, scanBarcode, setScannerMode } from './helpers'

const barcode = '210000231231'
const productName = 'Restock Oats'

function violationText(violations: { id: string; help: string; nodes: { html: string; target: string[] }[] }[]) {
  return violations.map((violation) => (
    `${violation.id}: ${violation.help} (${violation.nodes.map((node) => node.target.join(' ')).join('; ')})`
  )).join('\n')
}

async function expectContrast(page: Page, selector: string) {
  const results = await new AxeBuilder({ page }).include(selector).withRules(['color-contrast']).analyze()
  expect(results.violations, violationText(results.violations)).toEqual([])
}

async function setScheme(page: Page, scheme: 'light' | 'dark') {
  await page.evaluate((value) => {
    window.localStorage.setItem('mantine-color-scheme-value', value)
    document.documentElement.setAttribute('data-mantine-color-scheme', value)
  }, scheme)
  await page.reload()
  await expect.poll(() => page.evaluate(() => document.documentElement.getAttribute('data-mantine-color-scheme'))).toBe(scheme)
}

test('settings has one household default and readable restock screens', async ({ page, request }) => {
  await page.goto('/settings')
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible()
  await expect(page.getByText('Household default')).toBeVisible()
  await expect(page.getByText(/How far ahead the shopping list looks/)).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Change' })).toHaveCount(1)
  await expectContrast(page, '.restock-settings')

  await page.getByRole('button', { name: 'Change' }).click()
  await expect(page.getByRole('dialog', { name: 'Household default' })).toBeVisible()
  await expect(page.getByText('Months of supply')).toBeVisible()
  await expect(page.getByText('Fixed amount')).toBeVisible()
  await expect(page.getByText('0 means don\'t restock.')).toBeVisible()
  await page.keyboard.press('Escape')

  const restock = await request.get('/api/restock')
  expect(restock.ok()).toBe(true)
  const body = await restock.json() as { categories: { id: string; name: string }[] }
  const produce = body.categories.find((category) => category.name === 'Produce')
  if (!produce) throw new Error('Produce category missing')
  await page.goto(`/inventory/categories/${produce.id}`)
  await expect(page.getByRole('heading', { name: 'Produce' })).toBeVisible()
  await expectContrast(page, '.restock-target')
  await setScheme(page, 'dark')
  await expect(page.getByRole('heading', { name: 'Produce' })).toBeVisible()
  await expectContrast(page, '.restock-target')
})

test('don\'t restock removes the shopping row and keeps it off after reload', async ({ page, request }) => {
  test.setTimeout(90_000)
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
  const row = inventoryRow(page, productName)
  await expect(row.getByText('2 bag', { exact: true })).toBeVisible()
  const finishOpening = page.getByRole('button', { name: 'This scan is complete' })
  if (await finishOpening.isVisible()) {
    await finishOpening.click()
    await expect(page.getByRole('alert', { name: 'Opening inventory' })).toBeHidden()
  }

  await row.getByRole('button', { name: `Actions for ${productName}` }).click()
  const hint = page.locator('.bin-menu-hint', { hasText: 'Sets this item\'s target to 0' })
  await expect(hint).toBeVisible()
  await expect(page.getByRole('menuitem', { name: /Don't restock/ })).toBeVisible()
  await page.keyboard.press('Escape')

  await page.getByRole('link', { name: 'Scan Queue' }).click()
  await setScannerMode(page, 'stock_out')
  await commitSelectedScan(page, await scanBarcode(page, barcode))

  await page.getByRole('link', { name: 'Shopping List' }).click()
  await page.getByRole('button', { name: /the list$/ }).click()
  const derivedRow = page
    .getByRole('row')
    .filter({ hasText: productName })
    .filter({ has: page.getByText('Derived', { exact: true }) })
  await expect(derivedRow).toBeVisible()

  await derivedRow.getByRole('button', { name: `Actions for ${productName}` }).click()
  await page.getByRole('menuitem', { name: `Don't restock ${productName}` }).click()
  await expect(derivedRow).toHaveCount(0)

  await page.reload()
  await expect(page.getByRole('row').filter({ hasText: productName })).toHaveCount(0)
})
