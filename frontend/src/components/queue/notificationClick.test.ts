import { describe, expect, it } from 'vitest';
import workerSource from '../../../public/sw.js?raw';

const origin = 'https://pantry.test';
const reviewPath = '/?scanAlert=stock_in';
const reviewURL = `${origin}/?scanAlert=stock_in`;

interface TraceCall {
  op: string;
  client?: string;
  url?: string;
  options?: { type?: string; includeUncontrolled?: boolean };
}

interface FakeClient {
  url: string;
  focus: () => Promise<FakeClient>;
  navigate: (url: string) => Promise<FakeClient | null>;
  postMessage: (data: unknown) => void;
}

interface ShownNotification {
  title: string;
  options: { data?: { url?: string }; tag?: string };
}

interface WorkerApi {
  chooseNotificationClient: (list: Array<{ url: string }>, target: string, pageOrigin: string) => { url: string } | null;
  absoluteAlertURL: (value: unknown, pageOrigin: string) => string;
}

// The service worker is a classic script. Load that same source so the test
// exercises the client selection the phone runs, not a TypeScript copy.
const loadWorkerSource = new Function(
  'self',
  'URL',
  'console',
  `${workerSource}\nreturn { chooseNotificationClient, absoluteAlertURL };`,
) as (self: unknown, urlConstructor: typeof URL, console: Console) => WorkerApi;

function fakeClient(url: string, trace: TraceCall[], navigateTo?: (url: string) => Promise<FakeClient | null>): FakeClient {
  const client: FakeClient = {
    url,
    async focus() {
      trace.push({ op: 'focus', client: url });
      return client;
    },
    navigate: navigateTo ?? (async (next) => {
      trace.push({ op: 'navigate', client: url, url: next });
      return client;
    }),
    postMessage(data) {
      const message = data as { url?: string };
      trace.push({ op: 'postMessage', client: url, url: message?.url });
    },
  };
  return client;
}

function loadWorker(clients: FakeClient[], trace: TraceCall[] = []) {
  const listeners = new Map<string, (event: unknown) => void>();
  const shown: { current: ShownNotification | null } = { current: null };
  const self = {
    location: { origin, href: `${origin}/sw.js` },
    addEventListener(type: string, handler: (event: unknown) => void) {
      listeners.set(type, handler);
    },
    registration: {
      async showNotification(title: string, options: ShownNotification['options']) {
        shown.current = { title, options };
      },
    },
    clients: {
      async matchAll(options: TraceCall['options']) {
        trace.push({ op: 'matchAll', options });
        return clients;
      },
      async openWindow(url: string) {
        trace.push({ op: 'openWindow', url });
        return null;
      },
    },
  };
  const api = loadWorkerSource(self, URL, console);
  return { trace, listeners, shown, api };
}

async function click(worker: ReturnType<typeof loadWorker>, dataURL: unknown) {
  let pending: Promise<unknown> = Promise.resolve();
  worker.listeners.get('notificationclick')?.({
    notification: {
      data: { url: dataURL },
      close() {
        worker.trace.push({ op: 'close' });
      },
    },
    waitUntil(promise: Promise<unknown>) {
      pending = promise;
    },
  });
  await pending;
}

async function push(worker: ReturnType<typeof loadWorker>, payload: unknown) {
  let pending: Promise<unknown> = Promise.resolve();
  worker.listeners.get('push')?.({
    data: payload === null ? null : { json: () => payload },
    waitUntil(promise: Promise<unknown>) {
      pending = promise;
    },
  });
  await pending;
}

describe('notification click client selection', () => {
  it('stores an absolute pending-scan review url on the notification', async () => {
    const worker = loadWorker([]);
    await push(worker, {
      title: 'Stock in batch is ready',
      body: '1 scan is waiting',
      tag: 'pantry-scan-batch-stock_in',
      url: reviewPath,
    });
    expect(worker.shown.current?.options.data?.url).toBe(reviewURL);
    expect(worker.shown.current?.options.tag).toBe('pantry-scan-batch-stock_in');
  });

  it('keeps a same-origin absolute url and drops every other address', () => {
    const worker = loadWorker([]);
    expect(worker.api.absoluteAlertURL(reviewPath, origin)).toBe(reviewURL);
    expect(worker.api.absoluteAlertURL(reviewURL, origin)).toBe(reviewURL);
    expect(worker.api.absoluteAlertURL('https://evil.example/phish', origin)).toBe(`${origin}/`);
    expect(worker.api.absoluteAlertURL('//evil.example/phish', origin)).toBe(`${origin}/`);
    expect(worker.api.absoluteAlertURL('javascript:alert(1)', origin)).toBe(`${origin}/`);
    expect(worker.api.absoluteAlertURL('', origin)).toBe(`${origin}/`);
    expect(worker.api.absoluteAlertURL(null, origin)).toBe(`${origin}/`);
  });

  it('prefers the Pantry window already on the review, otherwise the first Pantry window', () => {
    const worker = loadWorker([]);
    const wikipedia = { url: 'https://en.wikipedia.org/wiki/Pantry' };
    const inventory = { url: `${origin}/inventory` };
    const otherReview = { url: `${origin}/?scanAlert=stock_out` };
    const review = { url: reviewURL };
    const broken = { url: 'http://[' };

    expect(worker.api.chooseNotificationClient(
      [wikipedia, inventory, otherReview, review],
      reviewURL,
      origin,
    )).toBe(review);
    expect(worker.api.chooseNotificationClient(
      [wikipedia, broken, inventory, otherReview],
      reviewURL,
      origin,
    )).toBe(inventory);
    expect(worker.api.chooseNotificationClient([wikipedia, broken], reviewURL, origin)).toBeNull();
  });

  it('navigates the review tab and focuses it, never a tab from another site', async () => {
    const trace: TraceCall[] = [];
    const wikipedia = fakeClient('https://en.wikipedia.org/wiki/Pantry', trace);
    const inventory = fakeClient(`${origin}/inventory`, trace);
    const review = fakeClient(reviewURL, trace);
    const worker = loadWorker([wikipedia, inventory, review], trace);

    await click(worker, reviewPath);

    expect(trace).toContainEqual({
      op: 'matchAll',
      options: { type: 'window', includeUncontrolled: true },
    });
    expect(trace.filter((call) => call.op === 'navigate')).toEqual([
      { op: 'navigate', client: reviewURL, url: reviewURL },
    ]);
    expect(trace.filter((call) => call.op === 'focus')).toEqual([
      { op: 'focus', client: reviewURL },
    ]);
    const navigateAt = trace.findIndex((call) => call.op === 'navigate');
    const focusAt = trace.findIndex((call) => call.op === 'focus');
    expect(navigateAt).toBeGreaterThan(-1);
    expect(focusAt).toBeGreaterThan(navigateAt);
    expect(trace.some((call) => call.op === 'openWindow')).toBe(false);
    expect(trace.some((call) => call.client === wikipedia.url)).toBe(false);
    expect(trace.some((call) => call.client === inventory.url)).toBe(false);
  });

  it('navigates the only Pantry window to the review, then focuses that window', async () => {
    const trace: TraceCall[] = [];
    const wikipedia = fakeClient('https://en.wikipedia.org/wiki/Pantry', trace);
    const inventory = fakeClient(`${origin}/inventory`, trace);
    const worker = loadWorker([wikipedia, inventory], trace);

    await click(worker, reviewURL);

    expect(trace.filter((call) => call.op === 'navigate')).toEqual([
      { op: 'navigate', client: `${origin}/inventory`, url: reviewURL },
    ]);
    expect(trace.filter((call) => call.op === 'focus')).toEqual([
      { op: 'focus', client: `${origin}/inventory` },
    ]);
    expect(trace.some((call) => call.client === wikipedia.url)).toBe(false);
  });

  it('opens the review when no Pantry window is open and does not focus another site', async () => {
    const trace: TraceCall[] = [];
    const wikipedia = fakeClient('https://en.wikipedia.org/wiki/Pantry', trace);
    const worker = loadWorker([wikipedia], trace);

    await click(worker, reviewPath);

    expect(trace.filter((call) => call.op === 'openWindow')).toEqual([
      { op: 'openWindow', url: reviewURL },
    ]);
    expect(trace.some((call) => call.op === 'focus' || call.op === 'navigate')).toBe(false);
    expect(trace.some((call) => call.client === wikipedia.url)).toBe(false);
  });

  it('opens Pantry itself when the payload url is on another origin', async () => {
    const trace: TraceCall[] = [];
    const wikipedia = fakeClient('https://en.wikipedia.org/wiki/Pantry', trace);
    const worker = loadWorker([wikipedia], trace);

    await click(worker, 'https://evil.example/phish');

    expect(trace.filter((call) => call.op === 'openWindow')).toEqual([
      { op: 'openWindow', url: `${origin}/` },
    ]);
    expect(trace.some((call) => call.op === 'focus')).toBe(false);
  });

  it('focuses the Pantry window when navigate() is missing or returns another origin', async () => {
    const trace: TraceCall[] = [];
    const inventory = fakeClient(`${origin}/inventory`, trace, async () => {
      trace.push({ op: 'navigate', client: inventory.url, url: reviewURL });
      return fakeClient('https://evil.example/landed', trace);
    });
    const worker = loadWorker([inventory], trace);

    await click(worker, reviewPath);

    expect(trace.filter((call) => call.op === 'focus')).toEqual([
      { op: 'focus', client: `${origin}/inventory` },
    ]);
    expect(trace.some((call) => call.client === 'https://evil.example/landed')).toBe(false);

    const fallbackTrace: TraceCall[] = [];
    const withoutNavigate = fakeClient(`${origin}/inventory`, fallbackTrace);
    Reflect.deleteProperty(withoutNavigate, 'navigate');
    const fallback = loadWorker([withoutNavigate], fallbackTrace);
    await click(fallback, reviewPath);
    expect(fallbackTrace.filter((call) => call.op === 'focus')).toEqual([
      { op: 'focus', client: `${origin}/inventory` },
    ]);
    expect(fallbackTrace.some((call) => call.op === 'navigate')).toBe(false);
  });
});
