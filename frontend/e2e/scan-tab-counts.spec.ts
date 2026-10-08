import { devices, expect, test, type APIRequestContext, type Locator, type Page } from '@playwright/test'

// The circle badge locked its width to its height, so a two-digit count
// painted as "3.." and "99+" painted as "9..". These totals force both cases
// even when earlier specs have already left scans in the shared database.
const STOCK_IN_TOTAL = 32
const STOCK_OUT_TOTAL = 100

interface ScanRow {
  id: string
  direction: 'stock_in' | 'stock_out' | null
  status: string
}

const createdIds: string[] = []
const runTag = `${Date.now()}`

const formatReviewCount = (count: number) => (count > 99 ? '99+' : String(count))

const listScans = async (request: APIRequestContext, status: string): Promise<ScanRow[]> => {
  const response = await request.get('/api/scans', { params: { userId: 'user-1', status } })
  expect(response.ok()).toBe(true)
  return response.json() as Promise<ScanRow[]>
}

const seedDirection = async (
  request: APIRequestContext,
  direction: 'stock_in' | 'stock_out',
  count: number,
) => {
  const tag = direction === 'stock_in' ? '1' : '2'
  const chunkSize = 20
  for (let start = 0; start < count; start += chunkSize) {
    const batch = []
    for (let index = start; index < Math.min(count, start + chunkSize); index += 1) {
      batch.push(request.post('/api/scans', {
        data: {
          barcode: `76${tag}${runTag}${index}`,
          userId: 'user-1',
          direction,
        },
      }))
    }
    const results = await Promise.all(batch)
    for (const result of results) {
      expect(result.ok()).toBe(true)
      const created = await result.json() as { id: string }
      createdIds.push(created.id)
    }
  }
}

const cancelCreated = async (request: APIRequestContext) => {
  const chunkSize = 20
  for (let start = 0; start < createdIds.length; start += chunkSize) {
    const batch = createdIds.slice(start, start + chunkSize).map((id) =>
      request.patch(`/api/scans/${id}`, { data: { status: 'cancelled' } }))
    const results = await Promise.all(batch)
    for (const result of results) expect(result.ok()).toBe(true)
  }
  createdIds.length = 0
}

let expectedStockIn = ''
let expectedStockOut = ''

const countIsFullyVisible = async (tab: Locator) => tab.locator('.mantine-Badge-label').evaluate((label) => {
  const badge = label.closest('.mantine-Badge-root')
  const tabEl = label.closest('[role="tab"]')
  if (!(badge instanceof HTMLElement) || !(tabEl instanceof HTMLElement)) return false
  const textClipped = label.scrollWidth > label.clientWidth + 1
    || badge.scrollWidth > badge.clientWidth + 1
  const tabBox = tabEl.getBoundingClientRect()
  const badgeBox = badge.getBoundingClientRect()
  const outsideTab = badgeBox.right > tabBox.right + 1 || badgeBox.left < tabBox.left - 1
  return !textClipped && !outsideTab
})

const expectFullCounts = async (page: Page) => {
  const stockIn = page.getByRole('tab', { name: /Stock in/ })
  const stockOut = page.getByRole('tab', { name: /Stock out/ })
  await expect(stockIn.locator('.mantine-Badge-label')).toHaveText(expectedStockIn)
  await expect(stockOut.locator('.mantine-Badge-label')).toHaveText(expectedStockOut)
  await expect.poll(() => countIsFullyVisible(stockIn)).toBe(true)
  await expect.poll(() => countIsFullyVisible(stockOut)).toBe(true)
}

test.describe('scan queue tab count badges', () => {
  test.beforeAll(async ({ request }) => {
    const open = [
      ...await listScans(request, 'pending'),
      ...await listScans(request, 'flagged'),
    ]
    const stockIn = open.filter((entry) => entry.direction === 'stock_in').length
    const stockOut = open.filter((entry) => entry.direction !== 'stock_in').length
    await seedDirection(request, 'stock_in', Math.max(0, STOCK_IN_TOTAL - stockIn))
    await seedDirection(request, 'stock_out', Math.max(0, STOCK_OUT_TOTAL - stockOut))
    expectedStockIn = formatReviewCount(Math.max(stockIn, STOCK_IN_TOTAL))
    expectedStockOut = formatReviewCount(Math.max(stockOut, STOCK_OUT_TOTAL))
  })

  test.afterAll(async ({ request }) => {
    await cancelCreated(request)
  })

  test('shows the full count on iPhone 13, Pixel 7, and desktop', async ({ page }) => {
    test.setTimeout(60_000)
    const viewports = [
      devices['iPhone 13'].viewport,
      devices['Pixel 7'].viewport,
      { width: 1280, height: 800 },
    ]
    for (const viewport of viewports) {
      await page.setViewportSize(viewport)
      await page.goto('/')
      await expectFullCounts(page)
    }
  })
})
