import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { DisambiguationModal } from './DisambiguationModal';
import type { ProductSummary } from '../../types';

const products: ProductSummary[] = [
  { id: 'a', name: 'Apple Juice', category: 'Drinks', unitOfMeasure: 'bottle' },
  { id: 'b', name: 'Apple Sauce', category: 'Pantry', unitOfMeasure: 'jar' },
];

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 201, headers: { 'Content-Type': 'application/json' } });

describe('DisambiguationModal', () => {
  it('allows choosing a match and remembering it as a barcode override', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(jsonResponse({})));
    vi.stubGlobal('fetch', fetchMock);
    const onSelect = vi.fn();
    const onClose = vi.fn();
    render(
      <MantineProvider>
        <DisambiguationModal
          opened
          barcode="999"
          products={products}
          onSelect={onSelect}
          onClose={onClose}
        />
      </MantineProvider>,
    );

    fireEvent.click(screen.getByRole('radio', { name: /Apple Sauce/ }));
    fireEvent.click(screen.getByRole('checkbox', { name: /Remember this choice/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Use product' }));

    await waitFor(() => expect(onSelect).toHaveBeenCalledWith(products[1]));
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/products/overrides',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ barcode: '999', productId: 'b' }) }),
    );
    expect(onClose).toHaveBeenCalled();
  });
});
