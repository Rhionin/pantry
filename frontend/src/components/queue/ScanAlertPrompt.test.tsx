import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ScanAlertPrompt } from './ScanAlertPrompt';
import { ScanQueuePage } from './ScanQueuePage';
import { openScanAlert } from './browserScanAlert';
import { setBackgroundAlerts } from './pushAlerts';

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

describe('ScanAlertPrompt', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    sessionStorage.clear();
    setBackgroundAlerts(false);
    Reflect.deleteProperty(navigator, 'serviceWorker');
    window.history.replaceState(null, '', '/');
  });

  it('asks only after the button is used', async () => {
    FakeNotification.permission = 'default';
    FakeNotification.requestPermission.mockClear();
    vi.stubGlobal('Notification', FakeNotification);
    render(<ScanAlertPrompt />);

    expect(FakeNotification.requestPermission).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Notify me about scans' })).toHaveAccessibleDescription(
      /Failed scans and unrecognized products alert right away/,
    );

    FakeNotification.permission = 'granted';
    fireEvent.click(screen.getByRole('button', { name: 'Notify me about scans' }));

    expect(FakeNotification.requestPermission).toHaveBeenCalledOnce();
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Notify me about scans' })).not.toBeInTheDocument();
    });
  });

  it('says alerts stay in this tab when push is not available', async () => {
    FakeNotification.permission = 'granted';
    vi.stubGlobal('Notification', FakeNotification);
    render(<ScanAlertPrompt />);
    expect(await screen.findByRole('status')).toHaveTextContent('Scan alerts are on while this tab is open.');
    expect(screen.queryByRole('button', { name: 'Notify me about scans' })).not.toBeInTheDocument();
  });

  it('stores a push subscription and says alerts work while Pantry is closed', async () => {
    FakeNotification.permission = 'granted';
    vi.stubGlobal('Notification', FakeNotification);
    vi.stubGlobal('PushManager', class PushManager {});
    const subscription = {
      endpoint: 'https://push.example/device',
      unsubscribe: () => Promise.resolve(true),
      toJSON: () => ({
        endpoint: 'https://push.example/device',
        keys: { p256dh: 'p256', auth: 'auth' },
      }),
    };
    const registration = {
      pushManager: {
        getSubscription: () => Promise.resolve(null),
        subscribe: () => Promise.resolve(subscription),
      },
    };
    Object.defineProperty(navigator, 'serviceWorker', {
      configurable: true,
      value: {
        register: () => Promise.resolve(registration),
        ready: Promise.resolve(registration),
        getRegistration: () => Promise.resolve(registration),
        addEventListener: () => {},
        removeEventListener: () => {},
      },
    });
    const posted: unknown[] = [];
    vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/push/vapid-public-key')) {
        return Promise.resolve(jsonResponse({ publicKey: 'BElk' }));
      }
      if (url.endsWith('/api/push/subscriptions') && init?.method === 'POST') {
        posted.push(JSON.parse(String(init.body)));
        return Promise.resolve(jsonResponse({ endpoint: 'https://push.example/device' }));
      }
      throw new Error(`Unexpected request: ${url}`);
    });

    render(<ScanAlertPrompt />);
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Scan alerts are on for this browser, including when Pantry is closed.',
    );
    expect(posted).toEqual([
      { endpoint: 'https://push.example/device', keys: { p256dh: 'p256', auth: 'auth' } },
    ]);
  });

  it('explains a blocked permission without asking again', () => {
    FakeNotification.permission = 'denied';
    FakeNotification.requestPermission = vi.fn();
    vi.stubGlobal('Notification', FakeNotification);
    render(<ScanAlertPrompt />);
    expect(screen.getByText(/Scan alerts are blocked/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Notify me about scans' })).not.toBeInTheDocument();
    expect(FakeNotification.requestPermission).not.toHaveBeenCalled();
  });

  it('explains when the browser cannot notify an open tab', () => {
    vi.stubGlobal('Notification', undefined);
    render(<ScanAlertPrompt />);
    expect(screen.getByText('This browser cannot show notifications from an open tab.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Notify me about scans' })).not.toBeInTheDocument();
  });
});

describe('ScanQueuePage scan alerts', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    sessionStorage.clear();
    setBackgroundAlerts(false);
    window.history.replaceState(null, '', '/');
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
    expect(await screen.findByRole('button', { name: 'Notify me about scans' })).toBeInTheDocument();
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

  it('selects the tab named by a service worker open', async () => {
    const modePosts: string[] = [];
    window.history.replaceState(null, '', '/?scanAlert=stock_out');
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

    render(<MantineProvider><ScanQueuePage /></MantineProvider>);

    expect(await screen.findByRole('tab', { name: /^Stock out/ })).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(modePosts).toEqual(['stock_out']));
    expect(window.location.search).not.toContain('scanAlert');
  });
});
