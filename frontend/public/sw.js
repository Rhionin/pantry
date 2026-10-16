// Scan alerts while Stockbot is closed. The page registers this file at the
// site root so a click can open the scan queue. The server sends a compact
// JSON payload: a short title, a body naming the count and the first item
// names, and a url on this origin. A path that is not on this origin is ignored.

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
  event.waitUntil(self.registration.showNotification(title, options));
});

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
