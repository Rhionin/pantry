import AxeBuilder from '@axe-core/playwright'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

async function createProduct(
  request: APIRequestContext,
  input: { name: string; category: string; unitOfMeasure: string },
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

// The suite shares one SQLite database and runs specs in file order. Earlier
// specs leave ungrouped look-alikes, and both creating a product and opening
// this page refresh suggestions, so those leftovers become inbox cards.
// Dismiss them until the inbox stays empty. Pair dismissals stick, so a later
// refresh cannot put the same cards back. Products this spec creates are
// checked after that, so an empty inbox still means these products did not match.
async function dismissOpenSuggestions(request: APIRequestContext) {
  let remaining: { id: string; title: string }[] = []
  for (let attempt = 0; attempt < 8; attempt += 1) {
    const listed = await request.get('/api/group-suggestions')
    if (!listed.ok()) throw new Error(`list suggestions: ${listed.status()} ${await listed.text()}`)
    remaining = await listed.json() as { id: string; title: string }[]
    if (remaining.length === 0) return
    for (const card of remaining) {
      const dismissed = await request.post(`/api/group-suggestions/${card.id}/dismiss`)
      if (!dismissed.ok()) throw new Error(`dismiss ${card.title}: ${dismissed.status()} ${await dismissed.text()}`)
    }
  }
  throw new Error(`suggestions still open: ${remaining.map((card) => card.title).join(', ')}`)
}

test.beforeEach(async ({ request }) => {
  await dismissOpenSuggestions(request)
})

test.afterEach(async ({ request }) => {
  await dismissOpenSuggestions(request)
})

test('suggestion rescan meets axe in light and dark', async ({ page, request }) => {
  test.setTimeout(60_000)
  await createProduct(request, { name: 'Axe apple juice', category: 'Juice', unitOfMeasure: 'bottle' })
  await createProduct(request, { name: 'Axe black beans', category: 'Canned Vegetables', unitOfMeasure: 'can' })

  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/groups/suggestions')
  await setScheme(page, 'light')
  await expect(page.getByRole('heading', { name: 'Suggestions' })).toBeVisible()
  await expect(page.getByText('Nothing to review.')).toBeVisible()
  await page.getByRole('button', { name: 'Look for more groups' }).click()
  await expect(page.getByText('No new groups found')).toBeVisible()
  await expectClean(page)

  await setScheme(page, 'dark')
  await expect(page.getByRole('heading', { name: 'Suggestions' })).toBeVisible()
  await expect(page.getByText('Nothing to review.')).toBeVisible()
  await page.getByRole('button', { name: 'Look for more groups' }).click()
  await expect(page.getByText('No new groups found')).toBeVisible()
  await expectClean(page)

  await createProduct(request, { name: "Hunt's Tomato Ketchup", category: 'Condiments', unitOfMeasure: 'bottle' })
  await createProduct(request, { name: 'Heinz Tomato Ketchup', category: 'Condiments', unitOfMeasure: 'bottle' })
  await page.goto('/groups/suggestions')
  await setScheme(page, 'light')
  await expect(page.getByLabel('Group name')).toHaveValue('Tomato ketchup')
  await expectClean(page)
  await setScheme(page, 'dark')
  await expect(page.getByLabel('Group name')).toHaveValue('Tomato ketchup')
  await expectClean(page)
})
