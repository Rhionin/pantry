// Scan alerts while Pantry is closed. The page registers this file at the
// site root so a click can open the scan queue. Payloads are JSON from the
// pantry server. A path that is not on this origin is ignored.

self.addEventListener('push', (event) => {
  let payload = {};
  try {
    payload = event.data ? event.data.json() : {};
  } catch {
    payload = {};
  }
  const title = typeof payload.title === 'string' && payload.title !== '' ? payload.title : 'Pantry';
  const options = {
    body: typeof payload.body === 'string' ? payload.body : '',
    lang: 'en',
    icon: '/favicon.svg',
    requireInteraction: true,
    data: { url: alertPath(payload.url) },
  };
  if (typeof payload.tag === 'string' && payload.tag !== '') {
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
