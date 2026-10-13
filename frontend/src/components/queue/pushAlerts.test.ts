import { afterEach, describe, expect, it, vi } from 'vitest';
import { report } from '../../telemetry/client';
import {
  backgroundAlertsEnabled,
  disableScanAlerts,
  ensurePushSubscription,
  scanAlertsPreferred,
  setBackgroundAlerts,
  writeScanAlertPreference,
} from './pushAlerts';

vi.mock('../../telemetry/client', () => ({
  report: vi.fn(),
  reportApiResult: vi.fn(),
}));

class FakeNotification {
  static permission: NotificationPermission = 'granted';
}

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });

const installPush = (options?: {
  pushManager?: boolean;
  serviceWorker?: boolean;
  publicKey?: string;
  subscribe?: () => Promise<unknown>;
  saveStatus?: number;
  register?: () => Promise<unknown>;
}) => {
  const pushManager = options?.pushManager ?? true;
  const serviceWorker = options?.serviceWorker ?? true;
  if (pushManager) vi.stubGlobal('PushManager', class PushManager {});
  else vi.stubGlobal('PushManager', undefined);
  if (!serviceWorker) {
    Reflect.deleteProperty(navigator, 'serviceWorker');
  } else {
    const registration = {
      pushManager: {
        getSubscription: () => Promise.resolve(null),
        subscribe: options?.subscribe ?? (() => Promise.resolve({
          endpoint: 'https://push.example/device',
          unsubscribe: () => Promise.resolve(true),
          toJSON: () => ({
            endpoint: 'https://push.example/device',
            keys: { p256dh: 'p256', auth: 'auth' },
          }),
        })),
      },
    };
    Object.defineProperty(navigator, 'serviceWorker', {
      configurable: true,
      value: {
        register: options?.register ?? (() => Promise.resolve(registration)),
        ready: Promise.resolve(registration),
        getRegistration: () => Promise.resolve(registration),
      },
    });
  }
  vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/api/push/vapid-public-key')) {
      return Promise.resolve(jsonResponse({ publicKey: options?.publicKey ?? 'BElk' }));
    }
    if (url.endsWith('/api/push/subscriptions') && init?.method === 'POST') {
      if ((options?.saveStatus ?? 200) >= 400) {
        return Promise.resolve(jsonResponse({ error: 'Could not save this device.' }, options?.saveStatus));
      }
      return Promise.resolve(jsonResponse({ endpoint: 'https://push.example/device' }));
    }
    throw new Error(`Unexpected request: ${init?.method ?? 'GET'} ${url}`);
  });
};

describe('ensurePushSubscription', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
    localStorage.clear();
    setBackgroundAlerts(false);
    Reflect.deleteProperty(navigator, 'serviceWorker');
  });

  it('names a missing push service and reports it', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    FakeNotification.permission = 'granted';
    vi.stubGlobal('Notification', FakeNotification);
    writeScanAlertPreference(true);
    installPush({ pushManager: false, serviceWorker: false });

    await expect(ensurePushSubscription()).resolves.toEqual({ ok: false, reason: 'no-push' });
    expect(backgroundAlertsEnabled()).toBe(false);
    expect(warn).toHaveBeenCalledWith('push subscription failed: no-push');
    expect(report).toHaveBeenCalledWith({
      kind: 'js_error',
      message: 'push subscription failed: no-push',
    });
    warn.mockRestore();
  });

  it('names a failed service worker registration and reports the browser error', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    FakeNotification.permission = 'granted';
    vi.stubGlobal('Notification', FakeNotification);
    writeScanAlertPreference(true);
    installPush({
      register: () => Promise.reject(new Error("The provided scope ('/') is not allowed.")),
    });

    await expect(ensurePushSubscription()).resolves.toEqual({ ok: false, reason: 'register-failed' });
    expect(backgroundAlertsEnabled()).toBe(false);
    expect(warn).toHaveBeenCalledWith(
      "push subscription failed: register-failed: The provided scope ('/') is not allowed.",
    );
    expect(report).toHaveBeenCalledWith({
      kind: 'js_error',
      message: "push subscription failed: register-failed: The provided scope ('/') is not allowed.",
    });
    warn.mockRestore();
  });

  it('names an empty push key and reports it', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    FakeNotification.permission = 'granted';
    vi.stubGlobal('Notification', FakeNotification);
    writeScanAlertPreference(true);
    installPush({ publicKey: '' });

    await expect(ensurePushSubscription()).resolves.toEqual({ ok: false, reason: 'empty-vapid' });
    expect(warn).toHaveBeenCalledWith('push subscription failed: empty-vapid');
    expect(report).toHaveBeenCalledWith({
      kind: 'js_error',
      message: 'push subscription failed: empty-vapid',
    });
    warn.mockRestore();
  });

  it('names a rejected subscribe and reports the browser error', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    FakeNotification.permission = 'granted';
    vi.stubGlobal('Notification', FakeNotification);
    writeScanAlertPreference(true);
    installPush({
      subscribe: () => Promise.reject(new Error('Registration failed - permission denied')),
    });

    await expect(ensurePushSubscription()).resolves.toEqual({ ok: false, reason: 'subscribe-rejected' });
    expect(warn).toHaveBeenCalledWith(
      'push subscription failed: subscribe-rejected: Registration failed - permission denied',
    );
    expect(report).toHaveBeenCalledWith({
      kind: 'js_error',
      message: 'push subscription failed: subscribe-rejected: Registration failed - permission denied',
    });
    warn.mockRestore();
  });

  it('names a failed save and reports it', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    FakeNotification.permission = 'granted';
    vi.stubGlobal('Notification', FakeNotification);
    writeScanAlertPreference(true);
    installPush({ saveStatus: 500 });

    const result = await ensurePushSubscription();
    expect(result).toEqual({ ok: false, reason: 'save-failed' });
    expect(warn).toHaveBeenCalledWith(expect.stringContaining('push subscription failed: save-failed'));
    expect(report).toHaveBeenCalledWith({
      kind: 'js_error',
      message: expect.stringContaining('push subscription failed: save-failed'),
    });
    warn.mockRestore();
  });

  it('stores the subscription when the browser accepts it', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    FakeNotification.permission = 'granted';
    vi.stubGlobal('Notification', FakeNotification);
    writeScanAlertPreference(true);
    const posted: unknown[] = [];
    installPush();
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

    await expect(ensurePushSubscription()).resolves.toEqual({ ok: true });
    expect(backgroundAlertsEnabled()).toBe(true);
    expect(posted).toEqual([
      { endpoint: 'https://push.example/device', keys: { p256dh: 'p256', auth: 'auth' } },
    ]);
    expect(warn).not.toHaveBeenCalled();
    expect(report).not.toHaveBeenCalled();
    warn.mockRestore();
  });

  it('does not subscribe or warn when this device is off', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    FakeNotification.permission = 'granted';
    vi.stubGlobal('Notification', FakeNotification);
    writeScanAlertPreference(false);

    await expect(ensurePushSubscription()).resolves.toEqual({ ok: false, reason: 'not-granted' });
    expect(warn).not.toHaveBeenCalled();
    expect(report).not.toHaveBeenCalled();
    warn.mockRestore();
  });

  it('unsubscribes and deletes the server row when alerts are turned off', async () => {
    const deleted: unknown[] = [];
    let unsubscribed = false;
    const subscription = {
      endpoint: 'https://push.example/device',
      unsubscribe: () => {
        unsubscribed = true;
        return Promise.resolve(true);
      },
      toJSON: () => ({
        endpoint: 'https://push.example/device',
        keys: { p256dh: 'p256', auth: 'auth' },
      }),
    };
    const registration = {
      pushManager: {
        getSubscription: () => Promise.resolve(subscription),
        subscribe: () => Promise.resolve(subscription),
      },
    };
    Object.defineProperty(navigator, 'serviceWorker', {
      configurable: true,
      value: {
        getRegistration: () => Promise.resolve(registration),
      },
    });
    vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).endsWith('/api/push/subscriptions') && init?.method === 'DELETE') {
        deleted.push(JSON.parse(String(init.body)));
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      throw new Error(`Unexpected request: ${init?.method} ${String(input)}`);
    });
    writeScanAlertPreference(true);
    setBackgroundAlerts(true);

    await expect(disableScanAlerts()).resolves.toBeNull();
    expect(unsubscribed).toBe(true);
    expect(deleted).toEqual([{ endpoint: 'https://push.example/device' }]);
    expect(scanAlertsPreferred()).toBe(false);
    expect(backgroundAlertsEnabled()).toBe(false);
  });
});
