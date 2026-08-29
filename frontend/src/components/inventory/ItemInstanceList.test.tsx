import { render, screen, within } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { describe, expect, it, vi } from 'vitest';
import type { ItemInstanceWithStatus } from '../../types';
import { ItemInstanceList } from './ItemInstanceList';

const DAY_MS = 24 * 60 * 60 * 1000;

const instance = (
  id: string,
  expiresAt: string | null,
  expiryStatus: ItemInstanceWithStatus['expiryStatus'] = 'ok',
): ItemInstanceWithStatus => ({
  id,
  itemId: 'item-1',
  stockInAt: '2026-01-01T00:00:00Z',
  expiresAt,
  removedAt: null,
  removalReason: null,
  createdAt: '2026-01-01T00:00:00Z',
  expiryStatus,
});

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });

describe('ItemInstanceList', () => {
  it('sorts use-oldest-first and computes expiry badges from expiration dates', async () => {
    const now = Date.now();
    const fetchMock = vi.fn(() => Promise.resolve(jsonResponse([
      instance('no-expiry', null, 'expired'),
      instance('near', new Date(now + (2 * DAY_MS)).toISOString()),
      instance('expired', new Date(now - (2 * DAY_MS)).toISOString()),
    ])));
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ItemInstanceList itemId="item-1" productName="Milk" onInventoryChanged={vi.fn()} />
      </MantineProvider>,
    );

    const rows = await screen.findAllByRole('listitem');
    expect(within(rows[0]).getByText('Expired')).toBeInTheDocument();
    expect(within(rows[1]).getByText('Near expiry')).toBeInTheDocument();
    expect(within(rows[2]).getByText('No expiration date')).toBeInTheDocument();
    expect(within(rows[2]).queryByText('Expired')).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/inventory/item-1/instances',
      expect.any(Object),
    );
  });
});
