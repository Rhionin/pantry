import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ScanQueuePage } from './ScanQueuePage';
import type { ScanEntry } from '../../types';

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

const scanEntry = (overrides: Partial<ScanEntry>): ScanEntry => ({
  id: 'scan-1',
  userId: 'user-1',
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
  beforeEach(() => {
    FakeEventSource.instances = [];
    vi.stubGlobal('EventSource', FakeEventSource);
  });

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
      '/api/scans?userId=user-1&status=pending',
      expect.any(Object),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/scans?userId=user-1&status=flagged',
      expect.any(Object),
    );
  });

  it('adds a scan pushed over the event stream without an extra fetch', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('status=pending') || url.includes('status=flagged')) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);
    await screen.findByText('No pending scans.');

    const eventSource = FakeEventSource.instances[0];
    expect(eventSource.url).toBe('/api/events');
    expect(eventSource.hasListener('error')).toBe(false);

    const callCountBeforeEvent = fetchMock.mock.calls.length;
    eventSource.dispatch('scan', scanEntry({ id: 'pushed', barcode: '999' }));

    expect(await screen.findByText('Barcode: 999')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(callCountBeforeEvent);
  });

  it('ignores a manufactured error event and leaves displayed scans unchanged', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('status=pending')) {
        return Promise.resolve(jsonResponse([scanEntry({ id: 'existing', barcode: '111' })]));
      }
      if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);
    await screen.findByText('Barcode: 111');

    const eventSource = FakeEventSource.instances[0];
    eventSource.dispatch('error', {});

    expect(screen.getByText('Barcode: 111')).toBeInTheDocument();
    expect(screen.queryAllByRole('article')).toHaveLength(1);
  });

  it('closes the event stream connection on unmount', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(jsonResponse([])));
    vi.stubGlobal('fetch', fetchMock);

    const { unmount } = render(<MantineProvider><ScanQueuePage /></MantineProvider>);
    await screen.findByText('No pending scans.');

    const eventSource = FakeEventSource.instances[0];
    expect(eventSource.closed).toBe(false);

    unmount();

    expect(eventSource.closed).toBe(true);
  });

  it('defaults to Stock_Out_View on initial mount', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('status=pending')) {
        return Promise.resolve(jsonResponse([
          scanEntry({ id: 'scan-1', barcode: '111', direction: 'stock_out' }),
          scanEntry({ id: 'scan-2', barcode: '222', direction: 'stock_in' }),
        ]));
      }
      if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);

    // Verify the Tabs component renders with Stock_Out_View as the default
    const tabs = await screen.findByRole('tablist');
    
    // Find tabs by their ID attributes to ensure we get the correct tab elements
    const stockOutTab = screen.getByRole('tab', { name: 'Stock out' });
    const stockInTab = screen.getByRole('tab', { name: 'Stock in' });

    // Verify Stock_Out tab is selected (aria-selected="true" for active, "false" for inactive)
    expect(stockOutTab).toHaveAttribute('aria-selected', 'true');
    expect(stockInTab).toHaveAttribute('aria-selected', 'false');

    // Verify the grid shows only stock_out entries (direction === 'stock_out' or null)
    const cards = await screen.findAllByRole('article');
    expect(cards).toHaveLength(1);
    expect(within(cards[0]).getByText('Barcode: 111')).toBeInTheDocument();

    // Verify clicking Stock_In tab changes the content
    stockInTab.click();

    // Now stock_in entries should be visible
    await screen.findByText('Barcode: 222');
    const cardsAfterSwitch = await screen.findAllByRole('article');
    expect(cardsAfterSwitch).toHaveLength(1);
    expect(within(cardsAfterSwitch[0]).getByText('Barcode: 222')).toBeInTheDocument();
  });

  it('defaults to Stock_Out_View on a fresh mount after prior instance was switched to Stock_In_View', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('status=pending')) {
        return Promise.resolve(jsonResponse([
          scanEntry({ id: 'scan-1', barcode: '111', direction: 'stock_out' }),
          scanEntry({ id: 'scan-2', barcode: '222', direction: 'stock_in' }),
        ]));
      }
      if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    const { unmount } = render(<MantineProvider><ScanQueuePage /></MantineProvider>);

    // First mount: switch to Stock_In_View
    const tabs = await screen.findByRole('tablist');
    const stockInTab = screen.getByRole('tab', { name: 'Stock in' });
    stockInTab.click();

    // Verify we're now showing stock_in entries
    await screen.findByText('Barcode: 222');
    const cardsAfterSwitch = await screen.findAllByRole('article');
    expect(cardsAfterSwitch).toHaveLength(1);
    expect(within(cardsAfterSwitch[0]).getByText('Barcode: 222')).toBeInTheDocument();

    unmount();

    // Second mount: should default back to Stock_Out_View (no persistence)
    render(<MantineProvider><ScanQueuePage /></MantineProvider>);

    // Verify the Tabs component renders with Stock_Out_View as the default on fresh mount
    const stockOutTab2 = screen.getByRole('tab', { name: 'Stock out' });
    const stockInTab2 = screen.getByRole('tab', { name: 'Stock in' });

    // Verify Stock_Out tab is selected (aria-selected="true" for active, "false" for inactive)
    expect(stockOutTab2).toHaveAttribute('aria-selected', 'true');
    expect(stockInTab2).toHaveAttribute('aria-selected', 'false');

    // Verify the grid shows only stock_out entries again
    const cards2 = await screen.findAllByRole('article');
    expect(cards2).toHaveLength(1);
    expect(within(cards2[0]).getByText('Barcode: 111')).toBeInTheDocument();

    // Verify clicking Stock_In tab still works on the fresh mount
    stockInTab2.click();

    await screen.findByText('Barcode: 222');
    const cardsAfterSwitch2 = await screen.findAllByRole('article');
    expect(cardsAfterSwitch2).toHaveLength(1);
    expect(within(cardsAfterSwitch2[0]).getByText('Barcode: 222')).toBeInTheDocument();
  });
});
