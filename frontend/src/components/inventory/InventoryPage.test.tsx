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
  private readonly listeners = new Map<string, (event: MessageEvent) => void>();

  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners.set(type, listener);
  }

  close() {
    this.closed = true;
  }

  hasListener(type: string) {
    return this.listeners.has(type);
  }

  dispatch(type: string, payload: unknown) {
    this.listeners.get(type)?.({ data: JSON.stringify(payload) } as MessageEvent);
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

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
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
    expect(eventSource.hasListener('error')).toBe(false);

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
});
