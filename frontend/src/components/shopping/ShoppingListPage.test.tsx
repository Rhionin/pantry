import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { Notifications } from '@mantine/notifications';
import { describe, expect, it, vi } from 'vitest';
import type { InventoryItem, ShoppingConsiderations, ShoppingListEntry } from '../../types';
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
      if (url === '/api/providers') return Promise.resolve(jsonResponse([]));
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
      if (url === '/api/shopping-list/considerations') {
        return Promise.resolve(jsonResponse({
          retailerDeals: 'unavailable',
          retailerDetail: '',
          considerations: [],
        }));
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
      if (url === '/api/providers') return Promise.resolve(jsonResponse([]));
      if (url === '/api/shopping-list' && init?.method === undefined) return Promise.resolve(jsonResponse(entries));
      if (url === '/api/inventory') return Promise.resolve(jsonResponse([inventoryItem('pasta', 'Pasta', null)]));
      if (url === '/api/shopping-list/items') {
        expect(init?.method).toBe('POST');
        expect(init?.body).toBe(JSON.stringify({ itemId: 'pasta', quantity: 4 }));
        const created = { id: 'manual-pasta', itemId: 'pasta', quantity: 4, source: 'manual', purchasedAt: null } satisfies ShoppingListEntry;
        entries = [created];
        return Promise.resolve(jsonResponse(created, 201));
      }
      if (url === '/api/shopping-list/considerations') {
        return Promise.resolve(jsonResponse({
          retailerDeals: 'unavailable',
          retailerDetail: '',
          considerations: [],
        }));
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
      if (url === '/api/providers') {
        return Promise.resolve(jsonResponse([{
          id: 'kroger',
          displayName: 'Kroger',
          capabilities: { auth: 'oauth2_authorization_code', delivery: 'server_push', confirmation: 'per_request', mutation: 'add_only', identity: 'derived' },
          connectionState: 'connected',
          credentialsConfigured: true,
        }]));
      }
      if (url === '/api/shopping-list/considerations') {
        return Promise.resolve(jsonResponse({
          retailerDeals: 'unavailable',
          retailerDetail: '',
          considerations: [],
        }));
      }
      if (url.startsWith('/api/shopping-list') && init?.method === undefined) return Promise.resolve(jsonResponse([entry]));
      if (url === '/api/inventory') return Promise.resolve(jsonResponse([inventoryItem('tea', 'Green Tea', null)]));
      if (url === '/api/shopping-list/export') {
        return Promise.resolve(jsonResponse({ error: 'Kroger is not connected' }, 409));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPage();

    const table = await screen.findByRole('table', { name: 'Shopping list entries' });
    expect(within(table).getByText('Green Tea')).toBeInTheDocument();
    expect(screen.queryByLabelText('Client ID')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Client secret')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Redirect URI')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save credentials' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Disconnect' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start a new cart' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add to Kroger cart' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /setup/i })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Add to Kroger cart' }));

    expect(await screen.findByText('Kroger is not connected')).toBeInTheDocument();
    expect(screen.queryByText(/sent to your cart/)).not.toBeInTheDocument();
    expect(within(table).getByText('Green Tea')).toBeInTheDocument();
  });

  it('offers a sale without blocking export, and sends the sale brand only after it is taken', async () => {
    const entry = { id: 'auto-gv', itemId: 'gv', quantity: 1, source: 'auto', purchasedAt: null } satisfies ShoppingListEntry;
    const notes: ShoppingConsiderations = {
      retailerDeals: 'unavailable',
      retailerDetail: "Live store prices aren't connected. Sales you note yourself still show up here.",
      considerations: [{
        lineItemId: 'gv',
        needKey: 'cut green beans',
        genericName: 'cut green beans',
        chosenItemId: 'gv',
        preferredItemId: '',
        ignorePrice: false,
        members: [
          { itemId: 'gv', name: 'Great Value Cut Green Beans', priceCents: null, onSale: false, saleLabel: '', dealSource: '' },
          { itemId: 'kr', name: 'Kroger Cut Green Beans', priceCents: 79, onSale: true, saleLabel: 'Weekly ad', dealSource: 'recorded' },
        ],
        offer: {
          itemId: 'kr',
          name: 'Kroger Cut Green Beans',
          label: 'Weekly ad',
          priceCents: 79,
          usualPriceCents: null,
          source: 'recorded',
        },
      }],
    };
    const exportBodies: Array<BodyInit | null | undefined> = [];
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === '/api/providers') return Promise.resolve(jsonResponse([]));
      if (url === '/api/shopping-list' && init?.method === undefined) return Promise.resolve(jsonResponse([entry]));
      if (url === '/api/inventory') return Promise.resolve(jsonResponse([inventoryItem('gv', 'Great Value Cut Green Beans', 4)]));
      if (url === '/api/shopping-list/considerations') return Promise.resolve(jsonResponse(notes));
      if (url === '/api/shopping-list/export') {
        exportBodies.push(init?.body);
        return Promise.resolve(jsonResponse({ exported: 1 }));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPage();

    expect(await screen.findByText(/Kroger Cut Green Beans is on sale at \$0\.79/)).toBeInTheDocument();
    expect(screen.getByText(/Live store prices aren't connected/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Export to cart' }));
    await screen.findByText('1 item sent to your cart.');
    expect(exportBodies[0]).toBeUndefined();

    fireEvent.click(screen.getByRole('button', { name: 'Take the deal on Kroger Cut Green Beans' }));
    fireEvent.click(screen.getByRole('button', { name: 'Export to cart' }));
    await waitFor(() => expect(exportBodies).toHaveLength(2));
    expect(exportBodies[1]).toBe(JSON.stringify({ useItemIds: { gv: 'kr' } }));
    expect(screen.getByRole('button', { name: 'Keep Great Value Cut Green Beans' })).toBeInTheDocument();
  });

  it('saves a preferred brand and notes a sale price', async () => {
    const entry = { id: 'auto-gv', itemId: 'gv', quantity: 1, source: 'auto', purchasedAt: null } satisfies ShoppingListEntry;
    let notes: ShoppingConsiderations = {
      retailerDeals: 'unavailable',
      retailerDetail: 'Live store prices aren\'t connected. Sales you note yourself still show up here.',
      considerations: [{
        lineItemId: 'gv',
        needKey: 'cut green beans',
        genericName: 'cut green beans',
        chosenItemId: 'gv',
        preferredItemId: '',
        ignorePrice: false,
        members: [
          { itemId: 'gv', name: 'Great Value Cut Green Beans', priceCents: null, onSale: false, saleLabel: '', dealSource: '' },
          { itemId: 'kr', name: 'Kroger Cut Green Beans', priceCents: null, onSale: false, saleLabel: '', dealSource: '' },
        ],
        offer: null,
      }],
    };
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/providers') return Promise.resolve(jsonResponse([]));
      if (url === '/api/shopping-list' && method === 'GET') return Promise.resolve(jsonResponse([entry]));
      if (url === '/api/inventory') {
        return Promise.resolve(jsonResponse([
          inventoryItem('gv', 'Great Value Cut Green Beans', 4),
          inventoryItem('kr', 'Kroger Cut Green Beans', 4),
        ]));
      }
      if (url === '/api/shopping-list/considerations') return Promise.resolve(jsonResponse(notes));
      if (url === '/api/shopping-list/preferences' && method === 'PUT') {
        const body = JSON.parse(String(init?.body)) as { itemId: string; ignorePrice: boolean };
        notes = {
          ...notes,
          considerations: [{ ...notes.considerations[0], preferredItemId: body.itemId, ignorePrice: body.ignorePrice }],
        };
        return Promise.resolve(jsonResponse({ itemId: body.itemId, ignorePrice: body.ignorePrice }));
      }
      if (url === '/api/shopping-list/deals' && method === 'PUT') {
        const body = JSON.parse(String(init?.body)) as { itemId: string; priceCents: number; label: string };
        expect(body).toEqual({ itemId: 'kr', priceCents: 89, label: 'Noted sale' });
        const members = notes.considerations[0].members.map((member) => (
          member.itemId === body.itemId
            ? { ...member, priceCents: body.priceCents, onSale: true, saleLabel: body.label, dealSource: 'recorded' }
            : member
        ));
        notes = { ...notes, considerations: [{ ...notes.considerations[0], members }] };
        return Promise.resolve(jsonResponse(body));
      }
      throw new Error(`Unexpected request: ${url} ${method}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPage();

    const brand = await screen.findByLabelText('Preferred brand for cut green beans');
    fireEvent.change(brand, { target: { value: 'kr' } });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/shopping-list/preferences',
      expect.objectContaining({ method: 'PUT', body: JSON.stringify({ itemId: 'kr', ignorePrice: false }) }),
    ));

    const always = await screen.findByRole('checkbox', { name: 'Always buy this brand of cut green beans' });
    await waitFor(() => expect(always).toBeEnabled());
    fireEvent.click(always);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/shopping-list/preferences',
      expect.objectContaining({ method: 'PUT', body: JSON.stringify({ itemId: 'kr', ignorePrice: true }) }),
    ));

    fireEvent.change(screen.getByLabelText('Brand on sale'), { target: { value: 'kr' } });
    fireEvent.change(screen.getByLabelText('Sale price in cents'), { target: { value: '89' } });
    fireEvent.click(screen.getByRole('button', { name: 'Note sale' }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/shopping-list/deals',
      expect.objectContaining({ method: 'PUT', body: JSON.stringify({ itemId: 'kr', priceCents: 89, label: 'Noted sale' }) }),
    ));
  });
});
