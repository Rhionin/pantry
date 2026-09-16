import { render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { describe, expect, it, vi } from 'vitest';
import { ItemRow } from './ItemRow';
import type { InventoryItem } from '../../types';

const inventoryItem: InventoryItem = {
  item: {
    id: 'item-1',
    userId: 'user-1',
    productId: 'product-1',
    product: {
      id: 'product-1',
      name: 'Milk',
      category: 'Dairy',
      unitOfMeasure: 'carton',
    },
    targetQuantity: null,
    createdAt: '2026-01-01T00:00:00Z',
  },
  instanceCount: 3,
  nearExpiryCount: 0,
  expiredCount: 0,
  needsAttention: false,
};

describe('ItemRow provenance badge', () => {
  it('displays provenance badge when product has externalSource', () => {
    render(
      <MantineProvider>
        <ItemRow
          inventoryItem={{
            ...inventoryItem,
            item: {
              ...inventoryItem.item,
              product: { ...inventoryItem.item.product, externalSource: 'openfoodfacts' },
            },
          }}
          selected={false}
          controlsId="controls-item-1"
          onSelect={vi.fn()}
        />
      </MantineProvider>,
    );

    const badge = screen.getByLabelText('Product data from Open Food Facts');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveTextContent('Open Food Facts');
  });

  it('does not display badge when product has no externalSource', () => {
    render(
      <MantineProvider>
        <ItemRow
          inventoryItem={inventoryItem}
          selected={false}
          controlsId="controls-item-1"
          onSelect={vi.fn()}
        />
      </MantineProvider>,
    );

    const badge = screen.queryByLabelText(/Product data from/);
    expect(badge).not.toBeInTheDocument();
  });
});
