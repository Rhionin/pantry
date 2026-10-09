import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ScanQueuePage } from './ScanQueuePage';
import { openScanAlert } from './browserScanAlert';

class FakeNotification {
  static permission: NotificationPermission = 'default';
  static requestPermission = vi.fn(() => Promise.resolve(FakeNotification.permission));
  onclick: (() => void) | null = null;
  title: string;
  options?: NotificationOptions;
  constructor(title: string, options?: NotificationOptions) {
    this.title = title;
    this.options = options;
  }
  close() {}
}

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  url: string;
  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }
  addEventListener() {}
  close() {}
}

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

describe('ScanQueuePage scan alerts', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    sessionStorage.clear();
  });

  it('does not request notification permission while the queue loads', async () => {
    FakeNotification.permission = 'default';
    FakeNotification.requestPermission = vi.fn(() => Promise.resolve<NotificationPermission>('default'));
    vi.stubGlobal('Notification', FakeNotification);
    vi.stubGlobal('EventSource', FakeEventSource);
    vi.stubGlobal('fetch', (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('status=pending') || url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      if (url.endsWith('/api/scanner/config')) {
        return Promise.resolve(jsonResponse({
          stockInBarcode: 'STOCK_IN',
          stockOutBarcode: 'STOCK_OUT',
          currentMode: 'stock_in',
          connected: false,
        }));
      }
      throw new Error(`Unexpected request: ${url}`);
    });

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);
    expect(await screen.findByRole('tab', { name: /^Stock in/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Notify me about scans' })).not.toBeInTheDocument();
    expect(screen.queryByRole('switch', { name: 'Scan alerts' })).not.toBeInTheDocument();
    expect(FakeNotification.requestPermission).not.toHaveBeenCalled();
  });

  it('selects the tab a scan alert named', async () => {
    const modePosts: string[] = [];
    vi.stubGlobal('EventSource', FakeEventSource);
    vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/scanner/mode') && init?.method === 'POST') {
        const body = JSON.parse(String(init.body)) as { mode: string };
        modePosts.push(body.mode);
        return Promise.resolve(jsonResponse({ mode: body.mode }));
      }
      if (url.includes('status=pending') || url.includes('status=flagged')) return Promise.resolve(jsonResponse([]));
      if (url === '/api/inventory' || url === '/api/products') return Promise.resolve(jsonResponse([]));
      if (url.endsWith('/api/scanner/config')) {
        return Promise.resolve(jsonResponse({
          stockInBarcode: 'STOCK_IN',
          stockOutBarcode: 'STOCK_OUT',
          currentMode: 'stock_in',
          connected: true,
        }));
      }
      throw new Error(`Unexpected request: ${url}`);
    });

    openScanAlert('stock_out');
    render(<MantineProvider><ScanQueuePage /></MantineProvider>);

    expect(await screen.findByRole('tab', { name: /^Stock out/ })).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(modePosts).toEqual(['stock_out']));
    expect(screen.getByText('Scans remove from stock')).toBeInTheDocument();
  });
});
