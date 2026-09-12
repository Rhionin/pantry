import type { InventoryItem } from '../../types';

// Applies one incoming Inventory_Event: replaces the item with a matching
// id, or appends it if no such item is currently displayed. Requirement 5.2.
export const mergeInventoryEvent = (
  items: InventoryItem[],
  event: InventoryItem,
): InventoryItem[] => {
  const index = items.findIndex((item) => item.item.id === event.item.id);
  if (index === -1) return [...items, event];
  return items.map((item) => (item.item.id === event.item.id ? event : item));
};
