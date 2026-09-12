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
});
