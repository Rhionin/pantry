import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';
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

  // Reproduction test A (FEAT-001): scanning the STOCK_IN control barcode in
  // the browser must switch the displayed scanner mode to STOCK IN and must NOT
  // POST the literal control string 'STOCK_IN' as a product scan. Against
  // current code this FAILS because the browser capture path
  // (BarcodeInputField -> captureBarcode -> POST /api/scans) never classifies
  // the reserved control barcodes, so 'STOCK_IN' is sent as a product barcode
  // and no mode switch occurs. This test is the acceptance gate for the fix in
  // FEAT-002/FEAT-003.
  it('scanning the STOCK_IN control barcode switches to STOCK IN mode without posting a scan', async () => {
    const scanPosts: string[] = [];
    const modePosts: string[] = [];
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/scanner/mode') && init?.method === 'POST') {
        const body = init.body ? (JSON.parse(String(init.body)) as { mode: string }) : { mode: '' };
        modePosts.push(body.mode);
        return Promise.resolve(jsonResponse({ mode: body.mode }));
      }
      if (url.endsWith('/api/scanner/config')) {
        return Promise.resolve(jsonResponse({ stockInBarcode: 'STOCK_IN', stockOutBarcode: 'STOCK_OUT' }));
      }
      if (url.endsWith('/api/scans') && init?.method === 'POST') {
        const body = init.body ? (JSON.parse(String(init.body)) as { barcode: string }) : { barcode: '' };
        scanPosts.push(body.barcode);
        return Promise.resolve(jsonResponse(scanEntry({ id: 'created', barcode: body.barcode })));
      }
      if (url.includes('status=pending') || url.includes('status=flagged')) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);
    await screen.findByText('No pending scans.');

    // The page defaults to STOCK OUT; scanning STOCK_IN must flip it to STOCK IN.
    expect(screen.getByText('STOCK OUT')).toBeInTheDocument();

    const input = screen.getByLabelText(/barcode scanner input/i);
    fireEvent.change(input, { target: { value: 'STOCK_IN' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    // The control barcode must switch the displayed mode to STOCK IN.
    expect(await screen.findByText('STOCK IN')).toBeInTheDocument();

    // The literal control string must never be posted as a product scan;
    // instead the mode-switch endpoint must be called.
    expect(scanPosts).not.toContain('STOCK_IN');
    expect(modePosts).toContain('stock_in');
  });

  it('scanning the STOCK_OUT control barcode switches to STOCK OUT mode without posting a scan', async () => {
    const scanPosts: string[] = [];
    const modePosts: string[] = [];
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/scanner/mode') && init?.method === 'POST') {
        const body = init.body ? (JSON.parse(String(init.body)) as { mode: string }) : { mode: '' };
        modePosts.push(body.mode);
        return Promise.resolve(jsonResponse({ mode: body.mode }));
      }
      if (url.endsWith('/api/scanner/config')) {
        return Promise.resolve(jsonResponse({ stockInBarcode: 'STOCK_IN', stockOutBarcode: 'STOCK_OUT' }));
      }
      if (url.endsWith('/api/scans') && init?.method === 'POST') {
        const body = init.body ? (JSON.parse(String(init.body)) as { barcode: string }) : { barcode: '' };
        scanPosts.push(body.barcode);
        return Promise.resolve(jsonResponse(scanEntry({ id: 'created', barcode: body.barcode })));
      }
      if (url.includes('status=pending') || url.includes('status=flagged')) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);
    await screen.findByText('No pending scans.');

    // Switch to STOCK IN first via SSE so the STOCK_OUT scan produces a visible change.
    const eventSource = FakeEventSource.instances[0];
    eventSource.dispatch('scanner_mode', { mode: 'stock_in' });
    expect(await screen.findByText('STOCK IN')).toBeInTheDocument();

    const input = screen.getByLabelText(/barcode scanner input/i);
    fireEvent.change(input, { target: { value: 'STOCK_OUT' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(await screen.findByText('STOCK OUT')).toBeInTheDocument();
    expect(scanPosts).not.toContain('STOCK_OUT');
    expect(modePosts).toContain('stock_out');
  });

  it('scanning a normal product barcode still creates a scan entry', async () => {
    const scanPosts: string[] = [];
    const modePosts: string[] = [];
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/scanner/mode') && init?.method === 'POST') {
        const body = init.body ? (JSON.parse(String(init.body)) as { mode: string }) : { mode: '' };
        modePosts.push(body.mode);
        return Promise.resolve(jsonResponse({ mode: body.mode }));
      }
      if (url.endsWith('/api/scanner/config')) {
        return Promise.resolve(jsonResponse({ stockInBarcode: 'STOCK_IN', stockOutBarcode: 'STOCK_OUT' }));
      }
      if (url.endsWith('/api/scans') && init?.method === 'POST') {
        const body = init.body ? (JSON.parse(String(init.body)) as { barcode: string }) : { barcode: '' };
        scanPosts.push(body.barcode);
        return Promise.resolve(jsonResponse(scanEntry({ id: 'created', barcode: body.barcode })));
      }
      if (url.includes('status=pending')) {
        return Promise.resolve(jsonResponse(scanPosts.map((barcode, index) =>
          scanEntry({ id: `created-${index}`, barcode }))));
      }
      if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);
    await screen.findByText('No pending scans.');

    const input = screen.getByLabelText(/barcode scanner input/i);
    fireEvent.change(input, { target: { value: '0123456789012' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    // A normal product barcode is posted as a scan and rendered as a card.
    expect(await screen.findByText('Barcode: 0123456789012')).toBeInTheDocument();
    expect(scanPosts).toEqual(['0123456789012']);
    // No mode switch is triggered for an ordinary barcode.
    expect(modePosts).toHaveLength(0);
  });

  it('scanning an unrecognized string still posts it as a product barcode', async () => {
    const scanPosts: string[] = [];
    const modePosts: string[] = [];
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/scanner/mode') && init?.method === 'POST') {
        const body = init.body ? (JSON.parse(String(init.body)) as { mode: string }) : { mode: '' };
        modePosts.push(body.mode);
        return Promise.resolve(jsonResponse({ mode: body.mode }));
      }
      if (url.endsWith('/api/scanner/config')) {
        return Promise.resolve(jsonResponse({ stockInBarcode: 'STOCK_IN', stockOutBarcode: 'STOCK_OUT' }));
      }
      if (url.endsWith('/api/scans') && init?.method === 'POST') {
        const body = init.body ? (JSON.parse(String(init.body)) as { barcode: string }) : { barcode: '' };
        scanPosts.push(body.barcode);
        return Promise.resolve(jsonResponse(scanEntry({ id: 'created', barcode: body.barcode })));
      }
      if (url.includes('status=pending') || url.includes('status=flagged')) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);
    await screen.findByText('No pending scans.');

    const input = screen.getByLabelText(/barcode scanner input/i);
    // 'stock_in' (lowercase) must NOT match the exact-case control string.
    fireEvent.change(input, { target: { value: 'stock_in' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    await vi.waitFor(() => expect(scanPosts).toEqual(['stock_in']));
    expect(modePosts).toHaveLength(0);
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
    await screen.findByRole('tablist');

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
    await screen.findByRole('tablist');
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

  it('Stock_In_View renders only direction === "stock_in" entries', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('status=pending')) {
        return Promise.resolve(jsonResponse([
          scanEntry({ id: 'stock-in-1', barcode: '111', direction: 'stock_in' }),
          scanEntry({ id: 'stock-out-1', barcode: '222', direction: 'stock_out' }),
          scanEntry({ id: 'null-direction-1', barcode: '333', direction: null }),
        ]));
      }
      if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);

    // Click on Stock_In tab to activate Stock_In_View
    const stockInTab = screen.getByRole('tab', { name: 'Stock in' });
    stockInTab.click();

    // Verify only the stock_in entry is rendered
    const cards = await screen.findAllByRole('article');
    expect(cards).toHaveLength(1);
    expect(within(cards[0]).getByText('Barcode: 111')).toBeInTheDocument();
  });

  it('Stock_Out_View renders direction === "stock_out" and direction === null entries, but not stock_in', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('status=pending')) {
        return Promise.resolve(jsonResponse([
          scanEntry({ id: 'stock-in-1', barcode: '111', direction: 'stock_in' }),
          scanEntry({ id: 'stock-out-1', barcode: '222', direction: 'stock_out' }),
          scanEntry({ id: 'null-direction-1', barcode: '333', direction: null }),
        ]));
      }
      if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);

    // Stock_Out_View is the default, verify both stock_out and null-direction entries are rendered
    const cards = await screen.findAllByRole('article');
    expect(cards).toHaveLength(2);
    expect(within(cards[0]).getByText('Barcode: 222')).toBeInTheDocument();
    expect(within(cards[1]).getByText('Barcode: 333')).toBeInTheDocument();
  });

  it('Stock_Out_View excludes stock_in entries even when mixed with other directions', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('status=pending')) {
        return Promise.resolve(jsonResponse([
          scanEntry({ id: 'stock-in-1', barcode: '111', direction: 'stock_in' }),
          scanEntry({ id: 'stock-in-2', barcode: '222', direction: 'stock_in' }),
          scanEntry({ id: 'stock-out-1', barcode: '333', direction: 'stock_out' }),
          scanEntry({ id: 'null-direction-1', barcode: '444', direction: null }),
        ]));
      }
      if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);

    // Stock_Out_View is the default, verify stock_in entries are NOT rendered
    const cards = await screen.findAllByRole('article');
    expect(cards).toHaveLength(2);
    expect(within(cards[0]).getByText('Barcode: 333')).toBeInTheDocument();
    expect(within(cards[1]).getByText('Barcode: 444')).toBeInTheDocument();

    // Verify stock_in entries are NOT in the document
    expect(screen.queryByText('Barcode: 111')).not.toBeInTheDocument();
    expect(screen.queryByText('Barcode: 222')).not.toBeInTheDocument();
  });

  it('switching from Stock_Out_View to Stock_In_View renders only stock_in entries', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('status=pending')) {
        return Promise.resolve(jsonResponse([
          scanEntry({ id: 'stock-in-1', barcode: '111', direction: 'stock_in' }),
          scanEntry({ id: 'stock-out-1', barcode: '222', direction: 'stock_out' }),
          scanEntry({ id: 'null-direction-1', barcode: '333', direction: null }),
        ]));
      }
      if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);

    // Initially Stock_Out_View should be active and showing stock_out and null-direction entries
    let cards = await screen.findAllByRole('article');
    expect(cards).toHaveLength(2);
    expect(within(cards[0]).getByText('Barcode: 222')).toBeInTheDocument();
    expect(within(cards[1]).getByText('Barcode: 333')).toBeInTheDocument();

    // Switch to Stock_In_View
    const stockInTab = screen.getByRole('tab', { name: 'Stock in' });
    stockInTab.click();

    // Wait for the stock_in entry to appear and verify stock_out/null entries disappear
    await screen.findByText('Barcode: 111');
    expect(screen.queryByText('Barcode: 222')).not.toBeInTheDocument();
    expect(screen.queryByText('Barcode: 333')).not.toBeInTheDocument();

    cards = screen.getAllByRole('article');
    expect(cards).toHaveLength(1);
    expect(within(cards[0]).getByText('Barcode: 111')).toBeInTheDocument();
  });

  describe('Selection clearing on tab switch', () => {
    it('clears selection when switching from Stock_Out_View to Stock_In_View', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'stock-in-1', barcode: '111', direction: 'stock_in' }),
            scanEntry({ id: 'stock-out-1', barcode: '222', direction: 'stock_out' }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      // Start in Stock_Out_View with 1 eligible entry
      const cards = await screen.findAllByRole('article');
      expect(cards).toHaveLength(1);
      expect(within(cards[0]).getByText('Barcode: 222')).toBeInTheDocument();

      // Select the entry
      const checkbox = within(cards[0]).getByRole('checkbox');
      checkbox.click();
      expect(checkbox).toBeChecked();

      // Switch to Stock_In_View
      const stockInTab = screen.getByRole('tab', { name: 'Stock in' });
      stockInTab.click();

      // Verify Stock_In_View displays the stock_in entry
      await screen.findByText('Barcode: 111');
      const cardsAfterSwitch = screen.getAllByRole('article');
      const cardsInStockInView = cardsAfterSwitch.filter((card) => {
        try {
          within(card).getByText('Barcode: 111');
          return true;
        } catch {
          return false;
        }
      });
      expect(cardsInStockInView).toHaveLength(1);

      // Verify the stock_in entry is not selected (selection was cleared)
      const checkboxInStockInView = within(cardsInStockInView[0]).getByRole('checkbox');
      expect(checkboxInStockInView).not.toBeChecked();
    });

    it('clears selection when switching from Stock_In_View to Stock_Out_View', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'stock-in-1', barcode: '111', direction: 'stock_in' }),
            scanEntry({ id: 'stock-out-1', barcode: '222', direction: 'stock_out' }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      // Switch to Stock_In_View first
      const stockInTab = screen.getByRole('tab', { name: 'Stock in' });
      stockInTab.click();

      // Verify Stock_In_View displays the stock_in entry
      let cards = await screen.findAllByRole('article');
      const cardsInStockInView = cards.filter((card) => {
        try {
          within(card).getByText('Barcode: 111');
          return true;
        } catch {
          return false;
        }
      });
      expect(cardsInStockInView).toHaveLength(1);

      // Select the entry
      const checkbox = within(cardsInStockInView[0]).getByRole('checkbox');
      checkbox.click();
      expect(checkbox).toBeChecked();

      // Switch back to Stock_Out_View
      const stockOutTab = screen.getByRole('tab', { name: 'Stock out' });
      stockOutTab.click();

      // Verify Stock_Out_View displays the stock_out entry
      await screen.findByText('Barcode: 222');
      cards = screen.getAllByRole('article');
      const cardsInStockOutView = cards.filter((card) => {
        try {
          within(card).getByText('Barcode: 222');
          return true;
        } catch {
          return false;
        }
      });
      expect(cardsInStockOutView).toHaveLength(1);

      // Verify the stock_out entry is not selected (selection was cleared)
      const checkboxInStockOutView = within(cardsInStockOutView[0]).getByRole('checkbox');
      expect(checkboxInStockOutView).not.toBeChecked();
    });

    it('clears selection when switching tabs with multiple selected entries', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'stock-in-1', barcode: '111', direction: 'stock_in' }),
            scanEntry({ id: 'stock-in-2', barcode: '222', direction: 'stock_in' }),
            scanEntry({ id: 'stock-out-1', barcode: '333', direction: 'stock_out' }),
            scanEntry({ id: 'stock-out-2', barcode: '444', direction: 'stock_out' }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      // Start in Stock_Out_View with 2 eligible entries
      let cards = await screen.findAllByRole('article');
      const outCards = cards.filter((card) => {
        try {
          within(card).getByText(/Barcode: (333|444)/);
          return true;
        } catch {
          return false;
        }
      });
      expect(outCards).toHaveLength(2);

      // Select both entries
      const checkbox1 = within(outCards[0]).getByRole('checkbox');
      const checkbox2 = within(outCards[1]).getByRole('checkbox');
      checkbox1.click();
      checkbox2.click();
      expect(checkbox1).toBeChecked();
      expect(checkbox2).toBeChecked();

      // Switch to Stock_In_View
      const stockInTab = screen.getByRole('tab', { name: 'Stock in' });
      stockInTab.click();

      // Verify Stock_In_View is now active and shows stock_in entries
      await screen.findByText('Barcode: 111');
      cards = screen.getAllByRole('article');
      const inCards = cards.filter((card) => {
        try {
          within(card).getByText(/Barcode: (111|222)/);
          return true;
        } catch {
          return false;
        }
      });
      expect(inCards).toHaveLength(2);

      // Verify none of the entries are selected
      const inCheckbox1 = within(inCards[0]).getByRole('checkbox');
      const inCheckbox2 = within(inCards[1]).getByRole('checkbox');
      expect(inCheckbox1).not.toBeChecked();
      expect(inCheckbox2).not.toBeChecked();
    });

    it('clears selection when switching back and forth between tabs', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'stock-in-1', barcode: '111', direction: 'stock_in' }),
            scanEntry({ id: 'stock-out-1', barcode: '222', direction: 'stock_out' }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      // Start in Stock_Out_View
      let cards = await screen.findAllByRole('article');
      let checkbox = within(cards[0]).getByRole('checkbox');

      // Select entry in Stock_Out_View
      checkbox.click();
      expect(checkbox).toBeChecked();

      // Switch to Stock_In_View
      const stockInTab = screen.getByRole('tab', { name: 'Stock in' });
      stockInTab.click();

      // Verify selection was cleared
      await screen.findByText('Barcode: 111');
      cards = screen.getAllByRole('article').filter((card) => {
        try {
          within(card).getByText('Barcode: 111');
          return true;
        } catch {
          return false;
        }
      });
      checkbox = within(cards[0]).getByRole('checkbox');
      expect(checkbox).not.toBeChecked();

      // Select entry in Stock_In_View
      checkbox.click();
      expect(checkbox).toBeChecked();

      // Switch back to Stock_Out_View
      const stockOutTab = screen.getByRole('tab', { name: 'Stock out' });
      stockOutTab.click();

      // Verify selection was cleared again
      await screen.findByText('Barcode: 222');
      cards = screen.getAllByRole('article').filter((card) => {
        try {
          within(card).getByText('Barcode: 222');
          return true;
        } catch {
          return false;
        }
      });
      checkbox = within(cards[0]).getByRole('checkbox');
      expect(checkbox).not.toBeChecked();
    });

    it('clears selection when using Select_All_Control then switching tabs', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'stock-in-1', barcode: '111', direction: 'stock_in' }),
            scanEntry({ id: 'stock-in-2', barcode: '222', direction: 'stock_in' }),
            scanEntry({ id: 'stock-out-1', barcode: '333', direction: 'stock_out' }),
            scanEntry({ id: 'stock-out-2', barcode: '444', direction: 'stock_out' }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      // Start in Stock_Out_View
      await screen.findByText('Barcode: 333');

      // Use Select_All_Control to select all eligible entries in Stock_Out_View
      const selectAllCheckbox = screen.getByRole('checkbox', { name: 'Select all eligible scans for batch approval' });
      selectAllCheckbox.click();

      // Verify all eligible entries are selected
      let cards = screen.getAllByRole('article').filter((card) => {
        try {
          within(card).getByText(/Barcode: (333|444)/);
          return true;
        } catch {
          return false;
        }
      });
      const checkbox1 = within(cards[0]).getByRole('checkbox');
      const checkbox2 = within(cards[1]).getByRole('checkbox');
      expect(checkbox1).toBeChecked();
      expect(checkbox2).toBeChecked();

      // Switch to Stock_In_View
      const stockInTab = screen.getByRole('tab', { name: 'Stock in' });
      stockInTab.click();

      // Verify stock_in entries are not selected (selection was cleared)
      await screen.findByText('Barcode: 111');
      cards = screen.getAllByRole('article').filter((card) => {
        try {
          within(card).getByText(/Barcode: (111|222)/);
          return true;
        } catch {
          return false;
        }
      });
      const inCheckbox1 = within(cards[0]).getByRole('checkbox');
      const inCheckbox2 = within(cards[1]).getByRole('checkbox');
      expect(inCheckbox1).not.toBeChecked();
      expect(inCheckbox2).not.toBeChecked();
    });
  });

  describe('Select_All_Control (view-scoped)', () => {
    it('is unchecked when no eligible entries exist in the Active_View', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            // Only flagged entries with no direction (not eligible because status !== 'pending')
            scanEntry({ id: 'flagged-1', barcode: '111', status: 'flagged', direction: null }),
          ]));
        }
        if (url.includes('status=flagged')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'flagged-2', barcode: '222', status: 'flagged', direction: null }),
          ]));
        }
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      const selectAllCheckbox = await screen.findByRole('checkbox', { name: 'Select all eligible scans for batch approval' });
      expect(selectAllCheckbox).not.toBeChecked();
    });

    it('is disabled when no eligible entries exist in the Active_View', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            // Only pending entries with no direction (not eligible because direction === null)
            scanEntry({ id: 'pending-1', barcode: '111', status: 'pending', direction: null }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      const selectAllCheckbox = await screen.findByRole('checkbox', { name: 'Select all eligible scans for batch approval' });
      expect(selectAllCheckbox).toBeDisabled();
    });

    it('is checked when all eligible entries in the Active_View are selected', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'stock-out-1', barcode: '111', direction: 'stock_out' }),
            scanEntry({ id: 'stock-out-2', barcode: '222', direction: 'stock_out' }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      const selectAllCheckbox = await screen.findByRole('checkbox', { name: 'Select all eligible scans for batch approval' });
      
      // Initially unchecked
      expect(selectAllCheckbox).not.toBeChecked();

      // Click to select all
      selectAllCheckbox.click();

      // Now should be checked
      expect(selectAllCheckbox).toBeChecked();

      // Verify all cards are selected
      const cards = await screen.findAllByRole('article');
      const checkboxes = cards.map((card) => within(card).getByRole('checkbox'));
      checkboxes.forEach((checkbox) => {
        expect(checkbox).toBeChecked();
      });
    });

    it('toggles only eligible entries within the Active_View', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'eligible-1', barcode: '111', status: 'pending', direction: 'stock_out' }),
            scanEntry({ id: 'eligible-2', barcode: '222', status: 'pending', direction: 'stock_out' }),
            scanEntry({ id: 'ineligible-1', barcode: '333', status: 'pending', direction: null }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      const selectAllCheckbox = await screen.findByRole('checkbox', { name: 'Select all eligible scans for batch approval' });

      // Verify we have 3 entries (2 eligible, 1 ineligible)
      const cards = await screen.findAllByRole('article');
      expect(cards).toHaveLength(3);

      // Click Select_All_Control
      selectAllCheckbox.click();

      // Verify only the 2 eligible entries are selected, not the ineligible one
      const cardCheckboxes = cards.map((card) => within(card).getByRole('checkbox'));
      expect(cardCheckboxes[0]).toBeChecked(); // eligible-1
      expect(cardCheckboxes[1]).toBeChecked(); // eligible-2
      expect(cardCheckboxes[2]).not.toBeChecked(); // ineligible-1 (direction === null)
    });

    it('Select_All_Control respects view filtering by toggling only visible eligible entries', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'stock-in-1', barcode: '111', direction: 'stock_in' }),
            scanEntry({ id: 'stock-out-1', barcode: '222', direction: 'stock_out' }),
            scanEntry({ id: 'stock-out-2', barcode: '333', direction: 'stock_out' }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      // Start in Stock_Out_View with 2 eligible entries
      let cards = await screen.findAllByRole('article');
      expect(cards).toHaveLength(2); // stock-out-1 and stock-out-2
      expect(within(cards[0]).getByText('Barcode: 222')).toBeInTheDocument();
      expect(within(cards[1]).getByText('Barcode: 333')).toBeInTheDocument();

      // Use Select_All_Control in Stock_Out_View - should select only the 2 visible entries
      const selectAllCheckbox = screen.getByRole('checkbox', { name: 'Select all eligible scans for batch approval' });
      selectAllCheckbox.click();

      // Verify both visible entries are selected
      expect(within(cards[0]).getByRole('checkbox')).toBeChecked();
      expect(within(cards[1]).getByRole('checkbox')).toBeChecked();

      // Switch to Stock_In_View (clears selection per Requirement 11)
      const stockInTab = screen.getByRole('tab', { name: 'Stock in' });
      stockInTab.click();

      // Verify we're now showing only the stock_in entry
      await screen.findByText('Barcode: 111');
      cards = screen.getAllByRole('article');
      // Filter to only entries showing stock_in content
      cards = cards.filter((card) => {
        try {
          within(card).getByText('Barcode: 111');
          return true;
        } catch {
          return false;
        }
      });
      expect(cards).toHaveLength(1);

      // The stock_in entry should NOT be selected (selection was cleared on tab switch)
      expect(within(cards[0]).getByRole('checkbox')).not.toBeChecked();

      // Use Select_All_Control in Stock_In_View - should select only this 1 entry
      const selectAllCheckboxStockIn = screen.getByRole('checkbox', { name: 'Select all eligible scans for batch approval' });
      selectAllCheckboxStockIn.click();
      expect(within(cards[0]).getByRole('checkbox')).toBeChecked();
    });


    it('preserves selection state of ineligible entries when Select_All_Control is activated', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'eligible-1', barcode: '111', status: 'pending', direction: 'stock_out' }),
            scanEntry({ id: 'ineligible-1', barcode: '222', status: 'pending', direction: null }),
            scanEntry({ id: 'ineligible-2', barcode: '333', status: 'pending', direction: null }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      // Get all entries (eligible and ineligible)
      const cards = await screen.findAllByRole('article');
      expect(cards).toHaveLength(3);

      // Manually select the ineligible entries before using Select_All_Control
      const ineligibleCheckbox1 = within(cards[1]).getByRole('checkbox');
      const ineligibleCheckbox2 = within(cards[2]).getByRole('checkbox');
      ineligibleCheckbox1.click();
      ineligibleCheckbox2.click();

      expect(ineligibleCheckbox1).toBeChecked();
      expect(ineligibleCheckbox2).toBeChecked();

      // Now use Select_All_Control to toggle all eligible entries
      const selectAllCheckbox = screen.getByRole('checkbox', { name: 'Select all eligible scans for batch approval' });
      selectAllCheckbox.click();

      // Verify:
      // - The eligible entry is now selected
      // - The ineligible entries REMAIN selected (preserved)
      const eligibleCheckbox = within(cards[0]).getByRole('checkbox');
      expect(eligibleCheckbox).toBeChecked();
      expect(ineligibleCheckbox1).toBeChecked();
      expect(ineligibleCheckbox2).toBeChecked();
    });

    it('deselects all eligible entries in Active_View when all are already selected', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'eligible-1', barcode: '111', direction: 'stock_out' }),
            scanEntry({ id: 'eligible-2', barcode: '222', direction: 'stock_out' }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      const selectAllCheckbox = await screen.findByRole('checkbox', { name: 'Select all eligible scans for batch approval' });

      // Select all
      selectAllCheckbox.click();
      expect(selectAllCheckbox).toBeChecked();

      // Deselect all
      selectAllCheckbox.click();
      expect(selectAllCheckbox).not.toBeChecked();

      // Verify all individual checkboxes are also unchecked
      const cards = screen.getAllByRole('article');
      const cardCheckboxes = cards.map((card) => within(card).getByRole('checkbox'));
      cardCheckboxes.forEach((checkbox) => {
        expect(checkbox).not.toBeChecked();
      });
    });

    it('only shows eligible entries as eligible in each view', async () => {
      const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('status=pending')) {
          return Promise.resolve(jsonResponse([
            scanEntry({ id: 'stock-in-eligible', barcode: '111', status: 'pending', direction: 'stock_in' }),
            scanEntry({ id: 'stock-out-eligible', barcode: '222', status: 'pending', direction: 'stock_out' }),
            scanEntry({ id: 'undirected', barcode: '333', status: 'pending', direction: null }),
          ]));
        }
        if (url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
        if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
        throw new Error(`Unexpected request: ${url}`);
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<MantineProvider><ScanQueuePage /></MantineProvider>);

      // Start in Stock_Out_View
      let selectAllCheckbox = await screen.findByRole('checkbox', { name: 'Select all eligible scans for batch approval' });
      
      // In Stock_Out_View, only the stock_out entry and undirected are shown
      // but only stock_out is eligible (undirected is not eligible because direction === null)
      let cards = screen.getAllByRole('article');
      // Filter to entries in Stock_Out_View (showing barcodes 222 and 333)
      cards = cards.filter((card) => {
        try {
          within(card).getByText(/Barcode: (222|333)/);
          return true;
        } catch {
          return false;
        }
      });
      expect(cards).toHaveLength(2);

      // Use Select_All_Control - should only select the stock_out entry
      selectAllCheckbox.click();

      // Find the cards again after click
      cards = screen.getAllByRole('article').filter((card) => {
        try {
          within(card).getByText(/Barcode: (222|333)/);
          return true;
        } catch {
          return false;
        }
      });

      // The first card should be the stock_out entry (222), second is undirected (333)
      let stockOutCheckbox: HTMLElement | null = null;
      let undirectedCheckbox: HTMLElement | null = null;
      for (const card of cards) {
        try {
          within(card).getByText('Barcode: 222');
          stockOutCheckbox = within(card).getByRole('checkbox');
        } catch {
          // not this card
        }
        try {
          within(card).getByText('Barcode: 333');
          undirectedCheckbox = within(card).getByRole('checkbox');
        } catch {
          // not this card
        }
      }

      expect(stockOutCheckbox).toBeChecked(); // eligible in this view
      expect(undirectedCheckbox).not.toBeChecked(); // ineligible (direction === null)

      // Switch to Stock_In_View
      const stockInTab = screen.getByRole('tab', { name: 'Stock in' });
      stockInTab.click();

      selectAllCheckbox = screen.getByRole('checkbox', { name: 'Select all eligible scans for batch approval' });
      
      // Wait for stock_in entry to appear and filter cards
      await screen.findByText('Barcode: 111');
      cards = screen.getAllByRole('article').filter((card) => {
        try {
          within(card).getByText('Barcode: 111');
          return true;
        } catch {
          return false;
        }
      });
      expect(cards).toHaveLength(1);

      const stockInCheckbox = within(cards[0]).getByRole('checkbox');
      expect(stockInCheckbox).not.toBeChecked(); // Not selected (selection was cleared on tab switch)

      // Select all in Stock_In_View
      selectAllCheckbox.click();
      expect(stockInCheckbox).toBeChecked(); // Now selected
    });
  });
});
