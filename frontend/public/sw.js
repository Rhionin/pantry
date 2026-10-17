// Scan alerts while Stockbot is closed or hidden. The page registers this
// file at the site root so a click can open the scan queue. The server sends
// a compact JSON payload: a short title, a body naming the count and the
// first item names, and a url on this origin. A path that is not on this
// origin is ignored. A visible window gets an in-app toast instead of a
// system notification.

self.addEventListener('push', (event) => {
  const payload = readPayload(event);
  const title = payload.title !== '' ? payload.title : 'Stockbot';
  const options = {
    body: payload.body,
    lang: 'en',
    icon: '/favicon.svg',
    requireInteraction: true,
    data: { url: absoluteAlertURL(payload.url, self.location.origin) },
  };
  if (payload.tag !== '') {
    options.tag = payload.tag;
  }
  event.waitUntil(presentAlert(title, options));
});

// The push subscription is userVisibleOnly. Chrome accepts a skipped
// showNotification when a window of this origin is already visible, and
// otherwise may show a generic notice or stop delivering silent pushes.
// matchAll is limited to this origin, including windows this worker does
// not control yet. The hostname stored on the subscription decides whether
// the push arrives; it does not change which windows are in this list.
function shouldSuppressSystemNotification(clients) {
  if (clients == null || typeof clients[Symbol.iterator] !== 'function') return false;
  for (const client of clients) {
    if (!client || typeof client !== 'object') continue;
    if (client.visibilityState === 'visible' || client.focused === true) return true;
  }
  return false;
}

async function presentAlert(title, options) {
  let windows = [];
  try {
    windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
  } catch {
    windows = [];
  }
  if (!shouldSuppressSystemNotification(windows)) {
    await self.registration.showNotification(title, options);
    return;
  }
  const message = {
    type: 'pantry-scan-alert-toast',
    title,
    body: typeof options.body === 'string' ? options.body : '',
    tag: typeof options.tag === 'string' ? options.tag : '',
    url: options.data && typeof options.data.url === 'string' ? options.data.url : '',
  };
  let delivered = false;
  for (const client of windows) {
    if (!shouldSuppressSystemNotification([client])) continue;
    if (typeof client.postMessage !== 'function') continue;
    try {
      client.postMessage(message);
      delivered = true;
    } catch {
      // A client can be closing. Another open window can still show the toast.
    }
  }
  // Nothing on screen took the toast. A system notification still satisfies
  // userVisibleOnly, which a silent push would not.
  if (!delivered) await self.registration.showNotification(title, options);
}

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const url = absoluteAlertURL(event.notification.data && event.notification.data.url, self.location.origin);
  event.waitUntil(openQueue(url));
});

function readPayload(event) {
  const empty = { title: '', body: '', tag: '', url: '' };
  if (!event.data) return empty;
  let raw;
  try {
    raw = event.data.json();
  } catch {
    return empty;
  }
  if (!raw || typeof raw !== 'object') return empty;
  return {
    title: typeof raw.title === 'string' ? raw.title : '',
    body: typeof raw.body === 'string' ? raw.body : '',
    tag: typeof raw.tag === 'string' ? raw.tag : '',
    url: typeof raw.url === 'string' ? raw.url : '',
  };
}

function alertPath(value) {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//')) return '/';
  return value;
}

// The phone opens this address from the notification. A root-relative review
// path such as /?scanAlert=stock_in becomes an absolute URL on this origin.
// Anything on another origin is dropped, so a tampered payload cannot
// navigate the browser away from Stockbot.
function absoluteAlertURL(value, origin) {
  const fallback = new URL('/', origin).href;
  if (typeof value !== 'string' || value === '') return fallback;
  if (value.startsWith('/')) return new URL(alertPath(value), origin).href;
  let parsed;
  try {
    parsed = new URL(value);
  } catch {
    return fallback;
  }
  if (parsed.origin !== origin) return fallback;
  if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') return fallback;
  return parsed.href;
}

function clientURL(client) {
  try {
    return new URL(client.url);
  } catch {
    return null;
  }
}

// Prefer a window already showing the review. Otherwise any Stockbot window.
// Clients from other sites are ignored so focus() cannot raise them.
function chooseNotificationClient(clients, target, origin) {
  let fallback = null;
  for (const client of clients) {
    const parsed = clientURL(client);
    if (!parsed || parsed.origin !== origin) continue;
    if (parsed.href === target) return client;
    if (fallback === null) fallback = client;
  }
  return fallback;
}

async function openQueue(url) {
  const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
  const client = chooseNotificationClient(windows, url, self.location.origin);
  if (!client) {
    await self.clients.openWindow(url);
    return;
  }
  // The open page can select the review when navigate() is missing. Firefox
  // does not implement navigate on every version.
  try {
    client.postMessage({ type: 'pantry-scan-alert', url });
  } catch {
    // A client can be closing. Navigation still opens the review.
  }
  let destination = client;
  if (typeof client.navigate === 'function') {
    try {
      const navigated = await client.navigate(url);
      const navigatedURL = navigated && clientURL(navigated);
      if (navigatedURL && navigatedURL.origin === self.location.origin) {
        destination = navigated;
      }
    } catch {
      // Firefox does not implement navigate on every version. The message
      // still opens the scan queue in a window that is already loaded.
    }
  }
  // Navigate before focus. On Firefox for Android, focus() raises the last
  // open tab unless the Stockbot window has already been moved to the review.
  // A client from another origin is never focused.
  const destinationURL = clientURL(destination);
  if (!destinationURL || destinationURL.origin !== self.location.origin) {
    await self.clients.openWindow(url);
    return;
  }
  try {
    await destination.focus();
  } catch {
    // A hidden window can refuse focus. The message still selects the queue.
  }
}
