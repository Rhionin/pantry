import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ScanEntryCard } from './ScanEntryCard';
import type { ItemInstanceWithStatus, ScanEntry } from '../../types';

const entry: ScanEntry = {
  id: 'scan-1',
  userId: 'default-user',
  barcode: '123',
  scannedAt: '2026-03-20T10:00:00Z',
  direction: 'stock_out',
  unitCount: 1,
  expiresAt: null,
  status: 'pending',
  productId: 'product-1',
  product: { id: 'product-1', name: 'Milk', category: 'Dairy', unitOfMeasure: 'carton' },
  committedAt: null,
  createdAt: '2026-03-20T10:00:00Z',
};

const instances: ItemInstanceWithStatus[] = [
  { id: 'later', itemId: 'item-1', stockInAt: '2026-03-02T00:00:00Z', expiresAt: '2026-05-01T00:00:00Z', removedAt: null, removalReason: null, createdAt: '2026-03-02T00:00:00Z', expiryStatus: 'ok' },
  { id: 'oldest', itemId: 'item-1', stockInAt: '2026-03-01T00:00:00Z', expiresAt: '2026-04-01T00:00:00Z', removedAt: null, removalReason: null, createdAt: '2026-03-01T00:00:00Z', expiryStatus: 'ok' },
  { id: 'undated', itemId: 'item-1', stockInAt: '2026-03-03T00:00:00Z', expiresAt: null, removedAt: null, removalReason: null, createdAt: '2026-03-03T00:00:00Z', expiryStatus: 'ok' },
];

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

describe('ScanEntryCard stock-out review', () => {
  it('orders instances oldest first and commits the specifically selected instance', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/inventory/item-1/instances') return Promise.resolve(jsonResponse(instances));
      if (url === '/api/scans/scan-1/commit') return Promise.resolve(jsonResponse({ ...entry, status: 'committed' }));
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onChanged = vi.fn();
    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={onChanged}
        />
      </MantineProvider>,
    );

    const options = await screen.findAllByRole('radio');
    expect(options.map((option) => option.getAttribute('value'))).toEqual(['', 'oldest', 'later', 'undated']);
    fireEvent.click(options[2]);
    fireEvent.click(screen.getByRole('button', { name: 'Commit scan' }));

    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const commitCall = fetchMock.mock.calls.find(([url]) => String(url).includes('/commit'));
    expect(JSON.parse(commitCall?.[1]?.body as string)).toEqual({ instanceId: 'later' });
  });
});
