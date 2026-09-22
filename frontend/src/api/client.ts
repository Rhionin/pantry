// Typed fetch wrappers for the Pantry Management backend API.
// Endpoint list and request/response shapes verified against
// internal/server/server.go and the individual handler files.
import type {
  BatchCommitResponse,
  InventoryItem,
  ItemInstance,
  ItemInstanceWithStatus,
  LookupResult,
  Product,
  ScanDirection,
  ScanEntry,
  ScannerConfig,
  ScanStatus,
  ShoppingListEntry,
  TargetQuantitySuggestion,
} from '../types';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '';

// Thrown for any non-2xx response. message is read from the backend's
// {"error": "..."} body shape (see internal/server/handler_wrapper.go writeError).
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers);
  if (options?.body !== undefined) {
    headers.set('Content-Type', 'application/json');
  }

  const res = await fetch(`${BASE_URL}${path}`, { ...options, headers });

  if (!res.ok) {
    let message = res.statusText;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) {
        message = body.error;
      }
    } catch {
      // Response body was not JSON; fall back to statusText.
    }
    throw new ApiError(res.status, message);
  }

  // DELETE endpoints return `{}` with a 200 status (see handler_wrapper.go),
  // not an empty 204 body, but callers of those endpoints don't need the value.
  const text = await res.text();
  return text ? (JSON.parse(text) as T) : (undefined as T);
}

function toQueryString(params: Record<string, string | undefined>): string {
  const entries = Object.entries(params).filter(
    (entry): entry is [string, string] => entry[1] !== undefined,
  );
  if (entries.length === 0) return '';
  return `?${new URLSearchParams(entries).toString()}`;
}

// --- Products ---

export function lookupProduct(barcode: string): Promise<LookupResult> {
  return apiFetch(`/api/products/lookup${toQueryString({ barcode })}`);
}

export function listProducts(): Promise<Product[]> {
  return apiFetch('/api/products');
}

interface CreateProductInput {
  name: string;
  category: string;
  unitOfMeasure: string;
}

export function createProduct(input: CreateProductInput): Promise<Product> {
  return apiFetch('/api/products', { method: 'POST', body: JSON.stringify(input) });
}

export function updateProduct(id: string, input: CreateProductInput): Promise<Product> {
  return apiFetch(`/api/products/${id}`, { method: 'PUT', body: JSON.stringify(input) });
}

interface CreateProductOverrideInput {
  barcode: string;
  productId: string;
}

interface CreateProductOverrideResponse {
  barcode: string;
  productId: string;
  source: string;
}

export function createProductOverride(
  input: CreateProductOverrideInput,
): Promise<CreateProductOverrideResponse> {
  return apiFetch('/api/products/overrides', { method: 'POST', body: JSON.stringify(input) });
}

// --- Scan queue ---

interface CreateScanEntryInput {
  barcode: string;
  direction?: ScanDirection;
  unitCount?: number;
  expiresAt?: string;
  userId: string;
}

export function createScanEntry(input: CreateScanEntryInput): Promise<ScanEntry> {
  return apiFetch('/api/scans', { method: 'POST', body: JSON.stringify(input) });
}

export function listScanEntries(userId: string, status?: ScanStatus): Promise<ScanEntry[]> {
  return apiFetch(`/api/scans${toQueryString({ userId, status })}`);
}

export function getScanHistory(userId: string): Promise<ScanEntry[]> {
  return apiFetch(`/api/scans/history${toQueryString({ userId })}`);
}

interface UpdateScanEntryInput {
  direction?: ScanDirection;
  unitCount?: number;
  expiresAt?: string;
  productId?: string;
  status?: ScanStatus;
}

export function updateScanEntry(id: string, input: UpdateScanEntryInput): Promise<ScanEntry> {
  return apiFetch(`/api/scans/${id}`, { method: 'PATCH', body: JSON.stringify(input) });
}

export function commitScanEntry(id: string, instanceId?: string): Promise<ScanEntry> {
  return apiFetch(`/api/scans/${id}/commit`, {
    method: 'POST',
    body: JSON.stringify(instanceId ? { instanceId } : {}),
  });
}

interface BatchCommitInput {
  scanEntryIds: string[];
  direction?: ScanDirection;
  unitCount?: number;
  expiresAt?: string;
  commit: boolean;
}

export function batchCommitScanEntries(input: BatchCommitInput): Promise<BatchCommitResponse> {
  return apiFetch('/api/scans/batch-commit', { method: 'POST', body: JSON.stringify(input) });
}

// --- Scanner mode and config ---

interface SetScannerModeResponse {
  mode: ScanDirection;
}

// Switches the shared scanner mode and triggers a 'scanner_mode' SSE event so
// every connected browser converges on the same direction (matches the
// headless listener behavior). See internal/server/handler_scanner.go.
export function setScannerMode(mode: ScanDirection): Promise<SetScannerModeResponse> {
  return apiFetch('/api/scanner/mode', { method: 'POST', body: JSON.stringify({ mode }) });
}

// Returns the reserved control-barcode strings the backend classifies against
// (defaults 'STOCK_IN'/'STOCK_OUT'), so the browser matches the exact values.
export function getScannerConfig(): Promise<ScannerConfig> {
  return apiFetch('/api/scanner/config');
}

// --- Inventory ---

export function getInventoryList(query?: string): Promise<InventoryItem[]> {
  return apiFetch(`/api/inventory${toQueryString({ q: query })}`);
}

export function listItemInstances(itemId: string): Promise<ItemInstanceWithStatus[]> {
  return apiFetch(`/api/inventory/${itemId}/instances`);
}

export function addItemInstance(itemId: string, expiresAt?: string): Promise<ItemInstance> {
  return apiFetch(`/api/inventory/${itemId}/instances`, {
    method: 'POST',
    body: JSON.stringify(expiresAt ? { expiresAt } : {}),
  });
}

export function removeItemInstance(instanceId: string): Promise<void> {
  return apiFetch(`/api/inventory/instances/${instanceId}`, { method: 'DELETE' });
}

// --- Suggestions and target quantity ---

export function getSuggestion(itemId: string): Promise<TargetQuantitySuggestion> {
  return apiFetch(`/api/suggestions/${itemId}`);
}

interface SetTargetQuantityResponse {
  itemId: string;
  targetQuantity: number;
}

export function setTargetQuantity(
  itemId: string,
  targetQuantity: number,
): Promise<SetTargetQuantityResponse> {
  return apiFetch(`/api/items/${itemId}/target-quantity`, {
    method: 'POST',
    body: JSON.stringify({ targetQuantity }),
  });
}

// --- Shopping list ---

export function getShoppingList(): Promise<ShoppingListEntry[]> {
  return apiFetch('/api/shopping-list');
}

export function addShoppingListItem(itemId: string, quantity: number): Promise<ShoppingListEntry> {
  return apiFetch('/api/shopping-list/items', {
    method: 'POST',
    body: JSON.stringify({ itemId, quantity }),
  });
}

export function removeShoppingListItem(id: string): Promise<void> {
  return apiFetch(`/api/shopping-list/items/${id}`, { method: 'DELETE' });
}

export function markShoppingListItemPurchased(id: string): Promise<ShoppingListEntry> {
  return apiFetch(`/api/shopping-list/items/${id}`, {
    method: 'PATCH',
    body: JSON.stringify({ purchased: true }),
  });
}

export interface ExportShoppingListResponse {
  exported: number;
  failedItems?: string[];
}

export function exportShoppingList(): Promise<ExportShoppingListResponse> {
  return apiFetch('/api/shopping-list/export', { method: 'POST' });
}
