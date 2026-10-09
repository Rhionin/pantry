import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import type { ProcessingFailure, ScanEntry } from '../../types';
import { ScanAlertsSetting } from '../settings/ScanAlertsSetting';
import { ScanAlertHost } from './ScanAlertHost';
import { SCAN_ALERTS_ASKED_KEY, SCAN_ALERTS_ENABLED_KEY } from './scanAlertPreference';

class FakeNotification {
  static permission: NotificationPermission = 'granted';
  static instances: FakeNotification[] = [];
  static requestPermission = vi.fn();
  onclick: (() => void) | null = null;
  title: string;
  options?: NotificationOptions;
  constructor(title: string, options?: NotificationOptions) {
    this.title = title;
    this.options = options;
    FakeNotification.instances.push(this);
  }
  close() {}
}

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  url: string;
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
  close() {}
  dispatch(type: string, payload: unknown) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data: JSON.stringify(payload) } as MessageEvent);
    }
  }
}

const LocationProbe = () => {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}</div>;
};

const scan = (overrides: Partial<ScanEntry>): ScanEntry => ({
  id: 'scan-1',
  userId: 'user-1',
  barcode: '111',
  scannedAt: '2026-03-20T10:00:00Z',
  direction: 'stock_out',
  unitCount: 1,
  expiresAt: null,
  status: 'flagged',
  productId: null,
  product: null,
  committedAt: null,
  createdAt: '2026-03-20T10:00:00Z',
  ...overrides,
});

describe('ScanAlertHost', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    sessionStorage.clear();
    localStorage.clear();
    FakeNotification.instances = [];
    FakeEventSource.instances = [];
  });

  const renderHost = (path = '/inventory') => {
    vi.stubGlobal('Notification', FakeNotification);
    vi.stubGlobal('EventSource', FakeEventSource);
    render(
      <MemoryRouter initialEntries={[path]}>
        <LocationProbe />
        <Routes>
          <Route path="*" element={<ScanAlertHost />} />
        </Routes>
      </MemoryRouter>,
    );
    return FakeEventSource.instances[0];
  };

  it('does not ask for permission and notifies immediately for a failure and an unrecognized product', () => {
    const source = renderHost();
    expect(FakeNotification.requestPermission).not.toHaveBeenCalled();

    source.dispatch('scan_processing_failed', {
      id: 'lookup-1',
      barcode: '999',
      message: "Couldn't look up that barcode.",
    } satisfies ProcessingFailure);
    source.dispatch('scan', scan({ id: 'flag-1', barcode: '000111' }));

    expect(FakeNotification.instances.map((notification) => notification.title)).toEqual([
      'Scan failed',
      'Unrecognized product',
    ]);
    expect(FakeNotification.instances[0]?.options?.body).toBe(
      "Couldn't look up that barcode. Open the scan queue to try again.",
    );
    expect(FakeNotification.instances[0]?.options?.requireInteraction).toBe(true);
    expect(FakeNotification.instances[1]?.options?.body).toBe(
      'Barcode 000111 needs a product. Open the scan queue to match it.',
    );
  });

  it('focuses the scan queue for the direction named by the notification', () => {
    const source = renderHost('/inventory');
    source.dispatch('scan', scan({ id: 'flag-1', barcode: '000111', direction: 'stock_in' }));
    const notification = FakeNotification.instances[0];
    expect(notification).toBeDefined();
    notification?.onclick?.();
    expect(screen.getByTestId('location')).toHaveTextContent('/');
    expect(sessionStorage.getItem('pantry-scan-alert-direction')).toBe('stock_in');
  });

  it('asks for notification permission on the first interaction and never again', () => {
    FakeNotification.permission = 'default';
    const requestPermission = vi.fn(() => Promise.resolve<NotificationPermission>('default'));
    FakeNotification.requestPermission = requestPermission;
    const source = renderHost();
    expect(requestPermission).not.toHaveBeenCalled();

    fireEvent.pointerDown(document.body);
    expect(requestPermission).toHaveBeenCalledOnce();
    expect(localStorage.getItem(SCAN_ALERTS_ASKED_KEY)).toBe('1');

    fireEvent.pointerDown(document.body);
    fireEvent.keyDown(document.body, { key: 'a' });
    source.dispatch('scan', scan({ id: 'scan-2', barcode: '222' }));
    expect(requestPermission).toHaveBeenCalledOnce();

    cleanup();
    FakeEventSource.instances = [];
    renderHost();
    fireEvent.keyDown(document.body, { key: 'Enter' });
    FakeEventSource.instances.at(-1)?.dispatch('scan', scan({ id: 'scan-3', barcode: '333' }));
    expect(requestPermission).toHaveBeenCalledOnce();
  });

  it('asks on the first keypress when the page has not been clicked', () => {
    FakeNotification.permission = 'default';
    const requestPermission = vi.fn(() => Promise.resolve<NotificationPermission>('default'));
    FakeNotification.requestPermission = requestPermission;
    renderHost();

    fireEvent.keyDown(document.body, { key: 'Tab' });
    expect(requestPermission).toHaveBeenCalledOnce();
    fireEvent.pointerDown(document.body);
    expect(requestPermission).toHaveBeenCalledOnce();
  });

  it('asks on the first scan and does not ask again', () => {
    FakeNotification.permission = 'default';
    const requestPermission = vi.fn(() => Promise.resolve<NotificationPermission>('default'));
    FakeNotification.requestPermission = requestPermission;
    const source = renderHost();

    source.dispatch('scan', scan({ status: 'pending', productId: 'product-1' }));
    expect(requestPermission).toHaveBeenCalledOnce();
    expect(FakeNotification.instances).toHaveLength(0);

    source.dispatch('scan_processing_failed', {
      id: 'lookup-1',
      barcode: '999',
      message: "Couldn't look up that barcode.",
    } satisfies ProcessingFailure);
    fireEvent.pointerDown(document.body);
    expect(requestPermission).toHaveBeenCalledOnce();
  });

  it('does not ask when scan alerts are switched off', () => {
    FakeNotification.permission = 'default';
    const requestPermission = vi.fn(() => Promise.resolve<NotificationPermission>('default'));
    FakeNotification.requestPermission = requestPermission;
    localStorage.setItem(SCAN_ALERTS_ENABLED_KEY, 'off');
    const source = renderHost();

    fireEvent.pointerDown(document.body);
    fireEvent.keyDown(document.body, { key: 'a' });
    source.dispatch('scan', scan({ id: 'scan-2' }));
    expect(requestPermission).not.toHaveBeenCalled();
    expect(localStorage.getItem(SCAN_ALERTS_ASKED_KEY)).toBeNull();
  });

  it('switching scan alerts off suppresses notifications', () => {
    FakeNotification.permission = 'granted';
    FakeNotification.requestPermission = vi.fn();
    vi.stubGlobal('Notification', FakeNotification);
    vi.stubGlobal('EventSource', FakeEventSource);
    render(
      <MantineProvider>
        <MemoryRouter initialEntries={['/inventory']}>
          <ScanAlertHost />
          <ScanAlertsSetting />
        </MemoryRouter>
      </MantineProvider>,
    );
    const source = FakeEventSource.instances[0];
    source.dispatch('scan', scan({ id: 'flag-1', barcode: '000111' }));
    expect(FakeNotification.instances).toHaveLength(1);

    const toggle = screen.getByRole('switch', { name: 'Scan alerts' });
    fireEvent.pointerDown(toggle);
    fireEvent.click(toggle);

    expect(FakeNotification.requestPermission).not.toHaveBeenCalled();
    expect(toggle).not.toBeChecked();
    expect(localStorage.getItem(SCAN_ALERTS_ENABLED_KEY)).toBe('off');

    source.dispatch('scan', scan({ id: 'flag-2', barcode: '000222' }));
    expect(FakeNotification.instances).toHaveLength(1);
  });
});
