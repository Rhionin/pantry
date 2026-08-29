// Shared TypeScript types mirroring the backend's camelCase JSON API shapes.
// Field-by-field verified against the Go domain structs and handler response
// types in internal/product, internal/scan, internal/inventory,
// internal/suggestion, internal/shopping, and internal/server.

export interface ProductSummary {
  id: string;
  name: string;
  category: string;
  unitOfMeasure: string;
}

export interface Product extends ProductSummary {
  createdAt: string; // ISO 8601
}

export type ScanDirection = 'stock_in' | 'stock_out';

export type ScanStatus = 'pending' | 'flagged' | 'committed' | 'cancelled';

export interface ScanEntry {
  id: string;
  userId: string;
  barcode: string;
  scannedAt: string; // ISO 8601
  direction: ScanDirection | null;
  unitCount: number;
  expiresAt: string | null;
  status: ScanStatus;
  productId: string | null;
  product: ProductSummary | null;
  committedAt: string | null;
  createdAt: string;
}

export interface Item {
  id: string;
  userId: string;
  productId: string;
  product: ProductSummary;
  targetQuantity: number | null;
  createdAt: string;
}

export interface ItemInstance {
  id: string;
  itemId: string;
  stockInAt: string; // ISO 8601
  expiresAt: string | null;
  removedAt: string | null;
  removalReason: string | null;
  createdAt: string;
}

export type ExpiryStatus = 'ok' | 'near_expiry' | 'expired';

export type ItemInstanceWithStatus = ItemInstance & { expiryStatus: ExpiryStatus };

export interface InventoryItem {
  item: Item;
  instanceCount: number;
  nearExpiryCount: number;
  expiredCount: number;
  needsAttention: boolean;
}

export interface TargetQuantitySuggestion {
  itemId: string;
  suggestedQuantity: number;
  reasoning: string;
  consumptionEventCount: number;
  dataInsufficient: boolean;
}

// Matches ShoppingListEntryResponse in internal/server/handler_shopping_list.go.
// Auto-derived entries have an empty string id (no backing shopping_list_items row).
export interface ShoppingListEntry {
  id: string;
  itemId: string;
  quantity: number;
  source: 'auto' | 'manual';
  purchasedAt: string | null;
}

// Result of a three-tier product lookup (user override -> global DB -> external API).
// Matches LookupResult in internal/product/lookup.go.
export interface LookupResult {
  product: ProductSummary | null;
  source: string;
}

// Matches batchCommitResponse in internal/server/handler_scan_batch_commit.go.
export interface BatchCommitResponse {
  updatedCount: number;
  committedIds?: string[];
  failedIds?: string[];
  failedReasons?: string[];
}
