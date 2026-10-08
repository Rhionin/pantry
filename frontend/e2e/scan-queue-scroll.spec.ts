import { devices, expect, test, type Page } from '@playwright/test'
import { createKnownProduct, resetToStockIn, scanBarcode, setScannerMode } from './helpers'

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
  await expect(page.getByRole('article', { name: `Scan ${newest.barcode}`, exact: true }).getByText(`Barcode: ${newest.barcode}`)).toBeVisible()

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
  await expect(page.getByRole('article', { name: `Scan ${items[0].barcode}`, exact: true })).toBeInViewport()

  await page.setViewportSize({ width: 390, height: 700 })
  await expect.poll(() => queue.evaluate((element) => element.scrollHeight > element.clientHeight + 1)).toBe(true)
  await queue.evaluate((element) => {
    element.scrollTop = element.scrollHeight
  })
  await expect(page.getByRole('article', { name: `Scan ${items[0].barcode}`, exact: true })).toBeInViewport()
})

const phoneItems = Array.from({ length: 10 }, (_, index) => ({
  barcode: `2100000002${String(index).padStart(2, '0')}`,
  name: `Touch Item ${index}`,
}))

const stockOutItems = Array.from({ length: 8 }, (_, index) => ({
  barcode: `2100000003${String(index).padStart(2, '0')}`,
  name: `Touch Out ${index}`,
}))

const iphone = devices['iPhone 13']

// A finger drag, not a scrollTop assignment. Chromium turns these CDP touch
// points into a real pan; one jump does not.
const touchSwipe = async (page: Page, fromX: number, fromY: number, toY: number) => {
  const client = await page.context().newCDPSession(page)
  const steps = 14
  const x = Math.round(fromX)
  const startY = Math.round(fromY)
  await client.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x, y: startY }],
  })
  for (let step = 1; step <= steps; step += 1) {
    const y = Math.round(startY + ((toY - startY) * step) / steps)
    await client.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x, y }],
    })
    await new Promise((resolve) => setTimeout(resolve, 16))
  }
  await client.send('Input.dispatchTouchEvent', {
    type: 'touchEnd',
    touchPoints: [],
  })
  await client.detach()
}

const swipeQueue = async (page: Page) => {
  const queue = page.locator('.scan-page-queue')
  const box = await queue.boundingBox()
  if (box === null) throw new Error('scan queue has no box')
  const before = await queue.evaluate((element) => element.scrollTop)
  const articles = page.locator('.scan-page-queue article')
  const viewportHeight = page.viewportSize()?.height ?? 0
  let markerIndex = -1
  for (let index = 0; index < await articles.count(); index += 1) {
    const articleBox = await articles.nth(index).boundingBox()
    if (articleBox !== null && articleBox.y >= viewportHeight - 8) {
      markerIndex = index
      break
    }
  }
  expect(markerIndex).toBeGreaterThan(-1)
  const marker = articles.nth(markerIndex)
  const beforeBox = await marker.boundingBox()
  expect(beforeBox).not.toBeNull()

  await touchSwipe(
    page,
    box.x + box.width / 2,
    box.y + Math.min(box.height * 0.72, 320),
    box.y + 24,
  )

  await expect.poll(() => queue.evaluate((element) => element.scrollTop)).toBeGreaterThan(before + 40)
  await expect(marker).toBeInViewport()
  const afterBox = await marker.boundingBox()
  expect(afterBox).not.toBeNull()
  expect(afterBox!.y).toBeLessThan(beforeBox!.y - 30)
}

test.describe('phone touch scroll', () => {
  test.use({
    viewport: iphone.viewport,
    hasTouch: true,
    isMobile: true,
    userAgent: iphone.userAgent,
    deviceScaleFactor: iphone.deviceScaleFactor,
  })

  test('a finger swipe scrolls the scan queue and brings a lower row into view', async ({ page, request }) => {
    test.setTimeout(120_000)
    for (const item of [...phoneItems, ...stockOutItems]) {
      await createKnownProduct(request, {
        barcode: item.barcode,
        name: item.name,
        category: 'Grocery',
        unitOfMeasure: 'bag',
      })
    }

    await page.goto('/')
    await resetToStockIn(page)
    for (const item of phoneItems) {
      await scanBarcode(page, item.barcode)
    }

    const queue = page.locator('.scan-page-queue')
    await expect.poll(() => queue.evaluate((element) => element.scrollHeight > element.clientHeight + 1)).toBe(true)
    await expect(page.locator('.scan-session-card').first()).toHaveClass(/scan-session-card--expanded/)
    await expect.poll(() => page.locator('.scan-session-card').first().evaluate((element) => getComputedStyle(element).overflowY)).toBe('clip')
    await expect.poll(() => page.locator('.mantine-AppShell-main').evaluate((element) => getComputedStyle(element).overflowY)).toBe('hidden')
    await expect(page.locator('.scan-page-queue article').first()).toHaveAttribute(
      'aria-label',
      `Scan ${phoneItems[phoneItems.length - 1].barcode}`,
    )

    await swipeQueue(page)

    await page.locator('.scan-session-toggle').first().click()
    await expect(page.locator('.scan-session-card--expanded')).toHaveCount(0)
    await page.locator('.scan-session-toggle').first().click()
    await expect(page.locator('.scan-session-card--expanded')).toHaveCount(1)
    await queue.evaluate((element) => {
      element.scrollTop = 0
    })
    await swipeQueue(page)

    await setScannerMode(page, 'stock_out')
    for (const item of stockOutItems) {
      await scanBarcode(page, item.barcode)
    }
    await expect(page.getByRole('tab', { name: /Stock out/ })).toHaveAttribute('aria-selected', 'true')
    await expect.poll(() => queue.evaluate((element) => element.scrollHeight > element.clientHeight + 1)).toBe(true)
    await swipeQueue(page)
  })
})
