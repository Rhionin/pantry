import { describe, expect, it } from 'vitest';
import fc from 'fast-check';
import { deriveShoppingListEntries } from './shoppingList';
import type { InventoryItem } from '../types';

// Builds an InventoryItem with a unique id and the given target quantity /
// instance count, filling in the remaining fields with fixed placeholder values
// that the property under test does not depend on.
function buildInventoryItem(id: string, targetQuantity: number | null, instanceCount: number): InventoryItem {
  return {
    item: {
      id,
      userId: 'user-1',
      productId: `product-${id}`,
      product: {
        id: `product-${id}`,
        name: `Item ${id}`,
        category: 'category',
        unitOfMeasure: 'unit',
      },
      targetQuantity,
      createdAt: '2024-01-01T00:00:00Z',
    },
    instanceCount,
    nearExpiryCount: 0,
    expiredCount: 0,
    needsAttention: false,
  };
}

const inventoryItemsArbitrary = fc
  .array(
    fc.record({
      targetQuantity: fc.option(fc.integer({ min: 0, max: 100 }), { nil: null }),
      instanceCount: fc.integer({ min: 0, max: 100 }),
    }),
    { maxLength: 25 },
  )
  .map((raws) => raws.map((raw, index) => buildInventoryItem(`item-${index}`, raw.targetQuantity, raw.instanceCount)));

// Feature: pantry-management, Property 12: Shopping list gap quantity is always correct
//
// Validates: Requirements 4.1, 4.2, 4.8
//
// For any item with a target quantity T and a current instance count C, if C < T the item SHALL
// appear in the shopping list with quantity exactly (T - C); if C >= T (or T is null) the item
// SHALL NOT appear in the auto-generated portion of the shopping list.
describe('deriveShoppingListEntries', () => {
  it('includes exactly the below-target items with the correct gap quantity, and excludes the rest', () => {
    fc.assert(
      fc.property(inventoryItemsArbitrary, (items) => {
        const entries = deriveShoppingListEntries(items);
        const entriesByItemId = new Map(entries.map((entry) => [entry.itemId, entry]));

        const belowTarget = items.filter(
          (inventoryItem) =>
            inventoryItem.item.targetQuantity !== null &&
            inventoryItem.instanceCount < inventoryItem.item.targetQuantity,
        );

        expect(entries).toHaveLength(belowTarget.length);

        for (const inventoryItem of items) {
          const { targetQuantity } = inventoryItem.item;
          const entry = entriesByItemId.get(inventoryItem.item.id);

          if (targetQuantity !== null && inventoryItem.instanceCount < targetQuantity) {
            expect(entry).toBeDefined();
            expect(entry?.quantity).toBe(targetQuantity - inventoryItem.instanceCount);
            expect(entry?.source).toBe('auto');
            expect(entry?.purchasedAt).toBeNull();
          } else {
            expect(entry).toBeUndefined();
          }
        }
      }),
    );
  });
});
