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

function channels(color: string): [number, number, number] {
  const match = color.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/)
  if (!match) throw new Error(`Unparsed color: ${color}`)
  return [Number(match[1]), Number(match[2]), Number(match[3])]
}

function contrast(foreground: string, background: string) {
  const lin = (channel: number) => {
    const value = channel / 255
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  }
  const lum = (rgb: [number, number, number]) => 0.2126 * lin(rgb[0]) + 0.7152 * lin(rgb[1]) + 0.0722 * lin(rgb[2])
  const hi = Math.max(lum(channels(foreground)), lum(channels(background)))
  const lo = Math.min(lum(channels(foreground)), lum(channels(background)))
  return (hi + 0.05) / (lo + 0.05)
}

async function expectListColors(page: Page, scheme: 'light' | 'dark') {
  const row = page.locator('.bin-row').first()
  await row.hover()
  const sample = await row.evaluate((el) => {
    const style = getComputedStyle(el)
    const summary = el.querySelector('.bin-row-summary')
    const well = el.querySelector('.bin-well')
    const input = document.querySelector('.bin-page .mantine-Input-input')
    return {
      color: style.color,
      background: style.backgroundColor,
      summary: summary ? getComputedStyle(summary).color : '',
      well: well ? getComputedStyle(well).backgroundColor : '',
      placeholder: input ? getComputedStyle(input, '::placeholder').color : '',
      field: input ? getComputedStyle(input).backgroundColor : '',
      border: input ? getComputedStyle(input).borderTopColor : '',
    }
  })
  expect(contrast(sample.color, sample.background), JSON.stringify(sample)).toBeGreaterThanOrEqual(4.5)
  expect(contrast(sample.summary, sample.background), JSON.stringify(sample)).toBeGreaterThanOrEqual(4.5)
  expect(contrast(sample.placeholder, sample.field), JSON.stringify(sample)).toBeGreaterThanOrEqual(4.5)
  expect(contrast(sample.border, sample.field), JSON.stringify(sample)).toBeGreaterThanOrEqual(3)
  const well = channels(sample.well)
  if (scheme === 'light') {
    expect(well[0], JSON.stringify(sample)).toBeGreaterThan(220)
  } else {
    expect(well[0], JSON.stringify(sample)).toBeGreaterThan(30)
    expect(well[0], JSON.stringify(sample)).toBeLessThan(70)
  }
  await expectClean(page)
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
    name: 'Contrast lemon', category: 'Drinks', unitOfMeasure: 'canister', netAmount: 18.3, netUnit: 'oz',
  })
  const powder = await createProduct(request, {
    name: 'Contrast unsized', category: 'Drinks', unitOfMeasure: 'canister',
  })
  const lemonOverride = await request.post('/api/products/overrides', {
    data: { barcode: '052000449901', productId: lemon.id },
  })
  expect(lemonOverride.ok()).toBe(true)
  const powderOverride = await request.post('/api/products/overrides', {
    data: { barcode: '052000449902', productId: powder.id },
  })
  expect(powderOverride.ok()).toBe(true)

  await page.goto('/')
  await resetToStockIn(page)
  await commitSelectedScan(page, await scanBarcode(page, '052000449901'))
  await commitSelectedScan(page, await scanBarcode(page, '052000449902'))

  const targeted = await request.post('/api/groups', {
    data: { name: 'Contrast powder', productIds: [lemon.id], target: { quantity: 48, dimension: 'mass' } },
  })
  if (!targeted.ok()) throw new Error(await targeted.text())
  const targetedGroup = await targeted.json() as { id: string }

  const empty = await request.post('/api/groups', {
    data: { name: 'Loose contrast powder', productIds: [powder.id] },
  })
  if (!empty.ok()) throw new Error(await empty.text())
  const emptyGroup = await empty.json() as { id: string }

  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`/groups/${targetedGroup.id}`)
  await expect(page.getByRole('heading', { name: 'Contrast powder' })).toBeVisible()
  await expect(page.getByRole('meter', { name: 'Stock on hand' })).toHaveAttribute('aria-valuenow', '18.3')
  await setScheme(page, 'light')
  await expect(page.getByRole('heading', { name: 'Contrast powder' })).toBeVisible()
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
  await expect(page.getByRole('heading', { name: 'Contrast powder' })).toBeVisible()
  await expectClean(page)

  await page.goto(`/groups/${emptyGroup.id}`)
  await expect(page.getByRole('heading', { name: 'Loose contrast powder' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Keep on hand time, pinned, 3 months, household default' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Keep on hand amount, not set' })).toBeVisible()
  await expect(page.getByText('Set an amount or time')).toBeVisible()
  await expect(page.getByRole('meter', { name: 'Stock on hand' })).toHaveAttribute('aria-valuetext', 'No sizes listed')
  await expectClean(page)

  await setScheme(page, 'light')
  await expect(page.getByRole('heading', { name: 'Loose contrast powder' })).toBeVisible()
  await expectClean(page)

  const unsetAmount = await tabTo(page, 'Keep on hand amount, not set')
  await unsetAmount.press('Enter')
  await expect(page.getByLabel('Ounces')).toBeVisible()
  await page.getByRole('button', { name: 'Cancel' }).click()
  const unsetTime = await tabTo(page, 'Keep on hand time, pinned, 3 months, household default')
  await unsetTime.press('Enter')
  await expect(page.getByLabel('Months')).toBeVisible()

  await page.goto('/groups')
  await setScheme(page, 'light')
  await expect(page.getByRole('heading', { name: 'Product groups' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'All' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByPlaceholder('Name or product')).toBeVisible()
  await expectClean(page)
  await expectListColors(page, 'light')

  await setScheme(page, 'dark')
  await expect(page.getByRole('heading', { name: 'Product groups' })).toBeVisible()
  await expectClean(page)
  await expectListColors(page, 'dark')

  await page.goto('/groups/suggestions')
  await expect(page.getByRole('heading', { name: 'Suggestions' })).toBeVisible()
  await expectClean(page)
  await setScheme(page, 'light')
  await expect(page.getByRole('heading', { name: 'Suggestions' })).toBeVisible()
  await expectClean(page)

  // Later specs share this database and match the household-default list line.
  expect((await request.delete(`/api/groups/${targetedGroup.id}`)).ok()).toBe(true)
  expect((await request.delete(`/api/groups/${emptyGroup.id}`)).ok()).toBe(true)
})
