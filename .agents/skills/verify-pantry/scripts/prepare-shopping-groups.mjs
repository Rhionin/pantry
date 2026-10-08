// Seeds the products a Groups inbox and a group shopping plan need.
// The server must be restarted afterwards so the one-time suggestion seed
// sees these products. External lookup is already off.
const API = process.env.PANTRY_API_URL ?? 'http://127.0.0.1:18080';

const products = [
  { barcode: '910000000081', name: 'Great Value Cut Green Beans', unit: 'can' },
  { barcode: '910000000082', name: 'Kroger Cut Green Beans', unit: 'can' },
  { barcode: '910000000083', name: 'Del Monte Cut Green Beans', unit: 'can' },
  { barcode: '910000000084', name: 'Great Value Whole Kernel Corn', unit: 'can' },
  { barcode: '910000000085', name: 'Kroger Whole Kernel Corn', unit: 'can' },
];

async function send(path, { method = 'GET', body } = {}) {
  const res = await fetch(`${API}${path}`, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  if (!res.ok) throw new Error(`${method} ${path} -> ${res.status} ${text}`);
  return text ? JSON.parse(text) : null;
}

for (const spec of products) {
  const created = await send('/api/products', {
    method: 'POST',
    body: {
      name: spec.name,
      category: 'Canned',
      unitOfMeasure: spec.unit,
      netAmount: 14.5,
      netUnit: 'oz',
    },
  });
  await send('/api/products/overrides', {
    method: 'POST',
    body: { barcode: spec.barcode, productId: created.id },
  });
  const scan = await send('/api/scans', {
    method: 'POST',
    body: { barcode: spec.barcode, direction: 'stock_in', userId: 'user-1', unitCount: 1 },
  });
  await send(`/api/scans/${scan.id}/commit`, { method: 'POST', body: {} });
  await send(`/api/products/${created.id}/supply-override`, {
    method: 'PUT',
    body: { quantity: 4 },
  });
  spec.productId = created.id;
}

await send('/api/onboarding/complete', { method: 'POST', body: {} });
console.log(`prepared ${products.length} products`);
