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
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenCalledWith('/api/inventory', expect.any(Object));
  });

  it('hides a missing category and keeps the source label out of the controls', async () => {
    const pineapple = inventoryItem('pineapple', 'Crushed Pineapple', 'undefined', false);
    pineapple.item.product.externalSource = 'openfoodfacts';
    const fetchMock = vi.fn(() => Promise.resolve(jsonResponse([
      pineapple,
      inventoryItem('milk', 'Evaporated milk', '', false),
      inventoryItem('pasta', 'Lasagna', 'Pastas', false),
    ])));
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><InventoryPage /></MantineProvider>);

    await screen.findByText('Crushed Pineapple');
    expect(screen.queryByText(/^undefined$/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/^null$/i)).not.toBeInTheDocument();
    expect(screen.getByText('Pastas')).toBeInTheDocument();

    const articleNamed = (name: string) => screen.getAllByRole('article').find((article) =>
      within(article).queryByRole('heading', { name }) !== null,
    );
    const pineappleArticle = articleNamed('Crushed Pineapple');
    const milkArticle = articleNamed('Evaporated milk');
    expect(pineappleArticle).toBeDefined();
    expect(milkArticle).toBeDefined();
    const source = within(pineappleArticle as HTMLElement).getByText('Open Food Facts');
    expect(source.closest('a, button, .mantine-Badge-root')).toBeNull();
    expect(within(milkArticle as HTMLElement).queryByText('Open Food Facts')).not.toBeInTheDocument();
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
      if (url === '/api/settings/supply') {
        return Promise.resolve(jsonResponse({ months: 3, opening: false, wipePhrase: 'WIPE INVENTORY' }));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><InventoryPage /></MantineProvider>);
    await screen.findByText('Sourdough');
    expect(fetchMock).toHaveBeenCalledTimes(2);

    fireEvent.click(screen.getByRole('button', { name: 'View instances' }));

    const article = screen.getByRole('article');
    await within(article).findByRole('heading', { name: 'Sourdough instances' });
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

  it('changes the on-hand count from the expanded row without collapsing it', async () => {
    let onHand = 0;
    const oats = (): InventoryItem => ({
      ...inventoryItem('oats', 'Rolled Oats', 'Grains', false),
      instanceCount: onHand,
    });
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/settings/supply') {
        return Promise.resolve(jsonResponse({ months: 3, opening: false, wipePhrase: 'WIPE INVENTORY' }));
      }
      if (url === '/api/inventory' && method === 'GET') {
        return Promise.resolve(jsonResponse([oats()]));
      }
      if (url === '/api/inventory/oats/instances' && method === 'GET') {
        return Promise.resolve(jsonResponse([]));
      }
      if (url === '/api/inventory/oats/instances' && method === 'POST') {
        onHand += 1;
        return Promise.resolve(jsonResponse({ id: `inst-${onHand}`, itemId: 'oats' }, 201));
      }
      if (url === '/api/inventory/oats/stock-out' && method === 'POST') {
        onHand = Math.max(0, onHand - 1);
        return Promise.resolve(jsonResponse({}));
      }
      throw new Error(`Unexpected request: ${method} ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><InventoryPage /></MantineProvider>);
    await screen.findByText('Rolled Oats');
    expect(screen.getByText('0 unit')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'View instances' }));
    const article = screen.getByRole('article');
    expect(within(article).getByRole('button', { name: 'Stock out' })).toBeDisabled();
    expect(within(article).getByText('There is nothing on hand.')).toBeInTheDocument();

    fireEvent.click(within(article).getByRole('button', { name: 'Stock in' }));
    await within(article).findByText('1 unit');
    expect(within(article).getByRole('button', { name: 'Hide instances' })).toBeInTheDocument();
    expect(within(article).getByRole('button', { name: 'Stock out' })).toBeEnabled();
    expect(within(article).queryByText('There is nothing on hand.')).not.toBeInTheDocument();

    fireEvent.click(within(article).getByRole('button', { name: 'Stock out' }));
    await within(article).findByText('0 unit');
    expect(within(article).getByRole('button', { name: 'Hide instances' })).toBeInTheDocument();
    expect(within(article).getByRole('button', { name: 'Stock out' })).toBeDisabled();
    expect(within(article).getByText('There is nothing on hand.')).toBeInTheDocument();
  });

  it('shows the opening banner only while the first scan is unfinished', async () => {
    let opening = true;
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/settings/supply') {
        return Promise.resolve(jsonResponse({ months: 3, opening, wipePhrase: 'WIPE INVENTORY' }));
      }
      if (url === '/api/onboarding/complete' && method === 'POST') {
        opening = false;
        return Promise.resolve(jsonResponse({ startedAt: '2026-04-01T00:00:00Z' }));
      }
      if (url === '/api/inventory') {
        return Promise.resolve(jsonResponse([inventoryItem('bread', 'Sourdough', 'Bakery', false)]));
      }
      throw new Error(`Unexpected request: ${method} ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><InventoryPage /></MantineProvider>);
    await screen.findByText('Sourdough');
    expect(screen.getByRole('alert', { name: 'Opening inventory' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Wipe inventory' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'This scan is complete' }));
    await waitFor(() => expect(screen.queryByRole('alert', { name: 'Opening inventory' })).not.toBeInTheDocument());
    expect(fetchMock).toHaveBeenCalledWith('/api/onboarding/complete', expect.objectContaining({ method: 'POST' }));
  });
});
