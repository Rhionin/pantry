import { deletePushSubscription, getVapidPublicKey, savePushSubscription } from '../../api/client';
import type { ScanDirection } from '../../types';

let background = false;
const listeners = new Set<(on: boolean) => void>();

export function backgroundAlertsEnabled(): boolean {
  return background;
}

export function setBackgroundAlerts(on: boolean): void {
  background = on;
  for (const listener of listeners) listener(on);
}

export function onBackgroundAlerts(listener: (on: boolean) => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function directionFromAlertURL(url: string): ScanDirection | null {
  try {
    const value = new URL(url, window.location.origin).searchParams.get('scanAlert');
    if (value === 'stock_in' || value === 'stock_out') return value;
  } catch {
    return null;
  }
  return null;
}

function applicationServerKey(publicKey: string): BufferSource {
  const padding = '='.repeat((4 - (publicKey.length % 4)) % 4);
  const base64 = (publicKey + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(base64);
  const bytes = new Uint8Array(raw.length);
  for (let index = 0; index < raw.length; index += 1) {
    bytes[index] = raw.charCodeAt(index);
  }
  return bytes;
}

async function dropBrowserSubscription(): Promise<void> {
  if (!('serviceWorker' in navigator)) return;
  const registration = await navigator.serviceWorker.getRegistration('/');
  const subscription = await registration?.pushManager.getSubscription();
  if (!subscription) return;
  const endpoint = subscription.endpoint;
  try {
    await subscription.unsubscribe();
  } catch {
    // The server row is what would keep sending. Drop that even if the
    // browser already forgot the subscription.
  }
  if (endpoint !== '') {
    try {
      await deletePushSubscription(endpoint);
    } catch {
      // The next launch tries again. A leftover row expires when the push
      // service answers 404 or 410.
    }
  }
}

// ensurePushSubscription registers /sw.js and stores the browser's push
// endpoint. It does not ask for permission. Call it after the person has
// already allowed notifications, including on a later visit.
export async function ensurePushSubscription(): Promise<boolean> {
  if (typeof Notification === 'undefined') {
    setBackgroundAlerts(false);
    return false;
  }
  if (Notification.permission === 'denied') {
    void dropBrowserSubscription();
    setBackgroundAlerts(false);
    return false;
  }
  if (Notification.permission !== 'granted') return false;
  if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
    setBackgroundAlerts(false);
    return false;
  }
  try {
    const registration = await navigator.serviceWorker.register('/sw.js', {
      scope: '/',
      updateViaCache: 'none',
    });
    const ready = await navigator.serviceWorker.ready;
    const pushManager = ready.pushManager ?? registration.pushManager;
    let subscription = await pushManager.getSubscription();
    if (subscription === null) {
      const key = await getVapidPublicKey();
      if (key.publicKey === '') {
        setBackgroundAlerts(false);
        return false;
      }
      subscription = await pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: applicationServerKey(key.publicKey),
      });
    }
    const body = subscription.toJSON();
    if (!body.endpoint || !body.keys?.p256dh || !body.keys.auth) {
      setBackgroundAlerts(false);
      return false;
    }
    await savePushSubscription({
      endpoint: body.endpoint,
      keys: { p256dh: body.keys.p256dh, auth: body.keys.auth },
    });
    setBackgroundAlerts(true);
    return true;
  } catch {
    setBackgroundAlerts(false);
    return false;
  }
}
