import { expect, test } from '@playwright/test'

test('diagnostics shows the page-load snapshot without private data', async ({ page, request }) => {
  const report = page.waitForResponse(
    (res) => res.url().includes('/api/telemetry/client') && res.request().method() === 'POST',
    { timeout: 15_000 },
  )
  await page.goto('/')
  await report
  await expect(page.getByRole('heading', { name: 'Scan queue' })).toBeVisible()

  await expect.poll(async () => {
    const res = await request.get('/api/telemetry')
    expect(res.ok()).toBe(true)
    const body = await res.json() as { pageLoad?: { samples?: number; note?: string } }
    return body.pageLoad?.samples ?? 0
  }).toBeGreaterThan(0)

  const snapRes = await request.get('/api/telemetry')
  const snap = await snapRes.json() as {
    pageLoad: { note: string; latest?: { ttfbMs: number; domContentLoadedMs: number } }
  }
  expect(snap.pageLoad.note.length).toBeGreaterThan(0)
  expect(JSON.stringify(snap)).not.toContain('barcode')

  await page.getByRole('link', { name: 'Diagnostics' }).click()
  await expect(page.getByRole('heading', { name: 'Diagnostics' })).toBeVisible()
  await expect(page.getByText(/^Time to first byte:/)).toBeVisible()
  await expect(page.getByText(/^DOM ready:/)).toBeVisible()
  if (snap.pageLoad.latest !== undefined) {
    await expect(page.getByText(snap.pageLoad.note)).toBeVisible()
  }
  await expect(page.locator('body')).not.toContainText('barcode')
})
