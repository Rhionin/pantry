import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { Notifications } from '@mantine/notifications';
import { describe, expect, it, vi } from 'vitest';
import type { InventoryItem, ShoppingListEntry } from '../../types';
import { ShoppingListPage } from './ShoppingListPage';

const inventoryItem = (id: string, name: string, targetQuantity: number | null): InventoryItem => ({
  item: {
    id,
    userId: 'user-1',
    productId: `product-${id}`,
    product: { id: `product-${id}`, name, category: 'Pantry', unitOfMeasure: 'boxes' },
    targetQuantity,
    createdAt: '2026-01-01T00:00:00Z',
  },
  instanceCount: 1,
  nearExpiryCount: 0,
  expiredCount: 0,
  needsAttention: false,
});

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    statusText: status >= 400 ? 'Request failed' : 'OK',
    headers: { 'Content-Type': 'application/json' },
  });

const renderPage = () => render(
  <MantineProvider>
    <Notifications />
    <ShoppingListPage />
  </MantineProvider>,
);

describe('ShoppingListPage', () => {
  it('renders derived and manual entries with product details and marks a manual item purchased', async () => {
    const initialEntries: ShoppingListEntry[] = [
      { id: '', itemId: 'rice', quantity: 2, source: 'auto', purchasedAt: null },
      { id: 'manual-1', itemId: 'tea', quantity: 3, source: 'manual', purchasedAt: null },
    ];
    let purchased = false;
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === '/api/shopping-list' && init?.method === undefined) {
        return Promise.resolve(jsonResponse(purchased ? initialEntries.slice(0, 1) : initialEntries));
      }
      if (url === '/api/inventory') {
        return Promise.resolve(jsonResponse([
          inventoryItem('rice', 'Brown Rice', 3),
          inventoryItem('tea', 'Green Tea', null),
        ]));
      }
      if (url === '/api/shopping-list/items/manual-1') {
        expect(init?.method).toBe('PATCH');
        expect(init?.body).toBe(JSON.stringify({ purchased: true }));
        purchased = true;
        return Promise.resolve(jsonResponse({ ...initialEntries[1], purchasedAt: '2026-03-01T00:00:00Z' }));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPage();

    const table = await screen.findByRole('table', { name: 'Shopping list entries' });
    expect(within(table).getByText('Brown Rice')).toBeInTheDocument();
    expect(within(table).getByText('2 boxes')).toBeInTheDocument();
    expect(within(table).getByText('Derived')).toBeInTheDocument();
    expect(within(table).getByText('Green Tea')).toBeInTheDocument();
    expect(within(table).getByText('Set a target quantity for automatic restocking.')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Mark Green Tea purchased' }));

    await waitFor(() => expect(
      within(screen.getByRole('table', { name: 'Shopping list entries' })).queryByText('Green Tea'),
    ).not.toBeInTheDocument());
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/shopping-list/items/manual-1',
      expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ purchased: true }) }),
    );
  });

  it('adds a manual item using the selected item and quantity', async () => {
    let entries: ShoppingListEntry[] = [];
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === '/api/shopping-list' && init?.method === undefined) return Promise.resolve(jsonResponse(entries));
      if (url === '/api/inventory') return Promise.resolve(jsonResponse([inventoryItem('pasta', 'Pasta', null)]));
      if (url === '/api/shopping-list/items') {
        expect(init?.method).toBe('POST');
        expect(init?.body).toBe(JSON.stringify({ itemId: 'pasta', quantity: 4 }));
        const created = { id: 'manual-pasta', itemId: 'pasta', quantity: 4, source: 'manual', purchasedAt: null } satisfies ShoppingListEntry;
        entries = [created];
        return Promise.resolve(jsonResponse(created, 201));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPage();

    await screen.findByText('Your shopping list is empty.');
    fireEvent.change(screen.getByLabelText('Pantry item'), { target: { value: 'pasta' } });
    fireEvent.change(screen.getByLabelText('Quantity'), { target: { value: '4' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add to shopping list' }));

    expect(await screen.findByText('4 boxes')).toBeInTheDocument();
  });

  it('shows failed cart items in a notification and keeps them in the list', async () => {
    const entry = { id: 'manual-1', itemId: 'tea', quantity: 2, source: 'manual', purchasedAt: null } satisfies ShoppingListEntry;
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === '/api/shopping-list' && init?.method === undefined) return Promise.resolve(jsonResponse([entry]));
      if (url === '/api/inventory') return Promise.resolve(jsonResponse([inventoryItem('tea', 'Green Tea', null)]));
      if (url === '/api/shopping-list/export') {
        return Promise.resolve(jsonResponse({ error: 'cart export failed for items: Green Tea' }, 500));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPage();

    const table = await screen.findByRole('table', { name: 'Shopping list entries' });
    expect(within(table).getByText('Green Tea')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Export to cart' }));

    expect(await screen.findByText('Cart export incomplete')).toBeInTheDocument();
    expect(screen.getByText(/cart export failed for items: Green Tea/)).toBeInTheDocument();
    expect(within(table).getByText('Green Tea')).toBeInTheDocument();
  });
});
