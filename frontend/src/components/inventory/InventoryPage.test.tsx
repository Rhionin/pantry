import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { InventoryItem } from '../../types';
import { InventoryPage } from './InventoryPage';

// Records the URL a component connects to, the listeners it registers, and
// whether it closes the connection, so tests can simulate the server pushing
// a message without a real network connection.
class FakeEventSource {
  static instances: FakeEventSource[] = [];

  readonly url: string;
  closed = false;
  private readonly listeners = new Map<string, Array<(event: MessageEvent) => void>>();

  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    const current = this.listeners.get(type) ?? [];
    current.push(listener);
    this.listeners.set(type, current);
  }

  close() {
    this.closed = true;
  }

  hasListener(type: string) {
    return (this.listeners.get(type)?.length ?? 0) > 0;
  }

  dispatch(type: string, payload: unknown) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data: JSON.stringify(payload) } as MessageEvent);
    }
  }
}

const inventoryItem = (
  id: string,
  name: string,
  category: string,
  needsAttention: boolean,
): InventoryItem => ({
  item: {
    id,
    userId: 'user-1',
    productId: `product-${id}`,
    product: { id: `product-${id}`, name, category, unitOfMeasure: 'unit' },
    targetQuantity: null,
    createdAt: '2026-01-01T00:00:00Z',
  },
  instanceCount: needsAttention ? 2 : 1,
  nearExpiryCount: needsAttention ? 1 : 0,
  expiredCount: needsAttention ? 1 : 0,
  needsAttention,
});

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });

describe('InventoryPage', () => {
  beforeEach(() => {
    FakeEventSource.instances = [];
    vi.stubGlobal('EventSource', FakeEventSource);
  });

  it('shows attention items first and filters the fetched inventory locally', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(jsonResponse([
      inventoryItem('bread', 'Sourdough', 'Bakery', false),
      inventoryItem('milk', 'Whole Milk', 'Dairy', true),
    ])));
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><InventoryPage /></MantineProvider>);

    await screen.findByText('Whole Milk');
    const sectionHeadings = screen.getAllByRole('heading', { level: 2 });
    expect(sectionHeadings.map((heading) => heading.textContent)).toEqual([
      'Needs Attention',
      'Inventory items',
    ]);
    const attentionSection = screen.getByRole('region', { name: 'Needs Attention' });
    expect(within(attentionSection).getByText('1 near expiry')).toBeInTheDocument();
    expect(within(attentionSection).getByText('1 expired')).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Search inventory'), {
      target: { value: 'bakery' },
    });

    expect(screen.getByText('Sourdough')).toBeInTheDocument();
    expect(screen.queryByText('Whole Milk')).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith('/api/inventory', expect.any(Object));
  });

  it('fetches instances only after an item is selected', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/inventory') {
        return Promise.resolve(jsonResponse([
          inventoryItem('bread', 'Sourdough', 'Bakery', false),
        ]));
      }
      if (url === '/api/inventory/bread/instances') {
        return Promise.resolve(jsonResponse([]));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><InventoryPage /></MantineProvider>);
    await screen.findByText('Sourdough');
    expect(fetchMock).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole('button', { name: 'View instances' }));

    await screen.findByRole('heading', { name: 'Sourdough instances' });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/inventory/bread/instances',
      expect.any(Object),
    ));
  });

  it('updates a displayed item pushed over the event stream', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(jsonResponse([
      inventoryItem('bread', 'Sourdough', 'Bakery', false),
    ])));
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><InventoryPage /></MantineProvider>);
    await screen.findByText('Sourdough');

    const eventSource = FakeEventSource.instances[0];
    expect(eventSource.url).toBe('/api/events');
    expect(eventSource.hasListener('error')).toBe(true);

    eventSource.dispatch('inventory', inventoryItem('bread', 'Sourdough', 'Bakery', true));

    const attentionSection = await screen.findByRole('region', { name: 'Needs Attention' });
    expect(within(attentionSection).getByText('Sourdough')).toBeInTheDocument();
  });

  it('ignores a manufactured error event and leaves displayed inventory unchanged', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(jsonResponse([
      inventoryItem('bread', 'Sourdough', 'Bakery', false),
    ])));
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><InventoryPage /></MantineProvider>);
    await screen.findByText('Sourdough');

    const eventSource = FakeEventSource.instances[0];
    eventSource.dispatch('error', {});

    expect(screen.getByText('Sourdough')).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Needs Attention' })).not.toBeInTheDocument();
  });

  it('closes the event stream connection on unmount', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(jsonResponse([])));
    vi.stubGlobal('fetch', fetchMock);

    const { unmount } = render(<MantineProvider><InventoryPage /></MantineProvider>);
    await screen.findByText('Your inventory is empty.');

    const eventSource = FakeEventSource.instances[0];
    expect(eventSource.closed).toBe(false);

    unmount();

    expect(eventSource.closed).toBe(true);
  });

  it('requires the exact confirmation phrase before wiping inventory', async () => {
    let inventory = [inventoryItem('bread', 'Sourdough', 'Bakery', false)];
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/inventory/wipe' && method === 'POST') {
        inventory = [];
        return Promise.resolve(jsonResponse({}));
      }
      if (url === '/api/inventory' && method === 'GET') {
        return Promise.resolve(jsonResponse(inventory));
      }
      return Promise.reject(new Error(`Unexpected request: ${method} ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><InventoryPage /></MantineProvider>);
    await screen.findByText('Sourdough');

    fireEvent.click(screen.getByRole('button', { name: 'Wipe inventory' }));
    const confirmButton = await screen.findByRole('button', { name: 'Confirm wipe' });
    expect(confirmButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText('Type WIPE INVENTORY to confirm'), {
      target: { value: 'wipe inventory' },
    });
    expect(confirmButton).toBeDisabled();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    fireEvent.change(screen.getByLabelText('Type WIPE INVENTORY to confirm'), {
      target: { value: 'WIPE INVENTORY' },
    });
    expect(confirmButton).toBeEnabled();
    fireEvent.click(confirmButton);

    expect(await screen.findByText('Your inventory is empty.')).toBeInTheDocument();
    expect(screen.queryByText('Sourdough')).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith('/api/inventory/wipe', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ confirmation: 'WIPE INVENTORY' }),
    }));
  });

  it('keeps the displayed inventory when a wipe is rejected', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/inventory/wipe' && method === 'POST') {
        return Promise.resolve(jsonResponse({ error: 'Type WIPE INVENTORY to confirm wiping the inventory.' }, 400));
      }
      if (url === '/api/inventory' && method === 'GET') {
        return Promise.resolve(jsonResponse([inventoryItem('bread', 'Sourdough', 'Bakery', false)]));
      }
      return Promise.reject(new Error(`Unexpected request: ${method} ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><InventoryPage /></MantineProvider>);
    await screen.findByText('Sourdough');

    fireEvent.click(screen.getByRole('button', { name: 'Wipe inventory' }));
    fireEvent.change(await screen.findByLabelText('Type WIPE INVENTORY to confirm'), {
      target: { value: 'WIPE INVENTORY' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Confirm wipe' }));

    expect(await screen.findByText('Type WIPE INVENTORY to confirm wiping the inventory.')).toBeInTheDocument();
    expect(screen.getByText('Sourdough')).toBeInTheDocument();
  });
});
