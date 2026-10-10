import AxeBuilder from '@axe-core/playwright'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

async function createProduct(
  request: APIRequestContext,
  input: { name: string; category: string; unitOfMeasure: string; barcode: string; netAmount?: number; netUnit?: string },
) {
  const response = await request.post('/api/products', {
    data: {
      name: input.name,
      category: input.category,
      unitOfMeasure: input.unitOfMeasure,
      netAmount: input.netAmount,
      netUnit: input.netUnit,
    },
  })
  if (!response.ok()) throw new Error(`create ${input.name}: ${response.status()} ${await response.text()}`)
  const product = await response.json() as { id: string }
  const override = await request.post('/api/products/overrides', {
    data: { barcode: input.barcode, productId: product.id },
  })
  expect(override.ok()).toBe(true)
  const scan = await request.post('/api/scans', {
    data: { barcode: input.barcode, direction: 'stock_in', userId: 'user-1', unitCount: 1 },
  })
  expect(scan.ok()).toBe(true)
  const entry = await scan.json() as { id: string }
  const commit = await request.post(`/api/scans/${entry.id}/commit`, { data: {} })
  expect(commit.ok()).toBe(true)
  return product
}

async function setScheme(page: Page, scheme: 'light' | 'dark') {
  await page.evaluate((value) => {
    window.localStorage.setItem('mantine-color-scheme-value', value)
    document.documentElement.setAttribute('data-mantine-color-scheme', value)
  }, scheme)
  await page.reload()
  await expect.poll(() => page.evaluate(() => document.documentElement.getAttribute('data-mantine-color-scheme'))).toBe(scheme)
}

function violationText(violations: { id: string; help: string; nodes: { target: string[] }[] }[]) {
  return violations.map((violation) => (
    `${violation.id}: ${violation.help} (${violation.nodes.map((node) => node.target.join(' ')).join('; ')})`
  )).join('\n')
}

async function expectClean(page: Page, extra: string[] = []) {
  let builder = new AxeBuilder({ page }).include('.shelf-page')
  for (const selector of extra) builder = builder.include(selector)
  const results = await builder.analyze()
  expect(results.violations, violationText(results.violations)).toEqual([])
}

test('inventory shelf meets axe collapsed, expanded, and in the add sheet', async ({ page, request }) => {
  test.setTimeout(90_000)
  expect((await request.post('/api/onboarding/complete')).ok()).toBe(true)
  const lemon = await createProduct(request, {
    name: 'Shelf lemon powder', category: 'Drinks', unitOfMeasure: 'canister', barcode: '177000000001',
    netAmount: 18, netUnit: 'oz',
  })
  await createProduct(request, {
    name: 'Shelf loose tea', category: 'Drinks', unitOfMeasure: 'tin', barcode: '177000000002',
  })
  const grouped = await request.post('/api/groups', {
    data: { name: 'Shelf drink powder', productIds: [lemon.id], target: { quantity: 48, dimension: 'mass' } },
  })
  if (!grouped.ok()) throw new Error(await grouped.text())

  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/inventory')
  await setScheme(page, 'light')
  const powder = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Shelf drink powder' }) })
  const tea = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Shelf loose tea' }) })
  await expect(powder).toBeVisible()
  await expect(tea.getByRole('img', { name: 'No photo of Shelf loose tea' })).toBeVisible()
  await expectClean(page)

  const expand = powder.getByRole('button', { name: 'Expand Shelf drink powder' })
  await expand.focus()
  await expect(expand).toHaveAttribute('aria-expanded', 'false')
  await page.keyboard.press('Enter')
  await expect(expand).toHaveAttribute('aria-expanded', 'true')
  await expect(powder.getByText('Shelf lemon powder')).toBeVisible()
  await expect(powder.getByRole('link', { name: 'Open group settings ›' })).toBeVisible()
  await expectClean(page)

  await powder.getByRole('button', { name: 'Actions for Shelf lemon powder' }).click()
  await expect(page.getByRole('menuitem', { name: 'Move to another group' })).toBeVisible()
  await expectClean(page, ['[role="menu"]'])
  await page.keyboard.press('Escape')

  await tea.getByRole('button', { name: 'Actions for Shelf loose tea' }).click()
  await page.getByRole('menuitem', { name: 'Add to a group…' }).click()
  const sheet = page.getByRole('dialog', { name: 'Add Shelf loose tea to…' })
  await expect(sheet.getByRole('button', { name: /Shelf drink powder/ })).toBeVisible()
  await expect(sheet.getByRole('button', { name: 'New group "Shelf loose tea"' })).toBeVisible()
  await expectClean(page, ['[role="dialog"]'])
  await sheet.getByRole('button', { name: 'Close' }).click()
  await expect(sheet).toBeHidden()

  await expand.focus()
  await page.keyboard.press('Enter')
  await expect(expand).toHaveAttribute('aria-expanded', 'false')

  await setScheme(page, 'dark')
  await expect(powder).toBeVisible()
  await expect(tea.getByRole('img', { name: 'No photo of Shelf loose tea' })).toBeVisible()
  await expectClean(page)
  await powder.getByRole('button', { name: 'Expand Shelf drink powder' }).click()
  await expect(powder.getByText('Shelf lemon powder')).toBeVisible()
  await expectClean(page)
  await tea.getByRole('button', { name: 'Actions for Shelf loose tea' }).click()
  await expect(page.getByRole('menuitem', { name: 'Add to a group…' })).toBeVisible()
  await expectClean(page, ['[role="menu"]'])
  await page.getByRole('menuitem', { name: 'Add to a group…' }).click()
  await expect(page.getByRole('dialog', { name: 'Add Shelf loose tea to…' })).toBeVisible()
  await expectClean(page, ['[role="dialog"]'])
  await page.getByRole('dialog', { name: 'Add Shelf loose tea to…' }).getByRole('button', { name: 'Close' }).click()

  await page.setViewportSize({ width: 1280, height: 800 })
  await expect(page.getByRole('heading', { name: 'Inventory' })).toBeVisible()
  await expect(powder.getByText('18 oz on hand')).toBeVisible()
  await expectClean(page)
})

test.afterEach(async ({ request }) => {
  const listed = await request.get('/api/groups')
  if (!listed.ok()) return
  const groups = await listed.json() as { id: string; name: string }[]
  for (const group of groups) {
    if (group.name.startsWith('Shelf ')) await request.delete(`/api/groups/${group.id}`)
  }
})
