// Browser-side telemetry. Reports stay on the pantry server (POST /api/telemetry/client).
// Paths are stored without query strings so barcodes never leave the page URL.

export type ClientEventKind =
  | 'page_load'
  | 'route'
  | 'js_error'
  | 'unhandled_rejection'
  | 'api'
  | 'sse_error'
  | 'sse_gap'
  | 'sse_open';

export interface ClientEvent {
  kind: ClientEventKind;
  page?: string;
  route?: string;
  status?: number;
  durationMs?: number;
  ttfbMs?: number;
  domContentLoadedMs?: number;
  message?: string;
  session?: string;
  skipped?: number;
  jsResources?: number;
  jsTransferBytes?: number;
  jsEncodedBytes?: number;
  cssResources?: number;
  cssTransferBytes?: number;
  cssEncodedBytes?: number;
  firstPaint?: boolean;
}

export interface PageLoadTiming {
  durationMs: number;
  ttfbMs: number;
  domContentLoadedMs: number;
}

export interface AssetTotals {
  jsResources: number;
  jsTransferBytes: number;
  jsEncodedBytes: number;
  cssResources: number;
  cssTransferBytes: number;
  cssEncodedBytes: number;
}

interface NavigationTimingLike {
  loadEventEnd: number;
  duration: number;
  responseStart: number;
  domContentLoadedEventEnd: number;
  responseEnd?: number;
}

interface ResourceTimingLike {
  name?: string;
  initiatorType?: string;
  transferSize?: number;
  encodedBodySize?: number;
}

const sessionKey = 'pantry-telemetry-session';
const eventTypes = ['scan', 'scan_processing', 'scan_processing_failed', 'inventory', 'scanner_mode'] as const;

// Routes the first screen actually requests. Exact paths only, so a lookup
// URL cannot be recorded as the product list.
const firstPaintRoutes = new Set([
  '/api/inventory',
  '/api/scans',
  '/api/products',
  '/api/scanner/config',
  '/api/shopping-list',
  '/api/shopping-list/considerations',
  '/api/providers',
  '/api/build',
]);

// Calls that start shortly after the load event still belong to first paint:
// React effects run just after the document finishes.
const firstPaintGraceMs = 1500;

let lastSSEId = 0;
let loadEventAtMs = Number.POSITIVE_INFINITY;

export function sanitizePage(raw: string | undefined): string {
  if (!raw) return '';
  let path = raw.trim();
  const hash = path.indexOf('#');
  if (hash >= 0) path = path.slice(0, hash);
  const query = path.indexOf('?');
  if (query >= 0) path = path.slice(0, query);
  if (path.includes('://')) {
    try {
      path = new URL(path).pathname;
    } catch {
      return '';
    }
  }
  return path.length > 128 ? path.slice(0, 128) : path;
}

export function sanitizeMessage(raw: string | undefined): string {
  if (!raw) return '';
  const cleaned = raw.replace(/\?[^\s]*/g, '').replace(/\s+/g, ' ').trim();
  return cleaned.length > 180 ? cleaned.slice(0, 180) : cleaned;
}

export function routeFromPath(path: string): string {
  const withoutQuery = path.split('?')[0] ?? path;
  return sanitizePage(withoutQuery) || withoutQuery;
}

// sseGap returns how many event ids were skipped between last and nextId.
// A non-numeric id is ignored. The first id establishes a baseline and is not a gap.
export function sseGap(last: number, nextId: string): { next: number; skipped: number } | null {
  if (!nextId) return null;
  const next = Number(nextId);
  if (!Number.isInteger(next) || next <= 0) return null;
  const skipped = last > 0 && next > last + 1 ? next - last - 1 : 0;
  return { next, skipped };
}

export function resetSSETracking(): void {
  lastSSEId = 0;
}

export function resetPageLoadTracking(): void {
  loadEventAtMs = Number.POSITIVE_INFINITY;
}

// pageLoadTiming reads a navigation entry. Inside the load listener,
// loadEventEnd is still 0, and duration is defined as loadEventEnd minus
// startTime, so it is 0 too. nowMs is performance.now(), which shares that
// time origin and is the full-load time we can actually see.
export function pageLoadTiming(nav: NavigationTimingLike, nowMs: number): PageLoadTiming {
  let durationMs = 0;
  if (nav.loadEventEnd > 0) {
    durationMs = nav.loadEventEnd;
  } else if (nowMs > 0) {
    durationMs = nowMs;
  } else if (nav.domContentLoadedEventEnd > 0) {
    durationMs = nav.domContentLoadedEventEnd;
  } else if ((nav.responseEnd ?? 0) > 0) {
    durationMs = nav.responseEnd ?? 0;
  }
  return {
    durationMs,
    ttfbMs: nav.responseStart > 0 ? nav.responseStart : 0,
    domContentLoadedMs: nav.domContentLoadedEventEnd > 0 ? nav.domContentLoadedEventEnd : 0,
  };
}

export function notePageLoad(nowMs: number): void {
  if (Number.isFinite(nowMs) && nowMs >= 0) {
    loadEventAtMs = nowMs;
  }
}

// firstPaintRoute marks an API call that started before the load event, or
// within a short grace after it. Later polls of the same route are not first
// paint. The flag is all that is sent; the route string is sanitized separately.
export function firstPaintRoute(route: string, startedAtMs: number): boolean {
  const path = routeFromPath(route);
  if (!firstPaintRoutes.has(path)) return false;
  return startedAtMs <= loadEventAtMs + firstPaintGraceMs;
}

// assetTotals sums JS and CSS resource timing. Names and query strings are
// not copied into the result.
export function assetTotals(entries: readonly ResourceTimingLike[]): AssetTotals {
  const totals: AssetTotals = {
    jsResources: 0,
    jsTransferBytes: 0,
    jsEncodedBytes: 0,
    cssResources: 0,
    cssTransferBytes: 0,
    cssEncodedBytes: 0,
  };
  for (const entry of entries) {
    const kind = assetKind(entry);
    if (kind === 'js') {
      totals.jsResources += 1;
      totals.jsTransferBytes += positiveBytes(entry.transferSize);
      totals.jsEncodedBytes += positiveBytes(entry.encodedBodySize);
    } else if (kind === 'css') {
      totals.cssResources += 1;
      totals.cssTransferBytes += positiveBytes(entry.transferSize);
      totals.cssEncodedBytes += positiveBytes(entry.encodedBodySize);
    }
  }
  return totals;
}

function assetKind(entry: ResourceTimingLike): 'js' | 'css' | '' {
  const resourcePath = stripResourceName(entry.name);
  if (resourcePath.endsWith('.js') || resourcePath.endsWith('.mjs')) return 'js';
  if (resourcePath.endsWith('.css')) return 'css';
  if (entry.initiatorType === 'script') return 'js';
  if (entry.initiatorType === 'css') return 'css';
  return '';
}

function stripResourceName(name: string | undefined): string {
  if (!name) return '';
  const noHash = name.split('#')[0] ?? '';
  const noQuery = (noHash.split('?')[0] ?? '').toLowerCase();
  return noQuery;
}

function positiveBytes(value: number | undefined): number {
  if (value === undefined || !Number.isFinite(value) || value <= 0) return 0;
  return Math.round(value);
}

export function deliver(event: ClientEvent, fetchImpl: typeof fetch = fetch): void {
  const body: ClientEvent = {
    kind: event.kind,
    page: event.page ? sanitizePage(event.page) : undefined,
    route: event.route ? sanitizePage(event.route) : undefined,
    status: event.status,
    durationMs: event.durationMs,
    ttfbMs: event.ttfbMs,
    domContentLoadedMs: event.domContentLoadedMs,
    message: event.message ? sanitizeMessage(event.message) : undefined,
    session: event.session,
    skipped: event.skipped,
    jsResources: event.jsResources,
    jsTransferBytes: event.jsTransferBytes,
    jsEncodedBytes: event.jsEncodedBytes,
    cssResources: event.cssResources,
    cssTransferBytes: event.cssTransferBytes,
    cssEncodedBytes: event.cssEncodedBytes,
    firstPaint: event.firstPaint,
  };
  try {
    const pending = fetchImpl('/api/telemetry/client', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      keepalive: true,
    });
    void Promise.resolve(pending).catch(() => {
      // A failed report must not surface in the UI.
    });
  } catch {
    // Telemetry must not affect the page.
  }
}

function reportingEnabled(): boolean {
  // Vitest sets MODE to "test". Reports must not hit the fetch mocks the page tests install.
  return import.meta.env.MODE !== 'test';
}

function currentPage(): string {
  try {
    return sanitizePage(window.location.pathname);
  } catch {
    return '';
  }
}

function sessionID(): string {
  try {
    const existing = sessionStorage.getItem(sessionKey);
    if (existing) return existing;
    const id = crypto.randomUUID().replace(/-/g, '').slice(0, 16);
    sessionStorage.setItem(sessionKey, id);
    return id;
  } catch {
    return '';
  }
}

export function report(event: ClientEvent): void {
  if (!reportingEnabled()) return;
  deliver({
    ...event,
    page: event.page ?? currentPage(),
    session: event.session ?? sessionID(),
  });
}

export function reportRoute(page: string): void {
  report({ kind: 'route', page });
}

export function reportApiResult(route: string, status: number, durationMs: number, startedAtMs = 0): void {
  if (!reportingEnabled()) return;
  report({
    kind: 'api',
    route: routeFromPath(route),
    status,
    durationMs,
    firstPaint: firstPaintRoute(route, startedAtMs) || undefined,
  });
}

export function noteSSEEvent(event: { lastEventId?: string }): void {
  const gap = sseGap(lastSSEId, event.lastEventId ?? '');
  if (!gap) return;
  if (gap.skipped > 0) {
    report({ kind: 'sse_gap', skipped: gap.skipped });
  }
  lastSSEId = gap.next;
}

// trackEventSource listens to every event type, not only the ones this page
// renders, so an id gap means a missed frame rather than "this page ignores
// inventory events".
export function trackEventSource(source: EventSource): void {
  for (const type of eventTypes) {
    source.addEventListener(type, (event) => {
      noteSSEEvent(event as MessageEvent);
    });
  }
  source.addEventListener('error', () => {
    report({ kind: 'sse_error' });
  });
  report({ kind: 'sse_open' });
}

export function installBrowserTelemetry(): void {
  if (!reportingEnabled()) return;

  window.addEventListener('error', (event) => {
    report({ kind: 'js_error', message: event.message });
  });
  window.addEventListener('unhandledrejection', (event) => {
    const reason: unknown = event.reason;
    const message = reason instanceof Error
      ? reason.message
      : typeof reason === 'string'
        ? reason
        : 'unhandled rejection';
    report({ kind: 'unhandled_rejection', message });
  });

  const sendLoad = () => {
    const now = performance.now();
    notePageLoad(now);
    const nav = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined;
    if (!nav) {
      report({ kind: 'page_load' });
      return;
    }
    const timing = pageLoadTiming(nav, now);
    const assets = assetTotals(performance.getEntriesByType('resource') as PerformanceResourceTiming[]);
    report({ kind: 'page_load', ...timing, ...assets });
  };
  if (document.readyState === 'complete') {
    sendLoad();
  } else {
    window.addEventListener('load', sendLoad, { once: true });
  }
}
