// Scan alerts while Pantry is closed. The page registers this file at the
// site root so a click can open the scan queue. The server sends a compact
// JSON payload: a short title, a body naming the count and the first item
// names, and a url on this origin. A path that is not on this origin is ignored.

self.addEventListener('push', (event) => {
  const payload = readPayload(event);
  const title = payload.title !== '' ? payload.title : 'Pantry';
  const options = {
    body: payload.body,
    lang: 'en',
    icon: '/favicon.svg',
    requireInteraction: true,
    data: { url: alertPath(payload.url) },
  };
  if (payload.tag !== '') {
    options.tag = payload.tag;
  }
  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const path = alertPath(event.notification.data && event.notification.data.url);
  const target = new URL(path, self.location.origin).href;
  event.waitUntil(openQueue(path, target));
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

async function openQueue(path, target) {
  const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
  for (const client of windows) {
    let origin = '';
    try {
      origin = new URL(client.url).origin;
    } catch {
      continue;
    }
    if (origin !== self.location.origin) continue;
    client.postMessage({ type: 'pantry-scan-alert', url: path });
    try {
      await client.focus();
    } catch {
      // A hidden window can refuse focus. The message still selects the queue.
    }
    if (typeof client.navigate === 'function') {
      try {
        await client.navigate(target);
      } catch {
        // Firefox does not implement navigate on every version. The message
        // still opens the scan queue in a window that is already loaded.
      }
    }
    return;
  }
  await self.clients.openWindow(target);
}
