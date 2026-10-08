// Typed fetch wrappers for the Pantry Management backend API.
// Endpoint list and request/response shapes verified against
// internal/server/server.go and the individual handler files.
import { reportApiResult } from '../telemetry/client';
import type {
  BatchCommitResponse,
  BuildInfo,
  InventoryItem,
  ItemInstance,
  ItemInstanceWithStatus,
  LookupResult,
  Product,
  ProductDetail,
  ProductWriteInput,
  ProductWriteResult,
  ContributionSettings,
  ScanDirection,
  ScanEntry,
  ScannerConfig,
  ScanStatus,
  ProvisionReport,
  ProviderInfo,
  ShoppingListEntry,
  SupplyOverride,
  SupplySettings,
  ProductGroup,
  GroupSuggestion,
  GroupTarget,
  GroupPreview,
  TargetConflictMember,
} from '../types';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '';

// Thrown for any non-2xx response. message is read from the backend's
// {"error": "..."} body shape (see internal/server/handler_wrapper.go writeError).
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly members: TargetConflictMember[];

  constructor(status: number, message: string, code = '', members: TargetConflictMember[] = []) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.members = members;
  }
}

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers);
  if (options?.body !== undefined) {
    headers.set('Content-Type', 'application/json');
  }

  const started = performance.now();
  const route = path.split('?')[0] ?? path;
  let res: Response;
  try {
    res = await fetch(`${BASE_URL}${path}`, { ...options, headers });
  } catch (err) {
    reportApiResult(route, 0, performance.now() - started, started);
    throw err;
  }

  if (!res.ok) {
    let message = res.statusText;
    try {
      const body = (await res.json()) as { error?: string; code?: string; members?: TargetConflictMember[] };
      if (body.error) {
        message = body.error;
      }
      reportApiResult(route, res.status, performance.now() - started, started);
      throw new ApiError(res.status, message, body.code ?? '', body.members ?? []);
    } catch (err) {
      if (err instanceof ApiError) throw err;
      // Response body was not JSON; fall back to statusText.
    }
    reportApiResult(route, res.status, performance.now() - started, started);
    throw new ApiError(res.status, message);
  }

  // DELETE endpoints return `{}` with a 200 status (see handler_wrapper.go),
  // not an empty 204 body, but callers of those endpoints don't need the value.
  const text = await res.text();
  reportApiResult(route, res.status, performance.now() - started, started);
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

export function getProduct(id: string): Promise<ProductDetail> {
  return apiFetch(`/api/products/${id}`);
}

export function createProduct(input: ProductWriteInput): Promise<ProductWriteResult> {
  return apiFetch('/api/products', { method: 'POST', body: JSON.stringify(input) });
}

export function updateProduct(id: string, input: ProductWriteInput): Promise<ProductWriteResult> {
  return apiFetch(`/api/products/${id}`, { method: 'PUT', body: JSON.stringify(input) });
}

export function getContributionSettings(): Promise<ContributionSettings> {
  return apiFetch('/api/settings/contribution');
}

export function updateContributionSettings(enabled: boolean): Promise<ContributionSettings> {
  return apiFetch('/api/settings/contribution', {
    method: 'PUT',
    body: JSON.stringify({ enabled }),
  });
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

export function stockOutItem(itemId: string): Promise<void> {
  return apiFetch(`/api/inventory/${itemId}/stock-out`, { method: 'POST', body: '{}' });
}

// Exact phrase POST /api/inventory/wipe requires. Kept in sync with
// supply.WipePhrase.
export const WIPE_INVENTORY_CONFIRMATION = 'WIPE INVENTORY';

export function wipeInventory(confirmation: string): Promise<void> {
  return apiFetch('/api/inventory/wipe', {
    method: 'POST',
    body: JSON.stringify({ confirmation }),
  });
}

export function getSupplySettings(): Promise<SupplySettings> {
  return apiFetch('/api/settings/supply');
}

export function setSupplyMonths(months: number): Promise<SupplySettings> {
  return apiFetch('/api/settings/supply', {
    method: 'PUT',
    body: JSON.stringify({ months }),
  });
}

export function completeOnboarding(): Promise<{ startedAt: string }> {
  return apiFetch('/api/onboarding/complete', { method: 'POST' });
}

export function getSupplyOverride(productId: string): Promise<SupplyOverride> {
  return apiFetch(`/api/products/${productId}/supply-override`);
}

export function setSupplyOverride(productId: string, override: SupplyOverride): Promise<SupplyOverride> {
  return apiFetch(`/api/products/${productId}/supply-override`, {
    method: 'PUT',
    body: JSON.stringify(override),
  });
}

// --- Shopping list ---

export function getShoppingList(provider?: string): Promise<ShoppingListEntry[]> {
  return apiFetch(`/api/shopping-list${toQueryString({ provider })}`);
}

export function fillShoppingCart(): Promise<ShoppingListEntry[]> {
  return apiFetch('/api/shopping-list/fill', { method: 'POST' });
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

export function swapShoppingLine(id: string, itemId: string): Promise<ShoppingListEntry> {
  return apiFetch(`/api/shopping-list/items/${id}/swap`, {
    method: 'POST',
    body: JSON.stringify({ itemId }),
  });
}

export function markShoppingListItemPurchased(id: string): Promise<ShoppingListEntry> {
  return apiFetch(`/api/shopping-list/items/${id}`, {
    method: 'PATCH',
    body: JSON.stringify({ purchased: true }),
  });
}

export function exportShoppingList(provider?: string): Promise<ProvisionReport> {
  const body: { provider?: string } = {};
  if (provider) body.provider = provider;
  return apiFetch('/api/shopping-list/export', {
    method: 'POST',
    body: Object.keys(body).length > 0 ? JSON.stringify(body) : undefined,
  });
}

export function listProviders(): Promise<ProviderInfo[]> {
  return apiFetch('/api/providers');
}

export function saveProviderCredentials(
  providerId: string,
  credentials: { clientId: string; clientSecret: string; redirectUri: string; modality: string },
): Promise<void> {
  return apiFetch(`/api/providers/${providerId}/credentials`, {
    method: 'PUT',
    body: JSON.stringify(credentials),
  });
}

export function clearProviderCredentials(providerId: string): Promise<void> {
  return apiFetch(`/api/providers/${providerId}/credentials`, { method: 'DELETE' });
}

export function authorizeProvider(providerId: string): Promise<{ authorizationUrl: string }> {
  return apiFetch(`/api/providers/${providerId}/authorize`);
}

export function disconnectProvider(providerId: string): Promise<void> {
  return apiFetch(`/api/providers/${providerId}/connection`, { method: 'DELETE' });
}

export function resetProviderLedger(providerId: string): Promise<void> {
  return apiFetch(`/api/providers/${providerId}/ledger/reset`, { method: 'POST' });
}

export function setShoppingListAdjustment(entryId: string, provider: string, adjustment: number): Promise<{ adjustment: number }> {
  return apiFetch(`/api/shopping-list/items/${entryId}/adjustment${toQueryString({ provider })}`, {
    method: 'PUT',
    body: JSON.stringify({ adjustment }),
  });
}

export function clearShoppingListAdjustment(entryId: string, provider: string): Promise<void> {
  return apiFetch(`/api/shopping-list/items/${entryId}/adjustment${toQueryString({ provider })}`, {
    method: 'DELETE',
  });
}

export function resolveUnknownProvision(entryId: string, provider: string, reachedProvider: boolean): Promise<{ entryId: string }> {
  return apiFetch(`/api/shopping-list/items/${entryId}/unknown-resolution`, {
    method: 'POST',
    body: JSON.stringify({ provider, reachedProvider }),
  });
}

export function saveItemDeal(itemId: string, priceCents: number, label: string): Promise<void> {
  return apiFetch('/api/shopping-list/deals', {
    method: 'PUT',
    body: JSON.stringify({ itemId, priceCents, label }),
  });
}

export function clearItemDeal(itemId: string): Promise<void> {
  return apiFetch(`/api/shopping-list/deals/${itemId}`, { method: 'DELETE' });
}

// --- Build identity ---

export function listGroups(): Promise<ProductGroup[]> {
  return apiFetch('/api/groups');
}

export function getGroup(id: string): Promise<ProductGroup> {
  return apiFetch(`/api/groups/${id}`);
}

export function createGroup(name: string, productIds: string[] = [], target?: GroupTarget): Promise<ProductGroup> {
  return apiFetch('/api/groups', {
    method: 'POST',
    body: JSON.stringify({ name, productIds, ...(target ? { target } : {}) }),
  });
}

export function renameGroup(id: string, name: string): Promise<ProductGroup> {
  return apiFetch(`/api/groups/${id}`, {
    method: 'PATCH',
    body: JSON.stringify({ name }),
  });
}

export function deleteGroup(id: string): Promise<void> {
  return apiFetch(`/api/groups/${id}`, { method: 'DELETE' });
}

export function addGroupMembers(id: string, productIds: string[], fromGroupId = '', target?: GroupTarget): Promise<ProductGroup> {
  return apiFetch(`/api/groups/${id}/members`, {
    method: 'POST',
    body: JSON.stringify({
      productIds,
      ...(fromGroupId ? { fromGroupId } : {}),
      ...(target ? { target } : {}),
    }),
  });
}

export function removeGroupMember(id: string, productId: string): Promise<{ deleted: boolean; group?: ProductGroup }> {
  return apiFetch(`/api/groups/${id}/members/${productId}`, { method: 'DELETE' });
}

export function setGroupRule(id: string, rule: string, pinnedProductId = '', confirm = true): Promise<ProductGroup> {
  return apiFetch(`/api/groups/${id}/rule`, {
    method: 'PUT',
    body: JSON.stringify({ rule, pinnedProductId, confirm }),
  });
}

export function previewGroup(input: { groupId: string; rule: string; pinnedProductId?: string }): Promise<GroupPreview> {
  return apiFetch('/api/groups/preview', {
    method: 'POST',
    body: JSON.stringify({
      groupId: input.groupId,
      rule: input.rule,
      pinnedProductId: input.pinnedProductId ?? '',
    }),
  });
}

export function getDefaultGroupRule(): Promise<{ rule: string }> {
  return apiFetch('/api/settings/group-rule');
}

export function setDefaultGroupRule(rule: string): Promise<{ rule: string }> {
  return apiFetch('/api/settings/group-rule', {
    method: 'PUT',
    body: JSON.stringify({ rule }),
  });
}

export function setGroupTarget(id: string, target: GroupTarget): Promise<ProductGroup> {
  return apiFetch(`/api/groups/${id}/target`, {
    method: 'PUT',
    body: JSON.stringify(target),
  });
}

export function noteGroupFromScan(productId: string): Promise<void> {
  return apiFetch('/api/group-suggestions/from-scan', {
    method: 'POST',
    body: JSON.stringify({ productId }),
  });
}

export function listSuggestions(): Promise<GroupSuggestion[]> {
  return apiFetch('/api/group-suggestions');
}

export function acceptSuggestion(id: string, productIds: string[], name?: string, target?: GroupTarget): Promise<ProductGroup> {
  return apiFetch(`/api/group-suggestions/${id}/accept`, {
    method: 'POST',
    body: JSON.stringify({
      productIds,
      ...(name ? { name } : {}),
      ...(target ? { target } : {}),
    }),
  });
}

export function dismissSuggestion(id: string): Promise<void> {
  return apiFetch(`/api/group-suggestions/${id}/dismiss`, { method: 'POST' });
}

export function skipSuggestion(id: string): Promise<void> {
  return apiFetch(`/api/group-suggestions/${id}/skip`, { method: 'POST' });
}

// --- Build identity ---

export function getBuildInfo(): Promise<BuildInfo> {
  return apiFetch('/api/build');
}
