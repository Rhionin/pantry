// Pure function mirroring internal/shopping/derive.go's DeriveShoppingList
// gap-quantity logic (Property 12: shopping list gap quantity is always correct).
import type { InventoryItem, ShoppingListEntry } from '../types';

// Auto-derived entries have no backing shopping_list_items row, so there is no
// natural id for them. We reuse the underlying item's id as the entry id — this
// mirrors itemId being the correlation key the backend's merged response
// (handler_shopping_list_get.go) uses to match derived and manual entries, and
// keeps ids stable and unique per item.
export function deriveShoppingListEntries(items: InventoryItem[]): ShoppingListEntry[] {
  const entries: ShoppingListEntry[] = [];

  for (const inventoryItem of items) {
    const { targetQuantity } = inventoryItem.item;
    if (targetQuantity === null || inventoryItem.instanceCount >= targetQuantity) {
      continue;
    }

    entries.push({
      id: inventoryItem.item.id,
      itemId: inventoryItem.item.id,
      quantity: targetQuantity - inventoryItem.instanceCount,
      source: 'auto',
      purchasedAt: null,
    });
  }

  return entries;
}
