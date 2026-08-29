import { describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ScanQueuePage } from './ScanQueuePage';
import type { ScanEntry } from '../../types';

const scanEntry = (overrides: Partial<ScanEntry>): ScanEntry => ({
  id: 'scan-1',
  userId: 'default-user',
  barcode: '111',
  scannedAt: '2026-03-20T10:00:00Z',
  direction: null,
  unitCount: 1,
  expiresAt: null,
  status: 'pending',
  productId: null,
  product: null,
  committedAt: null,
  createdAt: '2026-03-20T10:00:00Z',
  ...overrides,
});

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

describe('ScanQueuePage', () => {
  it('shows pending and flagged scans oldest first with a flagged indicator', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('status=pending')) {
        return Promise.resolve(jsonResponse([
          scanEntry({
            id: 'newer',
            barcode: '222',
            scannedAt: '2026-03-20T11:00:00Z',
            productId: 'product-1',
            product: { id: 'product-1', name: 'Milk', category: 'Dairy', unitOfMeasure: 'carton' },
          }),
        ]));
      }
      if (url.includes('status=flagged')) {
        return Promise.resolve(jsonResponse([
          scanEntry({ id: 'older', barcode: '111', scannedAt: '2026-03-20T09:00:00Z', status: 'flagged' }),
        ]));
      }
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);

    const cards = await screen.findAllByRole('article');
    expect(within(cards[0]).getByText('Barcode: 111')).toBeInTheDocument();
    expect(within(cards[0]).getByText('Flagged')).toBeInTheDocument();
    expect(within(cards[1]).getByText('Barcode: 222')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/scans?userId=default-user&status=pending',
      expect.any(Object),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/scans?userId=default-user&status=flagged',
      expect.any(Object),
    );
  });
});
