import AxeBuilder from '@axe-core/playwright'
import { expect, test, type APIRequestContext, type Locator, type Page, type TestInfo } from '@playwright/test'
import { writeFileSync } from 'node:fs'

const minimumContrast = 4.5

function channels(color: string): [number, number, number, number] {
  const match = color.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)(?:,\s*([\d.]+))?\)/)
  if (!match) throw new Error(`Unparsed color: ${color}`)
  return [Number(match[1]), Number(match[2]), Number(match[3]), match[4] === undefined ? 1 : Number(match[4])]
}

function contrast(foreground: string, background: string) {
  const lin = (channel: number) => {
    const value = channel / 255
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  }
  const lum = (rgb: [number, number, number]) => 0.2126 * lin(rgb[0]) + 0.7152 * lin(rgb[1]) + 0.0722 * lin(rgb[2])
  const [foregroundRed, foregroundGreen, foregroundBlue] = channels(foreground)
  const [backgroundRed, backgroundGreen, backgroundBlue] = channels(background)
  const lighter = Math.max(
    lum([foregroundRed, foregroundGreen, foregroundBlue]),
    lum([backgroundRed, backgroundGreen, backgroundBlue]),
  )
  const darker = Math.min(
    lum([foregroundRed, foregroundGreen, foregroundBlue]),
    lum([backgroundRed, backgroundGreen, backgroundBlue]),
  )
  return (lighter + 0.05) / (darker + 0.05)
}

async function setScheme(page: Page, scheme: 'light' | 'dark') {
  await page.emulateMedia({ colorScheme: scheme })
  await page.evaluate((value) => {
    window.localStorage.setItem('mantine-color-scheme-value', value)
    document.documentElement.setAttribute('data-mantine-color-scheme', value)
  }, scheme)
  await page.reload()
  await expect.poll(() => page.evaluate(() => document.documentElement.getAttribute('data-mantine-color-scheme'))).toBe(scheme)
}

async function paint(locator: Locator) {
  return locator.evaluate((el) => {
    const parse = (color: string) => {
      const match = color.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)(?:,\s*([\d.]+))?\)/)
      if (!match) return null
      return {
        r: Number(match[1]),
        g: Number(match[2]),
        b: Number(match[3]),
        a: match[4] === undefined ? 1 : Number(match[4]),
      }
    }
    const over = (
      front: { r: number; g: number; b: number; a: number },
      back: { r: number; g: number; b: number; a: number },
    ) => ({
      r: front.r * front.a + back.r * (1 - front.a),
      g: front.g * front.a + back.g * (1 - front.a),
      b: front.b * front.a + back.b * (1 - front.a),
      a: front.a + back.a * (1 - front.a),
    })
    const layers: { r: number; g: number; b: number; a: number }[] = []
    let node: Element | null = el
    while (node) {
      const background = parse(getComputedStyle(node).backgroundColor)
      if (background && background.a > 0) layers.push(background)
      if (background && background.a >= 0.99) break
      node = node.parentElement
    }
    if (layers.length === 0 || layers[layers.length - 1].a < 0.99) {
      layers.push({ r: 255, g: 255, b: 255, a: 1 })
    }
    let solid = layers[layers.length - 1]
    for (let index = layers.length - 2; index >= 0; index -= 1) solid = over(layers[index], solid)
    const ink = parse(getComputedStyle(el).color) ?? { r: 0, g: 0, b: 0, a: 1 }
    const painted = over(ink, solid)
    const rgb = (color: { r: number; g: number; b: number }) =>
      `rgb(${Math.round(color.r)}, ${Math.round(color.g)}, ${Math.round(color.b)})`
    return { foreground: rgb(painted), background: rgb(solid) }
  })
}

function violationText(violations: { id: string; help: string; nodes: { target: string[]; html: string }[] }[]) {
  return violations.map((violation) => (
    `${violation.id}: ${violation.help} (${violation.nodes.map((node) => `${node.target.join(' ')} ${node.html.slice(0, 140)}`).join('; ')})`
  )).join('\n')
}

async function expectContrast(locator: Locator) {
  const sample = await paint(locator)
  const ratio = contrast(sample.foreground, sample.background)
  expect(ratio, JSON.stringify(sample)).toBeGreaterThanOrEqual(minimumContrast)
  return { ...sample, ratio: Math.round(ratio * 100) / 100 }
}

async function shot(locator: Locator, testInfo: TestInfo, name: string) {
  await locator.screenshot({ path: testInfo.outputPath(`${name}.png`) })
}

async function seedPlainRice(request: APIRequestContext) {
  const created = await request.post('/api/products', {
    data: { name: 'Plain rice', category: 'Dry goods', unitOfMeasure: 'bag' },
  })
  expect(created.ok()).toBe(true)
  const products = await request.get('/api/products')
  const product = (await products.json() as { id: string; name: string }[]).find((row) => row.name === 'Plain rice')
  if (!product) throw new Error('Plain rice was not created')
  const override = await request.post('/api/products/overrides', {
    data: { barcode: '0000002662661', productId: product.id },
  })
  expect(override.ok()).toBe(true)
  const scan = await request.post('/api/scans', {
    data: { barcode: '0000002662661', direction: 'stock_in', userId: 'user-1', unitCount: 1 },
  })
  expect(scan.ok()).toBe(true)
  const entry = await scan.json() as { id: string }
  const commit = await request.post(`/api/scans/${entry.id}/commit`, { data: {} })
  expect(commit.ok()).toBe(true)
  const inventory = await request.get('/api/inventory')
  expect(inventory.ok()).toBe(true)
  const rows = await inventory.json() as { item: { id: string; product: { name: string } } }[]
  const rice = rows.find((row) => row.item.product.name === 'Plain rice')
  if (!rice) throw new Error('inventory is missing Plain rice')
  const added = await request.post('/api/shopping-list/items', { data: { itemId: rice.item.id, quantity: 1 } })
  expect(added.ok()).toBe(true)
}

test('dialog, Kroger panel, and remove action meet WCAG AA contrast', async ({ page, request }, testInfo) => {
  test.setTimeout(120_000)
  await seedPlainRice(request)
  const measured: { name: string; scheme: string; width: number; foreground: string; background: string; ratio: number }[] = []

  try {
    for (const width of [390, 1440]) {
      await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
      for (const scheme of ['light', 'dark'] as const) {
        await page.goto('/shopping')
        await setScheme(page, scheme)
        const kroger = page.getByText('Kroger is unconfigured.')
        const remove = page.getByRole('button', { name: 'Remove Plain rice' })
        await expect(kroger).toBeVisible()
        await expect(remove).toBeVisible()
        measured.push({ name: 'kroger-unconfigured', scheme, width, ...await expectContrast(kroger) })
        measured.push({ name: 'remove-plain-rice', scheme, width, ...await expectContrast(remove) })
        await shot(kroger, testInfo, `kroger-unconfigured-${scheme}-${width}`)
        await shot(remove, testInfo, `remove-plain-rice-${scheme}-${width}`)
        const shopping = await new AxeBuilder({ page }).withRules(['color-contrast']).analyze()
        expect(shopping.violations, violationText(shopping.violations)).toEqual([])
      }
    }

    const saved = await request.put('/api/providers/kroger/credentials', {
      data: {
        clientId: 'contrast-client',
        clientSecret: 'contrast-secret',
        redirectUri: 'http://127.0.0.1:5173/shopping',
        modality: 'PICKUP',
      },
    })
    expect(saved.ok()).toBe(true)

    for (const width of [390, 1440]) {
      await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
      for (const scheme of ['light', 'dark'] as const) {
        await page.goto('/shopping')
        await setScheme(page, scheme)
        await page.getByRole('button', { name: 'Menu' }).click()
        await page.getByRole('menuitem', { name: 'Manage Kroger connection' }).click()
        const dialog = page.getByRole('dialog', { name: 'Kroger connection' })
        const body = dialog.getByText('A client secret is saved and is not shown.')
        await expect(body).toBeVisible()
        measured.push({ name: 'dialog-body', scheme, width, ...await expectContrast(body) })
        await shot(body, testInfo, `dialog-body-${scheme}-${width}`)
        const dialogAxe = await new AxeBuilder({ page }).include('[role="dialog"]').withRules(['color-contrast']).analyze()
        expect(dialogAxe.violations, violationText(dialogAxe.violations)).toEqual([])
        await page.keyboard.press('Escape')
      }
    }
    writeFileSync(testInfo.outputPath('ratios.json'), JSON.stringify(measured, null, 2))
  } finally {
    await request.delete('/api/providers/kroger/credentials')
  }
})
