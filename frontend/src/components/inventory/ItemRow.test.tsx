import type { ReactNode } from 'react';
import { render, screen, within } from '@testing-library/react';
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

const renderRow = (
  item: InventoryItem = inventoryItem,
  props: { selected?: boolean; children?: ReactNode } = {},
) => render(
  <MantineProvider>
    <ItemRow
      inventoryItem={item}
      selected={props.selected ?? false}
      controlsId="controls-item-1"
      onSelect={vi.fn()}
    >
      {props.children}
    </ItemRow>
  </MantineProvider>,
);

const withProduct = (product: Partial<InventoryItem['item']['product']>): InventoryItem => ({
  ...inventoryItem,
  item: {
    ...inventoryItem.item,
    product: { ...inventoryItem.item.product, ...product },
  },
});

describe('ItemRow', () => {
  it('names the data source in quiet text instead of a control', () => {
    renderRow(withProduct({ externalSource: 'openfoodfacts' }));

    const source = screen.getByLabelText('Product data from Open Food Facts');
    expect(source).toHaveTextContent('Open Food Facts');
    expect(source.closest('.mantine-Badge-root, a, button')).toBeNull();
    expect(screen.queryByRole('link', { name: /Open Food Facts/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Open Food Facts/i })).not.toBeInTheDocument();
  });

  it('omits the source line when the product has no external source', () => {
    renderRow();

    expect(screen.queryByLabelText(/Product data from/)).not.toBeInTheDocument();
  });

  it('shows a real category and hides missing category text', () => {
    const { rerender } = renderRow();
    expect(screen.getByText('Dairy')).toBeInTheDocument();

    for (const category of ['', '   ', 'undefined', 'UNDEFINED', 'null', 'Null']) {
      rerender(
        <MantineProvider>
          <ItemRow
            inventoryItem={withProduct({ category })}
            selected={false}
            controlsId="controls-item-1"
            onSelect={vi.fn()}
          />
        </MantineProvider>,
      );
      const shown = category.trim();
      if (shown !== '') expect(screen.queryByText(shown)).not.toBeInTheDocument();
      expect(screen.queryByText('Dairy')).not.toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'Milk' })).toBeInTheDocument();
    }
  });

  it('keeps instances inside the product and uses a compact disclosure', () => {
    renderRow(inventoryItem, {
      selected: true,
      children: <p>Stocked in Jan 1, 2026</p>,
    });

    const article = screen.getByRole('article');
    expect(within(article).getByText('Stocked in Jan 1, 2026')).toBeInTheDocument();
    const disclosure = within(article).getByRole('button', { name: 'Hide instances' });
    expect(disclosure).toHaveAttribute('data-variant', 'subtle');
    expect(disclosure).not.toHaveStyle({ width: '100%' });
  });
});
