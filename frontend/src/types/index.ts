// Shared TypeScript types mirroring the backend's camelCase JSON API shapes.
// Field-by-field verified against the Go domain structs and handler response
// types in internal/product, internal/scan, internal/inventory,
// internal/suggestion, internal/shopping, and internal/server.

export type ExternalSource = 'openfoodfacts' | 'openproductsfacts' | 'openbeautyfacts' | 'openpetfoodfacts';

export interface ProductSummary {
  id: string;
  name: string;
  category: string;
  unitOfMeasure: string;
  // Thumbnail sourced from Open Food Facts. Omitted from the JSON response
  // (via omitempty) when no image is available.
  imageUrl?: string;
  // Optional field indicating the external database source for this product.
  externalSource?: ExternalSource;
}

export interface Product extends ProductSummary {
  createdAt: string; // ISO 8601
}

export type ScanDirection = 'stock_in' | 'stock_out';

export type ScanStatus = 'pending' | 'flagged' | 'committed' | 'cancelled';

// Reserved control-barcode strings the backend classifies against, exposed via
// GET /api/scanner/config so the browser recognizes the exact same values.
// Matches scannerConfigResponse in internal/server/handler_scanner.go.
export interface ScannerConfig {
  stockInBarcode: string;
  stockOutBarcode: string;
  currentMode: ScanDirection;
  // True while the headless capture device is open. False when it is missing
  // or no listener is configured.
  connected: boolean;
}

// A barcode the server has accepted and is still looking up. Not a stored
// scan entry. The following scan event replaces it.
export interface ProcessingNotice {
  id: string;
  userId: string;
  barcode: string;
  direction: ScanDirection | null;
  scannedAt: string;
}

export interface ProcessingFailure {
  id: string;
  barcode: string;
  message: string;
}

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

export type ReplenishmentMode = 'target' | 'replenish';

export type ConnectionState = 'not_required' | 'connected' | 'reauth_required' | 'disconnected';

export interface ProviderCapabilities {
  auth: string;
  delivery: string;
  confirmation: string;
  mutation: string;
  identity: string;
}

// Matches ProviderInfo in internal/server/handler_providers.go.
export interface ProviderInfo {
  id: string;
  displayName: string;
  capabilities: ProviderCapabilities;
  connectionState: ConnectionState;
  credentialsConfigured: boolean;
  credentials?: ProviderCredentials;
}

// Non-secret view of a provider's application credentials. The client secret
// is never included.
export interface ProviderCredentials {
  clientId: string;
  redirectUri: string;
  modality: string;
  secretSet: boolean;
  source: 'saved' | 'environment' | 'none';
}

export interface ShoppingListBasis {
  mode: ReplenishmentMode;
  targetQuantity?: number;
  instanceCount: number;
  consumedUnits: number;
  requested: number;
}

// Matches ShoppingListEntryResponse in internal/server/handler_shopping_list.go.
// Auto-derived entries have an empty string id (no backing shopping_list_items row).
// quantity is the provision quantity: an adjustment when one is set, otherwise
// the computed quantity.
export interface ShoppingListEntry {
  id: string;
  itemId: string;
  quantity: number;
  source: 'auto' | 'manual';
  purchasedAt: string | null;
  replenishmentMode?: ReplenishmentMode;
  provider?: string;
  computedQuantity?: number;
  adjustment?: number;
  basis?: ShoppingListBasis;
}

export interface ProvisionEntryResult {
  entryId: string;
  itemId: string;
  name: string;
  quantity: number;
  outcome: 'confirmed' | 'failed' | 'unknown' | string;
  outcomeReason?: string;
}

export interface ProvisionReport {
  provider: string;
  exported: number;
  failedItems?: string[];
  unknownItems?: string[];
  entries?: ProvisionEntryResult[];
}

// One brand that can fill a shared replenishment need.
export interface BrandMember {
  itemId: string;
  name: string;
  priceCents: number | null;
  onSale: boolean;
  saleLabel: string;
  dealSource: string;
}

// A sale on a brand other than the one the list currently buys.
export interface DealOffer {
  itemId: string;
  name: string;
  label: string;
  priceCents: number | null;
  usualPriceCents: number | null;
  source: string;
}

// Matches considerationResponse in internal/server/handler_shopping_brand.go.
export interface ShoppingConsideration {
  lineItemId: string;
  needKey: string;
  genericName: string;
  chosenItemId: string;
  preferredItemId: string;
  ignorePrice: boolean;
  members: BrandMember[];
  offer: DealOffer | null;
}

// Matches considerationsResponse. retailerDeals is "unavailable" or "stubbed".
export interface ShoppingConsiderations {
  retailerDeals: string;
  retailerDetail: string;
  considerations: ShoppingConsideration[];
}

// Result of a three-tier product lookup (user override -> global DB -> external API).
// Matches LookupResult in internal/product/lookup.go.
export interface LookupResult {
  product: ProductSummary | null;
  source: string;
}

// Matches buildInfoResponse in internal/server/handler_build.go.
// commit is the full git SHA the image is tagged with, or "unknown".
// committedAt and subject are omitted when the build was not stamped.
export interface BuildInfo {
  commit: string;
  committedAt?: string;
  subject?: string;
}

// Matches batchCommitResponse in internal/server/handler_scan_batch_commit.go.
export interface BatchCommitResponse {
  updatedCount: number;
  committedIds?: string[];
  failedIds?: string[];
  failedReasons?: string[];
}
