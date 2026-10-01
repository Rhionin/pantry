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
}

const sessionKey = 'pantry-telemetry-session';
const eventTypes = ['scan', 'scan_processing', 'scan_processing_failed', 'inventory', 'scanner_mode'] as const;

let lastSSEId = 0;

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

export function reportApiResult(route: string, status: number, durationMs: number): void {
  report({
    kind: 'api',
    route: routeFromPath(route),
    status,
    durationMs,
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
    const nav = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined;
    if (!nav) {
      report({ kind: 'page_load' });
      return;
    }
    const duration = nav.loadEventEnd > 0 ? nav.loadEventEnd : nav.duration;
    report({
      kind: 'page_load',
      durationMs: duration,
      ttfbMs: nav.responseStart,
      domContentLoadedMs: nav.domContentLoadedEventEnd,
    });
  };
  if (document.readyState === 'complete') {
    sendLoad();
  } else {
    window.addEventListener('load', sendLoad, { once: true });
  }
}
