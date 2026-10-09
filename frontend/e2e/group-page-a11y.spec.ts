import AxeBuilder from '@axe-core/playwright'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import { commitSelectedScan, resetToStockIn, scanBarcode } from './helpers'

async function createProduct(
  request: APIRequestContext,
  input: { name: string; category: string; unitOfMeasure: string; netAmount?: number; netUnit?: string },
) {
  const response = await request.post('/api/products', { data: input })
  if (!response.ok()) throw new Error(`create ${input.name}: ${response.status()} ${await response.text()}`)
  return response.json() as Promise<{ id: string; name: string }>
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

async function expectClean(page: Page) {
  const results = await new AxeBuilder({ page }).include('.bin-page').analyze()
  expect(results.violations, violationText(results.violations)).toEqual([])
}

async function tabTo(page: Page, name: string) {
  const target = page.getByRole('button', { name })
  await page.getByRole('link', { name: 'Product groups' }).focus()
  for (let step = 0; step < 16; step += 1) {
    if (await target.evaluate((el) => document.activeElement === el)) return target
    await page.keyboard.press('Tab')
  }
  throw new Error(`Tab never reached ${name}`)
}

test('group page meets axe in light and dark', async ({ page, request }) => {
  test.setTimeout(90_000)
  const lemon = await createProduct(request, {
    name: 'Gatorade Lemon-Lime', category: 'Drinks', unitOfMeasure: 'canister', netAmount: 18.3, netUnit: 'oz',
  })
  const powder = await createProduct(request, {
    name: 'Unsized powder', category: 'Drinks', unitOfMeasure: 'canister',
  })
  const lemonOverride = await request.post('/api/products/overrides', {
    data: { barcode: '052000338881', productId: lemon.id },
  })
  expect(lemonOverride.ok()).toBe(true)
  const powderOverride = await request.post('/api/products/overrides', {
    data: { barcode: '052000338882', productId: powder.id },
  })
  expect(powderOverride.ok()).toBe(true)

  await page.goto('/')
  await resetToStockIn(page)
  await commitSelectedScan(page, await scanBarcode(page, '052000338881'))
  await commitSelectedScan(page, await scanBarcode(page, '052000338882'))

  const targeted = await request.post('/api/groups', {
    data: { name: 'Gatorade powder', productIds: [lemon.id], target: { quantity: 48, dimension: 'mass' } },
  })
  if (!targeted.ok()) throw new Error(await targeted.text())
  const targetedGroup = await targeted.json() as { id: string }

  const empty = await request.post('/api/groups', {
    data: { name: 'Loose powder', productIds: [powder.id] },
  })
  if (!empty.ok()) throw new Error(await empty.text())
  const emptyGroup = await empty.json() as { id: string }

  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`/groups/${targetedGroup.id}`)
  await expect(page.getByRole('heading', { name: 'Gatorade powder' })).toBeVisible()
  await expect(page.getByRole('meter', { name: 'Stock on hand' })).toHaveAttribute('aria-valuenow', '18.3')
  await setScheme(page, 'light')
  await expect(page.getByRole('heading', { name: 'Gatorade powder' })).toBeVisible()
  await expectClean(page)

  const amount = await tabTo(page, 'Keep on hand amount, pinned, 48 ounces')
  const ring = await amount.evaluate((el) => {
    const style = getComputedStyle(el)
    return { outlineStyle: style.outlineStyle, outlineWidth: style.outlineWidth }
  })
  expect(ring.outlineStyle).not.toBe('none')
  expect(parseFloat(ring.outlineWidth)).toBeGreaterThanOrEqual(2)
  await page.keyboard.press('Enter')
  await expect(page.getByLabel('Ounces')).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: 'Keep on hand amount, pinned, 48 ounces' })).toBeVisible()

  const time = await tabTo(page, 'Keep on hand time, not set')
  await time.press('Space')
  await expect(page.getByLabel('Months')).toBeFocused()
  await page.getByRole('button', { name: 'Cancel' }).click()

  await setScheme(page, 'dark')
  await expect(page.getByRole('heading', { name: 'Gatorade powder' })).toBeVisible()
  await expectClean(page)

  await page.goto(`/groups/${emptyGroup.id}`)
  await expect(page.getByRole('heading', { name: 'Loose powder' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Keep on hand time, pinned, 3 months, household default' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Keep on hand amount, not set' })).toBeVisible()
  await expect(page.getByText('Set an amount or time')).toBeVisible()
  await expect(page.getByRole('meter', { name: 'Stock on hand' })).toHaveAttribute('aria-valuetext', 'No sizes listed')
  await expectClean(page)

  await setScheme(page, 'light')
  await expect(page.getByRole('heading', { name: 'Loose powder' })).toBeVisible()
  await expectClean(page)

  const unsetAmount = await tabTo(page, 'Keep on hand amount, not set')
  await unsetAmount.press('Enter')
  await expect(page.getByLabel('Ounces')).toBeVisible()
  await page.getByRole('button', { name: 'Cancel' }).click()
  const unsetTime = await tabTo(page, 'Keep on hand time, pinned, 3 months, household default')
  await unsetTime.press('Enter')
  await expect(page.getByLabel('Months')).toBeVisible()
})
