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
  const barley = await createProduct(request, {
    name: 'Lemon barley water', category: 'Drinks', unitOfMeasure: 'bottle', barcode: '166000000001',
  })
  const sesame = await createProduct(request, {
    name: 'Sesame crunch', category: 'Snacks', unitOfMeasure: 'bag', barcode: '166000000002',
  })
  await createProduct(request, {
    name: 'Rice crackers', category: 'Snacks', unitOfMeasure: 'box', barcode: '166000000003',
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
  await expect(page.getByRole('heading', { name: 'Rice crackers' })).toBeVisible()
  await page.getByRole('button', { name: 'Add Rice crackers to a group' }).click()
  const addDialog = page.getByRole('dialog', { name: 'Add to group' })
  await expect(addDialog.getByText('Rice crackers', { exact: true })).toBeVisible()
  await expect(addDialog.getByRole('radio', { name: 'Barley drinks' })).toBeVisible()
  await addDialog.getByRole('radio', { name: 'Sesame snacks' }).check()
  await addDialog.getByRole('button', { name: 'Add to group' }).click()
  await expect(addDialog).toBeHidden()
  await expect(page.getByRole('heading', { name: 'Rice crackers' })).toHaveCount(0)

  const sesameCard = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Sesame snacks' }) })
  await sesameCard.getByRole('button', { name: 'Show products' }).click()
  await expect(sesameCard.getByText('Rice crackers · 1 on hand')).toBeVisible()

  await sesameCard.getByRole('button', { name: 'Move Rice crackers to another group' }).click()
  const moveDialog = page.getByRole('dialog', { name: 'Move to another group' })
  await expect(moveDialog.getByText('Already in Sesame snacks.')).toBeVisible()
  await expect(moveDialog.getByRole('radio', { name: 'Sesame snacks' })).toHaveCount(0)
  await moveDialog.getByRole('radio', { name: 'Barley drinks' }).check()
  await moveDialog.getByRole('button', { name: 'Move to this group' }).click()
  await expect(moveDialog).toBeHidden()
  await expect(sesameCard.getByText('Rice crackers · 1 on hand')).toHaveCount(0)

  const barleyCard = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Barley drinks' }) })
  await barleyCard.getByRole('button', { name: 'Show products' }).click()
  await expect(barleyCard.getByText('Rice crackers · 1 on hand')).toBeVisible()

  await page.goto('/groups')
  await page.getByRole('button', { name: 'New group' }).click()
  const createDialog = page.getByRole('dialog', { name: 'New group' })
  await createDialog.getByLabel('Name').fill('Rice cakes')
  await createDialog.getByRole('button', { name: 'Create group' }).click()
  await expect(page.getByRole('heading', { name: 'Rice cakes' })).toBeVisible()
  await expect(page).toHaveURL(/\/groups\/[^/]+$/)
  await expect(page.getByText('This group has no products yet.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Add a product' })).toBeVisible()
})
