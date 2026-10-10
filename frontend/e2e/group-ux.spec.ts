import { expect, test, type APIRequestContext } from '@playwright/test'

interface Product {
  id: string
  name: string
}

async function createProduct(
  request: APIRequestContext,
  input: { name: string; category: string; unitOfMeasure: string; barcode: string },
): Promise<Product> {
  const response = await request.post('/api/products', {
    data: { name: input.name, category: input.category, unitOfMeasure: input.unitOfMeasure },
  })
  expect(response.ok()).toBe(true)
  const product = await response.json() as Product
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

test('add an inventory item to a group, and open a group just created', async ({ page, request }) => {
  test.setTimeout(60_000)
  expect((await request.post('/api/onboarding/complete')).ok()).toBe(true)
  const barley = await createProduct(request, {
    name: 'Lemon barley water', category: 'Drinks', unitOfMeasure: 'bottle', barcode: '166000000001',
  })
  const sesame = await createProduct(request, {
    name: 'Sesame crunch', category: 'Snacks', unitOfMeasure: 'bag', barcode: '166000000002',
  })
  await createProduct(request, {
    name: 'Rice crackers', category: 'Snacks', unitOfMeasure: 'box', barcode: '166000000003',
  })
  await createProduct(request, {
    name: 'Rice cakes', category: 'Snacks', unitOfMeasure: 'bag', barcode: '166000000004',
  })

  const sesameGroup = await request.post('/api/groups', {
    data: { name: 'Sesame snacks', productIds: [sesame.id] },
  })
  expect(sesameGroup.ok()).toBe(true)
  const barleyGroup = await request.post('/api/groups', {
    data: { name: 'Barley drinks', productIds: [barley.id] },
  })
  expect(barleyGroup.ok()).toBe(true)

  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/inventory')
  const crackers = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Rice crackers' }) })
  await expect(crackers).toBeVisible()
  await crackers.getByRole('button', { name: 'Actions for Rice crackers' }).click()
  await page.getByRole('menuitem', { name: 'Add to a group…' }).click()
  const addSheet = page.getByRole('dialog', { name: 'Add Rice crackers to…' })
  await expect(addSheet.getByRole('button', { name: /Barley drinks/ })).toBeVisible()
  await addSheet.getByRole('button', { name: /Sesame snacks/ }).click()
  await expect(addSheet).toBeHidden()
  await expect(page.getByRole('heading', { name: 'Rice crackers' })).toHaveCount(0)

  const sesameCard = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Sesame snacks' }) })
  await sesameCard.getByRole('button', { name: 'Expand Sesame snacks' }).click()
  await expect(sesameCard.getByText('Rice crackers')).toBeVisible()
  await expect(sesameCard.getByText('box, 1 on hand')).toBeVisible()

  await sesameCard.getByRole('button', { name: 'Actions for Rice crackers' }).click()
  await page.getByRole('menuitem', { name: 'Move to another group' }).click()
  const moveSheet = page.getByRole('dialog', { name: 'Move Rice crackers to…' })
  await expect(moveSheet.getByText('Already in Sesame snacks.')).toBeVisible()
  await expect(moveSheet.getByRole('button', { name: /Sesame snacks/ })).toHaveCount(0)
  await moveSheet.getByRole('button', { name: /Barley drinks/ }).click()
  await expect(moveSheet).toBeHidden()
  await expect(sesameCard.getByText('Rice crackers')).toHaveCount(0)

  const barleyCard = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Barley drinks' }) })
  await barleyCard.getByRole('button', { name: 'Expand Barley drinks' }).click()
  await expect(barleyCard.getByText('Rice crackers')).toBeVisible()

  const cakes = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Rice cakes' }) })
  await cakes.getByRole('button', { name: 'Actions for Rice cakes' }).click()
  await page.getByRole('menuitem', { name: 'Start a group with this' }).click()
  await expect(page.getByRole('heading', { name: 'Rice cakes' })).toBeVisible()
  await expect(page).toHaveURL(/\/groups\/[^/]+$/)
  await expect(page.getByRole('button', { name: 'Add a product' })).toBeVisible()
  await expect(page.getByText('Rice cakes')).toBeVisible()
})

test.afterEach(async ({ request }) => {
  const listed = await request.get('/api/groups')
  if (!listed.ok()) return
  const groups = await listed.json() as { id: string; name: string }[]
  for (const group of groups) {
    if (group.name === 'Sesame snacks' || group.name === 'Barley drinks' || group.name === 'Rice cakes') {
      await request.delete(`/api/groups/${group.id}`)
    }
  }
})
